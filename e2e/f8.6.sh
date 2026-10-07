#!/usr/bin/env bash
# F8.6 表格里 Tab / S-Tab 左右移格（specs/m8-feedback/task.md F8.6；tech-design §6.8 默认键表）
# NORMAL 下 Tab = grid.right、S-Tab = grid.left，所有行都生效（新增行也是）；编辑中的 Tab 照旧提交并右移。自建库：要新增行、编辑。
. "$(dirname "$0")/lib.sh"
SOLO=1
e2e_build || exit 1

D=$(mktemp -d "${TMPDIR:-/tmp}/sqlmux-e2e-cfg.XXXXXX"); trap 'e2e_stop; rm -rf "$D"' EXIT
e2e_own_db || { echo "e2e: could not create the private database" >&2; exit 1; }
mkdir -p "$D/own"; printf '[[connection]]\nname = "doraemon"\nengine = "postgres"\ndsn = "%s"\n' "$E2E_DB" >"$D/own/connections.toml"; chmod 600 "$D/own/connections.toml"
psql_n() { psql "$E2E_DB" -At -c "$1"; }

start -C "$D/own"; open_table t_order; wait_for 8 settled
check "（准备）光标在第 1 行第 1 列" pos_is 1,1
key Tab
check "Tab：光标右移一格（普通行）" pos_is 1,2
key Tab; key BTab
check "Tab 再 S-Tab：回到第 2 列" pos_is 1,2
key BTab
check "S-Tab：回到第 1 列" pos_is 1,1
key BTab
check "第 1 列上 S-Tab：不动（同 h）" pos_is 1,1
key '$'; key Tab
check "最后一列上 Tab：不动（同 l，t_order 有 10 列）" pos_is 1,10
key 0

# ---- 新增行（黄色行）：NORMAL 下 Tab / S-Tab 都能左右移（状态栏在新增行上没有行列号，看终端光标的列）
# 当前格的底色 #3d59a1；返回新增行（数据区第 2 行）里它第一次出现的列
cx() { local x y; y=$(( $(grid_y) + 2 )); for x in $(seq 36 159); do style_has $x $y bg=#3d59a1 >/dev/null && { echo $x; return; }; done; }
key o; sleep 0.4
check "（准备）o 新增一行：行号栏是 +，仍在 NORMAL" eval 'mode_is NORMAL && e2e_plain | grep -qE "│ +\+ │ <default>"'
x0=$(cx); key Tab; sleep 0.2; x1=$(cx); key Tab; sleep 0.2; x2=$(cx)
check "新增行上 NORMAL 按 Tab：光标一格一格右移（x 单调增加）" eval '(( x1 > x0 && x2 > x1 )) || { echo "  $x0 $x1 $x2"; false; }'
key BTab; sleep 0.2; x3=$(cx)
check "S-Tab：左移一格，回到上一格" eval '(( x3 == x1 )) || { echo "  $x1 -> $x3"; false; }'
key BTab; sleep 0.2; x4=$(cx)
check "再 S-Tab：回到第一格" eval '(( x4 == x0 ))'

# ---- 编辑中的 Tab 照旧：提交并右移到下一格
key Enter; e2e_type "9999"; key Tab; sleep 0.4
check "新增行编辑中 Tab：提交这一格（id 是 9999），光标移到下一格" eval 'e2e_plain | grep -qE "│ +\+ │ +9999 │" && (( $(cx) > x0 ))'
key Escape; key Escape
e2e_done
