// Translation of: Source/JavaScriptCore/runtime/JSTypeInfo.h
//
// TypeInfo holds per-type inline and out-of-line flags used for fast dispatch.

package runtime

// TypeInfo corresponds to JSC::TypeInfo. It stores a JSType plus inline and
// out-of-line flags that describe object behaviour (e.g. MasqueradesAsUndefined,
// OverridesGetOwnPropertySlot, etc.).
type TypeInfo struct {
	mType  JSType   // JSType enum value (uint8)
	mFlags uint8    // inline flags (lower 8 bits)
	mFlags2 uint16   // out-of-line flags (upper bits, shifted)
}

// Inline (flags1) bit constants — stored in m_flags.
const (
	MasqueradesAsUndefined             = 1 << 0 // WebCore uses this for document.all
	ImplementsDefaultHasInstance        = 1 << 1
	OverridesGetCallData               = 1 << 2
	OverridesGetOwnPropertySlot        = 1 << 3
	OverridesGetPrototype              = 1 << 4
	HasStaticPropertyTable             = 1 << 5
	TypeInfoPerCellBit                 = 1 << 7 // set on the cell, not the Structure
)

// Out-of-line (flags2) bit constants — stored in m_flags2 after shifting.
const (
	ImplementsHasInstance                                   = 1 << 8
	OverridesGetOwnPropertyNames                            = 1 << 9
	OverridesGetOwnSpecialPropertyNames                     = 1 << 10
	ProhibitsPropertyCaching                                = 1 << 11
	GetOwnPropertySlotIsImpure                              = 1 << 12
	NewImpurePropertyFiresWatchpoints                       = 1 << 13
	IsImmutablePrototypeExoticObject                        = 1 << 14
	GetOwnPropertySlotIsImpureForPropertyAbsence            = 1 << 15
	InterceptsGetOwnPropertySlotByIndexEvenWhenLengthIsNotZero = 1 << 16
	StructureIsImmortal                                     = 1 << 17
	OverridesPut                                            = 1 << 18
	GetOwnPropertySlotMayBeWrongAboutDontEnum                = 1 << 20
	OverridesIsExtensible                                   = 1 << 21
)

const numberOfInlineBits = 8

// NewTypeInfo constructs a TypeInfo from a JSType and combined flags.
func NewTypeInfo(t JSType, flags uint32) TypeInfo {
	return TypeInfo{
		mType:   t,
		mFlags:  uint8(flags & 0xff),
		mFlags2: uint16(flags >> numberOfInlineBits),
	}
}

// NewTypeInfoSplit constructs a TypeInfo from separate inline/out-of-line flags.
func NewTypeInfoSplit(t JSType, inlineFlags uint8, outOfLineFlags uint16) TypeInfo {
	return TypeInfo{
		mType:   t,
		mFlags:  inlineFlags,
		mFlags2: outOfLineFlags,
	}
}

// Type returns the JSType.
func (ti TypeInfo) Type() JSType { return ti.mType }

// IsObject returns true if the type is >= ObjectType.
func (ti TypeInfo) IsObject() bool { return IsObjectType(ti.mType) }

// IsFinalObject returns true if the type is FinalObjectType.
func (ti TypeInfo) IsFinalObject() bool { return ti.mType == FinalObjectType }

// IsNumberObject returns true if the type is NumberObjectType.
func (ti TypeInfo) IsNumberObject() bool { return ti.mType == NumberObjectType }

// Flags returns the combined flags as uint32.
func (ti TypeInfo) Flags() uint32 {
	return (uint32(ti.mFlags2) << numberOfInlineBits) | uint32(ti.mFlags)
}

// InlineTypeFlags returns the inline flags byte.
func (ti TypeInfo) InlineTypeFlags() uint8 { return ti.mFlags }

// OutOfLineTypeFlags returns the out-of-line flags word.
func (ti TypeInfo) OutOfLineTypeFlags() uint16 { return ti.mFlags2 }

// MasqueradesAsUndefined returns true if the MasqueradesAsUndefined flag is set.
func (ti TypeInfo) MasqueradesAsUndefined() bool { return ti.mFlags&uint8(MasqueradesAsUndefined) != 0 }

// ImplementsHasInstance returns true if ImplementsHasInstance is set.
func (ti TypeInfo) ImplementsHasInstance() bool {
	return ti.mFlags2&uint16(ImplementsHasInstance>>numberOfInlineBits) != 0
}

// ImplementsDefaultHasInstance returns true if ImplementsDefaultHasInstance is set.
func (ti TypeInfo) ImplementsDefaultHasInstance() bool {
	return ti.mFlags&uint8(ImplementsDefaultHasInstance) != 0
}

// OverridesGetCallData returns true if OverridesGetCallData is set.
func (ti TypeInfo) OverridesGetCallData() bool {
	return ti.mFlags&uint8(OverridesGetCallData) != 0
}

// OverridesGetOwnPropertySlot returns true if OverridesGetOwnPropertySlot is set.
func (ti TypeInfo) OverridesGetOwnPropertySlot() bool {
	return OverridesGetOwnPropertySlotFromFlags(ti.mFlags)
}

