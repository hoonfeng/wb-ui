package yarr

// SyntaxChecker is a placeholder for YARR syntax checking.
type SyntaxChecker struct{}

// CheckSyntax checks the syntax of a regex pattern string.
func CheckSyntax(pattern string, flags Flags) ErrorCode {
	_, err := NewYarrPattern(pattern, flags)
	return err
}
