package domain

type FeatureFlags struct {
	EnableDistributedLease     bool
	EnableRealtimePoller       bool
	EnableOutboxDispatcherV2   bool
	EnableStrictReconciliation bool
}

func DefaultFeatureFlags() FeatureFlags {
	return FeatureFlags{
		EnableDistributedLease:     true,
		EnableRealtimePoller:       true,
		EnableOutboxDispatcherV2:   true,
		EnableStrictReconciliation: true,
	}
}
