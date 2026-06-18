// ===========================================
// Deployment State Machine Validation
// ===========================================
package models

// IsValidTransition enforces valid deterministic state machine progressions for a deployment lifecycle.
func IsValidTransition(from, to ProjectStatus) bool {
	if from == to {
		return true
	}

	// Unconditional terminal or interrupt transitions
	if to == StatusFailed || to == StatusCancelled || to == StatusDeleting || to == StatusStopped {
		return true
	}

	switch from {
	case StatusPending, StatusStopped, StatusFailed, StatusCancelled, StatusRunning, StatusRestarting:
		return to == StatusQueued || to == StatusBuilding || to == StatusPreparing
	case StatusQueued:
		return to == StatusPreparing || to == StatusBuilding
	case StatusPreparing:
		return to == StatusCloning || to == StatusBuilding
	case StatusCloning:
		return to == StatusBuilding
	case StatusBuilding:
		return to == StatusProvisioning || to == StatusStarting || to == StatusHealthchecking
	case StatusProvisioning:
		return to == StatusStarting || to == StatusHealthchecking
	case StatusStarting:
		return to == StatusHealthchecking || to == StatusRunning
	case StatusHealthchecking:
		return to == StatusMigrating || to == StatusPromoting || to == StatusRollback || to == StatusRunning
	case StatusMigrating:
		return to == StatusPromoting || to == StatusRollback || to == StatusRunning
	case StatusPromoting:
		return to == StatusCleanup || to == StatusCompleted || to == StatusRunning
	case StatusCleanup, StatusCompleted:
		return to == StatusRunning
	case StatusRollback:
		return to == StatusFailed || to == StatusStopped || to == StatusRunning
	}

	return false
}

// IsValidDeploymentTransition enforces valid deterministic progressions for deployment execution status.
func IsValidDeploymentTransition(from, to DeploymentStatus) bool {
	if from == to {
		return true
	}

	// Unconditional terminal or interrupt transitions
	if to == DepStatusFailed || to == DepStatusCancelled || to == DepStatusRollback {
		return true
	}

	switch from {
	case DepStatusCompleted, DepStatusFailed, DepStatusCancelled, DepStatusRollback:
		return to == DepStatusQueued || to == DepStatusPreparing
	case DepStatusQueued:
		return to == DepStatusPreparing
	case DepStatusPreparing:
		return to == DepStatusCloning || to == DepStatusBuilding || to == DepStatusCompleted
	case DepStatusCloning:
		return to == DepStatusBuilding || to == DepStatusCompleted
	case DepStatusBuilding:
		return to == DepStatusProvisioning || to == DepStatusStarting || to == DepStatusHealthchecking
	case DepStatusProvisioning:
		return to == DepStatusStarting || to == DepStatusHealthchecking
	case DepStatusStarting:
		return to == DepStatusHealthchecking
	case DepStatusHealthchecking:
		return to == DepStatusMigrating || to == DepStatusPromoting || to == DepStatusRollback
	case DepStatusMigrating:
		return to == DepStatusPromoting || to == DepStatusRollback
	case DepStatusPromoting:
		return to == DepStatusCleanup || to == DepStatusCompleted
	case DepStatusCleanup:
		return to == DepStatusCompleted
	}

	return false
}
