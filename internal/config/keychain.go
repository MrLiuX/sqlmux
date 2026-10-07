package config

import (
	"cmp"
	"errors"
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
	if hostOS == "darwin" {
		tool, args, stdin = "security", []string{"add-generic-password", "-U", "-a", "sqlmux", "-s", service, "-w", pw}, nil
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
	return KeychainCmd(name, pw), true, nil
}

// KeychainCmd is the password_cmd StorePassword returns for name and pw:
// what the file would say, known before anything is stored (F8.16).
func KeychainCmd(name, pw string) string {
	if hostOS != "darwin" {
		return "secret-tool lookup sqlmux " + shellQuote(name)
	}
	cmd := "security find-generic-password -a sqlmux -s " + shellQuote("sqlmux-"+name) + " -w"
	if !printable(pw) { // security -w then writes the whole password in hex (keychain_find.c)
		cmd += " | xxd -r -p"
	}
	return cmd
}

// keychainTool is a keychain tool and the password_cmd of its that
// StorePassword writes.
type keychainTool struct {
	cmd      *regexp.Regexp
	tool     string
	clear    func(item string) []string
	notFound int // the exit code of the tool for an item that is not there
}

// keychainTools are the two password_cmd StorePassword writes, the service
// (sqlmux- and the name) or the name quoted for sh in the one group, so an
// item of the user's own, even under -a sqlmux, does not match.
var keychainTools = []keychainTool{
	{regexp.MustCompile(`^security find-generic-password -a sqlmux -s '(sqlmux-(?:[^']|'\\'')*)' -w( \| xxd -r -p)?$`), "security",
		func(service string) []string {
			return []string{"delete-generic-password", "-a", "sqlmux", "-s", service}
		}, 44},
	{regexp.MustCompile(`^secret-tool lookup sqlmux '((?:[^']|'\\'')*)'$`), "secret-tool",
		func(name string) []string { return []string{"clear", "sqlmux", name} }, -1},
}

// keychainItem is the item passwordCmd reads, when that is a command
// StorePassword wrote: the tool, and what it calls the item by.
func keychainItem(passwordCmd string) (k *keychainTool, item string) {
	for i := range keychainTools {
		if m := keychainTools[i].cmd.FindStringSubmatch(passwordCmd); m != nil {
			return &keychainTools[i], strings.ReplaceAll(m[1], `'\''`, "'")
		}
	}
	return nil, ""
}

// OwnPassword is whether passwordCmd is a command StorePassword wrote.
func OwnPassword(passwordCmd string) bool {
	k, _ := keychainItem(passwordCmd)
	return k != nil
}

// SamePassword is whether two password_cmd read one item, both being
// commands StorePassword wrote: not the same text, a password that is not
// printable making the command longer (F8.16).
func SamePassword(a, b string) bool {
	ka, ia := keychainItem(a)
	kb, ib := keychainItem(b)
	return ka != nil && ka == kb && strings.EqualFold(ia, ib) // a name is one whatever its case (F8.16)
}

// DeletePassword takes out of the keychain the item passwordCmd reads, when
// that is the command StorePassword wrote; a command of the user's own is not
// touched, nor a keychain without its tool (F8.16). An item that was not
// there is not an error; err is the tool's complaint.
func DeletePassword(passwordCmd string) error {
	k, item := keychainItem(passwordCmd)
	if k == nil {
		return nil
	}
	tool := k.tool
	path, lookErr := exec.LookPath(tool)
	if lookErr != nil {
		return nil //nolint:nilerr // no such tool is no keychain
	}
	out, runErr := exec.Command(path, k.clear(item)...).CombinedOutput()
	var exit *exec.ExitError
	if errors.As(runErr, &exit) && exit.ExitCode() == k.notFound {
		return nil
	}
	if runErr != nil {
		why, _, _ := strings.Cut(strings.TrimSpace(string(out)), "\n")
		return fmt.Errorf("%s: %s", tool, cmp.Or(why, runErr.Error()))
	}
	return nil
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
