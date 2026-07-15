// Translation of: Source/WebCore/editing/Editor.h  (composition subset)
//                  Source/WebCore/editing/Editor.cpp (composition subset)
// Completeness: 65%
// Simplifications:
//   - the IME composition state management is fully translated
//   - the undo/redo stack is a simple slice of UndoStep (EditCommandComposition
//     snapshots); the full WebKit UndoManager integration is deferred
//   - composition is stored as a string + offset on the Editor rather than as
//     a live Text node reference with a CharacterRange, because this port's
//     DOM selection / editing infrastructure is minimal
//   - setComposition / confirmComposition / cancelComposition operate on the
//     composition string directly and dispatch CompositionEvents to the
//     focused element; they also spawn TypingCommand for proper undo grouping
//
// The Editor owns the composition state for a Document. When an IME sends
// composition updates (intermediate pinyin/kana text), the platform IME
// handler calls SetComposition; when the user confirms or cancels, the handler
// calls ConfirmComposition or CancelComposition. The Editor dispatches the
// appropriate CompositionEvents (compositionstart / compositionupdate /
// compositionend) to the focused element so that JS listeners and the DOM
// reflect the composition lifecycle.

package editing

import (
	"wb-ui/dom"
)

// SetCompositionMode mirrors the private Editor::SetCompositionMode enum
// used by Editor::setComposition(text, mode).
type SetCompositionMode int

const (
	// ConfirmComposition mode means the composition text is committed as
	// final input (compositionend dispatched with the committed text).
	ConfirmComposition SetCompositionMode = iota
	// CancelComposition mode means the composition is discarded
	// (compositionend dispatched with empty data).
	CancelComposition
)

// Editor is the Go translation of the IME-relevant subset of WebCore::Editor.
// It tracks the active composition string, its selection range (the highlighted
// segment within the composition), and custom underlines/highlights requested
// by the IME. It dispatches CompositionEvents to the focused element when
// composition state changes.
type Editor struct {
	// client is the EditorClient that receives platform notifications,
	// mirroring Editor::m_client.
	client EditorClient

	// document is the owning document, mirroring Editor::m_document.
	document *dom.Document

	// focused is the currently focused element, set by the platform layer
	// (window/event dispatcher). In WebKit this is resolved via the
	// FocusController; this port tracks it explicitly since there is no
	// FocusController.
	focused *dom.Element

	// compositionNode is the focused element that owns the active
	// composition. In WebKit this is a Text*; in this port it is the
	// editable Element (input/textarea/contenteditable). A nil value
	// means no composition is active, mirroring hasComposition().
	compositionNode *dom.Element

	// compositionStart / compositionEnd are the character offsets of the
	// composition selection within the composition string, mirroring
	// Editor::m_compositionStart / m_compositionEnd.
	compositionStart uint
	compositionEnd   uint

	// compositionText is the current intermediate composition string
	// (e.g. the pinyin letters being typed before a candidate is chosen).
	compositionText string

	// customCompositionUnderlines are the underlines requested by the IME
	// for the active composition, mirroring m_customCompositionUnderlines.
	customCompositionUnderlines []CompositionUnderline

	// customCompositionHighlights are the highlights requested by the IME,
	// mirroring m_customCompositionHighlights.
	customCompositionHighlights []CompositionHighlight

	// lastTypingCommand is the most recently created TypingCommand that is
	// still open for more typing, mirroring TypingCommand::lastTypingCommand.
	// When the user types another character, the text is appended to this
	// command rather than creating a new one; this is what makes consecutive
	// keystrokes a single undo step.
	lastTypingCommand *TypingCommand

	// undoStack / redoStack are the undo/redo stacks, mirroring
	// Editor::m_undoStack / m_redoStack (which in WebKit delegate to
	// UndoManager). Each entry is an UndoStep snapshot (typically an
	// EditCommandComposition wrapping a TypingCommand or other composite).
	undoStack []UndoStep
	// redoStack is cleared whenever a new command is executed.
	redoStack []UndoStep
}

// SetFocusedElement records the currently focused element, mirroring how
// WebKit's FocusController notifies the Editor of focus changes. The platform
// event dispatcher calls this when focus changes so that subsequent
// composition events are dispatched to the right target.
func (e *Editor) SetFocusedElement(el *dom.Element) {
	e.focused = el
	if e.client != nil {
		e.client.SetInputMethodState(el)
	}
}

