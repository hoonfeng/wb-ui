// Translation of: Source/WebCore/editing/EditCommand.h
//                  Source/WebCore/editing/EditCommand.cpp
//                  Source/WebCore/editing/CompositeEditCommand.h (interface parts)
// Completeness: 50%
// Simplifications:
//   - the C++ class hierarchy EditCommand → SimpleEditCommand →
//     CompositeEditCommand is modeled with Go interfaces and a concrete
//     CompositeEditCommand struct that holds a list of sub-commands
//   - EditCommand retains a reference to the document and the
//     starting/ending VisibleSelection so that selection restoration works
//     through undo/redo
//   - inputEventNames / inputEventDataNames are not stored on the command;
//     callers map EditAction → inputType via InputTypeFor()
//   - the "open for more typing" mechanism (TypingCommand::isOpenForMoreTyping)
//     is implemented on TypingCommand itself, not on a shared EditCommand
//     "open typing" flag
//   - SmartReplace / EditCommandComposition / UndoStep interplay is reduced:
//     EditCommandComposition is a snapshot of a CompositeEditCommand captured
//     at the time the composite is committed to the undo stack
//   - the C++ EditCommand::m_groupingLevel (for batching edits across commands)
//     is omitted; grouping is handled by EditCommandComposition when pushed
//     onto the undo stack

package editing

import "wb-ui/engine/dom"

// EditCommand is the Go translation of the interface portion of
// WebCore::EditCommand. It is the contract that every editing operation
// (TypingCommand, ReplaceSelectionCommand, etc.) must satisfy to participate
// in the undo/redo stack.
//
// In WebKit EditCommand is an abstract base class; in this port it is a Go
// interface. Concrete commands embed the shared EditCommandBase struct (which
// supplies the common state and helper methods) and implement DoApply /
// DoUnapply themselves.
type EditCommand interface {
	// DoApply applies the command to the document, mirroring EditCommand::doApply().
	// It must be idempotent within a single application: applying the same
	// command twice without an intervening DoUnapply is undefined.
	DoApply() error

	// DoUnapply reverses the command, mirroring EditCommand::doUnapply().
	// Only commands that were previously applied may be unapplied.
	DoUnapply() error

	// DoReapply re-applies an unapplied command, mirroring EditCommand::doReapply().
	// The default implementation forwards to DoApply.
	DoReapply() error

	// EditingAction returns the classification of the command, mirroring
	// EditCommand::editingAction(). Used for InputEvent.inputType mapping
	// and accessibility announcements.
	EditingAction() EditAction

	// StartingSelection returns the selection that was active when the
	// command was applied, mirroring EditCommand::startingSelection().
	// Used to restore the selection on undo.
	StartingSelection() *VisibleSelection

	// EndingSelection returns the selection that should be active after the
	// command is applied, mirroring EditCommand::endingSelection().
	// Used to restore the selection on redo.
	EndingSelection() *VisibleSelection

	// SetStartingSelection records the selection to be restored on undo,
	// mirroring EditCommand::setStartingSelection().
	SetStartingSelection(s *VisibleSelection)

	// SetEndingSelection records the selection to be restored on redo,
	// mirroring EditCommand::setEndingSelection().
	SetEndingSelection(s *VisibleSelection)

	// IsSimpleEditCommand reports whether the command is a SimpleEditCommand
	// (leaf operation), mirroring EditCommand::isSimpleEditCommand().
	IsSimpleEditCommand() bool

	// IsCompositeEditCommand reports whether the command is a
	// CompositeEditCommand (container of sub-commands), mirroring
	// EditCommand::isCompositeEditCommand().
	IsCompositeEditCommand() bool

	// Document returns the owning document, mirroring EditCommand::document().
	Document() *dom.Document

	// IsPreservesTypingStyle is the Go form of EditCommand::preservesTypingStyle().
	// In this port typing style is not maintained, so this always returns false
	// unless overridden.
	IsPreservesTypingStyle() bool
}

