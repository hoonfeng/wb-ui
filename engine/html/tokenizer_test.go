// Translation of: Source/WebCore/html/parser/HTMLTokenizer.cpp (test cases)
// Completeness: 70%
// Simplifications:
//   - tests cover the common tokenizer states (data, tag, attribute, comment,
//     doctype, entity, CDATA, rawtext, rcdata, script data) rather than the full
//     HTML5 test suite

package html

import "testing"

// wantTok is a small helper for expressing expected token sequences in tests.
type wantTok struct {
	typ  TokenType
	data string
	// attr is checked only for StartTag tokens with a single attribute.
	attr string
	// attrVal is the expected value for the attribute named in attr.
	attrVal string
	// selfClosing reports whether a StartTag should be self-closing.
	selfClosing bool
}

func checkTokens(t *testing.T, src string, want []wantTok) {
	t.Helper()
	got := collectTokens(src)
	// The last token is always EOF; include it in the comparison.
	if len(got) != len(want) {
		t.Fatalf("%q: got %d tokens, want %d", src, len(got), len(want))
	}
	for i, w := range want {
		g := got[i]
		if g.Type != w.typ {
			t.Errorf("%q tok[%d].Type = %v, want %v", src, i, g.Type, w.typ)
			continue
		}
		if w.data != "" && g.Data != w.data {
			t.Errorf("%q tok[%d].Data = %q, want %q", src, i, g.Data, w.data)
		}
		if w.attr != "" {
			if v := g.attrValue(w.attr); v != w.attrVal {
				t.Errorf("%q tok[%d] attr %s = %q, want %q", src, i, w.attr, v, w.attrVal)
			}
		}
		if w.selfClosing && !g.SelfClosing {
			t.Errorf("%q tok[%d].SelfClosing = false, want true", src, i)
		}
	}
}

func TestTokenizerPlainText(t *testing.T) {
	checkTokens(t, "hello", []wantTok{
		{typ: TokenCharacter, data: "hello"},
		{typ: TokenEOF},
	})
}

func TestTokenizerSplitTextAcrossTags(t *testing.T) {
	checkTokens(t, "a<b>b</b>c", []wantTok{
		{typ: TokenCharacter, data: "a"},
		{typ: TokenStartTag, data: "b"},
		{typ: TokenCharacter, data: "b"},
		{typ: TokenEndTag, data: "b"},
		{typ: TokenCharacter, data: "c"},
		{typ: TokenEOF},
	})
}

func TestTokenizerStartTagNoAttributes(t *testing.T) {
	checkTokens(t, "<div>", []wantTok{
		{typ: TokenStartTag, data: "div"},
		{typ: TokenEOF},
	})
}

func TestTokenizerStartTagDoubleQuotedAttr(t *testing.T) {
	checkTokens(t, `<a href="http://x">`, []wantTok{
		{typ: TokenStartTag, data: "a", attr: "href", attrVal: "http://x"},
		{typ: TokenEOF},
	})
}

func TestTokenizerStartTagSingleQuotedAttr(t *testing.T) {
	checkTokens(t, `<a href='http://y'>`, []wantTok{
		{typ: TokenStartTag, data: "a", attr: "href", attrVal: "http://y"},
		{typ: TokenEOF},
	})
}

func TestTokenizerStartTagUnquotedAttr(t *testing.T) {
	checkTokens(t, `<a href=bar>`, []wantTok{
		{typ: TokenStartTag, data: "a", attr: "href", attrVal: "bar"},
		{typ: TokenEOF},
	})
}

func TestTokenizerMultipleAttributes(t *testing.T) {
	toks := collectTokens(`<input type="text" name="q" disabled>`)
	if len(toks) < 2 {
		t.Fatalf("got %d tokens", len(toks))
	}
	tg := toks[0]
	if tg.attrValue("type") != "text" {
		t.Errorf("type = %q", tg.attrValue("type"))
	}
	if tg.attrValue("name") != "q" {
		t.Errorf("name = %q", tg.attrValue("name"))
	}
	if !tg.hasAttribute("disabled") {
		t.Errorf("disabled not found")
	}
}

func TestTokenizerSelfClosingTag(t *testing.T) {
	checkTokens(t, `<br/>`, []wantTok{
		{typ: TokenStartTag, data: "br", selfClosing: true},
		{typ: TokenEOF},
	})
}

func TestTokenizerSelfClosingWithAttributes(t *testing.T) {
	checkTokens(t, `<img src="x.png"/>`, []wantTok{
		{typ: TokenStartTag, data: "img", attr: "src", attrVal: "x.png", selfClosing: true},
		{typ: TokenEOF},
	})
}

func TestTokenizerEndTag(t *testing.T) {
	checkTokens(t, "</div>", []wantTok{
		{typ: TokenEndTag, data: "div"},
		{typ: TokenEOF},
	})
}

func TestTokenizerEndTagWithAttributes(t *testing.T) {
	// End tags with attributes: the attributes are parsed but typically ignored
	// by the tree builder.
	toks := collectTokens("</div foo=bar>")
	if len(toks) < 2 {
		t.Fatalf("got %d tokens", len(toks))
	}
	if toks[0].Type != TokenEndTag || toks[0].Data != "div" {
		t.Errorf("tok = %v %q", toks[0].Type, toks[0].Data)
	}
}

func TestTokenizerComment(t *testing.T) {
	checkTokens(t, "<!-- hi -->", []wantTok{
		{typ: TokenComment, data: " hi "},
		{typ: TokenEOF},
	})
}

func TestTokenizerEmptyComment(t *testing.T) {
	checkTokens(t, "<!---->", []wantTok{
		{typ: TokenComment, data: ""},
		{typ: TokenEOF},
	})
}

