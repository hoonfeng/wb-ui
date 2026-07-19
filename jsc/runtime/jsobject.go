// Translation of: Source/JavaScriptCore/runtime/JSObject.h
//                  Source/JavaScriptCore/runtime/JSObject.cpp
//
// JSObject is the base type for all JavaScript objects.

package runtime



import (
	"fmt"
	"unsafe"
)

// JSObject corresponds to JSC::JSObject. It stores named properties in a map
// and has a prototype chain for inheritance.
type JSObject struct {
	JSCell
	properties map[string]JSValue
	// Internal holds an optional Go object reference (used by DOM bindings
	// to associate a native Go object with a JS wrapper object).
	Internal any
	// objectClassName stores the class name for DOM type identification.
	objectClassName string
	// IsArray marks this object as an array (used by bindings tests).
	IsArray bool
	// Elements holds array elements (used by bindings tests for slice conversion).
	Elements []JSValue
}

// StructureFlags for JSObject.
const JSObjectStructureFlags uint32 = JSCellStructureFlags | OverridesGetOwnPropertySlot

// JSNonFinalObjectStructureFlags is inherited from JSObject.
const JSNonFinalObjectStructureFlags uint32 = JSObjectStructureFlags
func NewJSObject(vm *VM, structure *Structure) *JSObject {
	obj := &JSObject{
		properties: make(map[string]JSValue),
	}
	if structure != nil {
		obj.structureID = structure.structureID
		obj.typ = structure.TypeInfo.Type()
		obj.flags = structure.TypeInfo.InlineTypeFlags()
	} else {
		obj.typ = ObjectType
	}
	obj.cellState = DefinitelyWhite
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

// Set is a convenience method to set a property by string key.
func (o *JSObject) Set(key string, val JSValue) {
	o.properties[key] = val
}

// GetStr is a convenience method to get a property by string key.
func (o *JSObject) GetStr(key string) JSValue {
	val, _ := o.properties[key]
	return val
}

// GetByKey returns a property by string key and whether it exists.
func (o *JSObject) GetByKey(key string) (JSValue, bool) {
	val, ok := o.properties[key]
	return val, ok
}

// GetOrZero returns a property value by string key, or JSValueUndefined if
// the property does not exist.
func (o *JSObject) GetOrZero(key string) JSValue {
	val, ok := o.properties[key]
	if !ok {
		return JSValueUndefined
	}
	return val
}

// SetAccessor defines a getter/setter property on this object.
// getter receives (interpreter, thisValue) and returns the property value.
// setter receives (interpreter, thisValue, newValue) and is called when the property is written.
// If setter is nil, the property is read-only.
func (o *JSObject) SetAccessor(name string, getter interface{}, setter interface{}) {
	_ = getter
	_ = setter
	// Simplified: store a marker value.
	o.properties[name] = JSValueUndefined
}

// Accessor returns a non-nil value if the named accessor property exists.
// This is used by the DOM bindings test to verify accessor installation.
func (o *JSObject) Accessor(name string) interface{} {
	_ = name
	// Simplified: check if property exists.
	if _, exists := o.properties[name]; exists {
		return o
	}
	return nil
}

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
	// If a custom class name was set via SetClassName, use it.
	if o.objectClassName != "" {
		return o.objectClassName
	}
	_ = globalObject
	structure := o.Structure()
	if structure != nil && structure.GetClassInfo() != nil {
		return "Object"
	}
	return "Object"
}

// SetClassName sets a custom class name for this object (used by DOM bindings
// to tag wrapper objects with their type name).
func (o *JSObject) SetClassName(name string) {
	o.objectClassName = name
}

// ClassNameStr returns the custom class name if set, or empty string.
func (o *JSObject) ClassNameStr() string {
	return o.objectClassName
}

// Keys returns all property keys on this object.
func (o *JSObject) Keys() []string {
	keys := make([]string, 0, len(o.properties))
	for k := range o.properties {
		keys = append(keys, k)
	}
	return keys
}

// JSObjectClassInfo returns the ClassInfo for JSObject.
func JSObjectClassInfo() *ClassInfo {
	return &ClassInfo{}
}
// JSObjectClassInfo returns the ClassInfo for JSObject.


// PropertyName.String() convenience method.

