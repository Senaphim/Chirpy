package main

import (
	"fmt"
	"net/http"
)

func main() {
	serveMux := http.NewServeMux()
	serveMux.Handle("/app/", http.StripPrefix("/app", http.FileServer(http.Dir("."))))
	hh := http.HandlerFunc(handleHealth)
	serveMux.Handle("/healthz", hh)

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
