package tools

import (
	"os"
	"strings"
)

// readConfirmAnswer reads one line from stdin and reports whether it is a yes.
// Only y, yes and 1 count as yes, after trimming surrounding whitespace and
// ignoring case. Every other line is a no. The whole line is consumed, so any
// text after the answer is discarded and never read by a later prompt. The
// byte-at-a-time read stops at the newline without buffering past it.
func readConfirmAnswer() bool {
	var line []byte
	buf := make([]byte, 1)
	for {
		n, err := os.Stdin.Read(buf)
		if n > 0 {
			if buf[0] == '\n' {
				break
			}
			line = append(line, buf[0])
		}
		if err != nil {
			break
		}
	}
	switch strings.ToLower(strings.TrimSpace(string(line))) {
	case "y", "yes", "1":
		return true
	}
	return false
}

// this is used to help run the bench marks mostly
// main metcod would never replace it but bench mark does
var Approve = func(kind string) bool { return readConfirmAnswer() }
