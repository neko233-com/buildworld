package config

import (
	"log"
	"time"

	"github.com/fsnotify/fsnotify"
)

type Watcher struct {
	watcher *fsnotify.Watcher
	stop    chan struct{}
}

func Watch(path string, onChange func(*Config)) (*Watcher, error) {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	if err := w.Add(path); err != nil {
		_ = w.Close()
		return nil, err
	}

	watcher := &Watcher{
		watcher: w,
		stop:    make(chan struct{}),
	}

	go watcher.loop(path, onChange)

	return watcher, nil
}

func (w *Watcher) loop(path string, onChange func(*Config)) {
	var debounce *time.Timer

	for {
		select {
		case event, ok := <-w.watcher.Events:
			if !ok {
				return
			}
			if event.Op&fsnotify.Write == fsnotify.Write {
				if debounce != nil {
					debounce.Stop()
				}
				debounce = time.AfterFunc(100*time.Millisecond, func() {
					cfg, err := Load(path)
					if err != nil {
						log.Printf("config reload failed: %v", err)
						return
					}
					log.Printf("config reloaded from %s", path)
					onChange(cfg)
				})
			}
		case err, ok := <-w.watcher.Errors:
			if !ok {
				return
			}
			log.Printf("config watcher error: %v", err)
		case <-w.stop:
			return
		}
	}
}

func (w *Watcher) Stop() {
	close(w.stop)
	_ = w.watcher.Close()
}
