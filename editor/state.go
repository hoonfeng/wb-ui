// Translation of: CodeMirror 6 — packages/state/src/state.ts
//                  https://github.com/codemirror/state/blob/main/src/state.ts
//
// Completeness: 70%
// Differences from CM6:
//   - No Go generics; field and facet values are interface{}.
//   - The reconfiguration mechanism is simplified (no dynamic facets
//     or slot-based provider system in v1).
//   - Methods use PascalCase (Go convention).
//
// EditorState is the immutable state of the editor. It holds the document,
// selection, extension-provided field values, and computed facet values.
// Updates produce a new state (the old state is unchanged).

package editor

// EditorState is the immutable state of the editor.
type EditorState struct {
	// Doc is the document text.
	Doc Text
	// Selection is the current selection.
	Selection EditorSelection
	// fields stores StateField values, keyed by StateField.ID().
	fields map[int]interface{}
	// facets stores computed Facet values, keyed by Facet.ID().
	facets map[int]interface{}
	// config is the configuration (extensions) used to create this state.
	config Config
}

// NewState creates a new EditorState from the given configuration.
func NewState(conf Config) EditorState {
	doc := conf.Doc
	if doc == nil || doc.Length() < 0 {
		doc = Empty()
	}
	var sel EditorSelection
	if conf.Selection != nil {
		sel = conf.Selection.Bound(doc.Length())
	} else {
		sel = SelectionCaret(0)
	}

	fields := make(map[int]interface{})
	facets := make(map[int]interface{})

	state := EditorState{
		Doc:       doc,
		Selection: sel,
		fields:    fields,
		facets:    facets,
		config:    conf,
	}

	// Initialize all StateField values.
	for _, f := range CollectStateFields(conf.Extensions) {
		fields[f.ID()] = f.Create(state)
	}

	// Compute all facet values.
	// Collect all Facets referenced by extensions.
	allFacets := collectAllFacets(conf.Extensions)
	for _, facet := range allFacets {
		values := CollectFacetValues(facet, conf.Extensions)
		facets[facet.ID()] = facet.Combine(values)
	}

	return state
}

// Field returns the value of the given StateField, or nil if not present.
func (s EditorState) Field(f *StateField) interface{} {
	return s.fields[f.ID()]
}

// Facet returns the computed value of the given Facet, or nil if not present.
func (s EditorState) Facet(f *Facet) interface{} {
	return s.facets[f.ID()]
}

// Extensions returns the active extensions.
func (s EditorState) Extensions() []Extension {
	return s.config.Extensions
}

// Config returns the configuration used to create this state.
func (s EditorState) Config() Config {
	return s.config
}

// Update applies a TransactionSpec to the state, returning a new state
// and the transaction that was applied. The old state is unchanged.
//
// Mirrors CM6's EditorState.update().
func (s EditorState) Update(spec TransactionSpec) (EditorState, Transaction) {
	tr := NewTransaction(&s, spec)

	newDoc := s.Doc
	if !tr.Changes().Empty() {
		newDoc = tr.Changes().Apply(s.Doc)
	}

	newSel := tr.Selection()
	if !spec.HasSelection {
		newSel = newSel.Bound(newDoc.Length())
	}

	// Update all StateField values.
	newFields := make(map[int]interface{})
	for _, f := range CollectStateFields(s.config.Extensions) {
		old := s.fields[f.ID()]
		newFields[f.ID()] = f.Update(old, tr)
	}

	newState := EditorState{
		Doc:       newDoc,
		Selection: newSel,
		fields:    newFields,
		facets:    s.facets, // facets are static in v1
		config:    s.config,
	}
	return newState, tr
}

// Apply applies a transaction to the state, returning a new state.
// Mirrors CM6's EditorState.apply().
func (s EditorState) Apply(tr Transaction) EditorState {
	// Build a spec from the transaction and update.
	spec := TransactionSpec{
		Changes:        tr.Changes(),
		Selection:      &tr.selection,
		HasSelection:   true,
		ScrollIntoView: tr.ScrollIntoView(),
		UserEvent:      tr.UserEvent(),
		Annotations:    tr.AnnotationsMap(),
	}
	newState, _ := s.Update(spec)
	return newState
}

// AsField is a helper to get a StateField value with a type assertion.
// Returns the value and true if the field exists, nil and false otherwise.
func (s EditorState) AsField(f *StateField) (interface{}, bool) {
	v, ok := s.fields[f.ID()]
	return v, ok
}

// collectAllFacets collects all Facet extensions from the given list.
func collectAllFacets(extensions []Extension) []*Facet {
	var facets []*Facet
	for _, ext := range extensions {
		switch v := ext.(type) {
		case *Facet:
			facets = append(facets, v)
		case ExtensionSpec:
			facets = append(facets, collectAllFacets(v.Extensions)...)
		}
	}
	return facets
}
