package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"time"
	"unicode/utf8"
)

// historySQLMax is the longest statement history.jsonl keeps (F14.2).
const historySQLMax = 64 << 10

// History is one statement a query ran (§11), a line of history.jsonl: what
// ran where and when, and what came of it: the engine's command tag and the
// time it took, or the error's first line.
type History struct {
	At        time.Time `json:"at"`
	Conn      string    `json:"conn"`
	Schema    string    `json:"schema,omitempty"`
	SQL       string    `json:"sql"`
	Truncated bool      `json:"truncated,omitempty"` // SQL is its first historySQLMax bytes
	Tag       string    `json:"tag,omitempty"`       // SELECT 3, UPDATE 1
	TookMS    int64     `json:"took_ms"`
	Error     string    `json:"error,omitempty"`
}

// historyPath is the file the history is in, in the data directory (§14).
func historyPath() string { return filepath.Join(DataDir(), "history.jsonl") }

// AppendHistory adds h to the file in one write to a file opened for append,
// which the system keeps whole against another process doing the same: no lock.
func AppendHistory(h History) error {
	if len(h.SQL) > historySQLMax {
		n := historySQLMax
		for !utf8.RuneStart(h.SQL[n]) {
			n--
		}
		h.SQL, h.Truncated = h.SQL[:n], true
	}
	line, err := json.Marshal(h)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(historyPath()), 0o700); err != nil {
		return err
	}
	f, err := os.OpenFile(historyPath(), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	_, err = f.Write(append(line, '\n'))
	return errors.Join(err, f.Close())
}

// historyLines is the lines of the file, none when it is not there yet.
func historyLines() ([][]byte, error) {
	data, err := os.ReadFile(historyPath())
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	return bytes.Split(bytes.TrimRight(data, "\n"), []byte("\n")), err
}

// LoadHistory is the history, the oldest first; a line that is no entry is
// left out.
func LoadHistory() ([]History, error) {
	lines, err := historyLines()
	var hs []History
	for _, l := range lines {
		var h History
		if json.Unmarshal(l, &h) == nil {
			hs = append(hs, h)
		}
	}
	return hs, err
}

// TrimHistory keeps the last keep lines of the file, the file written again
// through one renamed into place.
// ponytail: a line another process appends meanwhile may be lost; they are
// trimmed at the start only, and a statement of a second is not worth a lock
func TrimHistory(keep int) error {
	lines, err := historyLines()
	if err != nil || len(lines) <= keep {
		return err
	}
	return writeFile(historyPath(), append(bytes.Join(lines[len(lines)-keep:], []byte("\n")), '\n'))
}
