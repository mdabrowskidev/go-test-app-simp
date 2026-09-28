package main

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/gorilla/mux"
	_ "github.com/lib/pq"
)

func main() {
	//create router
	router := mux.NewRouter()
	router.HandleFunc("/hello", getHelloWorld()).Methods("GET")
	router.HandleFunc("/test-page", getTestPage()).Methods("GET")

	//start server
	log.Fatal(http.ListenAndServe(":8080", jsonContentTypeMiddleware(router)))
}

func getHelloWorld() func(http.ResponseWriter, *http.Request) {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode("Hello World")
	})
}

func getTestPage() func(http.ResponseWriter, *http.Request) {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode("Test Page")
	})
}

func jsonContentTypeMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		next.ServeHTTP(w, r)
	})
}
