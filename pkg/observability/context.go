package observability

import (
	"context"
	"crypto/rand"
	"fmt"
)

// Context keys for tracing
type contextKey string

const (
	traceIDKey   contextKey = "trace_id"
	paymentIDKey contextKey = "payment_id"
	userIDKey    contextKey = "user_id"
)

// WithTraceID adds a trace ID to the context
func WithTraceID(ctx context.Context, traceID string) context.Context {
	return context.WithValue(ctx, traceIDKey, traceID)
}

// GetTraceID retrieves the trace ID from context
func GetTraceID(ctx context.Context) string {
	if traceID, ok := ctx.Value(traceIDKey).(string); ok {
		return traceID
	}
	return ""
}

// WithPaymentID adds a payment ID to the context
func WithPaymentID(ctx context.Context, paymentID string) context.Context {
	return context.WithValue(ctx, paymentIDKey, paymentID)
}

// GetPaymentID retrieves the payment ID from context
func GetPaymentID(ctx context.Context) string {
	if paymentID, ok := ctx.Value(paymentIDKey).(string); ok {
		return paymentID
	}
	return ""
}

// WithUserID adds a user ID to the context
func WithUserID(ctx context.Context, userID string) context.Context {
	return context.WithValue(ctx, userIDKey, userID)
}

// GetUserID retrieves the user ID from context
func GetUserID(ctx context.Context) string {
	if userID, ok := ctx.Value(userIDKey).(string); ok {
		return userID
	}
	return ""
}

// GenerateTraceID generates a unique trace ID
func GenerateTraceID() string {
	bytes := make([]byte, 16)
	rand.Read(bytes)
	return fmt.Sprintf("%x", bytes)
}
