package controller

import (
	"context"
	"fmt"
	"sync"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	log "k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	cronrestartv1 "uni.com/cronrestart/api/v1"
	"uni.com/cronrestart/pkg/constants"
)

type CronManager struct {
	sync.Mutex
	cfg          *rest.Config
	client       client.Client
	jobQueue     *sync.Map
	cronExecutor CronExecutor
}

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
func (cm *CronManager) Run(ctx context.Context) {
	cm.cronExecutor.Run()
	cm.gcLoop()

	<-ctx.Done()

	cm.cronExecutor.Stop()
}

// GC loop
func (cm *CronManager) gcLoop() {
	ticker := time.NewTicker(constants.GCInterval)
	defer ticker.Stop()
	go func() {
		for range ticker.C {
			log.Infof("GC loop started every %v", constants.GCInterval)
			cm.GC()
		}
	}()
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

func NewCronManager(cfg *rest.Config, client client.Client) *CronManager {
	cm := &CronManager{
		cfg:      cfg,
		client:   client,
		jobQueue: &sync.Map{},
	}
	cm.cronExecutor = NewCronRestartExecutor(nil)
	return cm
}
