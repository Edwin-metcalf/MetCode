package main

import (
	"bytes"
	"testing"
)

func TestWantsVersion(t *testing.T) {
	cases := []struct {
		args []string
		want bool
	}{
		{nil, false},
		{[]string{}, false},
		{[]string{"--version"}, true},
		{[]string{"-v"}, true},
		{[]string{"--help"}, false},
		{[]string{"x", "--version"}, true},
	}
	for _, c := range cases {
		if got := wantsVersion(c.args); got != c.want {
			t.Errorf("wantsVersion(%v) = %v, want %v", c.args, got, c.want)
		}
	}
}

func TestPrintVersion(t *testing.T) {
	var buf bytes.Buffer
	printVersion(&buf)
	if got, want := buf.String(), version+"\n"; got != want {
		t.Errorf("printVersion wrote %q, want %q", got, want)
	}
}

func TestVersionDefaultsToDev(t *testing.T) {
	if version != "dev" {
		t.Errorf("version = %q, want %q for a plain build", version, "dev")
	}
}
