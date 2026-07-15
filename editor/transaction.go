// Translation of: CodeMirror 6 — packages/state/src/transaction.ts
//                  https://github.com/codemirror/state/blob/main/src/transaction.ts
//
// Completeness: 75%
// Differences from CM6:
//   - StateEffect (side effects) is simplified; the first version supports
//     only a "scroll into view" flag rather than a full effect system.
//   - Annotation (metadata) is omitted in v1.
//   - The `reconfigured` field is replaced by a simpler `configChanged` bool.
//   - Methods use PascalCase (Go convention).
//
// Transaction represents an atomic state update. It bundles document changes,
// a new selection, and metadata about the update.

package editor

// TransactionSpec is the input to create a Transaction. All fields are
// optional; a spec with only changes and no selection will compute the
// new selection from the changes.
//
// Mirrors CM6's TransactionSpec / StateUpdate.
type TransactionSpec struct {
	// Changes is the ChangeSet to apply to the document. May be empty.
	Changes ChangeSet
	// Selection is the new selection after the changes. If nil, the
	// selection is mapped through the changes.
	Selection *EditorSelection
	// HasSelection reports whether Selection should be used (to distinguish
	// "no selection change" from "set selection to nil").
	HasSelection bool
	// ScrollIntoView requests the viewport to scroll to the new selection.
	ScrollIntoView bool
	// UserEvent is a string identifying the user action (e.g. "input.type",
	// "delete.backward", "undo"). Used by extensions for behavior.
	UserEvent string
	// Annotations is arbitrary metadata key-value pairs.
	Annotations map[string]string
	// IsRemote reports whether this transaction originated from a remote
	// peer (collaboration). Not used in v1 but reserved.
	IsRemote bool
}

// Transaction is an applied state update. It is immutable.
type Transaction struct {
	// changes is the ChangeSet applied to the document.
	changes ChangeSet
	// selection is the new selection after the changes.
	selection EditorSelection
	// startState is the state the transaction was applied to (for
	// computing inverses). May be nil for transactions created without
	// a full state.
	startState *EditorState
	// annotations holds metadata.
	annotations map[string]string
	// scrollIntoView requests viewport scroll.
	scrollIntoView bool
	// userEvent identifies the user action.
	userEvent string
}

// NewTransaction creates a Transaction from a start state and a spec.
// The changes are applied to the state's document, and the selection is
// either taken from the spec or mapped through the changes.
func NewTransaction(state *EditorState, spec TransactionSpec) Transaction {
	changes := spec.Changes
	var selection EditorSelection
	if spec.HasSelection && spec.Selection != nil {
		selection = *spec.Selection
	} else if state != nil {
		selection = state.Selection.Map(changes)
	} else {
		selection = SelectionCaret(0)
	}
	anns := spec.Annotations
	if anns == nil {
		anns = map[string]string{}
	}
	if spec.UserEvent != "" {
		anns["userEvent"] = spec.UserEvent
	}
	return Transaction{
		changes:        changes,
		selection:      selection,
		startState:     state,
		annotations:    anns,
		scrollIntoView: spec.ScrollIntoView,
		userEvent:      spec.UserEvent,
	}
}

// Changes returns the ChangeSet applied by this transaction.
func (t Transaction) Changes() ChangeSet { return t.changes }

// Selection returns the new selection after this transaction.
func (t Transaction) Selection() EditorSelection { return t.selection }

// StartState returns the state this transaction was applied to.
func (t Transaction) StartState() *EditorState { return t.startState }

// ScrollIntoView reports whether the viewport should scroll to the new
// selection.
func (t Transaction) ScrollIntoView() bool { return t.scrollIntoView }

// UserEvent returns the user event string (e.g. "input.type").
func (t Transaction) UserEvent() string { return t.userEvent }

// Annotation returns the annotation value for the given key, or "" if
// not present.
func (t Transaction) Annotation(key string) string {
	if t.annotations == nil {
		return ""
	}
	return t.annotations[key]
}

// AnnotationsMap returns all annotations.
func (t Transaction) AnnotationsMap() map[string]string {
	return t.annotations
}

// InvertChanges returns a ChangeSet that undoes this transaction's changes.
// Requires a startState with a document.
func (t Transaction) InvertChanges() ChangeSet {
	if t.startState == nil {
		return ChangeSet{}
	}
	return t.changes.Invert(t.startState.Doc)
}

// ReconfigureSpec is a TransactionSpec for reconfiguring the editor state
// (changing extensions). Not fully implemented in v1; reserved for future.
type ReconfigureSpec struct {
	Extensions []Extension
}

// TransactionGroup groups consecutive transactions that should be treated
// as a single undo step (mirrors CM6's history.TransactionGroup and the
// WebKit TypingCommand openForMoreTyping concept from Phase 0).
type TransactionGroup struct {
	// transactions are the grouped transactions.
	transactions []Transaction
	// userEvent is the userEvent annotation that identifies this group
	// (e.g. "input.type", "delete", "undo"). Used by History to merge
	// consecutive transactions of the same type.
	userEvent string
}

// NewTransactionGroup creates an empty TransactionGroup with the given
// userEvent. If userEvent is empty, the group is untyped (used for
// programmatic grouping).
func NewTransactionGroup(userEvent string) *TransactionGroup {
	return &TransactionGroup{userEvent: userEvent}
}

// UserEvent returns the group's userEvent annotation.
func (g *TransactionGroup) UserEvent() string { return g.userEvent }

// Add appends a transaction to the group.
func (g *TransactionGroup) Add(tr Transaction) {
	g.transactions = append(g.transactions, tr)
}

// Empty reports whether the group has no transactions.
func (g *TransactionGroup) Empty() bool {
	return len(g.transactions) == 0
}

// Transactions returns the grouped transactions.
func (g *TransactionGroup) Transactions() []Transaction {
	return g.transactions
}

// MergeChanges returns a single ChangeSet that combines all the changes
// in the group.
func (g *TransactionGroup) MergeChanges() ChangeSet {
	if len(g.transactions) == 0 {
		return ChangeSet{}
	}
	// For simplicity, compose by applying changes sequentially.
	// A more efficient implementation would compose ChangeSets algebraically.
	combined := g.transactions[0].Changes()
	for _, tr := range g.transactions[1:] {
		combined = composeChangeSets(combined, tr.Changes())
	}
	return combined
}

// composeChangeSets composes two ChangeSets into one. This is a simplified
// implementation that assumes the second ChangeSet is applied after the
// first. A full implementation would map the second's ranges through the
// first and merge overlapping changes.
func composeChangeSets(first, second ChangeSet) ChangeSet {
	if first.Empty() {
		return second
	}
	if second.Empty() {
		return first
	}
	// For v1, we just concatenate the changes after mapping the second
	// through the first. This is correct for non-overlapping sequential
	// edits (the common case for typing).
	var descs []ChangeDesc
	var inserted []Text
	for _, c := range first.changes {
		descs = append(descs, c)
	}
	for _, c := range second.changes {
		mapped := first.MapDesc(c, 1)
		descs = append(descs, mapped)
	}
	for _, t := range first.inserted {
		inserted = append(inserted, t)
	}
	for _, t := range second.inserted {
		inserted = append(inserted, t)
	}
	return NewChangeSetWithText(descs, inserted)
}
