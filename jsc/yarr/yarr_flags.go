package yarr

// Flags represents regex flags as a bitfield.
type Flags uint16

const (
	FlagHasIndices   Flags = 1 << 0
	FlagGlobal       Flags = 1 << 1
	FlagIgnoreCase   Flags = 1 << 2
	FlagMultiline    Flags = 1 << 3
	FlagDotAll       Flags = 1 << 4
	FlagUnicode      Flags = 1 << 5
	FlagUnicodeSets  Flags = 1 << 6
	FlagSticky       Flags = 1 << 7
	FlagDeletedValue Flags = 1 << 8
)

const NumberOfFlags = 8

// ModFlags represents the subset of flags that can be changed in mod mode.
type ModFlags uint8

const (
	ModFlagIgnoreCase ModFlags = 1 << 0
	ModFlagMultiline  ModFlags = 1 << 1
	ModFlagDotAll     ModFlags = 1 << 2
)

// ParseFlags parses a string into regex flags.
func ParseFlags(s string) (Flags, bool) {
	var result Flags
	for _, ch := range s {
		switch ch {
		case 'd':
			result |= FlagHasIndices
		case 'g':
			result |= FlagGlobal
		case 'i':
			result |= FlagIgnoreCase
		case 'm':
			result |= FlagMultiline
		case 's':
			result |= FlagDotAll
		case 'u':
			result |= FlagUnicode
		case 'v':
			result |= FlagUnicodeSets
		case 'y':
			result |= FlagSticky
		default:
			return 0, false
		}
	}
	return result, true
}

// FlagsString converts flags to a string.
func FlagsString(f Flags) string {
	buf := make([]byte, 0, NumberOfFlags)
	if f&FlagHasIndices != 0 {
		buf = append(buf, 'd')
	}
	if f&FlagGlobal != 0 {
		buf = append(buf, 'g')
	}
	if f&FlagIgnoreCase != 0 {
		buf = append(buf, 'i')
	}
	if f&FlagMultiline != 0 {
		buf = append(buf, 'm')
	}
	if f&FlagDotAll != 0 {
		buf = append(buf, 's')
	}
	if f&FlagUnicode != 0 {
		buf = append(buf, 'u')
	}
	if f&FlagUnicodeSets != 0 {
		buf = append(buf, 'v')
	}
	if f&FlagSticky != 0 {
		buf = append(buf, 'y')
	}
	return string(buf)
}
