package domain

import (
	"context"

	"github.com/google/uuid"
)

type TraceContext struct {
	TraceID       string
	OperationID   string
	CorrelationID string
}

type traceCtxKey struct{}

func WithTraceContext(ctx context.Context, tc TraceContext) context.Context {
	return context.WithValue(ctx, traceCtxKey{}, tc)
}

func GetTraceContext(ctx context.Context) TraceContext {
	tc, ok := ctx.Value(traceCtxKey{}).(TraceContext)
	if !ok {
		fresh := uuid.New().String()
		return TraceContext{
			TraceID:       fresh,
			OperationID:   fresh,
			CorrelationID: fresh,
		}
	}
	return tc
}

func EnsureTraceContext(ctx context.Context) context.Context {
	if val := ctx.Value(traceCtxKey{}); val != nil {
		return ctx
	}
	fresh := uuid.New().String()
	return WithTraceContext(ctx, TraceContext{
		TraceID:       fresh,
		OperationID:   fresh,
		CorrelationID: fresh,
	})
}
