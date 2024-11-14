package middleware

import (
	"context"
	"net/http"
	"time"

	"tig/internal/logging"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

type responseWriter struct {
    http.ResponseWriter
    status int
}

func (w *responseWriter) WriteHeader(status int) {
    w.status = status
    w.ResponseWriter.WriteHeader(status)
}

type Middleware func(http.Handler) http.Handler

func Chain(h http.Handler, middlewares ...Middleware) http.Handler {
    for _, m := range middlewares {
        h = m(h)
    }
    return h
}

func RequestID(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        requestID := uuid.New().String()
        ctx := context.WithValue(r.Context(), "request_id", requestID)
        w.Header().Set("X-Request-ID", requestID)
        next.ServeHTTP(w, r.WithContext(ctx))
    })
}

func Logger(logger *logging.Logger) Middleware {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            start := time.Now()
            
            // Create response wrapper to capture status code
            wrapper := &responseWriter{ResponseWriter: w, status: http.StatusOK}
            
            next.ServeHTTP(wrapper, r)
            
            logger.WithRequestID(r.Context()).Info("request completed",
                zap.String("method", r.Method),
                zap.String("path", r.URL.Path),
                zap.Int("status", wrapper.status),
                zap.Duration("duration", time.Since(start)),
            )
        })
    }
}

func Recover(logger *logging.Logger) Middleware {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            defer func() {
                if err := recover(); err != nil {
                    logger.WithRequestID(r.Context()).Error("panic recovered",
                        zap.Any("error", err),
                    )
                    http.Error(w, "Internal Server Error", http.StatusInternalServerError)
                }
            }()
            next.ServeHTTP(w, r)
        })
    }
}
