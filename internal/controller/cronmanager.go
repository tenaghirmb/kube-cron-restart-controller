package controller

import (
	"context"
	"fmt"
	"sync"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	log "k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	cronrestartv1 "uni.com/cronrestart/api/v1"
	v1 "uni.com/cronrestart/api/v1"
	"uni.com/cronrestart/pkg/constants"
	cronutils "uni.com/cronrestart/pkg/cron"
)

type CronManager struct {
	sync.Mutex
	client        client.Client
	jobQueue      *sync.Map
	cronExecutor  CronExecutor
	eventRecorder record.EventRecorder
}

var _ manager.Runnable = &CronManager{}

func (cm *CronManager) createOrUpdate(j CronJob) error {
	cm.cronExecutor.RemoveJob(j)
	entryId, err := cm.cronExecutor.AddJob(j)
	if err != nil {
		return fmt.Errorf("Failed to add job to cronExecutor,because of %v", err)
	}
	j.SetEntryId(entryId)
	cm.jobQueue.Store(j.ID(), j)
	log.Infof("cronRestarter job %s of cronRestarter %s in %s updated, %d active jobs exist", j.Name(), j.CronRestarterMeta().Name, j.CronRestarterMeta().Namespace, queueLength(cm.jobQueue))

	return nil
}

func (cm *CronManager) delete(id string) error {
	if loadJob, ok := cm.jobQueue.Load(id); ok {
		j, _ := loadJob.(*CronJobRestarter)
		cm.cronExecutor.RemoveJob(j)
		cm.jobQueue.Delete(id)
		log.Infof("Remove cronRestarter job %s of cronRestarter %s in %s from jobQueue,%d active jobs left", j.Name(), j.CronRestarterMeta().Name, j.CronRestarterMeta().Namespace, queueLength(cm.jobQueue))
	}
	return nil
}

// Run starts the cron manager
func (cm *CronManager) Start(ctx context.Context) error {
	log.Info("Starting CronManager component...")

	// Start the cron executor
	cm.cronExecutor.Run()
	log.Info("Regular Cron Engine Clock has been activated successfully.")

	go cm.misfireCompensate(ctx)

	go cm.gcLoop(ctx)

	<-ctx.Done()

	log.Info("Stopping CronManager component...")
	cm.cronExecutor.Stop()
	return nil
}

// GC loop
func (cm *CronManager) gcLoop(ctx context.Context) {
	ticker := time.NewTicker(constants.GCInterval)
	defer ticker.Stop()

	log.Infof("GC loop initialized to run every %v", constants.GCInterval)

	for {
		select {
		case <-ticker.C:
			log.Info("Triggering routine garbage collection...")
			cm.GC()
		case <-ctx.Done():
			log.Info("Shutting down GC loop cleaner safely.")
			return
		}
	}
}

// GC will collect all jobs whose ref does not exist and recycle.
func (cm *CronManager) GC() {
	current := queueLength(cm.jobQueue)
	log.V(2).Infof("Current active jobs: %d,try to clean up the abandon ones.", current)

	gcJobFunc := func(_, j interface{}) bool {
		restarter := j.(*CronJobRestarter).RestarterRef
		job := j.(*CronJobRestarter)
		instance := &cronrestartv1.CronRestarter{}

		// check exists first
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := cm.client.Get(ctx, types.NamespacedName{
			Namespace: restarter.Namespace,
			Name:      restarter.Name,
		}, instance); err != nil {
			if apierrors.IsNotFound(err) {
				log.Infof("remove job %s(%s) of cronRestarter %s in namespace %s", job.Name(), job.SchedulePlan(), restarter.Name, restarter.Namespace)
				cm.cronExecutor.RemoveJob(job)
				cm.delete(job.ID())
			}
		}
		return true
	}

	cm.jobQueue.Range(gcJobFunc)

	left := queueLength(cm.jobQueue)

	log.V(2).Infof("Current active jobs: %d, clean up %d jobs.", left, current-left)
}

func queueLength(que *sync.Map) int64 {
	len := int64(0)
	que.Range(func(k, v interface{}) bool {
		len++
		return true
	})
	return len
}

func NewCronManager(client client.Client, recorder record.EventRecorder) *CronManager {
	cm := &CronManager{
		client:        client,
		jobQueue:      &sync.Map{},
		eventRecorder: recorder,
	}
	cm.cronExecutor = NewCronRestartExecutor(nil)
	return cm
}

