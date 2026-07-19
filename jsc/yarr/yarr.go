// Package yarr implements WebKit's Yet Another Regex Runtime (YARR) regex engine.
package yarr

import "math"

const (
	YarrStackSpaceForBackTrackInfoPatternCharacter       = 2
	YarrStackSpaceForBackTrackInfoCharacterClass          = 2
	YarrStackSpaceForBackTrackInfoBackReference           = 3
	YarrStackSpaceForBackTrackInfoAlternative             = 1
	YarrStackSpaceForBackTrackInfoParentheticalAssertion  = 1
	YarrStackSpaceForBackTrackInfoParenthesesOnce         = 2
	YarrStackSpaceForBackTrackInfoParenthesesTerminal     = 1
	YarrStackSpaceForBackTrackInfoParentheses             = 4
	YarrStackSpaceForDotStarEnclosure                     = 1
)

const (
	QuantifyInfinite   = math.MaxUint32
	QuantifyInfinite64 = math.MaxUint64
	OffsetNoMatch      = math.MaxUint32
	MatchLimit         = 100000000
)

// MatchFrom indicates whether matching is from the VM thread or Compiler thread.
type MatchFrom uint8

const (
	MatchFromVMThread     MatchFrom = 0
	MatchFromCompilerThread MatchFrom = 1
)

// JSRegExpResult represents the result of a regex match.
type JSRegExpResult int

const (
	JSRegExpResultMatch          JSRegExpResult = 1
	JSRegExpResultNoMatch        JSRegExpResult = 0
	JSRegExpResultErrorNoMatch   JSRegExpResult = -1
	JSRegExpResultJITCodeFailure JSRegExpResult = -2
	JSRegExpResultErrorHitLimit  JSRegExpResult = -3
	JSRegExpResultErrorNoMemory  JSRegExpResult = -4
	JSRegExpResultErrorInternal  JSRegExpResult = -5
)

// CharSize represents character size for regex matching.
type CharSize uint8

const (
	CharSize8  CharSize = 0
	CharSize16 CharSize = 1
)

// BuiltInCharacterClassID represents builtin character class IDs.
type BuiltInCharacterClassID uint32

const (
	BuiltInCharacterClassIDDigitClassID         BuiltInCharacterClassID = 0
	BuiltInCharacterClassIDSpaceClassID         BuiltInCharacterClassID = 1
	BuiltInCharacterClassIDWordClassID          BuiltInCharacterClassID = 2
	BuiltInCharacterClassIDDotClassID           BuiltInCharacterClassID = 3
	BuiltInCharacterClassIDBaseUnicodePropertyID BuiltInCharacterClassID = 4
)

// SpecificPattern identifies specific patterns for optimization.
type SpecificPattern uint8

const (
	SpecificPatternNone              SpecificPattern = 0
	SpecificPatternAtom              SpecificPattern = 1
	SpecificPatternLeadingSpacesStar SpecificPattern = 2
	SpecificPatternLeadingSpacesPlus SpecificPattern = 3
	SpecificPatternTrailingSpacesStar SpecificPattern = 4
	SpecificPatternTrailingSpacesPlus SpecificPattern = 5
	SpecificPatternNewlines          SpecificPattern = 6
)
