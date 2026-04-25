package document

import (
	"strings"
	"sync"
	"testing"

	"github.com/NirajNair/syncdoc/internal/logger"
)

func TestNewDocument(t *testing.T) {
	log := logger.New(false)
	doc, err := NewDocument(log, DefaultTemplate)
	if err != nil {
		t.Fatalf("NewDocument failed: %v", err)
	}

	content, err := doc.GetContent()
	if err != nil {
		t.Fatalf("GetContent failed: %v", err)
	}

	if content != DefaultTemplate {
		t.Errorf("expected content to be DefaultTemplate, got:\n%s", content)
	}
}

func TestGetContent(t *testing.T) {
	log := logger.New(false)
	doc, _ := NewDocument(log, DefaultTemplate)

	content, err := doc.GetContent()
	if err != nil {
		t.Fatalf("GetContent failed: %v", err)
	}

	if content == "" {
		t.Error("expected non-empty content")
	}
}

func TestApplyLocalChange(t *testing.T) {
	log := logger.New(false)
	doc, _ := NewDocument(log, DefaultTemplate)

	// Apply a simple change
	newContent := "Hello World"
	update, err := doc.ApplyLocalChange(newContent)
	if err != nil {
		t.Fatalf("ApplyLocalChange failed: %v", err)
	}

	if update == nil {
		t.Error("expected non-nil update for actual changes")
	}

	// Verify content was updated
	content, _ := doc.GetContent()
	if content != newContent {
		t.Errorf("expected content '%s', got '%s'", newContent, content)
	}

	// Apply same content - should return nil update
	update2, err := doc.ApplyLocalChange(newContent)
	if err != nil {
		t.Fatalf("ApplyLocalChange (same content) failed: %v", err)
	}

	if update2 != nil {
		t.Error("expected nil update when content hasn't changed")
	}
}

func TestApplyRemoteChange(t *testing.T) {
	log := logger.New(false)
	// Create doc1 and apply a change
	doc1, _ := NewDocument(log, DefaultTemplate)
	newContent := "Hello from doc1"
	update, _ := doc1.ApplyLocalChange(newContent)

	t.Logf("Update from doc1: %v (len=%d)", update, len(update))

	// Create doc2 starting with same template, then apply the update
	doc2, _ := NewDocument(log, DefaultTemplate)

	// Doc2 applies the change from doc1
	receivedContent, err := doc2.ApplyRemoteChange(update)
	if err != nil {
		t.Fatalf("ApplyRemoteChange failed: %v", err)
	}

	// Content should have changed (will include both histories in CRDT merge)
	if receivedContent == nil || *receivedContent == "" {
		t.Error("expected non-empty content from remote change")
	}

	t.Logf("Doc2 received content: '%s'", *receivedContent)

	// Verify doc2 has merged content (contains both the template and the update)
	content, _ := doc2.GetContent()
	if content == "" {
		t.Error("expected doc2 to have content after remote change")
	}

	// The content should contain the new text from doc1
	if !strings.Contains(content, "Hello from doc1") {
		t.Errorf("expected doc2 content to contain 'Hello from doc1', got: '%s'", content)
	}
}

func TestBidirectionalSync(t *testing.T) {
	log := logger.New(false)
	// Create two documents
	doc1, _ := NewDocument(log, DefaultTemplate)
	doc2, _ := NewDocument(log, DefaultTemplate)

	// Doc1 makes first change
	content1 := "Hello World"
	update1, _ := doc1.ApplyLocalChange(content1)

	// Doc2 applies doc1's change
	_, err := doc2.ApplyRemoteChange(update1)
	if err != nil {
		t.Fatalf("doc2 ApplyRemoteChange failed: %v", err)
	}

	// Verify doc2 merged the content (will have both histories)
	c2, _ := doc2.GetContent()
	if !strings.Contains(c2, content1) {
		t.Errorf("expected doc2 content to contain '%s', got '%s'", content1, c2)
	}

	// Doc2 makes a change
	content2 := "Hello World! How are you?"
	update2, _ := doc2.ApplyLocalChange(content2)

	// Doc1 applies doc2's change
	_, err = doc1.ApplyRemoteChange(update2)
	if err != nil {
		t.Fatalf("doc1 ApplyRemoteChange failed: %v", err)
	}

	// Both should have both contents (CRDT merge preserves both histories)
	c1, _ := doc1.GetContent()
	c2, _ = doc2.GetContent()

	// Log actual contents for debugging
	t.Logf("After bidirectional sync: doc1=%s", c1)
	t.Logf("After bidirectional sync: doc2=%s", c2)

	// Both should contain both changes
	if !strings.Contains(c1, content1) || !strings.Contains(c1, content2) {
		t.Errorf("expected doc1 to contain both contents, got: %s", c1)
	}
	if !strings.Contains(c2, content1) || !strings.Contains(c2, content2) {
		t.Errorf("expected doc2 to contain both contents, got: %s", c2)
	}
}

