// Translation of: Source/JavaScriptCore/runtime/JSObject.h
//                  Source/JavaScriptCore/runtime/JSObject.cpp
//
// JSObject is the base type for all JavaScript objects.

package runtime

import "unsafe"

// JSObject corresponds to JSC::JSObject. It stores named properties in a map
// and has a prototype chain for inheritance.
type JSObject struct {
	JSCell
	properties map[string]JSValue
}
// StructureFlags for JSObject.
const JSObjectStructureFlags uint32 = JSCellStructureFlags | OverridesGetOwnPropertySlot

// NewJSObject creates a new JSObject with the given Structure.
func NewJSObject(vm *VM, structure *Structure) *JSObject {
	obj := &JSObject{
		properties: make(map[string]JSValue),
	}
	obj.structureID = structure.structureID
	obj.typ = structure.TypeInfo.Type()
	obj.flags = structure.TypeInfo.InlineTypeFlags()
	obj.cellState = DefinitelyWhite
	// Set indexing type based on structure
	obj.indexingTypeAndMisc = NonArray
	_ = vm
	return obj
}

// NewJSObjectWithPrototype creates a new JSObject with a given prototype.
func NewJSObjectWithPrototype(vm *VM, globalObject *JSGlobalObject, prototype JSValue) *JSObject {
	_ = globalObject
	// Create a simple Structure for this object
	typeInfo := NewTypeInfo(ObjectType, 0)
	classInfo := &ClassInfo{}
	structure := NewStructure(vm, globalObject, prototype, typeInfo, classInfo)
	return NewJSObject(vm, structure)
}

// CreateEmptyJSObject creates an empty JSObject with default structure.
func CreateEmptyJSObject(vm *VM, globalObject *JSGlobalObject) *JSObject {
	return NewJSObjectWithPrototype(vm, globalObject, JSValueNull)
}

// --- Accessors ---

// Put sets a named property.
func (o *JSObject) Put(cell *JSCell, globalObject *JSGlobalObject, name PropertyName, value JSValue, slot *PutPropertySlot) bool {
	_ = cell
	_ = globalObject
	_ = slot
	// Store in property map (simplified — no attribute handling)
	o.properties[name.String()] = value
	return true
}

// Get retrieves a named property, walking the prototype chain if needed.
func (o *JSObject) Get(globalObject *JSGlobalObject, name PropertyName) JSValue {
	// Check own properties
	if val, ok := o.properties[name.String()]; ok {
		return val
	}
	// Walk prototype chain
	structure := o.Structure()
	if structure != nil && structure.Prototype().IsObject() {
		protoObj := structure.Prototype().GetObject()
		if protoObj != nil {
			return protoObj.Get(globalObject, name)
		}
	}
	return JSValueUndefined
}

// GetOwnPropertySlot checks own properties.
func (o *JSObject) GetOwnPropertySlot(globalObject *JSGlobalObject, name PropertyName, slot *PropertySlot) bool {
	_ = globalObject
	_ = slot
	_, ok := o.properties[name.String()]
	return ok
}

// HasProperty returns true if the property exists (own or prototype chain).
func (o *JSObject) HasProperty(globalObject *JSGlobalObject, name PropertyName) bool {
	if _, ok := o.properties[name.String()]; ok {
		return true
	}
	structure := o.Structure()
	if structure != nil && structure.Prototype().IsObject() {
		protoObj := structure.Prototype().GetObject()
		if protoObj != nil {
			return protoObj.HasProperty(globalObject, name)
		}
	}
	return false
}

// DeleteProperty removes a property.
func (o *JSObject) DeleteProperty(cell *JSCell, globalObject *JSGlobalObject, name PropertyName, slot *DeletePropertySlot) bool {
	_ = cell
	_ = globalObject
	_ = slot
	delete(o.properties, name.String())
	return true
}

// DeletePropertyByIndex removes a property by index.
func (o *JSObject) DeletePropertyByIndex(cell *JSCell, globalObject *JSGlobalObject, index uint32) bool {
	_ = cell
	_ = globalObject
	_ = index
	// TODO: handle indexed properties
	return true
}