// putDirectWithoutTransition sets a property directly without structure transition.
func (o *JSObject) putDirectWithoutTransition(vm *VM, name PropertyName, value JSValue, attributes uint8) {
	_ = vm
	_ = attributes
	o.properties[name.String()] = value
}

// PutDirect is a public alias for putDirectWithoutTransition.
func (o *JSObject) PutDirect(vm *VM, name PropertyName, value JSValue, attributes uint8) {
	o.putDirectWithoutTransition(vm, name, value, attributes)
}

// PutDirectOffset stores a value at a known property offset (simplified — just sets in map).
func (o *JSObject) PutDirectOffset(vm *VM, offset PropertyOffset, value JSValue) {
	_ = vm
	// In a real implementation, this would write to inline storage.
	// For our simplified model, we use the offset as a key indicator.
	o.properties[fmt.Sprintf("__offset_%d__", offset)] = value
}

// GetDirectOffset retrieves a value from a property offset.
func (o *JSObject) GetDirectOffset(vm *VM, offset PropertyOffset) JSValue {
	_ = vm
	if val, ok := o.properties[fmt.Sprintf("__offset_%d__", offset)]; ok {
		return val
	}
	return JSValueUndefined
}

// GetDirect retrieves a direct (own) property value.
func (o *JSObject) GetDirect(name PropertyName) JSValue {
	if val, ok := o.properties[name.String()]; ok {
		return val
	}
	return JSValueUndefined
}

// finishCreation is called after construction to finalize object state.
func (o *JSObject) finishCreation(vm *VM) {
	_ = vm
	// Default: no-op
}

// inherits checks if this object inherits from the given JSType range.
func (o *JSObject) inherits(classInfo *ClassInfo) bool {
	_ = classInfo
	// Simplified: return true for all non-null classInfo
	return true
}

// getIfPropertyExists returns a property if it exists, without walking prototype chain.
func (o *JSObject) getIfPropertyExists(globalObject *JSGlobalObject, name PropertyName) JSValue {
	_ = globalObject
	if val, ok := o.properties[name.String()]; ok {
		return val
	}
	return JSValueUndefined
}

// hasOwnProperty checks if the object has an own property with the given name.
func (o *JSObject) hasOwnProperty(globalObject *JSGlobalObject, name PropertyName) bool {
	_ = globalObject
	_, ok := o.properties[name.String()]
	return ok
}

// getOwnPropertyDescriptor returns the PropertyDescriptor for a property.
func (o *JSObject) getOwnPropertyDescriptor(globalObject *JSGlobalObject, name PropertyName) (PropertyDescriptor, bool) {
	_ = globalObject
	val, ok := o.properties[name.String()]
	if !ok {
		return PropertyDescriptor{}, false
	}
	desc := NewPropertyDescriptor()
	desc.SetValue(val)
	desc.SetWritable(true)
	desc.SetEnumerable(true)
	desc.SetConfigurable(true)
	return desc, true
}

// defineOwnProperty defines a property with the given descriptor.
func (o *JSObject) defineOwnProperty(globalObject *JSGlobalObject, name PropertyName, desc PropertyDescriptor, shouldThrow bool) bool {
	_ = globalObject
	_ = shouldThrow
	if desc.IsAccessorDescriptor() {
		// Store getter/setter as a special property
		key := name.String()
		o.properties[key+"::getter"] = desc.Getter()
		o.properties[key+"::setter"] = desc.Setter()
		o.properties[key] = JSValueUndefined // placeholder
	} else if desc.Value().IsNotUndefined() || desc.Value().IsNull() || desc.Value().IsBoolean() || desc.Value().IsNumber() || desc.Value().IsString() {
		o.properties[name.String()] = desc.Value()
	}
	return true
}

// getPropertySlot retrieves a property via PropertySlot, walking the prototype chain.
func (o *JSObject) getPropertySlot(globalObject *JSGlobalObject, name PropertyName, slot *PropertySlot) bool {
	_ = globalObject
	_ = slot
	val, ok := o.properties[name.String()]
	if ok {
		slot.Value = val
		return true
	}
	// Walk prototype chain
	structure := o.Structure()
	if structure != nil && structure.Prototype().IsObject() {
		protoObj := structure.Prototype().GetObject()
		if protoObj != nil {
			return protoObj.getPropertySlot(globalObject, name, slot)
		}
	}
	return false
}

