#!/usr/bin/env bash
# F18.11 Redis 树过滤扫描在途时的提示与取消（specs/m18-windows-mssql/task.md F18.11；tech-design §7.8「Redis 的树」、§8.7「树」）
# 一个 6 万多键的库、经一个慢代理（一次来回 500 ms）：树上 / 输入模式，过滤的 SCAN 在途时连接和 db 节点照画、db 节点的尾注是 dim 的「搜索中…」，
# 扫完换回键数、键列出来；连续改模式时旧的扫描被取消，只有最后一个模式的结果出现；扫完一个都没有才按空处理。每个脚本自己起 redis-server。
. "$(dirname "$0")/lib.sh"
e2e_build || exit 1
. "$(dirname "$0")/palette.sh"
. "$(dirname "$0")/redis.sh"

D=$(mktemp -d "${TMPDIR:-/tmp}/sqlmux-e2e-cfg.XXXXXX"); trap 'e2e_stop; redis_stop; rm -rf "$D"' EXIT
redis_start R || exit 1
rc -n 0 EVAL "for i = 1, 60000 do redis.call('SET', 'q:other:' .. i, 1) end for i = 1, 346 do redis.call('SET', 'p:employees:' .. i .. '@x.com', 1) end for i = 1, 20 do redis.call('SET', 'r:team:' .. i, 1) end" 0 >/dev/null
redis_slow 500 || exit 1
mkdir -p "$D/c"
(umask 077; printf '[[connection]]\nname = "rd"\nengine = "redis"\ndsn = "redis://localhost:%s/0"\n' "$SPORT" >"$D/c/connections.toml")
side() { e2e_plain | python3 -c 'import re, sys; [print(re.sub("[-] ?", "", l[1:33]).rstrip("│ ")) for l in sys.stdin.read().split("\n")]'; }
tree() { side | python3 -c "
import re, sys
for l in sys.stdin:
    if l.strip(): print(re.sub(r' +', ' ', l.rstrip()))" | sed -n 1,14p; }

DIM=#565f89
leader() { e2e_keys Space; e2e_type "$1"; sleep 0.6; }
srow() { side | grep -n "$1" | head -1 | cut -d: -f1; }   # the screen row of the first sidebar row that has it
SOLO=1
e2e_start -C "$D/c" "$E2E_BIN"; wait_for 8 flag_is alternate_on 1; solo
wait_for 40 eval 'tree | grep -q "db0 60k"'
check "（准备）慢 Redis 上连好：db0 的尾注是键数 60k，命名空间 p、q、r 都在" eval 'tree | grep -q "▾ db0 60k" && tree | grep -qE "▸ p [0-9]+" && tree | grep -qE "▸ q [0-9]+"'
rc monitor >"$D/mon.txt" 2>&1 & MON=$!; sleep 0.5

# ---- 扫描在途：连接和 db 节点照画，db 节点的尾注是 dim 的「搜索中…」
key C-h; key /; e2e_type "p:employees"
wait_for 6 eval 'tree | grep -q "搜索中"'
check "过滤 p:employees 的扫描在途：连接 rd 和 db0 节点都画着，db0 的尾注是「搜索中…」，计数 0/60366" eval 'tree | grep -q "^ ▾ rd$" && tree | grep -q "▾ db0 搜索中…" && tree | grep -q "p:employees 0/60366" || { tree | sed -n 1,9p; false; }'
y=$(srow "db0.*搜索中"); x=$(e2e_find "搜索中…" "$y" | cut -d" " -f1)
check "「搜索中…」是 dim 色" eval '[[ -n $x ]] && style_has $x $y fg=$DIM'
wait_for 30 eval '! tree | grep -q "搜索中"'
check "扫完：尾注换回键数 60k，键列出来：p 346 → employees 346 → 键" eval 'tree | grep -q "▾ db0 60k" && tree | grep -q "▾ p 346" && tree | grep -q "▾ employees 346" && tree | grep -q "p:employees 346/60366" || { tree | sed -n 1,10p; false; }'
check "（对照）q、r 下面没有东西混进来（过滤只留匹配的）" eval '! tree | grep -q "▾ q " && ! tree | grep -q "▾ r "'

# ---- 连续改模式：旧的扫描被取消，只有最后一个模式的结果出现
key /; sleep 0.3; e2e_keys C-u; sleep 0.2; e2e_type "p:employees"; sleep 1.2
L0=$(( $(wc -l <"$D/mon.txt" | tr -d " ") + 1 ))
for i in $(seq 11); do e2e_keys BSpace; done; e2e_type "r:team"
wait_for 6 eval 'tree | grep -q "搜索中"'; wait_for 30 eval '! tree | grep -q "搜索中"'; sleep 1
check "改成 r:team：结果只有 r:team 的 20 个键（r 20 → team 20），旧模式 p:employees 的结果没有出现" eval 'tree | grep -q "r:team 20/60366" && tree | grep -q "▾ team 20" && ! tree | grep -q "employees" || { tree | sed -n 1,10p; false; }'
kill $MON 2>/dev/null; wait $MON 2>/dev/null
check "服务端上看到：旧模式 *p:employees* 的扫描在改模式时被取消（整库要 7 次 SCAN，改了之后它至多再发 1 次，不会跑完），新模式 *r:team* 完整 7 次" eval 'o=$(tail -n +$L0 "$D/mon.txt" | grep -i "\"scan\"" | grep -c "p:employees"); n=$(tail -n +$L0 "$D/mon.txt" | grep -i "\"scan\"" | grep -c "r:team"); [[ $o -le 2 && $n == 7 ]] || { echo "  old $o new $n"; false; }'
check "过滤的 SCAN 用 COUNT 10000（不是 1000）" eval 'grep -i "\"scan\"" "$D/mon.txt" | grep -q "\"count\" \"10000\"" && ! grep -i "\"scan\"" "$D/mon.txt" | grep -q "\"count\" \"1000\""'

# ---- 扫完一个都没有：按空处理（连接和 db 节点隐藏）
key /; sleep 0.3; e2e_keys C-u; sleep 0.2; sleep 0.3; e2e_type "nothing-here"
wait_for 6 eval 'tree | grep -q "搜索中"'; wait_for 30 eval '! tree | grep -q "搜索中"'; sleep 1
check "扫完没有匹配：连接和 db 节点不画（按空处理），计数 0/60366" eval 'tree | grep -q "nothing-here 0/60366" && ! tree | grep -q "db0" && ! tree | grep -q "搜索中" || { tree | sed -n 1,8p; false; }'
# ---- esc：过滤清空，在途的扫描被取消（服务端上 SCAN 不再继续发）
key Escape; sleep 0.5
check "（准备）esc 清掉 nothing-here：db0 回到 60k，没有过滤计数" eval 'tree | grep -q "▾ db0 60k" && ! tree | grep -q "/60366"'
rc monitor >"$D/mon2.txt" 2>&1 & MON=$!; sleep 0.5
key /; sleep 0.3; e2e_type "p:employees"
wait_for 6 eval 'tree | grep -q "搜索中"'
key Escape; sleep 0.3; L1=$(grep -ic "\"scan\"" "$D/mon2.txt"); sleep 5
check "扫描在途按 esc：过滤清空（db0 回到 60k，没有 0/60366 那种计数，没有「搜索中…」）" eval 'tree | grep -q "▾ db0 60k" && ! tree | grep -q "搜索中" && ! tree | grep -q "/60366" || { tree | sed -n 1,8p; false; }'
check "服务端上：esc 之后 5 秒里 SCAN 不再增加（整库要 7 次，被取消的扫描至多再发 1 次）" eval 'n=$(grep -ic "\"scan\"" "$D/mon2.txt"); [[ $n -le $((L1 + 1)) && $n -lt 7 ]] || { echo "  at esc $L1, after 5s $n"; false; }'
kill $MON 2>/dev/null; wait $MON 2>/dev/null

# ---- 两个窗口：窗口 B 的扫描在途时去窗口 A 改过滤，回到 B，B 照样出结果（审查发现的 bug：B 卡在「搜索中…」）
key /; sleep 0.3; e2e_type "p:employees"
wait_for 6 eval 'tree | grep -q "搜索中"'
key Enter; sleep 0.3   # 退出输入框、过滤保留（INSERT 里 SPC 是打字）
leader c; sleep 0.5
key C-h; sleep 0.3; key /; sleep 0.3; e2e_type "r:team"; key Enter; sleep 0.3
check "（准备）窗口 data-2 里过滤 r:team，窗口 data 的扫描还在途时切走了" eval 'e2e_plain | grep -q "0: data  1: data-2\*" && tree | grep -q "r:team"'
wait_for 30 eval 'tree | grep -q "r:team 20/60366"'
leader p; sleep 0.5
wait_for 30 eval '! tree | grep -q "搜索中"'
check "回到窗口 data：它自己的过滤 p:employees 出了结果（p 346 → employees 346），没有停在「搜索中…」" eval 'tree | grep -q "p:employees 346/60366" && tree | grep -q "▾ employees 346" && ! tree | grep -q "搜索中" || { tree | sed -n 1,10p; false; }'

e2e_done
