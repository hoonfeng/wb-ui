// Translation of: Source/WebCore/editing/TypingCommand.h
//                  Source/WebCore/editing/TypingCommand.cpp
// Completeness: 55%
// Simplifications:
//   - operates on a generic dom.Element plain-text editable scope (treats
//     the element's text content as the buffer); the full WebKit version
//     operates on the live DOM Selection inside a contenteditable element
//   - "open for more typing" semantics are preserved: when the user presses
//     a sequence of keys, consecutive insertions are merged into a single
//     TypingCommand that is committed to the undo stack as a single step
//     when closeTyping() is called (typically on cursor movement, blur, or
//     a non-typing command)
//   - the marked-text / autocorrection / smart-replace features are omitted
//   - whitelist of TypingCommand::Type values: DeleteSelection, DeleteKey,
//     ForwardDeleteKey, InsertText, InsertLineBreak, InsertParagraphSeparator
//   - the Option bit-mask (SelectInsertedText / AddsToKillRing /
//     RetainAutocorrectionIndicator / PreventSpellChecking / SmartDelete /
//     IsAutocompletion) is preserved but only a subset is honored in this port

package editing

import "wb-ui/engine/dom"

// TypingCommandType mirrors WebCore::TypingCommand::ETypingCommand.
// It classifies a typing operation.
type TypingCommandType uint8

const (
	// TypingDeleteSelection deletes the current selection, mirroring
	// DeleteSelection.
	TypingDeleteSelection TypingCommandType = iota
	// TypingDeleteKey deletes one character before the caret (backspace),
	// mirroring DeleteKey.
	TypingDeleteKey
	// TypingForwardDeleteKey deletes one character after the caret, mirroring
	// ForwardDeleteKey.
	TypingForwardDeleteKey
	// TypingInsertText inserts text at the caret, mirroring InsertText.
	TypingInsertText
	// TypingInsertLineBreak inserts a line break ("\n"), mirroring
	// InsertLineBreak.
	TypingInsertLineBreak
	// TypingInsertParagraphSeparator inserts a paragraph separator ("\n"),
	// mirroring InsertParagraphSeparator. In the plain-text editable scope
	// this is the same as InsertLineBreak.
	TypingInsertParagraphSeparator
	// TypingInsertParagraphSeparatorInQuotedContent is reserved for
	// mail quoting; treated the same as InsertParagraphSeparator here.
	TypingInsertParagraphSeparatorInQuotedContent
)

// TypingCommandOption mirrors WebCore::TypingCommand::ETypingCommand
// options (the bit-mask enum).
type TypingCommandOption uint32

const (
	// OptionSelectInsertedText mirrors SelectInsertedText.
	OptionSelectInsertedText TypingCommandOption = 1 << iota
	// OptionAddsToKillRing mirrors AddsToKillRing.
	OptionAddsToKillRing
	// OptionRetainAutocorrectionIndicator mirrors RetainAutocorrectionIndicator.
	OptionRetainAutocorrectionIndicator
	// OptionPreventSpellChecking mirrors PreventSpellChecking.
	OptionPreventSpellChecking
	// OptionSmartDelete mirrors SmartDelete.
	OptionSmartDelete
	// OptionIsAutocompletion mirrors IsAutocompletion.
	OptionIsAutocompletion
)

// TypingCommandOptions is a convenience alias for a bitmask of options.
type TypingCommandOptions = TypingCommandOption

