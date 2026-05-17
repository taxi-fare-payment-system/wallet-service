package server_utils

import "context"

type authBearerCtxKey struct{}

func WithAuthBearer(ctx context.Context, authorization string) context.Context {
	return context.WithValue(ctx, authBearerCtxKey{}, authorization)
}

func AuthBearerFromContext(ctx context.Context) string {
	v, _ := ctx.Value(authBearerCtxKey{}).(string)
	return v
}
