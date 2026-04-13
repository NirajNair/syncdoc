package document

import (
	"fmt"
	"unicode/utf8"

	"github.com/automerge/automerge-go"
	"github.com/sergi/go-diff/diffmatchpatch"
)

const defaultTemplate = `Welcome to SyncDoc!
Type away and save the file to watch the magic happen...
`

// Document manages the CRDT state and sync logic
type Document struct {
	doc              *automerge.Doc
	text             *automerge.Text
	lastKnownContent string
	pendingChanges   chan func() // Queue that holds local changes during remote writes
}

// Creates a new Document with initial content via a tempalte
func NewDocument() (*Document, error) {
	doc := automerge.New()
	text := doc.Path("content").Text()
	if err := text.Set(defaultTemplate); err != nil {
		return nil, fmt.Errorf("Error creating DRDT document: %v", err.Error())
	}

	return &Document{
		doc:              doc,
		text:             text,
		lastKnownContent: defaultTemplate,
		pendingChanges:   make(chan func(), 10),
	}, nil
}

// Returns current text content from CRDT
func (d *Document) GetContent() (string, error) {
	return d.text.Get()
}

// Applies local file changes to CRDT after diffing
// Returns sync data to send (nil if no changes)
func (d *Document) ApplyLocalChange(newContent string) ([]byte, error) {
	if newContent == d.lastKnownContent {
		return nil, nil
	}

	if err := d.applyDiff(newContent); err != nil {
		return nil, fmt.Errorf("Error applying diff: %v", err.Error())
	}

	d.lastKnownContent = newContent

	return d.doc.Save(), nil
}

// Merges remote sync data into the CRDT
// Returns the new content to write to file (nil if no change needed)
func (d *Document) ApplyRemoteChange(syncData []byte) (string, error) {
	// Load remote changes
	doc, err := automerge.Load(syncData)
	if err != nil {
		return "", fmt.Errorf("Error merging remote changes: %w", err)
	}
	d.doc = doc
	d.text = doc.Path("content").Text()

	newContent, err := d.text.Get()
	if err != nil {
		return "", fmt.Errorf("Error getting content after remote changes are merged: %w", err)
	}

	if newContent == d.lastKnownContent {
		return "", nil
	}

	d.lastKnownContent = newContent

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

// Proocesses any queued local changes
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

// Calculates diffs and applies them to CRDT text object
func (d *Document) applyDiff(newString string) error {
	dmp := diffmatchpatch.New()

	diffs := dmp.DiffMain(d.lastKnownContent, newString, false)
	diffs = dmp.DiffCleanupSemantic(diffs)

	// Cursor tracks position in the CRDT
	cursor := 0

	for _, diff := range diffs {
		// Use RuneCount for proper Unicode handling
		charCount := utf8.RuneCountInString(diff.Text)

		switch diff.Type {
		case diffmatchpatch.DiffEqual:
			cursor += charCount

		case diffmatchpatch.DiffDelete:
			if err := d.text.Delete(cursor, charCount); err != nil {
				return fmt.Errorf("failed to delete: %w", err)
			}

		case diffmatchpatch.DiffInsert:
			if err := d.text.Insert(cursor, diff.Text); err != nil {
				return fmt.Errorf("failed to insert: %w", err)
			}
			cursor += charCount
		}
	}

	return nil
}
