package controller

import (
	"sync"
	"time"

	"github.com/robfig/cron/v3"
	log "k8s.io/klog/v2"
)

type CronExecutor interface {
	Run()
	Stop()
	AddJob(job CronJob) (cron.EntryID, error)
	RemoveJob(job CronJob)
	GetTasks() []TaskStatus
}

type CronRestartExecutor struct {
	mu      sync.RWMutex
	Engine  *cron.Cron
	taskMap map[string]*TaskStatus
}

type TaskStatus struct {
	ID       cron.EntryID `json:"id"`
	JobName  string       `json:"jobName"`
	Schedule string       `json:"schedule"`
	Target   *TargetRef   `json:"target"`
	PrevRun  time.Time    `json:"prevRun"`
	NextRun  time.Time    `json:"nextRun"`
}

func (ce *CronRestartExecutor) GetTasks() []TaskStatus {
	ce.mu.RLock()
	defer ce.mu.RUnlock()

	var list []TaskStatus
	entries := ce.Engine.Entries()

	for _, entry := range entries {
		job, ok := entry.Job.(CronJob)
		if !ok {
			log.Warningf("Failed to parse cronjob %v to web console", entry.ID)
			continue
		}
		list = append(list, TaskStatus{
			ID:       entry.ID,
			JobName:  job.Namespace() + "/" + job.Name(),
			Schedule: job.SchedulePlan(),
			Target:   job.Ref(),
			PrevRun:  entry.Prev,
			NextRun:  entry.Next,
		})
	}
	return list
}

func (ce *CronRestartExecutor) Run() {
	ce.Engine.Start()
}

func (ce *CronRestartExecutor) Stop() {
	ce.Engine.Stop()
}

func (ce *CronRestartExecutor) AddJob(job CronJob) (cron.EntryID, error) {
	entryId, err := ce.Engine.AddJob(job.SchedulePlan(), job)
	if err != nil {
		log.Errorf("Failed to add job to engine, because of %v", err)
	}
	return entryId, err
}

func (ce *CronRestartExecutor) RemoveJob(job CronJob) {
	ce.Engine.Remove(job.EntryId())
}

func NewCronRestartExecutor(timezone *time.Location) CronExecutor {
	if nil == timezone {
		timezone = time.Now().Location()
	}
	c := &CronRestartExecutor{
		Engine:  cron.New(cron.WithLocation(timezone), cron.WithChain(cron.Recover(cron.DefaultLogger))),
		taskMap: make(map[string]*TaskStatus),
	}
	return c
}