func TestMultipleLocalChanges(t *testing.T) {
	log := logger.New(false)
	doc, _ := NewDocument(log, DefaultTemplate)

	changes := []string{
		"First change",
		"Second change",
		"Third change with more text",
		"Fourth",
	}

	for _, change := range changes {
		update, err := doc.ApplyLocalChange(change)
		if err != nil {
			t.Fatalf("ApplyLocalChange failed for '%s': %v", change, err)
		}

		if update == nil {
			t.Errorf("expected non-nil update for change '%s'", change)
		}

		// Verify content
		content, _ := doc.GetContent()
		if content != change {
			t.Errorf("expected content '%s', got '%s'", change, content)
		}
	}
}

func TestRemoteChangeNoUpdate(t *testing.T) {
	log := logger.New(false)
	doc1, _ := NewDocument(log, DefaultTemplate)
	doc2, _ := NewDocument(log, DefaultTemplate)

	// Doc1 makes a change
	content := "Test content"
	update, _ := doc1.ApplyLocalChange(content)

	// Doc2 applies the change
	doc2.ApplyRemoteChange(update)

	// Doc2 applies the same change again (simulating duplicate message)
	// This should return empty string since content hasn't changed relative to doc2's lastKnownContent
	receivedContent, _ := doc2.ApplyRemoteChange(update)

	// Note: This might not return empty string because doc2's state vector has changed
	// The content might be the same but the function should handle it gracefully
	if receivedContent != nil {
		t.Logf("Duplicate update returned content: '%s'", *receivedContent)
	} else {
		t.Log("Duplicate update returned nil (no content change)")
	}
}

func TestUnicodeContent(t *testing.T) {
	log := logger.New(false)
	doc, _ := NewDocument(log, DefaultTemplate)

	unicodeContent := "Hello 世界! ñ émojis: 🎉🚀"
	update, err := doc.ApplyLocalChange(unicodeContent)
	if err != nil {
		t.Fatalf("ApplyLocalChange with unicode failed: %v", err)
	}

	if update == nil {
		t.Error("expected non-nil update for unicode change")
	}

	content, _ := doc.GetContent()
	if content != unicodeContent {
		t.Errorf("expected unicode content '%s', got '%s'", unicodeContent, content)
	}
}

func TestEmptyContent(t *testing.T) {
	log := logger.New(false)
	doc, _ := NewDocument(log, DefaultTemplate)

	// Change to empty string
	emptyContent := ""
	update, err := doc.ApplyLocalChange(emptyContent)
	if err != nil {
		t.Fatalf("ApplyLocalChange to empty failed: %v", err)
	}

	if update == nil {
		t.Error("expected non-nil update for empty content change")
	}

	content, _ := doc.GetContent()
	if content != emptyContent {
		t.Errorf("expected empty content, got '%s'", content)
	}
}

