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

package v1

import (
	"github.com/robfig/cron/v3"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

type RestartTargetRef struct {
	// +kubebuilder:validation:Required
	ApiVersion string `json:"apiVersion"`
	// +kubebuilder:validation:Required
	Kind string `json:"kind"`
	// +kubebuilder:validation:Required
	Name string `json:"name"`
}

type CronRestarterSpec struct {
	// If schedule contains '@', the timezone will be ignored. Otherwise, the timezone will be used to determine the schedule.
	// +kubebuilder:validation:Optional
	Timezone string `json:"timezone,omitempty"`

	// +kubebuilder:validation:Optional
	ExcludeDates []string `json:"excludeDates,omitempty"`

	// +kubebuilder:validation:Required
	RestartTargetRef RestartTargetRef `json:"restartTargetRef"`

	// Schedule is a cron expression that defines the schedule for restarting the target resource.
	// It supports standard cron expressions as well as predefined schedules like "annually", "yearly", "monthly", "weekly", "daily", "midnight", "hourly", and "every".
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Pattern=`^((@hourly|@daily|@weekly|@monthly|@annually|@yearly|@midnight)|(@every\s+[0-9]+[smh])|((([0-59\*a-zA-Z0-9,\-\/]+)\s+){4}([0-7\*a-zA-Z0-9,\-\/]+)))$`
	Schedule string `json:"schedule"`

	// MisfirePolicy defines the behavior when a scheduled execution is missed.
	// It can be set to "Ignore" (default) to skip missed executions, or "FireAndProceed" to execute the missed job immediately.
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Enum=Ignore;FireAndProceed
	// +kubebuilder:default=Ignore
	MisfirePolicy MisfirePolicy `json:"misfirePolicy,omitempty"`

	// MisfireDeadWindowMinutes specifies the threshold in minutes.
	// If the next regular execution time is closer than this window, the misfire recovery will be ignored.
	// Default to 5 minutes if not specified.
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Minimum=1
	MisfireDeadWindowMinutes *int32 `json:"misfireDeadWindowMinutes,omitempty"`
}

type Condition struct {
	State         JobState    `json:"state"`
	LastProbeTime metav1.Time `json:"lastProbeTime"`
	Message       string      `json:"message"`
}

type CronRestarterStatus struct {
	JobId             cron.EntryID `json:"entryId"`
	State             JobState     `json:"state"`
	Message           string       `json:"message"`
	ProcessingTick    metav1.Time  `json:"processingTick,omitempty"`
	LastTickTimestamp metav1.Time  `json:"lastTickTimestamp,omitempty"`
	LastExecutionTime metav1.Time  `json:"lastExecutionTime,omitempty"`
	Conditions        []Condition  `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Schedule",type=string,JSONPath=`.spec.schedule`,description="The cron schedule for restarting the target resource"
// +kubebuilder:printcolumn:name="TIMEZONE",type=string,JSONPath=`.spec.timezone`,priority=1,description="The timezone for the cron schedule"
// +kubebuilder:printcolumn:name="EXCLUDE-DATES",type=string,JSONPath=`.spec.excludeDates`,priority=1,description="The dates to exclude from the cron schedule"
// +kubebuilder:printcolumn:name="TARGET",type=string,JSONPath=`.spec.restartTargetRef`,priority=1,description="The target resource to be restarted"
// +kubebuilder:printcolumn:name="LAST-EXEC",type=string,JSONPath=`.status.lastExecutionTime`,description="The last time the cron job was executed"
// +kubebuilder:printcolumn:name="STATE",type=string,JSONPath=`.status.state`,description="The current state of the cron job"

type CronRestarter struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   CronRestarterSpec   `json:"spec,omitempty"`
	Status CronRestarterStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

type CronRestarterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []CronRestarter `json:"items"`
}

func init() {
	SchemeBuilder.Register(&CronRestarter{}, &CronRestarterList{})
}
