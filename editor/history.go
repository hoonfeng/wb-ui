// Translation of: CodeMirror 6 — @codemirror/commands history.ts
//                  https://github.com/codemirror/commands/blob/main/src/history.ts
//
// Completeness: 50%
// Differences from CM6:
//   - CM6 uses a branch-based history with per-branch undo; this port uses
//     a simple undo/redo stack of TransactionGroups, similar to WebKit's
//     TypingCommand openForMoreTyping merging.
//   - The `HistoryState` facet and `isolateHistory` extension are omitted.
//   - Methods use PascalCase (Go convention).
//
// History maintains an undo stack and a redo stack of TransactionGroups.
// Consecutive transactions with the same UserEvent are merged into a single
// group, so e.g. typing "abc" produces one undo step (not three).

package editor

// History maintains undo and redo stacks for an editor.
type History struct {
	// undoStack holds TransactionGroups that can be undone.
	undoStack []*TransactionGroup
	// redoStack holds TransactionGroups that can be redone.
	redoStack []*TransactionGroup
	// currentGroup is the group being built for the current editing
	// sequence. When it is finalized (e.g. by a different userEvent or by
	// an explicit CloseTyping call), it is moved to the undo stack.
	currentGroup *TransactionGroup
}

// NewHistory creates an empty History.
func NewHistory() *History {
	return &History{}
}

// Record adds a transaction to the history. If the transaction has the same
// userEvent as the current group, it is merged into that group. Otherwise,
// the current group is finalized (moved to the undo stack) and a new group
// is started.
func (h *History) Record(tr Transaction) {
	if tr.UserEvent() == "" {
		// Non-user transactions (e.g. programmatic changes) are not recorded.
		return
	}

	// Check if we can merge with the current group.
	if h.currentGroup != nil && !h.currentGroup.Empty() {
		if h.currentGroup.userEvent == tr.UserEvent() {
			h.currentGroup.Add(tr)
			return
		}
		// Different userEvent — finalize the current group.
		h.finalizeGroup()
	}

	// Start a new group.
	h.currentGroup = NewTransactionGroup(tr.UserEvent())
	h.currentGroup.Add(tr)
}

// finalizeGroup moves the current group to the undo stack and clears the
// redo stack (since a new edit invalidates the redo history).
func (h *History) finalizeGroup() {
	if h.currentGroup != nil && !h.currentGroup.Empty() {
		h.undoStack = append(h.undoStack, h.currentGroup)
		h.redoStack = nil // clear redo on new edit
	}
	h.currentGroup = nil
}

// CloseTyping finalizes the current group, forcing the next transaction to
// start a new undo step. This is analogous to WebKit's
// TypingCommand::closeTyping().
func (h *History) CloseTyping() {
	h.finalizeGroup()
}

// Undo undoes the last group on the undo stack. Returns the inverse
// transactions (in application order) and true if successful.
// After Undo, the view should apply the inverse transactions.
func (h *History) Undo(view *EditorView) bool {
	h.finalizeGroup() // ensure the current group is on the stack
	if len(h.undoStack) == 0 {
		return false
	}

	// Pop the last group from the undo stack.
	idx := len(h.undoStack) - 1
	group := h.undoStack[idx]
	h.undoStack = h.undoStack[:idx]

	// Build inverse transactions and apply them.
	// The inverse of a group is the inverse of each transaction in reverse
	// order. Each transaction's original document (the document that
	// existed before that transaction was applied) is available via
	// tr.StartState().Doc.
	for i := len(group.transactions) - 1; i >= 0; i-- {
		tr := group.transactions[i]
		// Get the original document (before the transaction was applied).
		// This is needed because Invert extracts the original text content
		// that was replaced by the change.
		var originalDoc Text
		if tr.StartState() != nil {
			originalDoc = tr.StartState().Doc
		} else {
			originalDoc = view.state.Doc // fallback (may be incorrect)
		}
		// Create the inverse change set.
		inverseChanges := tr.Changes().Invert(originalDoc)
		// Map selection through the inverse changes.
		newSel := view.state.Selection.Map(inverseChanges)
		// Update the state (applies inverseChanges to view.state.Doc).
		view.state, _ = view.state.Update(TransactionSpec{
			Changes:    inverseChanges,
			Selection:  &newSel,
			UserEvent:  "undo",
		})
	}

	// Push the group onto the redo stack.
	h.redoStack = append(h.redoStack, group)

	// Invalidate caches.
	view.tokenCacheValid = false
	view.recalculateLayout()
	view.updateViewport()

	return true
}

// Redo redoes the last group on the redo stack. Returns true if successful.
func (h *History) Redo(view *EditorView) bool {
	if len(h.redoStack) == 0 {
		return false
	}

	// Pop the last group from the redo stack.
	idx := len(h.redoStack) - 1
	group := h.redoStack[idx]
	h.redoStack = h.redoStack[:idx]

	// Re-apply all transactions in the group.
	// Each transaction's Changes already describe the original edit;
	// re-applying them produces the same document change.
	for _, tr := range group.transactions {
		view.state, _ = view.state.Update(TransactionSpec{
			Changes:    tr.Changes(),
			Selection:  nil,
			HasSelection: false, // map selection through changes
			UserEvent:  "redo",
		})
	}

	// Push the group back onto the undo stack.
	h.undoStack = append(h.undoStack, group)

	// Invalidate caches.
	view.tokenCacheValid = false
	view.recalculateLayout()
	view.updateViewport()

	return true
}

// CanUndo reports whether there are any undo steps.
func (h *History) CanUndo() bool {
	h.finalizeGroup()
	return len(h.undoStack) > 0
}

// CanRedo reports whether there are any redo steps.
func (h *History) CanRedo() bool {
	return len(h.redoStack) > 0
}

// Clear empties both the undo and redo stacks.
func (h *History) Clear() {
	h.undoStack = nil
	h.redoStack = nil
	h.currentGroup = nil
}
