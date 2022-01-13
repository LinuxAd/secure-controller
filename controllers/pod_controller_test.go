package controllers

import (
	"context"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"time"
)

var _ = Describe("Pod Controller", func() {
	const (
		DeployName      = "test-deployment"
		DeployNameSpace = "default"
		Image           = "nginx:latest"
		DeployReplicas  = 3

		timeout  = time.Second * 20
		interval = time.Millisecond * 250
	)

	replicas := int32(DeployReplicas)

	Context("When creating a new deployment", func() {
		It("Should add annotation to pods to say managed by", func() {
			By("By creating a new deployment")
			ctx := context.Background()
			deployment := &appsv1.Deployment{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "apps/v1",
					Kind:       "Deployment",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      DeployName,
					Namespace: DeployNameSpace,
					Labels: map[string]string{
						"app": "nginx",
					},
					Annotations: map[string]string{
						"wam.com/min-containerd-version": "v1.5.2",
						"mwam.com/min-kubelet-version":   "v1.21.1",
						"mwam.com/no-sensitive-mount":    "true",
					},
				},
				Spec: appsv1.DeploymentSpec{
					Replicas: &replicas,
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"app": "nginx",
						},
					},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{
								"app": "nginx",
							},
						},
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "nginx",
									Image: Image,
									Ports: []corev1.ContainerPort{
										{
											ContainerPort: 80,
										},
									},
								},
							},
						},
					},
				},
			}

			// create the deployment
			Expect(k8sClient.Create(ctx, deployment)).Should(Succeed())

			deploymentKey := types.NamespacedName{Name: DeployName, Namespace: DeployNameSpace}
			createdDeployment := &appsv1.Deployment{}

			Eventually(func() bool {
				err := k8sClient.Get(ctx, deploymentKey, createdDeployment)
				if err != nil {
					return false
				}
				return true
			}, timeout, interval).Should(BeTrue())

			By("Checking the deployment annotations")

			//Eventually(func() string {
			//	err := k8sClient.Get(ctx, deploymentKey, createdDeployment)
			//	if err != nil {
			//		return ""
			//	}
			//
			//	val, ok := createdDeployment.Annotations[managedAnnotation]
			//	if !ok {
			//		return ""
			//	}
			//	return val
			//
			//}, timeout, interval).Should(Equal(managedAnnotation), "could not find expected annotation: %s", managedAnnotation)
		})
	})
})
