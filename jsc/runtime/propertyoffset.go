// PropertyOffset corresponds to JSC::PropertyOffset (runtime/PropertyOffset.h)
package runtime

// PropertyOffset corresponds to JSC::PropertyOffset.
type PropertyOffset int

const (
	InvalidOffset        PropertyOffset = -1
	FirstOutOfLineOffset PropertyOffset = 64
	KnownPolyProtoOffset PropertyOffset = 0
)

// CheckOffset checks that a PropertyOffset is valid.
func CheckOffset(offset PropertyOffset) {
	_ = offset
	ASSERT(offset >= InvalidOffset)
}

// ValidateOffset validates a PropertyOffset.
func ValidateOffset(offset PropertyOffset) {
	ASSERT(isValidOffset(offset))
}

// IsValidOffset returns true if the offset is valid.
func isValidOffset(offset PropertyOffset) bool {
	return offset >= 0
}

// IsInlineOffset returns true if the offset is inline.
func IsInlineOffset(offset PropertyOffset) bool {
	ASSERT(isValidOffset(offset))
	return offset < FirstOutOfLineOffset
}

// IsOutOfLineOffset returns true if the offset is out-of-line.
func IsOutOfLineOffset(offset PropertyOffset) bool {
	ASSERT(isValidOffset(offset))
	return !IsInlineOffset(offset)
}

// OffsetInInlineStorage returns the index in inline storage.
func OffsetInInlineStorage(offset PropertyOffset) int {
	ASSERT(IsInlineOffset(offset))
	return int(offset)
}

// OffsetInOutOfLineStorage returns the index in out-of-line storage.
func OffsetInOutOfLineStorage(offset PropertyOffset) int {
	ASSERT(IsOutOfLineOffset(offset))
	return int(offset - FirstOutOfLineOffset)
}

// OffsetInRespectiveStorage returns the index in the respective storage.
func OffsetInRespectiveStorage(offset PropertyOffset) int {
	if IsInlineOffset(offset) {
		return OffsetInInlineStorage(offset)
	}
	return OffsetInOutOfLineStorage(offset)
}
