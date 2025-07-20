package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync/atomic"
)

type apiConfig struct {
	fileserverHits atomic.Int32
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
	cfg.fileserverHits.Store(0)
	w.Header().Add("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(http.StatusText(http.StatusOK)))
}

func main() {
	cfg := &apiConfig{}
	serveMux := http.NewServeMux()

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

	// Start server
	server := http.Server{
		Addr:    ":8080",
		Handler: serveMux,
	}
	err := server.ListenAndServe()
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
		Valid bool `json:"valid"`
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

	resp := returnVals{
		Valid: true,
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
