package security

import (
	"os"
	"path/filepath"
)

// ProjectScopeDetector finds active workspace boundaries.
type ProjectScopeDetector struct{}

// NewProjectScopeDetector creates an instance.
func NewProjectScopeDetector() *ProjectScopeDetector {
	return &ProjectScopeDetector{}
}

// DetectActiveProject searches parents from Cwd to find a .symmemory.toml or .git folder,
// returning the directory basename as the active project name.
func (psd *ProjectScopeDetector) DetectActiveProject() string {
	cwd, err := os.Getwd()
	if err != nil {
		return "default_project"
	}

	curr := cwd
	for {
		// Check for .symmemory.toml configuration
		tomlPath := filepath.Join(curr, ".symmemory.toml")
		if _, err := os.Stat(tomlPath); err == nil {
			return filepath.Base(curr)
		}

		// Check for .git folder boundary
		gitPath := filepath.Join(curr, ".git")
		if _, err := os.Stat(gitPath); err == nil {
			return filepath.Base(curr)
		}

		// Traverse up
		parent := filepath.Dir(curr)
		if parent == curr {
			break // Reached root directory
		}
		curr = parent
	}

	// Default to current folder if nothing found
	return filepath.Base(cwd)
}
