// Translation of: Source/WebCore/editing/DeleteKeyCommand.h
//                  Source/WebCore/editing/DeleteKeyCommand.cpp
//                  Source/WebCore/editing/DeleteFromTextNodeCommand.h (leaf)
//                  Source/WebCore/editing/DeleteFromTextNodeCommand.cpp
// Completeness: 50%
// Simplifications:
//   - operates on a plain-text editable element by character offset,
//     matching the simplification used by InsertTextCommand
//   - supports both backward (backspace, direction=-1) and forward
//     (Delete key, direction=+1) deletion
//   - direction=-1: delete the character immediately before offset
//     (offset points after the character to delete)
//   - direction=+1: delete the character immediately at offset
//     (offset points at the character to delete)
//   - rune-aware deletion (multi-byte characters are not split)

package editing

import (
	"wb-ui/engine/dom"
)

// DeleteDirection indicates the direction of deletion.
type DeleteDirection int

const (
	// DeleteBackward deletes the character before the offset (backspace).
	DeleteBackward DeleteDirection = -1
	// DeleteForward deletes the character at the offset (forward delete / Delete key).
	DeleteForward DeleteDirection = 1
)

// DeleteCommand deletes a single character from a plain-text editable element.
// It mirrors WebCore's DeleteFromTextNodeCommand leaf command with a direction
// parameter, combining the functionality of DeleteKeyCommand (backspace) and
// ForwardDeleteKeyCommand.
type DeleteCommand struct {
	EditCommandBase

	// Target is the editable element whose text content is the buffer.
	Target *dom.Element
	// Offset is the character offset (in runes) within Target's text content.
	Offset int
	// Direction is DeleteBackward (-1) or DeleteForward (+1).
	Direction DeleteDirection
	// Action records the EditAction for InputEvent.inputType mapping.
	Action EditAction
	// DeletedText holds the text removed by DoApply, saved for DoUnapply.
	DeletedText string
	// DeletedOffset is the actual start offset where deletion occurred
	// (adjusted for backward deletion), saved for DoUnapply.
	DeletedOffset int
}

// NewDeleteCommand constructs a DeleteCommand, mirroring the WebKit
// DeleteFromTextNodeCommand::create / DeleteKeyCommand::create pattern.
func NewDeleteCommand(doc *dom.Document, target *dom.Element, offset int, direction DeleteDirection, action EditAction) *DeleteCommand {
	return &DeleteCommand{
		EditCommandBase: EditCommandBase{Document_: doc},
		Target:          target,
		Offset:          offset,
		Direction:       direction,
		Action:          action,
	}
}

// DoApply performs the deletion, saving the deleted text and actual offset
// for undo.
func (c *DeleteCommand) DoApply() error {
	if c.Target == nil {
		return nil
	}
	current := c.Target.TextContent()
	runes := []rune(current)
	if len(runes) == 0 {
		return nil
	}

	// Clamp offset to valid range.
	if c.Offset < 0 {
		c.Offset = 0
	}
	if c.Offset > len(runes) {
		c.Offset = len(runes)
	}

	var from, to int
	if c.Direction == DeleteBackward {
		// Delete the character before offset.
		if c.Offset == 0 {
			return nil // nothing to delete
		}
		from = c.Offset - 1
		to = c.Offset
		c.DeletedOffset = from
	} else {
		// Delete the character at offset.
		if c.Offset >= len(runes) {
			return nil // nothing to delete
		}
		from = c.Offset
		to = c.Offset + 1
		c.DeletedOffset = from
	}

	c.DeletedText = string(runes[from:to])
	newRunes := append(append([]rune{}, runes[:from]...), runes[to:]...)
	c.Target.SetTextContent(string(newRunes))
	return nil
}

// DoUnapply re-inserts the deleted text at the saved offset.
func (c *DeleteCommand) DoUnapply() error {
	if c.Target == nil || c.DeletedText == "" {
		return nil
	}
	current := c.Target.TextContent()
	runes := []rune(current)
	if c.DeletedOffset < 0 {
		c.DeletedOffset = 0
	}
	if c.DeletedOffset > len(runes) {
		c.DeletedOffset = len(runes)
	}
	textRunes := []rune(c.DeletedText)
	newRunes := append(append([]rune{}, runes[:c.DeletedOffset]...), append(textRunes, runes[c.DeletedOffset:]...)...)
	c.Target.SetTextContent(string(newRunes))
	return nil
}

// DoReapply re-applies the deletion by delegating to DoApply.
func (c *DeleteCommand) DoReapply() error { return c.DoApply() }

// EditingAction returns the action classification.
func (c *DeleteCommand) EditingAction() EditAction { return c.Action }

// IsSimpleEditCommand returns true; DeleteCommand is a leaf.
func (c *DeleteCommand) IsSimpleEditCommand() bool { return true }

// IsCompositeEditCommand returns false.
func (c *DeleteCommand) IsCompositeEditCommand() bool { return false }

// GetNodesInCommand records the target element as the touched node.
func (c *DeleteCommand) GetNodesInCommand(nodes map[dom.Node]struct{}) {
	if c.Target != nil {
		nodes[c.Target] = struct{}{}
	}
}
