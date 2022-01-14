/*
Copyright 2022.
*/

package controllers

import (
	"context"
	"github.com/go-logr/logr"
	"github.com/rogpeppe/go-internal/semver"
	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"strings"
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
	rs         = "ReplicaSet"
	deployment = "Deployment"
	managedKey = "secure-controller"
)

var (
	managed = map[string]string{
		managedKey: "true",
	}
)

//+kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch;update;patch
//+kubebuilder:rbac:groups=apps,resources=replicasets,verbs=get;list;watch;update;patch
//+kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;update;patch
//+kubebuilder:rbac:groups=apps,resources=nodes,verbs=get;list;watch;update;patch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Pod object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.10.0/pkg/reconcile

func checkSkippedNamespaces(str string, subs ...string) bool {
	for _, su := range subs {
		if strings.Contains(str, su) {
			return true
		}
	}
	return false
}

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

	if pod.Spec.NodeName == "" {
		log.Info("pod has no node yet")
		return ctrl.Result{}, nil
	}

	if checkSkippedNamespaces(pod.Namespace, "kube-", "secure-controller", "local-path-storage") {
		log.Info("skipping pod due to being in privileged namespace")
		return ctrl.Result{}, nil
	}

	log = log.WithValues("node_name", pod.Spec.NodeName)

	if len(pod.OwnerReferences) == 0 {
		log.Info("pod has no owner")
	}

	// Get all labels up the tree for replicasets and deployments etc
	labels, err := r.GetLabelsAll(ctx, &pod)
	if err != nil {
		log.Error(err, "error getting annotation tree")
		return ctrl.Result{}, err
	}

	var minContainerVersion string
	var minKubeleteVersion string
	if x, ok := labels[containerdVersionAnnotation]; ok {
		minContainerVersion = x
	}
	if y, ok := labels[minKubeletVersionAnnotation]; ok {
		minKubeleteVersion = y
	}

	// if either of the labels aren't present, nothing to do
	if len(minContainerVersion) == 0 || len(minKubeleteVersion) == 0 {
		return ctrl.Result{}, nil
	}

	if err := r.addLabel(ctx, &pod); err != nil {
		return ctrl.Result{}, err
	}

	var node corev1.Node
	if err := r.Get(ctx, client.ObjectKey{Name: pod.Spec.NodeName}, &node); err != nil {
		log.Error(err, "could not get info on pod node")
	}

	nodeContainerRuntime := node.Status.NodeInfo.ContainerRuntimeVersion
	nodeKubeletVersion := node.Status.NodeInfo.KubeletVersion

	vs := node.Status.VolumesAttached
	// TODO: use the output from this to build the functionality around sandboxing insecure workloads
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

func (r *PodReconciler) GetLabelsAll(ctx context.Context, obj metav1.Object) (map[string]string, error) {
	var labels map[string]string
	log := r.Log.WithValues("object", obj.GetName())
	// Get obj labels
	labels = mergeMaps(labels, obj.GetLabels())
	var err error

	// Get OwnerRef
	if len(obj.GetOwnerReferences()) > 0 {
		for _, o := range obj.GetOwnerReferences() {
			log.Info("got owner", "kind", o.Kind)
			lbl := make(map[string]string)
			switch o.Kind {
			case rs:
				lbl, err = r.getRSLabels(ctx, o.Name, obj.GetNamespace())
				if err != nil {
					return labels, err
				}
			case deployment:
				lbl, err = r.getDepLabels(ctx, o.Name, obj.GetNamespace())
				if err != nil {
					return labels, err
				}
			}
			labels = mergeMaps(labels, lbl)

		}
	}

	return labels, err
}

func (r *PodReconciler) getRSLabels(ctx context.Context, name, namespace string) (map[string]string, error) {
	annotations := make(map[string]string)

	res, err := r.GetReplicaSet(ctx, name, namespace)
	if err != nil {
		return annotations, err
	}

	// add our "managed" annotation
	return r.GetLabelsAll(ctx, res)
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

func (r *PodReconciler) getDepLabels(ctx context.Context, name, namespace string) (map[string]string, error) {
	labels := make(map[string]string)

	dep, err := r.getDeployment(ctx, name, namespace)
	if err != nil {
		return labels, err
	}

	if err := r.addLabel(ctx, dep); err != nil {
		return labels, err
	}

	return r.GetLabelsAll(ctx, dep)
}

func (r *PodReconciler) getDeployment(ctx context.Context, name, namespace string) (*v1.Deployment, error) {
	var dep v1.Deployment
	var err error
	obj := client.ObjectKey{Name: name, Namespace: namespace}

	if err := r.Get(ctx, obj, &dep); err != nil {
		if apierrors.IsNotFound(err) {
			r.Log.Error(err, "could not find testfiles owner", "deployment_name", name)
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

func (r *PodReconciler) addLabel(ctx context.Context, obj client.Object) error {
	obj.SetLabels(mergeMaps(obj.GetLabels(), managed))
	err := r.Update(ctx, obj)
	if err != nil {
		return err
	}

	return r.Get(ctx, client.ObjectKey{Name: obj.GetName(), Namespace: obj.GetNamespace()}, obj)
}
