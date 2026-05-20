package domain

type RuntimeLimits struct {
	MaxSubscribers       int
	MaxQueueDepth        int
	MaxConcurrentPollers int
	MaxOutboxWorkers     int
}

func DefaultRuntimeLimits() RuntimeLimits {
	return RuntimeLimits{
		MaxSubscribers:       1000,
		MaxQueueDepth:        5000,
		MaxConcurrentPollers: 100,
		MaxOutboxWorkers:     5,
	}
}
