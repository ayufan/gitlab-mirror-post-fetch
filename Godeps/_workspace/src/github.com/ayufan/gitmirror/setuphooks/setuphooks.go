package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"
	"text/template"
	"time"

	"github.com/dustin/go-jsonpointer"
	"github.com/dustin/httputil"
)

const base = "https://api.github.com"

var (
	username = flag.String("user", "", "Your github username")
	password = flag.String("pass", "", "Your github password")
	org      = flag.String("org", "", "Organization to check")
	noop     = flag.Bool("n", false, "If true, don't make any hook changes")
	test     = flag.Bool("t", false, "Test hooks when creating them")
	testAll  = flag.Bool("T", false, "Test all hooks")
	del      = flag.Bool("d", false, "Delete, instead of adding a hook.")
	events   = flag.String("events", "push", "Comma separated list of events")
	repoFlag = flag.String("repo", "", "Specific repo (default: all)")
	verbose  = flag.Bool("v", false, "Print more stuff")
	secret   = flag.String("secret", "",
		"Optional secret to authenticate inbound hooks")

	tmpl *template.Template
)

type hook struct {
	ID     int                    `json:"id,omitempty"`
	URL    string                 `json:"url,omitempty"`
	Name   string                 `json:"name"`
	Events []string               `json:"events,omitempty"`
	Active bool                   `json:"active"`
	Config map[string]interface{} `json:"config"`
}

func init() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage:\n%s [opts] template\n\nOptions:\n",
			os.Args[0])
		flag.PrintDefaults()
		tdoc := map[string]string{
			"{{.ID}}":          "numeric ID of repo",
			"{{.Owner.Login}}": "github username of repo owner",
			"{{.Owner.ID}}":    "github numeric id of repo owner",
			"{{.Name}}":        "short name of repo (e.g. gitmirror)",
			"{{.FullName}}":    "full name of repo (e.g. dustin/gitmirror)",
			"{{.Language}}":    "repository language (if detected)",
		}

		a := sort.StringSlice{}
		for k := range tdoc {
			a = append(a, k)
		}
		a.Sort()

		fmt.Fprintf(os.Stderr, "\nTemplate parameters:\n")
		tw := tabwriter.NewWriter(os.Stderr, 8, 4, 2, ' ', 0)
		for _, k := range a {
			fmt.Fprintf(tw, "  %v\t- %v\n", k, tdoc[k])
		}
		tw.Flush()
		fmt.Fprintf(os.Stderr, "\nExample templates:\n"+
			"  http://example.com/gitmirror/{{.FullName}}.git\n"+
			"  http://example.com/gitmirror/{{.Owner.Login}}/{{.Language}}/{{.Name}}.git\n"+
			"  http://example.com/gitmirror/{{.Name}}.git\n")
	}
}

func retryableHTTP(name string, st int, req *http.Request, jd interface{}) {
	var err error
	for i := 0; i < 3; i++ {
		if i > 0 {
			log.Printf("Retrying %v to %v", req.Method, req.URL)
			time.Sleep(time.Second * time.Duration(i))
		}

		var res *http.Response
		res, err = http.DefaultClient.Do(req)
		if err != nil {
			continue
		}
		defer res.Body.Close()
		if res.StatusCode == st {
			if jd != nil {
				d := json.NewDecoder(res.Body)
				err = d.Decode(jd)
				if err != nil {
					continue
				}
			}
			return
		}
		err = httputil.HTTPError(res)
	}
	log.Fatalf("Couldn't do %v against %s: %v", req.Method, req.URL, err)
}

func (h hook) Test(r repo) {
	log.Printf("Testing %v -> %v", r.FullName,
		jsonpointer.Get(h.Config, "/url"))
	u := base + "/repos/" + r.FullName + "/hooks/" +
		strconv.Itoa(h.ID) + "/test"

	req, err := http.NewRequest("POST", u, nil)
	maybeFatal("hook test", err)

	req.SetBasicAuth(*username, *password)
	retryableHTTP("hook test", 204, req, nil)
}

type repo struct {
	ID    int
	Owner struct {
		Login string
		ID    int
	}
	Name     string
	FullName string `json:"full_name"`
	Language *string
}

func maybeFatal(m string, err error) {
	if err != nil {
		log.Fatalf("%s: %v", m, err)
	}
}

func parseLink(s string) map[string]string {
	rv := map[string]string{}
	if s == "" {
		return rv
	}
	for _, link := range strings.Split(s, ", ") {
		parts := strings.Split(link, "; ")
		u := parts[0][1 : len(parts[0])-1]

		if !strings.HasPrefix(parts[1], `rel="`) {
			panic("Unexpected: " + link)
		}

		rv[parts[1][5:len(parts[1])-1]] = u
	}
	return rv
}

