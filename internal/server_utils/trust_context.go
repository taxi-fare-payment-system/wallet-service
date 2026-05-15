package server_utils

import "context"

type trustUserIDKey struct{}
type trustUserRoleKey struct{}

// WithTrustUserID stores the gateway user id for outbound internal service calls.
func WithTrustUserID(ctx context.Context, userID string) context.Context {
	return context.WithValue(ctx, trustUserIDKey{}, userID)
}

// TrustUserIDFromContext returns X-User-ID forwarded from the gateway.
func TrustUserIDFromContext(ctx context.Context) string {
	v, _ := ctx.Value(trustUserIDKey{}).(string)
	return v
}

// WithTrustUserRole stores the gateway role for outbound internal service calls.
func WithTrustUserRole(ctx context.Context, role string) context.Context {
	return context.WithValue(ctx, trustUserRoleKey{}, role)
}

// TrustUserRoleFromContext returns X-User-Role forwarded from the gateway.
func TrustUserRoleFromContext(ctx context.Context) string {
	v, _ := ctx.Value(trustUserRoleKey{}).(string)
	return v
}
