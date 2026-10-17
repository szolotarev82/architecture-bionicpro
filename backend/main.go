package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
)

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS, HEAD")
		w.Header().Set("Access-Control-Allow-Headers", "*")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func reportHandler(repo *ClickHouseRepo) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		login, ok := r.Context().Value(UserKey).(string)
		if !ok || login == "" {
			http.Error(w, "unauthorized: user login not found in context", http.StatusUnauthorized)
			return
		}

		reports, err := repo.GetUserReports(r.Context(), login)
		if err != nil {
			log.Printf("Error fetching reports for user %s: %v", login, err)
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(reports); err != nil {
			log.Printf("Failed to encode JSON response: %v", err)
		}
	}
}

func main() {
	port := os.Getenv("SERVER_PORT")
	if port == "" {
		port = "8000"
	}

	dsn := os.Getenv("CLICKHOUSE_DSN")
	if dsn == "" {
		log.Fatal("missing required env var: CLICKHOUSE_DSN")
	}

	log.Printf("Initializing repo with DSN: %s", dsn)
	repo := NewClickHouseRepo(dsn)

	defer func() {
		if err := repo.Close(); err != nil {
			log.Printf("Error closing ClickHouse connections: %v", err)
		}
	}()

	authMiddleware := NewAuth()

	mux := http.NewServeMux()
	protectedHandler := authMiddleware(reportHandler(repo))

	mux.HandleFunc("/reports", protectedHandler)
	mux.HandleFunc("/reports/", protectedHandler)

	handlerWithCors := corsMiddleware(mux)

	srv := &http.Server{
		Addr:    "0.0.0.0:" + port,
		Handler: handlerWithCors,
	}

	log.Printf("Backend starting on host 0.0.0.0 and port %s", port)
	log.Fatal(srv.ListenAndServe())
}