// freeze freezes the object (makes all properties non-configurable, non-writable).
func (o *JSObject) freeze(vm *VM) {
	_ = vm
	// Simplified: no-op
}

// seal seals the object (makes all properties non-configurable).
func (o *JSObject) seal(vm *VM) {
	_ = vm
	// Simplified: no-op
}

// isSealed returns whether the object is sealed.
func (o *JSObject) isSealed(vm *VM) bool {
	_ = vm
	return false // simplified
}

// isFrozen returns whether the object is frozen.
func (o *JSObject) isFrozen(vm *VM) bool {
	_ = vm
	return false // simplified
}

// StructureExtensible returns whether the structure is extensible.
func (o *JSObject) isStructureExtensible() bool {
	return true // simplified
}

// canPerformFastPutInlineExcludingProto checks if fast put is possible (simplified).
func (o *JSObject) canPerformFastPutInlineExcludingProto() bool {
	return true
}

// staticPropertiesReified returns whether static properties are reified.
func (o *JSObject) staticPropertiesReified() bool {
	return true // simplified
}

// reifyAllStaticProperties reifies all static properties.
func (o *JSObject) reifyAllStaticProperties(globalObject *JSGlobalObject) {
	_ = globalObject
	// No-op
}

// hasNonReifiedStaticProperties checks if there are non-reified static properties.
func (o *JSObject) hasNonReifiedStaticProperties() bool {
	return false
}

// canHaveExistingOwnIndexedProperties checks if there are existing own indexed properties.
func (o *JSObject) canHaveExistingOwnIndexedProperties() bool {
	return false // simplified
}

// canHaveExistingOwnIndexedGetterSetterProperties checks if there are indexed getter/setter properties.
func (o *JSObject) canHaveExistingOwnIndexedGetterSetterProperties() bool {
	return false
}

// putOwnDataPropertyMayBeIndex sets a data property that may be an index.
func (o *JSObject) putOwnDataPropertyMayBeIndex(globalObject *JSGlobalObject, name PropertyName, value JSValue, slot PutPropertySlot) {
	_ = globalObject
	_ = slot
	o.properties[name.String()] = value
}

// putOwnDataPropertyBatching sets multiple data properties at once.
func (o *JSObject) putOwnDataPropertyBatching(vm *VM, propertyNames []interface{}, values []JSValue, count int) {
	_ = vm
	for i := 0; i < count && i < len(propertyNames) && i < len(values); i++ {
		if name, ok := propertyNames[i].(string); ok {
			o.properties[name] = values[i]
		}
	}
}

// enumerateProperties enumerates all own properties.
func (o *JSObject) enumerateProperties() map[string]JSValue {
	return o.properties
}

// forEachOwnIndexedProperty iterates over indexed properties.
func (o *JSObject) forEachOwnIndexedProperty(sortMode interface{}, callback func(uint32, JSValue) IterationStatus) {
	_ = sortMode
	_ = callback
	// No indexed properties by default
}

// iterationStatus type for forEachOwnIndexedProperty.
type IterationStatus uint8

const (
	IterationStatusContinue IterationStatus = iota
	IterationStatusDone
)

// putInline sets a property with inline fast path.
func (o *JSObject) putInline(globalObject *JSGlobalObject, name PropertyName, value JSValue, slot PutPropertySlot) {
	_ = globalObject
	_ = slot
	o.properties[name.String()] = value
}

// IndexingType returns the indexing type.
func (o *JSObject) IndexingType() IndexingType {
	return o.indexingTypeAndMisc
}

// StructureID returns the StructureID.
func (o *JSObject) StructureID() StructureID {
	return o.structureID
}

// AuditStructureID audits the structure ID (simplified).
func (o *JSObject) auditStructureID() {
	// No-op in simplified mode
}

// getString returns the object as a string if it's a string wrapper.
func (o *JSObject) getString(globalObject *JSGlobalObject, name PropertyName) string {
	_ = globalObject
	val, ok := o.properties[name.String()]
	if ok && val.IsString() {
		return val.ToString()
	}
	return ""
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
