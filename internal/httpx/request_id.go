package httpx

import (
	"context"
	"net/http"

	"github.com/google/uuid"
)

type requestIDKey struct{}

func RequestIDFromContext(ctx context.Context) string {
	v, _ := ctx.Value(requestIDKey{}).(string)
	return v
}

// RequestIDMiddleware sets/echoes X-Request-ID on both the request context and response.
// If the header is not present, a new UUIDv4 is generated.
func RequestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rid := r.Header.Get("X-Request-ID")
		if rid == "" {
			rid = uuid.NewString()
		}
		w.Header().Set("X-Request-ID", rid)
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), requestIDKey{}, rid)))
	})
}
