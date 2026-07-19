package yarr

// YarrInterpreter represents the YARR bytecode interpreter for regex matching.
// It uses Go's regexp package as the backend.
type YarrInterpreter struct {
	pattern *YarrPattern
}

// NewYarrInterpreter creates a new interpreter for the given pattern.
func NewYarrInterpreter(pattern *YarrPattern) *YarrInterpreter {
	return &YarrInterpreter{pattern: pattern}
}

// MatchResult stores the result of a regex match.
type MatchResult struct {
	Start int
	End   int
	Groups []GroupResult
}

// GroupResult stores a group match.
type GroupResult struct {
	Start int
	End   int
	Valid bool
}

// Match executes the regex against the given string.
func (interp *YarrInterpreter) Match(input string) *MatchResult {
	if interp.pattern == nil || interp.pattern.GoRegex == nil {
		return nil
	}
	loc := interp.pattern.GoRegex.FindStringSubmatchIndex(input)
	if loc == nil {
		return nil
	}
	result := &MatchResult{
		Start: loc[0],
		End:   loc[1],
	}
	for i := 2; i < len(loc); i += 2 {
		gr := GroupResult{Valid: loc[i] >= 0}
		if gr.Valid {
			gr.Start = loc[i]
			gr.End = loc[i+1]
		}
		result.Groups = append(result.Groups, gr)
	}
	return result
}

// MatchGlobal executes the regex globally against the given string.
func (interp *YarrInterpreter) MatchGlobal(input string) []*MatchResult {
	if interp.pattern == nil || interp.pattern.GoRegex == nil {
		return nil
	}
	locs := interp.pattern.GoRegex.FindAllStringSubmatchIndex(input, -1)
	if locs == nil {
		return nil
	}
	var results []*MatchResult
	for _, loc := range locs {
		result := &MatchResult{
			Start: loc[0],
			End:   loc[1],
		}
		for i := 2; i < len(loc); i += 2 {
			gr := GroupResult{Valid: loc[i] >= 0}
			if gr.Valid {
				gr.Start = loc[i]
				gr.End = loc[i+1]
			}
			result.Groups = append(result.Groups, gr)
		}
		results = append(results, result)
	}
	return results
}
