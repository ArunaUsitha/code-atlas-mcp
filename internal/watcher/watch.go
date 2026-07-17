package watcher

import (
	"log"

	"github.com/fsnotify/fsnotify"
)

type CodeWatcher struct {
	watcher *fsnotify.Watcher
}

func NewCodeWatcher() (*CodeWatcher, error) {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	return &CodeWatcher{watcher: w}, nil
}

// WatchWorkspace registers files in the workspace directories
func (cw *CodeWatcher) WatchWorkspace(rootPath string, onFileChange func(filePath string)) {
	go func() {
		for {
			select {
			case event, ok := <-cw.watcher.Events:
				if !ok {
					return
				}
				// Filter modifications
				if event.Has(fsnotify.Write) {
					onFileChange(event.Name) // Trigger AST parser callback
				}
			case err, ok := <-cw.watcher.Errors:
				if !ok {
					return
				}
				log.Println("watcher error:", err)
			}
		}
	}()

	err := cw.watcher.Add(rootPath)
	if err != nil {
		log.Println("Error adding path to watcher:", err)
	}
}

func (cw *CodeWatcher) Close() {
	if cw.watcher != nil {
		cw.watcher.Close()
	}
}
