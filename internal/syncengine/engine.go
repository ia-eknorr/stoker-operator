package syncengine

import (
	"os"
	"path/filepath"
	"time"
)

// SyncResult holds the outcome of a sync operation.
type SyncResult struct {
	FilesAdded     int
	FilesModified  int
	FilesDeleted   int
	ProjectsSynced []string
	Duration       time.Duration
	DryRunDiff     *DryRunDiff
}

// Engine handles syncing files from a source directory to a destination directory.
type Engine struct {
	ExcludePatterns []string
}

// discoverProjects scans the projects/ subdirectory under dstRoot and returns
// the list of Ignition project directory names found there.
func discoverProjects(dstRoot string) []string {
	projectsDir := filepath.Join(dstRoot, "projects")
	entries, err := os.ReadDir(projectsDir)
	if err != nil {
		return nil
	}

	var projects []string
	for _, e := range entries {
		if e.IsDir() {
			projects = append(projects, e.Name())
		}
	}
	return projects
}
