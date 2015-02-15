package main

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha1"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"hash"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

var (
	thePath = flag.String("dir", "/tmp", "working directory")
	git     = flag.String("git", "/usr/bin/git", "path to git")
	addr    = flag.String("addr", ":8124", "binding address to listen on")
	secret  = flag.String("secret", "",
	"Optional secret for authenticating hooks")
)

type commandRequest struct {
	w       http.ResponseWriter
	abspath string
	bg      bool
	after   time.Time
	cmds    []*exec.Cmd
	ch      chan bool
}

var reqch = make(chan commandRequest, 100)
var updates = map[string]time.Time{}

func exists(path string) bool {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return false
	}
	return true
}

func maybePanic(err error) {
	if err != nil {
		panic(err)
	}
}

func runCommands(w http.ResponseWriter, bg bool,
	abspath string, cmds []*exec.Cmd) {

	stderr := ioutil.Discard
	stdout := ioutil.Discard

	if !bg {
		stderr = &bytes.Buffer{}
		stdout = &bytes.Buffer{}
	}

	for _, cmd := range cmds {
		if exists(cmd.Path) {
			if cmd.Dir == "" {
				cmd.Dir = abspath
			}
			cmd.Stdout = stdout
			cmd.Stderr = stderr

			log.Printf("Running %v in %v", cmd.Args, cmd.Dir)
			fmt.Fprintf(stdout, "# Running %v\n", cmd.Args)
			fmt.Fprintf(stderr, "# Running %v\n", cmd.Args)

			err := cmd.Run()

			if err != nil {
				log.Printf("Error running %v in %v:  %v",
					cmd.Args, abspath, err)
				if !bg {
					fmt.Fprintf(stderr,
						"\n[gitmirror internal error:  %v]\n", err)
				}
			}

		}
	}

	if !bg {
		fmt.Fprintf(w, "---- stdout ----\n")
		_, err := stdout.(*bytes.Buffer).WriteTo(w)
		maybePanic(err)
		fmt.Fprintf(w, "\n----\n\n\n---- stderr ----\n")
		_, err = stderr.(*bytes.Buffer).WriteTo(w)
		maybePanic(err)
		fmt.Fprintf(w, "\n----\n")
	}
}

func shouldRun(path string, after time.Time) bool {
	if path == "/tmp" {
		return true
	}
	lastRun := updates[path]
	return lastRun.Before(after)
}

func didRun(path string, t time.Time) {
	updates[path] = t
}

func pathRunner(ch chan commandRequest) {
	for r := range ch {
		if shouldRun(r.abspath, r.after) {
			t := time.Now()
			runCommands(r.w, r.bg, r.abspath, r.cmds)
			didRun(r.abspath, t)
		} else {
			log.Printf("Skipping redundant update: %v", r.abspath)
			if !r.bg {
				fmt.Fprintf(r.w, "Redundant request.")
			}
		}
		select {
		case r.ch <- true:
		default:
		}
	}
}

func commandRunner() {
	m := map[string]chan commandRequest{}

	for r := range reqch {
		ch, running := m[r.abspath]
		if !running {
			ch = make(chan commandRequest, 10)
			m[r.abspath] = ch
			go pathRunner(ch)
		}
		ch <- r
	}
}

func queueCommand(w http.ResponseWriter, bg bool,
	abspath string, cmds []*exec.Cmd) chan bool {
	req := commandRequest{w, abspath, bg, time.Now(),
		cmds, make(chan bool)}
	reqch <- req
	return req.ch
}

func updateGit(w http.ResponseWriter, section string,
	bg bool, payload []byte) bool {

	abspath := filepath.Join(*thePath, section)

	if !exists(abspath) {
		if !bg {
			http.Error(w, "Not found", http.StatusNotFound)
		}
		return false
	}

	cmds := []*exec.Cmd{
		exec.Command(*git, "remote", "update", "-p"),
		exec.Command(*git, "gc", "--auto"),
		exec.Command(filepath.Join(abspath, "hooks/post-fetch")),
		exec.Command(filepath.Join(*thePath, "bin/post-fetch")),
	}

	cmds[2].Stdin = bytes.NewBuffer(payload)
	cmds[3].Stdin = bytes.NewBuffer(payload)

	return <-queueCommand(w, bg, abspath, cmds)
}

func getPath(req *http.Request) string {
	//	if qp := req.URL.Query().Get("name"); qp != "" {
	//		return filepath.Clean(qp)
	//	}
	return filepath.Clean(filepath.FromSlash(req.URL.Path))[1:]
}

