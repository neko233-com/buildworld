package engine

import (
	"fmt"
	"os"
	"path/filepath"
)

// BuildEnvironment owns every disposable workspace and language cache used by
// a server or worker process. Relative roots are resolved beside the running
// binary so Windows builds never silently spill into the system temp folder.
type BuildEnvironment struct {
	root string
}

func NewBuildEnvironment(root string) *BuildEnvironment {
	return &BuildEnvironment{root: ResolveBuildTempRoot(root)}
}

func ResolveBuildTempRoot(root string) string {
	if root == "" {
		root = "./build_temp"
	}
	if filepath.IsAbs(root) {
		return filepath.Clean(root)
	}
	executable, err := os.Executable()
	if err != nil {
		absolute, absErr := filepath.Abs(root)
		if absErr == nil {
			return filepath.Clean(absolute)
		}
		return filepath.Clean(root)
	}
	return filepath.Clean(filepath.Join(filepath.Dir(executable), root))
}

func (e *BuildEnvironment) Root() string { return e.root }

func (e *BuildEnvironment) WorkspacesRoot() string {
	return filepath.Join(e.root, "workspaces")
}

func (e *BuildEnvironment) PluginCacheRoot() string {
	return filepath.Join(e.root, "cache", "plugins")
}

func (e *BuildEnvironment) Ensure() error {
	directories := []string{
		e.WorkspacesRoot(),
		filepath.Join(e.root, "scripts"),
		filepath.Join(e.root, "cache", "go", "build"),
		filepath.Join(e.root, "cache", "go", "mod"),
		filepath.Join(e.root, "cache", "go", "path"),
		filepath.Join(e.root, "cache", "node", "npm"),
		filepath.Join(e.root, "cache", "node", "corepack"),
		filepath.Join(e.root, "cache", "node", "pnpm"),
	}
	for _, directory := range directories {
		if err := os.MkdirAll(directory, 0o755); err != nil {
			return fmt.Errorf("create build environment directory %s: %w", directory, err)
		}
	}
	return nil
}

// Environment returns process variables that isolate Go, Node.js/npm and all
// generic temporary files from the host profile and operating-system temp dir.
func (e *BuildEnvironment) Environment(workspace string) ([]string, error) {
	if err := e.Ensure(); err != nil {
		return nil, err
	}
	temporary := filepath.Join(workspace, ".buildworld", "tmp")
	if err := os.MkdirAll(temporary, 0o755); err != nil {
		return nil, fmt.Errorf("create build temporary directory: %w", err)
	}
	goCache := filepath.Join(e.root, "cache", "go")
	nodeCache := filepath.Join(e.root, "cache", "node")
	return []string{
		"BUILDWORLD_BUILD_TEMP=" + e.root,
		"BUILDWORLD_WORKSPACE=" + workspace,
		"TMP=" + temporary,
		"TEMP=" + temporary,
		"TMPDIR=" + temporary,
		"GOCACHE=" + filepath.Join(goCache, "build"),
		"GOMODCACHE=" + filepath.Join(goCache, "mod"),
		"GOPATH=" + filepath.Join(goCache, "path"),
		"NPM_CONFIG_CACHE=" + filepath.Join(nodeCache, "npm"),
		"npm_config_cache=" + filepath.Join(nodeCache, "npm"),
		"NPM_CONFIG_PREFIX=" + filepath.Join(e.root, "env", "node"),
		"NPM_CONFIG_USERCONFIG=" + filepath.Join(nodeCache, "npmrc"),
		"COREPACK_HOME=" + filepath.Join(nodeCache, "corepack"),
		"PNPM_HOME=" + filepath.Join(nodeCache, "pnpm"),
		"YARN_CACHE_FOLDER=" + filepath.Join(nodeCache, "yarn"),
	}, nil
}
