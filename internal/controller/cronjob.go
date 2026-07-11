package controller

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/robfig/cron/v3"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	log "k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	cronrestartv1 "uni.com/cronrestart/api/v1"
	"uni.com/cronrestart/pkg/constants"
	cronutils "uni.com/cronrestart/pkg/cron"
)

type TargetRef struct {
	RefName      string
	RefNamespace string
	RefKind      string
	RefGroup     string
	RefVersion   string
}

type CronJob interface {
	ID() string
	EntryId() cron.EntryID
	Name() string
	Namespace() string
	SetID(id string)
	SetEntryId(entryId cron.EntryID)
	SchedulePlan() string
	Ref() *TargetRef
	CronRestarterMeta() *cronrestartv1.CronRestarter
	Run()
}

type CronJobRestarter struct {
	client        client.Client
	APIReader     client.Reader
	EventRecorder record.EventRecorder
	RestarterRef  *cronrestartv1.CronRestarter
	TargetRef     *TargetRef
	entryId       cron.EntryID
	excludeDates  []string
	plan          string
	id            string
	name          string
	namespace     string
}

func (cr *CronJobRestarter) SetID(id string) {
	cr.id = id
}

func (cr *CronJobRestarter) SetEntryId(entryId cron.EntryID) {
	cr.entryId = entryId
}

func (cr *CronJobRestarter) Name() string {
	return cr.name
}

func (cr *CronJobRestarter) Namespace() string {
	return cr.namespace
}

func (cr *CronJobRestarter) ID() string {
	return cr.id
}

func (cr *CronJobRestarter) EntryId() cron.EntryID {
	return cr.entryId
}

func (cr *CronJobRestarter) SchedulePlan() string {
	return cr.plan
}

func (cr *CronJobRestarter) Ref() *TargetRef {
	return cr.TargetRef
}

func (cr *CronJobRestarter) CronRestarterMeta() *cronrestartv1.CronRestarter {
	return cr.RestarterRef
}

func (cr *CronJobRestarter) Run() {
	var err error
	if IsTodayOff(cr.excludeDates) {
		return
	}

	startTime := time.Now()
	times := 0
	for {
		now := time.Now()

		// timeout and exit
		if startTime.Add(constants.MaxRetryTimeout).Before(now) {
			log.Errorf("failed to restart %s %s in %s namespace after retrying %d times and exit,because of %v", cr.TargetRef.RefKind, cr.TargetRef.RefName, cr.TargetRef.RefNamespace, times, err)
			break
		}

		err := cr.RestartRef()
		if err == nil || errors.Is(err, constants.NoNeedRestart) {
			break
		}
		time.Sleep(constants.UpdateRetryInterval)
		times++
	}

	// update cronRestarter status
	cr.ResultHandle(err)
}

func (cr *CronJobRestarter) ResultHandle(err error) {
	ctx := context.Background()

	var instance cronrestartv1.CronRestarter
	if e := cr.client.Get(ctx, types.NamespacedName{Namespace: cr.RestarterRef.Namespace, Name: cr.RestarterRef.Name}, &instance); e != nil {
		log.Errorf("failed to find cronRestarter %s in %s namespace, because of %v", cr.RestarterRef.Name, cr.RestarterRef.Namespace, e)
		return
	}

	var (
		state     cronrestartv1.JobState
		message   string
		eventType string
	)

	if errors.Is(err, constants.NoNeedRestart) {
		return
	} else if err != nil {
		state = cronrestartv1.Failed
		message = fmt.Sprintf("cron restarter failed to execute, because of %v", err)
		eventType = v1.EventTypeWarning
	} else {
		state = cronrestartv1.Succeed
		message = fmt.Sprintf("cron restarter job %s executed successfully.", cr.name)
		eventType = v1.EventTypeNormal
	}

	condition := cronrestartv1.Condition{
		LastProbeTime: metav1.Now(),
		State:         state,
		Message:       message,
	}

	original := instance.DeepCopy()

	instance.Status.JobId = cr.EntryId()
	instance.Status.State = state
	instance.Status.Message = message
	if len(instance.Status.Conditions) >= constants.MaxConditions {
		instance.Status.Conditions = instance.Status.Conditions[len(instance.Status.Conditions)-constants.MaxConditions+1:]
	}
	instance.Status.Conditions = append(instance.Status.Conditions, condition)

	err = cr.updateCronRestarterStatusWithRetry(original, &instance)
	if err != nil {
		if errors.Is(err, constants.NoNeedUpdate) {
			log.Warning("No need to update cronRestarter, because it had been deleted")
			return
		}
		cr.EventRecorder.Event(&instance, v1.EventTypeWarning, "Failed", fmt.Sprintf("Failed to update cronRestarter status: %v", err))
	} else {
		cr.EventRecorder.Event(&instance, eventType, string(state), message)
	}
}