func createRepo(w http.ResponseWriter, path string, repo_path string,
	private bool, bg bool, payload []byte) bool {
	repo := fmt.Sprintf("git://github.com/%v.git",
		repo_path)
	if private {
		repo = fmt.Sprintf("git@github.com:%v.git",
			repo_path)
	}

	abspath := filepath.Join(*thePath, path)

	cmds := []*exec.Cmd{
		exec.Command(*git, "clone", "--mirror", "--bare", repo,
			filepath.Join(*thePath, path)),
		exec.Command(filepath.Join(abspath, "hooks/post-clone")),
		exec.Command(filepath.Join(*thePath, "bin/post-clone")),
		exec.Command(filepath.Join(abspath, "hooks/post-fetch")),
		exec.Command(filepath.Join(*thePath, "bin/post-fetch")),
	}

	for i:=1; i<len(cmds); i++ {
		cmds[i].Stdin = bytes.NewBuffer(payload)
		cmds[i].Dir = abspath
	}

	return <-queueCommand(w, bg, "/tmp", cmds)
}

func doUpdate(w http.ResponseWriter, path string,
	bg bool, payload []byte) {
	if bg {
		go updateGit(w, path, bg, payload)
		w.WriteHeader(201)
	} else {
		updateGit(w, path, bg, payload)
	}
}

func doCreate(w http.ResponseWriter, path string, repo_path string,
	private bool, bg bool, payload []byte) {
	if bg {
		go createRepo(w, path, repo_path, private, bg, payload)
		w.WriteHeader(201)
	} else {
		createRepo(w, path, repo_path, private, bg, payload)
	}
}

func handleGet(w http.ResponseWriter, req *http.Request, bg bool) {
	doUpdate(w, getPath(req), bg, nil)
}

// parseForm parses an HTTP POST form from an io.Reader.
func readPayload(r io.Reader) ([]byte, error) {
	maxFormSize := int64(1 << 63 - 1)
	maxFormSize = int64(10 << 20) // 10 MB is a lot of text.
	b, err := ioutil.ReadAll(io.LimitReader(r, maxFormSize+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > maxFormSize {
		err = errors.New("http: POST too large")
		return nil, err
	}
	return b, nil
}

func checkHMAC(h hash.Hash, sig string) bool {
	got := fmt.Sprintf("sha1=%x", h.Sum(nil))
	return len(got) == len(sig) && subtle.ConstantTimeCompare(
		[]byte(got), []byte(sig)) == 1
}

func handleGitHubCallback(w http.ResponseWriter, req *http.Request, bg bool) {
	// We're teeing the form parsing into a sha1 HMAC so we can
	// authenticate what we actually parsed (if we *secret is set,
	// anyway)
	mac := hmac.New(sha1.New, []byte(*secret))
	r := io.TeeReader(req.Body, mac)
	payload, err := readPayload(r)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	if !(*secret == "" || checkHMAC(mac, req.Header.Get("X-Hub-Signature"))) {
		http.Error(w, "not authorized", http.StatusUnauthorized)
		return
	}

	p := struct {
			Repository struct {
				Private  bool
				Name     string
				FullName string `json:"full_name"`
			}
		}{}

	err = json.Unmarshal(payload, &p)
	if err != nil {
		log.Printf("Error unmarshalling data: %v", err)
		http.Error(w, "Error parsing JSON", http.StatusInternalServerError)
		return
	}

	path := p.Repository.FullName
	repo_path := p.Repository.FullName
	private := p.Repository.Private

	if exists(filepath.Join(*thePath, path)) {
		doUpdate(w, path, bg, payload)
	} else {
		doCreate(w, path, repo_path, private, bg, payload)
	}
}

func handleReq(w http.ResponseWriter, req *http.Request) {
	backgrounded := req.URL.Query().Get("bg") == "true"

	log.Printf("Handling %v %v", req.Method, req.URL.Path)

	switch req.Method {
	case "GET":
		handleGet(w, req, backgrounded)
	case "POST":
		switch req.URL.Path {
		case "/callback/github":
			handleGitHubCallback(w, req, backgrounded)
		default:
			http.Error(w, "Path not found",
				http.StatusNotFound)
		}
	default:
		http.Error(w, "Method not allowed",
			http.StatusMethodNotAllowed)
	}
}

func main() {
	flag.Parse()

	log.SetFlags(log.Lmicroseconds)

	go commandRunner()

	http.HandleFunc("/", handleReq)
	http.HandleFunc("/favicon.ico",
		func(w http.ResponseWriter, req *http.Request) {
			http.Error(w, "No favicon", http.StatusGone)
		})

	log.Fatal(http.ListenAndServe(*addr, nil))
}
