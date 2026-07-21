package controller

import (
	"testing"
	"time"

	. "github.com/onsi/gomega"
)

func TestCronRestartExecutorCanAddAndRemoveJob(t *testing.T) {
	g := NewWithT(t)

	executor := NewCronRestartExecutor(nil)
	job := &CronJobRestarter{name: "job", namespace: "default", plan: "* * * * *"}

	entryId, err := executor.AddJob(job)
	g.Expect(err).ToNot(HaveOccurred())
	job.SetEntryId(entryId)

	tasks := executor.GetTasks()
	g.Expect(tasks).To(HaveLen(1))
	g.Expect(tasks[0].JobName).To(Equal("default/job"))

	executor.RemoveJob(job)
	g.Eventually(func() int { return len(executor.GetTasks()) }, 5*time.Second, 100*time.Millisecond).Should(Equal(0))
}

func TestCronRestartExecutorRunAndStopDoNotPanic(t *testing.T) {
	g := NewWithT(t)

	executor := NewCronRestartExecutor(nil)
	g.Expect(func() { executor.Run(); executor.Stop() }).ToNot(Panic())
}
