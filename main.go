package main

import (
	"io"
	"log"
	"time"
	"errors"
	"strings"
	"os/exec"
	"net/http"
	"io/ioutil"
	"encoding/json"
	"github.com/gorilla/mux"
	"github.com/gorilla/sessions"
	"github.com/gorilla/securecookie"
)

var (
	counter = 0
	users   = make(map[int]User)
	store   = sessions.NewCookieStore(securecookie.GenerateRandomKey(16))
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

func getUser(writer http.ResponseWriter, request *http.Request) (*sessions.Session, User, error) {
	session, _ := store.Get(request, "cookie-name")

	if session.Values["userId"] == nil {
		user, err := initUser()
		if err != nil {
			http.Error(writer, err.Error(), http.StatusInternalServerError)
		} else {
			session.Values["userId"] = counter
			users[counter] = user
			session.Save(request, writer)
			counter++
		}
	}

	if session.Values["userId"] != nil {
		user := users[session.Values["userId"].(int)]
		return session, user, nil
	}

	return session, User{}, errors.New("cannot create interactive interpreter")
}

// Reset the interactive interpreter
func ResetIaHandler(writer http.ResponseWriter, request *http.Request) {
	session, _ := store.Get(request, "cookie-name")

	session.Save(request, writer)
}

// Exec a line of code for the interactive interpreter and return the result
func IaHandler(writer http.ResponseWriter, request *http.Request) {
	_, user, err := getUser(writer, request)

	if err == nil {
		body, err := ioutil.ReadAll(request.Body)

		if err == nil {
			line := &Line{}
			parseError := json.Unmarshal(body, line)

			if parseError != nil {
				panic(parseError)
				http.Error(writer, parseError.Error(), http.StatusBadRequest)
			} else {
				user.out.Write([]byte(line.Line))
				if !strings.HasSuffix(line.Line, "\n") {
					user.out.Write([]byte("\n"))
				}
			}
		}
	}
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
