package api

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

// LiveReload watches a disk-backed web bundle and tells connected browser
// sessions when a frontend source asset changes. It intentionally only serves
// the BuildWorld UI bundle; project builds and their workspaces remain owned by
// the build runner.
type LiveReload struct {
	root        string
	watcher     *fsnotify.Watcher
	mu          sync.Mutex
	subscribers map[chan reloadEvent]struct{}
	done        chan struct{}
	stopOnce    sync.Once
}

type reloadEvent struct {
	Path string `json:"path"`
}

func NewLiveReload(root string) (*LiveReload, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve static directory: %w", err)
	}
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("create static asset watcher: %w", err)
	}
	live := &LiveReload{
		root:        absRoot,
		watcher:     watcher,
		subscribers: make(map[chan reloadEvent]struct{}),
		done:        make(chan struct{}),
	}
	if err := live.watchTree(absRoot); err != nil {
		watcher.Close()
		return nil, err
	}
	go live.loop()
	return live, nil
}

func (l *LiveReload) watchTree(root string) error {
	return filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || !entry.IsDir() {
			return err
		}
		if err := l.watcher.Add(path); err != nil {
			return fmt.Errorf("watch static directory %s: %w", path, err)
		}
		return nil
	})
}

func (l *LiveReload) loop() {
	for {
		select {
		case <-l.done:
			return
		case event, ok := <-l.watcher.Events:
			if !ok {
				return
			}
			if event.Op&fsnotify.Create != 0 {
				if info, err := os.Stat(event.Name); err == nil && info.IsDir() {
					_ = l.watchTree(event.Name)
				}
			}
			if isReloadableAsset(event.Name) && event.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Rename|fsnotify.Remove) != 0 {
				l.publish(event.Name)
			}
		case <-l.watcher.Errors:
			// A transient watch error should not terminate browser sessions.
		}
	}
}

func isReloadableAsset(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".html", ".htm", ".css", ".js", ".mjs", ".cjs", ".jsx", ".ts", ".tsx", ".json":
		return true
	default:
		return false
	}
}

func (l *LiveReload) publish(path string) {
	relative, err := filepath.Rel(l.root, path)
	if err != nil || strings.HasPrefix(relative, "..") {
		return
	}
	event := reloadEvent{Path: "/" + filepath.ToSlash(relative)}
	l.mu.Lock()
	defer l.mu.Unlock()
	for subscriber := range l.subscribers {
		select {
		case subscriber <- event:
		default:
		}
	}
}

func (l *LiveReload) subscribe() (chan reloadEvent, func()) {
	channel := make(chan reloadEvent, 1)
	l.mu.Lock()
	l.subscribers[channel] = struct{}{}
	l.mu.Unlock()
	return channel, func() {
		l.mu.Lock()
		delete(l.subscribers, channel)
		l.mu.Unlock()
	}
}

func (l *LiveReload) ServeEvents(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming is unavailable", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, "retry: 1000\n\n")
	flusher.Flush()

	channel, unsubscribe := l.subscribe()
	defer unsubscribe()
	for {
		select {
		case <-r.Context().Done():
			return
		case event := <-channel:
			data, _ := json.Marshal(event)
			fmt.Fprintf(w, "event: reload\ndata: %s\n\n", data)
			flusher.Flush()
		}
	}
}

func (l *LiveReload) ServeScript(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	fmt.Fprint(w, `(function () {
  var source = new EventSource('/__buildworld/livereload');
  source.addEventListener('reload', function (event) {
    var change = JSON.parse(event.data || '{}');
    if (/\.css$/i.test(change.path || '')) {
      document.querySelectorAll('link[rel="stylesheet"]').forEach(function (link) {
        var url = new URL(link.href, window.location.href);
        url.searchParams.set('buildworld_reload', Date.now().toString());
        link.href = url.toString();
      });
      return;
    }
    window.location.reload();
  });
})();`)
}

func (l *LiveReload) Stop() {
	l.stopOnce.Do(func() {
		close(l.done)
		_ = l.watcher.Close()
		l.mu.Lock()
		l.subscribers = make(map[chan reloadEvent]struct{})
		l.mu.Unlock()
	})
}

// publishAfter is deliberately small and test-friendly: editors commonly emit
// a burst of writes while replacing a generated asset.
func (l *LiveReload) publishAfter(path string) {
	time.AfterFunc(80*time.Millisecond, func() { l.publish(path) })
}
