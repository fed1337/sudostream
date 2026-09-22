package observability

import (
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	unknownRouteLabel     = "unknown"
	httpStatusClientError = 400
	httpStatusServerError = 500
)

const (
	// RequestIDKey is the Gin context key for the request correlation ID.
	RequestIDKey = "request_id"
	// UserIDKey is the Gin context key for the authenticated user ID.
	UserIDKey = "user_id"
)

// RequestLogger emits structured slog records after each request completes.
func RequestLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		requestID := newRequestID()
		c.Set(RequestIDKey, requestID)
		c.Header("X-Request-Id", requestID)

		start := time.Now()
		c.Next()

		attrs := []any{
			slog.String("request_id", requestID),
			slog.String("method", c.Request.Method),
			slog.String("route", routeLabel(c)),
			slog.String("path", c.Request.URL.Path),
			slog.Int("status", c.Writer.Status()),
			slog.Int64("duration_ms", time.Since(start).Milliseconds()),
		}

		if userID, ok := c.Get(UserIDKey); ok {
			if id, valid := userID.(string); valid && id != "" {
				attrs = append(attrs, slog.String("user_id", id))
			}
		}

		level := slog.LevelInfo
		statusCode := c.Writer.Status()
		switch {
		case statusCode >= httpStatusServerError:
			level = slog.LevelError
		case statusCode >= httpStatusClientError:
			level = slog.LevelWarn
		}

		slog.Log(c.Request.Context(), level, "http request", attrs...)
	}
}

// LogAction emits a structured operational log with standard fields.
func LogAction(c *gin.Context, action string, attrs ...any) {
	fields := []any{
		slog.String("action", action),
		slog.String("route", routeLabel(c)),
	}

	if requestID, ok := c.Get(RequestIDKey); ok {
		if id, valid := requestID.(string); valid && id != "" {
			fields = append(fields, slog.String("request_id", id))
		}
	}

	if userID, ok := c.Get(UserIDKey); ok {
		if id, valid := userID.(string); valid && id != "" {
			fields = append(fields, slog.String("user_id", id))
		}
	}

	fields = append(fields, attrs...)
	slog.Info("operation", fields...)
}

func routeLabel(c *gin.Context) string {
	route := c.FullPath()
	if route == "" {
		return unknownRouteLabel
	}

	return route
}

func newRequestID() string {
	var buf [16]byte
	_, err := rand.Read(buf[:])
	if err != nil {
		return unknownRouteLabel
	}

	return hex.EncodeToString(buf[:])
}
