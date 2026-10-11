#!/usr/bin/env bash
# F12.8 值编辑器的位置、大小和提示行（specs/m12-grid/task.md F12.8；tech-design §10「值编辑器」）
# 浮窗在光标所在格的下方：左边对齐那一格，从格的下一行开始，宽是 pane 内容区的一半，竖长；下方放不下就翻到格的上方（底边贴格的上一行）；
# 底边框上一行是提示：esc 关闭 · C-s 写回并关闭 · :w 写回 · gqq 格式化 JSON，只读时 esc 关闭 · y 复制。自建库 t_v（40 行）。
. "$(dirname "$0")/lib.sh"
SOLO=1
e2e_build || exit 1
. "$(dirname "$0")/palette.sh"
. "$(dirname "$0")/cellops.sh"
. "$(dirname "$0")/valeditor.sh"

D=$(mktemp -d "${TMPDIR:-/tmp}/sqlmux-e2e-cfg.XXXXXX"); trap 'e2e_stop; rm -rf "$D"' EXIT
e2e_own_db || { echo "e2e: could not create the private database" >&2; exit 1; }
psql "$E2E_DB" -q -c "create table t_v (id int primary key, doc jsonb, note text)" -c "insert into t_v select g, '{\"a\":1,\"b\":[1,2,3]}', 'n' || g from generate_series(1, 40) g" >/dev/null
mkdir -p "$D/own" "$D/ro"; printf '[[connection]]\nname = "doraemon"\nengine = "postgres"\ndsn = "%s"\n' "$E2E_DB" >"$D/own/connections.toml"; chmod 600 "$D/own/connections.toml"
printf '[[connection]]\nname = "doraemon"\nengine = "postgres"\ndsn = "%s"\nread_only = true\n' "$E2E_DB" >"$D/ro/connections.toml"; chmod 600 "$D/ro/connections.toml"
hint() { local b; b=($(popbox)); e2e_text $((b[0] + 1)) $((b[0] + b[2] - 2)) $((b[1] + b[3] - 2)) | sed 's/ *$//; s/^ *//'; }   # 底边框上一行

start -C "$D/own"; open_table t_v; wait_for 8 settled
G=($(geom 1)); CY=$(( $(grid_y) + 1 ))      # 第 1 行的屏幕行
key l; CX=$(colx doc); key e; sleep 0.7
b=($(popbox))
check "e：浮窗左边（边框）对齐光标所在格（doc 列，格的左边在 x=$((CX - 1))），从格的下一行开始（y=$((CY + 1))）" eval '[[ ${b[0]} == $((CX - 1)) && ${b[1]} == $((CY + 1)) ]] || { echo "  box ${b[*]}, cell x $CX y $CY"; false; }'
check "宽是 pane 内容区的一半（pane 宽 ${G[2]}，取一半 ±2）；竖长：高度 ≥ 10 行，比宽的一半还高" eval '(( b[2] >= G[2] / 2 - 2 && b[2] <= G[2] / 2 + 2 && b[3] >= 10 && b[3] > b[2] / 2 ))'
check "底边框上一行有提示：esc 关闭 · C-s 写回并关闭 · :w 写回 · gqq 格式化 JSON" eval '[[ $(hint) == "esc 关闭 · C-s 写回并关闭 · :w 写回 · gqq 格式化 JSON" ]] || { echo "  hint: [$(hint)]"; false; }'
check "标题行不变：doc · jsonb" eval '[[ $(pop_title) == "doc · jsonb" ]]'
key Escape; sleep 0.4
# ---- 靠底部的格：下方放不下，翻到格的上方
key G; key l; sleep 0.3; key e; sleep 0.7
b=($(popbox)); CYB=$(( $(grid_y) + 37 ))     # 最后一屏最底下一行（第 40 行在屏上的位置）
cy=$(for y in $(seq $((CYB - 6)) $((CYB + 2))); do e2e_text 35 100 "$y" | grep -q "│ *40 │" && echo $y; done | head -1)
check "光标在最后一行（y=${cy}）：下方放不下，浮窗翻到格的上方，底边贴格的上一行（底边 y=$((cy - 1))）" eval '[[ -n $cy ]] && (( b[1] + b[3] - 1 == cy - 1 && b[1] < cy )) || { echo "  box ${b[*]}, cell y $cy"; false; }'
check "翻到上方后提示行还在" eval '[[ $(hint) == *"C-s 写回并关闭"* ]]'
key Escape; sleep 0.4
# ---- 窄窗口：宽度最小 40
_e2e_kill
start -x 90 -y 30 -C "$D/own"; open_table t_v; wait_for 8 settled
key l; key e; sleep 0.7
b=($(popbox))
check "窄窗口（90 列，pane 内容区一半不到 40）：宽度不小于 40" eval '(( b[2] >= 40 ))'
key Escape; sleep 0.4
_e2e_kill
# ---- 只读：提示行只有 esc 关闭 · y 复制
start -C "$D/ro"; open_table t_v; wait_for 8 settled
key l; key e; sleep 0.7
check "只读连接：提示行是「esc 关闭 · y 复制」（没有 C-s）" eval '[[ $(hint) == "esc 关闭 · y 复制" ]] || { echo "  hint: [$(hint)]"; false; }'
e2e_done