// FocusedElement returns the currently focused element, or nil.
func (e *Editor) FocusedElement() *dom.Element { return e.focused }

// NewEditor constructs an Editor for the given document and client, mirroring
// the Editor constructor that receives a reference to its Document and a
// WeakPtr<EditorClient>.
func NewEditor(doc *dom.Document, client EditorClient) *Editor {
	return &Editor{
		document: doc,
		client:   client,
	}
}

// Client returns the EditorClient, mirroring Editor::client().
func (e *Editor) Client() EditorClient { return e.client }

// Document returns the owning document, mirroring Editor::document().
func (e *Editor) Document() *dom.Document { return e.document }

// HasComposition reports whether an IME composition is active, mirroring
// Editor::hasComposition() (which tests m_compositionNode).
func (e *Editor) HasComposition() bool { return e.compositionNode != nil }

// CompositionText returns the current composition string, mirroring
// Editor::compositionText(). Returns empty when no composition is active.
func (e *Editor) CompositionText() string {
	if !e.HasComposition() {
		return ""
	}
	return e.compositionText
}

// CompositionRange returns the start/end offsets of the composition
// selection, mirroring Editor::getCompositionSelection(). Returns false
// when no composition is active.
func (e *Editor) CompositionRange() (start, end uint, ok bool) {
	if !e.HasComposition() {
		return 0, 0, false
	}
	return e.compositionStart, e.compositionEnd, true
}

// CustomCompositionUnderlines returns the underlines for the active
// composition, mirroring Editor::customCompositionUnderlines().
func (e *Editor) CustomCompositionUnderlines() []CompositionUnderline {
	return e.customCompositionUnderlines
}

// CustomCompositionHighlights returns the highlights for the active
// composition, mirroring Editor::customCompositionHighlights().
func (e *Editor) CustomCompositionHighlights() []CompositionHighlight {
	return e.customCompositionHighlights
}

// SetComposition updates the active composition string and dispatches
// compositionstart (on first composition) / compositionupdate events to the
// focused element, mirroring Editor::setComposition(text, underlines,
// highlights, annotations, selectionStart, selectionEnd).
//
// The underlines and highlights describe how the IME wants the composition
// text to be styled. selectionStart/selectionEnd indicate the IME's caret
// or selected segment within the composition string.
func (e *Editor) SetComposition(text string, underlines []CompositionUnderline, highlights []CompositionHighlight, selectionStart, selectionEnd uint) {
	if e.document == nil {
		return
	}
	focused := e.focusedElement()
	if focused == nil {
		return
	}

	if !e.HasComposition() {
		// compositionstart: composition begins.
		e.compositionNode = focused
		e.dispatchCompositionEvent(dom.EventCompositionStart, text)
	} else {
		// compositionupdate: intermediate string changed.
		e.dispatchCompositionEvent(dom.EventCompositionUpdate, text)
	}

	e.compositionText = text
	e.compositionStart = selectionStart
	e.compositionEnd = selectionEnd
	e.customCompositionUnderlines = underlines
	e.customCompositionHighlights = highlights

	if e.client != nil {
		e.client.DidUpdateComposition()
	}
}

// ConfirmComposition commits the current composition as final text and
// dispatches compositionend, mirroring Editor::confirmComposition().
// If no composition is active this is a no-op.
func (e *Editor) ConfirmComposition() {
	if !e.HasComposition() {
		return
	}
	e.setCompositionInternal(e.compositionText, ConfirmComposition)
}

// ConfirmCompositionWithText commits the given text as the final composition
// result (replacing the current composition string), mirroring
// Editor::confirmComposition(const String& text). If no composition is
// active, this replaces the selection.
func (e *Editor) ConfirmCompositionWithText(text string) {
	e.setCompositionInternal(text, ConfirmComposition)
}

// CancelComposition discards the current composition and dispatches
// compositionend with empty data, mirroring Editor::cancelComposition().
func (e *Editor) CancelComposition() {
	if !e.HasComposition() {
		return
	}
	e.setCompositionInternal("", CancelComposition)
}

