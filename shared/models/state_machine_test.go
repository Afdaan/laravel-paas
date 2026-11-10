package models

import "testing"

func TestDeploymentTransitionAllowsPreHealthcheckMigration(t *testing.T) {
	// Laravel + SQLite migrates before the readiness probe (deployment_worker.go).
	if !IsValidDeploymentTransition(DepStatusStarting, DepStatusMigrating) {
		t.Fatal("starting -> migrating must be valid")
	}
	if !IsValidDeploymentTransition(DepStatusStarting, DepStatusHealthchecking) {
		t.Fatal("starting -> healthchecking must stay valid")
	}
	if IsValidDeploymentTransition(DepStatusStarting, DepStatusPromoting) {
		t.Fatal("starting -> promoting must stay invalid")
	}
}

func TestDeploymentTransitionAllowsRecoveryRequeue(t *testing.T) {
	inFlight := []DeploymentStatus{
		DepStatusPreparing,
		DepStatusCloning,
		DepStatusBuilding,
		DepStatusStarting,
		DepStatusHealthchecking,
		DepStatusMigrating,
		DepStatusPromoting,
	}

	for _, status := range inFlight {
		if !IsValidDeploymentTransition(status, DepStatusQueued) {
			t.Fatalf("%s -> queued must be valid for recovery", status)
		}
	}
}

func TestTerminalDeploymentStatuses(t *testing.T) {
	terminal := []DeploymentStatus{DepStatusCompleted, DepStatusFailed, DepStatusCancelled, DepStatusRollback}
	for _, status := range terminal {
		if !IsTerminalDeploymentStatus(status) {
			t.Fatalf("%s must be terminal", status)
		}
	}
	for _, status := range []DeploymentStatus{DepStatusQueued, DepStatusPreparing, DepStatusBuilding, DepStatusPromoting} {
		if IsTerminalDeploymentStatus(status) {
			t.Fatalf("%s must not be terminal", status)
		}
	}
}
