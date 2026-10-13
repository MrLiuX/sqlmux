#!/usr/bin/env bash
# F15.4 提示自动换一条：每 5 分钟（一个 tea.Tick），不重复（specs/design/07-render.md §7.8「特性提示」）。要等 5 分多钟，所以单独一个脚本。
. "$(dirname "$0")/lib.sh"
e2e_build || exit 1

D=$(mktemp -d "${TMPDIR:-/tmp}/sqlmux-e2e-cfg.XXXXXX"); trap 'e2e_stop; rm -rf "$D"' EXIT
mkdir -p "$D/on"; printf '[ui]\n' >"$D/on/config.toml"
tip() { bar | python3 -c "
import re, sys
m = re.search(r'Tip: (.*?)(?:  |\$)', sys.stdin.read())
print(m.group(1) if m else '')"; }

start -C "$D/on"
T0=$(tip)
check "启动：状态栏有提示" eval '[[ -n $T0 ]]'
sleep 150
check "2 分半之后还是同一条（没到 5 分钟不换）" eval '[[ $(tip) == "$T0" ]]'
sleep 170
T1=$(tip)
check "5 分钟之后自己换了一条（不是原来那条，不是空的）" eval '[[ -n $T1 && $T1 != "$T0" ]]' || { echo "  $T0 -> $T1"; }
e2e_done