// setCompositionInternal is the shared path for confirm/cancel, mirroring
// Editor::setComposition(text, SetCompositionMode). It clears the
// composition state and dispatches compositionend.
func (e *Editor) setCompositionInternal(text string, mode SetCompositionMode) {
	prevNode := e.compositionNode
	e.compositionNode = nil
	e.customCompositionUnderlines = nil
	e.customCompositionHighlights = nil
	e.compositionText = ""

	if prevNode != nil {
		endData := text
		if mode == CancelComposition {
			endData = ""
		}
		e.dispatchCompositionEvent(dom.EventCompositionEnd, endData)
	}

	if e.client != nil {
		if mode == CancelComposition {
			e.client.CanceledComposition()
		} else {
			e.client.DiscardedComposition(e.document)
		}
	}
}

// focusedElement returns the currently focused element, or nil if none.
// In WebKit this goes through the FocusController; this port uses the
// focused field set by SetFocusedElement.
func (e *Editor) focusedElement() *dom.Element {
	return e.focused
}

// dispatchCompositionEvent builds and dispatches a CompositionEvent to the
// focused element, mirroring the event dispatch in Editor::setComposition.
func (e *Editor) dispatchCompositionEvent(typ string, data string) {
	if e.document == nil {
		return
	}
	target := e.focusedElement()
	if target == nil {
		return
	}
	ev := dom.NewCompositionEvent(typ, data)
	target.DispatchEvent(ev)
}

// ===== Undo / Redo / TypingCommand machinery =====
//
// The following methods implement the subset of Editor that manages typing
// commands and the undo/redo stack. They mirror the corresponding WebKit
// methods in Editor.cpp but use a simpler stack model:
//
//   - ExecuteCommand applies a command and pushes a snapshot onto undoStack
//   - TypingCommand is special: consecutive InsertText calls are merged
//     into a single command via lastTypingCommand; only when closeTyping()
//     is called (cursor move, blur, non-typing command) is the snapshot
//     pushed onto the undo stack
//   - Undo pops from undoStack, applies Unapply, pushes onto redoStack
//   - Redo pops from redoStack, applies Reapply, pushes onto undoStack

// LastTypingCommand returns the most recently created TypingCommand that is
// still open for more typing, mirroring TypingCommand::lastTypingCommand.
// Returns nil if there is no open typing command.
func (e *Editor) LastTypingCommand() *TypingCommand {
	return e.lastTypingCommand
}

// RecordTypingCommand registers a typing command as the "current open" command,
// mirroring Editor::registerTypingCommand. If a previous typing command is
// still open, it is closed (its snapshot is pushed onto the undo stack)
// before the new one becomes current.
func (e *Editor) RecordTypingCommand(cmd *TypingCommand) {
	if cmd == nil {
		return
	}
	// Close any prior open typing command first.
	if e.lastTypingCommand != nil && e.lastTypingCommand.IsOpenForMoreTyping() {
		e.closeTypingInternal()
	}
	e.lastTypingCommand = cmd
}

// CloseTyping closes the currently open typing command (if any), committing
// it to the undo stack as a single UndoStep. Mirrors Editor::closeTyping().
// Called by the platform layer when the user moves the caret, blurs the
// editable element, or invokes a non-typing command.
func (e *Editor) CloseTyping() {
	e.closeTypingInternal()
}

// closeTypingInternal is the shared path for CloseTyping and the auto-close
// in RecordTypingCommand.
func (e *Editor) closeTypingInternal() {
	if e.lastTypingCommand == nil {
		return
	}
	if e.lastTypingCommand.IsOpenForMoreTyping() {
		e.lastTypingCommand.CloseTyping()
	}
	// Push a snapshot onto the undo stack. We only push if the command
	// actually performed work (has sub-commands or performed inline edits);
	// since TypingCommand in this port performs inline edits, we always push.
	step := Wrap(e.lastTypingCommand, "Typing")
	e.pushUndoStep(step)
	e.lastTypingCommand = nil
}

