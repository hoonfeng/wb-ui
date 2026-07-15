// Translation of: CodeMirror 6 — packages/state/src/facet.ts
//                  packages/state/src/stateField.ts
//                  https://github.com/codemirror/state/blob/main/src/facet.ts
//
// Completeness: 60%
// Differences from CM6:
//   - No Go generics; Facet and StateField use interface{} for values,
//     requiring type assertions at the call site.
//   - The Facet combining/dynamic/static distinction is simplified;
//     all facets are "static" (computed once per state configuration).
//   - Extension is an empty interface (any value can be an extension).
//   - Methods use PascalCase (Go convention).
//
// Extension system: Facet collects values from extensions;
// StateField holds per-state mutable data updated on each transaction.

package editor

// Extension is any value that can contribute to an editor state.
// Facets, StateFields, and extension-providing functions implement this
// interface (or simply are values of these types).
type Extension interface {
	// IsExtension is a marker method that prevents arbitrary types from
	// being used as extensions. All Facet, StateField, and ExtensionSpec
	// values satisfy this interface.
	IsExtension()
}

// Facet is a typed slot that collects values from extensions. Each facet
// has a combine function that reduces the collected values into a single
// output value.
//
// In CM6, Facets are created via Facet.define(input, combine). This port
// uses Facet.New(input, combine) and stores values as interface{}.
type Facet struct {
	// id is a unique identifier for this facet.
	id int
	// combine reduces the collected input values into a single output.
	// If nil, the first input value is used (or nil if no inputs).
	combine func(inputs []interface{}) interface{}
	// staticInputs is the set of input values registered when the facet
	// is defined (via Facet.New). Extensions can also provide values at
	// state-creation time.
	staticInputs []interface{}
}

var nextFacetID int

// NewFacet creates a new Facet with the given combine function and
// static inputs. Mirrors CM6's Facet.define().
func NewFacet(combine func(inputs []interface{}) interface{}, staticInputs []interface{}) *Facet {
	nextFacetID++
	return &Facet{
		id:           nextFacetID,
		combine:      combine,
		staticInputs: staticInputs,
	}
}

// ID returns the unique identifier for this facet.
func (f *Facet) ID() int { return f.id }

// IsExtension marks Facet as an Extension.
func (f *Facet) IsExtension() {}

// Combine reduces a slice of input values into a single output using
// the facet's combine function. If no combine function is set, returns
// the first input or nil.
func (f *Facet) Combine(inputs []interface{}) interface{} {
	if f.combine == nil {
		if len(inputs) == 0 {
			return nil
		}
		return inputs[0]
	}
	return f.combine(inputs)
}

// StaticInputs returns the static input values registered with this facet.
func (f *Facet) StaticInputs() []interface{} { return f.staticInputs }

// StateField is a slot that holds per-state mutable data, updated on each
// transaction. It mirrors CM6's StateField.
type StateField struct {
	// id is a unique identifier for this field.
	id int
	// create initializes the field value for a new state.
	create func(state EditorState) interface{}
	// update produces a new field value given the old value and a
	// transaction. If it returns the same value (by identity), no
	// copy is made.
	update func(value interface{}, tr Transaction) interface{}
}

var nextFieldID int

// NewStateField creates a new StateField with the given create and update
// functions. Mirrors CM6's StateField.define().
func NewStateField(create func(state EditorState) interface{}, update func(value interface{}, tr Transaction) interface{}) *StateField {
	nextFieldID++
	return &StateField{
		id:     nextFieldID,
		create: create,
		update: update,
	}
}

// ID returns the unique identifier for this field.
func (f *StateField) ID() int { return f.id }

// IsExtension marks StateField as an Extension.
func (f *StateField) IsExtension() {}

// Create initializes the field value for the given state.
func (f *StateField) Create(state EditorState) interface{} {
	if f.create == nil {
		return nil
	}
	return f.create(state)
}

// Update produces a new field value given the old value and a transaction.
func (f *StateField) Update(value interface{}, tr Transaction) interface{} {
	if f.update == nil {
		return value
	}
	return f.update(value, tr)
}

// ExtensionSpec is a simple Extension wrapper that applies a set of
// extensions to a state. It is used as a container for grouping extensions.
type ExtensionSpec struct {
	// Extensions are the extensions to apply.
	Extensions []Extension
}

// IsExtension marks ExtensionSpec as an Extension.
func (e ExtensionSpec) IsExtension() {}

// ExtensionFunc is a function that returns an Extension. It allows
// lazy extension creation. Mirrors CM6's Extension.
type ExtensionFunc func() Extension

// IsExtension marks ExtensionFunc as an Extension.
func (f ExtensionFunc) IsExtension() {}

// Config is the configuration for an editor state, consisting of a set
// of extensions. Mirrors CM6's EditorStateConfig.
type Config struct {
	// Extensions are the extensions to apply to the state.
	Extensions []Extension
	// Doc is the initial document (if empty, an empty document is used).
	Doc Text
	// Selection is the initial selection (if nil, a caret at position 0).
	Selection *EditorSelection
}

// CollectFacetValues collects all values provided for the given facet
// from the given extensions. Returns a slice of interface{} values.
func CollectFacetValues(facet *Facet, extensions []Extension) []interface{} {
	var values []interface{}
	// Add static inputs from the facet itself.
	values = append(values, facet.StaticInputs()...)
	// Walk extensions looking for facet providers.
	for _, ext := range extensions {
		switch v := ext.(type) {
		case *Facet:
			// A Facet used as an extension provides its static inputs.
			if v == facet {
				values = append(values, v.StaticInputs()...)
			}
		case *StateField:
			// StateFields don't provide facet values directly.
		case ExtensionSpec:
			// Recurse into grouped extensions.
			values = append(values, CollectFacetValues(facet, v.Extensions)...)
		}
	}
	return values
}

// CollectStateFields collects all StateField extensions from the given
// extensions list.
func CollectStateFields(extensions []Extension) []*StateField {
	var fields []*StateField
	for _, ext := range extensions {
		switch v := ext.(type) {
		case *StateField:
			fields = append(fields, v)
		case ExtensionSpec:
			fields = append(fields, CollectStateFields(v.Extensions)...)
		}
	}
	return fields
}
