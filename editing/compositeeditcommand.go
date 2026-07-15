// Translation of: Source/WebCore/editing/CompositeEditCommand.h
//                  Source/WebCore/editing/CompositeEditCommand.cpp
// Completeness: 55%
// Simplifications:
//   - the C++ composite command's "scope" mechanism (which tracks the
//     currently-applied composite so that sub-commands register themselves
//     into it) is replaced by an explicit ApplyCommand entry point that
//     appends and applies
//   - the full set of helpers (insertNodeAt / insertNodeBefore / insertNodeAfter /
//     insertParagraphSeparator / etc.) is not translated here; only the
//     container machinery needed by TypingCommand and EditCommandComposition
//     is provided. Rich-text editing helpers will be added when contenteditable
//     is needed
//   - pruning of empty nodes (PruneEmptyNode / pruneEmptyNodesAncestor) is
//     deferred
//   - VisuallyEquivalentRange / RangeFromStartToEnd are omitted (not needed
//     by the code editor's plain-text model)

package editing

import "wb-ui/dom"

// CompositeEditCommandBase supplies the storage and default implementations
// for a CompositeEditCommand. Concrete composite commands (TypingCommand,
// ReplaceSelectionCommand, EditCommandComposition) embed this struct and
// implement DoApply / DoUnapply themselves.
//
// Embedding EditCommandBase gives the document reference and starting/ending
// selection; this struct adds the list of sub-commands.
type CompositeEditCommandBase struct {
	EditCommandBase
	// Commands_ is the ordered list of sub-commands.
	Commands_ []EditCommand
	// IsComposition records whether this composite has been committed to
	// the undo stack as an EditCommandComposition snapshot. Avoids double-
	// counting when a TypingCommand is later re-wrapped.
	IsComposition bool
}

// ApplyCommand adds a sub-command and applies it immediately, mirroring
// CompositeEditCommand::applyCommand.
func (c *CompositeEditCommandBase) ApplyCommand(cmd EditCommand) error {
	if err := cmd.DoApply(); err != nil {
		return err
	}
	c.Commands_ = append(c.Commands_, cmd)
	return nil
}

// AppendCommand adds a sub-command without applying it (used during undo/redo
// replay), mirroring CompositeEditCommand::append.
func (c *CompositeEditCommandBase) AppendCommand(cmd EditCommand) {
	c.Commands_ = append(c.Commands_, cmd)
}

// RemoveCommand removes the sub-command at the given index and returns it,
// mirroring CompositeEditCommand::removeCommand.
func (c *CompositeEditCommandBase) RemoveCommand(idx int) EditCommand {
	if idx < 0 || idx >= len(c.Commands_) {
		return nil
	}
	removed := c.Commands_[idx]
	c.Commands_ = append(c.Commands_[:idx], c.Commands_[idx+1:]...)
	return removed
}

// HasCommands reports whether the composite has any sub-commands.
func (c *CompositeEditCommandBase) HasCommands() bool { return len(c.Commands_) > 0 }

// Commands returns the sub-commands (read-only view).
func (c *CompositeEditCommandBase) Commands() []EditCommand { return c.Commands_ }

// IsEditCommandComposition reports whether this composite has been committed
// as an EditCommandComposition snapshot.
func (c *CompositeEditCommandBase) IsEditCommandComposition() bool { return c.IsComposition }

// IsSimpleEditCommand returns false for composites (overridden on
// SimpleEditCommand implementations).
func (c *CompositeEditCommandBase) IsSimpleEditCommand() bool { return false }

// IsCompositeEditCommand returns true for composites.
func (c *CompositeEditCommandBase) IsCompositeEditCommand() bool { return true }

// DoReapply re-applies an unapplied composite by re-applying each sub-command
// in order. The composite's own DoApply should delegate here.
//
// Concrete composites typically write:
//
//	func (c *MyCmd) DoApply() error  { return c.CompositeEditCommandBase.ApplyAll() }
//	func (c *MyCmd) DoUnapply() error { return c.CompositeEditCommandBase.UnapplyAll() }
//	func (c *MyCmd) DoReapply() error { return c.CompositeEditCommandBase.ReapplyAll() }
//
// and override DoApply only when they need to perform extra setup before
// running the sub-commands.
func (c *CompositeEditCommandBase) ApplyAll() error {
	for _, cmd := range c.Commands_ {
		if err := cmd.DoApply(); err != nil {
			return err
		}
	}
	return nil
}

// UnapplyAll un-applies each sub-command in reverse order, mirroring the
// CompositeEditCommand::doUnapply logic.
func (c *CompositeEditCommandBase) UnapplyAll() error {
	for i := len(c.Commands_) - 1; i >= 0; i-- {
		if err := c.Commands_[i].DoUnapply(); err != nil {
			return err
		}
	}
	return nil
}

// ReapplyAll re-applies each sub-command in order, mirroring the
// CompositeEditCommand::doReapply logic.
func (c *CompositeEditCommandBase) ReapplyAll() error {
	for _, cmd := range c.Commands_ {
		if err := cmd.DoReapply(); err != nil {
			return err
		}
	}
	return nil
}

// NewCompositeBase constructs a CompositeEditCommandBase for the given
// document, mirroring the CompositeEditCommand constructor.
func NewCompositeBase(doc *dom.Document) CompositeEditCommandBase {
	return CompositeEditCommandBase{
		EditCommandBase: EditCommandBase{Document_: doc},
	}
}
