package config

import (
	"cmp"
	"fmt"
	"os/exec"
	"regexp"
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

// keychainCmds are the two password_cmd StorePassword writes, the service
// or the name quoted for sh in the one group.
var keychainCmds = []struct {
	cmd    *regexp.Regexp
	tool   string
	delete func(name string) []string
}{
	{regexp.MustCompile(`^security find-generic-password -a sqlmux -s '((?:[^']|'\\'')*)' -w( \| xxd -r -p)?$`), "security",
		func(service string) []string {
			return []string{"delete-generic-password", "-a", "sqlmux", "-s", service}
		}},
	{regexp.MustCompile(`^secret-tool lookup sqlmux '((?:[^']|'\\'')*)'$`), "secret-tool",
		func(name string) []string { return []string{"clear", "sqlmux", name} }},
}

// DeletePassword takes out of the keychain the item passwordCmd reads, when
// that is the command StorePassword wrote; a command of the user's own is not
// touched, nor a keychain without its tool (F8.16). stored says an item was
// asked to go; err is the tool's complaint.
func DeletePassword(passwordCmd string) (stored bool, err error) {
	for _, k := range keychainCmds {
		m := k.cmd.FindStringSubmatch(passwordCmd)
		if m == nil {
			continue
		}
		path, lookErr := exec.LookPath(k.tool)
		if lookErr != nil {
			return false, nil //nolint:nilerr // no such tool is no keychain
		}
		if out, runErr := exec.Command(path, k.delete(strings.ReplaceAll(m[1], `'\''`, "'"))...).CombinedOutput(); runErr != nil {
			why, _, _ := strings.Cut(strings.TrimSpace(string(out)), "\n")
			return false, fmt.Errorf("%s: %s", k.tool, cmp.Or(why, runErr.Error()))
		}
		return true, nil
	}
	return false, nil
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
