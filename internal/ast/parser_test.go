package ast

import "testing"

func TestParseRejectsSyntaxErrors(t *testing.T) {
	if _, err := Parse([]byte("func broken( {"), LangGo); err == nil {
		t.Fatal("expected malformed source to be rejected")
	}
}
