// Identifier corresponds to JSC::Identifier (runtime/Identifier.h)
package runtime

// Identifier corresponds to JSC::Identifier.
type Identifier struct {
	m_impl string
}

// NewIdentifier creates a new Identifier from a string.
func NewIdentifier(s string) Identifier {
	return Identifier{m_impl: s}
}

// String returns the string representation.
func (id Identifier) String() string {
	return id.m_impl
}

// Impl returns the underlying string.
func (id Identifier) Impl() string {
	return id.m_impl
}

// IsSymbol returns true if this Identifier is a symbol.
func (id Identifier) IsSymbol() bool {
	return false
}

// IsEmpty returns true if this Identifier is empty.
func (id Identifier) IsEmpty() bool {
	return id.m_impl == ""
}

// PropertyNameArrayBuilder corresponds to JSC::PropertyNameArrayBuilder.
type PropertyNameArrayBuilder struct {
	names []Identifier
}

// NewPropertyNameArrayBuilder creates a new PropertyNameArrayBuilder.
func NewPropertyNameArrayBuilder(vm *VM, mode PropertyNameMode, privateSymbolMode PrivateSymbolMode) PropertyNameArrayBuilder {
	_ = vm
	_ = privateSymbolMode
	_ = mode
	return PropertyNameArrayBuilder{}
}

// Add adds an Identifier to the builder.
func (b *PropertyNameArrayBuilder) Add(name Identifier) {
	b.names = append(b.names, name)
}

// Size returns the number of names.
func (b *PropertyNameArrayBuilder) Size() int {
	return len(b.names)
}

// Get returns the Identifier at index i.
func (b *PropertyNameArrayBuilder) Get(i int) Identifier {
	return b.names[i]
}

// Names returns all Identifiers.
func (b *PropertyNameArrayBuilder) Names() []Identifier {
	return b.names
}
