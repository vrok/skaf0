package main

import (
	"context"
	"testing"

	"github.com/GoogleContainerTools/skaffold/v2/pkg/skaffold/schema/latest"
	"github.com/rjeczalik/notify"
	"github.com/stretchr/testify/assert"
)

func TestNewArtifactResolver(t *testing.T) {
	resolver := NewArtifactResolver()
	assert.NotNil(t, resolver)
	assert.Empty(t, resolver.artifacts)
	assert.Empty(t, resolver.watches)
}

func TestAddWatch(t *testing.T) {
	resolver := NewArtifactResolver()
	watchChan := make(chan notify.EventInfo)
	err := resolver.AddWatch("/test/path", watchChan)
	assert.NoError(t, err)
	assert.Contains(t, resolver.watches, "/test/path")
}

func TestGetDependencies(t *testing.T) {
	resolver := NewArtifactResolver()
	ctx := context.Background()
	artifact := &latest.Artifact{
		ImageName: "test-image",
		Workspace: "test-workspace",
	}
	tag := "latest"

	// Test creating new artifact
	deps, err := resolver.GetDependencies(ctx, artifact, nil, tag)
	assert.NoError(t, err)
	assert.Len(t, deps, 1)
	assert.Contains(t, resolver.artifacts, "test-image")

	// Test retrieving existing artifact
	deps, err = resolver.GetDependencies(ctx, artifact, nil, tag)
	assert.NoError(t, err)
	assert.Len(t, deps, 1)
}

func TestTriggerRebuild(t *testing.T) {
	resolver := NewArtifactResolver()
	watchChan := make(chan notify.EventInfo, 10)

	// Setup test artifact
	artifact := &latest.Artifact{
		ImageName: "test-image",
		Workspace: "test-workspace",
	}
	_, err := resolver.GetDependencies(context.Background(), artifact, nil, "latest")
	assert.NoError(t, err)

	// Add watch for the artifact
	watchPath, err := resolver.WatchPath("test-workspace")
	assert.NoError(t, err)
	err = resolver.AddWatch(watchPath, watchChan)
	assert.NoError(t, err)

	// Test triggering rebuild
	err = resolver.TriggerRebuild("test-image")
	assert.NoError(t, err)

	// Verify event was sent to channel
	select {
	case event := <-watchChan:
		// Verify the event path matches the trigger file
		assert.Equal(t, resolver.artifacts["test-image"].triggerFile, event.Path())
	default:
		t.Error("Expected event to be sent to channel but none was received")
	}

	// Test triggering non-existent artifact
	err = resolver.TriggerRebuild("non-existent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "artifact not found")
}

func TestTriggerRebuilds(t *testing.T) {
	resolver := NewArtifactResolver()
	watchChan := make(chan notify.EventInfo, 10)

	// Setup test artifacts
	artifacts := []*latest.Artifact{
		{ImageName: "frontend", Workspace: "frontend-workspace"},
		{ImageName: "backend", Workspace: "backend-workspace"},
		{ImageName: "backend-api", Workspace: "backend-workspace"},
	}

	for _, a := range artifacts {
		_, err := resolver.GetDependencies(context.Background(), a, nil, "latest")
		assert.NoError(t, err)
		watchPath, err := resolver.WatchPath(a.Workspace)
		assert.NoError(t, err)
		err = resolver.AddWatch(watchPath, watchChan)
		assert.NoError(t, err)
	}

	tests := []struct {
		name          string
		pattern       string
		expectedCount int
		expectError   bool
	}{
		{
			name:          "exact match",
			pattern:       "frontend",
			expectedCount: 1,
			expectError:   false,
		},
		{
			name:          "multiple exact matches",
			pattern:       "frontend,backend",
			expectedCount: 2,
			expectError:   false,
		},
		{
			name:          "wildcard match",
			pattern:       "backend*",
			expectedCount: 2,
			expectError:   false,
		},
		{
			name:          "match all",
			pattern:       "*",
			expectedCount: 3,
			expectError:   false,
		},
		{
			name:        "no matches",
			pattern:     "nonexistent",
			expectError: true,
		},
		{
			name:        "invalid pattern",
			pattern:     "[invalid",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := resolver.TriggerRebuilds(tt.pattern)
			if tt.expectError {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
		})
	}
}

func TestGetArtifactTriggerFiles(t *testing.T) {
	resolver := NewArtifactResolver()

	// Setup test artifact
	artifact := &latest.Artifact{
		ImageName: "test-image",
		Workspace: "test-workspace",
	}
	_, err := resolver.GetDependencies(context.Background(), artifact, nil, "latest")
	assert.NoError(t, err)

	triggerFiles := resolver.GetArtifactTriggerFiles()
	assert.Len(t, triggerFiles, 1)
	assert.Contains(t, triggerFiles, "test-image")
}

func TestGetWatches(t *testing.T) {
	resolver := NewArtifactResolver()
	watchChan := make(chan notify.EventInfo)

	// Add test watches
	watchPaths := []string{
		"/test/path1/...",
		"/test/path2/...",
	}

	for _, path := range watchPaths {
		err := resolver.AddWatch(path, watchChan)
		assert.NoError(t, err)
	}

	watches := resolver.GetWatches()
	assert.Len(t, watches, 2)
	for _, path := range watchPaths {
		assert.Contains(t, watches, path)
	}
}

func TestGetArtifacts(t *testing.T) {
	resolver := NewArtifactResolver()

	// Setup test artifacts
	artifacts := []*latest.Artifact{
		{ImageName: "frontend", Workspace: "frontend-workspace"},
		{ImageName: "backend", Workspace: "backend-workspace"},
	}

	for _, a := range artifacts {
		_, err := resolver.GetDependencies(context.Background(), a, nil, "latest")
		assert.NoError(t, err)
	}

	artifactNames := resolver.GetArtifacts()
	assert.Len(t, artifactNames, 2)
	for _, a := range artifacts {
		assert.Contains(t, artifactNames, a.ImageName)
	}
}
