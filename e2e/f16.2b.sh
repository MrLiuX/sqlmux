#!/usr/bin/env bash
# F16.2 Redis 的树（二）：键和命名空间上的操作（specs/m16-redis/task.md F16.2 的 e；tech-design §7.8「Redis 的树」、§8.7「树」、§13）
# 键：dd / 右键「删除」（UNLINK，不问）、「改名…」（状态栏问新名字，RENAME；目标存在时再确认覆盖）、「设 TTL…」（秒或 90m、空 = 不过期）；
# 命名空间：「删除…」先扫出下面的每个键（不设预算，确认框里的数是真的）、确认（confirm_dangerous）后每 500 个一条 UNLINK，C-c 能停、toast 已删数量；
# 只读连接上这些都被拒。树的形状、过滤、预算在 f16.2.sh。
. "$(dirname "$0")/lib.sh"
e2e_build || exit 1
. "$(dirname "$0")/palette.sh"
. "$(dirname "$0")/redis.sh"

D=$(mktemp -d "${TMPDIR:-/tmp}/sqlmux-e2e-cfg.XXXXXX"); trap 'e2e_stop; redis_stop; rm -rf "$D"' EXIT
redis_start R || exit 1
mkdir -p "$D/c" "$D/n"
conns() { cat <<TOML
[[connection]]
name = "rd"
engine = "redis"
dsn = "redis://localhost:$RPORT/0"

[[connection]]
name = "ro"
engine = "redis"
dsn = "redis://localhost:$RPORT/0"
read_only = true

[[connection]]
name = "big"
engine = "redis"
dsn = "redis://localhost:$RPORT/8"
TOML
}
(umask 077; conns >"$D/c/connections.toml"; conns >"$D/n/connections.toml")
printf 'confirm_dangerous = true\n' >"$D/c/config.toml"; printf 'confirm_dangerous = false\n' >"$D/n/config.toml"
reseed() { rc flushall >/dev/null; redis_seed; }
side() { e2e_plain | python3 -c 'import re, sys; [print(re.sub("[-] ?", "", l[1:33]).rstrip("│ ")) for l in sys.stdin.read().split("\n")]'; }
side_all() { e2e_plain | python3 -c 'import re, sys; [print(re.sub("[-] ?", "", l)) for l in sys.stdin.read().split("\n")]'; }
tree() { side | python3 -c "
import re, sys
on = False
for l in sys.stdin:
    if not l.strip(): continue
    ind = len(l) - len(l.lstrip())
    if on and ind <= 2: break
    if not on and re.match(r'^[▸▾] +db\\d+', l.strip()): on = True
    if on: print(re.sub(r' +', ' ', l.strip()))"; }
nth_row() { side | python3 -c "
import re, sys
n = int(sys.argv[2])
for i, l in enumerate(sys.stdin.read().split('\\n')):
    if re.search(sys.argv[1], l.strip()):
        n -= 1
        if n == 0: print(i + 1); break" "$1" "${2:-1}"; }
click_row() { local y; y=$(nth_row "$@"); [[ -n $y ]] || return 1; e2e_click 14 "$y"; sleep 0.5; }
expand() { click_row "^[▸▾] +$1( |\$)" "${2:-1}"; }
rclick() { _sgr 2 "$1" "$2" M; _sgr 2 "$1" "$2" m; sleep 0.6; }
menu_pick() { local y; y=$(nth_row "$1"); rclick 14 "$y"; local i; for ((i = 0; i < $2; i++)); do key j; done; key Enter; sleep 0.8; }   # ROW-REGEX N: right-click the row, the Nth item, ↵
to_row() { local y i; y=$(nth_row "$1"); key C-h; key g g; for ((i = 4; i < y; i++)); do key j; done; }   # the tree's cursor on the first row matching the regex
toast() { e2e_text 1 "$(e2e_flag pane_width)" "$(( $(H) - 1 ))" | python3 -c 'import sys; print(sys.stdin.read().strip("└─┘│ \n"))'; }
prompt() { e2e_text 1 "$(e2e_flag pane_width)" "$(H)" | python3 -c 'import re, sys; print(re.split(r"\s{3,}", sys.stdin.read().strip())[0])'; }   # the status bar's left: what it asks
quit_app() { e2e_type ":"; sleep 0.4; e2e_type qa; key Enter; wait_for 8 exited; sleep 0.5; }
sstart() { e2e_start $SX -C "${DIR:-$D/c}" "$E2E_BIN $1"; wait_for 8 flag_is alternate_on 1; sleep 2; }
ttl() { rc ttl "$1"; }
confirm_open() { e2e_plain | grep -qE "y (删除|覆盖) +n 取消"; }

# ---- 键：dd 和右键「删除…」（confirm_dangerous 时先问）
reseed; sstart rd
expand str; to_row '^plain$'; key d; key d; sleep 0.8
check "光标在键 plain 上 dd：弹确认「删除键 str:plain？」，服务端上还没动" eval 'confirm_open && e2e_plain | grep -q "删除键 str:plain？" && [[ $(rc exists str:plain) == 1 ]]' || e2e_plain | sed -n 14,30p | cut -c30-130
key n; sleep 0.6
check "n 取消：str:plain 还在，确认框关了" eval '! confirm_open && [[ $(rc exists str:plain) == 1 ]]'
key d; key d; sleep 0.8; key Escape; sleep 0.6
check "esc 也是取消" eval '! confirm_open && [[ $(rc exists str:plain) == 1 ]]'
key d; key d; sleep 0.8; key y; sleep 1.2
check "y 删除：服务端上 str:plain 没了，toast「已删除键 str:plain」；树里 str 的尾注 6、plain 不在了" eval '[[ $(rc exists str:plain) == 0 && $(toast) == *"已删除键 str:plain"* ]] && tree | grep -qx "▾ str 6" && ! tree | grep -qx "plain"' || { toast; tree; }
sleep 3.5; menu_pick '^ttl$' 1; key y; sleep 1.2
check "右键「删除…」（第一项）、y：str:ttl 没了，尾注 5" eval '[[ $(rc exists str:ttl) == 0 ]] && tree | grep -qx "▾ str 5"' || { toast; tree; }
check "别的键没动（str:number 还在、db0 一共 27 个）" eval '[[ $(rc exists str:number) == 1 && $(rc dbsize) == 27 ]]'
quit_app; _e2e_kill
reseed; DIR=$D/n; sstart rd
expand str; to_row '^plain$'; key d; key d; sleep 1.2
check "confirm_dangerous = false：dd 直接删，不问，toast「已删除键 str:plain」" eval '! confirm_open && [[ $(rc exists str:plain) == 0 && $(toast) == *"已删除键 str:plain"* ]]' || { toast; }
quit_app; _e2e_kill; DIR=

# ---- 改名
reseed; sstart rd
expand str
menu_pick '^number$' 2
check "右键「改名…」：状态栏问「新名字：」，输入框里是整个键名 str:number" eval '[[ $(prompt) == "新名字：str:number" ]]' || prompt
for i in $(seq 1 12); do key BSpace; done; e2e_type "str:renamed"; key Enter; sleep 1.5
check "改成 str:renamed：服务端上旧名没了、新名在（值还是 42），toast「str:number 改名为 str:renamed」，树里 number 变成 renamed" eval '[[ $(rc exists str:number) == 0 && $(rc get str:renamed) == 42 && $(toast) == *"str:number 改名为 str:renamed"* ]] && tree | grep -qx "renamed" && ! tree | grep -qx "number"' || { toast; tree; }
menu_pick '^renamed$' 2
for i in $(seq 1 12); do key BSpace; done; e2e_type "str:json"; key Enter; sleep 1
check "改成已经有的 str:json：弹确认「str:json 已经有了，覆盖它？」，服务端上还没动" eval 'confirm_open && e2e_plain | grep -q "str:json 已经有了，覆盖它？" && [[ $(rc get str:renamed) == 42 && $(rc exists str:json) == 1 ]]' || { e2e_plain | sed -n 14,30p | cut -c30-130; }
key n; sleep 0.8
check "n 取消：两个键都在，值没变（renamed = 42，json 还是 JSON 文本）" eval '! confirm_open && [[ $(rc get str:renamed) == 42 && $(rc get str:json) == *Ann* ]]'
menu_pick '^renamed$' 2
for i in $(seq 1 12); do key BSpace; done; e2e_type "str:json"; key Enter; sleep 1; key y; sleep 1.5
check "再来一次、y 覆盖：str:json 的值是 42（renamed 的），renamed 没了，toast「…改名为 str:json」" eval '[[ $(rc get str:json) == 42 && $(rc exists str:renamed) == 0 && $(toast) == *"改名为 str:json"* ]]' || { toast; rc get str:json; }
sleep 3.5; menu_pick '^plain$' 2; key Enter; sleep 0.6
check "名字没改就 ↵：什么都不发（str:plain 还在，没有 toast 说改了）" eval '[[ $(rc exists str:plain) == 1 && $(toast) != *"改名为"* ]]' || toast
quit_app; _e2e_kill

# ---- TTL
reseed; sstart rd
expand str
menu_pick '^plain$' 3
check "右键「设 TTL…」：状态栏问「TTL（秒，或 90m；空 = 不过期）：」，输入框是空的" eval '[[ $(prompt) == "TTL（秒，或 90m；空 = 不过期）：" ]]' || prompt
e2e_type "abc"; key Enter; sleep 0.8
check "写 abc：toast「TTL 要是秒数或 90m、1h30m 这样的时长」，问题还在（输入框里还是 abc），键没动" eval '[[ $(toast) == *"TTL 要是秒数或 90m、1h30m 这样的时长"* && $(prompt) == "TTL（秒，或 90m；空 = 不过期）：abc" && $(ttl str:plain) == -1 ]]' || { toast; prompt; ttl str:plain; }
for i in 1 2 3; do key BSpace; done; e2e_type "90m"; key Enter; sleep 1.5
check "改写成 90m：服务端上 str:plain 的 TTL 是 90 分钟（5390–5400 秒）" eval 't=$(ttl str:plain); (( t >= 5390 && t <= 5400 ))' || ttl str:plain
sleep 3.5; menu_pick '^plain$' 3; e2e_type "120"; key Enter; sleep 1.5
check "写 120（秒）：TTL 在 110–120 秒" eval 't=$(ttl str:plain); (( t >= 110 && t <= 120 ))' || ttl str:plain
sleep 3.5; menu_pick '^plain$' 3; e2e_type "1h30m"; key Enter; sleep 1.5
check "写 1h30m：同样 5390–5400 秒" eval 't=$(ttl str:plain); (( t >= 5390 && t <= 5400 ))' || ttl str:plain
sleep 3.5; menu_pick '^plain$' 3; key Enter; sleep 1.5
check "什么都不写 ↵（空 = 不过期）：TTL 取消，-1" eval '[[ $(ttl str:plain) == -1 ]]' || ttl str:plain
check "别的键的 TTL 没动：str:ttl 还是十年（> 300000000 秒）" eval '(( $(ttl str:ttl) > 300000000 ))' || ttl str:ttl
quit_app; _e2e_kill

# ---- 命名空间：删除、问的数、取消、分批；glob 字符不能误伤
reseed
rc set 'we*ird:1' a >/dev/null; rc set 'we*ird:2' a >/dev/null; rc set weXird:1 a >/dev/null; rc set 'a[b]:1' a >/dev/null; rc set 'ab:1' a >/dev/null; rc set 'q?:1' a >/dev/null; rc set 'qx:1' a >/dev/null
sstart rd
menu_pick '^[▸▾] +app( |$)' 0
check "右键 app 命名空间「删除…」：弹确认「删除 6 个键？  app:*」（键 app 自己不算）" eval 'confirm_open && e2e_plain | grep -q "删除 6 个键？" && e2e_plain | grep -qF "app:*"' || e2e_plain | sed -n 14,30p | cut -c30-130
key n; sleep 0.8
check "n 取消：app 下面六个键一个没少" eval '! confirm_open && [[ $(rc --scan --pattern "app:*" | wc -l | tr -d " ") == 6 ]]' || rc --scan --pattern 'app:*' | wc -l
menu_pick '^[▸▾] +app( |$)' 0; key Escape; sleep 0.8
check "esc 也是取消" eval '! confirm_open && [[ $(rc --scan --pattern "app:*" | wc -l | tr -d " ") == 6 ]]'
menu_pick '^[▸▾] +app( |$)' 0; key y; sleep 2
check "y 删除：app:* 六个键没了，键 app 还在，toast「已删除 6 个键」，树里命名空间 app 没了、根上的键 app 还在" eval '[[ $(rc --scan --pattern "app:*" | wc -l | tr -d " ") == 0 && $(rc exists app) == 1 && $(toast) == *"已删除 6 个键"* ]] && ! tree | grep -q "▸ app" && tree | grep -qx "app"' || { toast; tree; }
sleep 3.5
y=$(nth_row '^[▸▾] +we\*ird( |$)' 1); rclick 14 "$y"; key Enter; sleep 1; key y; sleep 2
check "命名空间 we*ird（名字里有 *）：只删 we*ird:1、we*ird:2，weXird:1 还在" eval '[[ $(rc exists "we*ird:1" "we*ird:2") == 0 && $(rc exists weXird:1) == 1 ]]' || { rc --scan; }
sleep 3.5
y=$(nth_row '^[▸▾] +a\[b\]( |$)' 1); rclick 14 "$y"; key Enter; sleep 1; key y; sleep 2
check "命名空间 a[b]（名字里有 [ ]）：只删 a[b]:1，ab:1 还在" eval '[[ $(rc exists "a[b]:1") == 0 && $(rc exists ab:1) == 1 ]]' || { rc --scan; }
sleep 3.5
y=$(nth_row '^[▸▾] +q\?( |$)' 1); rclick 14 "$y"; key Enter; sleep 1; key y; sleep 2
check "命名空间 q?（名字里有 ?）：只删 q?:1，qx:1 还在" eval '[[ $(rc exists "q?:1") == 0 && $(rc exists qx:1) == 1 ]]' || { rc --scan; }
sleep 3.5
y=$(nth_row '^[▸▾] +\(空\)( |$)' 1); rclick 14 "$y"; key Enter; sleep 1; key y; sleep 2
check "最前面没有名字的命名空间（:lead 的）：删的是 :* 下面的 :lead，没别的" eval '[[ $(rc exists :lead) == 0 && $(rc exists a::b trail: nodelimiter) == 3 ]]' || { rc --scan; }
quit_app; _e2e_kill

# ---- confirm_dangerous = false：不问，直接删
reseed; DIR=$D/n; sstart rd
menu_pick '^[▸▾] +cache( |$)' 0; sleep 1.5
check "confirm_dangerous = false：命名空间 cache「删除…」直接删（没有确认框），toast「已删除 2 个键」" eval '! confirm_open && [[ $(rc --scan --pattern "cache:*" | wc -l | tr -d " ") == 0 && $(toast) == *"已删除 2 个键"* ]]' || { toast; }
quit_app; _e2e_kill; DIR=

# ---- 只读连接：删除、改名、TTL、命名空间删除都被拒，服务端上一点没动
reseed; sstart ro
sleep 0.5
check "（准备）连上了只读连接 ro：树里的节点是 ro（有锁的图标），db0 里 29 个键" eval 'tree | head -1 | grep -q "db0 29"' || tree
expand str
to_row '^plain$'; key d; key d; sleep 1
check "只读连接上 dd：toast「ro 是只读连接」，str:plain 还在" eval '[[ $(toast) == *"ro 是只读连接"* && $(rc exists str:plain) == 1 ]]' || { toast; }
sleep 3.5; menu_pick '^plain$' 2
check "只读连接上改名：toast「ro 是只读连接」，状态栏没有「新名字」" eval '[[ $(toast) == *"ro 是只读连接"* && $(prompt) != *新名字* ]]' || { toast; prompt; }
sleep 3.5; menu_pick '^plain$' 3
check "只读连接上设 TTL：toast「ro 是只读连接」，TTL 没动" eval '[[ $(toast) == *"ro 是只读连接"* && $(ttl str:plain) == -1 ]]' || { toast; }
sleep 3.5; menu_pick '^[▸▾] +app( |$)' 0
check "只读连接上删命名空间：toast「ro 是只读连接」，没有确认框，app 下面的键一个没少" eval '[[ $(toast) == *"ro 是只读连接"* ]] && ! confirm_open && [[ $(rc --scan --pattern "app:*" | wc -l | tr -d " ") == 6 ]]' || { toast; }
quit_app; _e2e_kill

# ---- 超过预算的命名空间：确认框里的数是真的（不设预算），C-c 能在找键和删键的时候停，删完 toast 数量
rc flushall >/dev/null; rc -n 8 EVAL "for i = 1, 1000000 do redis.call('SET', 'big:' .. i, i) end" 0 >/dev/null
rc -n 8 set other:key x >/dev/null
sstart big
check "（准备）db8 有 100 万个键：树只读了预算内的 10000 个，命名空间 big 的尾注 10000" eval 'tree | head -1 | grep -qE "^▾ db8 1.0m$" && tree | grep -qx "▸ big 10000"' || tree
y=$(nth_row '^[▸▾] +big +[0-9]+$' 1); rclick 14 "$y"; e2e_keys Enter; sleep 0.3; e2e_keys C-c
check "找键的时候（100 万个键要扫将近一秒）按 C-c：toast「已取消」，没有确认框，一个键没删" wait_for 3 eval '[[ $(toast) == *已取消* ]] && ! confirm_open && [[ $(rc -n 8 dbsize) == 1000001 ]]' || { toast; rc -n 8 dbsize; }
sleep 3; y=$(nth_row '^[▸▾] +big +[0-9]+$' 1); rclick 14 "$y"; e2e_keys Enter
for i in $(seq 1 60); do confirm_open && break; sleep 0.2; done
check "扫完：确认框里的数是真的，「删除 1000000 个键？」（树上只读了 10000 个）" eval 'confirm_open && e2e_plain | grep -q "删除 1000000 个键？"' || e2e_plain | sed -n 14,30p | cut -c30-130
rc client pause 3000 write >/dev/null
key y; sleep 0.8; e2e_keys C-c
check "y 之后服务端把写停了 3 秒（CLIENT PAUSE），C-c：界面不等，toast「已取消，已删除 N 个键」（N 是 500 的倍数，这里是 0）" wait_for 3 eval '[[ $(toast) == *"已取消，已删除 "* ]] && n=$(grep -oE "已删除 [0-9]+" <<<"$(toast)" | grep -oE "[0-9]+") && (( n % 500 == 0 ))' || toast
sleep 3.5
check "服务端恢复之后：被停住的那一条 UNLINK（至多 500 个）执行了，后面的批没有发出去：db8 里还剩 99 万多个键" eval 'n=$(rc -n 8 dbsize); (( n >= 999000 && n <= 1000001 ))' || rc -n 8 dbsize
after_cancel_ok() { to_row '^[▾▸] +db8( |$)'; key R; sleep 3; tree | head -1 | grep -qE "^▾ db8 " && ! toast | grep -q "："; }
check "取消之后树和连接还能用（在 db8 上 R 重扫，不报错，重新连上）" eval 'after_cancel_ok' || { toast; tree | head -3; }
sleep 3.5; y=$(nth_row '^[▸▾] +big +[0-9]+$' 1); rclick 14 "$y"; e2e_keys Enter
for i in $(seq 1 60); do confirm_open && break; sleep 0.2; done
key y
check "再删一次、y：分批 UNLINK 删光，toast「已删除 N 个键」，db8 里只剩 other:key" wait_for 10 eval '[[ $(toast) == *"已删除 "*" 个键"* && $(rc -n 8 dbsize) == 1 && $(rc -n 8 exists other:key) == 1 ]]' || { toast; rc -n 8 dbsize; }
quit_app; _e2e_kill

e2e_done
