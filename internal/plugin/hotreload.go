package plugin

import (
	"log"
	"path/filepath"
	"time"

	"github.com/fsnotify/fsnotify"
)

type HotReload struct {
	watcher *fsnotify.Watcher
	loader  *Loader
	stop    chan struct{}
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
	}, nil
}

func (h *HotReload) Watch(pluginName string) {
	pluginPath := filepath.Join(h.loader.path, pluginName)
	h.watcher.Add(pluginPath)
	go h.loop()
}

func (h *HotReload) loop() {
	var debounce *time.Timer

	for {
		select {
		case event, ok := <-h.watcher.Events:
			if !ok {
				return
			}
			if event.Op&fsnotify.Write == fsnotify.Write {
				pluginName := filepath.Base(filepath.Dir(event.Name))
				log.Printf("Plugin %s changed, reloading...", pluginName)

				if debounce != nil {
					debounce.Stop()
				}
				debounce = time.AfterFunc(100*time.Millisecond, func() {
					h.loader.Unload(pluginName)
					h.loader.Load(pluginName)
				})
			}
		case <-h.stop:
			return
		}
	}
}

func (h *HotReload) Stop() {
	close(h.stop)
	h.watcher.Close()
}
