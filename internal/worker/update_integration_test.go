package worker

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/neko233-com/buildworld/internal/workercontrol"
)

func TestNativeWorkerUpdateReadinessAndRollback(t *testing.T) {
	root := t.TempDir()
	binaryName := "buildworld-worker"
	if runtime.GOOS == "windows" {
		binaryName += ".exe"
	}
	binary := filepath.Join(root, binaryName)
	build := exec.Command("go", "build", "-buildvcs=false", "-ldflags", "-X github.com/neko233-com/buildworld/internal/buildinfo.Version=v1.17.0", "-o", binary, "./cmd/worker")
	build.Dir = filepath.Join("..", "..")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build worker fixture: %v %s", err, out)
	}
	original, _ := os.ReadFile(binary)
	for _, rollback := range []bool{false, true} {
		name := "ready"
		if rollback {
			name = "rollback"
		}
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			installed := filepath.Join(dir, binaryName)
			if err := os.WriteFile(installed, original, 0700); err != nil {
				t.Fatal(err)
			}
			candidate := original
			version := "v1.17.0"
			if rollback {
				version = "v1.17.1"
				fixture := filepath.Join(dir, "candidate.go")
				source := `package main;import("os";"fmt");func main(){if len(os.Args)==2&&os.Args[1]=="--version"{fmt.Print("{\"version\":\"v1.17.1\"}");return};os.Exit(9)}`
				_ = os.WriteFile(fixture, []byte(source), 0600)
				out := filepath.Join(dir, "invalid-start"+filepath.Ext(binaryName))
				if data, err := exec.Command("go", "build", "-buildvcs=false", "-o", out, fixture).CombinedOutput(); err != nil {
					t.Fatalf("build failing candidate: %v %s", err, data)
				}
				candidate, _ = os.ReadFile(out)
			}
			metadata := []byte(fmt.Sprintf(`{"schemaVersion":1,"repository":"https://github.com/neko233-com/buildworld","version":%q,"sourceCommit":%q}`, version, strings.Repeat("a", 40)))
			_ = os.WriteFile(filepath.Join(dir, "release-info.json"), []byte(`{"controller":"must remain unchanged"}`), 0600)
			var archive bytes.Buffer
			zipper := zip.NewWriter(&archive)
			for name, data := range map[string][]byte{binaryName: candidate, "release-info.json": metadata, "web/dist/index.html": []byte("html"), "sdk/pipeline/index.js": {}, "sdk/pipeline/index.d.ts": {}, "sdk/pipeline/package.json": {}} {
				entry, _ := zipper.Create(name)
				_, _ = entry.Write(data)
			}
			_ = zipper.Close()
			sum := sha256.Sum256(archive.Bytes())
			operation := &workercontrol.Operation{ID: "native-test-update", Kind: "update", Version: version, Platform: runtime.GOOS + "/" + runtime.GOARCH, SHA256: hex.EncodeToString(sum[:])}
			var mu sync.Mutex
			pids := map[int]bool{}
			completed := make(chan workercontrol.Completion, 1)
			controller := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.URL.Path == "/api/agents/auto-register" {
					_ = json.NewEncoder(w).Encode(map[string]any{"agent": map[string]string{"id": "stable-worker"}, "token": "fixture-worker-token"})
					return
				}
				if strings.Contains(r.URL.Path, "/bundle/") {
					_, _ = w.Write(archive.Bytes())
					return
				}
				var report workercontrol.Heartbeat
				if json.NewDecoder(r.Body).Decode(&report) != nil {
					w.WriteHeader(400)
					return
				}
				if report.Runtime != nil {
					mu.Lock()
					pids[report.Runtime.PID] = true
					mu.Unlock()
				}
				if report.Completion != nil {
					select {
					case completed <- *report.Completion:
					default:
					}
					_ = json.NewEncoder(w).Encode(workercontrol.Reply{Status: "ok"})
					return
				}
				_ = json.NewEncoder(w).Encode(workercontrol.Reply{Status: "ok", Operation: operation})
			}))
			defer controller.Close()
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			address := listener.Addr().String()
			listener.Close()
			command := exec.Command(installed, "--server", controller.URL, "--token", "fixture-enrollment", "--listen", address, "--state-dir", filepath.Join(dir, "state"), "--build-temp", filepath.Join(dir, "build_temp"))
			command.Dir = dir
			hideProcess(command)
			if err = command.Start(); err != nil {
				t.Fatal(err)
			}
			defer command.Wait()
			defer func() {
				mu.Lock()
				defer mu.Unlock()
				pids[command.Process.Pid] = true
				for pid := range pids {
					if process, e := os.FindProcess(pid); e == nil {
						_ = process.Kill()
					}
				}
			}()
			select {
			case result := <-completed:
				want := "success"
				if rollback {
					want = "rolled_back"
				}
				if result.Status != want {
					t.Fatalf("update completion = %+v, want %s", result, want)
				}
			case <-time.After(40 * time.Second):
				t.Fatal("native worker update did not complete")
			}
			current, _ := os.ReadFile(filepath.Join(dir, "release-info.json"))
			if string(current) != `{"controller":"must remain unchanged"}` {
				t.Fatal("worker update changed controller metadata")
			}
			if rollback {
				current, _ := os.ReadFile(installed)
				originalSum, currentSum := sha256.Sum256(original), sha256.Sum256(current)
				if currentSum != originalSum {
					t.Fatal("rollback did not restore original binary")
				}
			}
			mu.Lock()
			count := len(pids)
			mu.Unlock()
			if !rollback && count < 2 {
				t.Fatal("update did not acknowledge a new process")
			}
		})
	}
}
