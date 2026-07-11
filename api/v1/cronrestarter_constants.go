package v1

// MisfirePolicy defines the policy for handling missed schedules when the controller restarts.
// +kubebuilder:validation:Type=string
type MisfirePolicy string

const (
	// MisfireIgnore means ignore the missed schedules and wait for the next regular tick.
	MisfireIgnore MisfirePolicy = "Ignore"

	// MisfireFireAndProceed means trigger one compensatory run immediately, then proceed with regular schedules.
	MisfireFireAndProceed MisfirePolicy = "FireAndProceed"
)

type JobState string

const (
	Succeed   JobState = "Succeed"
	Failed    JobState = "Failed"
	Submitted JobState = "Submitted"
)