// TypingCommand is the Go translation of WebCore::TypingCommand. It is a
// CompositeEditCommand that records the user's recent typing activity on a
// single editable scope. Consecutive keystrokes within the same "typing
// session" are merged into a single TypingCommand so that the user perceives
// one "undo" step per typing session rather than one per keystroke.
//
// The "typing session" is opened when the first keystroke arrives and closed
// by closeTyping() (called by the Editor on selection change, blur, or any
// non-typing command).
//
// In this port the TypingCommand operates on a plain-text editable scope
// represented as a (dom.Element, caretOffset) pair: the element's text
// content is the buffer and caretOffset is the current character offset
// within it. The code editor (Phase 1+) uses its own document model and
// does not rely on TypingCommand; this struct is provided for contenteditable
// rich-text editing and future HTMLInputElement/HTMLTextAreaElement support
// (Phase 9).
type TypingCommand struct {
	CompositeEditCommandBase

	// Target is the editable element whose text content is the buffer.
	Target *dom.Element
	// CaretOffset is the current character offset within Target's text
	// content. Updated as the user types or deletes.
	CaretOffset int
	// CmdType records what kind of typing this command is. InsertText is the
	// most common; DeleteKey / ForwardDeleteKey / InsertParagraphSeparator
	// close the current typing session and start a new one.
	CmdType TypingCommandType
	// TextToInsert is the text for TypingInsertText.
	TextToInsert string
	// Options is the bit-mask of TypingCommandOption flags.
	Options TypingCommandOptions
	// isOpenForMoreTyping records whether this command is still accepting
	// additional keystrokes. Set false by closeTyping(). Accessed via the
	// IsOpenForMoreTyping() method (the field is private to avoid the
	// field/method name collision in Go).
	isOpenForMoreTyping bool
	// SelectionStart / SelectionEnd record a selection range that should be
	// deleted before inserting text (for TypingDeleteSelection followed by
	// TypingInsertText). -1 means no selection.
	SelectionStart int
	SelectionEnd   int
	// deletedText records the text removed by DeleteKey/ForwardDeleteKey/
	// DeleteSelection so that DoUnapply can re-insert it for perfect undo.
	deletedText string
}

// NewTypingCommand constructs a TypingCommand, mirroring TypingCommand::create.
// The command starts in the "open for more typing" state.
func NewTypingCommand(doc *dom.Document, target *dom.Element, caret int, cmdType TypingCommandType, text string, options TypingCommandOptions) *TypingCommand {
	return &TypingCommand{
		CompositeEditCommandBase: NewCompositeBase(doc),
		Target:                   target,
		CaretOffset:              caret,
		CmdType:                  cmdType,
		TextToInsert:             text,
		Options:                  options,
		isOpenForMoreTyping:      true,
		SelectionStart:           -1,
		SelectionEnd:             -1,
	}
}

// DoApply applies the typing operation, mirroring TypingCommand::doApply.
//
// For InsertText, the text is appended at the caret and the caret advances.
// For InsertLineBreak / InsertParagraphSeparator, "\n" is inserted.
// For DeleteKey, one character before the caret is removed and the caret
// retreats by one.
// For ForwardDeleteKey, one character after the caret is removed.
// For DeleteSelection, the range [SelectionStart, SelectionEnd) is removed
// and the caret is set to SelectionStart.
func (t *TypingCommand) DoApply() error {
	switch t.CmdType {
	case TypingInsertText:
		InsertTextWithoutSanitization(t.Target, t.CaretOffset, t.TextToInsert)
		t.CaretOffset += len([]rune(t.TextToInsert))
	case TypingInsertLineBreak, TypingInsertParagraphSeparator, TypingInsertParagraphSeparatorInQuotedContent:
		InsertTextWithoutSanitization(t.Target, t.CaretOffset, "\n")
		t.CaretOffset++
	case TypingDeleteKey:
		if t.CaretOffset > 0 {
			t.deletedText = DeleteTextRange(t.Target, t.CaretOffset-1, t.CaretOffset)
			t.CaretOffset--
		}
	case TypingForwardDeleteKey:
		t.deletedText = DeleteTextRange(t.Target, t.CaretOffset, t.CaretOffset+1)
	case TypingDeleteSelection:
		if t.SelectionStart >= 0 && t.SelectionEnd > t.SelectionStart {
			t.deletedText = DeleteTextRange(t.Target, t.SelectionStart, t.SelectionEnd)
			t.CaretOffset = t.SelectionStart
		}
	}
	return nil
}

