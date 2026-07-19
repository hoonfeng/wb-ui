// PropertyDescriptor corresponds to JSC::PropertyDescriptor (runtime/PropertyDescriptor.h)
// Used by Object.defineProperty, Object.getOwnPropertyDescriptor, etc.
package runtime

// PropertyDescriptor corresponds to JSC::PropertyDescriptor.
type PropertyDescriptor struct {
	m_value           JSValue
	m_getter          JSValue
	m_setter          JSValue
	m_attributes      uint8 // PropertyAttribute flags
	m_seenAttributes  uint8 // bitmask of seen attributes
}

const (
	WritablePresent   uint8 = 1 << 0
	EnumerablePresent uint8 = 1 << 1
	ConfigurablePresent uint8 = 1 << 2
)

const defaultAttributes uint8 = 0

// NewPropertyDescriptor creates an empty PropertyDescriptor.
func NewPropertyDescriptor() PropertyDescriptor {
	return PropertyDescriptor{m_attributes: defaultAttributes, m_seenAttributes: 0}
}

// NewPropertyDescriptorValue creates a PropertyDescriptor with a value and attributes.
func NewPropertyDescriptorValue(value JSValue, attributes uint8) PropertyDescriptor {
	return PropertyDescriptor{
		m_value:         value,
		m_attributes:    attributes,
		m_seenAttributes: EnumerablePresent | ConfigurablePresent | WritablePresent,
	}
}

func (d *PropertyDescriptor) Value() JSValue              { return d.m_value }
func (d *PropertyDescriptor) SetValue(v JSValue)          { d.m_value = v }

func (d *PropertyDescriptor) Getter() JSValue              { return d.m_getter }
func (d *PropertyDescriptor) SetGetter(g JSValue)          { d.m_getter = g }

func (d *PropertyDescriptor) Setter() JSValue              { return d.m_setter }
func (d *PropertyDescriptor) SetSetter(s JSValue)          { d.m_setter = s }

func (d *PropertyDescriptor) Writable() bool               { return d.m_attributes&PropertyAttributeReadOnly == 0 }
func (d *PropertyDescriptor) SetWritable(b bool)           { if !b { d.m_attributes |= PropertyAttributeReadOnly } else { d.m_attributes &^= PropertyAttributeReadOnly }; d.m_seenAttributes |= WritablePresent }

func (d *PropertyDescriptor) Enumerable() bool             { return d.m_attributes&PropertyAttributeDontEnum == 0 }
func (d *PropertyDescriptor) SetEnumerable(b bool)         { if !b { d.m_attributes |= PropertyAttributeDontEnum } else { d.m_attributes &^= PropertyAttributeDontEnum }; d.m_seenAttributes |= EnumerablePresent }

func (d *PropertyDescriptor) Configurable() bool           { return d.m_attributes&PropertyAttributeDontDelete == 0 }
func (d *PropertyDescriptor) SetConfigurable(b bool)       { if !b { d.m_attributes |= PropertyAttributeDontDelete } else { d.m_attributes &^= PropertyAttributeDontDelete }; d.m_seenAttributes |= ConfigurablePresent }

func (d *PropertyDescriptor) IsAccessorDescriptor() bool   { return d.GetterPresent() || d.SetterPresent() }
func (d *PropertyDescriptor) IsDataDescriptor() bool       { return (d.m_seenAttributes & (WritablePresent)) != 0 || d.m_value.IsValid() }
func (d *PropertyDescriptor) IsGenericDescriptor() bool    { return !d.IsAccessorDescriptor() && !d.IsDataDescriptor() }
func (d *PropertyDescriptor) Attributes() uint8            { return d.m_attributes }
func (d *PropertyDescriptor) SetAttributes(a uint8)        { d.m_attributes = a }

func (d *PropertyDescriptor) EnumerablePresent() bool      { return d.m_seenAttributes&EnumerablePresent != 0 }
func (d *PropertyDescriptor) ConfigurablePresent() bool    { return d.m_seenAttributes&ConfigurablePresent != 0 }
func (d *PropertyDescriptor) WritablePresent() bool        { return d.m_seenAttributes&WritablePresent != 0 }
func (d *PropertyDescriptor) GetterPresent() bool          { return d.m_getter.IsValid() }
func (d *PropertyDescriptor) SetterPresent() bool          { return d.m_setter.IsValid() }
func (d *PropertyDescriptor) IsEmpty() bool                { return !d.m_value.IsValid() && !d.m_getter.IsValid() && !d.m_setter.IsValid() && d.m_seenAttributes == 0 }
