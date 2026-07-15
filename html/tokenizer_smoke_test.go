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

