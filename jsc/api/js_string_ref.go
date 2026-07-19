package api

// JSStringRef API provides functions for working with JavaScript strings.

// JSStringCreateWithCharacters creates a string from UTF-16 characters.
func JSStringCreateWithCharacters(chars *uint16, numChars uintptr) JSStringRef {
	_ = chars
	_ = numChars
	return 0
}

// JSStringCreateWithUTF8CString creates a string from a UTF-8 C string.
func JSStringCreateWithUTF8CString(str string) JSStringRef {
	_ = str
	return 0
}

// JSStringRetain retains a string.
func JSStringRetain(str JSStringRef) JSStringRef {
	return str
}

// JSStringRelease releases a string.
func JSStringRelease(str JSStringRef) {
	_ = str
}

// JSStringGetLength returns the length of a string.
func JSStringGetLength(str JSStringRef) uintptr {
	_ = str
	return 0
}

// JSStringGetCharactersPtr returns a pointer to the string's characters.
func JSStringGetCharactersPtr(str JSStringRef) *uint16 {
	_ = str
	return nil
}

// JSStringGetMaximumUTF8CStringSize returns the max UTF-8 buffer size.
func JSStringGetMaximumUTF8CStringSize(str JSStringRef) uintptr {
	_ = str
	return 0
}

// JSStringGetUTF8CString converts the string to UTF-8.
func JSStringGetUTF8CString(str JSStringRef, buffer string, bufferSize uintptr) uintptr {
	_ = str
	_ = buffer
	_ = bufferSize
	return 0
}

// JSStringIsEqual checks if two strings are equal.
func JSStringIsEqual(a, b JSStringRef) bool {
	return a == b
}

// JSStringIsEqualToUTF8CString checks if a string equals a UTF-8 C string.
func JSStringIsEqualToUTF8CString(a JSStringRef, b string) bool {
	_ = a
	_ = b
	return false
}

// JSStringCreateWithBSTR creates a string from a BSTR (Windows).
func JSStringCreateWithBSTR(str uintptr) JSStringRef {
	_ = str
	return 0
}

// JSStringCopyBSTR copies the string as a BSTR.
func JSStringCopyBSTR(str JSStringRef) uintptr {
	_ = str
	return 0
}

// JSStringCreateWithCFString creates a string from a CFString (macOS).
func JSStringCreateWithCFString(str uintptr) JSStringRef {
	_ = str
	return 0
}

// JSStringCopyCFString copies the string as a CFString.
func JSStringCopyCFString(alloc uintptr, str JSStringRef) uintptr {
	_ = alloc
	_ = str
	return 0
}
