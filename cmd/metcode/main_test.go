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
		{[]string{"--host", "http://192.168.1.50:11434"}, false},
	}
	for _, c := range cases {
		if got := wantsVersion(c.args); got != c.want {
			t.Errorf("wantsVersion(%v) = %v, want %v", c.args, got, c.want)
		}
	}
}

func TestParseHostFlag(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want string
	}{
		{name: "no args", args: nil, want: ""},
		{name: "separate value", args: []string{"--host", "http://192.168.1.50:11434"}, want: "http://192.168.1.50:11434"},
		{name: "equals value", args: []string{"--host=http://192.168.1.50:11434"}, want: "http://192.168.1.50:11434"},
		{name: "single dash", args: []string{"-host", "http://192.168.1.50:11434"}, want: "http://192.168.1.50:11434"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := parseHostFlag(c.args)
			if err != nil {
				t.Fatalf("parseHostFlag(%v) returned error: %v", c.args, err)
			}
			if got != c.want {
				t.Errorf("parseHostFlag(%v) = %q, want %q", c.args, got, c.want)
			}
		})
	}
}

func TestResolveHost(t *testing.T) {
	cases := []struct {
		name     string
		flagHost string
		envHost  string
		want     string
	}{
		{name: "flag wins over env", flagHost: "http://flag:11434", envHost: "http://env:11434", want: "http://flag:11434"},
		{name: "env without flag", envHost: "http://env:11434", want: "http://env:11434"},
		{name: "empty flag falls through to env", flagHost: "", envHost: "http://env:11434", want: "http://env:11434"},
		{name: "default when neither is set", want: defaultHost},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := resolveHost(c.flagHost, c.envHost); got != c.want {
				t.Errorf("resolveHost(%q, %q) = %q, want %q", c.flagHost, c.envHost, got, c.want)
			}
		})
	}
}

func TestPrintVersion(t *testing.T) {
	var buf bytes.Buffer
	printVersion(&buf)
	if got, want := buf.String(), resolveVersion()+"\n"; got != want {
		t.Errorf("printVersion wrote %q, want %q", got, want)
	}
}

func TestVersionDefaultsToDev(t *testing.T) {
	if version != "dev" {
		t.Errorf("version = %q, want %q for a plain build", version, "dev")
	}
}
