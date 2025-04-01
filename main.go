package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"

	_ "unsafe"

	"github.com/GoogleContainerTools/skaffold/v2/cmd/skaffold/app"
	"github.com/GoogleContainerTools/skaffold/v2/pkg/skaffold/docker"
	"github.com/GoogleContainerTools/skaffold/v2/pkg/skaffold/schema/latest"
	"github.com/manifoldco/promptui"
	"github.com/rjeczalik/notify"
)

//go:linkname hijacked_getDependenciesFunc github.com/GoogleContainerTools/skaffold/v2/pkg/skaffold/graph.getDependenciesFunc
var hijacked_getDependenciesFunc func(ctx context.Context, a *latest.Artifact, cfg docker.Config, r docker.ArtifactResolver, tag string) ([]string, error)
var originalGetDependenciesFunc func(ctx context.Context, a *latest.Artifact, cfg docker.Config, r docker.ArtifactResolver, tag string) ([]string, error)

func overloaded_sourceDependenciesForArtifact(ctx context.Context, a *latest.Artifact, cfg docker.Config, r docker.ArtifactResolver, tag string) ([]string, error) {
	return ar.GetDependencies(ctx, a, cfg, tag)
}

//go:linkname hijacked_Watch github.com/GoogleContainerTools/skaffold/v2/pkg/skaffold/trigger/fsnotify.Watch
var hijacked_Watch func(path string, c chan<- notify.EventInfo, events ...notify.Event) error
var originalWatch func(path string, c chan<- notify.EventInfo, events ...notify.Event) error

func overloaded_Watch(path string, c chan<- notify.EventInfo, events ...notify.Event) error {
	fmt.Fprintln(os.Stderr, "Watch called for path:", path, "with events:", events)
	return ar.AddWatch(path, c, events...)
}

var ar *ArtifactResolver

var (
	flagAddr = flag.String("skaf0-addr", "127.0.0.1:57455", "address to listen on")
)

func ctrl(args []string) error {
	if len(args) == 0 {
		fmt.Println("Usage: skaf0 ctrl <command>")
		fmt.Println("Available commands:")
		fmt.Println("  list     - List all available artifacts")
		fmt.Println("  rebuild  - Rebuild specific artifacts. Usage: rebuild <pattern1> [<pattern2> ...]")
		fmt.Println("             Patterns can be artifact names or wildcards like 'frontend-*' or '*'")
		return fmt.Errorf("Usage: skaf0 ctrl <command>")
	}

	command := args[0]
	addr := *flagAddr
	if addr == "" {
		addr = "127.0.0.1:57455"
	}
	baseURL := fmt.Sprintf("http://%s", addr)

	switch command {
	case "list":
		artifacts, err := fetchArtifacts(baseURL)
		if err != nil {
			return fmt.Errorf("error listing artifacts: %w", err)
		}
		for _, artifact := range artifacts {
			fmt.Println(artifact)
		}
		return nil
	case "rebuild":
		return rebuildArtifacts(args[1:], baseURL)
	default:
		return fmt.Errorf("unknown command: %s", command)
	}
}

func selectArtifact(baseURL string) (string, error) {
	artifacts, err := fetchArtifacts(baseURL)
	if err != nil {
		return "", fmt.Errorf("error listing artifacts: %w", err)
	}

	prompt := promptui.Select{
		Label: "Select artifact to rebuild",
		Items: artifacts,
	}

	_, result, err := prompt.Run()

	if err != nil {
		fmt.Printf("Prompt failed %v\n", err)
		return "", fmt.Errorf("prompt failed: %w", err)
	}

	return result, nil
}

func rebuildArtifacts(args []string, baseURL string) error {
	if len(args) < 1 {
		artifact, err := selectArtifact(baseURL)
		if err != nil {
			return fmt.Errorf("error selecting artifact: %w", err)
		}
		args = append(args, artifact)
	}

	artifacts := strings.Join(args, ",")
	encodedArtifacts := url.QueryEscape(artifacts)

	resp, err := http.Get(baseURL + "/rebuild/" + encodedArtifacts)
	if err != nil {
		return fmt.Errorf("error triggering rebuild: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("error reading response: %w", err)
		}
		return fmt.Errorf("error response from skaf0: %s - %s", resp.Status, body)
	}

	fmt.Printf("Rebuild triggered for artifacts: %s\n", artifacts)
	return nil
}

func fetchArtifacts(baseURL string) ([]string, error) {
	resp, err := http.Get(baseURL + "/artifacts")
	if err != nil {
		return nil, fmt.Errorf("error fetching artifacts: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("error response from skaf0: %s", resp.Status)
	}

	var artifacts []string
	if err := json.NewDecoder(resp.Body).Decode(&artifacts); err != nil {
		return nil, fmt.Errorf("error decoding response: %w", err)
	}

	return artifacts, nil
}

func main() {
	flag.Parse()

	if len(os.Args) > 1 && os.Args[1] == "ctrl" {
		ctrl(os.Args[2:])
		return
	}

	originalGetDependenciesFunc = hijacked_getDependenciesFunc
	hijacked_getDependenciesFunc = overloaded_sourceDependenciesForArtifact

	originalWatch = hijacked_Watch
	hijacked_Watch = overloaded_Watch

	ar = NewArtifactResolver()

	go func() {
		mux := http.NewServeMux()
		mux.HandleFunc("/triggers", func(w http.ResponseWriter, r *http.Request) {
			files := ar.GetArtifactTriggerFiles()
			json.NewEncoder(w).Encode(files)
		})
		mux.HandleFunc("/artifacts", func(w http.ResponseWriter, r *http.Request) {
			artifacts := ar.GetArtifacts()
			json.NewEncoder(w).Encode(artifacts)
		})
		mux.HandleFunc("/watches", func(w http.ResponseWriter, r *http.Request) {
			watches := ar.GetWatches()
			json.NewEncoder(w).Encode(watches)
		})
		mux.HandleFunc("/rebuild/", func(w http.ResponseWriter, r *http.Request) {
			artifact := strings.TrimPrefix(r.URL.Path, "/rebuild/")
			if err := ar.TriggerRebuilds(artifact); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			w.WriteHeader(http.StatusOK)
		})
		http.ListenAndServe(*flagAddr, mux)
	}()

	if err := app.Run(os.Stdout, os.Stderr); err != nil {
		fmt.Println("Error executing skaffold dev", err)
	}
}
