package main

import (
	"io"
	"log"
	"fmt"
	"time"
	"errors"
	"strings"
	"os/exec"
	"net/http"
	"io/ioutil"
	"encoding/json"
	"github.com/gorilla/mux"
	"github.com/gorilla/sessions"
	"github.com/svent/go-nbreader"
	"github.com/gorilla/securecookie"
)

var (
	counter = 0
	users   = make(map[int]User)
	host    = "http://bradleywood.me"
	store   = sessions.NewCookieStore(securecookie.GenerateRandomKey(16))
)

func main() {
	r := mux.NewRouter()
	r.HandleFunc("/exec", IaHandler).Methods("POST")
	r.HandleFunc("/reset", ResetIaHandler).Methods("POST")
	r.HandleFunc("/update", TerminalUpdate).Methods("POST")
	r.HandleFunc("/program", ExecProgramHandler).Methods("POST")

	r.HandleFunc("/exec", HandleOptions).Methods("OPTIONS")
	r.HandleFunc("/reset", HandleOptions).Methods("OPTIONS")
	r.HandleFunc("/update", HandleOptions).Methods("OPTIONS")
	r.HandleFunc("/program", HandleOptions).Methods("OPTIONS")

	store.Options = &sessions.Options{
		MaxAge:   60 * 60,
		HttpOnly: true,
	}

	srv := &http.Server{
		Handler:      r,
		Addr:         "127.0.0.1:3000",
		WriteTimeout: 15 * time.Second,
		ReadTimeout:  15 * time.Second,
	}

	log.Fatal(srv.ListenAndServe())
}

func HandleOptions(writer http.ResponseWriter, _ *http.Request) {
	setHeaders(writer)
	writer.Header().Set("Access-Control-Allow-Methods", "OPTIONS")
	writer.Header().Set("Access-Control-Allow-Headers", "Content-Type,Authorization")
}

func setHeaders(writer http.ResponseWriter) {
	writer.Header().Set("Access-Control-Allow-Credentials", "true")
	writer.Header().Set("Access-Control-Allow-Origin", host)
	writer.Header().Set("Content-Type", "application/json; charset=UTF-8")
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
	setHeaders(writer)

}

// Exec a line of code for the interactive interpreter and return the result
func IaHandler(writer http.ResponseWriter, request *http.Request) {
	setHeaders(writer)
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
	setHeaders(writer)

}

// report output from the terminal
func TerminalUpdate(writer http.ResponseWriter, request *http.Request) {
	setHeaders(writer)
	s, user, err := getUser(writer, request)

	if err == nil {
		output := readString(user.in)

		response := Result{Result: output}
		fmt.Println("Update", output, s.Values["userId"])
		bytes, err := json.Marshal(response)

		if err != nil {
			http.Error(writer, err.Error(), http.StatusInternalServerError)
		} else {
			writer.Write(bytes)
		}
	}
}

func readString(reader io.Reader) string {
	bytes := make([]byte, 2048)
	n, _ := reader.Read(bytes)

	return string(bytes[:n])
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

	nbReader := nbreader.NewNBReader(in, 2048, nbreader.ChunkTimeout(time.Millisecond*250))

	return User{process: cmd, out: out, in: nbReader}, nil
}

type User struct {
	process *exec.Cmd
	out     io.Writer
	in      io.Reader
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
