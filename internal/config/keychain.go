package config

import (
	"fmt"
	"os/exec"
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
	tool, args, stdin := "secret-tool", []string{"store", "--label", service, "sqlmux", name}, strings.NewReader(pw)
	cmd = "secret-tool lookup sqlmux " + shellQuote(name)
	if hostOS == "darwin" {
		tool, args, stdin = "security", []string{"add-generic-password", "-U", "-a", "sqlmux", "-s", service, "-w", pw}, nil
		cmd = "security find-generic-password -a sqlmux -s " + shellQuote(service) + " -w"
		if !printable(pw) { // security -w then writes the whole password in hex (keychain_find.c)
			cmd += " | xxd -r -p"
		}
	}
	path, lookErr := exec.LookPath(tool)
	if lookErr != nil {
		return "", false, nil //nolint:nilerr // no such tool is no keychain, not a failure
	}
	store := exec.Command(path, args...)
	if stdin != nil {
		store.Stdin = stdin
	}
	if out, runErr := store.CombinedOutput(); runErr != nil {
		why, _, _ := strings.Cut(strings.TrimSpace(string(out)), "\n")
		if why == "" {
			why = runErr.Error()
		}
		return "", false, fmt.Errorf("%s: %s", tool, why)
	}
	return cmd, true, nil
}

// shellQuote is s as one word of an sh command line.
func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

// printable is whether every byte of s is one of 0x20–0x7e.
func printable(s string) bool {
	for _, b := range []byte(s) {
		if b < 0x20 || b > 0x7e {
			return false
		}
	}
	return true
}
