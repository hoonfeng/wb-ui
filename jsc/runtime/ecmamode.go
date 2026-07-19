// ECMAMode corresponds to JSC::ECMAMode (runtime/ECMAMode.h)
package runtime

// ECMAMode corresponds to JSC::ECMAMode.
type ECMAMode struct {
	value uint8
}

var (
	ECMAModeStrict = ECMAMode{value: 0}
	ECMAModeSloppy = ECMAMode{value: 1}
)

// ECMAModeFromByte creates an ECMAMode from a byte value.
func ECMAModeFromByte(b uint8) ECMAMode { return ECMAMode{value: b} }

// ECMAModeFromBool creates an ECMAMode from a bool.
func ECMAModeFromBool(isStrict bool) ECMAMode {
	if isStrict {
		return ECMAModeStrict
	}
	return ECMAModeSloppy
}

// IsStrict returns true if this is strict mode.
func (m ECMAMode) IsStrict() bool { return m.value == 0 }

// Value returns the underlying byte value.
func (m ECMAMode) Value() uint8 { return m.value }
