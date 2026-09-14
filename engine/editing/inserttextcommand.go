// Translation of: Source/WebCore/editing/InsertTextCommand.h
//                  Source/WebCore/editing/InsertTextCommand.cpp
//                  Source/WebCore/editing/TextInsertionBaseCommand.h
// Completeness: 50%
// Simplifications:
//   - operates on a generic dom.Element plain-text editable scope (treats
//     the element's text content as the editable buffer); the full WebKit
//     version operates on the live DOM Selection inside a contenteditable
//     element via VisiblePosition / RenderTextControl
//   - whisperText / smartReplace / rebalanceWhitespace are omitted
//   - the position is recorded as (Container, Offset) directly rather than
//     as a live Position with caret affinity; the offset is in characters
//     within the focused element's text content
//   - newline insertion (InsertLineBreak / InsertParagraphSeparator) is
//     handled by InsertTextCommand itself by inserting "\n" rather than
//     creating a separate <br> / <p> as WebKit does (wb-ui's plain-text
//     editable scope does not support inline elements)

package editing

import (
	"strings"

	"wb-ui/engine/dom"
)

// InsertTextCommand inserts a literal text string into a plain-text editable
// element at the given offset. It is the Go translation of the leaf
// InsertTextCommand (the inner per-character command that TypingCommand
// spawns), not the user-facing InsertText entry point (which is TypingCommand).
//
// In WebKit the user-facing EditCommand that inserts text is TypingCommand,
// which may in turn spawn many leaf InsertTextCommand instances as the user
// types. In this port TypingCommand may also do the insertion directly
// without spawning a leaf InsertTextCommand, in which case this struct is
// used only when explicit sub-commands are desired.
type InsertTextCommand struct {
	EditCommandBase

	// Target is the editable element whose text content is the buffer.
	Target *dom.Element
	// Offset is the character offset within Target's text content where
	// the text is inserted.
	Offset int
	// Text is the string to insert.
	Text string
	// Action records the EditAction (InsertText / InsertLineBreak /
	// InsertParagraphSeparator) used for InputEvent.inputType mapping.
	Action EditAction
}

// NewInsertTextCommand constructs an InsertTextCommand, mirroring
// InsertTextCommand::create(doc, text, options).
func NewInsertTextCommand(doc *dom.Document, target *dom.Element, offset int, text string, action EditAction) *InsertTextCommand {
	return &InsertTextCommand{
		EditCommandBase: EditCommandBase{Document_: doc},
		Target:          target,
		Offset:          offset,
		Text:            text,
		Action:          action,
	}
}

// DoApply inserts the text, mirroring InsertTextCommand::doApply.
func (c *InsertTextCommand) DoApply() error {
	if c.Target == nil {
		return nil
	}
	current := c.Target.TextContent()
	if c.Offset < 0 {
		c.Offset = 0
	}
	if c.Offset > len(current) {
		c.Offset = len(current)
	}
	// rune-aware insertion to avoid splitting multi-byte sequences
	runes := []rune(current)
	textRunes := []rune(c.Text)
	newRunes := append(append([]rune{}, runes[:c.Offset]...), append(textRunes, runes[c.Offset:]...)...)
	c.Target.SetTextContent(string(newRunes))
	return nil
}

// DoUnapply removes the inserted text, mirroring InsertTextCommand::doUnapply.
func (c *InsertTextCommand) DoUnapply() error {
	if c.Target == nil {
		return nil
	}
	current := c.Target.TextContent()
	if c.Offset < 0 || c.Offset > len(current) {
		return nil
	}
	runes := []rune(current)
	textRunes := []rune(c.Text)
	end := c.Offset + len(textRunes)
	if end > len(runes) {
		end = len(runes)
	}
	newRunes := append(append([]rune{}, runes[:c.Offset]...), runes[end:]...)
	c.Target.SetTextContent(string(newRunes))
	return nil
}

// DoReapply re-applies the insertion by delegating to DoApply.
func (c *InsertTextCommand) DoReapply() error { return c.DoApply() }

// EditingAction returns the action (InsertText / InsertLineBreak /
// InsertParagraphSeparator).
func (c *InsertTextCommand) EditingAction() EditAction { return c.Action }

// IsSimpleEditCommand returns true; InsertTextCommand is a leaf.
func (c *InsertTextCommand) IsSimpleEditCommand() bool { return true }

// IsCompositeEditCommand returns false.
func (c *InsertTextCommand) IsCompositeEditCommand() bool { return false }

// GetNodesInCommand records the target element as the touched node.
func (c *InsertTextCommand) GetNodesInCommand(nodes map[dom.Node]struct{}) {
	if c.Target != nil {
		nodes[c.Target] = struct{}{}
	}
}

// InsertTextWithoutSanitization is a helper used by TypingCommand to insert
// text directly without the whitespace-rebalancing pass, mirroring
// InsertTextCommand::insertText withoutSanitization.
func InsertTextWithoutSanitization(target *dom.Element, offset int, text string) {
	if target == nil {
		return
	}
	current := target.TextContent()
	if offset < 0 {
		offset = 0
	}
	if offset > len(current) {
		offset = len(current)
	}
	runes := []rune(current)
	textRunes := []rune(text)
	newRunes := append(append([]rune{}, runes[:offset]...), append(textRunes, runes[offset:]...)...)
	target.SetTextContent(string(newRunes))
}

// DeleteTextRange deletes the range [from, to) within target's text content
// and returns the deleted string (for undo). This is a helper shared by
// DeleteKeyCommand and ForwardDeleteKeyCommand.
func DeleteTextRange(target *dom.Element, from, to int) string {
	if target == nil || from >= to {
		return ""
	}
	current := target.TextContent()
	runes := []rune(current)
	if from < 0 {
		from = 0
	}
	if to > len(runes) {
		to = len(runes)
	}
	if from >= to {
		return ""
	}
	deleted := string(runes[from:to])
	newRunes := append(append([]rune{}, runes[:from]...), runes[to:]...)
	target.SetTextContent(string(newRunes))
	return deleted
}

// InsertTextAtEnd appends text to the end of the target's text content.
// Used when no explicit caret position is tracked.
func InsertTextAtEnd(target *dom.Element, text string) {
	current := target.TextContent()
	target.SetTextContent(strings.Join([]string{current, text}, ""))
}
