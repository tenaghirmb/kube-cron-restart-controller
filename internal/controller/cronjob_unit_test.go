package controller

import (
	"context"
	"testing"
	"time"

	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	cronrestartv1 "uni.com/cronrestart/api/v1"
	"uni.com/cronrestart/pkg/constants"
	cronutils "uni.com/cronrestart/pkg/cron"
)

func TestCronRestarterJobFactoryWithTimezone(t *testing.T) {
	g := NewWithT(t)

	instance := &cronrestartv1.CronRestarter{
		ObjectMeta: metav1.ObjectMeta{Name: "example", Namespace: "default"},
		Spec: cronrestartv1.CronRestarterSpec{
			Schedule: "0 0 * * *",
			Timezone: "Asia/Shanghai",
			RestartTargetRef: cronrestartv1.RestartTargetRef{
				ApiVersion: "apps/v1",
				Kind:       "Deployment",
				Name:       "target",
			},
		},
	}

	job, err := CronRestarterJobFactory(instance, nil, nil, record.NewFakeRecorder(1))
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(job.SchedulePlan()).To(Equal("CRON_TZ=Asia/Shanghai 0 0 * * *"))
}

func TestCronRestarterJobFactoryIgnoresTimezoneWithDescriptor(t *testing.T) {
	g := NewWithT(t)

	instance := &cronrestartv1.CronRestarter{
		ObjectMeta: metav1.ObjectMeta{Name: "example", Namespace: "default"},
		Spec: cronrestartv1.CronRestarterSpec{
			Schedule: "@daily",
			Timezone: "Asia/Shanghai",
			RestartTargetRef: cronrestartv1.RestartTargetRef{
				ApiVersion: "apps/v1",
				Kind:       "Deployment",
				Name:       "target",
			},
		},
	}

	job, err := CronRestarterJobFactory(instance, nil, nil, record.NewFakeRecorder(1))
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(job.SchedulePlan()).To(Equal("@daily"))
}

func TestCronRestarterJobFactoryValidatesTargetRef(t *testing.T) {
	g := NewWithT(t)

	instance := &cronrestartv1.CronRestarter{
		ObjectMeta: metav1.ObjectMeta{Name: "example", Namespace: "default"},
		Spec: cronrestartv1.CronRestarterSpec{
			Schedule: "* * * * *",
			RestartTargetRef: cronrestartv1.RestartTargetRef{
				ApiVersion: "apps/v1",
				Kind:       "",
				Name:       "target",
			},
		},
	}

	_, err := CronRestarterJobFactory(instance, nil, nil, record.NewFakeRecorder(1))
	g.Expect(err).To(HaveOccurred())
}

func TestIsTodayOffDetectsExcludeDate(t *testing.T) {
	g := NewWithT(t)

	g.Expect(IsTodayOff([]string{"* * * * *"})).To(BeTrue())
	g.Expect(IsTodayOff([]string{"invalid schedule"})).To(BeFalse())
}

func TestRestartRefUpdatesDeploymentAnnotationAndCronStatus(t *testing.T) {
	g := NewWithT(t)

	scheme := runtime.NewScheme()
	g.Expect(clientgoscheme.AddToScheme(scheme)).To(Succeed())
	g.Expect(cronrestartv1.AddToScheme(scheme)).To(Succeed())

	now := metav1.Now()
	cron := &cronrestartv1.CronRestarter{
		TypeMeta:   metav1.TypeMeta{APIVersion: "autorestart.uni.com/v1", Kind: "CronRestarter"},
		ObjectMeta: metav1.ObjectMeta{Name: "cron", Namespace: "default", CreationTimestamp: now},
		Spec: cronrestartv1.CronRestarterSpec{
			Schedule: "* * * * *",
			RestartTargetRef: cronrestartv1.RestartTargetRef{
				ApiVersion: "apps/v1",
				Kind:       "Deployment",
				Name:       "target",
			},
		},
	}
	deployment := &appsv1.Deployment{
		TypeMeta:   metav1.TypeMeta{APIVersion: "apps/v1", Kind: "Deployment"},
		ObjectMeta: metav1.ObjectMeta{Name: "target", Namespace: "default"},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "e2e"}},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "e2e"}},
				Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "nginx", Image: "nginx:1.25"}}},
			},
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cron, deployment).WithStatusSubresource(cron).Build()
	jobInterface, err := CronRestarterJobFactory(cron, fakeClient, fakeClient, record.NewFakeRecorder(10))
	g.Expect(err).ToNot(HaveOccurred())
	job, ok := jobInterface.(*CronJobRestarter)
	g.Expect(ok).To(BeTrue())

	err = job.RestartRef()
	g.Expect(err).ToNot(HaveOccurred())

	updatedDeployment := &appsv1.Deployment{}
	g.Expect(fakeClient.Get(context.Background(), types.NamespacedName{Name: "target", Namespace: "default"}, updatedDeployment)).To(Succeed())
	g.Expect(updatedDeployment.Spec.Template.Annotations).ToNot(BeNil())
	g.Expect(updatedDeployment.Spec.Template.Annotations["kubectl.kubernetes.io/restartedAt"]).ToNot(BeEmpty())

	updatedCron := &cronrestartv1.CronRestarter{}
	g.Expect(fakeClient.Get(context.Background(), types.NamespacedName{Name: "cron", Namespace: "default"}, updatedCron)).To(Succeed())
	g.Expect(updatedCron.Status.LastExecutionTime.IsZero()).To(BeFalse())
	g.Expect(updatedCron.Status.ProcessingTick.IsZero()).To(BeTrue())
}

func TestRestartRefSkipsWhenAlreadyProcessingSameTick(t *testing.T) {
	g := NewWithT(t)

	scheme := runtime.NewScheme()
	g.Expect(clientgoscheme.AddToScheme(scheme)).To(Succeed())
	g.Expect(cronrestartv1.AddToScheme(scheme)).To(Succeed())

	now := time.Now()
	creationTimestamp := metav1.NewTime(now.Add(-2 * time.Minute))
	cron := &cronrestartv1.CronRestarter{
		ObjectMeta: metav1.ObjectMeta{Name: "cron", Namespace: "default", CreationTimestamp: creationTimestamp},
		Spec: cronrestartv1.CronRestarterSpec{
			Schedule: "* * * * *",
			RestartTargetRef: cronrestartv1.RestartTargetRef{
				ApiVersion: "apps/v1",
				Kind:       "Deployment",
				Name:       "target",
			},
		},
	}
	entry, err := cronutils.Get5FieldParser().Parse(cron.Spec.Schedule)
	g.Expect(err).ToNot(HaveOccurred())
	cron.Status.ProcessingTick = metav1.NewTime(entry.Next(creationTimestamp.Time))
	cron.Status.LastExecutionTime = creationTimestamp

	deployment := &appsv1.Deployment{
		TypeMeta:   metav1.TypeMeta{APIVersion: "apps/v1", Kind: "Deployment"},
		ObjectMeta: metav1.ObjectMeta{Name: "target", Namespace: "default"},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "e2e"}},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "e2e"}},
				Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "nginx", Image: "nginx:1.25"}}},
			},
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cron, deployment).Build()
	jobInterface, err := CronRestarterJobFactory(cron, fakeClient, fakeClient, record.NewFakeRecorder(10))
	g.Expect(err).ToNot(HaveOccurred())
	job, ok := jobInterface.(*CronJobRestarter)
	g.Expect(ok).To(BeTrue())

	err = job.RestartRef()
	g.Expect(err).To(HaveOccurred())
	g.Expect(err).To(Equal(constants.NoNeedRestart))
}