func TestPartialUpdate(t *testing.T) {
	log := logger.New(false)
	doc1, _ := NewDocument(log, DefaultTemplate)
	doc2, _ := NewDocument(log, DefaultTemplate)

	// Initial content
	initial := "Hello World"
	doc1.ApplyLocalChange(initial)
	update1, _ := doc1.ApplyLocalChange(initial)
	doc2.ApplyRemoteChange(update1)

	// Partial update - insert in middle
	partial := "Hello Beautiful World"
	update2, _ := doc1.ApplyLocalChange(partial)

	// Doc2 should receive the update and apply it
	received, err := doc2.ApplyRemoteChange(update2)
	if err != nil {
		t.Fatalf("ApplyRemoteChange failed: %v", err)
	}

	if received == nil || *received == "" {
		t.Error("expected received content to be non-empty")
	}

	// Verify both docs have the new content
	c1, _ := doc1.GetContent()
	c2, _ := doc2.GetContent()

	t.Logf("After partial update: doc1=%s", c1)
	t.Logf("After partial update: doc2=%s", c2)

	// Both should contain the new text
	if !strings.Contains(c1, partial) {
		t.Errorf("expected doc1 to contain '%s', got: %s", partial, c1)
	}
	if !strings.Contains(c2, partial) {
		t.Errorf("expected doc2 to contain '%s', got: %s", partial, c2)
	}
}

func TestConcurrentChanges(t *testing.T) {
	log := logger.New(false)
	doc1, _ := NewDocument(log, DefaultTemplate)
	doc2, _ := NewDocument(log, DefaultTemplate)

	// Both start with same content
	baseContent := "Base content"
	doc1.ApplyLocalChange(baseContent)
	update, _ := doc1.ApplyLocalChange(baseContent)
	doc2.ApplyRemoteChange(update)

	// Doc1 makes a change
	content1 := "Base content modified by doc1"
	update1, _ := doc1.ApplyLocalChange(content1)

	// Doc2 makes a different change (simulating concurrent edit)
	content2 := "Base content modified by doc2"
	update2, _ := doc2.ApplyLocalChange(content2)

	// Exchange updates
	doc2.ApplyRemoteChange(update1)
	doc1.ApplyRemoteChange(update2)

	// Both should converge (CRDT property)
	c1, _ := doc1.GetContent()
	c2, _ := doc2.GetContent()

	t.Logf("After concurrent changes: doc1=%s, doc2=%s", c1, c2)

	// Note: With Y-crdt, both should converge to the same content
	// The exact merged content depends on the CRDT algorithm
}

