package config

import (
	"context"
	"runtime"
	"slices"
	"testing"
)

// ShellCommand's argv is sh -c on unix, %COMSPEC% /c on windows, cmd when
// the variable is not set (§14).
func TestShellCommand(t *testing.T) {
	t.Setenv("COMSPEC", `C:\windows\system32\cmd.exe`)
	line := "echo hi"
	got := ShellCommand(context.Background(), line).Args
	want := []string{"sh", "-c", line}
	if runtime.GOOS == "windows" {
		want = []string{`C:\windows\system32\cmd.exe`, "/c", line}
	}
	if !slices.Equal(got, want) {
		t.Errorf("argv %v, want %v", got, want)
	}
	if runtime.GOOS != "windows" {
		return
	}
	t.Setenv("COMSPEC", "")
	if got := ShellCommand(context.Background(), line).Args; !slices.Equal(got, []string{"cmd", "/c", line}) {
		t.Errorf("without COMSPEC: argv %v", got)
	}
}