// EditCommandBase supplies common state shared by every concrete EditCommand.
// Embedding this struct in a command implementation provides the storage for
// the starting/ending selections and the owning document, mirroring the
// non-virtual data members of WebCore::EditCommand.
//
// Embedding is preferred over inheritance: a command struct embeds
// EditCommandBase and then implements the EditCommand interface methods,
// delegating the trivial getters to the embedded struct.
type EditCommandBase struct {
	// Document is the owning document, mirroring EditCommand::m_document.
	Document_ *dom.Document
	// StartingSel is the selection active when the command was applied,
	// mirroring EditCommand::m_startingSelection.
	StartingSel *VisibleSelection
	// EndingSel is the selection to restore on redo, mirroring
	// EditCommand::m_endingSelection.
	EndingSel *VisibleSelection
}

// Document returns the owning document, satisfying the EditCommand interface.
func (b *EditCommandBase) Document() *dom.Document { return b.Document_ }

// StartingSelection returns the starting selection.
func (b *EditCommandBase) StartingSelection() *VisibleSelection { return b.StartingSel }

// EndingSelection returns the ending selection.
func (b *EditCommandBase) EndingSelection() *VisibleSelection { return b.EndingSel }

// SetStartingSelection records the starting selection.
func (b *EditCommandBase) SetStartingSelection(s *VisibleSelection) { b.StartingSel = s }

// SetEndingSelection records the ending selection.
func (b *EditCommandBase) SetEndingSelection(s *VisibleSelection) { b.EndingSel = s }

// IsPreservesTypingStyle is the default; commands that preserve typing style
// override this in their own struct.
func (b *EditCommandBase) IsPreservesTypingStyle() bool { return false }

// SimpleEditCommand is the Go translation of the SimpleEditCommand interface
// portion of WebCore. A SimpleEditCommand is a leaf editing operation that
// performs a single DOM mutation (e.g. insert a text node, remove a node).
//
// In WebKit SimpleEditCommand adds doUnapply/doReapply/getNodesInCommand over
// EditCommand; in this port the contract is expressed as an additional
// interface that simple commands also satisfy.
type SimpleEditCommand interface {
	EditCommand
	// GetNodesInCommand adds the nodes touched by this command to the given
	// set, mirroring SimpleEditCommand::getNodesInCommand(NodeSet&). Used by
	// the framework to track which nodes need post-edit style recalculation.
	GetNodesInCommand(nodes map[dom.Node]struct{})
}

// CompositeEditCommand is the Go translation of the CompositeEditCommand
// interface portion of WebCore. A CompositeEditCommand is a container of
// sub-commands (SimpleEditCommand or nested CompositeEditCommand). Applying
// the composite applies each sub-command in order; unapplying reverses them.
//
// Concrete composite commands (TypingCommand, ReplaceSelectionCommand, etc.)
// satisfy this interface by embedding *CompositeEditCommandBase (defined in
// compositeeditcommand.go) which provides the storage and the default
// implementations of these methods.
type CompositeEditCommand interface {
	EditCommand
	// ApplyCommand adds a sub-command and applies it immediately, mirroring
	// CompositeEditCommand::applyCommand(PassRefPtr<EditCommand>). Returns
	// the error from the sub-command's DoApply.
	ApplyCommand(cmd EditCommand) error

	// AppendCommand adds a sub-command without applying it (used during
	// undo/redo replay), mirroring CompositeEditCommand::append().
	AppendCommand(cmd EditCommand)

	// RemoveCommand removes the sub-command at the given index, mirroring
	// CompositeEditCommand::removeCommand().
	RemoveCommand(idx int) EditCommand

	// HasCommands reports whether the composite has any sub-commands.
	HasCommands() bool

	// Commands returns the sub-commands (read-only view).
	Commands() []EditCommand

	// IsEditCommandComposition reports whether this composite has been
	// committed to the undo stack as an EditCommandComposition snapshot.
	IsEditCommandComposition() bool
}

// IsSimpleEditCommand returns false for the composite base (overridden on
// SimpleEditCommand implementations).
//
// This is provided as a free function so that command implementations can
// delegate to it without repeating the literal false.
func IsSimpleEditCommandDefault() bool { return false }

// IsCompositeEditCommandDefault returns true for composites (overridden on
// SimpleEditCommand implementations).
func IsCompositeEditCommandDefault() bool { return true }
