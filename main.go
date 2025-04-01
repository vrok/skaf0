package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"strings"

	_ "unsafe"

	"github.com/GoogleContainerTools/skaffold/v2/cmd/skaffold/app"
	"github.com/GoogleContainerTools/skaffold/v2/pkg/skaffold/docker"
	"github.com/GoogleContainerTools/skaffold/v2/pkg/skaffold/schema/latest"
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

func main() {
	flag.Parse()

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
			if err := ar.TriggerRebuild(artifact); err != nil {
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
