package main

import (
	"context"
	"log"
	"net/http"
	"strings"
	"time"
)

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		w.Header().Set("Access-Control-Allow-Credentials", "true")
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		lrw := &loggingResponseWriter{ResponseWriter: w, statusCode: http.StatusOK}
		next.ServeHTTP(lrw, r)
		log.Printf("%s %s %d %s", r.Method, r.URL.Path, lrw.statusCode, time.Since(start))
	})
}

type loggingResponseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (lrw *loggingResponseWriter) WriteHeader(code int) {
	lrw.statusCode = code
	lrw.ResponseWriter.WriteHeader(code)
}

type contextKey string

const userContextKey contextKey = "user"

func authMiddleware(store *Store) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				writeJSON(w, http.StatusUnauthorized, ApiResponse{
					Error: &ApiError{Code: "UNAUTHORIZED", Message: "Missing authorization header"},
				})
				return
			}
			token := strings.TrimPrefix(authHeader, "Bearer ")
			if token == authHeader {
				writeJSON(w, http.StatusUnauthorized, ApiResponse{
					Error: &ApiError{Code: "UNAUTHORIZED", Message: "Invalid authorization format"},
				})
				return
			}
			session, err := store.GetSession(token)
			if err != nil {
				writeJSON(w, http.StatusUnauthorized, ApiResponse{
					Error: &ApiError{Code: "UNAUTHORIZED", Message: "Invalid or expired session"},
				})
				return
			}
			user, err := store.GetUser(session.UserID)
			if err != nil {
				writeJSON(w, http.StatusUnauthorized, ApiResponse{
					Error: &ApiError{Code: "UNAUTHORIZED", Message: "User not found"},
				})
				return
			}
			ctx := r.Context()
			ctx = context.WithValue(ctx, userContextKey, user)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func requirePermission(permission string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := r.Context().Value(userContextKey).(*User)
		if !ok || user == nil {
			writeJSON(w, http.StatusForbidden, ApiResponse{
				Error: &ApiError{Code: "FORBIDDEN", Message: "Access denied"},
			})
			return
		}
		if user.Role == "admin" {
			next(w, r)
			return
		}
		for _, p := range user.Permissions {
			if p == permission {
				next(w, r)
				return
			}
		}
		writeJSON(w, http.StatusForbidden, ApiResponse{
			Error: &ApiError{Code: "FORBIDDEN", Message: "Insufficient permissions"},
		})
	}
}

func rateLimitMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r)
	})
}