func (cr *CronJobRestarter) RestartRef() (err error) {
	ctx := context.Background()

	targetGVK := schema.GroupVersionKind{
		Group:   cr.TargetRef.RefGroup,
		Version: cr.TargetRef.RefVersion,
		Kind:    cr.TargetRef.RefKind,
	}

	// 获取重启对象
	target := &unstructured.Unstructured{}
	target.SetGroupVersionKind(targetGVK)
	if err := cr.client.Get(ctx, types.NamespacedName{Namespace: cr.TargetRef.RefNamespace, Name: cr.TargetRef.RefName}, target); err != nil {
		log.Errorf("failed to find source target %s %s in %s namespace", cr.TargetRef.RefKind, cr.TargetRef.RefName, cr.TargetRef.RefNamespace)
		return fmt.Errorf("failed to find source target %s %s in %s namespace", cr.TargetRef.RefKind, cr.TargetRef.RefName, cr.TargetRef.RefNamespace)
	}

	// 验证cron表达式
	schedule := cr.SchedulePlan()
	cronSchedule, err := cronutils.Get5FieldParser().Parse(schedule)
	if err != nil {
		log.Errorf("failed to parse schedule %s for %s %s in %s namespace, because of %v", schedule, cr.TargetRef.RefKind, cr.TargetRef.RefName, cr.TargetRef.RefNamespace, err)
		return fmt.Errorf("failed to parse schedule %s for %s %s in %s namespace, because of %v", schedule, cr.TargetRef.RefKind, cr.TargetRef.RefName, cr.TargetRef.RefNamespace, err)
	}
	var cronRestarter cronrestartv1.CronRestarter
	if err := cr.APIReader.Get(ctx, types.NamespacedName{Namespace: cr.RestarterRef.Namespace, Name: cr.RestarterRef.Name}, &cronRestarter); err != nil {
		log.Errorf("failed to find cronRestarter %s in %s namespace, because of %v", cr.RestarterRef.Name, cr.RestarterRef.Namespace, err)
		return fmt.Errorf("failed to find cronRestarter %s in %s namespace, because of %v", cr.RestarterRef.Name, cr.RestarterRef.Namespace, err)
	}
	lastExecution := cronRestarter.Status.LastExecutionTime
	if lastExecution.IsZero() {
		lastExecution = cronRestarter.CreationTimestamp
	}
	timeKey := cronSchedule.Next(lastExecution.Time)

	if cronRestarter.Status.ProcessingTick.IsZero() {
		original := cronRestarter.DeepCopy()
		cronRestarter.Status.ProcessingTick = metav1.Time{Time: timeKey}
		cronRestarter.Status.LastTickTimestamp = metav1.Now()
		if err := cr.client.Status().Patch(ctx, &cronRestarter, client.MergeFrom(original)); err != nil {
			log.Errorf("failed to pre-tick cronRestarter %s in %s namespace, because of %v", cr.RestarterRef.Name, cr.RestarterRef.Namespace, err)
			if apierrors.IsConflict(err) {
				return constants.NoNeedRestart
			}
			return fmt.Errorf("failed to pre-tick cronRestarter %s in %s namespace, because of %v", cr.RestarterRef.Name, cr.RestarterRef.Namespace, err)
		}
	} else if cronRestarter.Status.ProcessingTick.Time.Truncate(time.Minute).Equal(timeKey.Truncate(time.Minute)) {
		log.Infof("skip restarting %s %s in %s namespace, because it has been restarted at %s", cr.TargetRef.RefKind, cr.TargetRef.RefName, cr.TargetRef.RefNamespace, timeKey.Format(time.RFC3339))
		return constants.NoNeedRestart
	}

	// 构建Patch数据以重启target
	patch := fmt.Sprintf(`{"spec":{"template":{"metadata":{"annotations":{"kubectl.kubernetes.io/restartedAt":"%s"}}}}}`, time.Now().UTC().Format(time.RFC3339))
	mergePatch := []byte(patch)

	// 应用Patch重启
	if err := cr.client.Patch(context.Background(), target, client.RawPatch(types.MergePatchType, mergePatch)); err != nil {
		return fmt.Errorf("failed to restart %s %s in %s namespace, because of %v", cr.TargetRef.RefKind, cr.TargetRef.RefName, cr.TargetRef.RefNamespace, err)
	}
	log.Infof("%s %s in namespace %s has been restarted successfully. cronRestarter: %s id: %s", cr.TargetRef.RefKind, cr.TargetRef.RefName, cr.TargetRef.RefNamespace, cr.Name(), cr.ID())

	// 更新cronRestarter的状态，标记任务完成
	original := cronRestarter.DeepCopy()
	cronRestarter.Status.ProcessingTick = metav1.Time{}
	cronRestarter.Status.LastExecutionTime = metav1.Now()
	if err := cr.client.Status().Patch(ctx, &cronRestarter, client.MergeFrom(original)); err != nil {
		log.Errorf("failed to post-tick cronRestarter %s in %s namespace after restarting %s %s, because of %v", cr.RestarterRef.Name, cr.RestarterRef.Namespace, cr.TargetRef.RefKind, cr.TargetRef.RefName, err)
	}

	return nil
}

