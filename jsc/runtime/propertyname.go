// PropertyName corresponds to JSC::PropertyName (runtime/PropertyName.h)
package runtime

// PropertyName corresponds to JSC::PropertyName.
type PropertyName struct {
	name string
}

// NewPropertyName creates a new PropertyName.
func NewPropertyName(name string) PropertyName {
	return PropertyName{name: name}
}

// String returns the string representation.
func (pn PropertyName) String() string {
	return pn.name
}

// IsEmpty returns true if the name is empty.
func (pn PropertyName) IsEmpty() bool {
	return pn.name == ""
}
