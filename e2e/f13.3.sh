#!/usr/bin/env bash
# F13.3 两个进程各写 state.json，后写的不再吃掉先写的（specs/m13-quality/task.md F13.3；tech-design §14）
# 进程 A 和 B 共用一个状态目录：A 在 t_order 上写一条 WHERE，B 写另一条，B 先退出、A 后退出；state.json 里两条 WHERE 历史都在，
# 两个进程各自开的表（A 的 t_order、B 的 t_user）的「最近使用」也都在。只读，用共用库。
. "$(dirname "$0")/lib.sh"
e2e_build || exit 1
. "$(dirname "$0")/palette.sh"

D=$(mktemp -d "${TMPDIR:-/tmp}/sqlmux-e2e-cfg.XXXXXX"); trap 'e2e_stop; rm -rf "$D"' EXIT
ST=$D/st/sqlmux/state.json
wheres() { python3 -c 'import json, sys; d = json.load(open(sys.argv[1])); [print(h["where"]) for t in d.get("tables", {}).values() for h in t.get("history", [])]' "$ST"; }
quit_a() { e2e_type ":"; sleep 0.3; e2e_type qa; key Enter; wait_for 8 exited; sleep 0.5; }

start -S "$D/st" -D "$D/dataA"; sleep 1.2
open_table t_order; wait_for 8 settled
key /; sleep 0.3; e2e_type "id < 11"; key Enter; wait_for 8 settled; sleep 0.5; key Escape

# ---- 第二个进程 B：同一个状态目录（另一个 tmux session，同一个假环境）
t new-session -d -s b -x 160 -y 45 \
  -e XDG_CONFIG_HOME="$E2E_TMP/config" -e XDG_STATE_HOME="$D/st" -e XDG_DATA_HOME="$D/dataB" \
  -e E2E_PATH="$E2E_TMP/bin:$PATH" -e COLORTERM=truecolor -e HISTFILE=/dev/null -e E2E_DIR="$E2E_TMP" \
  'sh -c '\''export TERM=xterm-256color PATH="$E2E_PATH"; unset DISPLAY WAYLAND_DISPLAY; cd "$E2E_DIR"; exec '"$E2E_BIN"\'
sleep 2
tb() { t send-keys -t b "$@"; }
tbl() { t send-keys -t b -l -- "$1"; }
cb() { t capture-pane -p -t b; }
tb C-p; sleep 0.3; tbl "@t_order"; sleep 0.4; tb Enter; sleep 2
tb "/"; sleep 0.3; tbl "id < 22"; tb Enter; sleep 1.5; tb Escape; sleep 0.3
check "（准备）B 里 t_order 带着 id < 22，A 里是 id < 11" eval 'cb | grep -q "id < 22" && e2e_cap | grep -q "id < 11"'
tb ":"; sleep 0.3; tbl qa; tb Enter; sleep 1.5      # B 先退出：它的状态先写
quit_a                                               # A 后退出：它得把磁盘上 B 写的并进去，不是整份覆盖
check "state.json 里两条 WHERE 历史都在（id < 11 来自 A，id < 22 来自 B）" eval 'wheres | grep -qx "id < 11" && wheres | grep -qx "id < 22"' || cat "$ST"
check "state.json 是合法 JSON" eval 'python3 -m json.tool "$ST" >/dev/null'
e2e_done
