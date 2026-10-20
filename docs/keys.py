#!/usr/bin/env python3
"""Writes docs/keys.md from `sqlmux keys --format toml` (the defaults: an empty config dir).

    python3 docs/keys.py [path/to/sqlmux] > docs/keys.md
"""
import json, re, subprocess, sys, tempfile

binary = sys.argv[1] if len(sys.argv) > 1 else "sqlmux"
with tempfile.TemporaryDirectory() as cfg:
    out = subprocess.run([binary, "keys", "--format", "toml"], env={"XDG_CONFIG_HOME": cfg, "PATH": "/usr/bin:/bin"},
                         check=True, capture_output=True, text=True).stdout
version = subprocess.run([binary, "--version"], check=True, capture_output=True, text=True).stdout.split()[1]

line = re.compile(r'^# ("(?:[^"\\]|\\.)*") = ("(?:[^"\\]|\\.)*")(?:  # (.*))?$')
scopes, about, leader = [], "", ""
for s in out.splitlines():
    if m := re.match(r"^\[keys\.(\w+)\]$", s):
        scopes.append((m[1], about, {}))
    elif s.startswith("# leader = "):
        leader = json.loads(s.split(" = ", 1)[1])
    elif (m := line.match(s)) and scopes:
        key, action, title = json.loads(m[1]), json.loads(m[2]), m[3] or ""
        keys = scopes[-1][2].setdefault(action, (title, []))[1]
        if key:
            keys.append(key)
    elif s.startswith("# ["):  # the [map.*] example after the keys
        break
    elif s.startswith("# "):
        about = s[2:]

def code(k):
    return "`" + k.replace("|", "\\|") + "`"

print(f"""# Keys

Every default key of sqlmux {version}, generated from `sqlmux keys --format toml` by `docs/keys.py`; do not edit by hand.
The leader is `{leader}`. Keys use vim notation (`<C-p>` is Ctrl+P, `<CR>` Enter, `<lt>` the `<` key, `<Leader>c` the leader then `c`).
An action with no key runs from the command palette (`<C-p>`, then `>`), or bind it yourself in `keymaps.toml`:
see [Configuration](config.md#keymapstoml).
""")
for name, desc, actions in reversed(scopes):  # the panes first, the overlays after
    print(f"## {name}\n\n{desc}\n\n| Keys | Action | What it does |\n|---|---|---|")
    for action, (title, keys) in actions.items():
        print(f"| {', '.join(map(code, keys)) or '—'} | `{action}` | {title} |")
    print()
