// Package cli holds the few things every tb command line shares: at the moment, painting
// errors and FAIL verdicts red on a terminal.
//
// Color is applied only when stderr is a character device and NO_COLOR is unset, so
// anything captured through a pipe (tests use CombinedOutput, scripts grep the output)
// sees plain text. No string that a test matches ever carries an escape code.
package cli

import (
	"fmt"
	"os"
)

// Red wraps s in ANSI red when stderr is a terminal and NO_COLOR is not set; otherwise it
// returns s unchanged.
func Red(s string) string {
	if !colorStderr() {
		return s
	}
	return "\x1b[31m" + s + "\x1b[0m"
}

// Errorln prints the arguments to stderr the way fmt.Fprintln would, in red on a terminal.
func Errorln(a ...any) {
	fmt.Fprintln(os.Stderr, Red(fmt.Sprint(a...)))
}

// Errorf prints a formatted line to stderr, in red on a terminal.
func Errorf(format string, a ...any) {
	fmt.Fprintln(os.Stderr, Red(fmt.Sprintf(format, a...)))
}

func colorStderr() bool {
	if _, set := os.LookupEnv("NO_COLOR"); set {
		return false
	}
	fi, err := os.Stderr.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}
