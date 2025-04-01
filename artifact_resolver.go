package main

import (
	"context"
	"fmt"
	"maps"
	"math/rand"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"github.com/GoogleContainerTools/skaffold/v2/pkg/skaffold/docker"
	"github.com/GoogleContainerTools/skaffold/v2/pkg/skaffold/schema/latest"
	"github.com/gobwas/glob"
	"github.com/rjeczalik/notify"
)

type artifact struct {
	imageName   string
	workspace   string
	triggerFile string
}

type ArtifactResolver struct {
	mtx       sync.Mutex
	artifacts map[string]*artifact
	watches   map[string]chan<- notify.EventInfo
}

func NewArtifactResolver() *ArtifactResolver {
	return &ArtifactResolver{
		artifacts: make(map[string]*artifact),
		watches:   make(map[string]chan<- notify.EventInfo),
	}
}

func (r *ArtifactResolver) AddWatch(path string, c chan<- notify.EventInfo, events ...notify.Event) error {
	r.mtx.Lock()
	defer r.mtx.Unlock()

	r.watches[path] = c
	return nil
}

type fakeEventInfo struct {
	path string
}

func (f fakeEventInfo) Event() notify.Event {
	return notify.Write
}

func (f fakeEventInfo) Path() string {
	return f.path
}

func (f fakeEventInfo) Sys() interface{} {
	return nil
}

// TriggerRebuilds triggers rebuilds for all artifacts whose names match the given pattern.
// The pattern can be a comma-separated list of glob patterns (as defined by filepath.Match).
// For example:
//   - "frontend" would match only the artifact named "frontend"
//   - "frontend,backend" would match artifacts named "frontend" and "backend"
//   - "front*" would match artifacts with names starting with "front"
//   - "*" would match all artifacts
//
// Returns an error if no artifacts match the pattern or if any rebuild fails.
func (r *ArtifactResolver) TriggerRebuilds(pattern string) error {
	artifacts := r.GetArtifacts()

	patterns := strings.Split(pattern, ",")
	for i := range patterns {
		patterns[i] = strings.TrimSpace(patterns[i])
	}

	var matchedArtifacts []string
	for _, p := range patterns {
		gp, err := glob.Compile(p)
		if err != nil {
			return fmt.Errorf("invalid pattern %q: %w", p, err)
		}
		for _, artifactName := range artifacts {
			if gp.Match(artifactName) {
				matchedArtifacts = append(matchedArtifacts, artifactName)
			}
		}
	}

	fmt.Fprintf(os.Stderr, "\033[31mTriggering rebuilds for pattern: '%s', matched artifacts: %v\033[0m\n", pattern, matchedArtifacts)

	if len(matchedArtifacts) == 0 {
		return fmt.Errorf("no artifacts matched pattern: %s", pattern)
	}

	var errs []string
	for _, artifactName := range matchedArtifacts {
		if err := r.TriggerRebuild(artifactName); err != nil {
			errs = append(errs, fmt.Sprintf("failed to trigger rebuild for %s: %v", artifactName, err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("errors triggering rebuilds: %s", strings.Join(errs, "; "))
	}
	return nil
}

func (r *ArtifactResolver) WatchPath(workspace string) (string, error) {
	// Construct the absolute path with "..." suffix for watching
	wd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("failed to get working directory: %w", err)
	}

	var watchPath string
	if filepath.IsAbs(workspace) {
		watchPath = filepath.Join(workspace, "...")
	} else {
		watchPath = filepath.Join(wd, workspace, "...")
	}

	return watchPath, nil
}

func (r *ArtifactResolver) TriggerRebuild(artifactName string) error {
	r.mtx.Lock()
	defer r.mtx.Unlock()

	art, ok := r.artifacts[artifactName]
	if !ok {
		return fmt.Errorf("artifact not found: %s", artifactName)
	}

	watchPath, err := r.WatchPath(art.workspace)
	if err != nil {
		return fmt.Errorf("failed to get watch path: %w", err)
	}

	watch, ok := r.watches[watchPath]
	if !ok {
		return fmt.Errorf("watch not found for artifact: %s (watch path: %s)", artifactName, watchPath)
	}

	// Write a random byte to trigger file to simulate a change
	if err := os.WriteFile(art.triggerFile, []byte{byte(rand.Intn(256))}, 0644); err != nil {
		return fmt.Errorf("failed to write to trigger file: %w", err)
	}

	watch <- &fakeEventInfo{path: art.triggerFile}

	return nil
}

func (r *ArtifactResolver) GetDependencies(ctx context.Context, a *latest.Artifact, cfg docker.Config, tag string) ([]string, error) {
	r.mtx.Lock()
	defer r.mtx.Unlock()

	art, ok := r.artifacts[a.ImageName]

	if !ok {
		f, err := os.CreateTemp("", "skaf0-*")
		if err != nil {
			return nil, fmt.Errorf("failed to create temp file: %w", err)
		}
		defer f.Close()

		fileName := f.Name()

		art = &artifact{
			imageName:   a.ImageName,
			workspace:   a.Workspace,
			triggerFile: fileName,
		}
		r.artifacts[a.ImageName] = art
	}

	return []string{art.triggerFile}, nil
}

func (r *ArtifactResolver) GetArtifactTriggerFiles() map[string]string {
	r.mtx.Lock()
	defer r.mtx.Unlock()

	result := make(map[string]string, len(r.artifacts))
	for k, v := range r.artifacts {
		result[k] = v.triggerFile
	}
	return result
}

func (r *ArtifactResolver) GetWatches() []string {
	r.mtx.Lock()
	defer r.mtx.Unlock()

	return slices.Collect(maps.Keys(r.watches))
}

func (r *ArtifactResolver) GetArtifacts() []string {
	r.mtx.Lock()
	defer r.mtx.Unlock()

	return slices.Collect(maps.Keys(r.artifacts))
}
