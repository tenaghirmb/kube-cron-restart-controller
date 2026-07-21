package controller

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	. "github.com/onsi/gomega"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	cronrestartv1 "uni.com/cronrestart/api/v1"
	"uni.com/cronrestart/pkg/constants"
)

func TestCronRestarterReconcilerReconcileAddsFinalizerAndUpdatesStatus(t *testing.T) {
	g := NewWithT(t)

	scheme := runtime.NewScheme()
	g.Expect(clientgoscheme.AddToScheme(scheme)).To(Succeed())
	g.Expect(cronrestartv1.AddToScheme(scheme)).To(Succeed())

	cron := &cronrestartv1.CronRestarter{
		TypeMeta:   metav1.TypeMeta{APIVersion: "autorestart.uni.com/v1", Kind: "CronRestarter"},
		ObjectMeta: metav1.ObjectMeta{Name: "cron", Namespace: "default"},
		Spec: cronrestartv1.CronRestarterSpec{
			Schedule: "* * * * *",
			RestartTargetRef: cronrestartv1.RestartTargetRef{
				ApiVersion: "apps/v1",
				Kind:       "Deployment",
				Name:       "target",
			},
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cron).WithStatusSubresource(cron).Build()
	executor := &fakeCronExecutor{}
	cronManager := &CronManager{client: fakeClient, cronExecutor: executor, jobQueue: &sync.Map{}}
	reconciler := &CronRestarterReconciler{
		Client:        fakeClient,
		APIReader:     fakeClient,
		EventRecorder: record.NewFakeRecorder(10),
		scheme:        scheme,
		CronManager:   cronManager,
	}

	result, err := reconciler.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: "cron", Namespace: "default"}})
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Requeue).To(BeFalse())
	g.Expect(executor.addedJobs).To(HaveLen(1))

	updated := &cronrestartv1.CronRestarter{}
	g.Expect(fakeClient.Get(context.Background(), types.NamespacedName{Name: "cron", Namespace: "default"}, updated)).To(Succeed())
	g.Expect(updated.Finalizers).To(ContainElement(constants.FinalizerName))
	g.Expect(updated.Status.State).To(Equal(cronrestartv1.Submitted))
}

func TestCronRestarterReconcilerReconcileDeletesFinalizer(t *testing.T) {
	g := NewWithT(t)

	scheme := runtime.NewScheme()
	g.Expect(clientgoscheme.AddToScheme(scheme)).To(Succeed())
	g.Expect(cronrestartv1.AddToScheme(scheme)).To(Succeed())

	cron := &cronrestartv1.CronRestarter{
		TypeMeta: metav1.TypeMeta{APIVersion: "autorestart.uni.com/v1", Kind: "CronRestarter"},
		ObjectMeta: metav1.ObjectMeta{
			Name:              "cron",
			Namespace:         "default",
			DeletionTimestamp: &metav1.Time{Time: time.Now()},
			Finalizers:        []string{constants.FinalizerName},
		},
		Spec: cronrestartv1.CronRestarterSpec{
			Schedule: "* * * * *",
			RestartTargetRef: cronrestartv1.RestartTargetRef{
				ApiVersion: "apps/v1",
				Kind:       "Deployment",
				Name:       "target",
			},
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cron).Build()
	executor := &fakeCronExecutor{}
	cronManager := &CronManager{client: fakeClient, cronExecutor: executor, jobQueue: &sync.Map{}}
	cronId := uuid.NewSHA1(uuid.NameSpaceDNS, []byte(cron.Namespace+cron.Name)).String()
	cronManager.jobQueue.Store(cronId, &CronJobRestarter{id: cronId, RestarterRef: cron, TargetRef: &TargetRef{RefKind: "Deployment", RefName: "target", RefNamespace: "default", RefGroup: "apps", RefVersion: "v1"}, plan: "* * * * *"})
	reconciler := &CronRestarterReconciler{
		Client:        fakeClient,
		APIReader:     fakeClient,
		EventRecorder: record.NewFakeRecorder(10),
		scheme:        scheme,
		CronManager:   cronManager,
	}

	result, err := reconciler.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: "cron", Namespace: "default"}})
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Requeue).To(BeFalse())
	g.Expect(executor.removedJobs).To(HaveLen(1))

	updated := &cronrestartv1.CronRestarter{}
	err = fakeClient.Get(context.Background(), types.NamespacedName{Name: "cron", Namespace: "default"}, updated)
	g.Expect(apierrors.IsNotFound(err)).To(BeTrue())
}
