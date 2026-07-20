package api

import (
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
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
