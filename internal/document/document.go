package document

import (
	"fmt"
	"sync"
	"unicode/utf8"

	"github.com/NirajNair/syncdoc/internal/logger"
	"github.com/sergi/go-diff/diffmatchpatch"
	y "github.com/skyterra/y-crdt"
)

const DefaultTemplate = `Welcome to SyncDoc!
Type away and save the file to watch the magic happen...
`

// Document manages the CRDT state and sync logic
type Document struct {
	mu               sync.Mutex
	doc              *y.Doc
	ytext            *y.YText
	lastKnownContent string
	pendingChanges   chan func() // Queue that holds local changes during remote writes
	lastStateVector  []byte      // Track state for incremental sync
	logger           *logger.Logger
}

// Creates a new Document with the given initial content
func NewDocument(logger *logger.Logger, initialContent string) (*Document, error) {
	// Create doc with GC disabled to avoid nil pointer issues in transaction cleanup
	doc := y.NewDoc(generateGUID(), false, nil, nil, false)
	ytext := doc.GetText("content")

	// Insert initial content via transaction (YText has no Set method)
	if initialContent != "" {
		doc.Transact(func(trans *y.Transaction) {
			ytext.Insert(0, initialContent, nil)
		}, nil)
	}

	// Initialize state vector for tracking incremental updates
	stateVector := y.EncodeStateVector(doc, nil, y.NewUpdateEncoderV1())

	return &Document{
		doc:              doc,
		ytext:            ytext,
		lastKnownContent: initialContent,
		pendingChanges:   make(chan func(), 10),
		lastStateVector:  stateVector,
		logger:           logger,
	}, nil
}

func (d *Document) getContentLocked() string {
	return d.ytext.ToString()
}

// Returns current text content from CRDT
func (d *Document) GetContent() (string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.getContentLocked(), nil
}

// Creates a complete CRDT update of current document state.
// Used for initial sync to avoid the lastKnownContent check in ApplyLocalChange
func (d *Document) GenerateFullUpdate() []byte {
	d.mu.Lock()
	defer d.mu.Unlock()
	// Generate full document update (nil state vector = all changes)
	update := y.EncodeStateAsUpdate(d.doc, nil)
	return update
}

// Applies local file changes to CRDT after diffings.
// Returns sync data to send (nil if no changes)
func (d *Document) ApplyLocalChange(newContent string) ([]byte, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if newContent == d.lastKnownContent {
		return nil, nil
	}

	// Pre-validate diff before transaction
	ops, err := d.calculateDiffOps(newContent)
	if err != nil {
		return nil, fmt.Errorf("error calculating diff: %w", err)
	}

	// Apply diff operations inside transaction
	d.doc.Transact(func(trans *y.Transaction) {
		for _, op := range ops {
			switch op.kind {
			case "delete":
				d.ytext.Delete(op.index, op.length)
			case "insert":
				d.ytext.Insert(op.index, op.text, nil)
			}
		}
	}, nil)

	d.lastKnownContent = newContent

	// Generate full document update (nil state vector = all changes)
	// This is needed for compatibility with different documents
	update := y.EncodeStateAsUpdate(d.doc, nil)

	return update, nil
}

// Merges remote sync data into the CRDT
// Returns pointer to new content to write to file (nil if no change needed)
// Returns nil pointer if content hasn't changed, returns pointer to empty string if content became empty
func (d *Document) ApplyRemoteChange(syncData []byte) (*string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if len(syncData) == 0 {
		return nil, nil
	}

	// Apply update to existing doc (merges, doesn't replace)
	d.doc.Transact(func(trans *y.Transaction) {
		y.ApplyUpdate(d.doc, syncData, nil)
	}, nil)

	newContent := d.getContentLocked()

	// Only return content if it actually changed from what we last sent to remote
	if newContent == d.lastKnownContent {
		return nil, nil
	}

	d.lastKnownContent = newContent
	d.lastStateVector = y.EncodeStateVector(d.doc, nil, y.NewUpdateEncoderV1())

	// Return pointer to new content (including empty string)
	return &newContent, nil
}

// Queues a local change function to be processed later.
// Used when in the middle of a remote write
func (d *Document) QueueLocalChange(fn func()) {
	select {
	case d.pendingChanges <- fn:
	default:
		d.logger.Debug("Change queue full, dropping local change")
	}
}

// Processes any queued local changes
func (d *Document) ProcessPendingChanges() {
	for {
		select {
		case fn := <-d.pendingChanges:
			fn()
		default:
			return
		}
	}
}

// diffOp represents a single diff operation
type diffOp struct {
	kind   string // "insert", "delete", "equal"
	index  int
	text   string
	length int
}

// Calculates the diff operations without applying them
// This allows pre-validation before entering a transaction
func (d *Document) calculateDiffOps(newString string) ([]diffOp, error) {
	dmp := diffmatchpatch.New()

	diffs := dmp.DiffMain(d.lastKnownContent, newString, false)
	diffs = dmp.DiffCleanupSemantic(diffs)

	var ops []diffOp
	cursor := 0

	for _, diff := range diffs {
		// Use RuneCount for proper Unicode handling
		charCount := utf8.RuneCountInString(diff.Text)

		switch diff.Type {
		case diffmatchpatch.DiffEqual:
			cursor += charCount

		case diffmatchpatch.DiffDelete:
			ops = append(ops, diffOp{kind: "delete", index: cursor, length: charCount})

		case diffmatchpatch.DiffInsert:
			ops = append(ops, diffOp{kind: "insert", index: cursor, text: diff.Text})
			cursor += charCount
		}
	}

	return ops, nil
}

// Generates a simple unique identifier for the document
// In production, consider using a proper UUID library
func generateGUID() string {
	return fmt.Sprintf("syncdoc-%d", y.GenerateNewClientID())
}
