package watcher

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/NirajNair/syncdoc/internal/logger"
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
	lastHash      string
	logger        *logger.Logger
}

// Create a new file watcher
func NewWatcher(logger *logger.Logger) (*Watcher, error) {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("Error creating file watcher: %v", err.Error())
	}
	return &Watcher{
		watcher:       w,
		isRemoteWrite: false,
		stopChan:      make(chan struct{}),
		logger:        logger,
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

// calculateHash computes SHA-256 hash of data
func calculateHash(data []byte) string {
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}

// Continuously listens for fsnotify events and processes it.
func (w *Watcher) watchLoop() {
	for {
		select {
		case event, ok := <-w.watcher.Events:
			if !ok {
				return
			}

			// Handle Write, Rename, and Create events
			if !event.Has(fsnotify.Write) &&
				!event.Has(fsnotify.Rename) &&
				!event.Has(fsnotify.Create) {
				continue
			}

			// After detecting rename, re-add file to watcher (inode changed)
			if event.Has(fsnotify.Rename) {
				// Small delay to ensure rename completes
				time.Sleep(50 * time.Millisecond)
				if err := w.watcher.Add(w.path); err != nil {
					w.logger.Debug("Error re-watching file after rename", "error", err)
					continue
				}
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
				w.logger.Debug("Error reading file", "error", err)
				continue
			}

			// Hash-based deduplication: only trigger if content actually changed
			currentHash := calculateHash(data)
			w.mu.Lock()
			if currentHash == w.lastHash {
				w.mu.Unlock()
				w.logger.Debug("File content unchanged, skipping sync")
				continue
			}
			w.lastHash = currentHash
			w.mu.Unlock()

			if w.onChange != nil {
				w.onChange(data)
			}

		case err, ok := <-w.watcher.Errors:
			if !ok {
				return
			}
			w.logger.Debug("Watcher error", "error", err)

		case <-w.stopChan:
			return
		}
	}
}

// Writes data to the file and suppresses the watch event
func (w *Watcher) WriteRemote(data []byte) error {
	w.mu.Lock()
	w.isRemoteWrite = true
	// Update lastHash so we don't trigger on our own write
	w.lastHash = calculateHash(data)
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