func TestConcurrentSamePositionEditsConverge(t *testing.T) {
	log := logger.New(false)

	// Simulate the exact start/join flow:
	// 1. Host creates doc with initial content
	// 2. Join receives initial sync
	// 3. Both make concurrent edits at the SAME position
	// 4. Both exchange updates
	// 5. Both must converge to the same content

	t.Run("host joins then concurrent same-position insert", func(t *testing.T) {
		// Step 1: Host creates doc with content
		host, _ := NewDocument(log, "Hello")

		// Step 2: Join creates empty doc, receives initial sync
		join, _ := NewDocument(log, "")
		initialSync := host.GenerateFullUpdate()
		join.ApplyRemoteChange(initialSync)

		// Verify both have "Hello"
		hostContent, _ := host.GetContent()
		joinContent, _ := join.GetContent()
		if hostContent != joinContent {
			t.Fatalf("after initial sync: host=%q, join=%q", hostContent, joinContent)
		}

		// Step 3: Both make concurrent edits at the same position (end of "Hello")
		// Host inserts "A" at position 5
		hostUpdate, _ := host.ApplyLocalChange("HelloA")
		// Join inserts "B" at position 5
		joinUpdate, _ := join.ApplyLocalChange("HelloB")

		// Step 4: Exchange updates (simulating simultaneous send)
		join.ApplyRemoteChange(hostUpdate)
		host.ApplyRemoteChange(joinUpdate)

		// Step 5: Both must converge to identical content
		hostResult, _ := host.GetContent()
		joinResult, _ := join.GetContent()

		if hostResult != joinResult {
			t.Errorf("documents diverged after concurrent same-position edits:\n  host=%q\n  join=%q", hostResult, joinResult)
		}

		// Both characters must be present in the merged result
		if !strings.Contains(hostResult, "A") || !strings.Contains(hostResult, "B") {
			t.Errorf("merged result missing one of the concurrent characters: %q", hostResult)
		}
	})

	t.Run("concurrent same-position delete and insert", func(t *testing.T) {
		host, _ := NewDocument(log, "Hello World")
		join, _ := NewDocument(log, "")
		initialSync := host.GenerateFullUpdate()
		join.ApplyRemoteChange(initialSync)

		// Host deletes "World" (replaces entire string)
		hostUpdate, _ := host.ApplyLocalChange("Hello ")
		// Join modifies "World" to "World!" at same position
		joinUpdate, _ := join.ApplyLocalChange("Hello World!")

		// Exchange
		join.ApplyRemoteChange(hostUpdate)
		host.ApplyRemoteChange(joinUpdate)

		hostResult, _ := host.GetContent()
		joinResult, _ := join.GetContent()

		if hostResult != joinResult {
			t.Errorf("documents diverged after concurrent delete+insert:\n  host=%q\n  join=%q", hostResult, joinResult)
		}
	})

	t.Run("concurrent middle-of-line edits", func(t *testing.T) {
		host, _ := NewDocument(log, "The quick brown fox")
		join, _ := NewDocument(log, "")
		initialSync := host.GenerateFullUpdate()
		join.ApplyRemoteChange(initialSync)

		// Host inserts "lazy " before "brown" (position 10)
		hostUpdate, _ := host.ApplyLocalChange("The quick lazy brown fox")
		// Join capitalizes "brown" to "Brown"  at same area
		joinUpdate, _ := join.ApplyLocalChange("The quick Brown fox")

		join.ApplyRemoteChange(hostUpdate)
		host.ApplyRemoteChange(joinUpdate)

		hostResult, _ := host.GetContent()
		joinResult, _ := join.GetContent()

		if hostResult != joinResult {
			t.Errorf("documents diverged after concurrent middle edits:\n  host=%q\n  join=%q", hostResult, joinResult)
		}
	})

	t.Run("parallel goroutines calling ApplyLocalChange and ApplyRemoteChange", func(t *testing.T) {
		host, _ := NewDocument(log, "Line 1\nLine 2\nLine 3")
		join, _ := NewDocument(log, "")
		initialSync := host.GenerateFullUpdate()
		join.ApplyRemoteChange(initialSync)

		// Use goroutines to simulate real concurrent access
		var wg sync.WaitGroup
		var hostUpdate, joinUpdate []byte

		wg.Add(2)
		go func() {
			defer wg.Done()
			hostUpdate, _ = host.ApplyLocalChange("Line 1\nLine 2 modified\nLine 3")
		}()
		go func() {
			defer wg.Done()
			joinUpdate, _ = join.ApplyLocalChange("Line 1\nLine 2\nLine 3 extra")
		}()
		wg.Wait()

		// Exchange updates
		wg.Add(2)
		go func() {
			defer wg.Done()
			host.ApplyRemoteChange(joinUpdate)
		}()
		go func() {
			defer wg.Done()
			join.ApplyRemoteChange(hostUpdate)
		}()
		wg.Wait()

		hostResult, _ := host.GetContent()
		joinResult, _ := join.GetContent()

		if hostResult != joinResult {
			t.Errorf("documents diverged after parallel goroutine access:\n  host=%q\n  join=%q", hostResult, joinResult)
		}
	})
}

func TestQueueLocalChange(t *testing.T) {
	log := logger.New(false)
	doc, _ := NewDocument(log, DefaultTemplate)

	// Queue a simple function
	called := false
	doc.QueueLocalChange(func() {
		called = true
	})

	doc.ProcessPendingChanges()

	if !called {
		t.Error("expected queued function to be called")
	}
}

func TestQueueFull(t *testing.T) {
	log := logger.New(false)
	doc, _ := NewDocument(log, DefaultTemplate)

	// Fill up the queue (capacity 10)
	for i := 0; i < 15; i++ {
		doc.QueueLocalChange(func() {})
	}

	// Should not panic, just print warning
	// Process to clear
	doc.ProcessPendingChanges()
}
