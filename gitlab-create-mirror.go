package main

import (
	"encoding/json"
	"bytes"
	"flag"
	"log"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"io"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"
	"github.com/dustin/httputil"
)

var (
	address          = flag.String("gitlab-url", getEnvOrDefault("GITLAB_URL", ""), "GitLab URL [GITLAB_URL]")
	api_path         = flag.String("gitlab-api-path", "/api/v3", "GitLab API path")
	group            = flag.String("gitlab-group", getEnvOrDefault("GITLAB_GROUP", "Mirrors"), "GitLab Group [GITLAB_GROUP]")
	private_token    = flag.String("gitlab-private-token", getEnvOrDefault("GITLAB_PRIVATE_TOKEN", ""), "GitLab Mirror Private Token [GITLAB_PRIVATE_TOKEN[")
	visibility_level = flag.String("gitlab-visibility-level", getEnvOrDefault("GITLAB_VISIBILITIY_LEVEL", "private"), "Select private, internal or public [GITLAB_VISIBILITIY_LEVEL]")
	git              = flag.String("git", "/usr/bin/git", "path to git")
	origin_remote    = flag.String("origin-remote", "origin", "Source remote name")
	gitlab_remote    = flag.String("gitlab-remote", "gitlab", "Git remote name")
)

const (
	groups_url   = "/groups"
	projects_url = "/projects"
)

type Group struct {
	Id                   int      `json:"id,omitempty"`
	Name                 string   `json:"name,omitempty"`
	Path                 string   `json:"path,omitempty"`
	OwnerId              int      `json:"owner_id,omitempty"`
}

type CreateProject struct {
	Name                 string     `json:"name,omitempty"`
	Description          string     `json:"description,omitempty"`
	Path                 string     `json:"path,omitempty"`
	IssuesEnabled        bool       `json:"issues_enabled"`
	MergeRequestsEnabled bool       `json:"merge_requests_enabled"`
	WikiEnabled          bool       `json:"wiki_enabled"`
	SnippetsEnabled      bool       `json:"snippets_enabled"`
	NamespaceId          int        `json:"namespace_id,omitempty"`
	VisibilityLevel      int        `json:"visibility_level"`
}

type Namespace struct {
	Id                   int        `json:"id,omitempty"`
	Name                 string     `json:"name,omitempty"`
	Description          string     `json:"description,omitempty"`
	Path                 string     `json:"path,omitempty"`
}

type Project struct {
	Id                   int        `json:"id,omitempty"`
	Name                 string     `json:"name,omitempty"`
	Description          string     `json:"description,omitempty"`
	Public               bool       `json:"public,omitempty"`
	Path                 string     `json:"path,omitempty"`
	SshRepoUrl           string     `json:"ssh_url_to_repo"`
	HttpRepoUrl          string     `json:"http_url_to_repo"`
	Namespace             *Namespace `json:"namespace"`
}

func getEnvOrDefault(env string, defaultValue string) string {
	value := os.Getenv(env)
	if value == "" {
		value = defaultValue
	}
	return value
}

func readPayload(r io.Reader) ([]byte, error) {
	maxPayloadSize := int64(1 << 63 - 1)
	maxPayloadSize = int64(10 << 20) // 10 MB is a lot of text.
	b, err := ioutil.ReadAll(io.LimitReader(r, maxPayloadSize+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > maxPayloadSize {
		err = errors.New("http: POST too large")
		return nil, err
	}
	return b, nil
}

func sendJsonRequest(name string, st int, req *http.Request, jd interface{}) {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("PRIVATE-TOKEN", *private_token)

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Fatalf("Couldn't execute %v against %s: %v", req.Method, req.URL, err)
		return
	}
	defer res.Body.Close()
	if res.StatusCode == st {
		if jd != nil {
			d := json.NewDecoder(res.Body)
			err = d.Decode(jd)
			if err != nil {
				log.Fatalf("Error decoding json payload %v", err)
			}
		}
		return
	}

	log.Fatalf("Couldn't execute %v against %s: %v", req.Method, req.URL, httputil.HTTPError(res))
}

func getURL(path string) string {
	return fmt.Sprintf("%v/%v/%v", *address, *api_path, path)
}

func groups() ([]*Group) {
	url := getURL(groups_url)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		log.Fatalf("Failed to create NewRequest", err)
	}

	var groups []*Group
	sendJsonRequest("get groups", 200, req, &groups)
	return groups
}

func findGroup(name string) *Group {
	groups := groups()

	for _, group := range groups {
		if group.Name == name {
			return group
		}
	}
	return nil
}

