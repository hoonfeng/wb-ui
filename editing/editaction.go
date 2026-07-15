// Translation of: Source/WebCore/editing/EditAction.h
// Completeness: 70%
// Simplifications:
//   - the C++ enum class EditAction is modeled as a Go iota-based integer enum
//   - the deprecated EditAction::Unspecified is preserved as the zero value
//   - inputType string mapping (insertText/deleteContentBackward/...) is provided
//     as InputTypeFor() to align with DOM InputEvent.inputType

package editing

// EditAction mirrors WebCore::EditAction. It classifies an editing operation
// for the purposes of undo/redo grouping, input event dispatch, and accessibility
// announcements. The values are intentionally aligned with the C++ enum so that
// mapping tables stay 1:1 with WebKit.
type EditAction int

const (
	// EditActionUnspecified is the zero value used when no specific action is
	// known. Matches EditAction::Unspecified.
	EditActionUnspecified EditAction = iota

	// Insert / delete typing actions. These are produced by TypingCommand.
	EditActionInsertText
	EditActionInsertParagraphSeparator
	EditActionInsertLineBreak
	EditActionDeleteKey
	EditActionForwardDeleteKey
	EditActionDeleteSelection

	// Clipboard actions.
	EditActionCopy
	EditActionCut
	EditActionPaste
	EditActionPasteAndMatchStyle

	// Style / formatting commands (rich text editing; not used by the code
	// editor but kept for completeness so that the same enum serves a future
	// contenteditable implementation).
	EditActionBold
	EditActionItalic
	EditActionUnderline
	EditActionStrikeThrough
	EditActionSubscript
	EditActionSuperscript
	EditActionSetFont
	EditActionSetForegroundColor
	EditActionSetBackgroundColor
	EditActionRemoveFormat
	EditActionCreateLink
	EditActionUnlink

	// Block-level formatting commands.
	EditActionFormatBlock
	EditActionIndent
	EditActionOutdent
	EditActionInsertOrderedList
	EditActionInsertUnorderedList
	EditActionJustifyCenter
	EditActionJustifyFull
	EditActionJustifyLeft
	EditActionJustifyRight

	// Selection / navigation commands.
	EditActionSelectAll
	EditActionSelectNone

	// Undo / redo.
	EditActionUndo
	EditActionRedo
)

// InputTypeFor returns the DOM InputEvent.inputType string that corresponds to
// the given EditAction, mirroring the mapping maintained by WebKit's
// Editor::inputEventDataNameForActionAndTypingStyle(). This is used when
// dispatching beforeinput / input events so JS listeners see the same
// inputType values that browsers produce.
func InputTypeFor(action EditAction) string {
	switch action {
	case EditActionInsertText:
		return "insertText"
	case EditActionInsertParagraphSeparator:
		return "insertParagraph"
	case EditActionInsertLineBreak:
		return "insertLineBreak"
	case EditActionDeleteKey:
		return "deleteContentBackward"
	case EditActionForwardDeleteKey:
		return "deleteContentForward"
	case EditActionDeleteSelection:
		return "deleteByCut"
	case EditActionPaste:
		return "insertFromPaste"
	case EditActionPasteAndMatchStyle:
		return "insertFromPaste"
	case EditActionCopy:
		return "copy"
	case EditActionCut:
		return "deleteByCut"
	case EditActionBold:
		return "formatBold"
	case EditActionItalic:
		return "formatItalic"
	case EditActionUnderline:
		return "formatUnderline"
	case EditActionStrikeThrough:
		return "formatStrikeThrough"
	case EditActionSubscript:
		return "formatSubscript"
	case EditActionSuperscript:
		return "formatSuperscript"
	case EditActionSetFont:
		return "formatFontName"
	case EditActionSetForegroundColor:
		return "formatFontColor"
	case EditActionSetBackgroundColor:
		return "formatBackColor"
	case EditActionRemoveFormat:
		return "formatRemove"
	case EditActionCreateLink:
		return "insertLink"
	case EditActionUnlink:
		return "formatUnlink"
	case EditActionFormatBlock:
		return "formatBlock"
	case EditActionIndent:
		return "formatIndent"
	case EditActionOutdent:
		return "formatOutdent"
	case EditActionInsertOrderedList:
		return "insertOrderedList"
	case EditActionInsertUnorderedList:
		return "insertUnorderedList"
	case EditActionJustifyCenter:
		return "formatJustifyCenter"
	case EditActionJustifyFull:
		return "formatJustifyFull"
	case EditActionJustifyLeft:
		return "formatJustifyLeft"
	case EditActionJustifyRight:
		return "formatJustifyRight"
	case EditActionSelectAll:
		return "selectAll"
	case EditActionSelectNone:
		return "selectNone"
	case EditActionUndo:
		return "historyUndo"
	case EditActionRedo:
		return "historyRedo"
	default:
		return ""
	}
}
