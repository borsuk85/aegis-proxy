package logger

import (
	"log"
	"net/http"
	"time"
)

// Logger handles application logging
type Logger struct {
	enabled   bool
	accessLog bool
	level     string
}

// New creates a new logger instance
func New(enabled, accessLog bool, level string) *Logger {
	return &Logger{
		enabled:   enabled,
		accessLog: accessLog,
		level:     level,
	}
}

// Debug logs a debug message
func (l *Logger) Debug(format string, v ...interface{}) {
	if !l.enabled || (l.level != "debug") {
		return
	}
	log.Printf("[DEBUG] "+format, v...)
}

// Info logs an info message
func (l *Logger) Info(format string, v ...interface{}) {
	if !l.enabled || (l.level != "debug" && l.level != "info") {
		return
	}
	log.Printf("[INFO] "+format, v...)
}

// Error logs an error message
func (l *Logger) Error(format string, v ...interface{}) {
	if !l.enabled {
		return
	}
	log.Printf("[ERROR] "+format, v...)
}

// responseWriter wraps http.ResponseWriter to capture status code
type responseWriter struct {
	http.ResponseWriter
	statusCode int
	written    int64
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	n, err := rw.ResponseWriter.Write(b)
	rw.written += int64(n)
	return n, err
}

// AccessLogMiddleware creates middleware for access logging
func (l *Logger) AccessLogMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !l.enabled || !l.accessLog {
			next.ServeHTTP(w, r)
			return
		}

		start := time.Now()
		wrapped := &responseWriter{
			ResponseWriter: w,
			statusCode:     200, // default status
		}

		next.ServeHTTP(wrapped, r)

		duration := time.Since(start)
		cacheStatus := wrapped.Header().Get("X-Cache")
		if cacheStatus == "" {
			cacheStatus = "-"
		}

		log.Printf("[ACCESS] %s %s %s %d %dms cache=%s bytes=%d",
			r.RemoteAddr,
			r.Method,
			r.URL.Path,
			wrapped.statusCode,
			duration.Milliseconds(),
			cacheStatus,
			wrapped.written,
		)
	})
}
