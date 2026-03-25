package server_utils

import (
	"context"
)

type RequestIDKey struct{}

func RequestIDFromContext(ctx context.Context) string {
	v, _ := ctx.Value(RequestIDKey{}).(string)
	return v
}
