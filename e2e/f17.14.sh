#!/usr/bin/env bash
# F17.14 手动列宽（specs/m17-polish-exec/task.md F17.14；tech-design §7.8「列宽」3、§14 col_widths）
# C-w > / C-w < 把光标所在列定宽为当前宽度 ± N（次数前缀，最窄 1 列），C-w = 清掉全部；定宽按列名记在 tab 上、
# 写进布局文件的 col_widths，换 tab 再回来、--layout 重开都在；COLS 隐藏再恢复不丢；转置视图不用；结果 tab 同样；
# 鼠标在表头行按住两列间的分隔符左右拖，松开时左边那列的宽度就是拖到的位置，行号列不能拖。只读，用共用库的 t_order。
. "$(dirname "$0")/lib.sh"
SOLO=1
e2e_build || exit 1
. "$(dirname "$0")/palette.sh"

D=$(mktemp -d "${TMPDIR:-/tmp}/sqlmux-e2e-cfg.XXXXXX"); trap 'e2e_stop; rm -rf "$D"' EXIT
mkdir -p "$D/keep" "$D/c"
(umask 077; printf '[[connection]]\nname = "doraemon"\nengine = "postgres"\ndsn = "%s"\n' "$SQLMUX_TEST_PG" >"$D/c/connections.toml")
save_layout() { e2e_keys C-p; sleep 0.3; e2e_type ">save layout"; sleep 0.2; for i in $(seq 20); do e2e_keys BSpace; done; e2e_type ">保存布局"; sleep 0.5; key Enter; sleep 0.4; e2e_type "$1"; key Enter; sleep 0.6; }
head_() { e2e_text 35 159 "${HY:-$(hy)}"; }   # HY: another header row (the result tab's)
# N FIELD: the header cell of the Nth column (1 = the first one, after the row numbers; a key icon in front counts as 1 column) — how wide it is
# drawn (its two blanks of padding included) / the x of the │ after it
cell() { head_ | python3 -c "
import sys
t = sys.stdin.read().rstrip('\n'); x = 35 + len(t.split('│')[0]) + 1; i = 0
for seg in t.split('│')[1:-1]:
    i += 1
    if i == $1:
        print(len(seg) if '$2' == 'w' else x + len(seg)); break
    x += len(seg) + 1
"; }
cw() { cell "$1" w; }
sx() { cell "$1" x; }
cw_is() { local got; got=$(cw "$1"); [[ $got == "$2" ]] || { echo "  $1: $got wide, want $2 ($(head_))"; false; }; }
widths() { local n; for n in 1 2 3 4 5; do echo -n "$n=$(cw $n) "; done; }
layout_file() { echo "$E2E_TMP/config/sqlmux/layouts/$1.toml"; }

start; open_table t_order; wait_for 8 settled
W0=$(widths)
echo "  default widths: $W0"
check "（准备）表头有 id user_id status amount paid 五列" eval '[[ $(cw 3) -gt 0 && $(cw 5) -gt 0 ]]'
ST=$(cw 3); AM=$(cw 4); ID=$(cw 1); UI=$(cw 2)

# ---- C-w > / C-w < 单列
key l; key l   # id → user_id → status
key C-w; key '>'; key C-w; key '>'; key C-w; key '>'
check "status 上 C-w > 三次：status 宽了 3 列，前面的 id、user_id 没动" eval 'cw_is 3 $((ST + 3)) && cw_is 1 $ID && cw_is 2 $UI'
key 3; key C-w; key '<'
check "3 C-w <：回到原来（所有列同 W0）" eval '[[ $(widths) == "$W0" ]] || { echo "  [$(widths)] want [$W0]"; false; }'
key 2; key C-w; key '>'
check "2 C-w >：宽 2 列" eval 'cw_is 3 $((ST + 2))'
key C-w; key '<'
check "C-w <（无前缀）：窄 1 列" eval 'cw_is 3 $((ST + 1))'
key 99; key C-w; key '<'
check "99 C-w <：收到 1 列为止（带两侧空白共 3）" eval 'cw_is 3 3'
key C-w; key '>'
check "再 C-w >：2 列（带空白 4）" eval 'cw_is 3 4'
key l; key 4; key C-w; key '>'
check "换到 amount 再来：amount 宽 4 列，status 仍是 4" eval 'cw_is 4 $((AM + 4)) && cw_is 3 4'
key C-w; key '='
check "C-w =：全部回到自动（同 W0）" eval '[[ $(widths) == "$W0" ]] || { echo "  [$(widths)] want [$W0]"; false; }'

# ---- 定宽不随内容变、不受 40 上限：meta 列放大到很宽，横向滚动
key h; key 5; key C-w; key '>'
check "status 宽 5 列" eval 'cw_is 3 $((ST + 5))'
key C-w; key '='

# ---- 按列名记在 tab 上：换 tab 再回来还在，别的 tab 不受影响
key h; key h   # id
key l; key l   # status
key 3; key C-w; key '>'
open_table t_user; wait_for 8 settled
check "t_user 另一个 tab：没有 status 的定宽（它没有 status 列也不报错、列宽是自己的）" eval '[[ $(cw 1) -gt 0 ]]'
key g T; sleep 0.5
check "回 t_order tab：status 还是 +3" eval 'cw_is 3 $((ST + 3))'

# ---- 转置视图不用它，转回来还在
key T; sleep 0.6; TW=$(e2e_plain | sed -n "$(( $(hy) - 1 )),$(( $(hy) + 12 ))p")
key T; sleep 0.4; key C-w; key '='; key T; sleep 0.6; TW0=$(e2e_plain | sed -n "$(( $(hy) - 1 )),$(( $(hy) + 12 ))p")
check "转置视图：有定宽和没有定宽画得一样" eval '[[ $TW == "$TW0" && -n $TW ]] || { echo "$TW"; echo ---; echo "$TW0"; false; }'
key T; sleep 0.4
key 3; key C-w; key '>'   # C-w = just cleared everything; set it again
check "转回表格：重新设的 status 宽 +3" eval 'cw_is 3 $((ST + 3))'

# ---- COLS 隐藏再恢复宽度还在
key g c; key j; key j; key Space; key Escape; sleep 0.4
check "COLS 隐藏 status：表头里没有它" eval '[[ $(head_) != *status* ]]'
key g c; key j; key j; key Space; key Escape; sleep 0.4
check "COLS 恢复 status：宽度还是 +3" eval 'cw_is 3 $((ST + 3))'

# ---- 布局文件的 col_widths，--layout 读回
save_layout w
check "layout.save w：布局文件里 col_widths = {status = N}，N 是 status 的内容宽（带空白减 2）" eval 'grep -qE "^ *status *= *$((ST + 3 - 2))$" "$(layout_file w)" && grep -q "col_widths" "$(layout_file w)" || { grep -n -A3 "col_widths" "$(layout_file w)"; false; }'
cp -R "$E2E_TMP/config/sqlmux/layouts" "$D/keep/"
e2e_type ":"; sleep 0.3; e2e_type qa; key Enter; sleep 1.2
cp -R "$D/keep/layouts" "$D/c/"
e2e_start -C "$D/c" -S "$D/st2" -D "$D/data2" "$E2E_BIN --layout w"; wait_for 6 flag_is alternate_on 1; sleep 2; wait_for 8 settled
check "--layout w：重开后 status 还是 +3" eval 'cw_is 3 $((ST + 3))'
key C-w; key '='
save_layout w0
check "清掉后再存：布局文件里没有 col_widths" eval '! grep -q "col_widths" "$(layout_file w0)"'

# ---- 鼠标：拖表头分隔符
key g g; key 0   # cursor to the first column
X=$(sx 2); Y=$(hy)
check "（准备）找到 user_id 右边的分隔符" eval '[[ -n $X && $Y -gt 0 ]]'
e2e_down "$X" "$Y"; sleep 0.2; e2e_drag_to $((X + 5)) "$Y"; sleep 0.2; e2e_up $((X + 5)) "$Y"; sleep 0.5
check "分隔符向右拖 5 列：user_id 右边的 │ 落在松开的位置，宽了 5 列" eval '[[ $(sx 2) == $((X + 5)) ]] || { echo "  sep at $(sx 2), want $((X + 5)) ($(head_))"; false; }'
check "拖动不当成点击表头：没有排序标记" eval '! e2e_text 35 159 "$(hy)" | grep -qE "[]"'
X2=$(sx 2)
e2e_down "$X2" "$Y"; e2e_drag_to $((X2 - 30)) "$Y"; e2e_up $((X2 - 30)) "$Y"; sleep 0.5
check "向左拖过头：user_id 收到 1 列为止（带空白共 3）" eval 'cw_is 2 3'
# 行号列：它右边的分隔符不能拖
RX=40
before=$(head_)
e2e_down "$RX" "$Y"; e2e_drag_to $((RX + 6)) "$Y"; e2e_up $((RX + 6)) "$Y"; sleep 0.4
check "行号列右边的分隔符拖不动：表头不变" eval '[[ $(head_) == "$before" ]]'

# ---- 退出重启（不给 --layout）：上次的布局里 col_widths 还在
key g g; key 0; key l; key l; key 4; key C-w; key '>'
check "（准备）status 宽 +4" eval 'cw_is 3 $((ST + 4))'
e2e_type ":"; sleep 0.3; e2e_type qa; key Enter; sleep 1.2
e2e_start -C "$D/c" -S "$D/st2" -D "$D/data2" "$E2E_BIN"; wait_for 6 flag_is alternate_on 1; sleep 2; wait_for 8 settled
check "重启后 status 还是 +4" eval 'cw_is 3 $((ST + 4))'

# ---- 结果 tab 同样（快速 SQL → C-t 开成结果 tab，在 ② 里）
e2e_type ";select id, status, amount from t_order order by id limit 5"; sleep 1.2; key Enter; sleep 1.8
e2e_keys C-t; sleep 1.5; key Escape; sleep 0.6
e2e_click 60 31; sleep 0.4; key 0; key l
HY=$(( $(geom 2 | cut -d" " -f2) + 1 ))
R0=$(cw 2)
check "（准备）结果 tab：id status amount 三列、没有定宽" eval '[[ $(head_) == *"status"*"amount"* && -n $R0 ]] || { head_; false; }'
key 2; key C-w; key '>'
check "结果 tab 上 2 C-w >：status 宽 2 列，id 没动" eval 'cw_is 2 $((R0 + 2))'
key C-w; key '='
check "结果 tab 上 C-w =：回到自动" eval 'cw_is 2 $R0'
R1=$(cw 1)
X=$(sx 1)
e2e_down "$X" "$HY"; e2e_drag_to $((X + 5)) "$HY"; sleep 0.4
check "结果 tab 的表头：按住拖 5 列，还没松开 id 右边的 │ 已经在 +5" eval '[[ $(sx 1) == $((X + 5)) ]] || { echo "  sep at $(sx 1), want $((X + 5)) ($(head_))"; false; }'
e2e_up $((X + 5)) "$HY"; sleep 0.5
check "松开：id 宽 +5，status amount 两列还在，不当成排序点击" eval 'cw_is 1 $((R1 + 5)) && [[ $(head_) == *status*amount* ]]'
key C-w; key '='
check "拖过之后 C-w =：回到自动" eval 'cw_is 1 $R1'
HB=$(head_); SX=$(( $(sx 1) - 3 ))
e2e_click "$SX" "$HY"; sleep 0.6
check "点结果 tab 的表头（列名上）不排序：表头一个字没变" eval '[[ $(head_) == "$HB" ]] || { echo "  [$HB] -> [$(head_)]"; false; }'
e2e_done
