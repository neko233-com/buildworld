package integration_test

import (
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"
)

func TestLanguagePackagingMatrix(t *testing.T) {
	if os.Getenv("BUILDWORLD_RUN_LANGUAGE_MATRIX") != "1" {
		t.Skip("set BUILDWORLD_RUN_LANGUAGE_MATRIX=1 to execute installed SDKs and package every fixture")
	}

	root := repositoryRoot(t)
	fixtures := filepath.Join(root, "testdata", "language-matrix")

	t.Run("Go_host_and_cross_platform_binaries", func(t *testing.T) {
		requireTool(t, "go")
		work := copyFixture(t, filepath.Join(fixtures, "go"))
		cache := filepath.Join(t.TempDir(), "go-cache")
		run(t, work, env("GOCACHE", filepath.Join(cache, "build"), "GOMODCACHE", filepath.Join(cache, "mod"), "GOPATH", filepath.Join(cache, "path")), "go", "test", "./...")
		run(t, work, nil, "go", "vet", "./...")
		requireMkdir(t, filepath.Join(work, "dist"))
		hostArtifact := filepath.Join("dist", executableName("buildworld-go"))
		run(t, work, nil, "go", "build", "-trimpath", "-ldflags=-s -w", "-o", hostArtifact, ".")
		run(t, work, env("GOOS", "linux", "GOARCH", "amd64", "CGO_ENABLED", "0"), "go", "build", "-trimpath", "-o", filepath.Join("dist", "buildworld-go-linux-amd64"), ".")
		requireFile(t, filepath.Join(work, hostArtifact))
		requireFile(t, filepath.Join(work, "dist", "buildworld-go-linux-amd64"))
	})

	t.Run("Rust_release_and_source_package", func(t *testing.T) {
		requireTool(t, "cargo")
		requireTool(t, "rustc")
		work := copyFixture(t, filepath.Join(fixtures, "rust"))
		cargoHome := filepath.Join(t.TempDir(), "cargo-home")
		isolated := env("CARGO_HOME", cargoHome, "CARGO_TERM_COLOR", "never")
		run(t, work, isolated, "cargo", "fmt", "--check")
		run(t, work, isolated, "cargo", "clippy", "--all-targets", "--", "-D", "warnings")
		run(t, work, isolated, "cargo", "test", "--locked")
		run(t, work, isolated, "cargo", "build", "--release", "--locked")
		run(t, work, isolated, "cargo", "package", "--allow-dirty", "--no-verify", "--locked")
		requireFile(t, filepath.Join(work, "target", "release", executableName("buildworld-rust-minimal")))
		requireGlob(t, filepath.Join(work, "target", "package", "*.crate"))
	})

	t.Run("TypeScript_ESM_declarations_and_npm_package", func(t *testing.T) {
		requireTool(t, "node")
		requireTool(t, "npm")
		work := copyFixture(t, filepath.Join(fixtures, "typescript"))
		compiler := filepath.Join(root, "web", "node_modules", "typescript", "bin", "tsc")
		requireFile(t, compiler)
		run(t, work, nil, "node", compiler, "-p", "tsconfig.json")
		run(t, work, nil, "npm", "test")
		packageDir := filepath.Join(work, "package")
		requireMkdir(t, packageDir)
		run(t, work, env("NPM_CONFIG_CACHE", filepath.Join(t.TempDir(), "npm-cache")), "npm", "pack", "--pack-destination", packageDir)
		requireFile(t, filepath.Join(work, "dist", "src", "index.js"))
		requireFile(t, filepath.Join(work, "dist", "src", "index.d.ts"))
		requireGlob(t, filepath.Join(packageDir, "*.tgz"))
	})

	t.Run("CSharp_build_publish_and_NuGet_package", func(t *testing.T) {
		requireTool(t, "dotnet")
		work := copyFixture(t, filepath.Join(fixtures, "csharp"))
		isolated := env(
			"DOTNET_CLI_HOME", filepath.Join(t.TempDir(), "dotnet-home"),
			"NUGET_PACKAGES", filepath.Join(t.TempDir(), "nuget-packages"),
			"DOTNET_NOLOGO", "1",
			"DOTNET_CLI_TELEMETRY_OPTOUT", "1",
		)
		project := "BuildWorld.CSharp.Minimal.csproj"
		run(t, work, isolated, "dotnet", "restore", project)
		run(t, work, isolated, "dotnet", "build", project, "-c", "Release", "--no-restore")
		run(t, work, isolated, "dotnet", "run", "--project", project, "-c", "Release", "--no-build")
		run(t, work, isolated, "dotnet", "publish", project, "-c", "Release", "--no-build", "-o", filepath.Join("out", "publish"))
		run(t, work, isolated, "dotnet", "pack", project, "-c", "Release", "--no-build", "-o", filepath.Join("out", "package"))
		requireFile(t, filepath.Join(work, "out", "publish", "BuildWorld.CSharp.Minimal.dll"))
		requireGlob(t, filepath.Join(work, "out", "package", "*.nupkg"))
	})

	t.Run("Java_Gradle_check_and_runnable_JAR", func(t *testing.T) {
		requireTool(t, "java")
		requireTool(t, "javac")
		requireTool(t, "gradle")
		work := copyFixture(t, filepath.Join(fixtures, "java"))
		isolated := env("GRADLE_USER_HOME", filepath.Join(t.TempDir(), "gradle-home"))
		run(t, work, isolated, "gradle", "--no-daemon", "--console=plain", "clean", "check", "jar")
		jar := firstGlob(t, filepath.Join(work, "build", "libs", "*.jar"))
		run(t, work, nil, "java", "-jar", jar)
	})

}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	_, current, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot resolve integration test location")
	}
	return filepath.Dir(filepath.Dir(current))
}

