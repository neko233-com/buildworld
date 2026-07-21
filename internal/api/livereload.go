package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/fsnotify/fsnotify"
)

// LiveReload watches a disk-backed web bundle and tells connected browser
// sessions when a frontend source asset changes. It intentionally only serves
// the BuildWorld UI bundle; project builds and their workspaces remain owned by
// the build runner.
type LiveReload struct {
	root         string
	watcher      *fsnotify.Watcher
	mu           sync.Mutex
	subscribers  map[chan reloadEvent]struct{}
	assets       map[string]assetState
	lastChange   map[string]publishedAssetChange
	done         chan struct{}
	stopped      chan struct{}
	stopOnce     sync.Once
	polls        atomic.Uint64
	nativeEvents atomic.Uint64
	published    atomic.Uint64
	assetCount   atomic.Uint64
	running      atomic.Bool
	heartbeat    time.Duration
}

type reloadEvent struct {
	Path string `json:"path"`
}

// LiveReloadStatus intentionally exposes only aggregate diagnostics. In
// particular, it never returns the watched directory or individual asset
// names, which keeps deployment paths private while still showing whether the
// native watcher and polling fallback are active.
type LiveReloadStatus struct {
	Running      bool   `json:"running"`
	Polls        uint64 `json:"polls"`
	NativeEvents uint64 `json:"native_events"`
	Published    uint64 `json:"published"`
	Assets       uint64 `json:"assets"`
	Subscribers  int    `json:"subscribers"`
}

type assetState struct {
	modTime int64
	size    int64
}

type assetChange struct {
	exists bool
	state  assetState
}

type publishedAssetChange struct {
	change assetChange
	at     time.Time
}

const (
	liveReloadPollInterval    = 350 * time.Millisecond
	liveReloadDuplicateWindow = 2 * liveReloadPollInterval
	liveReloadHeartbeat       = 15 * time.Second
)

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
		lastChange:  make(map[string]publishedAssetChange),
		done:        make(chan struct{}),
		stopped:     make(chan struct{}),
		heartbeat:   liveReloadHeartbeat,
	}
	if err := live.watchTree(absRoot); err != nil {
		watcher.Close()
		return nil, err
	}
	live.assets, err = snapshotReloadableAssets(absRoot)
	if err != nil {
		watcher.Close()
		return nil, fmt.Errorf("snapshot static assets: %w", err)
	}
	live.assetCount.Store(uint64(len(live.assets)))
	live.running.Store(true)
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
	defer func() {
		l.running.Store(false)
		close(l.stopped)
	}()
	ticker := time.NewTicker(liveReloadPollInterval)
	defer ticker.Stop()

	// Keep polling even if the native watcher closes or becomes unreliable.
	// This is required on filesystems where writes to an existing bundle do not
	// consistently surface through fsnotify (notably some macOS deployments).
	events := l.watcher.Events
	errors := l.watcher.Errors
	for {
		select {
		case <-l.done:
			return
		case event, ok := <-events:
			if !ok {
				events = nil
				continue
			}
			l.handleWatchEvent(event)
		case _, ok := <-errors:
			if !ok {
				errors = nil
			}
			// A transient watch error should not terminate browser sessions.
		case <-ticker.C:
			l.pollAssets()
		}
	}
}

func (l *LiveReload) handleWatchEvent(event fsnotify.Event) {
	l.nativeEvents.Add(1)
	path := filepath.Clean(event.Name)
	if event.Op&fsnotify.Create != 0 {
		if info, err := os.Stat(path); err == nil && info.IsDir() {
			_ = l.watchTree(path)
			return
		}
	}
	if !isReloadableAsset(path) || event.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Rename|fsnotify.Remove) == 0 {
		return
	}

	if event.Op&(fsnotify.Rename|fsnotify.Remove) != 0 {
		delete(l.assets, path)
		l.assetCount.Store(uint64(len(l.assets)))
		l.publishDetected(path, assetChange{})
		return
	}

	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			delete(l.assets, path)
			l.assetCount.Store(uint64(len(l.assets)))
			l.publishDetected(path, assetChange{})
		}
		return
	}
	if info.IsDir() {
		return
	}
	state := assetState{modTime: info.ModTime().UnixNano(), size: info.Size()}
	l.assets[path] = state
	l.assetCount.Store(uint64(len(l.assets)))
	l.publishDetected(path, assetChange{exists: true, state: state})
}

