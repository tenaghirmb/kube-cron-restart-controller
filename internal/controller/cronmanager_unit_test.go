package controller

import (
	"context"
	"sync"
	"testing"
	"time"

	. "github.com/onsi/gomega"
	"github.com/robfig/cron/v3"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	cronrestartv1 "uni.com/cronrestart/api/v1"
)

type fakeCronExecutor struct {
	addedJobs   []CronJob
	removedJobs []CronJob
}

func (f *fakeCronExecutor) Run()  {}
func (f *fakeCronExecutor) Stop() {}
func (f *fakeCronExecutor) AddJob(job CronJob) (cron.EntryID, error) {
	f.addedJobs = append(f.addedJobs, job)
	return cron.EntryID(len(f.addedJobs)), nil
}
func (f *fakeCronExecutor) RemoveJob(job CronJob) {
	f.removedJobs = append(f.removedJobs, job)
}
func (f *fakeCronExecutor) GetTasks() []TaskStatus { return nil }

type fakeJob struct {
	id        string
	entryId   cron.EntryID
	name      string
	namespace string
	plan      string
}

func (f *fakeJob) ID() string                      { return f.id }
func (f *fakeJob) EntryId() cron.EntryID           { return f.entryId }
func (f *fakeJob) Name() string                    { return f.name }
func (f *fakeJob) Namespace() string               { return f.namespace }
func (f *fakeJob) SetID(id string)                 { f.id = id }
func (f *fakeJob) SetEntryId(entryId cron.EntryID) { f.entryId = entryId }
func (f *fakeJob) SchedulePlan() string            { return f.plan }
func (f *fakeJob) Ref() *TargetRef                 { return &TargetRef{} }
func (f *fakeJob) CronRestarterMeta() *cronrestartv1.CronRestarter {
	return &cronrestartv1.CronRestarter{}
}
func (f *fakeJob) Run() {}

func TestCronManagerCreateOrUpdateStoresJob(t *testing.T) {
	g := NewWithT(t)

	executor := &fakeCronExecutor{}
	manager := &CronManager{cronExecutor: executor, jobQueue: &sync.Map{}}
	job := &fakeJob{id: "job-123", name: "cron-job", namespace: "default", plan: "* * * * *"}

	err := manager.createOrUpdate(job)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(executor.addedJobs).To(HaveLen(1))
	stored, ok := manager.jobQueue.Load("job-123")
	g.Expect(ok).To(BeTrue())
	g.Expect(stored).To(Equal(job))
}

func TestCronManagerDeleteRemovesJobFromQueue(t *testing.T) {
	g := NewWithT(t)

	executor := &fakeCronExecutor{}
	manager := &CronManager{cronExecutor: executor, jobQueue: &sync.Map{}}
	job := &CronJobRestarter{
		name:         "cron-job",
		namespace:    "default",
		id:           "job-456",
		plan:         "* * * * *",
		TargetRef:    &TargetRef{RefKind: "Deployment", RefName: "target", RefNamespace: "default", RefGroup: "apps", RefVersion: "v1"},
		RestarterRef: &cronrestartv1.CronRestarter{ObjectMeta: metav1.ObjectMeta{Name: "cron", Namespace: "default"}},
	}
	manager.jobQueue.Store(job.ID(), job)

	err := manager.delete(job.ID())
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(executor.removedJobs).To(HaveLen(1))
	_, ok := manager.jobQueue.Load(job.ID())
	g.Expect(ok).To(BeFalse())
}

func TestCronManagerShouldCompensate(t *testing.T) {
	g := NewWithT(t)

	instance := cronrestartv1.CronRestarter{
		Spec: cronrestartv1.CronRestarterSpec{
			Schedule:                 "@hourly",
			MisfirePolicy:            "FireAndProceed",
			MisfireDeadWindowMinutes: func() *int32 { v := int32(5); return &v }(),
		},
		Status: cronrestartv1.CronRestarterStatus{
			LastExecutionTime: metav1.NewTime(time.Now().Add(-70 * time.Minute)),
		},
	}

	manager := &CronManager{}
	g.Expect(manager.shouldCompensate(instance)).To(BeTrue())

	instance.Spec.MisfirePolicy = "Ignore"
	g.Expect(manager.shouldCompensate(instance)).To(BeFalse())
}

func TestCronManagerGCRemovesMissingCronRestarter(t *testing.T) {
	g := NewWithT(t)

	scheme := runtime.NewScheme()
	g.Expect(clientgoscheme.AddToScheme(scheme)).To(Succeed())
	g.Expect(cronrestartv1.AddToScheme(scheme)).To(Succeed())

	job := &CronJobRestarter{
		id:           "missing",
		RestarterRef: &cronrestartv1.CronRestarter{ObjectMeta: metav1.ObjectMeta{Name: "missing", Namespace: "default"}},
		TargetRef:    &TargetRef{RefKind: "Deployment", RefName: "target", RefNamespace: "default", RefGroup: "apps", RefVersion: "v1"},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	executor := &fakeCronExecutor{}
	manager := &CronManager{client: fakeClient, cronExecutor: executor, jobQueue: &sync.Map{}}
	manager.jobQueue.Store("missing", job)

	manager.GC()

	_, ok := manager.jobQueue.Load("missing")
	g.Expect(ok).To(BeFalse())
	g.Expect(executor.removedJobs).To(HaveLen(2))
}

func TestCronManagerMisfireCompensateExecutesMissedJob(t *testing.T) {
	g := NewWithT(t)

	scheme := runtime.NewScheme()
	g.Expect(clientgoscheme.AddToScheme(scheme)).To(Succeed())
	g.Expect(cronrestartv1.AddToScheme(scheme)).To(Succeed())

	deployment := &appsv1.Deployment{
		TypeMeta:   metav1.TypeMeta{APIVersion: "apps/v1", Kind: "Deployment"},
		ObjectMeta: metav1.ObjectMeta{Name: "target", Namespace: "default"},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "e2e"}},
			Template: corev1.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "e2e"}}, Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "nginx", Image: "nginx:1.25"}}}},
		},
	}

	now := time.Now()
	cronRes := &cronrestartv1.CronRestarter{
		ObjectMeta: metav1.ObjectMeta{Name: "cron", Namespace: "default", CreationTimestamp: metav1.NewTime(now.Add(-70 * time.Minute))},
		Spec: cronrestartv1.CronRestarterSpec{
			Schedule:                 "@hourly",
			RestartTargetRef:         cronrestartv1.RestartTargetRef{ApiVersion: "apps/v1", Kind: "Deployment", Name: "target"},
			MisfirePolicy:            "FireAndProceed",
			MisfireDeadWindowMinutes: func() *int32 { v := int32(1); return &v }(),
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(deployment, cronRes).WithStatusSubresource(cronRes).Build()
	executor := &fakeCronExecutor{}
	eventRecorder := &record.FakeRecorder{}
	manager := &CronManager{client: fakeClient, cronExecutor: executor, jobQueue: &sync.Map{}, eventRecorder: eventRecorder}

	manager.misfireCompensate(context.Background())

	updated := &cronrestartv1.CronRestarter{}
	g.Expect(fakeClient.Get(context.Background(), types.NamespacedName{Name: "cron", Namespace: "default"}, updated)).To(Succeed())
	g.Expect(updated.Status.State).To(Equal(cronrestartv1.Succeed))
	g.Expect(updated.Status.LastExecutionTime.IsZero()).To(BeFalse())
}