// misfireCompensate handles any missed schedules before the manager started.
func (cm *CronManager) misfireCompensate(ctx context.Context) {
	log.Info("Starting asynchronous misfire compensation loop...")

	var list cronrestartv1.CronRestarterList
	if err := cm.client.List(ctx, &list); err != nil {
		log.Errorf("Failed to list CronRestarter for misfire compensation: %v", err)
		return
	}

	sem := make(chan struct{}, constants.MaxConcurrentMisfire) // Limit the number of concurrent compensations
	var wg sync.WaitGroup

	for _, instance := range list.Items {
		if instance.Spec.MisfirePolicy == "" || instance.Spec.MisfirePolicy == v1.MisfireIgnore {
			continue
		}

		if instance.Spec.MisfirePolicy == v1.MisfireFireAndProceed {
			if !cm.shouldCompensate(instance) {
				continue
			}

			sem <- struct{}{}
			wg.Add(1)

			go func(inst cronrestartv1.CronRestarter) {
				defer func() {
					<-sem
					wg.Done()
				}()
				cm.executeCompensate(ctx, inst)
			}(instance)
		}
	}

	wg.Wait()
	log.Info("All asynchronous misfire compensations completed.")
}

func (cm *CronManager) executeCompensate(ctx context.Context, snapshotInstance cronrestartv1.CronRestarter) {
	// 🛡️ The first line of defense: Get the latest status from K8s Cache in real time to prevent the regular engine from executing it during queuing
	var latestInstance cronrestartv1.CronRestarter
	err := cm.client.Get(ctx, client.ObjectKey{
		Namespace: snapshotInstance.Namespace,
		Name:      snapshotInstance.Name,
	}, &latestInstance)
	if err != nil {
		log.Errorf("Failed to get latest instance for %s/%s: %v", snapshotInstance.Namespace, snapshotInstance.Name, err)
		return
	}

	if !cm.shouldCompensate(latestInstance) {
		log.Infof("Misfire cancel: %s/%s was already handled by regular cron engine.",
			latestInstance.Namespace, latestInstance.Name)
		return
	}

	log.Warningf("[Compensate Worker] Confirmed missed schedule for %s/%s. Executing compensatory run.",
		latestInstance.Namespace, latestInstance.Name)

	job, err := CronRestarterJobFactory(&latestInstance, cm.client, cm.client, cm.eventRecorder)
	if err != nil {
		log.Errorf("Failed to construct job factory for %s: %v", latestInstance.Name, err)
		return
	}

	// 🛡️ The second line of defense: direct synchronous call here job.Run()
	job.Run()
}

func (cm *CronManager) shouldCompensate(instance cronrestartv1.CronRestarter) bool {
	cronSchedule, err := cronutils.Get5FieldParser().Parse(instance.Spec.Schedule)
	if err != nil {
		log.Errorf("Misfire check skipped: failed to parse schedule %s for %s/%s", instance.Spec.Schedule, instance.Namespace, instance.Name)
		return false
	}
	if instance.Spec.MisfirePolicy == "" || instance.Spec.MisfirePolicy == v1.MisfireIgnore {
		log.Infof("Misfire check skipped for %s/%s because MisfirePolicy is Ignore or not set.", instance.Namespace, instance.Name)
		return false
	}

	lastExecution := instance.Status.LastExecutionTime
	if lastExecution.IsZero() {
		lastExecution = instance.CreationTimestamp
	}

	timeKey := cronSchedule.Next(lastExecution.Time)
	now := time.Now()

	// Leak trigger diagnostics
	if timeKey.Before(now) {
		deadWindow := time.Duration(5) * time.Minute
		if instance.Spec.MisfireDeadWindowMinutes != nil {
			deadWindow = time.Duration(*instance.Spec.MisfireDeadWindowMinutes) * time.Minute
		}

		nextRegularTick := cronSchedule.Next(now)
		if nextRegularTick.Sub(now) <= deadWindow {
			log.Infof("Misfire detected for %s/%s at %s, but skipped because next regular execution (%s) is within the dead window (%v)",
				instance.Namespace, instance.Name, timeKey.Format(time.RFC3339), nextRegularTick.Format(time.RFC3339), deadWindow)
			return false
		}
		return true
	}
	return false
}
