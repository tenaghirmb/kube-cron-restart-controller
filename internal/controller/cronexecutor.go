package controller

import (
	"time"

	"github.com/robfig/cron/v3"
	log "k8s.io/klog/v2"
)

type CronExecutor interface {
	Run()
	Stop()
	AddJob(job CronJob) (cron.EntryID, error)
	RemoveJob(job CronJob)
	ListEntries() []cron.Entry
}

type CronRestartExecutor struct {
	Engine *cron.Cron
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

func (ce *CronRestartExecutor) ListEntries() []cron.Entry {
	return ce.Engine.Entries()
}

func NewCronRestartExecutor(timezone *time.Location) CronExecutor {
	if nil == timezone {
		timezone = time.Now().Location()
	}
	c := &CronRestartExecutor{
		Engine: cron.New(cron.WithLocation(timezone), cron.WithChain(cron.Recover(cron.DefaultLogger))),
	}
	return c
}