func projects() ([]*Project) {
	url := getURL(projects_url)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		log.Fatalf("Failed to create NewRequest", err)
	}

	var projects []*Project
	sendJsonRequest("get groups", 200, req, &projects)
	return projects
}

func findProject(namespace string, project_name string) *Project {
	projects := projects()

	for _, project := range projects {
		if project.Namespace.Name == namespace && project.Name == project_name {
			return project
		}
	}
	return nil
}

func createProject(new_project CreateProject) Project {
	body, err := json.Marshal(&new_project)
	if err != nil {
		log.Fatalf("Failed to marshal project object: %v", err)
	}

	url := getURL(projects_url)
	req, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		log.Fatalf("Failed to create NewRequest", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.ContentLength = int64(len(body))

	var created_project Project
	sendJsonRequest("create project", 201, req, &created_project)
	return created_project
}

func readOriginRemote() url.URL {
	out, err := exec.Command(*git, "config", fmt.Sprintf("remote.%v.url", *origin_remote)).Output()
	if err != nil {
		log.Fatalf("No URL defined for %v.", *origin_remote)
	}

	rawurl := strings.TrimSpace(string(out))
	if !strings.Contains(rawurl, "://") {
		rawurl = strings.Replace(rawurl, ":", "/", 1)
		rawurl = "ssh://" + rawurl
	}

	result, err := url.Parse(rawurl)
	if err != nil {
		log.Fatalf("Invalid URL for %v: %s", *origin_remote, out)
	}

	result.User = nil
	result.RawQuery = ""
	result.Fragment = ""
	return *result
}

func doCreate(repo_name string, repo_url string) *Project {
	log.Printf("Looking for group %v...", *group)
	group_data := findGroup(*group)
	if group_data == nil {
		log.Fatalf("No group %v found.", *group)
	}

	log.Printf("Creating project %v in %v...", repo_name, *group)
	new_project := CreateProject {}
	new_project.Name = repo_name
	new_project.Description = fmt.Sprintf("Mirror of %v", repo_url)
	new_project.IssuesEnabled = false
	new_project.MergeRequestsEnabled = false
	new_project.WikiEnabled = false
	new_project.SnippetsEnabled = false
	new_project.NamespaceId = group_data.Id
	switch *visibility_level {
	case "private":
		new_project.VisibilityLevel = 0
	case "internal":
		new_project.VisibilityLevel = 10
	case "public":
		new_project.VisibilityLevel = 20
	default:
		log.Fatalf("Unsupported visibility_level: %v", *visibility_level)
	}
	created_project := createProject(new_project)
	return &created_project
}

func doCheckRemote() bool {
	log.Printf("Verifying existence of git remote %v...", *gitlab_remote)
	err := exec.Command(*git, "config", fmt.Sprintf("remote.%v.url", *gitlab_remote)).Run()
	if err != nil {
		return false
	}
	return true
}

func doCreateRemote() {
	repo_url := readOriginRemote()
	repo_name := repo_url.Path
	repo_name = strings.TrimPrefix(repo_name, "/")
	repo_name = strings.TrimSuffix(repo_name, ".git")
	repo_name = strings.Replace(repo_name, "/", "-", -1)

	log.Printf("Looking for project %v in %v...", repo_name, *group)
	project_data := findProject(*group, repo_name)
	if project_data == nil {
		project_data = doCreate(repo_name, repo_url.String())
	}

	log.Printf("Adding remote %v as %v...", project_data.SshRepoUrl, *gitlab_remote)
	err := exec.Command(*git, "remote", "add", "--mirror=push", *gitlab_remote, project_data.SshRepoUrl).Run()
	if err != nil {
		log.Fatalf("Failed to add git remote %v to %v", *gitlab_remote, project_data.SshRepoUrl)
	}

	log.Printf("We are waiting 3 seconds to settle down...")
	time.Sleep(3000 * time.Millisecond)
}

func doPush() {
	log.Printf("Pushing changes to %v...", *gitlab_remote)
	cmd := exec.Command(*git, "push", *gitlab_remote)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	if err != nil {
		log.Fatalf("Failed to push data to %v", *gitlab_remote)
	}
}

func main() {
	flag.Parse()

	if *address == "" {
		log.Fatalf("Address is required!")
	}
	if *private_token == "" {
		log.Fatalf("Private token is required!")
	}

	log.SetFlags(0)

	if !doCheckRemote() {
		doCreateRemote()
	}

	doPush()
}
