package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/Senaphim/Chirpy/internal/database"
	"github.com/google/uuid"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
)

type apiConfig struct {
	fileserverHits atomic.Int32
	queries        *database.Queries
	platform       string
}

func (cfg *apiConfig) middlewareMetricInc(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cfg.fileserverHits.Add(1)
		next.ServeHTTP(w, r)
	})
}

func (cfg *apiConfig) handleHits(w http.ResponseWriter, r *http.Request) {
	w.Header().Add("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	fmtStr := `<html>
  <body>
    <h1>Welcome, Chirpy Admin</h1>
    <p>Chirpy has been visited %d times!</p>
  </body>
</html>
	`
	fmt.Fprintf(w, fmtStr, cfg.fileserverHits.Load())
}

func (cfg *apiConfig) handleReset(w http.ResponseWriter, r *http.Request) {
	if cfg.platform != "dev" {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	err := cfg.queries.DeleteAll(r.Context())
	if err != nil {
		log.Printf("Error deleteing all users:\n%v", err)
		w.WriteHeader(500)
		return
	}

	cfg.fileserverHits.Store(0)
	w.Header().Add("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(http.StatusText(http.StatusOK)))
}

func main() {
	cfg := &apiConfig{}
	serveMux := http.NewServeMux()
	if err := godotenv.Load(); err != nil {
		log.Printf("Error loading environment variables: %v", err)
		return
	}
	dbUrl := os.Getenv("DB_URL")
	cfg.platform = os.Getenv("PLATFORM")
	db, err := sql.Open("postgres", dbUrl)
	if err != nil {
		log.Printf("Error opening database: %v", err)
		return
	}
	cfg.queries = database.New(db)

	// File server handler
	fsHandler := http.StripPrefix("/app", http.FileServer(http.Dir(".")))
	serveMux.Handle("/app/", cfg.middlewareMetricInc(fsHandler))

	// Other handlers
	hhe := http.HandlerFunc(handleHealth)
	serveMux.Handle("GET /api/healthz", hhe)
	hhi := http.HandlerFunc(cfg.handleHits)
	serveMux.Handle("GET /admin/metrics", hhi)
	hr := http.HandlerFunc(cfg.handleReset)
	serveMux.Handle("POST /admin/reset", hr)
	hc := http.HandlerFunc(handleChirp)
	serveMux.Handle("POST /api/validate_chirp", hc)
	hcr := http.HandlerFunc(cfg.handlerCreateUser)
	serveMux.Handle("POST /api/users", hcr)

	// Start server
	server := http.Server{
		Addr:    ":8080",
		Handler: serveMux,
	}
	err = server.ListenAndServe()
	if err != nil {
		fmt.Println(fmt.Errorf("Error serving request:\n%v", err))
	}

}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Add("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(http.StatusText(http.StatusOK)))
}

func handleChirp(w http.ResponseWriter, r *http.Request) {
	type parameters struct {
		Body string `json:"body"`
	}

	decoder := json.NewDecoder(r.Body)
	params := parameters{}
	if err := decoder.Decode(&params); err != nil {
		helperJsonError(w, "Error decoding parameters: %s", err)
		return
	}

	type returnVals struct {
		CleanedBody string `json:"cleaned_body"`
	}

	if len(params.Body) > 140 {
		type responseJson struct {
			Error string
		}

		resp := responseJson{
			Error: "Chirp is too long",
		}
		dat, err := json.Marshal(resp)
		if err != nil {
			helperJsonError(w, "Error marshalling response: %s", err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(400)
		w.Write(dat)
		return
	}

	cleanString := helperCleanString(params.Body)

	resp := returnVals{
		CleanedBody: cleanString,
	}
	dat, err := json.Marshal(resp)
	if err != nil {
		helperJsonError(w, "Error marshalling response: %s", err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(dat)
}

func helperJsonError(w http.ResponseWriter, responseMsg string, err error) {
	type responseJson struct {
		Error string
	}

	log.Printf(responseMsg, err)

	resp := responseJson{
		Error: "something went wrong",
	}
	dat, err := json.Marshal(resp)
	if err != nil {
		log.Printf("Error marshalling error response: %s", err)
		w.WriteHeader(500)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusInternalServerError)
	w.Write(dat)
}

func helperCleanString(post string) string {
	profanity := []string{"kerfuffle", "sharbert", "fornax", "Kerfuffle", "Sharbert", "Fornax"}
	cleanPost := post

	for _, profane := range profanity {
		cleanPost = strings.ReplaceAll(cleanPost, profane, "****")
	}

	return cleanPost
}

func (cfg *apiConfig) handlerCreateUser(w http.ResponseWriter, r *http.Request) {
	type email struct {
		Email string `json:"email"`
	}

	decoder := json.NewDecoder(r.Body)
	em := email{}
	if err := decoder.Decode(&em); err != nil {
		helperJsonError(w, "Error decoding email:%v", err)
		return
	}

	user, err := cfg.helperCreateUser(em.Email, r)
	if err != nil {
		log.Printf("Error creating user:\n%v", err)
		w.WriteHeader(500)
		return
	}

	type retUser struct {
		ID        uuid.UUID `json:"id"`
		CreatedAt time.Time `json:"created_at"`
		UpdatedAt time.Time `json:"updated_at"`
		Email     string    `json:"email"`
	}
	rUser := retUser{
		ID:        user.ID,
		CreatedAt: user.CreatedAt,
		UpdatedAt: user.UpdatedAt,
		Email:     user.Email,
	}
	dat, err := json.Marshal(rUser)
	if err != nil {
		helperJsonError(w, "Error marshalling response: %s", err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(201)
	w.Write(dat)
}

func (cfg *apiConfig) helperCreateUser(email string, r *http.Request) (database.User, error) {
	userDetails := database.CreateUserParams{
		ID:        uuid.New(),
		CreatedAt: time.Now().Local(),
		UpdatedAt: time.Now().Local(),
		Email:     email,
	}

	user, err := cfg.queries.CreateUser(r.Context(), userDetails)
	if err != nil {
		fmtErr := fmt.Errorf("Error adding user to database:\n%v", err)
		return database.User{}, fmtErr
	}

	return user, nil
}
