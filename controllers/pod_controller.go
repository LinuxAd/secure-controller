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
	//noSensMount                 = "no-sensitive-mount"
	rs                = "ReplicaSet"
	deployment        = "Deployment"
	managedAnnotation = "secure-controller"
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
	//_ = k8sLog.FromContext(ctx)
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
			log.Info("got owner", "kind", o.Kind)
			switch o.Kind {
			case rs:
				ann, err := r.getReplicaSetAnnotations(ctx, o.Name, obj.GetNamespace())
				if err != nil {
					return annotations, err
				}
				annotations = mergeMaps(annotations, ann)
			case deployment:
				ann, err := r.getDeploymentAnnotations(ctx, o.Name, obj.GetNamespace())
				if err != nil {
					return annotations, err
				}
				annotations = mergeMaps(annotations, ann)
			}
		}
	}

	return annotations, err
}

func (r *PodReconciler) addAnnotation(ctx context.Context, obj client.Object) error {
	current := obj.GetAnnotations()
	managed := map[string]string{
		managedAnnotation: "true",
	}

	obj.SetAnnotations(mergeMaps(current, managed))

	return r.Update(ctx, obj)
}

func (r *PodReconciler) getReplicaSetAnnotations(ctx context.Context, name, namespace string) (map[string]string, error) {
	annotations := make(map[string]string)

	res, err := r.GetReplicaSet(ctx, name, namespace)
	if err != nil {
		return annotations, err
	}

	// add our "managed" annotation
	if err := r.addAnnotation(ctx, res); err != nil {
		return annotations, err
	}

	return r.GetAllAnnotations(ctx, res)
}

func (r *PodReconciler) GetReplicaSet(ctx context.Context, name, namespace string) (*v1.ReplicaSet, error) {
	var res v1.ReplicaSet
	var err error
	obj := client.ObjectKey{Name: name, Namespace: namespace}
	if err := r.Get(ctx, obj, &res); err != nil {
		if apierrors.IsNotFound(err) {
			r.Log.Error(err, "could not find replicaset owner", "replicaset_name", name)
			return &res, nil
		}
	}
	return &res, err
}

func (r *PodReconciler) getDeploymentAnnotations(ctx context.Context, name, namespace string) (map[string]string, error) {
	annotations := make(map[string]string)

	dep, err := r.getDeployment(ctx, name, namespace)
	if err != nil {
		return annotations, err
	}

	if err := r.addAnnotation(ctx, dep); err != nil {
		return annotations, err
	}

	return r.GetAllAnnotations(ctx, dep)
}

func (r *PodReconciler) getDeployment(ctx context.Context, name, namespace string) (*v1.Deployment, error) {
	var dep v1.Deployment
	var err error
	obj := client.ObjectKey{Name: name, Namespace: namespace}

	if err := r.Get(ctx, obj, &dep); err != nil {
		if apierrors.IsNotFound(err) {
			r.Log.Error(err, "could not find deployment owner", "deployment_name", name)
			return &dep, nil
		}
	}
	return &dep, err
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

// SetupWithManager sets up the controller with the Manager.
func (r *PodReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Pod{}).
		Complete(r)
}
