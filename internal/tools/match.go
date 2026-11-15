package tools

import (
	"fmt"
	"strings"
)

// looksLikeToolCall reports whether s is name(...) where name is a real tool.
func looksLikeToolCall(s string) bool {
	s = strings.TrimSpace(s)
	open := strings.Index(s, "(")
	if open <= 0 || !strings.HasSuffix(s, ")") {
		return false
	}
	name := s[:open]
	for _, t := range All {
		if t.Function.Name == name {
			return true
		}
	}
	return false
}

// findLooseMatch finds old finds old in content compare line byu line ignore whitespace.
// It returns the byte range of the mmatched lines in the orginial content
func findLooseMatch(content, old string) (int, int, error) {
	want := strings.Split(strings.Trim(old, "\n"), "\n")

	for i := range want {
		want[i] = strings.TrimSpace(want[i])
	}

	if len(want) == 1 && want[0] == "" {
		return 0, 0, fmt.Errorf("old text is blank")
	}

	lines := strings.SplitAfter(content, "\n")
	offs := make([]int, len(lines)+1)
	for i, l := range lines {
		offs[i+1] = offs[i] + len(l)
	}

	var hits []int
	for i := 0; i+len(want) <= len(lines); i++ {
		match := true
		for j, w := range want {
			if strings.TrimSpace(lines[i+j]) != w {
				match = false
				break
			}
		}
		if match {
			hits = append(hits, i)
		}
	}

	switch len(hits) {
	case 0:
		return 0, 0, fmt.Errorf("no match when ignoring indentation")
	case 1:
		i := hits[0]
		return offs[i], offs[i+len(want)], nil
	default:
		return 0, 0, fmt.Errorf("matches %d places when ignoring indentation", len(hits))
	}
}

// converts leading 4 space groups in new_text to tabs when matched region of the file is indented with tabs
func reindentLike(newText, region string) string {
	if !strings.Contains(region, "\t") {
		return newText
	}
	lines := strings.Split(newText, "\n")
	for i, l := range lines {
		n := len(l) - len(strings.TrimLeft(l, " "))
		if n == 0 {
			continue
		}
		lines[i] = strings.Repeat("\t", n/4) + strings.Repeat(" ", n%4) + l[n:]
	}
	return strings.Join(lines, "\n")
}
