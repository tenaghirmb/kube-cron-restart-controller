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

package controller

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	autorestartv1 "uni.com/cronrestart/api/v1"
)

var _ = Describe("CronRestarter Controller", func() {
	Context("When reconciling a CronRestarter resource", func() {
		const (
			resourceName = "test-cronrestarter"
			deployName   = "restart-target-deploy"
			namespace    = "default"

			timeout  = time.Minute * 2
			interval = time.Millisecond * 250
		)

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: namespace,
		}
		deployNamespacedName := types.NamespacedName{
			Name:      deployName,
			Namespace: namespace,
		}

		BeforeEach(func() {
			By("1. 准备被重启的目标 Deployment 资源")
			targetDeploy := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      deployName,
					Namespace: namespace,
				},
				Spec: appsv1.DeploymentSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "test-nginx"},
					},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{"app": "test-nginx"},
						},
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "nginx",
									Image: "nginx:1.25",
								},
							},
						},
					},
				},
			}

			// 如果 Deployment 不存在，则创建它
			err := k8sClient.Get(ctx, deployNamespacedName, &appsv1.Deployment{})
			if err != nil && errors.IsNotFound(err) {
				Expect(k8sClient.Create(ctx, targetDeploy)).To(Succeed())
			}

			By("2. 创建自定义 CR CronRestarter 实例")
			cronrestarter := &autorestartv1.CronRestarter{}
			err = k8sClient.Get(ctx, typeNamespacedName, cronrestarter)
			if err != nil && errors.IsNotFound(err) {
				resource := &autorestartv1.CronRestarter{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: namespace,
					},
					Spec: autorestartv1.CronRestarterSpec{
						Schedule: "* * * * *", // 1分钟执行一次
						RestartTargetRef: autorestartv1.RestartTargetRef{
							ApiVersion: "apps/v1",
							Kind:       "Deployment",
							Name:       deployName,
						},
						MisfirePolicy: "Ignore",
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			By("清理测试中创建的 CronRestarter 实例")
			resource := &autorestartv1.CronRestarter{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			if err == nil {
				Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
			}

			By("清理测试中创建的 Deployment 实例")
			deploy := &appsv1.Deployment{}
			err = k8sClient.Get(ctx, deployNamespacedName, deploy)
			if err == nil {
				Expect(k8sClient.Delete(ctx, deploy)).To(Succeed())
			}
		})

		It("应该成功触发目标 Deployment 的滚动重启", func() {
			By("异步观察：检查目标 Deployment 的 PodTemplate 里的 annotations 是否存在重启时间戳")

			updatedDeployment := &appsv1.Deployment{}

			Eventually(func() bool {
				err := k8sClient.Get(ctx, deployNamespacedName, updatedDeployment)
				if err != nil {
					return false
				}

				// 获取底层 PodTemplate 的 Annotations 列表
				annotations := updatedDeployment.Spec.Template.Annotations
				if annotations == nil {
					return false
				}

				// 💡 提示：这里断言的 Key 取决于你在 Reconciler 代码中
				// 是往 Deployment 写入了哪个 Key 来标识重启。
				// 常见写法是 "kubectl.kubernetes.io/restartedAt"
				_, exists := annotations["kubectl.kubernetes.io/restartedAt"]
				return exists
			}, timeout, interval).Should(BeTrue(), "Controller 应该在触发调谐后，往关联 Deployment 注入重启时间戳注解")
		})
	})
})
