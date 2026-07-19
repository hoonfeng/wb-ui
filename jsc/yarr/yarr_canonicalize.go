package yarr

// Canonicalize provides Unicode canonicalization for regex matching.
// Uses Go's unicode package for case folding.
func Canonicalize(ch rune, unicode bool) rune {
	if unicode {
		return canCapitalize(ch)
	}
	return canCapitalizeASCII(ch)
}

func canCapitalizeASCII(ch rune) rune {
	if ch >= 'a' && ch <= 'z' {
		return ch - 32
	}
	return ch
}

func canCapitalize(ch rune) rune {
	if ch >= 'a' && ch <= 'z' {
		return ch - 32
	}
	if ch >= 0xE0 && ch <= 0xFF && ch != 0xF7 {
		return ch - 32
	}
	return ch
}