// Parses json stuff into a thing.  Returns the next URL if any
func getJSON(name, subu string, out interface{}) string {
	u := subu
	if !strings.HasPrefix(u, "http") {
		u = base + subu
	}

	req, err := http.NewRequest("GET", u, nil)
	maybeFatal(name, err)

	req.SetBasicAuth(*username, *password)
	for i := 0; i < 3; i++ {
		if i > 0 {
			log.Printf("Retrying JSON req to %v", req.URL)
		}

		var res *http.Response
		res, err = http.DefaultClient.Do(req)
		if err != nil {
			continue
		}
		defer res.Body.Close()
		if res.StatusCode != 200 {
			err = httputil.HTTPError(res)
			continue
		}

		links := parseLink(res.Header.Get("Link"))

		d := json.NewDecoder(res.Body)

		maybeFatal(name, d.Decode(out))

		return links["next"]
	}
	log.Fatalf("Error getting JSON from %v: %v", u, err)
	panic("unreachable")
}

func listRepos() chan repo {
	rv := make(chan repo)

	go func() {
		defer close(rv)
		next := "/user/repos?type=owner"
		if *org != "" {
			next = "/orgs/" + *org + "/repos"
		}

		for next != "" {
			repos := []repo{}
			log.Printf("Fetching repos from %v", next)
			next = getJSON("repo list", next, &repos)

			for _, r := range repos {
				rv <- r
			}
		}
	}()
	return rv
}

func mirrorFor(r repo) string {
	b := bytes.Buffer{}
	maybeFatal("executing template", tmpl.Execute(&b, r))
	return b.String()
}

func contains(haystack []string, needle string) bool {
	for _, n := range haystack {
		if n == needle {
			return true
		}
	}
	return false
}

func containsAll(haystack, needles []string) bool {
	for _, n := range needles {
		if !contains(haystack, n) {
			return false
		}
	}
	return true
}

func mirrorID(r repo, hooks []hook) int {
	u := mirrorFor(r)
	for _, h := range hooks {
		if h.Name == "web" && jsonpointer.Get(h.Config, "/url") == u &&
			(*events == "" ||
				containsAll(h.Events, strings.Split(*events, ","))) {
			if *testAll {
				h.Test(r)
			}
			return h.ID
		}
	}
	return -1
}

func createHook(r repo) hook {
	h := hook{
		Name:   "web",
		Active: true,
		Events: strings.Split(*events, ","),
		Config: map[string]interface{}{"url": mirrorFor(r)},
	}
	if *secret != "" {
		h.Config["secret"] = *secret
	}
	body, err := json.Marshal(&h)
	maybeFatal("encoding", err)

	req, err := http.NewRequest("POST",
		base+"/repos/"+r.FullName+"/hooks",
		bytes.NewReader(body))
	maybeFatal("creating hook", err)

	req.SetBasicAuth(*username, *password)
	req.Header.Set("Content-Type", "application/json")
	req.ContentLength = int64(len(body))

	rv := hook{}
	retryableHTTP("create hook", 201, req, &rv)

	return rv
}

func teardown(id int, r repo) {
	req, err := http.NewRequest("DELETE",
		fmt.Sprintf("%v/repos/%v/hooks/%v",
			base, r.FullName, id),
		nil)
	maybeFatal("deleting hook", err)

	req.SetBasicAuth(*username, *password)
	retryableHTTP("create hook", 204, req, nil)
}

func setup(id int, r repo) {
	h := createHook(r)
	if *test {
		h.Test(r)
	}
}

func updateHooks(r repo) {
	hooks := []hook{}
	getJSON(r.FullName, "/repos/"+r.FullName+"/hooks", &hooks)
	actions := map[string]func(int, repo){
		"setup":    setup,
		"teardown": teardown,
	}

	if *verbose {
		fmt.Printf("Hooks for %v:\n", r.FullName)
		for _, h := range hooks {
			conf := ""
			for k, v := range h.Config {
				conf += fmt.Sprintf("\n\t%v = %v", k, v)
			}
			fmt.Printf("%v: %v active=%v -%v\n\n",
				h.Name, h.Events, h.Active, conf)
		}
	}

	action := "setup"

	id := mirrorID(r, hooks)
	switch {
	case id >= 0 && *del:
		action = "teardown"
	case id == -1 && !*del:
		action = "setup"
	default:
		return
	}

	log.Printf("Updating %v (%v)", r.FullName, action)
	if !*noop {
		actions[action](id, r)
	}
}

func getRepo(name string) repo {
	rv := repo{}
	parts := strings.Split(name, "/")
	if len(parts) == 1 {
		rv.FullName = *username + "/" + parts[0]
		rv.Name = parts[0]
		rv.Owner.Login = *username
	} else {
		rv.FullName = parts[0] + "/" + parts[1]
		rv.Name = parts[1]
		rv.Owner.Login = parts[0]
	}
	return rv
}

func main() {
	log.SetFlags(0)
	flag.Parse()

	var tmplText = ""
	if flag.NArg() > 0 {
		tmplText = flag.Arg(0)
	} else {
		log.Printf("No template given, just listing")
		*noop = true
		*verbose = true
	}

	t, err := template.New("u").Parse(tmplText)
	maybeFatal("parsing template", err)
	tmpl = t

	if *repoFlag == "" {
		for r := range listRepos() {
			updateHooks(r)
		}
	} else {
		updateHooks(getRepo(*repoFlag))
	}
}
