// Translation of: Source/JavaScriptCore/runtime/Structure.h
//                  Source/JavaScriptCore/runtime/Structure.cpp
//
// Structure describes the "shape" of an object — its property layout, class info,
// prototype, and type flags. Corresponds to Hidden Class / Shape in other engines.

package runtime

// StructureStructureFlags is the default flags for Structure.
const StructureStructureFlags uint32 = JSCellStructureFlags

// Structure corresponds to JSC::Structure. It describes the property layout
// and type metadata for a group of objects with the same shape.
type Structure struct {
	JSCell
	TypeInfo   TypeInfo
	classInfo  *ClassInfo
	prototype  JSValue
	propertyTable map[string]uint32
	nextOffset uint32
	previous   *Structure
	rareData   interface{}
	isPinned   bool
	hasRareData bool
	isImmutablePrototypeExoticObject bool
}

// --- Factory ---

// NewStructure creates a new Structure for the given type/classInfo/prototype.
func NewStructure(vm *VM, globalObject *JSGlobalObject, prototype JSValue, typeInfo TypeInfo, classInfo *ClassInfo) *Structure {
	id := AllocateStructureID()
	s := &Structure{
		TypeInfo:      typeInfo,
		classInfo:     classInfo,
		prototype:     prototype,
		propertyTable: make(map[string]uint32),
	}
	s.structureID = id
	s.typ = StructureType
	s.cellState = DefinitelyWhite
	RegisterStructure(id, s)
	_ = vm
	_ = globalObject
	return s
}

// NewStructureWithID creates a Structure with explicit StructureID (bootstrapping).
func NewStructureWithID(id StructureID, typeInfo TypeInfo, classInfo *ClassInfo) *Structure {
	s := &Structure{
		TypeInfo:      typeInfo,
		classInfo:     classInfo,
		propertyTable: make(map[string]uint32),
	}
	s.structureID = id
	s.typ = StructureType
	s.cellState = DefinitelyWhite
	RegisterStructure(id, s)
	return s
}

// --- Accessors ---

func (s *Structure) GetClassInfo() *ClassInfo { return s.classInfo }
func (s *Structure) Prototype() JSValue       { return s.prototype }

func (s *Structure) SetPrototype(vm *VM, prototype JSValue) {
	_ = vm
	s.prototype = prototype
}

func (s *Structure) IsPinned() bool                           { return s.isPinned }
func (s *Structure) Pin()                                     { s.isPinned = true }
func (s *Structure) IsImmutablePrototypeExoticObject() bool    { return s.isImmutablePrototypeExoticObject }
func (s *Structure) HasRareData() bool                         { return s.hasRareData }
func (s *Structure) Previous() *Structure                      { return s.previous }

func (s *Structure) SetPreviousInChain(vm *VM, previous *Structure) {
	_ = vm
	s.previous = previous
}

func (s *Structure) PropertyTable() map[string]uint32 { return s.propertyTable }
func (s *Structure) NextOffset() uint32               { return s.nextOffset }
func (s *Structure) SetNextOffset(offset uint32)      { s.nextOffset = offset }

// AddProperty adds a property and returns its offset.
func (s *Structure) AddProperty(name string) uint32 {
	offset := s.nextOffset
	s.propertyTable[name] = offset
	s.nextOffset++
	return offset
}

// GetPropertyOffset returns the offset for a property, or false if not found.
func (s *Structure) GetPropertyOffset(name string) (uint32, bool) {
	offset, ok := s.propertyTable[name]
	return offset, ok
}

// HasProperty returns true if the property exists in this structure.
func (s *Structure) HasProperty(name string) bool {
	_, ok := s.propertyTable[name]
	return ok
}

func (s *Structure) IsValid() bool     { return s.structureID.IsValid() }
func (s *Structure) StructureFlags() uint32 { return s.TypeInfo.Flags() }

// Transition creates a new Structure inheriting properties plus a new one.
func (s *Structure) Transition(vm *VM, globalObject *JSGlobalObject, name string, attributes uint32) *Structure {
	_ = globalObject
	_ = attributes
	newStruct := NewStructure(vm, nil, s.prototype, s.TypeInfo, s.classInfo)
	for k, v := range s.propertyTable {
		newStruct.propertyTable[k] = v
	}
	newStruct.nextOffset = s.nextOffset
	newStruct.AddProperty(name)
	newStruct.previous = s
	return newStruct
}