func executableName(name string) string {
	if runtime.GOOS == "windows" {
		return name + ".exe"
	}
	return name
}

func env(values ...string) []string {
	result := make([]string, 0, len(values)/2)
	for index := 0; index+1 < len(values); index += 2 {
		result = append(result, values[index]+"="+values[index+1])
	}
	return result
}

func requireTool(t *testing.T, name string) string {
	t.Helper()
	path, err := exec.LookPath(name)
	if err != nil {
		t.Skipf("%s is not installed or not on PATH", name)
	}
	t.Logf("%s: %s", name, path)
	return path
}

func run(t *testing.T, directory string, extraEnv []string, name string, args ...string) {
	t.Helper()
	tool := requireTool(t, name)
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
	defer cancel()

	var command *exec.Cmd
	extension := strings.ToLower(filepath.Ext(tool))
	if runtime.GOOS == "windows" && (extension == ".cmd" || extension == ".bat") {
		commandArgs := append([]string{"/d", "/c", tool}, args...)
		command = exec.CommandContext(ctx, "cmd.exe", commandArgs...)
	} else {
		command = exec.CommandContext(ctx, tool, args...)
	}
	command.Dir = directory
	command.Env = append(os.Environ(), extraEnv...)
	output, err := command.CombinedOutput()
	t.Logf("$ %s %s\n%s", name, strings.Join(args, " "), strings.TrimSpace(string(output)))
	if ctx.Err() == context.DeadlineExceeded {
		t.Fatalf("%s exceeded the six-minute command timeout", name)
	}
	if err != nil {
		t.Fatalf("%s failed: %v", name, err)
	}
}

func copyFixture(t *testing.T, source string) string {
	t.Helper()
	destination := filepath.Join(t.TempDir(), filepath.Base(source))
	if err := copyTree(source, destination); err != nil {
		t.Fatalf("copy fixture %s: %v", source, err)
	}
	return destination
}

func copyTree(source, destination string) error {
	return filepath.Walk(source, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		target := filepath.Join(destination, relative)
		if info.IsDir() {
			return os.MkdirAll(target, info.Mode())
		}
		input, err := os.Open(path)
		if err != nil {
			return err
		}
		defer input.Close()
		output, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, info.Mode())
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(output, input)
		closeErr := output.Close()
		if copyErr != nil {
			return copyErr
		}
		return closeErr
	})
}

func requireMkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("create %s: %v", path, err)
	}
}

func requireFile(t *testing.T, path string) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil || info.IsDir() || info.Size() == 0 {
		t.Fatalf("expected non-empty artifact file %s", path)
	}
	t.Logf("artifact: %s (%d bytes)", path, info.Size())
}

func requireGlob(t *testing.T, pattern string) {
	t.Helper()
	_ = firstGlob(t, pattern)
}

func firstGlob(t *testing.T, pattern string) string {
	t.Helper()
	matches, err := filepath.Glob(pattern)
	if err != nil || len(matches) == 0 {
		t.Fatalf("expected artifact matching %s", pattern)
	}
	sort.Strings(matches)
	requireFile(t, matches[0])
	return matches[0]
}
