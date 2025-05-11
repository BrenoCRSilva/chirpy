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

	"github.com/BrenoCRSilva/chirpy/internal/auth"
	"github.com/BrenoCRSilva/chirpy/internal/database"
	"github.com/google/uuid"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
)

type apiConfig struct {
	fileServerHits atomic.Int32
	dbQueries      *database.Queries
}

type Chirp struct {
	ID        uuid.UUID `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Body      string    `json:"body"`
	UserID    uuid.UUID `json:"user_id"`
}

type ChirpRequest struct {
	Body   string    `json:"body"`
	UserID uuid.UUID `json:"user_id"`
}

type User struct {
	ID        uuid.UUID `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Email     string    `json:"email"`
}

type userRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (cfg *apiConfig) loginUser(w http.ResponseWriter, r *http.Request) {
	decoder := json.NewDecoder(r.Body)
	req := userRequest{}
	err := decoder.Decode(&req)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		log.Println("Error decoding JSON:", err)
		return
	}
	dbUser, err := cfg.dbQueries.GetUserByEmail(r.Context(), req.Email)
	if err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		log.Println("Error getting user by email:", err)
		return
	}
	if err := auth.CheckPasswordHash(req.Password, dbUser.HashedPassword); err != nil {
		log.Println("Error checking password hash:", err)
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	user := User{
		ID:        dbUser.ID,
		CreatedAt: dbUser.CreatedAt,
		UpdatedAt: dbUser.UpdatedAt,
		Email:     dbUser.Email,
	}
	respondWithJSON(w, http.StatusOK, user)
}

func (cfg *apiConfig) createUser(w http.ResponseWriter, r *http.Request) {
	decoder := json.NewDecoder(r.Body)
	req := userRequest{}
	err := decoder.Decode(&req)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		log.Println("Error decoding JSON:", err)
		return
	}
	hashed, err := auth.HashPassword(req.Password)
	if err != nil {
		log.Println("Error hashing password:", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	dbUserParams := database.CreateUserParams{
		Email:          req.Email,
		HashedPassword: hashed,
	}
	dbUser, err := cfg.dbQueries.CreateUser(r.Context(), dbUserParams)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		log.Println("Error creating user:", err)
		return
	}
	user := User{
		ID:        dbUser.ID,
		CreatedAt: dbUser.CreatedAt,
		UpdatedAt: dbUser.UpdatedAt,
		Email:     dbUser.Email,
	}
	respondWithJSON(w, http.StatusCreated, user)
}

func (cfg *apiConfig) getChirps(w http.ResponseWriter, r *http.Request) {
	chirps := make([]Chirp, 0)
	dbChirps, err := cfg.dbQueries.GetChirps(r.Context())
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		log.Println("Error getting chirps:", err)
	}
	for _, dbChirp := range dbChirps {
		chirps = append(chirps, Chirp{
			ID:        dbChirp.ID,
			CreatedAt: dbChirp.CreatedAt,
			UpdatedAt: dbChirp.UpdatedAt,
			Body:      dbChirp.Body,
			UserID:    dbChirp.UserID,
		})
	}
	respondWithJSON(w, http.StatusOK, chirps)
}

func (cfg *apiConfig) getChirp(w http.ResponseWriter, r *http.Request) {
	idPath := r.PathValue("chirpId")
	uuidChirp, err := uuid.Parse(idPath)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		log.Println("Error parsing chirp ID:", err)
	}
	dbChirp, err := cfg.dbQueries.GetChirp(r.Context(), uuidChirp)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		log.Println("Error getting chirp:", err)
	}
	chirp := Chirp{
		ID:        dbChirp.ID,
		CreatedAt: dbChirp.CreatedAt,
		UpdatedAt: dbChirp.UpdatedAt,
		Body:      dbChirp.Body,
		UserID:    dbChirp.UserID,
	}
	respondWithJSON(w, http.StatusOK, chirp)
}

