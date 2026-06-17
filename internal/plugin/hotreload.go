package plugin

import (
	"log"
	"path/filepath"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

type HotReload struct {
	watcher  *fsnotify.Watcher
	loader   *Loader
	stop     chan struct{}
	mu       sync.Mutex
	timers   map[string]*time.Timer
}

func NewHotReload(loader *Loader) (*HotReload, error) {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	return &HotReload{
		watcher: w,
		loader:  loader,
		stop:    make(chan struct{}),
		timers:  make(map[string]*time.Timer),
	}, nil
}

func (h *HotReload) Watch(pluginName string) {
	pluginPath := filepath.Join(h.loader.path, pluginName)
	h.watcher.Add(pluginPath)
	go h.loop()
}

func (h *HotReload) loop() {
	for {
		select {
		case event, ok := <-h.watcher.Events:
			if !ok {
				return
			}
			if event.Op&fsnotify.Write == fsnotify.Write {
				pluginName := filepath.Base(filepath.Dir(event.Name))
				log.Printf("Plugin %s changed, reloading...", pluginName)

				h.mu.Lock()
				if t, exists := h.timers[pluginName]; exists {
					t.Stop()
				}
				h.timers[pluginName] = time.AfterFunc(100*time.Millisecond, func() {
					h.loader.Unload(pluginName)
					h.loader.Load(pluginName)
				})
				h.mu.Unlock()
			}
		case <-h.stop:
			return
		}
	}
}

func (h *HotReload) Stop() {
	close(h.stop)
	h.watcher.Close()
	h.mu.Lock()
	for _, t := range h.timers {
		t.Stop()
	}
	h.mu.Unlock()
}
