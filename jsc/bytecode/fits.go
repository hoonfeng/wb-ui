// Fits.h - type trait for checking if values fit within a container
package bytecode

// Fits checks if a value can fit within a container of type T.
// Go version uses simple type assertions.
func FitsInInt8(v int32) bool { return v >= -128 && v <= 127 }
func FitsInUInt8(v int32) bool { return v >= 0 && v <= 255 }
func FitsInInt16(v int32) bool { return v >= -32768 && v <= 32767 }
func FitsInUInt16(v int32) bool { return v >= 0 && v <= 65535 }
