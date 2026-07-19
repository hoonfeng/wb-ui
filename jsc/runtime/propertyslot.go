// PropertySlot corresponds to JSC::PropertySlot (runtime/PropertySlot.h)
package runtime

// PropertySlot corresponds to JSC::PropertySlot.
// Used for [[Get]] and [[HasProperty]] internal operations.
type PropertySlot struct {
	Value    JSValue
	SlotBase *JSObject
	Offset   PropertyOffset
	Metadata *PropertySlotMetadata
}

// PropertySlotMetadata holds additional metadata for a property slot.
type PropertySlotMetadata struct {
	Attributes uint8
}

// NewPropertySlot creates an empty PropertySlot.
func NewPropertySlot() PropertySlot {
	return PropertySlot{
		Offset: InvalidOffset,
	}
}

// NewPropertySlotValue creates a PropertySlot with a value.
func NewPropertySlotValue(value JSValue) PropertySlot {
	return PropertySlot{
		Value:  value,
		Offset: InvalidOffset,
	}
}

// IsValid returns true if the slot has a value.
func (s PropertySlot) IsValid() bool {
	return s.Value.IsValid()
}

// PutPropertySlot corresponds to JSC::PutPropertySlot.
// Used for [[Set]] internal operation.
type PutPropertySlot struct {
	Context *JSObject
	Offset  PropertyOffset
}

// NewPutPropertySlot creates an empty PutPropertySlot.
func NewPutPropertySlot() PutPropertySlot {
	return PutPropertySlot{
		Offset: InvalidOffset,
	}
}

// DeletePropertySlot corresponds to JSC::DeletePropertySlot.
type DeletePropertySlot struct{}

// NewDeletePropertySlot creates an empty DeletePropertySlot.
func NewDeletePropertySlot() DeletePropertySlot {
	return DeletePropertySlot{}
}

// PropertySlotMetadata is defined above.

// PropertyNameMode corresponds to JSC::PropertyNameMode.
type PropertyNameMode uint8

const (
	PropertyNameModeStrings           PropertyNameMode = iota
	PropertyNameModeSymbols
	PropertyNameModeStringsAndSymbols
)

// PrivateSymbolMode corresponds to JSC::PrivateSymbolMode.
type PrivateSymbolMode uint8

const (
	PrivateSymbolModeExclude PrivateSymbolMode = iota
	PrivateSymbolModeInclude
)

// ImplementationVisibility corresponds to JSC::ImplementationVisibility.
type ImplementationVisibility uint8

const (
	ImplementationVisibilityPublic  ImplementationVisibility = iota
	ImplementationVisibilityPrivate
)

// DontEnumPropertiesMode corresponds to JSC::DontEnumPropertiesMode.
type DontEnumPropertiesMode uint8

const (
	IncludeDontEnumProperties DontEnumPropertiesMode = iota
	ExcludeDontEnumProperties
)

// PropertyAttribute flags (from JSC PropertyAttribute.h).
const (
	PropertyAttributeReadOnly            uint8 = 1 << 0
	PropertyAttributeDontEnum            uint8 = 1 << 1
	PropertyAttributeDontDelete          uint8 = 1 << 2
	PropertyAttributeAccessor            uint8 = 1 << 3
	PropertyAttributeCustomAccessor      uint8 = 1 << 4
	PropertyAttributeBuiltinOrFunction   uint8 = 1 << 5
	PropertyAttributeBuiltinOrFunctionOrAccessorOrCustomAccessor uint8 = PropertyAttributeBuiltinOrFunction | PropertyAttributeAccessor | PropertyAttributeCustomAccessor
	PropertyAttributeFunction            uint8 = PropertyAttributeBuiltinOrFunction
	PropertyAttributeBuiltin             uint8 = PropertyAttributeBuiltinOrFunction
	PropertyAttributeAccessorOrCustomAccessor uint8 = PropertyAttributeAccessor | PropertyAttributeCustomAccessor
)
type InternalMethodType uint8

const (
	InternalMethodTypeGet             InternalMethodType = iota
	InternalMethodTypeGetOwnProperty
	InternalMethodTypeHasProperty
)
