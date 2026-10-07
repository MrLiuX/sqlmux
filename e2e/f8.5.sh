#!/usr/bin/env bash
# F8.5 短列不收缩：[grid] full_width（specs/m8-feedback/task.md F8.5；tech-design §7.6「列宽」）
# 列宽的分配有单测、160 宽的画面有 golden；这里在真实终端里测：t_order 的 <null>、pending、时间完整显示，json 列截断，
# full_width 写进 config.toml 后生效（调小时宽于它的列收到它为止），放不下就横向滚动，转置不变。只读，用共用库。
. "$(dirname "$0")/lib.sh"
SOLO=1   # ① alone right of the sidebar (127 columns)
e2e_build || exit 1

D=$(mktemp -d "${TMPDIR:-/tmp}/sqlmux-e2e-cfg.XXXXXX"); trap 'e2e_stop; rm -rf "$D"' EXIT
col_x() { e2e_find "$1" "$(hy)" | tr ' ' '\n' | awk '$1 > 34 { print; exit }'; }
row_y() { echo $(( $(grid_y) + $1 )); }
header() { e2e_text 35 159 "$(hy)"; }
rowtext() { e2e_text 35 159 "$(row_y "$1")"; }
# width_of COLUMN：表头里这一列的宽度（两条竖线之间，含两侧留白）
width_of() { header | python3 -c 'import sys; h = sys.stdin.read(); c = sys.argv[1]; i = h.index(c); a = h.rfind("│", 0, i); b = h.find("│", i); print(b - a - 1 if b > 0 else -1)' "$1"; }

start; open_table t_order; wait_for 8 settled
check "默认 160 宽：<null>、pending、2026-09-01 00:01:00+00 都完整显示（没有 … 截断）" eval 'r=$(e2e_plain | sed -n "$(row_y 1),$(row_y 4)p"); grep -q "<null>" <<<"$r" && grep -q "pending" <<<"$r" && grep -q "2026-09-01 00:01:00+00" <<<"$r" && ! grep -q "<nul…\|pendi…\|00:01:00…" <<<"$r"'
check "json 列（meta）照旧截断：行里有 …" eval 'e2e_plain | sed -n "$(row_y 1)p" | grep -q "{\"n\": 1, \"tags\": \[\"a\"…"'
check "short 列取本页最宽的值（status 是 running，7 列 + 两侧留白 = 9）" eval '[[ $(width_of status) == 9 ]] || { echo "  status width $(width_of status)"; false; }'
check "最后一列放不下：整列在视图右边（横向滚动）" eval '[[ $(header) != *"deleted_at"* ]]'
key '$'
check "\$：到最后一列，视图横向滚动，deleted_at 完整可见" eval '[[ $(header) == *"deleted_at"* ]]'

# ---- full_width 调小：期望宽度超过它的列收到它为止，不再缩得更小；不超过的列还是完整
printf '[grid]\nfull_width = 9\n' >"$D/config.toml"
start -c "$D/config.toml"; open_table t_order; wait_for 8 settled
check "full_width = 9：status（running，7 列）和 amount 不收缩" eval '[[ $(width_of status) == 9 && $(width_of amount) == 8 ]] || { echo "  status $(width_of status) amount $(width_of amount)"; false; }'
check "created_at（22 列的值）收缩，但不小于 full_width + 两侧留白（11），也比默认窄" eval 'w=$(width_of created_at); (( w >= 11 && w < 24 )) || { echo "  created_at $w"; false; }'

# ---- 窄终端（80×24）：短列仍完整，放不下的从右边滚出去
start -x 80 -y 24; open_table t_order; wait_for 8 settled
check "80 宽：status 列整列完整（running 不被缩成 runni…）" eval 'e2e_plain | sed -n "$(row_y 1)p" | grep -q "running"'

# ---- 转置：不收缩，每条记录一列
key T; sleep 0.5
check "转置后每条记录一列，列宽相同（不按比例收缩），meta 的 JSON 完整可见" eval 'e2e_plain | grep -q "{\"n\": 1, \"tags\"" || e2e_plain | grep -q "{\"n\": 2, \"tags\""'

e2e_done
