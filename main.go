package main

import (
	"io"
	"os"
	"log"
	"path"
	"time"
	"regexp"
	"errors"
	"strings"
	"os/exec"
	"net/http"
	"io/ioutil"
	"encoding/json"
	"github.com/gorilla/mux"
	"github.com/gorilla/sessions"
	"github.com/tink-ab/tempfile"
	"github.com/svent/go-nbreader"
	"github.com/gorilla/securecookie"
)

var (
	counter  = 0
	users    = make(map[int]User)
	store    = sessions.NewCookieStore(securecookie.GenerateRandomKey(16))
	argRegex = regexp.MustCompile(`[^\s"']+|"([^"]*)"|'([^']*)`)
)

const (
	StatusOk          = 0
	StatusTimeout     = 1
	StatusBufOverflow = 2
	host              = "http://bradleywood.me"
	timeout           = time.Second * 5
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

	tmpDir := path.Join(os.TempDir(), "raven")
	os.Mkdir(tmpDir, os.ModePerm)

	srv := &http.Server{
		Handler:      r,
		Addr:         "127.0.0.1:3000",
		WriteTimeout: 15 * time.Second,
		ReadTimeout:  15 * time.Second,
	}

	log.Fatal(srv.ListenAndServe())

	for k, v := range users {
		if time.Since(v.start).Hours() >= 1 {
			v.process.Process.Kill()
			delete(users, k)
		}
	}

	os.RemoveAll(tmpDir)
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
	session, _ := store.Get(request, "cookie-name")
	if session.Values["userId"] == nil {
		id := session.Values["userId"].(int)
		users[id].process.Process.Kill()
		delete(users, id)
		session.Values["userId"] = nil
		session.Save(request, writer)
	}
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

	fp, err := tempfile.TempFile(path.Join(os.TempDir(), "raven"), "raven_demo_", ".rvn")

	if err != nil {
		panic(err)
		http.Error(writer, err.Error(), http.StatusInternalServerError)
	} else {
		body, err := ioutil.ReadAll(request.Body)

		if err == nil {
			program := Program{}
			err := json.Unmarshal(body, &program)
			if err != nil {
				panic(err)
				http.Error(writer, err.Error(), http.StatusBadRequest)
			} else {
				fp.Write([]byte(program.Src))
				fp.Close()

				result := execProgram(fp.Name(), program)

				output, err := json.Marshal(result)

				if err != nil {
					http.Error(writer, err.Error(), http.StatusInternalServerError)
				} else {
					writer.Write(output)
				}
			}
		}
	}
	os.Remove(fp.Name())
}

// report output from the terminal
func TerminalUpdate(writer http.ResponseWriter, request *http.Request) {
	setHeaders(writer)
	_, user, err := getUser(writer, request)

	if err == nil {
		output := readString(user.in)
		errorOutput := readString(user.err)

		response := Result{Stdout: output, Stderr: errorOutput}
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

func execProgram(path string, program Program) Result {
	cmd := exec.Command("java", "-jar", "raven.jar", path)

	args := argRegex.FindAllString(program.Args, -1)

	for i := range args {
		cmd.Args = append(cmd.Args, args[i])
	}

	status := StatusOk
	bufSize := 4096

	errPipe, _ := cmd.StderrPipe()
	in, _ := cmd.StdoutPipe()
	out, _ := cmd.StdinPipe()

	cmd.Start()

	buf := make([]byte, bufSize)
	bLen := 0

	errBuf := make([]byte, bufSize)
	errLen := 0

	ch := make(chan error)

	reader := func(reader io.Reader, buf []byte, len *int) {
		var err error = nil
		n := 0

		for {
			n, err = reader.Read(buf[*len:])
			*len += n
			if err == io.EOF {
				break
			}
		}
		ch <- err
	}

	go out.Write([]byte(program.Input))
	go reader(in, buf, &bLen)
	go reader(errPipe, errBuf, &errLen)

	select {
	case <-ch:
		break
	case <-time.After(timeout):
		status = StatusTimeout
		cmd.Process.Kill()
	}

	if bLen >= bufSize || errLen >= bufSize {
		status = StatusBufOverflow
	}

	return Result{Status: status, Stdout: string(buf[:bLen]), Stderr: string(errBuf[:errLen])}
}

// initialize the interactive interpreter
func initUser() (User, error) {
	cmd := exec.Command("java", "-jar", "raven.jar")
	in, _ := cmd.StdoutPipe()
	out, _ := cmd.StdinPipe()
	errPipe, _ := cmd.StderrPipe()
	err := cmd.Start()

	if err != nil {
		return User{}, err
	}

	nbReader := nbreader.NewNBReader(in, 2048, nbreader.Timeout(time.Millisecond*250))
	nbErrorReader := nbreader.NewNBReader(errPipe, 2048, nbreader.Timeout(time.Millisecond*50))

	return User{process: cmd, start: time.Now(), out: out, in: nbReader, err: nbErrorReader}, nil
}

type User struct {
	process *exec.Cmd
	start   time.Time
	out     io.Writer
	in      io.Reader
	err     io.Reader
}

type Line struct {
	Line string `json:"line"`
}

type Result struct {
	Status int    `json:"status"`
	Stdout string `json:"stdout"`
	Stderr string `json:"stderr"`
}

type Program struct {
	Src   string `json:"src"`
	Args  string `json:"args"`
	Input string `json:"stdin"`
}
