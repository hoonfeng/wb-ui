package tools

// FunctionAllowlist manages a list of functions allowed for specific operations.
type FunctionAllowlist struct {
	entries []string
}

// NewFunctionAllowlist creates an empty allowlist.
func NewFunctionAllowlist() *FunctionAllowlist {
	return &FunctionAllowlist{}
}

// Add adds a function name to the allowlist.
func (l *FunctionAllowlist) Add(name string) {
	l.entries = append(l.entries, name)
}

// Contains checks if a function name is in the allowlist.
func (l *FunctionAllowlist) Contains(name string) bool {
	for _, e := range l.entries {
		if e == name {
			return true
		}
	}
	return false
}
