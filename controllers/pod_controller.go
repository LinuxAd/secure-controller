/*
Copyright 2022.
*/

package controllers

import (
	"context"
	"github.com/go-logr/logr"
	"github.com/rogpeppe/go-internal/semver"
	v1 "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sLog "sigs.k8s.io/controller-rutime/pkg/log"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// PodReconciler reconciles a Deployment object
type PodReconciler struct {
	Log logr.Logger
	client.Client
	Scheme *runtime.Scheme
}

const (
	containerdVersionAnnotation = "min-containerd-version"
	minKubeletVersionAnnotation = "min-kubelet-version"
	noSensMount                 = "no-sensitive-mount"
	rs                          = "ReplicaSet"
	deployment                  = "Deployment"
	managedAnnotation           = "secure-controller"
)

var (
	secureNodeSelector = map[string]string{
		"secure": "true",
	}
)

//+kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch;update;patch
//+kubebuilder:rbac:groups=apps,resources=replicasets,verbs=get;list;watch
//+kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch
//+kubebuilder:rbac:groups=apps,resources=nodes,verbs=get;list;watch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Pod object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.10.0/pkg/reconcile

func (r *PodReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = k8sLog.FromContext(ctx)
	log := r.Log.WithValues("pod", req.Name, "namespace", req.Namespace)

	log.Info("reconcile loop started")

	// TODO(user): your logic here
	var pod corev1.Pod
	if err := r.Get(ctx, req.NamespacedName, &pod); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		log.Error(err, "unable to fetch pod")
	}
	if strings.Contains(pod.Namespace, "kube-") || strings.Contains(pod.Namespace, "secure-controller") {
		log.Info("skipping pod")
		return ctrl.Result{}, nil
	}

	log = log.WithValues("node_name", pod.Spec.NodeName)

	if len(pod.OwnerReferences) == 0 {
		log.Info("pod has no owner")
		// check the pod's annotations?
	}

	// Get all annotations up the tree for replicasets and deployments etc
	annotations, err := r.GetAllAnnotations(ctx, &pod)
	if err != nil {
		log.Error(err, "error getting annotation tree")
		return ctrl.Result{}, err
	}

	var minContainerVersion string
	var minKubeleteVersion string
	// handle containerdVersion
	if x, ok := annotations[containerdVersionAnnotation]; ok {
		minContainerVersion = x
	}
	if y, ok := annotations[minKubeletVersionAnnotation]; ok {
		minKubeleteVersion = y
	}

	// if either of the annotations aren't present, nothing to do
	if len(minContainerVersion) == 0 || len(minKubeleteVersion) == 0 {
		return ctrl.Result{}, nil
	}

	var node corev1.Node
	if err := r.Get(ctx, client.ObjectKey{Name: pod.Spec.NodeName}, &node); err != nil {
		log.Error(err, "could not get info on pod node")
	}

	nodeContainerRuntime := node.Status.NodeInfo.ContainerRuntimeVersion
	nodeKubeletVersion := node.Status.NodeInfo.KubeletVersion

	vs := node.Status.VolumesAttached
	for i, v := range vs {
		log.Info("node has volumes attached", "num", i, "vol_name", v.Name, "vol_dev_path", v.DevicePath)
	}

	log.Info("current node runtime versions",
		"got_containerRuntime", nodeContainerRuntime,
		"got_kubeletVersion", node.Status.NodeInfo.KubeletVersion)

	if len(minContainerVersion) > 0 {
		if !MinVersionMet(nodeContainerRuntime, minContainerVersion) {
			log.Info("minimum containerd runtime not met, deleting pod")
			return ctrl.Result{}, r.Delete(ctx, &pod)
		}
		log.Info("minimum containerd version met")
	}
	if len(minKubeleteVersion) > 0 {
		if !MinVersionMet(nodeKubeletVersion, minKubeleteVersion) {
			log.Info("minimum kubelet version not met, deleting pod")
			// in this case I'm just going to delete the pod, so it's rescheduled for now.
			return ctrl.Result{}, r.Delete(ctx, &pod)
		}
		log.Info("minimum kubelet version met")
	}
	log.Info("reconcile loop ended")
	return ctrl.Result{}, nil
}

