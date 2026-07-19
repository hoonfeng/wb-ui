// Translation of: Source/JavaScriptCore/runtime/IndexingType.h
//
// IndexingType describes the shape of indexed property storage in JSArray/JSObject.

package runtime

// IndexingType is a uint8 bitfield describing how an object stores indexed properties.
//   bit 0: IsArray (1 = object is an array instance)
//   bit 1-3: Shape (type of indexed storage)
// IndexingType is a uint8 bitfield describing how an object stores indexed properties.
type IndexingType uint8

// Shape values for indexing storage.
// Shape values for indexing storage.
const (
	IsArray             IndexingType = 0x01
	NoIndexingShape     IndexingType = 0x00
	UndecidedShape      IndexingType = 0x02
	Int32Shape          IndexingType = 0x04
	DoubleShape         IndexingType = 0x06
	ContiguousShape     IndexingType = 0x08
	ArrayStorageShape   IndexingType = 0x0A
	SlowPutArrayStorageShape IndexingType = 0x0C

	IndexingShapeMask      IndexingType = 0x0E
	IndexingShapeShift     IndexingType = 1
	NumberOfIndexingShapes IndexingType = 7
	IndexingTypeMask       IndexingType = IndexingShapeMask | IsArray

	CopyOnWrite                 IndexingType = 0x10
	IndexingShapeAndWritabilityMask = CopyOnWrite | IndexingShapeMask
	IndexingModeMask            = CopyOnWrite | IndexingTypeMask
	NumberOfCopyOnWriteIndexingModes = 3
	NumberOfArrayIndexingModes  = NumberOfIndexingShapes + NumberOfCopyOnWriteIndexingModes

	MayHaveIndexedAccessors IndexingType = 0x20
	IndexingTypeLockIsHeld  IndexingType = 0x40
	IndexingTypeLockHasParked IndexingType = 0x80
)

// Named indexing type combinations.
const (
	NonArray                        IndexingType = 0x0
	NonArrayWithInt32               IndexingType = Int32Shape
	NonArrayWithDouble              IndexingType = DoubleShape
	NonArrayWithContiguous          IndexingType = ContiguousShape
	NonArrayWithArrayStorage        IndexingType = ArrayStorageShape
	NonArrayWithSlowPutArrayStorage IndexingType = SlowPutArrayStorageShape
	ArrayClass                      IndexingType = IsArray
	ArrayWithUndecided              IndexingType = IsArray | UndecidedShape
	ArrayWithInt32                  IndexingType = IsArray | Int32Shape
	ArrayWithDouble                 IndexingType = IsArray | DoubleShape
	ArrayWithContiguous             IndexingType = IsArray | ContiguousShape
	ArrayWithArrayStorage           IndexingType = IsArray | ArrayStorageShape
	ArrayWithSlowPutArrayStorage    IndexingType = IsArray | SlowPutArrayStorageShape
	CopyOnWriteArrayWithInt32       IndexingType = IsArray | Int32Shape | CopyOnWrite
	CopyOnWriteArrayWithDouble      IndexingType = IsArray | DoubleShape | CopyOnWrite
	CopyOnWriteArrayWithContiguous  IndexingType = IsArray | ContiguousShape | CopyOnWrite
)

// All array-type indexing modes (writable + copy-on-write).
const (
	AllWritableArrayTypes   IndexingType = IndexingShapeMask | IsArray
	AllArrayTypes           IndexingType = AllWritableArrayTypes | CopyOnWrite
	AllWritableArrayTypesAndHistory = AllWritableArrayTypes | MayHaveIndexedAccessors
	AllArrayTypesAndHistory = AllArrayTypes | MayHaveIndexedAccessors
)

// HasIndexedProperties returns true if the indexing type has non-trivial shape.
func HasIndexedProperties(t IndexingType) bool {
	return (t & IndexingShapeMask) != NoIndexingShape
}

// HasUndecided returns true if the shape is Undecided.
func HasUndecided(t IndexingType) bool {
	return (t & IndexingShapeMask) == UndecidedShape
}

// HasInt32 returns true if the shape is Int32.
func HasInt32(t IndexingType) bool {
	return (t & IndexingShapeMask) == Int32Shape
}

// HasDouble returns true if the shape is Double.
func HasDouble(t IndexingType) bool {
	return (t & IndexingShapeMask) == DoubleShape
}

// HasContiguous returns true if the shape is Contiguous.
func HasContiguous(t IndexingType) bool {
	return (t & IndexingShapeMask) == ContiguousShape
}

// HasArrayStorage returns true if the shape is ArrayStorage.
func HasArrayStorage(t IndexingType) bool {
	return (t & IndexingShapeMask) == ArrayStorageShape
}

// HasAnyArrayStorage returns true if the shape is >= ArrayStorageShape.
func HasAnyArrayStorage(t IndexingType) bool {
	return uint8(t&IndexingShapeMask) >= uint8(ArrayStorageShape)
}

// HasSlowPutArrayStorage returns true if the shape is SlowPutArrayStorage.
func HasSlowPutArrayStorage(t IndexingType) bool {
	return (t & IndexingShapeMask) == SlowPutArrayStorageShape
}

// ShouldUseSlowPut returns true if slow-put semantics apply.
func ShouldUseSlowPut(t IndexingType) bool {
	return HasSlowPutArrayStorage(t)
}

// IsCopyOnWrite returns true if the copy-on-write flag is set.
func IsCopyOnWrite(t IndexingType) bool {
	return t&CopyOnWrite != 0
}
