package wtf

import "testing"

func TestStringBasic(t *testing.T) {
	s := NewString("hello")
	if s.IsEmpty() {
		t.Fatalf("NewString should not be empty")
	}
	if got, want := s.String(), "hello"; got != want {
		t.Fatalf("String() = %q, want %q", got, want)
	}
	if got, want := s.ByteLength(), 5; got != want {
		t.Fatalf("ByteLength = %d, want %d", got, want)
	}
}

func TestStringEquals(t *testing.T) {
	a := NewString("abc")
	b := NewString("abc")
	c := NewString("abd")
	if !a.Equals(b) {
		t.Fatalf("Equals should be true for equal strings")
	}
	if a.Equals(c) {
		t.Fatalf("Equals should be false for different strings")
	}
	if !a.EqualsString("abc") {
		t.Fatalf("EqualsString should be true")
	}
}

func TestStringIsASCIILatin1(t *testing.T) {
	if !NewString("hello").IsASCII() {
		t.Fatalf("ASCII string should be ASCII")
	}
	if NewString("héllo").IsASCII() {
		t.Fatalf("non-ASCII string should not be ASCII")
	}
	if !NewString("café").IsLatin1() {
		t.Fatalf("Latin-1 string should be Latin1")
	}
	if NewString("日本語").IsLatin1() {
		t.Fatalf("non-Latin-1 string should not be Latin1")
	}
}

func TestStringUTF16LengthAndCharCodeAt(t *testing.T) {
	ascii := NewString("ABC")
	if got, want := ascii.UTF16Length(), 3; got != want {
		t.Fatalf("ASCII UTF16Length = %d, want %d", got, want)
	}
	if got, ok := ascii.CharCodeAt(1); !ok || got != 'B' {
		t.Fatalf("CharCodeAt(1) = (%d,%v), want (B,true)", got, ok)
	}
	if _, ok := ascii.CharCodeAt(99); ok {
		t.Fatalf("CharCodeAt out of range should be false")
	}

	// U+1F600 (😀) is an astral character encoded as a surrogate pair in UTF-16.
	emoji := NewString("😀")
	if got, want := emoji.UTF16Length(), 2; got != want {
		t.Fatalf("emoji UTF16Length = %d, want %d (surrogate pair)", got, want)
	}
	// First code unit should be a high surrogate.
	if got, _ := emoji.CharCodeAt(0); got < 0xD800 || got > 0xDBFF {
		t.Fatalf("first code unit = %x, want high surrogate", got)
	}
}

func TestStringSubstring(t *testing.T) {
	s := NewString("Hello, World")
	if got, want := s.Substring(0, 5).String(), "Hello"; got != want {
		t.Fatalf("Substring(0,5) = %q, want %q", got, want)
	}
	if got, want := s.Substring(7, 5).String(), "World"; got != want {
		t.Fatalf("Substring(7,5) = %q, want %q", got, want)
	}
	// maxLength beyond end is clamped.
	if got, want := s.Substring(7, 100).String(), "World"; got != want {
		t.Fatalf("Substring(7,100) = %q, want %q", got, want)
	}
	// start beyond end returns empty.
	if got := s.Substring(999, 5); !got.IsEmpty() {
		t.Fatalf("Substring past end = %q, want empty", got)
	}
	// Astral characters split on surrogate boundaries.
	emoji := NewString("a😀b")
	if got, want := emoji.Substring(0, 2).UTF16Length(), 2; got != want {
		t.Fatalf("emoji substring UTF16Length = %d, want %d", got, want)
	}
}

func TestStringCase(t *testing.T) {
	if got, want := NewString("Hello").ToASCIIUpper().String(), "HELLO"; got != want {
		t.Fatalf("Upper = %q, want %q", got, want)
	}
	if got, want := NewString("Hello").ToASCIILower().String(), "hello"; got != want {
		t.Fatalf("Lower = %q, want %q", got, want)
	}
}

func TestStringContainsStartsEnds(t *testing.T) {
	s := NewString("Hello, World")
	if !s.Contains("World") {
		t.Fatalf("Contains(World) = false")
	}
	if s.Contains("xyz") {
		t.Fatalf("Contains(xyz) = true, want false")
	}
	if !s.StartsWith("Hello") {
		t.Fatalf("StartsWith(Hello) = false")
	}
	if !s.EndsWith("World") {
		t.Fatalf("EndsWith(World) = false")
	}
}

func TestStringIndexOfAppend(t *testing.T) {
	s := NewString("Hello, World")
	if got, want := s.IndexOf("World"), 7; got != want {
		t.Fatalf("IndexOf(World) = %d, want %d", got, want)
	}
	if got := s.IndexOf("zzz"); got != NotFound {
		t.Fatalf("IndexOf(zzz) = %d, want %d", got, NotFound)
	}
	combined := s.Append(NewString("!"))
	if got, want := combined.String(), "Hello, World!"; got != want {
		t.Fatalf("Append = %q, want %q", got, want)
	}
}

func TestStringIsolatedCopy(t *testing.T) {
	s := NewString("abc")
	c := s.IsolatedCopy()
	if !s.Equals(c) {
		t.Fatalf("IsolatedCopy should equal original")
	}
}

func TestStringEmptyAndNull(t *testing.T) {
	var s String
	if !s.IsEmpty() {
		t.Fatalf("zero String should be empty")
	}
	if !s.IsNull() {
		t.Fatalf("zero String should be null")
	}
}
