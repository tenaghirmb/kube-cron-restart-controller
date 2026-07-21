/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package e2e

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	cronrestartv1 "uni.com/cronrestart/api/v1"
)

var _ = Describe("CronRestarter E2E", func() {
	const (
		namespace = "e2e-cron-restart"
		timeout   = 3 * time.Minute
		interval  = 5 * time.Second
	)

	BeforeEach(func() {
		By("ensuring the e2e namespace exists")
		ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}
		err := k8sClient.Get(ctx, types.NamespacedName{Name: namespace}, ns)
		if err != nil && errors.IsNotFound(err) {
			Expect(k8sClient.Create(ctx, ns)).To(Succeed())
		} else {
			Expect(err).NotTo(HaveOccurred())
		}
	})

	It("should restart the target deployment on the configured schedule", func() {
		restartTargetName := fmt.Sprintf("e2e-restart-target-%d", time.Now().UnixNano())
		cronName := fmt.Sprintf("e2e-cronrestarter-%d", time.Now().UnixNano())
		DeferCleanup(func() {
			_ = k8sClient.Delete(ctx, &cronrestartv1.CronRestarter{ObjectMeta: metav1.ObjectMeta{Name: cronName, Namespace: namespace}})
			_ = k8sClient.Delete(ctx, &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: restartTargetName, Namespace: namespace}})
		})

		By("creating the target deployment")
		deployment := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      restartTargetName,
				Namespace: namespace,
			},
			Spec: appsv1.DeploymentSpec{
				Replicas: ptrInt32(1),
				Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "e2e-test"}},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "e2e-test"}},
					Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "nginx", Image: "nginx:1.25"}}},
				},
			},
		}
		Expect(k8sClient.Create(ctx, deployment)).To(Succeed())

		By("creating the CronRestarter resource")
		cron := &cronrestartv1.CronRestarter{
			ObjectMeta: metav1.ObjectMeta{
				Name:      cronName,
				Namespace: namespace,
			},
			Spec: cronrestartv1.CronRestarterSpec{
				Schedule:         "* * * * *",
				RestartTargetRef: cronrestartv1.RestartTargetRef{ApiVersion: "apps/v1", Kind: "Deployment", Name: restartTargetName},
				MisfirePolicy:    "Ignore",
			},
		}
		Expect(k8sClient.Create(ctx, cron)).To(Succeed())

		By("waiting for the deployment to receive a restart annotation")
		Eventually(func() bool {
			updated := &appsv1.Deployment{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: restartTargetName, Namespace: namespace}, updated); err != nil {
				return false
			}
			ann := updated.Spec.Template.Annotations
			return ann != nil && ann["kubectl.kubernetes.io/restartedAt"] != ""
		}, timeout, interval).Should(BeTrue())
	})
})

func ptrInt32(v int32) *int32 { return &v }
