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
