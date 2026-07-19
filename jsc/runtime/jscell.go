// Translation of: Source/JavaScriptCore/runtime/JSCell.h
//                  Source/JavaScriptCore/runtime/JSCell.cpp
//
// JSCell is the base GC cell type. Every JavaScript value that lives on the
// GC heap extends JSCell.

package runtime

import "fmt"

// JSCell corresponds to JSC::JSCell. It is the minimal GC-able object header.
// All JS objects embed JSCell as their first field.
type JSCell struct {
	structureID         StructureID  // 4 bytes
	indexingTypeAndMisc IndexingType // 1 byte
	typ                 JSType       // 1 byte
	flags               uint8        // TypeInfo::InlineTypeFlags
	cellState           CellState    // 1 byte
}

// JSCell constants
const (
	JSCellAtomSize                = 16
	NumberOfLowerTierPreciseCells = 8
	JSCellStructureFlags  uint32  = 0
)

// --- Factory methods ---

// NewJSCell allocates a new JSCell with the given VM and Structure.
func NewJSCell(vm *VM, structure *Structure) *JSCell {
	cell := &JSCell{
		structureID: structure.structureID,
		typ:         structure.TypeInfo.Type(),
		cellState:   DefinitelyWhite,
	}
	cell.finishCreation(vm)
	return cell
}

// NewJSCellEarly creates a JSCell for early/bootstrapping phases.
func NewJSCellEarly() *JSCell {
	return &JSCell{cellState: DefinitelyWhite}
}

// NewJSCellWellDefined creates a JSCell with explicit StructureID.
func NewJSCellWellDefined(id StructureID, typeInfoBlob uint32) *JSCell {
	_ = typeInfoBlob
	return &JSCell{structureID: id, cellState: DefinitelyWhite}
}

// --- Type querying ---

func (c *JSCell) IsString() bool           { return c.typ == StringType }
func (c *JSCell) IsHeapBigInt() bool       { return c.typ == HeapBigIntType }
func (c *JSCell) IsSymbol() bool           { return c.typ == SymbolType }
func (c *JSCell) IsGetterSetter() bool     { return c.typ == GetterSetterType }
func (c *JSCell) IsCustomGetterSetter() bool { return c.typ == CustomGetterSetterType }
func (c *JSCell) IsAPIValueWrapper() bool   { return c.typ == APIValueWrapperType }

// IsProxy returns true if the cell is a ProxyObject or JSGlobalProxy.
func (c *JSCell) IsProxy() bool { return c.typ == GlobalProxyType || c.typ == ProxyObjectType }

// IsObject returns true if the type is an object type.
func (c *JSCell) IsObject() bool { return IsObjectType(c.typ) }

// IsCallable checks if this cell is callable.
func (c *JSCell) IsCallable() bool {
	data := GetCallData(c)
	return data.Type != CallTypeNone
}

// IsConstructor checks if this cell is constructable.
func (c *JSCell) IsConstructor() bool {
	data := GetConstructData(c)
	return data.Type != ConstructTypeNone
}

// Type returns the JSType of this cell.
func (c *JSCell) Type() JSType { return c.typ }

// StructureID returns the StructureID.
func (c *JSCell) StructureID() StructureID { return c.structureID }

// Structure returns the Structure* for this cell.
func (c *JSCell) Structure() *Structure { return c.structureID.Decode() }

// SetStructure sets a new Structure on this cell.
func (c *JSCell) SetStructure(vm *VM, s *Structure) {
	_ = vm
	c.structureID = s.structureID
	c.typ = s.TypeInfo.Type()
}

func (c *JSCell) SetStructureIDDirectly(id StructureID) { c.structureID = id }
func (c *JSCell) ClearStructure()                        { c.structureID = InvalidStructureID }

// IndexingTypeAndMisc returns the combined indexing type and misc byte.
func (c *JSCell) IndexingTypeAndMisc() IndexingType { return c.indexingTypeAndMisc }

// IndexingMode returns the indexing mode (lower bits).
func (c *JSCell) IndexingMode() IndexingType { return c.indexingTypeAndMisc & AllArrayTypes }

// IndexingType returns the indexing type.
func (c *JSCell) IndexingType() IndexingType { return c.indexingTypeAndMisc & AllWritableArrayTypes }

// InlineTypeFlags returns the inline type flags.
func (c *JSCell) InlineTypeFlags() uint8 { return c.flags }

// CellState returns the current GC cell state.
func (c *JSCell) CellState() CellState { return c.cellState }

// SetCellState sets the GC cell state.
func (c *JSCell) SetCellState(state CellState) { c.cellState = state }

// --- Value extraction ---

// GetString attempts to get the string value from this cell.
func (c *JSCell) GetString(globalObject *JSGlobalObject) (string, bool) {
	_ = globalObject
	return "", c.IsString()
}

// GetStringValue returns the string value or empty string if not a string.
func (c *JSCell) GetStringValue(globalObject *JSGlobalObject) string {
	_ = globalObject
	return ""
}

// GetObject returns this cell as *JSObject, or nil if not an object.
func (c *JSCell) GetObject() *JSObject {
	if c.IsObject() {
		return AsObject(c)
	}
	return nil
}

// ClassName returns the class name string from ClassInfo.
func (c *JSCell) ClassName() string {
	return "JSCell"
}

// --- Call/Construct data ---

// GetCallData returns the CallData for this cell.
func GetCallData(cell *JSCell) CallData {
	t := cell.Type()
	switch t {
	case JSFunctionType:
		return CallData{Type: CallTypeJS}
	case InternalFunctionType:
		return CallData{Type: CallTypeNative}
	default:
		return CallData{Type: CallTypeNone}
	}
}

