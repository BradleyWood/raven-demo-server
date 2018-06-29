package main

import (
	"log"
	"time"
	"net/http"
	"github.com/gorilla/mux"
	"github.com/gorilla/sessions"
	"github.com/gorilla/securecookie"
)

var (
	store = sessions.NewCookieStore(securecookie.GenerateRandomKey(16))
)

func main() {
	r := mux.NewRouter()
	r.HandleFunc("/exec", IaHandler).Methods("POST")
	r.HandleFunc("/reset", ResetIaHandler).Methods("POST")
	r.HandleFunc("/program", ExecProgramHandler).Methods("POST")

	srv := &http.Server{
		Handler:      r,
		Addr:         "127.0.0.1:8000",
		WriteTimeout: 15 * time.Second,
		ReadTimeout:  15 * time.Second,
	}

	log.Fatal(srv.ListenAndServe())
}

// Reset the interactive interpreter
func ResetIaHandler(writer http.ResponseWriter, request *http.Request) {
	session, _ := store.Get(request, "cookie-name")

	session.Save(request, writer)
}

// Exec a line of code for the interactive interpreter and return the result
func IaHandler(writer http.ResponseWriter, request *http.Request) {
	session, _ := store.Get(request, "cookie-name")

	session.Save(request, writer)
}

// Execute a demo program
func ExecProgramHandler(writer http.ResponseWriter, request *http.Request) {
	session, _ := store.Get(request, "cookie-name")

	session.Save(request, writer)
}
