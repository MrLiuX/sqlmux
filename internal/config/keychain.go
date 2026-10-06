package config

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// hostOS picks the keychain tool; the tests set it.
var hostOS = runtime.GOOS

// StorePassword puts pw into the system keychain as connection name's and
// returns the password_cmd that reads it back (§13): macOS's security, or
// secret-tool elsewhere. stored is false, nothing done, when the system has
// no such tool; err is the tool's own complaint (a locked keychain).
// ponytail: security takes the password as an argument, so it shows in the
// process list for a moment; it has no stdin form, only a tty prompt
func StorePassword(name, pw string) (cmd string, stored bool, err error) {
	service := "sqlmux-" + name
	var store *exec.Cmd
	if hostOS == "darwin" {
		path, found := tool("security")
		if !found {
			return "", false, nil
		}
		store = exec.Command(path, "add-generic-password", "-U", "-a", "sqlmux", "-s", service, "-w", pw)
		cmd = "security find-generic-password -a sqlmux -s " + shellQuote(service) + " -w"
	} else {
		path, found := tool("secret-tool")
		if !found {
			return "", false, nil
		}
		store = exec.Command(path, "store", "--label", service, "sqlmux", name)
		store.Stdin = strings.NewReader(pw)
		cmd = "secret-tool lookup sqlmux " + shellQuote(name)
	}
	if out, runErr := store.CombinedOutput(); runErr != nil {
		why, _, _ := strings.Cut(strings.TrimSpace(string(out)), "\n")
		if why == "" {
			why = runErr.Error()
		}
		return "", false, fmt.Errorf("%s: %s", filepath.Base(store.Path), why)
	}
	return cmd, true, nil
}

// shellQuote is s as one word of an sh command line.
func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

// tool is where the command name is on PATH.
func tool(name string) (path string, found bool) {
	path, err := exec.LookPath(name)
	return path, err == nil
}
