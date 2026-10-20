#!/usr/bin/env bash
# F17.15 转置视图列宽的细节（specs/m17-polish-exec/task.md F17.15；tech-design §7.8「列宽」3「转置视图」）
# 起点是光标所在那条记录当前画的宽度（长值的那条上 3> 是它的宽度 + 3，不是第一条的）；点转置后的记录号跳到那条记录、
# 点非转置的表头仍排序（分隔符的命中区没抢它们）；转置的表一条记录都没有时 > < = 不出事；结果 tab 转置后同样能调。只读，用共用库。
. "$(dirname "$0")/lib.sh"
SOLO=1
e2e_build || exit 1
. "$(dirname "$0")/palette.sh"

head_() { e2e_text 35 159 "${HY:-$(hy)}"; }
# N FIELD: how wide the Nth column of the header row is drawn (blanks included) / the x of the │ after it; 0 is the row-number column
# (the field names' one in the turned view), 1 the next
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

# ---- 起点是光标所在那条记录当前画的宽度：t_log 第 2 条的 msg 很长（自动宽度比第 1 条宽）
start; open_table t_log; wait_for 8 settled
key T; sleep 0.6
W1=$(cw 1); W2=$(cw 2)
echo "  t_log records: $W1 $W2 ($(head_))"
check "（准备）第 2 条记录比第 1 条宽（自动宽度各取各的）" eval '[[ -n $W1 && -n $W2 && $W2 -gt $W1 ]]'
key l; key 3; key '>'
check "光标在第 2 条（长值）上 3>：统一宽度是它的 +3，第 1 条也是" eval 'cw_is 2 $((W2 + 3)) && cw_is 1 $((W2 + 3))'
key '='
check "=：回到各自的自动宽度" eval 'cw_is 1 $W1 && cw_is 2 $W2'
key h; key 3; key '>'
check "光标在第 1 条上 3>：统一宽度是第 1 条的 +3（不是第 2 条的）" eval 'cw_is 1 $((W1 + 3)) && cw_is 2 $((W1 + 3))'
key '='

# ---- 转置的表一条记录都没有：> < = 不出事
key T; sleep 0.4
key /; e2e_type "1 = 0"; key Enter; wait_for 8 settled; sleep 0.5
key T; sleep 0.6
check "（准备）WHERE 1 = 0：没有记录，转置视图" eval 'e2e_plain | grep -q "0 行"'
for k in '>' '<' '=' 3; do key "$k"; done; key '>'; key '<'; key '='
check "没有记录时按 > < = 3> 都不出事：还在跑，查询条还在" eval 'running && e2e_plain | grep -q "0 行"'

# ---- 点击：转置后点记录号跳到那条记录；非转置点表头仍排序
open_table t_order; wait_for 8 settled
key T; sleep 0.6
Y=$(hy)
check "（准备）转置，光标在第 1 条" eval '[[ $(pos) == 1,1 ]] || { echo "  [$(pos)]"; false; }'
e2e_click $(( $(sx 2) + 3 )) "$Y"; sleep 0.5
check "点第 3 条的记录号（表头行）：光标跳到第 3 条" eval '[[ $(pos) == 3,1 || $(pos) == 3,* ]] || { echo "  [$(pos)]"; false; }'
e2e_click $(( $(sx 0) + 3 )) "$Y"; sleep 0.5
check "点第 1 条的记录号：光标回到第 1 条" eval '[[ $(pos) == 1,* ]] || { echo "  [$(pos)]"; false; }'
key T; sleep 0.6
Y=$(hy)
amount_x=$(e2e_find "amount" "$Y" | cut -d' ' -f1)
e2e_click $((amount_x + 2)) "$Y"; sleep 1; wait_for 8 settled
check "非转置：点表头 amount 仍按 amount 排序（ORDER 条上是 amount）" eval 'e2e_plain | sed -n 3p | grep -q "ORDER amount" || { e2e_plain | sed -n 3p | cut -c34-120; false; }'

# ---- 结果 tab 转置后同样能调（快速 SQL → C-t 开成结果 tab，在 ② 里）
e2e_type ";select id, status, amount from t_order order by id limit 5"; sleep 1.2; key Enter; sleep 1.8
e2e_keys C-t; sleep 1.5; key Escape; sleep 0.6
e2e_click 60 31; sleep 0.4
G=($(geom 2)); HY=$((G[1] + 1))
key T; sleep 0.6; key 0   # 0: the first record, where the width starts from
R0=$(cw 1)
check "（准备）结果 tab 转置：记录列有宽度" eval '[[ -n $R0 && $R0 -gt 2 ]]'
key 3; key '>'
check "结果 tab 转置后 3>：每个记录列 +3" eval 'cw_is 1 $((R0 + 3)) && cw_is 2 $((R0 + 3))'
key '='
check "结果 tab 转置后 =：回自动" eval 'cw_is 1 $R0'
unset HY
e2e_done
