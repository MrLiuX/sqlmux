#!/usr/bin/env bash
# F8.10 边框样式 border = single | rounded | double | bold | hidden（specs/m8-feedback/task.md F8.10；tech-design §7.1、§14）
# 各样式的画面有 golden（默认布局各一张）；这里在真实终端里看：pane 的四角和横竖线用对应的字符，浮层（命令面板、确认框）用同一套，
# hidden 不画边框但标题照画，写错的值启动报错（英文）。只读，用共用库。
. "$(dirname "$0")/lib.sh"
e2e_build || exit 1

D=$(mktemp -d "${TMPDIR:-/tmp}/sqlmux-e2e-cfg.XXXXXX"); trap 'e2e_stop; rm -rf "$D"' EXIT
cfg() { printf '%s' "$1" >|"$D/config.toml"; }
line() { e2e_plain | sed -n "$1p"; }

# style NAME TL TR BL BR H V：这个样式的四角、横线、竖线
check_style() {
  local name=$1 tl=$2 tr=$3 bl=$4 br=$5 h=$6 v=$7
  cfg "border = \"$name\""$'\n'
  SOLO= start -c "$D/config.toml"; sleep 0.3
  check "$name：第 1 行的 pane 上边框是 ${tl} … ${tr}，没有 ┌" eval 'l=$(line 1); [[ $l == "${tl}${h} ⓪"* && $l == *"${tr}" && $l != *"┌"* ]]'
  check "$name：第 44 行的下边框是 ${bl} … ${br}" eval 'l=$(line 44); [[ $l == "${bl}"* && $l == *"${br}" && $(grep -o "${bl}" <<<"$l" | wc -l | tr -d " ") == 3 ]]'
  check "$name：竖线是 ${v}（第 10 行）" eval 'l=$(line 10); [[ $l == "${v}"* && $l == *"${v}" ]]'
}
check_style rounded ╭ ╮ ╰ ╯ ─ │
check_style double  ╔ ╗ ╚ ╝ ═ ║
check_style bold    ┏ ┓ ┗ ┛ ━ ┃

# ---- 浮层用同一套字符（命令面板）
cfg $'border = "rounded"\n'
SOLO= start -c "$D/config.toml"; e2e_keys C-p; sleep 0.5
check "rounded：命令面板的边框也是圆角（╭─ 命令面板 … ╮、╰ … ╯）" eval 'p=$(e2e_plain); grep -q "╭─ 命令面板" <<<"$p" && grep -qE "╰─+╯" <<<"$p" && ! grep -q "┌─ 命令面板" <<<"$p"'
key Escape
cfg $'border = "double"\n'
SOLO= start -c "$D/config.toml"; e2e_keys C-p; sleep 0.5
check "double：命令面板是双线" eval 'p=$(e2e_plain); grep -q "╔═ 命令面板" <<<"$p" && ! grep -q "┌─ 命令面板" <<<"$p"'
key Escape

# ---- hidden：不画边框，标题照画
cfg $'border = "hidden"\n'
SOLO= start -c "$D/config.toml"; sleep 0.3
check "hidden：没有任何边框的角（┌ ┐ └ ┘ ╭ ╮ ╰ ╯ ╔ ╗ ╚ ╝ ┏ ┓ ┗ ┛），pane 之间也没有竖线边框（第 10 行里没有 │）" eval '! e2e_plain | grep -qE "[┌┐└┘╭╮╰╯╔╗╚╝┏┓┗┛]" && ! line 10 | grep -q "│"'
check "hidden：标题照画（⓪ ① ② 都在）" eval 'p=$(e2e_plain); grep -q "⓪" <<<"$p" && grep -q "①" <<<"$p" && grep -q "②" <<<"$p"'

# ---- 默认 / 显式 single：和以前一样；焦点只换颜色不换样式
cfg $'border = "single"\n'
SOLO= start -c "$D/config.toml"; sleep 0.3
check "single：┌─ ⓪ … ┐，└ … ┘" eval 'line 1 | grep -q "^┌─ ⓪" && line 44 | grep -q "^└"'
check "焦点 pane 和非焦点 pane 的边框字符相同（只是颜色不同）：① 聚焦时 ① 的左上角也是 ┌" eval '[[ $(line 1 | grep -o "┌" | wc -l | tr -d " ") == 3 ]]'

# ---- 写错的值
cfg $'border = "dotted"\n'
SOLO= start -c "$D/config.toml"; sleep 1
check "border = \"dotted\"：启动报错，带路径，退出码 1，说明可选值（英文）" eval 'screen_has "[e2e-exit 1]" && screen_has "config.toml" && screen_has "border" && screen_has "rounded"'
e2e_done
