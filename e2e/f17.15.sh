#!/usr/bin/env bash
# F17.15 转置视图的列宽（specs/m17-polish-exec/task.md F17.15；tech-design §7.8「列宽」3「转置视图」、§14 transpose_width）
# 转置后 > / < 调全部记录列的统一宽度（次数前缀、最窄 1 列），= 回自动；字段名列不动；按 tab 记、写进布局文件的 transpose_width，
# 换 tab 再回来、--layout 重开、重启都在；切回非转置 col_widths 不变、再转置宽度还在；记录号行的 │ 拖动改同一个值，字段名列右边那条拖不动。
# 只读，用共用库的 t_order。
. "$(dirname "$0")/lib.sh"
SOLO=1
e2e_build || exit 1
. "$(dirname "$0")/palette.sh"

D=$(mktemp -d "${TMPDIR:-/tmp}/sqlmux-e2e-cfg.XXXXXX"); trap 'e2e_stop; rm -rf "$D"' EXIT
mkdir -p "$D/keep" "$D/c"
(umask 077; printf '[[connection]]\nname = "doraemon"\nengine = "postgres"\ndsn = "%s"\n' "$SQLMUX_TEST_PG" >"$D/c/connections.toml")
save_layout() { e2e_keys C-p; sleep 0.3; e2e_type ">save layout"; sleep 0.2; for i in $(seq 20); do e2e_keys BSpace; done; e2e_type ">保存布局"; sleep 0.5; key Enter; sleep 0.4; e2e_type "$1"; key Enter; sleep 0.6; }
head_() { e2e_text 35 159 "$(hy)"; }
# N FIELD: how wide the Nth column of the header row is drawn (blanks included) / the x of the │ after it; N = 0 is the row-number column
# (the field names' one in the turned view), 1 is the next
cell() { head_ | python3 -c "
import sys
t = sys.stdin.read().rstrip('\n'); segs = t.split('│'); x = 35; n = $1
if n == 0:
    print(len(segs[0]) if '$2' == 'w' else x + len(segs[0]))
else:
    x += len(segs[0]) + 1
    for i, seg in enumerate(segs[1:-1], 1):
        if i == n:
            print(len(seg) if '$2' == 'w' else x + len(seg)); break
        x += len(seg) + 1
"; }
cw() { cell "$1" w; }
sx() { cell "$1" x; }
cw_is() { local got; got=$(cw "$1"); [[ $got == "$2" ]] || { echo "  $1: $got wide, want $2 ($(head_))"; false; }; }
recs() { echo "$(cw 1) $(cw 2) $(cw 3)"; }            # the first three records' widths
recs_are() { [[ $(recs) == "$1 $1 $1" ]] || { echo "  records [$(recs)], want $1 each ($(head_))"; false; }; }
layout_file() { echo "$E2E_TMP/config/sqlmux/layouts/$1.toml"; }

start; open_table t_order; wait_for 8 settled
ST=$(cw 3)                                            # 非转置：status 的宽
key T; sleep 0.6
LBL=$(cw 0); A=$(cw 1)
echo "  turned: label column $LBL, records $(recs)"
check "（准备）转置：记录列都是同一个自动宽度" eval '[[ -n $A && $A -gt 3 ]] && recs_are $A'

# ---- > / < / = 全部记录列一起
key '>'; key '>'; key '>'
check "> 三次：每个记录列宽了 3 列，字段名列没动" eval 'recs_are $((A + 3)) && cw_is 0 $LBL'
key 3; key '<'
check "3<：回到原来" eval 'recs_are $A && cw_is 0 $LBL'
key 2; key '>'
check "2>：宽 2 列" eval 'recs_are $((A + 2))'
key '<'
check "<（无前缀）：窄 1 列" eval 'recs_are $((A + 1))'
key 99; key '<'
check "99<：收到 1 列为止（带两侧空白共 3），每条记录都是" eval 'recs_are 3'
key '>'
check "再 >：从 1 回升到 2（共 4）" eval 'recs_are 4'
key 8; key '>'
check "8>：宽 10（共 12）" eval 'recs_are 12'
key '='
check "=：回到自动（同 A）" eval 'recs_are $A && cw_is 0 $LBL'
key '='
check "再 =：没有定宽也无事" eval 'recs_are $A'

# ---- 光标在哪条记录上都一样（h / l 换记录再 >，统一宽度，不是只改光标那条）
key l; key l; key '>'; key '>'
check "光标在第 3 条记录上 >>：三条记录列都 +2，不是只改那一条" eval 'recs_are $((A + 2))'
key '='; key g g; key 0

# ---- 按 tab 记：换 tab 再回来还在，别的 tab 不受影响
key 4; key '>'
open_table t_user; wait_for 8 settled
check "t_user 另一个 tab（非转置）：表头正常" eval '[[ $(head_) == *name* ]]'
key T; sleep 0.5
B=$(cw 1)
check "t_user 转置后记录列是自己的自动宽度（不是 t_order 的 +4）" eval '[[ -n $B && $B != $((A + 4)) ]]'
key T; sleep 0.3
key g T; sleep 0.6
check "回 t_order tab：转置里每个记录列还是 +4" eval 'recs_are $((A + 4))'

# ---- 切回非转置：col_widths 不变；非转置 > 仍是调光标那一列；再转置宽度还在
key T; sleep 0.6
check "切回非转置：status 还是自动宽（转置的 > 没碰 col_widths）" eval 'cw_is 3 $ST'
key g g; key 0; key l; key l; key 2; key '>'
check "非转置里 2>：只有 status 宽 2 列" eval 'cw_is 3 $((ST + 2)) && cw_is 1 $(cw 1)'
key T; sleep 0.6
check "再转置：记录列宽度还是 +4，没被非转置的 > 带走" eval 'recs_are $((A + 4))'
key T; sleep 0.5
check "再切回：status 还是 +2" eval 'cw_is 3 $((ST + 2))'
key '='; key T; sleep 0.5
check "非转置的 = 只清 col_widths：再转置记录列还是 +4" eval 'recs_are $((A + 4))'

# ---- 布局文件的 transpose_width，--layout 读回
save_layout w
check "layout.save w：布局文件里 transpose_width = $((A + 4 - 2))（带空白减 2）" eval 'grep -qE "^ *transpose_width *= *$((A + 4 - 2))$" "$(layout_file w)" || { grep -n "transpose\|col_widths" "$(layout_file w)"; false; }'
cp -R "$E2E_TMP/config/sqlmux/layouts" "$D/keep/"
e2e_type ":"; sleep 0.3; e2e_type qa; key Enter; sleep 1.2
cp -R "$D/keep/layouts" "$D/c/"
e2e_start -C "$D/c" -S "$D/st2" -D "$D/data2" "$E2E_BIN --layout w"; wait_for 6 flag_is alternate_on 1; sleep 2; wait_for 8 settled
[[ $(cw 1) == "" || $(head_) == *id* ]] && { key T; sleep 0.6; }   # 布局里没记转置状态的话这里还是表格：转一下
check "--layout w：转置后记录列是 +4" eval 'recs_are $((A + 4))'
key '='
save_layout w0
check "= 后再存：布局文件里 transpose_width 是 0（同 page、n：这个 TOML 库的 omitempty 不省整数，0 就是没设）" eval '! grep -qE "transpose_width *= *[1-9]" "$(layout_file w0)" || { grep -n "transpose\|col_widths" "$(layout_file w0)"; echo "  $(head_)"; false; }'

# ---- 退出重启（不给 --layout）
key 5; key '>'
e2e_type ":"; sleep 0.3; e2e_type qa; key Enter; sleep 1.2
e2e_start -C "$D/c" -S "$D/st2" -D "$D/data2" "$E2E_BIN"; wait_for 6 flag_is alternate_on 1; sleep 2; wait_for 8 settled
[[ $(head_) == *id* ]] && { key T; sleep 0.6; }
check "重启后（转置）记录列是 +5" eval 'recs_are $((A + 5))'
key '='

# ---- 鼠标：记录号行的 │ 拖
X=$(sx 1); Y=$(hy)
e2e_down "$X" "$Y"; e2e_drag_to $((X + 5)) "$Y"; sleep 0.4
check "按住拖 5 列：还没松开第 1 条右边的 │ 已经在 +5，第 2、3 条也跟着宽了" eval '[[ $(sx 1) == $((X + 5)) && $(cw 2) == $((A + 5)) && $(cw 3) == $((A + 5)) ]] || { echo "  sep $(sx 1) want $((X + 5)); $(recs)"; false; }'
e2e_up $((X + 5)) "$Y"; sleep 0.5
check "松开：每个记录列 +5，字段名列没动" eval 'recs_are $((A + 5)) && cw_is 0 $LBL'
X2=$(sx 2)
e2e_down "$X2" "$Y"; e2e_drag_to $(( $(sx 0) + 1 )) "$Y"; e2e_up $(( $(sx 0) + 1 )) "$Y"; sleep 0.5
check "向左拖过头（从第 2 条右边的 │）：每个记录列收到 1 列为止（共 3）" eval 'recs_are 3'
key '='
H0=$(head_); LX=$(sx 0)
e2e_down "$LX" "$Y"; e2e_drag_to $((LX + 6)) "$Y"; e2e_up $((LX + 6)) "$Y"; sleep 0.4
check "字段名列右边的 │ 拖不动：表头不变" eval '[[ $(head_) == "$H0" ]] || { echo "  [$H0] -> [$(head_)]"; false; }'
e2e_done
