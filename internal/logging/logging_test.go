package logging

import (
	"bytes"
	"testing"
)

func TestInfoIsVerboseAndErrorsAreAlwaysWritten(t *testing.T) {
	oldVerbose, oldStdout, oldStderr := verbose.Load(), stdout, stderr
	t.Cleanup(func() {
		verbose.Store(oldVerbose)
		stdout, stderr = oldStdout, oldStderr
	})

	var out, errOut bytes.Buffer
	stdout, stderr = &out, &errOut

	SetVerbose(false)
	if got := ExternalLogLevel(); got != "error" {
		t.Fatalf("default external log level = %q, want %q", got, "error")
	}
	Infof("quiet")
	Infoln("also quiet")
	Errorf("visible error")
	if out.Len() != 0 {
		t.Fatalf("default logging wrote informational output: %q", out.String())
	}
	if got := errOut.String(); got != "visible error" {
		t.Fatalf("default logging error = %q, want %q", got, "visible error")
	}

	SetVerbose(true)
	if got := ExternalLogLevel(); got != "info" {
		t.Fatalf("verbose external log level = %q, want %q", got, "info")
	}
	Infof("details %d", 1)
	Infoln("more")
	if got, want := out.String(), "details 1more\n"; got != want {
		t.Fatalf("verbose output = %q, want %q", got, want)
	}
}
