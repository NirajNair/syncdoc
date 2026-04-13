package watcher

import (
	"fmt"
	"os"
	"sync"

	"github.com/fsnotify/fsnotify"
)

// Monitors a file for changes
type Watcher struct {
	mu sync.RWMutex

	watcher       *fsnotify.Watcher
	path          string
	isRemoteWrite bool
	onChange      func([]byte)
	stopChan      chan struct{}
}

// Create a new file watcher
func NewWatcher() (*Watcher, error) {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("Error creating file watcher: %v", err.Error())
	}
	return &Watcher{
		watcher:       w,
		isRemoteWrite: false,
		stopChan:      make(chan struct{}),
	}, nil
}

// Watch starts watching the file at the given path and calls onChange with file contents when local writes are detected
func (w *Watcher) Watch(path string, onChange func([]byte)) error {
	w.mu.Lock()
	w.path = path
	w.onChange = onChange
	w.mu.Unlock()

	// Add file to watcher
	if err := w.watcher.Add(path); err != nil {
		return fmt.Errorf("error watching file %s: %v", path, err)
	}

	// Start watching in a goroutine
	go w.watchLoop()

	return nil
}

// Continuously listens for fsnotify events and processes it.
func (w *Watcher) watchLoop() {
	for {
		select {
		case event, ok := <-w.watcher.Events:
			if !ok {
				return
			}

			if !event.Has(fsnotify.Write) {
				continue
			}

			w.mu.RLock()
			isRemote := w.isRemoteWrite
			w.mu.RUnlock()

			if isRemote {
				w.mu.Lock()
				w.isRemoteWrite = false
				w.mu.Unlock()
				continue
			}

			data, err := os.ReadFile(w.path)
			if err != nil {
				fmt.Printf("Error reading file: %v\n", err)
				continue
			}

			if w.onChange != nil {
				w.onChange(data)
			}

		case err, ok := <-w.watcher.Errors:
			if !ok {
				return
			}
			fmt.Printf("Watcher error: %v\n", err)

		case <-w.stopChan:
			return
		}
	}
}

// Writes data to the file and suppresses the watch event
func (w *Watcher) WriteRemote(data []byte) error {
	w.mu.Lock()
	w.isRemoteWrite = true
	w.mu.Unlock()

	if err := os.WriteFile(w.path, data, 0644); err != nil {
		w.mu.Lock()
		w.isRemoteWrite = false
		w.mu.Unlock()
		return fmt.Errorf("error writing file: %v", err)
	}

	return nil
}

// Writes data to the file without suppression (for local writes)
func (w *Watcher) Write(data []byte) error {
	return os.WriteFile(w.path, data, 0644)
}

// Stops the watcher and cleans up
func (w *Watcher) Close() error {
	close(w.stopChan)

	err := w.watcher.Close()
	if err != nil {
		return fmt.Errorf("Error closing file watcher: %v", err.Error())
	}
	return nil
}
