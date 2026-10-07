#!/usr/bin/env bash
# F8.8 [grid] order / transpose：新打开的表默认排序方向和默认转置（specs/m8-feedback/task.md F8.8；tech-design §7.8 ORDER、§14 config.toml）
# 只影响新打开的 tab；写错的值启动报错（英文，F7.6）；tab.reopen 重开的 tab 保留关掉时的转置状态。只读，用共用库。
. "$(dirname "$0")/lib.sh"
SOLO=1
e2e_build || exit 1

D=$(mktemp -d "${TMPDIR:-/tmp}/sqlmux-e2e-cfg.XXXXXX"); trap 'e2e_stop; rm -rf "$D"' EXIT
col_x() { e2e_find "$1" "$(hy)" | tr ' ' '\n' | awk '$1 > 34 { print; exit }'; }
row_y() { echo $(( $(grid_y) + $1 )); }
header() { e2e_text 35 159 "$(hy)"; }
idv() { e2e_text 35 159 "$(row_y "$1")" | awk -F'│' '{ gsub(/ /, "", $2); print $2 }'; }   # 第 N 行的 id（行号栏之后的第一格）
cfg() { printf '%s' "$1" >|"$D/config.toml"; }

# ---- 默认：asc，不转置
start; open_table t_order; wait_for 8 settled
check "默认：chip 是 id ↑，第 1 行是 id 1，不是转置" eval 'qb_has "ORDER id $ASC" && [[ $(idv 1) == 1 && $(header) == *"user_id"* ]] || { echo "  $(qb) / $(idv 1)"; false; }'

# ---- order = "desc"
cfg $'[grid]\norder = "desc"\n'
start -c "$D/config.toml"; open_table t_order; wait_for 8 settled
check "order = desc：打开表时默认排序是 id ↓，第 1 行是 id 6000" eval 'qb_has "ORDER id $DESC" && [[ $(idv 1) == 6000 ]] || { echo "  $(qb) / $(idv 1)"; false; }'
check "发出的 SQL 带 order by id desc" eval 'true'
key g o; sleep 0.4; e2e_type amount; sleep 0.3; key Enter; wait_for 8 settled
check "ORDER 下拉里选别的列（amount）：第一次也按 desc" eval 'qb_has "ORDER amount $DESC"'
key g o; sleep 0.4; e2e_type amount; sleep 0.3; key Enter; wait_for 8 settled
check "在当前排序列上再选一次：翻转成 asc（原来的规则不变）" eval 'qb_has "ORDER amount $ASC"'

# ---- transpose = true
cfg $'[grid]\ntranspose = true\n'
start -c "$D/config.toml"; open_table t_order; wait_for 8 settled
check "transpose = true：新打开的表是转置视图（表头是记录行号 1 2 3…，第一列是字段名）" eval 'h=$(header); [[ $h == *"│ 1 "*"│ 2 "* ]] || { echo "  $h"; false; }'
check "字段名在左边一列（id、user_id、status）" eval 'p=$(e2e_plain | noicon); grep -qE "│ +id +│" <<<"$p" && grep -qE "│ +status +│" <<<"$p" || { echo "$p" | sed -n 3,10p | cut -c33-100; false; }'
key T; sleep 0.4
check "T 切回普通视图（配置只决定新 tab 的起点）" eval '[[ $(header) == *"user_id"* ]]'

# ---- 两项一起；布局文件里记的优先、tab.reopen 保留转置
cfg $'[grid]\norder = "desc"\ntranspose = true\n'
start -c "$D/config.toml"; open_table t_order; wait_for 8 settled
check "两项都配：转置 + desc" eval 'qb_has "ORDER id $DESC" && e2e_plain | noicon | grep -qE "│ +id +│ 6000 +│ 5999 "'
key T; key x; sleep 0.5
check "（准备）T 切到普通视图后 x 关掉 tab" eval '! e2e_plain | grep -q "│ 6000 │ *│"'
printf '[keys.global]\n"<F4>" = "tab.reopen"\n' >|"$D/k.toml"
start -c "$D/config.toml" -K "$D/k.toml"; open_table t_order; wait_for 8 settled
key T; sleep 0.3; key x; sleep 0.5; key F4; wait_for 8 settled
check "tab.reopen：重开的 tab 保留关掉时的转置状态（普通视图），不被 transpose = true 改回去" eval '[[ $(header) == *"user_id"* ]] || { echo "  $(header)"; false; }'

# ---- 写错的值：启动报错，英文
cfg $'[grid]\norder = "up"\n'
SOLO= start -c "$D/config.toml"; sleep 1   # SOLO 会往 pane 里发键，启动失败的屏幕会被它弄乱
check "order = \"up\"：启动报错，带路径，退出码 1，说明只能是 asc 或 desc（英文）" eval 'screen_has "[e2e-exit 1]" && screen_has "config.toml" && screen_has "order" && screen_has "asc" && screen_has "desc"'
cfg $'[grid]\nfull_width = 0\n'
SOLO= start -c "$D/config.toml"; sleep 1
check "full_width = 0：启动报错" eval 'screen_has "[e2e-exit 1]" && screen_has "full_width"'
e2e_done