// DoUnapply reverses the typing operation, mirroring TypingCommand::doUnapply.
// Because the TypingCommand's sub-commands are not individually tracked in
// this minimal port (we apply mutations inline), DoUnapply only knows how to
// reverse the original CmdType-based operation by undoing the buffer change.
func (t *TypingCommand) DoUnapply() error {
	// Reverse by applying the inverse operation at the current caret.
	switch t.CmdType {
	case TypingInsertText:
		// Undo: delete the inserted text.
		insertedLen := len([]rune(t.TextToInsert))
		DeleteTextRange(t.Target, t.CaretOffset-insertedLen, t.CaretOffset)
		t.CaretOffset -= insertedLen
	case TypingInsertLineBreak, TypingInsertParagraphSeparator, TypingInsertParagraphSeparatorInQuotedContent:
		DeleteTextRange(t.Target, t.CaretOffset-1, t.CaretOffset)
		t.CaretOffset--
	case TypingDeleteKey:
		// Re-insert the deleted character before the caret and advance.
		if t.deletedText != "" {
			InsertTextWithoutSanitization(t.Target, t.CaretOffset, t.deletedText)
			t.CaretOffset += len([]rune(t.deletedText))
		}
	case TypingForwardDeleteKey:
		// Re-insert the deleted character at the caret; caret stays.
		if t.deletedText != "" {
			InsertTextWithoutSanitization(t.Target, t.CaretOffset, t.deletedText)
		}
	case TypingDeleteSelection:
		// Re-insert the deleted range at SelectionStart.
		if t.deletedText != "" {
			InsertTextWithoutSanitization(t.Target, t.SelectionStart, t.deletedText)
			t.CaretOffset = t.SelectionStart + len([]rune(t.deletedText))
		}
	}
	return nil
}

// DoReapply re-applies the typing operation after an undo, mirroring
// TypingCommand::doReapply.
func (t *TypingCommand) DoReapply() error {
	// Reset caret to starting position before reapplying. We approximate this
	// by resetting to the caret at the time the command was first applied.
	// For InsertText this means: caret -= insertedLen, then DoApply.
	switch t.CmdType {
	case TypingInsertText:
		t.CaretOffset -= len([]rune(t.TextToInsert))
	case TypingInsertLineBreak, TypingInsertParagraphSeparator, TypingInsertParagraphSeparatorInQuotedContent:
		t.CaretOffset--
	}
	return t.DoApply()
}

// EditingAction returns the EditAction for this typing command.
func (t *TypingCommand) EditingAction() EditAction {
	switch t.CmdType {
	case TypingInsertText:
		return EditActionInsertText
	case TypingInsertLineBreak:
		return EditActionInsertLineBreak
	case TypingInsertParagraphSeparator, TypingInsertParagraphSeparatorInQuotedContent:
		return EditActionInsertParagraphSeparator
	case TypingDeleteKey:
		return EditActionDeleteKey
	case TypingForwardDeleteKey:
		return EditActionForwardDeleteKey
	case TypingDeleteSelection:
		return EditActionDeleteSelection
	}
	return EditActionUnspecified
}

// IsPreservesTypingStyle is the Go form of TypingCommand::preservesTypingStyle().
// TypingCommand does not preserve typing style; the default is fine.
func (t *TypingCommand) IsPreservesTypingStyle() bool { return false }

// MarkApplied registers the command as having been applied. In this port it
// just clears the IsOpenForMoreTyping flag if the command type requires a
// new session (DeleteKey / ForwardDeleteKey / InsertParagraphSeparator
// close the session; InsertText keeps it open).
func (t *TypingCommand) MarkApplied() {
	switch t.CmdType {
	case TypingDeleteKey, TypingForwardDeleteKey,
		TypingInsertLineBreak, TypingInsertParagraphSeparator,
		TypingInsertParagraphSeparatorInQuotedContent:
		t.isOpenForMoreTyping = false
	}
}

// CloseTyping marks the command as no longer accepting further keystrokes,
// mirroring TypingCommand::closeTyping.
func (t *TypingCommand) CloseTyping() {
	t.isOpenForMoreTyping = false
}

// IsOpenForMoreTyping reports whether the command is still accepting
// additional keystrokes, mirroring TypingCommand::isOpenForMoreTyping.
func (t *TypingCommand) IsOpenForMoreTyping() bool {
	return t.isOpenForMoreTyping
}

// LastTypingCommandIfStillOpenForTyping returns the last TypingCommand on
// the Editor's undo stack if it is still open for more typing, mirroring
// TypingCommand::lastTypingCommandIfStillOpenForTyping(). Returns nil if no
// open typing command exists.
//
// The full WebKit version also checks that the command's edit action and
// selection are compatible with continuing the same typing session; this port
// only checks the IsOpenForMoreTyping flag and that the target element
// matches.
func LastTypingCommandIfStillOpenForTyping(editor *Editor, target *dom.Element) *TypingCommand {
	if editor == nil {
		return nil
	}
	last := editor.LastTypingCommand()
	if last == nil || !last.IsOpenForMoreTyping() {
		return nil
	}
	if target != nil && last.Target != target {
		return nil
	}
	return last
}

