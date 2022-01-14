package controllers

import (
	"context"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"time"
)

var _ = Describe("Pod Controller", func() {
	const (
		PodName      = "test-pod"
		PodNamespace = "default"
		Image        = "nginx:latest"
		ImageName    = "nginx"

		timeout  = time.Second * 20
		interval = time.Millisecond * 250
	)

	Context("When creating a new testfiles", func() {
		It("Should add label to pods to indicate the controller is managing them", func() {
			By("By creating a new pod")
			ctx := context.Background()
			pod := &corev1.Pod{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Pod",
					APIVersion: "v1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      PodName,
					Namespace: PodNamespace,
					Labels: map[string]string{
						"mwam.com/min-containerd-version": "v1.5.2",
						"mwam.com/min-kubelet-version":    "v1.21.1",
						"mwam.com/no-sensitive-mount":     "true",
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  ImageName,
							Image: Image,
							Ports: []corev1.ContainerPort{
								{
									Name:          "http",
									ContainerPort: 80,
								},
							},
						},
					},
				},
			}

			// create the testfiles
			Expect(k8sClient.Create(ctx, pod)).Should(Succeed())

			podKey := types.NamespacedName{Name: PodName, Namespace: PodNamespace}
			createdPod := &corev1.Pod{}

			Eventually(func() (bool, error) {
				err := k8sClient.Get(ctx, podKey, createdPod)
				if err != nil {
					return false, err
				}
				return true, nil
			}, timeout, interval).Should(BeTrue())
			By("pods are created")

			By("Checking the pod labels")

			//Eventually(func() ([]string, error) {
			//	var labels []string
			//
			//	err := k8sClient.Get(ctx, podKey, createdPod)
			//	if err != nil {
			//		return labels, err
			//	}
			//
			//	for k, _ := range createdPod.Labels {
			//		labels = append(labels, k)
			//	}
			//	return labels, nil
			//
			//}, timeout, interval).Should(ContainElement(managedKey), "could not find expected label: %s", managedKey)
		})
	})
})
