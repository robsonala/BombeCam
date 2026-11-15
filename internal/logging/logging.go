package logging

import (
	"fmt"
	"io"
	"os"
	"sync/atomic"
)

var verbose atomic.Bool

var (
	stdout io.Writer
	stderr io.Writer
)

func SetVerbose(enabled bool) {
	verbose.Store(enabled)
}

func Verbose() bool {
	return verbose.Load()
}

func Infof(format string, args ...any) {
	if Verbose() {
		out := stdout
		if out == nil {
			out = os.Stdout
		}
		_, _ = fmt.Fprintf(out, format, args...)
	}
}

func Infoln(args ...any) {
	if Verbose() {
		out := stdout
		if out == nil {
			out = os.Stdout
		}
		_, _ = fmt.Fprintln(out, args...)
	}
}

func Errorf(format string, args ...any) {
	out := stderr
	if out == nil {
		out = os.Stderr
	}
	_, _ = fmt.Fprintf(out, format, args...)
}

func ExternalLogLevel() string {
	if Verbose() {
		return "info"
	}
	return "error"
}
