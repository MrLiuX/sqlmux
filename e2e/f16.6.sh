#!/usr/bin/env bash
# F16.6 表格和树的方向键（specs/m16-redis/task.md F16.6；tech-design §6.9）
# vim 模式（默认）下 ←↓↑→ 也动表格光标、展开 / 折叠树节点（nvim 的 NORMAL 本来就认方向键）；hjkl 仍然在；PgDn / Delete 仍是非 vim 层独有。
. "$(dirname "$0")/lib.sh"
e2e_build || exit 1
. "$(dirname "$0")/palette.sh"

D=$(mktemp -d "${TMPDIR:-/tmp}/sqlmux-e2e-cfg.XXXXXX"); trap 'e2e_stop; rm -rf "$D"' EXIT
e2e_own_db || { echo "e2e: could not create the private database" >&2; exit 1; }
mkdir -p "$D/c"
(umask 077; printf '[[connection]]\nname = "doraemon"\nengine = "postgres"\ndsn = "%s"\n' "$E2E_DB" >"$D/c/connections.toml")
SELECT=#364a82
tsel() { local y; for y in $(seq 3 43); do [[ $(e2e_style 4 "$y") == *bg=$SELECT* ]] && { e2e_text 1 32 "$y"; return; }; done; }

start -C "$D/c"; open_table t_order; wait_for 8 settled
check "vim 模式（默认配置，没有 vim = false）：状态栏有 NORMAL，光标在 1,1" eval '[[ $(bar) == *NORMAL* && $(pos) == 1,1 ]]'
key Down; key Down
check "表格 ↓ ↓：光标 3,1" pos_is 3,1
key Right; key Right; key Right
check "→ → →：光标 3,4" pos_is 3,4
key Up
check "↑：光标 2,4" pos_is 2,4
key Left
check "←：光标 2,3" pos_is 2,3
key j; key l
check "hjkl 还在：j l 之后 3,4" pos_is 3,4
key Left; key Left; key Left; key Left; key Left
check "← 到最左列停住（不绕回）：x,1" eval '[[ $(pos) == *,1 ]]'
key PageDown; sleep 0.8
check "PgDn 是非 vim 层的键：vim 模式下不翻页（仍是 PAGE 1/）" eval '[[ $(qb) == *"PAGE 1/"* ]]'
key Delete; sleep 0.3
pend() { qb | sed -E 's/.*PAGE [^ ]+ *(COLS [0-9]+\/[0-9]+)?//; s/[^0-9]+/ /g; s/^ +//; s/ +$//'; }
check "Delete 也是：vim 模式下不标记删除（保存按钮上没有修改计数）" eval '[[ -z $(pend) ]]' || qb
key Enter; sleep 0.3
check "↵ 仍是编辑（状态栏 INSERT 之类，不是翻页）" eval '[[ $(bar) != *NORMAL* ]]' || bar
key Escape

# ---- 树
key C-h; sleep 0.3
check "C-h：焦点到树" eval '[[ $(focused) == 0 ]]'
tree_to() { local i; key g g; for ((i = 0; i < 60; i++)); do [[ $(tsel) == *"$1"* ]] && return 0; key Down; done; return 1; }
tree_to "Views" || tree_to "Tables"
V=$(tsel | grep -oE "(Views|Tables) \([0-9]+\)" | head -1)
check "树 ↓ 走到「${V}」：光标行就是它（↓ 动了光标）" eval '[[ -n "$V" && $(tsel) == *"$V"* ]]' || tsel
check "（Views 开始是折叠的）" eval '[[ $(tsel) == *"▸"*"$V"* ]]' || tsel
key Right; sleep 0.3
check "→ 展开：▸ 变 ▾" eval '[[ $(tsel) == *"▾"*"$V"* ]]' || tsel
key Left; sleep 0.3
check "← 折叠：▾ 变 ▸" eval '[[ $(tsel) == *"▸"*"$V"* ]]' || tsel
key Up; sleep 0.3
check "↑ 往上一行" eval '[[ $(tsel) != *"$V"* ]]' || tsel
key Down; sleep 0.3
check "↓ 回到「${V}」" eval '[[ $(tsel) == *"$V"* ]]' || tsel
key l; sleep 0.3
check "l 还在：展开" eval '[[ $(tsel) == *"▾"*"$V"* ]]' || tsel
key h; sleep 0.3
check "h 还在：折叠" eval '[[ $(tsel) == *"▸"*"$V"* ]]' || tsel

# ---- sqlmux keys 列出方向键（vim 模式，默认配置）
kk() { XDG_CONFIG_HOME=$D/$1 XDG_STATE_HOME=$D/st XDG_DATA_HOME=$D/st "$E2E_BIN" keys; }
mkdir -p "$D/k-vim/sqlmux"
K=$(kk k-vim)
check "sqlmux keys（vim）：grid 的 ↓ ↑ ← → 和 tree 的 ↓ ↑ ← → 都有" eval 'for w in "grid.down" "grid.up" "grid.left" "grid.right" "tree.down" "tree.up" "tree.collapse" "tree.expand"; do grep -E "\| (grid|tree) \| .(←|↑|↓|→|<Left>|<Up>|<Down>|<Right>). \| $w( |\|)" <<<"$K" >/dev/null || { echo "  missing $w"; false; }; done'
check "sqlmux keys（vim）：grid.page.next 和 grid.row.delete 没有键（PgDn / Delete 在非 vim 层）" eval '! grep -E "grid.page.next|grid.row.delete" <<<"$K" | grep -qE "PgDn|PageDown|Del"'

e2e_done