// ExecuteCommand applies a non-typing EditCommand and pushes its snapshot
// onto the undo stack, mirroring Editor::executeCommand. Any currently open
// typing command is closed first.
func (e *Editor) ExecuteCommand(cmd EditCommand) error {
	if cmd == nil {
		return nil
	}
	// Close any open typing command before applying a non-typing command.
	e.CloseTyping()
	if err := cmd.DoApply(); err != nil {
		return err
	}
	// Push a snapshot. For CompositeEditCommand we wrap into an
	// EditCommandComposition; for SimpleEditCommand we wrap into a tiny
	// composite containing just the one command.
	var step UndoStep
	if composite, ok := cmd.(CompositeEditCommand); ok {
		step = Wrap(composite, "Edit")
	} else {
		// Wrap the simple command into a synthetic composite for undo purposes.
		synthetic := NewCompositeBase(e.document)
		synthetic.Commands_ = []EditCommand{cmd}
		wrappedComposite := &syntheticComposite{CompositeEditCommandBase: synthetic}
		step = Wrap(wrappedComposite, "Edit")
	}
	e.pushUndoStep(step)
	return nil
}

// syntheticComposite is a minimal CompositeEditCommand used to wrap a
// SimpleEditCommand for undo purposes. It has no custom Apply behavior; the
// default ApplyAll iterates sub-commands.
type syntheticComposite struct {
	CompositeEditCommandBase
}

// DoApply delegates to ApplyAll (the default for composites).
func (s *syntheticComposite) DoApply() error { return s.ApplyAll() }

// DoUnapply delegates to UnapplyAll.
func (s *syntheticComposite) DoUnapply() error { return s.UnapplyAll() }

// DoReapply delegates to ReapplyAll.
func (s *syntheticComposite) DoReapply() error { return s.ReapplyAll() }

// EditingAction returns the wrapped command's action if it has one.
func (s *syntheticComposite) EditingAction() EditAction {
	if len(s.Commands_) > 0 {
		return s.Commands_[0].EditingAction()
	}
	return EditActionUnspecified
}

// pushUndoStep pushes an UndoStep onto the undo stack and clears the redo
// stack (the standard undo/redo invariant).
func (e *Editor) pushUndoStep(step UndoStep) {
	e.undoStack = append(e.undoStack, step)
	e.redoStack = e.redoStack[:0] // clear
}

// Undo pops the most recent UndoStep from the undo stack, calls Unapply, and
// pushes the step onto the redo stack. Mirrors Editor::undo(). Returns false
// if the undo stack is empty.
func (e *Editor) Undo() bool {
	if len(e.undoStack) == 0 {
		return false
	}
	// Close any open typing command first (it shouldn't be undone as a
	// separate partial step).
	e.CloseTyping()
	idx := len(e.undoStack) - 1
	step := e.undoStack[idx]
	e.undoStack = e.undoStack[:idx]
	if err := step.Unapply(); err != nil {
		// On error, push back onto undo stack so the user can retry.
		e.undoStack = append(e.undoStack, step)
		return false
	}
	e.redoStack = append(e.redoStack, step)
	return true
}

// Redo pops the most recent UndoStep from the redo stack, calls Reapply, and
// pushes the step onto the undo stack. Mirrors Editor::redo(). Returns false
// if the redo stack is empty.
func (e *Editor) Redo() bool {
	if len(e.redoStack) == 0 {
		return false
	}
	idx := len(e.redoStack) - 1
	step := e.redoStack[idx]
	e.redoStack = e.redoStack[:idx]
	if err := step.Reapply(); err != nil {
		e.redoStack = append(e.redoStack, step)
		return false
	}
	e.undoStack = append(e.undoStack, step)
	return true
}

// CanUndo reports whether the undo stack is non-empty.
func (e *Editor) CanUndo() bool { return len(e.undoStack) > 0 }

// CanRedo reports whether the redo stack is non-empty.
func (e *Editor) CanRedo() bool { return len(e.redoStack) > 0 }

// ClearUndoRedo clears both the undo and redo stacks, mirroring
// Editor::clearUndoRedoOperations.
func (e *Editor) ClearUndoRedo() {
	e.undoStack = e.undoStack[:0]
	e.redoStack = e.redoStack[:0]
	e.lastTypingCommand = nil
}

// UndoStackSize returns the number of undo steps (for testing/UI).
func (e *Editor) UndoStackSize() int { return len(e.undoStack) }

// RedoStackSize returns the number of redo steps (for testing/UI).
func (e *Editor) RedoStackSize() int { return len(e.redoStack) }
