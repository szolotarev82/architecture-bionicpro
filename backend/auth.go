package main

import (
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"math/big"
	"net/http"
	"os"
	"strings"
	"time"
)

type contextKey string

const UserKey contextKey = "user_login"

type jwtHeader struct {
	Alg string `json:"alg"`
	Kid string `json:"kid"`
}

type jwtPayload struct {
	Exp      int64  `json:"exp"`
	Iss      string `json:"iss"`
	Username string `json:"preferred_username"`
}

type JWK struct {
	Kid string `json:"kid"`
	N   string `json:"n"`
	E   string `json:"e"`
}

type JWKS struct {
	Keys []JWK `json:"keys"`
}

func decodeSegment(seg string) ([]byte, error) {
	if l := len(seg) % 4; l > 0 {
		seg += strings.Repeat("=", 4-l)
	}
	return base64.URLEncoding.DecodeString(seg)
}

func fetchRSAPublicKey(jwksURL, kid string) (*rsa.PublicKey, error) {
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(jwksURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var jwks JWKS
	if err := json.NewDecoder(resp.Body).Decode(&jwks); err != nil {
		return nil, err
	}

	for _, key := range jwks.Keys {
		if key.Kid == kid {
			nb, err := base64.RawURLEncoding.DecodeString(key.N)
			if err != nil {
				return nil, err
			}
			eb, err := base64.RawURLEncoding.DecodeString(key.E)
			if err != nil {
				return nil, err
			}

			var e int
			for _, b := range eb {
				e = (e << 8) | int(b)
			}

			return &rsa.PublicKey{
				N: new(big.Int).SetBytes(nb),
				E: e,
			}, nil
		}
	}
	return nil, fmt.Errorf("key not found in JWKS")
}

func NewAuth() func(next http.HandlerFunc) http.HandlerFunc {
	jwksURL := os.Getenv("JWKS_URL")
	if jwksURL == "" {
		log.Fatal("missing required env var: JWKS_URL")
	}

	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if r.Method == "OPTIONS" {
				next(w, r)
				return
			}

			auth := r.Header.Get("Authorization")
			if auth == "" || !strings.HasPrefix(auth, "Bearer ") {
				log.Printf("[AUTH-REQ] Missing or invalid Authorization header")
				http.Error(w, "missing or invalid Authorization header", http.StatusUnauthorized)
				return
			}

			rawToken := strings.TrimPrefix(auth, "Bearer ")
			parts := strings.Split(rawToken, ".")
			if len(parts) != 3 {
				http.Error(w, "invalid token format", http.StatusUnauthorized)
				return
			}

			headerBytes, err := decodeSegment(parts[0])
			if err != nil {
				http.Error(w, "invalid header encoding", http.StatusUnauthorized)
				return
			}

			var header jwtHeader
			if err := json.Unmarshal(headerBytes, &header); err != nil {
				http.Error(w, "invalid header json", http.StatusUnauthorized)
				return
			}

			if header.Alg != "RS256" {
				log.Printf("[AUTH] Unsupported algorithm: %s", header.Alg)
				http.Error(w, "unsupported signing algorithm", http.StatusUnauthorized)
				return
			}

			payloadBytes, err := decodeSegment(parts[1])
			if err != nil {
				http.Error(w, "invalid payload encoding", http.StatusUnauthorized)
				return
			}

			var payload jwtPayload
			if err := json.Unmarshal(payloadBytes, &payload); err != nil {
				http.Error(w, "invalid payload json", http.StatusUnauthorized)
				return
			}

			pubKey, err := fetchRSAPublicKey(jwksURL, header.Kid)
			if err != nil {
				log.Printf("[AUTH] Failed to fetch public key from Keycloak: %v", err)
				http.Error(w, "auth service unavailable", http.StatusServiceUnavailable)
				return
			}

			signatureBytes, err := decodeSegment(parts[2])
			if err != nil {
				http.Error(w, "invalid signature encoding", http.StatusUnauthorized)
				return
			}

			signedContent := parts[0] + "." + parts[1]
			hasher := sha256.New()
			hasher.Write([]byte(signedContent))
			hashed := hasher.Sum(nil)

			err = rsa.VerifyPKCS1v15(pubKey, crypto.SHA256, hashed, signatureBytes)
			if err != nil {
				log.Printf("[AUTH-CRYPT] Signature verification failed: %v", err)
				http.Error(w, "invalid token signature", http.StatusUnauthorized)
				return
			}

			if time.Now().Unix() > payload.Exp {
				log.Printf("[AUTH] Token expired")
				http.Error(w, "token has expired", http.StatusUnauthorized)
				return
			}

			if !strings.Contains(payload.Iss, "/realms/reports-realm") {
				log.Printf("[AUTH] Invalid token issuer: %s", payload.Iss)
				http.Error(w, "untrusted token issuer", http.StatusUnauthorized)
				return
			}

			if payload.Username == "" {
				http.Error(w, "missing user identifier in token", http.StatusUnauthorized)
				return
			}

			ctx := context.WithValue(r.Context(), UserKey, payload.Username)
			next(w, r.WithContext(ctx))
		}
	}
}