func (cfg *apiConfig) createChirp(w http.ResponseWriter, r *http.Request) {
	type ReturnError struct {
		Error string `json:"error"`
	}
	decoder := json.NewDecoder(r.Body)
	req := ChirpRequest{}
	err := decoder.Decode(&req)
	if err != nil {
		respondWithJSON(
			w,
			http.StatusInternalServerError,
			ReturnError{Error: "Something went wrong"},
		)
		return
	} else if len(req.Body) > 140 {
		respondWithJSON(w, http.StatusBadRequest, ReturnError{Error: "Chirp is too long"})
		return
	}
	dbChirpParams := database.CreateChirpParams{
		Body:   req.Body,
		UserID: req.UserID,
	}
	dbChirp, err := cfg.dbQueries.CreateChirp(r.Context(), dbChirpParams)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		log.Println("Error creating chirp:", err)
		return
	}
	chirp := Chirp{
		ID:        dbChirp.ID,
		CreatedAt: dbChirp.CreatedAt,
		UpdatedAt: dbChirp.UpdatedAt,
		Body:      dbChirp.Body,
		UserID:    dbChirp.UserID,
	}
	respondWithJSON(w, http.StatusCreated, chirp)
}

func (cfg *apiConfig) resetUsers(w http.ResponseWriter, r *http.Request) {
	platform := os.Getenv("PLATFORM")
	if platform == "" {
		w.WriteHeader(http.StatusInternalServerError)
		log.Println("PLATFORM environment variable is not set")
		return
	}
	if platform == "dev" {
		err := cfg.dbQueries.ResetUsers(r.Context())
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			log.Printf("Error resetting users: %v", err)
			return
		}
		w.WriteHeader(http.StatusOK)
		log.Println("Users reset successfully")
		return
	}
	w.WriteHeader(http.StatusForbidden)
	if _, err := w.Write([]byte(http.StatusText(http.StatusForbidden))); err != nil {
		log.Printf("Error writing response: %v", err)
	}
}

func (cfg *apiConfig) middlewareServerHitCounter(next http.Handler) http.Handler {
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

// func (cfg *apiConfig) resetMetrics(w http.ResponseWriter, _ *http.Request) {
// 	cfg.fileServerHits.Store(0)
// 	w.WriteHeader(http.StatusOK)
// }

func healthStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write([]byte("OK")); err != nil {
		log.Printf("Error writing response: %v", err)
	}
}

func respondWithJSON[T any](w http.ResponseWriter, statusCode int, payload T) {
	data, err := json.Marshal(payload)
	if err != nil {
		log.Println("Error marshalling JSON")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	if _, err := w.Write(data); err != nil {
		log.Printf("Error writing response: %v", err)
	}
}

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Fatalf("Error loading .env file: %v", err)
	}
	dbURL := os.Getenv("DB_URL")
	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		log.Fatalf("Error opening database: %v", err)
	}
	dbQueries := database.New(db)
	cfg := apiConfig{
		dbQueries: dbQueries,
	}
	mux := http.NewServeMux()
	srv := &http.Server{
		Handler: mux,
		Addr:    ":8080",
	}
	handler := http.StripPrefix("/app", http.FileServer(http.Dir(".")))
	mux.Handle("/app/", cfg.middlewareServerHitCounter(handler))
	mux.HandleFunc("GET /api/healthz", healthStatus)
	mux.HandleFunc("GET /admin/metrics", cfg.showMetrics)
	mux.HandleFunc("POST /admin/reset", cfg.resetUsers)
	mux.HandleFunc("POST /api/chirps", cfg.createChirp)
	mux.HandleFunc("POST /api/users", cfg.createUser)
	mux.HandleFunc("GET /api/chirps", cfg.getChirps)
	mux.HandleFunc("GET /api/chirps/{chirpId}", cfg.getChirp)
	mux.HandleFunc("POST /api/login", cfg.loginUser)
	err = srv.ListenAndServe()
	log.Fatal(err)
}
