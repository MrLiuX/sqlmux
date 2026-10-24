package config

import (
	"context"
	"os"
	"os/exec"
	"runtime"
)

// ShellCommand is how a command line of the config runs (§14): sh -c on
// unix, %COMSPEC% /c on windows, cmd when the variable is not set.
func ShellCommand(ctx context.Context, line string) *exec.Cmd {
	if runtime.GOOS != "windows" {
		return exec.CommandContext(ctx, "sh", "-c", line)
	}
	shell := os.Getenv("COMSPEC")
	if shell == "" {
		shell = "cmd"
	}
	return exec.CommandContext(ctx, shell, "/c", line)
}
