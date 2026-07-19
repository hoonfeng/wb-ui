package inspector

// InspectorProtocolTypes provides types for the inspector protocol.

// ProtocolType is an enum for inspector protocol value types.
type ProtocolType uint8

const (
	ProtocolTypeObject  ProtocolType = 0
	ProtocolTypeArray   ProtocolType = 1
	ProtocolTypeString  ProtocolType = 2
	ProtocolTypeNumber  ProtocolType = 3
	ProtocolTypeBoolean ProtocolType = 4
	ProtocolTypeNull    ProtocolType = 5
	ProtocolTypeAny     ProtocolType = 6
)

// ProtocolValue represents a value in the inspector protocol.
type ProtocolValue struct {
	Type   ProtocolType
	Value  interface{}
	Object map[string]*ProtocolValue
	Array  []*ProtocolValue
}
