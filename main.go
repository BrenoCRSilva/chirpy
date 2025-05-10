package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync/atomic"
	"time"

	"github.com/BrenoCRSilva/chirpy/internal/database"
	"github.com/google/uuid"
	_ "github.com/lib/pq"
)

type apiConfig struct {
	fileServerHits atomic.Int32
	dbQueries      *database.Queries
}

type User struct {
	ID        uuid.UUID `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Email     string    `json:"email"`
}

type userRequest struct {
	Email string `json:"email"`
}

func (cfg *apiConfig) placeholder(w http.ResponseWriter, r *http.Request) {
	decoder := json.NewDecoder(r.Body)
	req := userRequest{}
	err := decoder.Decode(&req)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
	}
	user, err := cfg.dbQueries.CreateUser(r.Context(), req.Email)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
	}
	data, err := json.Marshal(user)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
	}
	if _, err := w.Write(data); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
}

func (cfg *apiConfig) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cfg.fileServerHits.Add(1)
		next.ServeHTTP(w, r)
	})
}

func (cfg *apiConfig) showMetrics(w http.ResponseWriter, r *http.Request) {
	hits := cfg.fileServerHits.Load()
	text := fmt.Sprintf(`
    <html>
      <body>
        <h1>Welcome, Chirpy Admin</h1>
        <p>Chirpy has been visited %d times!</p>
      </body>
    </html>
    `, hits)
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write([]byte(text)); err != nil {
		log.Printf("Error writing response: %v", err)
	}
}

func (cfg *apiConfig) resetMetrics(w http.ResponseWriter, _ *http.Request) {
	cfg.fileServerHits.Store(0)
	w.WriteHeader(http.StatusOK)
}

func chirpValidateHandler(w http.ResponseWriter, r *http.Request) {
	type Chirp struct {
		Body string `json:"body"`
	}
	type ReturnError struct {
		Error string `json:"error"`
	}
	type ReturnValid struct {
		Valid bool `json:"valid"`
	}
	decoder := json.NewDecoder(r.Body)
	chirp := Chirp{}
	err := decoder.Decode(&chirp)
	if err != nil {
		error := ReturnError{
			Error: "Something went wrong",
		}
		data, err := json.Marshal(error)
		if err != nil {
			w.WriteHeader(500)
			log.Println("Error marshalling JSON")
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(500)
		if _, err := w.Write(data); err != nil {
			log.Printf("Error writing response: %v", err)
		}
		return
	} else if len(chirp.Body) > 140 {
		error := ReturnError{
			Error: "Chirp is too long",
		}
		data, err := json.Marshal(error)
		if err != nil {
			w.WriteHeader(500)
			log.Println("Error marshalling JSON")
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(400)
		if _, err := w.Write(data); err != nil {
			log.Printf("Error writing response: %v", err)
		}
		return
	} else {
		valid := ReturnValid{
			Valid: true,
		}
		data, err := json.Marshal(valid)
		if err != nil {
			w.WriteHeader(500)
			log.Println("Error marshalling JSON")
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		if _, err := w.Write(data); err != nil {
			log.Printf("Error writing response: %v", err)
		}
		return
	}
}

func main() {
	dbURL := os.Getenv("DB_URL")
	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		log.Fatalf("Error opening database: %v", err)
	}
	dbQueries := database.New(db)
	mux := http.NewServeMux()
	srv := &http.Server{
		Handler: mux,
		Addr:    ":8080",
	}
	cfg := apiConfig{dbQueries: dbQueries}
	handler := http.StripPrefix("/app", http.FileServer(http.Dir(".")))
	mux.Handle("/app/", cfg.middleware(handler))
	mux.HandleFunc("GET /api/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write([]byte("OK")); err != nil {
			log.Printf("Error writing response: %v", err)
		}
	})
	mux.HandleFunc("GET /admin/metrics", cfg.showMetrics)
	mux.HandleFunc("POST /admin/reset", cfg.resetMetrics)
	mux.HandleFunc("POST /api/validate_chirp", chirpValidateHandler)
	mux.HandleFunc("POST /api/users", cfg.placeholder)
	err = srv.ListenAndServe()
	log.Fatal(err)
}