// HasStaticPropertyTable returns true if HasStaticPropertyTable is set.
func (ti TypeInfo) HasStaticPropertyTable() bool {
	return ti.mFlags&uint8(HasStaticPropertyTable) != 0
}

// OverridesGetOwnPropertySlotFromFlags is the static version.
func OverridesGetOwnPropertySlotFromFlags(flags uint8) bool {
	return flags&uint8(OverridesGetOwnPropertySlot) != 0
}

// HasStaticPropertyTableFromFlags is the static version.
func HasStaticPropertyTableFromFlags(flags uint8) bool {
	return flags&uint8(HasStaticPropertyTable) != 0
}

// PerCellBitFromFlags extracts TypeInfoPerCellBit from inline flags.
func PerCellBitFromFlags(flags uint8) bool {
	return flags&uint8(TypeInfoPerCellBit) != 0
}

// StructureIsImmortal returns true if StructureIsImmortal is set.
func (ti TypeInfo) StructureIsImmortal() bool {
	return ti.mFlags2&uint16(StructureIsImmortal>>numberOfInlineBits) != 0
}

// OverridesGetOwnPropertyNames returns true if OverridesGetOwnPropertyNames is set.
func (ti TypeInfo) OverridesGetOwnPropertyNames() bool {
	return ti.mFlags2&uint16(OverridesGetOwnPropertyNames>>numberOfInlineBits) != 0
}

// OverridesGetOwnSpecialPropertyNames returns true if the flag is set.
func (ti TypeInfo) OverridesGetOwnSpecialPropertyNames() bool {
	return ti.mFlags2&uint16(OverridesGetOwnSpecialPropertyNames>>numberOfInlineBits) != 0
}

// OverridesAnyFormOfGetOwnPropertyNames returns true if either property names flag is set.
func (ti TypeInfo) OverridesAnyFormOfGetOwnPropertyNames() bool {
	return ti.OverridesGetOwnPropertyNames() || ti.OverridesGetOwnSpecialPropertyNames()
}

// OverridesPut returns true if OverridesPut is set.
func (ti TypeInfo) OverridesPut() bool {
	return ti.mFlags2&uint16(OverridesPut>>numberOfInlineBits) != 0
}

// OverridesGetPrototype returns true if OverridesGetPrototype is set.
func (ti TypeInfo) OverridesGetPrototype() bool {
	return ti.mFlags&uint8(OverridesGetPrototype) != 0
}

// OverridesIsExtensible returns true if OverridesIsExtensible is set.
func (ti TypeInfo) OverridesIsExtensible() bool {
	return ti.mFlags2&uint16(OverridesIsExtensible>>numberOfInlineBits) != 0
}

// ProhibitsPropertyCaching returns true if ProhibitsPropertyCaching is set.
func (ti TypeInfo) ProhibitsPropertyCaching() bool {
	return ti.mFlags2&uint16(ProhibitsPropertyCaching>>numberOfInlineBits) != 0
}

// GetOwnPropertySlotIsImpure returns true if the flag is set.
func (ti TypeInfo) GetOwnPropertySlotIsImpure() bool {
	return ti.mFlags2&uint16(GetOwnPropertySlotIsImpure>>numberOfInlineBits) != 0
}

// GetOwnPropertySlotIsImpureForPropertyAbsence returns true if the flag is set.
func (ti TypeInfo) GetOwnPropertySlotIsImpureForPropertyAbsence() bool {
	return ti.mFlags2&uint16(GetOwnPropertySlotIsImpureForPropertyAbsence>>numberOfInlineBits) != 0
}

// GetOwnPropertySlotMayBeWrongAboutDontEnum returns true if the flag is set.
func (ti TypeInfo) GetOwnPropertySlotMayBeWrongAboutDontEnum() bool {
	return ti.mFlags2&uint16(GetOwnPropertySlotMayBeWrongAboutDontEnum>>numberOfInlineBits) != 0
}

// NewImpurePropertyFiresWatchpoints returns true if the flag is set.
func (ti TypeInfo) NewImpurePropertyFiresWatchpoints() bool {
	return ti.mFlags2&uint16(NewImpurePropertyFiresWatchpoints>>numberOfInlineBits) != 0
}

// IsImmutablePrototypeExoticObject returns true if the flag is set.
func (ti TypeInfo) IsImmutablePrototypeExoticObject() bool {
	return ti.mFlags2&uint16(IsImmutablePrototypeExoticObject>>numberOfInlineBits) != 0
}

// InterceptsGetOwnPropertySlotByIndexEvenWhenLengthIsNotZero returns true if the flag is set.
func (ti TypeInfo) InterceptsGetOwnPropertySlotByIndexEvenWhenLengthIsNotZero() bool {
	return ti.mFlags2&uint16(InterceptsGetOwnPropertySlotByIndexEvenWhenLengthIsNotZero>>numberOfInlineBits) != 0
}

// MergeInlineTypeFlags merges Structure flags with per-cell flags.
func MergeInlineTypeFlags(structureFlags, oldCellFlags uint8) uint8 {
	return structureFlags | (oldCellFlags & uint8(TypeInfoPerCellBit))
}
