package api

import (
	"bufio"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/fsnotify/fsnotify"
)

func TestLiveReloadRecognizesFrontendAssets(t *testing.T) {
	for _, path := range []string{"index.html", "assets/site.css", "assets/main.js", "src/App.tsx", "data/config.json"} {
		if !isReloadableAsset(path) {
			t.Fatalf("expected %s to reload", path)
		}
	}
	for _, path := range []string{"assets/icon.svg", "README.md", "server"} {
		if isReloadableAsset(path) {
			t.Fatalf("did not expect %s to reload", path)
		}
	}
}

func TestLiveReloadPublishesPublicAssetPath(t *testing.T) {
	root := t.TempDir()
	live := &LiveReload{root: root, subscribers: make(map[chan reloadEvent]struct{})}
	channel, unsubscribe := live.subscribe()
	defer unsubscribe()
	live.publish(filepath.Join(root, "assets", "site.css"))

	select {
	case event := <-channel:
		if event.Path != "/assets/site.css" {
			t.Fatalf("event path = %q", event.Path)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for reload event")
	}
}

func TestLiveReloadStatusRouteReportsAggregateDiagnosticsWithoutPaths(t *testing.T) {
	root := t.TempDir()
	cssPath := filepath.Join(root, "assets", "site.css")
	if err := os.MkdirAll(filepath.Dir(cssPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cssPath, []byte("body{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	assets, err := snapshotReloadableAssets(root)
	if err != nil {
		t.Fatal(err)
	}
	live := &LiveReload{
		root:        root,
		subscribers: make(map[chan reloadEvent]struct{}),
		assets:      assets,
		lastChange:  make(map[string]publishedAssetChange),
	}
	live.running.Store(true)
	live.assetCount.Store(uint64(len(assets)))
	channel, unsubscribe := live.subscribe()

	live.handleWatchEvent(fsnotify.Event{Name: cssPath, Op: fsnotify.Write})
	live.pollAssets()
	select {
	case <-channel:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for synthetic native event")
	}

	response := httptest.NewRecorder()
	NewRouter(Deps{LiveReload: live}).ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/__buildworld/livereload/status", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", response.Code)
	}
	if got := response.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("Cache-Control = %q, want no-store", got)
	}
	if strings.Contains(response.Body.String(), root) || strings.Contains(response.Body.String(), cssPath) {
		t.Fatalf("status leaked watched path: %s", response.Body.String())
	}
	var status LiveReloadStatus
	if err := json.Unmarshal(response.Body.Bytes(), &status); err != nil {
		t.Fatal(err)
	}
	want := (LiveReloadStatus{Running: true, Polls: 1, NativeEvents: 1, Published: 1, Assets: 1, Subscribers: 1})
	if status != want {
		t.Fatalf("status = %+v, want %+v", status, want)
	}

	unsubscribe()
	if got := live.Status().Subscribers; got != 0 {
		t.Fatalf("subscribers after unsubscribe = %d, want 0", got)
	}
}

func TestLiveReloadPollingDetectsModifyCreateAndDelete(t *testing.T) {
	root := t.TempDir()
	cssPath := filepath.Join(root, "assets", "site.css")
	htmlPath := filepath.Join(root, "index.html")
	if err := os.MkdirAll(filepath.Dir(cssPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cssPath, []byte("body{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(htmlPath, []byte("<!doctype html>"), 0o644); err != nil {
		t.Fatal(err)
	}

	live, err := NewLiveReload(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(live.Stop)
	if status := live.Status(); !status.Running || status.Assets != 2 {
		t.Fatalf("initial status = %+v, want running with 2 assets", status)
	}

	// Disable the native fast path so this integration test proves that the
	// real-directory polling fallback remains independently functional.
	if err := live.watcher.Close(); err != nil {
		t.Fatal(err)
	}
	channel, unsubscribe := live.subscribe()
	t.Cleanup(unsubscribe)

	if err := os.WriteFile(cssPath, []byte("body{color:#123456}"), 0o644); err != nil {
		t.Fatal(err)
	}
	waitForReloadEvent(t, channel, "/assets/site.css")

	jsPath := filepath.Join(root, "assets", "main.js")
	if err := os.WriteFile(jsPath, []byte("console.log('ready')"), 0o644); err != nil {
		t.Fatal(err)
	}
	waitForReloadEvent(t, channel, "/assets/main.js")

	if err := os.Remove(htmlPath); err != nil {
		t.Fatal(err)
	}
	waitForReloadEvent(t, channel, "/index.html")

	live.Stop()
	status := live.Status()
	if status.Running || status.Polls == 0 || status.Published < 3 || status.Assets != 2 {
		t.Fatalf("stopped status = %+v, want stopped with polling and 2 assets", status)
	}
}

func TestLiveReloadProductionRootWhenConfigured(t *testing.T) {
	root := os.Getenv("BUILDWORLD_LIVERELOAD_QA_ROOT")
	if root == "" {
		t.Skip("BUILDWORLD_LIVERELOAD_QA_ROOT is not configured")
	}
	qaPath := filepath.Join(root, "buildworld-live-reload-production-qa.css")
	if _, err := os.Stat(qaPath); !os.IsNotExist(err) {
		t.Fatalf("QA path must not exist before the test: %s", qaPath)
	}
	t.Cleanup(func() { _ = os.Remove(qaPath) })

	live, err := NewLiveReload(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(live.Stop)
	channel, unsubscribe := live.subscribe()
	t.Cleanup(unsubscribe)

	if err := os.WriteFile(qaPath, []byte("/* production-root QA */\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	waitForReloadEvent(t, channel, "/buildworld-live-reload-production-qa.css")
	if err := os.Remove(qaPath); err != nil {
		t.Fatal(err)
	}
	waitForReloadEvent(t, channel, "/buildworld-live-reload-production-qa.css")
}

func waitForReloadEvent(t *testing.T, channel <-chan reloadEvent, want string) {
	t.Helper()
	deadline := time.NewTimer(3 * time.Second)
	defer deadline.Stop()
	for {
		select {
		case event := <-channel:
			if event.Path == want {
				return
			}
		case <-deadline.C:
			t.Fatalf("timed out waiting for reload event %q", want)
		}
	}
}

func TestLiveReloadSSESurvivesServerWriteTimeout(t *testing.T) {
	root := t.TempDir()
	live, err := NewLiveReload(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(live.Stop)
	// Exercise heartbeats quickly so this test covers both idle stream upkeep
	// and an asset event written after the ordinary response deadline.
	live.heartbeat = 10 * time.Millisecond

	server := httptest.NewUnstartedServer(NewRouter(Deps{LiveReload: live}))
	server.Config.WriteTimeout = 50 * time.Millisecond
	server.Start()
	t.Cleanup(server.Close)

	client := server.Client()
	client.Timeout = 2 * time.Second
	response, err := client.Get(server.URL + "/__buildworld/livereload")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", response.StatusCode)
	}
	reader := bufio.NewReader(response.Body)
	if line, err := reader.ReadString('\n'); err != nil || line != "retry: 1000\n" {
		t.Fatalf("initial SSE line = %q, err = %v", line, err)
	}
	if line, err := reader.ReadString('\n'); err != nil || line != "\n" {
		t.Fatalf("initial SSE separator = %q, err = %v", line, err)
	}

	// Go's server WriteTimeout is absolute for the response. Without the
	// per-response deadline override, the stream becomes unwritable here.
	time.Sleep(4 * server.Config.WriteTimeout)
	live.publish(filepath.Join(root, "assets", "timeout-qa.css"))

	var eventName string
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatalf("read SSE after server WriteTimeout: %v", err)
		}
		switch {
		case strings.HasPrefix(line, "event: "):
			eventName = strings.TrimSpace(strings.TrimPrefix(line, "event: "))
		case strings.HasPrefix(line, "data: "):
			if eventName != "reload" {
				continue
			}
			var event reloadEvent
			if err := json.Unmarshal([]byte(strings.TrimSpace(strings.TrimPrefix(line, "data: "))), &event); err != nil {
				t.Fatal(err)
			}
			if event.Path != "/assets/timeout-qa.css" {
				t.Fatalf("reload path = %q", event.Path)
			}
			return
		}
	}
}

func TestLiveReloadScriptHandlesCSSWithoutFullPageReload(t *testing.T) {
	live := &LiveReload{}
	response := httptest.NewRecorder()
	live.ServeScript(response, httptest.NewRequest("GET", "/__buildworld/livereload.js", nil))
	if response.Code != 200 {
		t.Fatalf("status = %d", response.Code)
	}
	body := response.Body.String()
	if !strings.Contains(body, "EventSource") || !strings.Contains(body, "link[rel=\"stylesheet\"]") || !strings.Contains(body, "window.location.reload") {
		t.Fatalf("unexpected live reload script: %s", body)
	}
}

func TestLiveReloadClientIsInjectedIntoStaticIndex(t *testing.T) {
	static := fstest.MapFS{
		"index.html": &fstest.MapFile{Data: []byte("<!doctype html><html><head><title>buildworld</title></head><body></body></html>")},
	}
	for _, scenario := range []struct {
		name       string
		liveReload *LiveReload
		wantScript bool
	}{
		{name: "enabled", liveReload: &LiveReload{}, wantScript: true},
		{name: "disabled", wantScript: false},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			router := NewRouter(Deps{StaticFS: static, LiveReload: scenario.liveReload})
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/", nil))
			if response.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200", response.Code)
			}
			hasScript := strings.Contains(response.Body.String(), `<script src="/__buildworld/livereload.js"></script>`)
			if hasScript != scenario.wantScript {
				t.Fatalf("live reload script present = %t, want %t; body=%s", hasScript, scenario.wantScript, response.Body.String())
			}
		})
	}
}
