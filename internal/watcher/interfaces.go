package watcher

// WatcherInterface defines the contract for file watching operations.
// This enables dependency injection for testing.
type WatcherInterface interface {
	Watch(path string, onChange func([]byte)) error
	WriteRemote(data []byte) error
	Close() error
}
