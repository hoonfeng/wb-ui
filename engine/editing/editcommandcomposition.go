// Translation of: Source/WebCore/editing/EditCommandComposition.h
//                  Source/WebCore/editing/EditCommandComposition.cpp
// Completeness: 50%
// Simplifications:
//   - EditCommandComposition is a CompositeEditCommand that represents a
//     committed snapshot on the undo stack; in WebKit it implements the
//     UndoStep interface and is held by UndoManager. In this port UndoManager
//     is the Editor's own (UndoStack/RedoStack) slice, and UndoStep is a Go
//     interface that EditCommandComposition satisfies
//   - the composition records the starting/ending selection of the wrapped
//     composite so that the selection can be restored on undo/redo
//   - composite commands are not "closed" via a separate method; once
//     finished() is called the composition is considered committed and
//     further ApplyCommand calls on the original composite will not affect
//     the snapshot (the snapshot takes a reference to the same Commands_
//     slice at finish() time)
//   - the C++ EditCommandComposition::unapply / reapply additionally dispatch
//     "input" / "beforeinput" events; this port defers that integration to
//     Phase 9 (form controls) where DOM InputEvent is translated

package editing

import "wb-ui/engine/dom"

// UndoStep is the Go translation of the UndoStep interface in
// Source/WebCore/editing/UndoStep.h. It is the contract that the Editor's
// undo/redo stack expects of a committed snapshot.
type UndoStep interface {
	// Unapply reverses the step, mirroring UndoStep::unapply().
	Unapply() error

	// Reapply re-applies the step, mirroring UndoStep::reapply().
	Reapply() error

	// Label returns the human-readable label for the step (e.g. "Typing"),
	// mirroring UndoStep::label().
	Label() string

	// SetLabel records a label for the step.
	SetLabel(string)

	// IsSimpleEditCommandForUndoReporting reports whether the step should
	// be reported as a "simple" undo in the UI (single keystroke vs grouped
	// action). Mirrors UndoStep::isSimpleEditCommandForUndoReporting().
	IsSimpleEditCommandForUndoReporting() bool
}

// EditCommandComposition is the Go translation of
// WebCore::EditCommandComposition. It wraps a CompositeEditCommand at the
// time it is committed to the undo stack, providing the UndoStep interface
// and recording the starting/ending selection for restoration.
//
// In WebKit the wrapping happens via EditCommandComposition::create(composite)
// which calls composite->setShouldRetainVisibleSelection(false) and then
// snapshots the composite's state. In this port the wrapping is explicit:
// when the Editor commits a TypingCommand (or any composite) it calls
// CommitComposition(composite) which constructs an EditCommandComposition
// referencing the same sub-command list and pushing it onto the undo stack.
type EditCommandComposition struct {
	CompositeEditCommandBase

	// label is the human-readable undo label.
	label string

	// wrapped is the original composite command that this composition
	// snapshots. DoUnapply/DoReapply delegate to it so that composites
	// which perform mutations inline (like TypingCommand) are properly
	// undone/redone, rather than only iterating the (possibly empty)
	// sub-command list.
	wrapped CompositeEditCommand

	// wasUndoReportingSimple mirrors
	// EditCommandComposition::m_isSimpleEditCommandForUndoReporting.
	wasUndoReportingSimple bool
}

// NewEditCommandComposition wraps the given composite command into an
// EditCommandComposition snapshot, mirroring EditCommandComposition::create.
//
// The snapshot takes a reference to the same Commands_ slice (Go slices are
// copy-on-write references, so mutations to the original composite's slice
// header will not affect the snapshot; but mutations to the underlying
// command objects themselves are shared, which matches WebKit's refcounted
// semantics).
func NewEditCommandComposition(doc *dom.Document, starting, ending *VisibleSelection, label string) *EditCommandComposition {
	c := &EditCommandComposition{
		CompositeEditCommandBase: NewCompositeBase(doc),
		label:                     label,
	}
	c.StartingSel = starting
	c.EndingSel = ending
	c.IsComposition = true
	return c
}

// Wrap wraps an existing composite into an EditCommandComposition snapshot,
// mirroring EditCommandComposition::create(composite). The wrapped composite's
// commands are shared by reference, and the composition retains a reference
// to the original composite so that DoUnapply/DoReapply can delegate to its
// inline undo/redo logic (needed by TypingCommand which mutates the DOM
// directly rather than via sub-commands).
func Wrap(composite CompositeEditCommand, label string) *EditCommandComposition {
	c := &EditCommandComposition{
		CompositeEditCommandBase: CompositeEditCommandBase{
			EditCommandBase: EditCommandBase{
				Document_:   composite.Document(),
				StartingSel: composite.StartingSelection(),
				EndingSel:   composite.EndingSelection(),
			},
			Commands_:      composite.Commands(),
			IsComposition:  true,
		},
		label:   label,
		wrapped: composite,
	}
	return c
}

// DoApply applies the composition. For a committed composition this is
// equivalent to reapply; we delegate to the wrapped composite (if any) so
// inline mutations are replayed, otherwise to ApplyAll for sub-command lists.
func (e *EditCommandComposition) DoApply() error {
	if e.wrapped != nil {
		return e.wrapped.DoApply()
	}
	return e.ApplyAll()
}

// DoUnapply reverses the composition, mirroring EditCommandComposition::doUnapply.
// Delegates to the wrapped composite's DoUnapply so that inline mutations
// (TypingCommand) are properly reversed.
func (e *EditCommandComposition) DoUnapply() error {
	if e.wrapped != nil {
		return e.wrapped.DoUnapply()
	}
	return e.UnapplyAll()
}

// DoReapply re-applies the composition, mirroring EditCommandComposition::doReapply.
// Delegates to the wrapped composite's DoReapply so that inline mutations
// (TypingCommand) are properly replayed.
func (e *EditCommandComposition) DoReapply() error {
	if e.wrapped != nil {
		return e.wrapped.DoReapply()
	}
	return e.ReapplyAll()
}

// EditingAction returns the EditAction of the composition (defaults to
// Unspecified; concrete composites override this when they wrap).
func (e *EditCommandComposition) EditingAction() EditAction { return EditActionUnspecified }

// Unapply implements UndoStep.Unapply by delegating to DoUnapply.
func (e *EditCommandComposition) Unapply() error { return e.DoUnapply() }

// Reapply implements UndoStep.Reapply by delegating to DoReapply.
func (e *EditCommandComposition) Reapply() error { return e.DoReapply() }

// Label returns the undo label, implementing UndoStep.Label.
func (e *EditCommandComposition) Label() string { return e.label }

// SetLabel records the undo label.
func (e *EditCommandComposition) SetLabel(s string) { e.label = s }

// IsSimpleEditCommandForUndoReporting reports whether the step is "simple"
// (single keystroke). Defaults to false; set via SetSimpleForUndoReporting.
func (e *EditCommandComposition) IsSimpleEditCommandForUndoReporting() bool {
	return e.wasUndoReportingSimple
}

// SetSimpleForUndoReporting records whether the step should be reported as
// simple in the undo UI.
func (e *EditCommandComposition) SetSimpleForUndoReporting(simple bool) {
	e.wasUndoReportingSimple = simple
}