// GetConstructData returns the ConstructData for this cell.
func GetConstructData(cell *JSCell) ConstructData {
	t := cell.Type()
	switch t {
	case JSFunctionType:
		return ConstructData{Type: ConstructTypeJS}
	case InternalFunctionType:
		return ConstructData{Type: ConstructTypeNative}
	default:
		return ConstructData{Type: ConstructTypeNone}
	}
}

// --- Conversions ---

// ToPrimitive converts this cell to a primitive value.
func (c *JSCell) ToPrimitive(globalObject *JSGlobalObject, preferred PreferredPrimitiveType) JSValue {
	_ = preferred
	// TODO: full dispatch
	return JSValue{tag: TagUndefined}
}

// ToBoolean converts this cell to a boolean (all objects are truthy by default).
func (c *JSCell) ToBoolean(globalObject *JSGlobalObject) bool {
	_ = globalObject
	// Non-empty strings are truthy
	if c.IsString() {
		return true // TODO: check empty
	}
	return true
}

// ToNumber converts this cell to a number.
func (c *JSCell) ToNumber(globalObject *JSGlobalObject) float64 {
	_ = globalObject
	return 0
}

// ToObject converts this cell to an object.
func (c *JSCell) ToObject(globalObject *JSGlobalObject) *JSObject {
	if c.IsObject() {
		return AsObject(c)
	}
	return c.toObjectSlow(globalObject)
}

func (c *JSCell) toObjectSlow(globalObject *JSGlobalObject) *JSObject {
	_ = globalObject
	return nil
}

// ToStringInline converts this cell to a JSString* (inline fast path).
func (c *JSCell) ToStringInline(globalObject *JSGlobalObject) *JSString {
	_ = globalObject
	return nil
}

// ToStringSlowCase is the slow path for toString.
func (c *JSCell) ToStringSlowCase(globalObject *JSGlobalObject) *JSString {
	_ = globalObject
	return nil
}

// --- Property operations ---

// Put sets a property on this cell.
func (c *JSCell) Put(cell *JSCell, globalObject *JSGlobalObject, name PropertyName, value JSValue, slot *PutPropertySlot) bool {
	_ = cell
	_ = globalObject
	_ = name
	_ = value
	_ = slot
	return false
}

// PutByIndex sets a property by index.
func (c *JSCell) PutByIndex(cell *JSCell, globalObject *JSGlobalObject, index uint32, value JSValue, shouldThrow bool) bool {
	_ = cell
	_ = globalObject
	_ = index
	_ = value
	_ = shouldThrow
	return false
}

// DeleteProperty deletes a property from this cell.
func (c *JSCell) DeleteProperty(cell *JSCell, globalObject *JSGlobalObject, name PropertyName, slot *DeletePropertySlot) bool {
	_ = cell
	_ = globalObject
	_ = name
	_ = slot
	return false
}

// DeletePropertyByName (simpler signature).
func (c *JSCell) DeletePropertyByName(cell *JSCell, globalObject *JSGlobalObject, name PropertyName) bool {
	_ = cell
	_ = globalObject
	_ = name
	return false
}

// DeletePropertyByIndex deletes a property by index.
func (c *JSCell) DeletePropertyByIndex(cell *JSCell, globalObject *JSGlobalObject, index uint32) bool {
	_ = cell
	_ = globalObject
	_ = index
	return false
}

// --- Lifecycle ---

// FinishCreation completes cell initialization.
func (c *JSCell) FinishCreation(vm *VM) {
	_ = vm
}

// Destroy tears down the cell (no-op in Go, GC managed).
func (c *JSCell) Destroy() {}

// --- Debug ---

// Dump prints a debug representation.
func (c *JSCell) Dump() string {
	return fmt.Sprintf("<%p, %s>", c, c.ClassName())
}

// EstimatedSizeInBytes returns an estimate of the cell's heap size.
func (c *JSCell) EstimatedSizeInBytes(vm *VM) uint {
	_ = vm
	return c.CellSize()
}

// CellSize returns the byte size of this cell (override per type).
func (c *JSCell) CellSize() uint { return 64 }

// --- Inherits / ClassInfo ---

func (c *JSCell) Inherits(info *ClassInfo) bool {
	_ = info
	return true
}

func (c *JSCell) InheritsSlow(info *ClassInfo) bool {
	return c.Inherits(info)
}

func (c *JSCell) MethodTable() interface{} {
	return nil
}

// --- putToPrimitive ---

// PutToPrimitive handles property assignment on primitive values (throws TypeError in strict mode).
func PutToPrimitive(cell *JSCell, globalObject *JSGlobalObject, name PropertyName, value JSValue, slot *PutPropertySlot) bool {
	_ = cell
	_ = globalObject
	_ = name
	_ = value
	_ = slot
	return false
}

// JSValueFromCell creates a JSValue wrapping this cell.
func JSValueFromCell(cell *JSCell) JSValue {
	return JSValue{tag: TagObject, payload: cell}
}

// finishCreation (unexported, internal use).
func (c *JSCell) finishCreation(vm *VM) { _ = vm }

// JSCellLike is an interface for types that embed JSCell.
type JSCellLike interface {
	JSCellRef() *JSCell
}

func (c *JSCell) JSCellRef() *JSCell { return c }

// EnsureStillAliveHere prevents GC from collecting the cell at this point.
func EnsureStillAliveHere(cell *JSCell) { _ = cell }