func (l *LiveReload) pollAssets() {
	l.polls.Add(1)
	current, err := snapshotReloadableAssets(l.root)
	if err != nil {
		// A partial scan must not look like a mass deletion. Retry next tick.
		return
	}
	for path, state := range current {
		previous, exists := l.assets[path]
		if !exists || previous != state {
			l.publishDetected(path, assetChange{exists: true, state: state})
		}
	}
	for path := range l.assets {
		if _, exists := current[path]; !exists {
			l.publishDetected(path, assetChange{})
		}
	}
	l.assets = current
	l.assetCount.Store(uint64(len(current)))
}

func snapshotReloadableAssets(root string) (map[string]assetState, error) {
	assets := make(map[string]assetState)
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if entry.IsDir() || !isReloadableAsset(path) {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		assets[filepath.Clean(path)] = assetState{
			modTime: info.ModTime().UnixNano(),
			size:    info.Size(),
		}
		return nil
	})
	return assets, err
}

func (l *LiveReload) publishDetected(path string, change assetChange) {
	now := time.Now()
	if previous, exists := l.lastChange[path]; exists && previous.change == change && now.Sub(previous.at) < liveReloadDuplicateWindow {
		return
	}
	l.lastChange[path] = publishedAssetChange{change: change, at: now}
	l.publish(path)
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
	l.published.Add(1)
	l.mu.Lock()
	defer l.mu.Unlock()
	for subscriber := range l.subscribers {
		select {
		case subscriber <- event:
		default:
		}
	}
}

// Status returns a race-free aggregate snapshot suitable for production
// diagnostics. Subscriber cardinality shares the same lock as subscription
// changes; all counters used by the watcher goroutine are atomic.
func (l *LiveReload) Status() LiveReloadStatus {
	l.mu.Lock()
	subscribers := len(l.subscribers)
	l.mu.Unlock()
	return LiveReloadStatus{
		Running:      l.running.Load(),
		Polls:        l.polls.Load(),
		NativeEvents: l.nativeEvents.Load(),
		Published:    l.published.Load(),
		Assets:       l.assetCount.Load(),
		Subscribers:  subscribers,
	}
}

// ServeStatus reports LiveReload health without exposing filesystem paths.
func (l *LiveReload) ServeStatus(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, l.Status())
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
	if _, ok := w.(http.Flusher); !ok {
		http.Error(w, "streaming is unavailable", http.StatusInternalServerError)
		return
	}
	controller := http.NewResponseController(w)
	// SSE responses outlive the ordinary API write timeout. Clear only this
	// response's deadline; regular request/response handlers keep the server's
	// bounded WriteTimeout.
	if err := controller.SetWriteDeadline(time.Time{}); err != nil && !errors.Is(err, http.ErrNotSupported) {
		http.Error(w, "streaming is unavailable", http.StatusInternalServerError)
		return
	}

	// Subscribe before the initial flush so a change cannot land in the small
	// gap between the browser receiving the retry directive and registration.
	channel, unsubscribe := l.subscribe()
	defer unsubscribe()
	heartbeatInterval := l.heartbeat
	if heartbeatInterval <= 0 {
		heartbeatInterval = liveReloadHeartbeat
	}
	heartbeat := time.NewTicker(heartbeatInterval)
	defer heartbeat.Stop()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	if _, err := fmt.Fprint(w, "retry: 1000\n\n"); err != nil {
		return
	}
	if err := controller.Flush(); err != nil {
		return
	}
	for {
		select {
		case <-r.Context().Done():
			return
		case <-l.done:
			return
		case event := <-channel:
			data, _ := json.Marshal(event)
			if _, err := fmt.Fprintf(w, "event: reload\ndata: %s\n\n", data); err != nil {
				return
			}
			if err := controller.Flush(); err != nil {
				return
			}
		case <-heartbeat.C:
			if _, err := fmt.Fprint(w, ": buildworld heartbeat\n\n"); err != nil {
				return
			}
			if err := controller.Flush(); err != nil {
				return
			}
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
		if l.done != nil {
			close(l.done)
		}
		if l.watcher != nil {
			_ = l.watcher.Close()
		}
		if l.stopped != nil {
			<-l.stopped
		}
		l.mu.Lock()
		l.subscribers = make(map[chan reloadEvent]struct{})
		l.mu.Unlock()
	})
}
