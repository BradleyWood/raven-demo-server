package main

import (
	"io"
	"log"
	"time"
	"os/exec"
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

	store.Options = &sessions.Options{
		MaxAge:   60 * 60,
		HttpOnly: true,
	}

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

// initialize the interactive interpreter
func initUser() (User, error) {
	cmd := exec.Command("java", "-jar", "raven.jar")
	in, _ := cmd.StdoutPipe()
	out, _ := cmd.StdinPipe()
	err := cmd.Start()

	if err != nil {
		return User{}, err
	}

	return User{process: cmd, out: out, in: in}, nil
}

type User struct {
	process *exec.Cmd
	out     io.WriteCloser
	in      io.ReadCloser
}

type Line struct {
	Line string `json:"line"`
}

type Result struct {
	Status int    `json:"status"`
	Result string `json:"result"`
}

type Program struct {
	Src   string   `json:"src"`
	Args  []string `json:"args"`
	Input string   `json:"stdin"`
}
