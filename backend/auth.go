package main

import (
	"context"
	"log"
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt/v5"
)

type contextKey string

const UserKey contextKey = "user_login"

func NewAuth() func(next http.HandlerFunc) http.HandlerFunc {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if r.Method == "OPTIONS" {
				next(w, r)
				return
			}

			auth := r.Header.Get("Authorization")
			if auth == "" || !strings.HasPrefix(auth, "Bearer ") {
				log.Printf("[AUTH-REQ] Missing or invalid Authorization header from %s", r.RemoteAddr)
				http.Error(w, "missing or invalid Authorization header", http.StatusUnauthorized)
				return
			}

			rawToken := strings.TrimPrefix(auth, "Bearer ")

			var claims jwt.MapClaims
			_, _, err := jwt.NewParser().ParseUnverified(rawToken, &claims)
			if err != nil {
				log.Printf("[AUTH-REQ] JWT Parse error: %v (from %s)", err, r.RemoteAddr)
				http.Error(w, "invalid token format", http.StatusUnauthorized)
				return
			}

			login, ok := claims["preferred_username"].(string)
			if !ok || login == "" {
				log.Printf("[AUTH-CLAIMS] Warning: preferred_username missing.")
				http.Error(w, "missing user identifier in token", http.StatusUnauthorized)
				return
			}

			ctx := context.WithValue(r.Context(), UserKey, login)
			next(w, r.WithContext(ctx))
		}
	}
}
