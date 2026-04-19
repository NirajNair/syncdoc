package document

// DocumentInterface defines the contract for CRDT document operations.
// This enables dependency injection for testing.
type DocumentInterface interface {
	ApplyLocalChange(newContent string) ([]byte, error)
	ApplyRemoteChange(syncData []byte) (*string, error)
	GenerateFullUpdate() []byte
}