// InsertTextStatic is the static entry point for inserting text, mirroring
// TypingCommand::insertText(doc, text, options, compositionType). It either
// appends to an existing open TypingCommand or creates a new one.
//
// In WebKit this method goes through the Editor's typing command machinery
// (Editor::insertText → TypingCommand::insertText); this port exposes it as
// a free function that the Editor calls directly.
func InsertTextStatic(editor *Editor, target *dom.Element, caret int, text string, options TypingCommandOptions) *TypingCommand {
	if editor == nil {
		return nil
	}
	if existing := LastTypingCommandIfStillOpenForTyping(editor, target); existing != nil {
		existing.TextToInsert += text
		// Insert only the newly typed text at the current caret; re-running
		// DoApply would re-insert the entire accumulated TextToInsert.
		InsertTextWithoutSanitization(existing.Target, existing.CaretOffset, text)
		existing.CaretOffset += len([]rune(text))
		return existing
	}
	cmd := NewTypingCommand(editor.Document(), target, caret, TypingInsertText, text, options)
	_ = cmd.DoApply()
	editor.RecordTypingCommand(cmd)
	return cmd
}

// DeleteSelectionStatic is the static entry point for deleting the current
// selection, mirroring TypingCommand::deleteSelection(doc, options).
func DeleteSelectionStatic(editor *Editor, target *dom.Element, selStart, selEnd int, options TypingCommandOptions) *TypingCommand {
	if editor == nil {
		return nil
	}
	cmd := NewTypingCommand(editor.Document(), target, selStart, TypingDeleteSelection, "", options)
	cmd.SelectionStart = selStart
	cmd.SelectionEnd = selEnd
	_ = cmd.DoApply()
	// Record the new command first (pushing any prior open typing command
	// onto the undo stack), then immediately close it so that each
	// Delete/ForwardDelete/ParagraphSeparator is its own undo step and the
	// next keystroke starts a fresh typing session.
	editor.RecordTypingCommand(cmd)
	editor.CloseTyping()
	return cmd
}

// DeleteKeyPressedStatic is the static entry point for backspace, mirroring
// TypingCommand::deleteKeyPressed.
func DeleteKeyPressedStatic(editor *Editor, target *dom.Element, caret int, options TypingCommandOptions) *TypingCommand {
	if editor == nil {
		return nil
	}
	cmd := NewTypingCommand(editor.Document(), target, caret, TypingDeleteKey, "", options)
	_ = cmd.DoApply()
	// Record the new command first (pushing any prior open typing command
	// onto the undo stack), then immediately close it so that each
	// Delete/ForwardDelete/ParagraphSeparator is its own undo step and the
	// next keystroke starts a fresh typing session.
	editor.RecordTypingCommand(cmd)
	editor.CloseTyping()
	return cmd
}

// ForwardDeleteKeyPressedStatic is the static entry point for forward delete,
// mirroring TypingCommand::forwardDeleteKeyPressed.
func ForwardDeleteKeyPressedStatic(editor *Editor, target *dom.Element, caret int, options TypingCommandOptions) *TypingCommand {
	if editor == nil {
		return nil
	}
	cmd := NewTypingCommand(editor.Document(), target, caret, TypingForwardDeleteKey, "", options)
	_ = cmd.DoApply()
	// Record the new command first (pushing any prior open typing command
	// onto the undo stack), then immediately close it so that each
	// Delete/ForwardDelete/ParagraphSeparator is its own undo step and the
	// next keystroke starts a fresh typing session.
	editor.RecordTypingCommand(cmd)
	editor.CloseTyping()
	return cmd
}

// InsertParagraphSeparatorStatic is the static entry point for Enter, mirroring
// TypingCommand::insertParagraphSeparator.
func InsertParagraphSeparatorStatic(editor *Editor, target *dom.Element, caret int, options TypingCommandOptions) *TypingCommand {
	if editor == nil {
		return nil
	}
	cmd := NewTypingCommand(editor.Document(), target, caret, TypingInsertParagraphSeparator, "", options)
	_ = cmd.DoApply()
	// Record the new command first (pushing any prior open typing command
	// onto the undo stack), then immediately close it so that each
	// Delete/ForwardDelete/ParagraphSeparator is its own undo step and the
	// next keystroke starts a fresh typing session.
	editor.RecordTypingCommand(cmd)
	editor.CloseTyping()
	return cmd
}