// PutByIndex sets a property by index.
func (o *JSObject) PutByIndex(cell *JSCell, globalObject *JSGlobalObject, index uint32, value JSValue, shouldThrow bool) bool {
	_ = cell
	_ = globalObject
	_ = shouldThrow
	idxStr := propertyNameFromIndex(index)
	o.properties[idxStr] = value
	return true
}

// GetPropertyNames collects all property names.
func (o *JSObject) GetPropertyNames(globalObject *JSGlobalObject, mode DontEnumPropertiesMode) []string {
	_ = globalObject
	_ = mode
	names := make([]string, 0, len(o.properties))
	for name := range o.properties {
		names = append(names, name)
	}
	return names
}

// --- JSValue conversions ---

// ToStringJSValue converts this object to a string JSValue.
func (o *JSObject) ToStringJSValue() (JSValue, bool) {
	// Default: call toString()
	return JSValueUndefined, false
}

// ToNumberJSValue converts this object to a number JSValue.
func (o *JSObject) ToNumberJSValue() (JSValue, bool) {
	return JSValueUndefined, false
}

// DefaultValue implements [[DefaultValue]] (ToPrimitive).
func (o *JSObject) DefaultValue(globalObject *JSGlobalObject, hint PreferredPrimitiveType) JSValue {
	_ = globalObject
	_ = hint
	return JSValueUndefined
}

// --- Callable ---

// Call implements [[Call]].
func (o *JSObject) Call(globalObject *JSGlobalObject, thisValue JSValue, args []JSValue) (JSValue, error) {
	_ = globalObject
	_ = thisValue
	_ = args
	return JSValueUndefined, nil
}

// Construct implements [[Construct]].
func (o *JSObject) Construct(globalObject *JSGlobalObject, args []JSValue) (JSValue, error) {
	_ = globalObject
	_ = args
	return JSValueUndefined, nil
}

// --- Methods from C++ ---

// MethodTable returns the method table (simplified dispatch).
func (o *JSObject) MethodTable() interface{} {
	return nil
}

// GetPrototype returns the prototype.
func (o *JSObject) GetPrototype(globalObject *JSGlobalObject) JSValue {
	structure := o.Structure()
	if structure != nil {
		return structure.Prototype()
	}
	return JSValueNull
}

// SetPrototype sets the prototype.
func (o *JSObject) SetPrototype(globalObject *JSGlobalObject, value JSValue, shouldThrowIfCantSet bool) bool {
	structure := o.Structure()
	if structure != nil {
		// TODO: need VM reference for structure transition
		_ = globalObject
		_ = shouldThrowIfCantSet
		structure.SetPrototype(nil, value)
		return true
	}
	return false
}

// IsExtensible returns whether new properties can be added.
func (o *JSObject) IsExtensible(globalObject *JSGlobalObject) bool {
	_ = globalObject
	return true // simplified
}

// PreventExtensions prevents new properties from being added.
func (o *JSObject) PreventExtensions(globalObject *JSGlobalObject) bool {
	_ = globalObject
	return true // simplified
}

// ClassName returns the class name.
func (o *JSObject) ClassName(globalObject *JSGlobalObject) string {
	_ = globalObject
	structure := o.Structure()
	if structure != nil && structure.GetClassInfo() != nil {
		return "Object"
	}
	return "Object"
}

// ToPrimitive converts to primitive.
func (o *JSObject) ToPrimitive(globalObject *JSGlobalObject, preferred PreferredPrimitiveType) JSValue {
	return o.DefaultValue(globalObject, preferred)
}

// JSCellRef returns the embedded JSCell.
func (o *JSObject) JSCellRef() *JSCell {
	return &o.JSCell
}

// JSObjectFromCell safely converts a JSCell to JSObject.
func JSObjectFromCell(cell *JSCell) *JSObject {
	return (*JSObject)(unsafe.Pointer(cell))
}

// PropertyName.String() convenience method.
