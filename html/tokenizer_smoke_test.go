package html

import "testing"

func collectTokens(src string) []*Token {
	tk := NewTokenizer(src)
	var out []*Token
	for {
		tok := tk.NextToken()
		if tok == nil {
			break
		}
		out = append(out, tok)
		if tok.Type == TokenEOF {
			break
		}
	}
	return out
}

func TestSmokeText(t *testing.T) {
	toks := collectTokens("hello")
	if len(toks) != 2 {
		t.Fatalf("got %d tokens, want 2", len(toks))
	}
	if toks[0].Type != TokenCharacter || toks[0].Data != "hello" {
		t.Errorf("tok[0] = %v %q, want char hello", toks[0].Type, toks[0].Data)
	}
	if toks[1].Type != TokenEOF {
		t.Errorf("tok[1] = %v, want EOF", toks[1].Type)
	}
}

func TestSmokeStartTagWithAttrs(t *testing.T) {
	toks := collectTokens(`<div id="a" class='b' x=y>`)
	if len(toks) != 2 {
		t.Fatalf("got %d tokens: %+v", len(toks), toks)
	}
	tg := toks[0]
	if tg.Type != TokenStartTag || tg.Data != "div" {
		t.Errorf("tok = %v %q, want start div", tg.Type, tg.Data)
	}
	if len(tg.Attributes) != 3 {
		t.Fatalf("attrs = %d, want 3", len(tg.Attributes))
	}
	want := map[string]string{"id": "a", "class": "b", "x": "y"}
	for _, a := range tg.Attributes {
		if a.Value != want[a.Name] {
			t.Errorf("attr %s = %q, want %q", a.Name, a.Value, want[a.Name])
		}
	}
}

func TestSmokeEntities(t *testing.T) {
	toks := collectTokens("&amp;&lt;&gt;&nbsp;")
	if len(toks) < 2 {
		t.Fatalf("got %d tokens", len(toks))
	}
	want := "&<>\u00a0"
	if toks[0].Data != want {
		t.Errorf("data = %q, want %q", toks[0].Data, want)
	}
}

func TestSmokeComment(t *testing.T) {
	toks := collectTokens("<!-- hi -->")
	if len(toks) != 2 {
		t.Fatalf("got %d tokens", len(toks))
	}
	if toks[0].Type != TokenComment || toks[0].Data != " hi " {
		t.Errorf("tok = %v %q, want comment ' hi '", toks[0].Type, toks[0].Data)
	}
}

func TestSmokeDoctype(t *testing.T) {
	toks := collectTokens(`<!DOCTYPE html>`)
	if len(toks) != 2 {
		t.Fatalf("got %d tokens", len(toks))
	}
	if toks[0].Type != TokenDoctype || toks[0].Data != "html" {
		t.Errorf("tok = %v %q, want doctype html", toks[0].Type, toks[0].Data)
	}
}

func TestSmokeSelfClosing(t *testing.T) {
	toks := collectTokens(`<br/>`)
	if len(toks) != 2 {
		t.Fatalf("got %d tokens", len(toks))
	}
	if toks[0].Type != TokenStartTag || toks[0].Data != "br" || !toks[0].SelfClosing {
		t.Errorf("tok = %+v, want self-closing br", toks[0])
	}
}

// TestEntityInAttributeValue verifies that character references inside
// attribute values (e.g. `v-if="a && b"`) are appended to the attribute value
// and do not corrupt the start-tag token. Regression test for the bug where
// processEntity always appended to character data, turning the StartTag token
// into a Character token (attribute parsing collapsed).
func TestEntityInAttributeValue(t *testing.T) {
	toks := collectTokens(`<span v-if="a && b" class="x">{{ t }}</span>`)
	if len(toks) != 4 {
		t.Fatalf("got %d tokens, want 4 (start/char/end/eof): %+v", len(toks), toks)
	}
	st := toks[0]
	if st.Type != TokenStartTag || st.Data != "span" {
		t.Fatalf("tok[0] = %v %q, want start span", st.Type, st.Data)
	}
	if len(st.Attributes) != 2 {
		t.Fatalf("attrs = %d, want 2: %+v", len(st.Attributes), st.Attributes)
	}
	if v := st.Attributes[0]; v.Name != "v-if" || v.Value != "a && b" {
		t.Errorf("attr[0] = %s=%q, want v-if=\"a && b\"", v.Name, v.Value)
	}
	if v := st.Attributes[1]; v.Name != "class" || v.Value != "x" {
		t.Errorf("attr[1] = %s=%q, want class=\"x\"", v.Name, v.Value)
	}
	if toks[1].Type != TokenCharacter || toks[1].Data != "{{ t }}" {
		t.Errorf("tok[1] = %v %q, want char '{{ t }}'", toks[1].Type, toks[1].Data)
	}
	if toks[2].Type != TokenEndTag || toks[2].Data != "span" {
		t.Errorf("tok[2] = %v %q, want end span", toks[2].Type, toks[2].Data)
	}
	if toks[3].Type != TokenEOF {
		t.Errorf("tok[3] = %v, want EOF", toks[3].Type)
	}
}

// TestEntityDecodedInAttributeValue verifies that real entities inside
// attribute values decode correctly (&amp; -> &, &lt; -> <).
func TestEntityDecodedInAttributeValue(t *testing.T) {
	toks := collectTokens(`<a title="x&amp;y&lt;z">t</a>`)
	if len(toks) != 4 {
		t.Fatalf("got %d tokens: %+v", len(toks), toks)
	}
	st := toks[0]
	if st.Type != TokenStartTag || st.Data != "a" {
		t.Fatalf("tok[0] = %v %q, want start a", st.Type, st.Data)
	}
	if len(st.Attributes) != 1 {
		t.Fatalf("attrs = %d, want 1: %+v", len(st.Attributes), st.Attributes)
	}
	if v := st.Attributes[0]; v.Name != "title" || v.Value != "x&y<z" {
		t.Errorf("attr = %s=%q, want title=\"x&y<z\"", v.Name, v.Value)
	}
	if toks[1].Data != "t" {
		t.Errorf("tok[1] = %q, want t", toks[1].Data)
	}
	if toks[3].Type != TokenEOF {
		t.Errorf("tok[3] = %v, want EOF", toks[3].Type)
	}
}

// TestEntityInDataStillCharacter verifies entities outside attributes still
// decode into character data.
func TestEntityInDataStillCharacter(t *testing.T) {
	toks := collectTokens("a &amp; b")
	if len(toks) != 2 {
		t.Fatalf("got %d tokens", len(toks))
	}
	if toks[0].Type != TokenCharacter || toks[0].Data != "a & b" {
		t.Errorf("tok[0] = %v %q, want char 'a & b'", toks[0].Type, toks[0].Data)
	}
}