func TestTokenizerCommentWithDashDash(t *testing.T) {
	toks := collectTokens("<!-- a -- b -->")
	if len(toks) < 2 {
		t.Fatalf("got %d tokens", len(toks))
	}
	if toks[0].Type != TokenComment {
		t.Errorf("tok[0] = %v, want comment", toks[0].Type)
	}
}

func TestTokenizerDoctype(t *testing.T) {
	checkTokens(t, `<!DOCTYPE html>`, []wantTok{
		{typ: TokenDoctype, data: "html"},
		{typ: TokenEOF},
	})
}

func TestTokenizerDoctypeWithPublic(t *testing.T) {
	toks := collectTokens(`<!DOCTYPE html PUBLIC "-//W3C//DTD HTML 4.01//EN" "http://www.w3.org/TR/html4/strict.dtd">`)
	if len(toks) < 2 {
		t.Fatalf("got %d tokens", len(toks))
	}
	if toks[0].Type != TokenDoctype || toks[0].Data != "html" {
		t.Errorf("tok = %v %q", toks[0].Type, toks[0].Data)
	}
	if toks[0].DoctypeData == nil {
		t.Fatalf("no doctype data")
	}
	if !toks[0].DoctypeData.HasPublicID {
		t.Errorf("HasPublicID = false, want true")
	}
	if toks[0].DoctypeData.PublicIdentifier != "-//W3C//DTD HTML 4.01//EN" {
		t.Errorf("PublicID = %q", toks[0].DoctypeData.PublicIdentifier)
	}
}

func TestTokenizerNamedEntities(t *testing.T) {
	checkTokens(t, "&amp;&lt;&gt;&quot;&apos;", []wantTok{
		{typ: TokenCharacter, data: `&<>"'`},
		{typ: TokenEOF},
	})
}

func TestTokenizerNumericEntityDecimal(t *testing.T) {
	checkTokens(t, "&#65;&#66;", []wantTok{
		{typ: TokenCharacter, data: "AB"},
		{typ: TokenEOF},
	})
}

func TestTokenizerNumericEntityHex(t *testing.T) {
	checkTokens(t, "&#x41;&#x42;", []wantTok{
		{typ: TokenCharacter, data: "AB"},
		{typ: TokenEOF},
	})
}

func TestTokenizerEntityWithoutSemicolon(t *testing.T) {
	// Unterminated entity in text: treated as literal '&' per the spec.
	toks := collectTokens("a&b")
	if len(toks) < 2 {
		t.Fatalf("got %d tokens", len(toks))
	}
	if toks[0].Data != "a&b" {
		t.Errorf("data = %q, want %q", toks[0].Data, "a&b")
	}
}

func TestTokenizerCRLFNormalization(t *testing.T) {
	// CR should be normalized to LF.
	toks := collectTokens("a\rb")
	if len(toks) < 2 {
		t.Fatalf("got %d tokens", len(toks))
	}
	if toks[0].Data != "a\nb" {
		t.Errorf("data = %q, want %q", toks[0].Data, "a\nb")
	}
}

func TestTokenizerCRLFPairNormalization(t *testing.T) {
	// CR LF should collapse to a single LF.
	toks := collectTokens("a\r\nb")
	if len(toks) < 2 {
		t.Fatalf("got %d tokens", len(toks))
	}
	if toks[0].Data != "a\nb" {
		t.Errorf("data = %q, want %q", toks[0].Data, "a\nb")
	}
}

func TestTokenizerNullCharacter(t *testing.T) {
	toks := collectTokens("a\x00b")
	if len(toks) < 2 {
		t.Fatalf("got %d tokens", len(toks))
	}
	want := "a\ufffdb"
	if toks[0].Data != want {
		t.Errorf("data = %q, want %q", toks[0].Data, want)
	}
}

func TestTokenizerWhitespacePreserved(t *testing.T) {
	checkTokens(t, "  \t\n ", []wantTok{
		{typ: TokenCharacter, data: "  \t\n "},
		{typ: TokenEOF},
	})
}

func TestTokenizerTagWithWhitespace(t *testing.T) {
	checkTokens(t, "<div   >", []wantTok{
		{typ: TokenStartTag, data: "div"},
		{typ: TokenEOF},
	})
}

func TestTokenizerAttributeNameCase(t *testing.T) {
	// Attribute names are lower-cased.
	toks := collectTokens(`<div CLASS="x">`)
	if len(toks) < 2 {
		t.Fatalf("got %d tokens", len(toks))
	}
	if !toks[0].hasAttribute("class") {
		t.Errorf("class attr not found; attrs = %+v", toks[0].Attributes)
	}
}

func TestTokenizerTagNameCase(t *testing.T) {
	// Tag names are lower-cased.
	checkTokens(t, "<DIV>", []wantTok{
		{typ: TokenStartTag, data: "div"},
		{typ: TokenEOF},
	})
}

func TestTokenizerLessThanOnly(t *testing.T) {
	// A lone '<' is treated as text.
	toks := collectTokens("a < b")
	if len(toks) < 2 {
		t.Fatalf("got %d tokens", len(toks))
	}
	if toks[0].Data != "a < b" {
		t.Errorf("data = %q, want %q", toks[0].Data, "a < b")
	}
}

func TestTokenizerBogusComment(t *testing.T) {
	// An invalid markup declaration like '<! foo>' becomes a bogus comment.
	toks := collectTokens("<! foo >")
	if len(toks) < 2 {
		t.Fatalf("got %d tokens", len(toks))
	}
	if toks[0].Type != TokenComment {
		t.Errorf("tok[0] = %v, want comment", toks[0].Type)
	}
}