func (cr *CronJobRestarter) updateCronRestarterStatusWithRetry(original *cronrestartv1.CronRestarter, instance *cronrestartv1.CronRestarter) error {
	var err error
	for i := 1; i <= constants.MaxRetryTimes; i++ {
		err = cr.client.Status().Patch(context.Background(), instance, client.MergeFrom(original))
		if err != nil {
			if apierrors.IsNotFound(err) {
				log.Error("Failed to update the status of cronRestarter, because instance is deleted")
				return constants.NoNeedUpdate
			}
			log.Errorf("Failed to update the status of cronRestarter %v, because of %v", instance, err)
			continue
		}
		break
	}
	return err
}

func checkRefValid(ref *TargetRef) error {
	if ref.RefVersion == "" || ref.RefName == "" || ref.RefNamespace == "" || ref.RefKind == "" {
		return errors.New("any properties in RestartTargetRef could not be empty")
	}
	return nil
}

func CronRestarterJobFactory(instance *cronrestartv1.CronRestarter, client client.Client, APIReader client.Reader, eventRecorder record.EventRecorder) (CronJob, error) {
	arr := strings.Split(instance.Spec.RestartTargetRef.ApiVersion, "/")
	group := arr[0]
	version := arr[1]
	ref := &TargetRef{
		RefName:      instance.Spec.RestartTargetRef.Name,
		RefKind:      instance.Spec.RestartTargetRef.Kind,
		RefNamespace: instance.Namespace,
		RefGroup:     group,
		RefVersion:   version,
	}

	if err := checkRefValid(ref); err != nil {
		return nil, err
	}

	schedule := instance.Spec.Schedule
	timezone := instance.Spec.Timezone
	if valid := cronutils.ValidateTimezone(timezone); valid {
		if !strings.ContainsAny(schedule, "@") {
			schedule = fmt.Sprintf("CRON_TZ=%s %s", timezone, schedule)
		} else {
			log.Warningf("The schedule %s contains '@', the timezone %s will be ignored", schedule, timezone)
		}
	}

	return &CronJobRestarter{
		TargetRef:     ref,
		RestarterRef:  instance,
		name:          instance.Name,
		namespace:     instance.Namespace,
		plan:          schedule,
		excludeDates:  instance.Spec.ExcludeDates,
		client:        client,
		APIReader:     APIReader,
		EventRecorder: eventRecorder,
	}, nil
}

func IsTodayOff(excludeDates []string) bool {

	if excludeDates == nil {
		return false
	}

	now := time.Now()

	for _, date := range excludeDates {
		schedule, err := cronutils.Get5FieldParser().Parse(date)
		if err != nil {
			log.Warningf("Failed to parse schedule %s,and skip this date,because of %v", date, err)
			continue
		}
		if nextTime := schedule.Next(now); nextTime.Format(constants.DateFormat) == now.Format(constants.DateFormat) {
			return true
		}
	}
	return false
}
