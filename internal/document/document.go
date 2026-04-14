package document

import (
	"fmt"
	"unicode/utf8"

	"github.com/sergi/go-diff/diffmatchpatch"
	y "github.com/skyterra/y-crdt"
)

const DefaultTemplate = `Welcome to SyncDoc!
Type away and save the file to watch the magic happen...
`

// Document manages the CRDT state and sync logic
type Document struct {
	doc              *y.Doc
	ytext            *y.YText
	lastKnownContent string
	pendingChanges   chan func() // Queue that holds local changes during remote writes
	lastStateVector  []byte      // Track state for incremental sync
}

// Creates a new Document with initial content via a template
func NewDocument() (*Document, error) {
	// Create doc with GC disabled to avoid nil pointer issues in transaction cleanup
	doc := y.NewDoc(generateGUID(), false, nil, nil, false)
	ytext := doc.GetText("content")

	// Insert initial content via transaction (YText has no Set method)
	doc.Transact(func(trans *y.Transaction) {
		ytext.Insert(0, DefaultTemplate, nil)
	}, nil)

	// Initialize state vector for tracking incremental updates
	stateVector := y.EncodeStateVector(doc, nil, y.NewUpdateEncoderV1())

	return &Document{
		doc:              doc,
		ytext:            ytext,
		lastKnownContent: DefaultTemplate,
		pendingChanges:   make(chan func(), 10),
		lastStateVector:  stateVector,
	}, nil
}

// Returns current text content from CRDT
func (d *Document) GetContent() (string, error) {
	return d.ytext.ToString(), nil
}

// Applies local file changes to CRDT after diffing
// Returns sync data to send (nil if no changes)
func (d *Document) ApplyLocalChange(newContent string) ([]byte, error) {
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
// Returns the new content to write to file (nil if no change needed)
func (d *Document) ApplyRemoteChange(syncData []byte) (string, error) {
	if len(syncData) == 0 {
		return "", nil
	}

	oldContent, _ := d.GetContent()

	// Apply update to existing doc (merges, doesn't replace)
	d.doc.Transact(func(trans *y.Transaction) {
		y.ApplyUpdate(d.doc, syncData, nil)
	}, nil)

	newContent, err := d.GetContent()
	if err != nil {
		return "", fmt.Errorf("error getting content after remote changes: %w", err)
	}

	// Only return content if it actually changed
	if newContent == oldContent || newContent == d.lastKnownContent {
		return "", nil
	}

	d.lastKnownContent = newContent
	d.lastStateVector = y.EncodeStateVector(d.doc, nil, y.NewUpdateEncoderV1())

	return newContent, nil
}

// Queues a local change function to be processed later.
// Used when in the middle of a remote write
func (d *Document) QueueLocalChange(fn func()) {
	select {
	case d.pendingChanges <- fn:
	default:
		fmt.Println("Warning: change queue full")
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
