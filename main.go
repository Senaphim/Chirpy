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

	"github.com/Senaphim/Chirpy/internal/auth"
	"github.com/Senaphim/Chirpy/internal/database"
	"github.com/google/uuid"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
)

type apiConfig struct {
	fileserverHits atomic.Int32
	queries        *database.Queries
	platform       string
	secret         string
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

	err = cfg.queries.ResetChirps(r.Context())
	if err != nil {
		log.Printf("Error deleting all chirps:\n%v", err)
		w.WriteHeader(500)
		return
	}

	err = cfg.queries.ResetRefreshTokens(r.Context())
	if err != nil {
		log.Printf("Error deleting all refresh tokens:\n%v", err)
		w.WriteHeader(http.StatusInternalServerError)
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
	cfg.secret = os.Getenv("SECRET")
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
	hc := http.HandlerFunc(cfg.handleChirp)
	serveMux.Handle("POST /api/chirps", hc)
	hcr := http.HandlerFunc(cfg.handlerCreateUser)
	serveMux.Handle("POST /api/users", hcr)
	hac := http.HandlerFunc(cfg.handlerAllChirps)
	serveMux.Handle("GET /api/chirps", hac)
	hci := http.HandlerFunc(cfg.handlerChirpById)
	serveMux.Handle("GET /api/chirps/{chirpID}", hci)
	hl := http.HandlerFunc(cfg.handlerLogin)
	serveMux.Handle("POST /api/login", hl)
	hre := http.HandlerFunc(cfg.handlerRefresh)
	serveMux.Handle("POST /api/refresh", hre)
	hrev := http.HandlerFunc(cfg.handlerRevoke)
	serveMux.Handle("POST /api/revoke", hrev)
	hcpe := http.HandlerFunc(cfg.handleChangePwd)
	serveMux.Handle("PUT /api/users", hcpe)
	hdc := http.HandlerFunc(cfg.handlerDeleteChirp)
	serveMux.Handle("DELETE /api/chirps/{chirpID}", hdc)

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

func (cfg *apiConfig) handleChirp(w http.ResponseWriter, r *http.Request) {
	type parameters struct {
		Body string    `json:"body"`
		Id   uuid.UUID `json:"user_id"`
	}

	jwt, err := auth.GetBearerToken(r.Header)
	if err != nil {
		log.Printf("Error getting bearer token:\n%v", err)
		w.WriteHeader(500)
		return
	}

	userId, err := auth.ValidateJWT(jwt, cfg.secret)
	if err != nil {
		log.Printf("Invalid JWT:\n%v", err)
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	decoder := json.NewDecoder(r.Body)
	params := parameters{
		Id: userId,
	}
	if err := decoder.Decode(&params); err != nil {
		helperJsonError(w, "Error decoding parameters: %s", err)
		return
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
	chirp, err := cfg.helperCreateChirp(cleanString, params.Id, r)
	if err != nil {
		log.Printf("Error creating chirp:\n%v", err)
		w.WriteHeader(500)
		return
	}

	type returnVals struct {
		Id        uuid.UUID `json:"id"`
		CreatedAt time.Time `json:"created_at"`
		UpdatedAt time.Time `json:"updated_at"`
		Body      string    `json:"body"`
		UserId    uuid.UUID `json:"user_id"`
	}

	resp := returnVals{
		Id:        chirp.ID,
		CreatedAt: chirp.CreatedAt,
		UpdatedAt: chirp.UpdatedAt,
		Body:      chirp.Body,
		UserId:    chirp.UserID,
	}
	dat, err := json.Marshal(resp)
	if err != nil {
		helperJsonError(w, "Error marshalling response: %s", err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(201)
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
		Email    string `json:"email"`
		Password string `json:"password"`
	}

	decoder := json.NewDecoder(r.Body)
	em := email{}
	if err := decoder.Decode(&em); err != nil {
		helperJsonError(w, "Error decoding email:%v", err)
		return
	}

	user, err := cfg.helperCreateUser(em.Email, em.Password, r)
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

func (cfg *apiConfig) helperCreateUser(
	email string,
	password string,
	r *http.Request,
) (database.User, error) {

	hashedPassword, err := auth.HashPassword(password)
	if err != nil {
		fmtErr := fmt.Errorf("Error with password:\n%v", err)
		return database.User{}, fmtErr
	}
	userDetails := database.CreateUserParams{
		ID:             uuid.New(),
		CreatedAt:      time.Now().Local(),
		UpdatedAt:      time.Now().Local(),
		Email:          email,
		HashedPassword: hashedPassword,
	}

	user, err := cfg.queries.CreateUser(r.Context(), userDetails)
	if err != nil {
		fmtErr := fmt.Errorf("Error adding user to database:\n%v", err)
		return database.User{}, fmtErr
	}

	return user, nil
}

func (cfg *apiConfig) helperCreateChirp(
	body string,
	user uuid.UUID,
	r *http.Request,
) (database.Chirp, error) {

	chirpDetails := database.CreateChirpParams{
		ID:        uuid.New(),
		CreatedAt: time.Now().Local(),
		UpdatedAt: time.Now().Local(),
		Body:      body,
		UserID:    user,
	}

	chirp, err := cfg.queries.CreateChirp(r.Context(), chirpDetails)
	if err != nil {
		fmtErr := fmt.Errorf("Error adding chirp to database:\n%v", err)
		return database.Chirp{}, fmtErr
	}

	return chirp, nil
}

func (cfg *apiConfig) handlerAllChirps(w http.ResponseWriter, r *http.Request) {
	chirps, err := cfg.queries.AllChirps(r.Context())
	if err != nil {
		log.Printf("Error fetching chirps:\n%v", err)
		return
	}

	type returnChirp struct {
		Id        uuid.UUID `json:"id"`
		CreatedAt time.Time `json:"created_at"`
		UpdatedAt time.Time `json:"updated_at"`
		Body      string    `json:"body"`
		UserId    uuid.UUID `json:"user_id"`
	}

	returnArray := []returnChirp{}

	for _, chirp := range chirps {
		rChirp := returnChirp{
			Id:        chirp.ID,
			CreatedAt: chirp.CreatedAt,
			UpdatedAt: chirp.UpdatedAt,
			Body:      chirp.Body,
			UserId:    chirp.UserID,
		}

		returnArray = append(returnArray, rChirp)
	}

	dat, err := json.Marshal(returnArray)
	if err != nil {
		helperJsonError(w, "error marshalling json response:%v", err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(200)
	w.Write(dat)
}

func (cfg *apiConfig) handlerChirpById(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("chirpID")
	chirp_uuid, err := uuid.Parse(id)
	if err != nil {
		log.Printf("Error parsing chirpID:\n%v", err)
		w.WriteHeader(http.StatusNotFound)
		return
	}

	chirp, err := cfg.queries.GetChirpById(r.Context(), chirp_uuid)
	if err != nil {
		log.Printf("Error fetching chirps:\n%v", err)
		w.WriteHeader(http.StatusNotFound)
		return
	}

	type returnChirp struct {
		Id        uuid.UUID `json:"id"`
		CreatedAt time.Time `json:"created_at"`
		UpdatedAt time.Time `json:"updated_at"`
		Body      string    `json:"body"`
		UserId    uuid.UUID `json:"user_id"`
	}
	rChirp := returnChirp{
		Id:        chirp.ID,
		CreatedAt: chirp.CreatedAt,
		UpdatedAt: chirp.UpdatedAt,
		Body:      chirp.Body,
		UserId:    chirp.UserID,
	}

	dat, err := json.Marshal(rChirp)
	if err != nil {
		helperJsonError(w, "error marshalling json response:%v", err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(200)
	w.Write(dat)
}

func (cfg *apiConfig) handlerLogin(w http.ResponseWriter, r *http.Request) {
	type user struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}

	decoder := json.NewDecoder(r.Body)
	usr := user{
		Email:    "",
		Password: "",
	}
	if err := decoder.Decode(&usr); err != nil {
		helperJsonError(w, "Error decoding user:\n%v", err)
		return
	}

	refreshToken, err := auth.MakeRefreshToken()
	if err != nil {
		log.Printf("Error making refresh token:\n%v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	dbUsr, err := cfg.queries.GetUserByEmail(r.Context(), usr.Email)
	if err != nil {
		log.Printf("Error fetching user from email:\n%v", err)
		w.WriteHeader(401)
		w.Write([]byte("Incorrect email or password"))
		return
	}

	err = auth.CheckPasswordHash(usr.Password, dbUsr.HashedPassword)
	if err != nil {
		log.Printf("Error bad password:\n%v", err)
		w.WriteHeader(401)
		w.Write([]byte("Incorrect email or password"))
		return
	}

	jwtExpiration, err := time.ParseDuration("3600s")
	if err != nil {
		log.Printf("Error parsing duration string:\n%v", err)
		w.WriteHeader(500)
		return
	}
	jwt, err := auth.MakeJWT(dbUsr.ID, cfg.secret, jwtExpiration)
	if err != nil {
		log.Printf("Error making JWT:\n%v", err)
		w.WriteHeader(500)
		return
	}

	refreshTokenExpiration, err := time.ParseDuration("1440h")
	if err != nil {
		log.Printf("Error parsing duration:\n%v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	expiryTime := time.Now().Add(refreshTokenExpiration)
	refreshParams := database.CreateRefreshTokenParams{
		Token:     refreshToken,
		CreatedAt: time.Now().Local(),
		UpdatedAt: time.Now().Local(),
		UserID:    dbUsr.ID,
		ExpiresAt: expiryTime,
	}
	cfg.queries.CreateRefreshToken(r.Context(), refreshParams)

	type retUser struct {
		ID           uuid.UUID `json:"id"`
		CreatedAt    time.Time `json:"created_at"`
		UpdatedAt    time.Time `json:"updated_at"`
		Email        string    `json:"email"`
		Token        string    `json:"token"`
		RefreshToken string    `json:"refresh_token"`
	}
	rUser := retUser{
		ID:           dbUsr.ID,
		CreatedAt:    dbUsr.CreatedAt,
		UpdatedAt:    dbUsr.UpdatedAt,
		Email:        dbUsr.Email,
		Token:        jwt,
		RefreshToken: refreshToken,
	}
	dat, err := json.Marshal(rUser)
	if err != nil {
		helperJsonError(w, "Error marshalling response: %s", err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(dat)
}

func (cfg *apiConfig) handlerRefresh(w http.ResponseWriter, r *http.Request) {
	token, err := auth.GetBearerToken(r.Header)
	if err != nil {
		log.Printf("Error getting bearer token:\n%v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	dbToken, err := cfg.queries.GetRefreshToken(r.Context(), token)
	if err != nil {
		log.Printf("Refresh token not found:\n%v", err)
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	if dbToken.RevokedAt.Valid {
		log.Printf("Refresh token expired")
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	tokenExpiration, err := time.ParseDuration("1h")
	if err != nil {
		log.Printf("Error parsing duration:\n%v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	oneHrToken, err := auth.MakeJWT(dbToken.UserID, cfg.secret, tokenExpiration)
	if err != nil {
		log.Printf("Error creating JWT:\n%v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	type retStruct struct {
		Token string `json:"token"`
	}
	rStruct := retStruct{
		Token: oneHrToken,
	}
	dat, err := json.Marshal(rStruct)
	if err != nil {
		helperJsonError(w, "Error marshalling response: %s", err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(dat)
}

func (cfg *apiConfig) handlerRevoke(w http.ResponseWriter, r *http.Request) {
	token, err := auth.GetBearerToken(r.Header)
	if err != nil {
		log.Printf("Error getting token from header:\n%v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	dbToken, err := cfg.queries.GetRefreshToken(r.Context(), token)
	if err != nil {
		log.Printf("Refresh token not found:\n%v", err)
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	if dbToken.RevokedAt.Valid {
		log.Printf("Refresh token expired")
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	revokeParams := database.RevokeRefreshTokenParams{
		Token:     dbToken.Token,
		UpdatedAt: time.Now().Local(),
		RevokedAt: sql.NullTime{
			Time:  time.Now().Local(),
			Valid: true,
		},
	}
	err = cfg.queries.RevokeRefreshToken(r.Context(), revokeParams)
	if err != nil {
		log.Printf("Error revoking refresh token:\n%v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (cfg *apiConfig) handleChangePwd(w http.ResponseWriter, r *http.Request) {
	token, err := auth.GetBearerToken(r.Header)
	if err != nil {
		log.Printf("Error getting token from header:\n%v", err)
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	userId, err := auth.ValidateJWT(token, cfg.secret)
	if err != nil {
		log.Printf("Invalid JWT:\n%v", err)
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	type newData struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}

	decoder := json.NewDecoder(r.Body)
	data := newData{}
	if err := decoder.Decode(&data); err != nil {
		helperJsonError(w, "Error decoding user:\n%v", err)
		return
	}

	hash, err := auth.HashPassword(data.Password)
	if err != nil {
		log.Printf("Failed to hash password:\n%v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	update := database.UpdateUsrEmailPwdParams{
		ID:             userId,
		UpdatedAt:      time.Now().Local(),
		Email:          data.Email,
		HashedPassword: hash,
	}
	user, err := cfg.queries.UpdateUsrEmailPwd(r.Context(), update)
	if err != nil {
		log.Printf("Failed to update user information:\n%v", err)
		w.WriteHeader(http.StatusInternalServerError)
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
	w.WriteHeader(http.StatusOK)
	w.Write(dat)
}

func (cfg *apiConfig) handlerDeleteChirp(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("chirpID")
	chirp_uuid, err := uuid.Parse(id)
	if err != nil {
		log.Printf("Error parsing chirpID:\n%v", err)
		w.WriteHeader(http.StatusNotFound)
		return
	}

	token, err := auth.GetBearerToken(r.Header)
	if err != nil {
		log.Printf("Error getting token from header:\n%v", err)
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	userId, err := auth.ValidateJWT(token, cfg.secret)
	if err != nil {
		log.Printf("Invalid JWT:\n%v", err)
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	chirp, err := cfg.queries.GetChirpById(r.Context(), chirp_uuid)
	if err != nil {
		log.Printf("Error fetching chirp:\n%v", err)
		w.WriteHeader(http.StatusNotFound)
		return
	}

	if chirp.UserID != userId {
		log.Printf("User ids do not match - failed to delete")
		w.WriteHeader(http.StatusForbidden)
		return
	}

	err = cfg.queries.DeleteChirpById(r.Context(), chirp_uuid)
	if err != nil {
		log.Printf("Error deleting chirp:\n%v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
