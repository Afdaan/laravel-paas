package domain

type FailureClass string

const (
	FailureTransient         FailureClass = "TRANSIENT"
	FailureRetryable         FailureClass = "RETRYABLE"
	FailurePermanent         FailureClass = "PERMANENT"
	FailureDependencyOutage  FailureClass = "DEPENDENCY_OUTAGE"
	FailureUserConfiguration FailureClass = "USER_CONFIGURATION"
	FailureFatal             FailureClass = "FATAL"
)

func ClassifyError(err error) FailureClass {
	if err == nil {
		return ""
	}
	// Add error classifications
	return FailureRetryable
}
