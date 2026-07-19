// PropertyInlineCache / Inline cache for property access (JIT only)
package bytecode

type PropertyInlineCache struct {
	structureID uint64
	lastOffset  uint32
	hitCount    uint32
}

type PropertyInlineCacheClearingWatchpoint struct{}
type PropertyInlineCacheSummary struct{}

// PutByIdFlags - flags for put_by_id bytecode
type PutByIdFlags uint8

const (
	PutByIdIsDirect  PutByIdFlags = 1 << 0
	PutByIdIsStrict  PutByIdFlags = 1 << 1
)

// ReduceWhitespace - utility to reduce whitespace in strings
func ReduceWhitespace(s string) string {
	result := make([]byte, 0, len(s))
	lastWasSpace := false
	for i := 0; i < len(s); i++ {
		if s[i] == ' ' || s[i] == '\t' || s[i] == '\n' || s[i] == '\r' {
			if !lastWasSpace {
				result = append(result, ' ')
				lastWasSpace = true
			}
		} else {
			result = append(result, s[i])
			lastWasSpace = false
		}
	}
	return string(result)
}