func MinVersionMet(got, want string) bool {
	// the result will be 0 if got == want, -1 if got < want, or +1 if got > want
	switch semver.Compare(got, want) {
	case 0, 1:
		return true
	case -1:
		return false
	default:
		return true
	}
}

func (r *PodReconciler) GetAllAnnotations(ctx context.Context, obj metav1.Object) (map[string]string, error) {
	var annotations map[string]string
	log := r.Log.WithValues("object", obj.GetName())
	// Get obj annotations
	annotations = mergeMaps(annotations, obj.GetAnnotations())
	var err error

	// Get OwnerRef
	if len(obj.GetOwnerReferences()) > 0 {
		for _, o := range obj.GetOwnerReferences() {
			// TODO: rewrite this as it's super messy and not DRY
			switch o.Kind {
			case rs:
				log.Info("got replicaset owner")
				var res v1.ReplicaSet
				oo := client.ObjectKey{Name: o.Name, Namespace: obj.GetNamespace()}
				if err := r.Get(ctx,
					oo,
					&res); err != nil {
					if apierrors.IsNotFound(err) {
						return annotations, nil
					}
					return annotations, err
				}

				x, err := r.GetAllAnnotations(ctx, &res)
				if err != nil {
					if apierrors.IsNotFound(err) {
						return annotations, nil
					}
					return annotations, err
				}

				res.Annotations = addAnnotation(res.Annotations)

				if err := r.Update(ctx, &res); err != nil {
					return annotations, err
				}

				annotations = mergeMaps(annotations, x)
			case deployment:
				log.Info("got deployment owner")
				var dep v1.Deployment
				if err := r.Get(ctx,
					client.ObjectKey{Name: o.Name, Namespace: obj.GetNamespace()},
					&dep); err != nil {
					return annotations, err
				}

				x, err := r.GetAllAnnotations(ctx, &dep)
				if err != nil {
					return annotations, err
				}
				annotations = mergeMaps(annotations, x)
			}
		}
	}

	return annotations, err
}

func (r *PodReconciler) GetReplicaSet(ctx context.Context, name, namespace string) *v1.ReplicaSet {
	var res v1.ReplicaSet
	oo := client.ObjectKey{Name: name, Namespace: namespace}
	if err := r.Get(ctx,
		oo,
		&res); err != nil {
		if apierrors.IsNotFound(err) {
			return annotations, nil
		}
		return annotations, err
	}
}

func addAnnotation(anno map[string]string) map[string]string {
	return mergeMaps(anno, map[string]string{
		managedAnnotation: "true",
	})
}

func mergeMaps(ms ...map[string]string) map[string]string {
	res := map[string]string{}

	for _, m := range ms {
		for k, v := range m {
			res[k] = v
		}
	}
	return res
}

//{
//"apiVersion": "v1",
//"kind": "Pod",
//"metadata": {
//"creationTimestamp": "2022-01-11T09:29:03Z",
//"generateName": "test-dep-fcc75fd64-",
//"labels": {
//"app": "test-dep",
//"pod-template-hash": "fcc75fd64"
//},
//"name": "test-dep-fcc75fd64-8672s",
//"namespace": "default",
//"ownerReferences": [
//{
//"apiVersion": "apps/v1",
//"blockOwnerDeletion": true,
//"controller": true,
//"kind": "ReplicaSet",
//"name": "test-dep-fcc75fd64",
//"uid": "1d56360b-3a84-48e0-886c-8d767d008ada"
//}
//],
//"resourceVersion": "697",
//"uid": "a97b0f38-e708-4dac-8443-adde4dad4b21"
//},

// SetupWithManager sets up the controller with the Manager.
func (r *PodReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Pod{}).
		Complete(r)
}
