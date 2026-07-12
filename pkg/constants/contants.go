package constants

import (
	"errors"
	"time"
)

type FailedFindJobReason string

const (
	// cronexecutor.go
	MaxOutOfDateTimeout = 5 * time.Minute
	JobTimeOut          = FailedFindJobReason("JobTimeOut")

	// cronmanager.go
	MaxRetryTimes        = 3
	GCInterval           = 10 * time.Minute
	MaxConditions        = 10
	MaxConcurrentMisfire = 5
	HTTPServerAddress    = ":9887"

	// cronjob.go
	UpdateRetryInterval = 3 * time.Second
	MaxRetryTimeout     = 10 * time.Second
	DateFormat          = "11-15-1990"

	// cronrestarter_controller.go
	FinalizerName = "cronrestarter.finalizers.uni.com"
)

var (
	NoNeedUpdate  = errors.New("NoNeedUpdate")
	NoNeedRestart = errors.New("NoNeedRestart")
)
