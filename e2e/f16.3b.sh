#!/usr/bin/env bash
# F16.3 键 tab（二）：信息行上的操作、R 刷新、别处改了键、过滤、HSCAN、JSON / DDL 视图、layout 恢复、只读、WATCH 冲突（specs/m16-redis/task.md F16.3；tech-design §8.7「键 tab」）
# 各类型的打开和写回在 f16.3.sh。这里 SOLO（①占满右边），信息行放得下 RENAME / DELETE。
. "$(dirname "$0")/lib.sh"
e2e_build || exit 1
. "$(dirname "$0")/palette.sh"
. "$(dirname "$0")/redis.sh"

SOLO=1
D=$(mktemp -d "${TMPDIR:-/tmp}/sqlmux-e2e-cfg.XXXXXX"); trap 'e2e_stop; redis_stop; rm -rf "$D"' EXIT
redis_start R || exit 1
mkdir -p "$D/c" "$D/k" "$D/l" "$D/st"
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
name = "d4"
engine = "redis"
dsn = "redis://localhost:$RPORT/4"
TOML
}
for d in c k l; do (umask 077; conns >"$D/$d/connections.toml"); done
printf 'confirm_dangerous = true\n' >"$D/k/config.toml"; printf '[redis]\nload_all = 3\n' >"$D/l/config.toml"
reseed() { rc flushall >/dev/null; redis_seed; rc -n 4 EVAL "for i = 1, 6000 do redis.call('HSET', 'hash:big', 'field' .. i, 'value' .. i) end" 0 >/dev/null; }
PR=158   # the right column of the key tab's pane in SOLO
side() { e2e_plain | python3 -c 'import re, sys; [print(re.sub("[\ue000-\uf8ff] ?", "", l[1:33]).rstrip("│ ")) for l in sys.stdin.read().split("\n")]'; }
nth_row() { side | python3 -c "
import re, sys
n = int(sys.argv[2])
for i, l in enumerate(sys.stdin.read().split('\\n')):
    if re.search(sys.argv[1], l.strip()):
        n -= 1
        if n == 0: print(i + 1); break" "$1" "${2:-1}"; }
click_row() { local y; y=$(nth_row "$@"); [[ -n $y ]] || return 1; e2e_click 14 "$y"; sleep 0.5; }
expand() { click_row "^[▸▾] +$1( |\$)" "${2:-1}"; }
to_row() { local y i; y=$(nth_row "$1" "${2:-1}"); key C-h; key g g; for ((i = 4; i < y; i++)); do key j; done; }
open_key() { to_row "$1" "$2"; key Enter; sleep 1.2; }
toast() { e2e_text 1 "$(e2e_flag pane_width)" "$(( $(H) - 1 ))" | python3 -c 'import sys; print(sys.stdin.read().strip("└─┘│ \n"))'; }
prompt() { e2e_text 1 "$(e2e_flag pane_width)" "$(H)" | python3 -c 'import re, sys; print(re.split(r"\s{3,}", sys.stdin.read().strip())[0])'; }
confirm_open() { e2e_plain | grep -qE "y (删除|覆盖) +n 取消"; }
quit_app() { e2e_type ":"; sleep 0.4; e2e_type qa; key Enter; wait_for 8 exited; sleep 0.5; }
sstart() { e2e_start $SX -C "${DIR:-$D/c}" ${ST:+-S "$ST"} "$E2E_BIN $1"; wait_for 8 flag_is alternate_on 1; sleep 2; [[ -n $SOLO ]] && solo; }
set_cell() { key Enter; e2e_type "$1"; sleep 0.2; key Enter; }
saved() { wait_for 5 eval '[[ $(e2e_text 36 $PR 3) == *已保存* ]]'; }
ptitle() { e2e_plain | sed -n 1p | python3 -c "import re, sys; t = sys.stdin.read(); print(re.sub('^[\u2460-\u2473\u24ea] *', '', re.sub('[\ue000-\uf8ff] ?', '', t[34:$PR + 1]).strip('┌─┐ ')).strip(' ─'))"; }
info() { e2e_text 36 $PR 2 | python3 -c 'import re, sys; print(re.sub(r"\s+", " ", re.sub("[\ue000-\uf8ff]", "", sys.stdin.read().strip("│ \n"))))'; }
grid_rows() { local y0 y; y0=$(grid_y); [[ -n $y0 ]] || return 1; for ((y = y0 - 1; y < y0 + 30; y++)); do [[ $y == $((y0)) ]] && continue; e2e_text 35 $((PR + 1)) "$y"; done | python3 -c "
import re, sys
on = True
for l in sys.stdin:
    t = re.sub(r'\\s+', ' ', re.sub('[\ue000-\uf8ff]', '', l.replace('│', ' ')).strip())
    if not t or t.startswith('键') or 'hjkl' in t or t.startswith('1:'): on = False
    if on: print(t)"; }
rowsare() { local got; got=$(grid_rows); [[ $got == "$1" ]] || { echo "  rows:"; echo "$got" | sed 's/^/    /'; false; }; }
chip_click() { local x; x=$(e2e_find "$1" 2 | cut -d' ' -f1); [[ -n $x ]] && { e2e_click $((x + 1)) 2; sleep 0.6; }; }   # a chip of the info row: TTL, RENAME, DELETE

# ---- 信息行上的操作：TTL、RENAME（点那一格）
reseed; sstart rd
expand hash; expand str; open_key '^user$'
check "信息行：TYPE hash、LEN 4、TTL 不过期，后面有 RENAME 和 DELETE 两个可点的" eval '[[ $(info) =~ ^TYPE\ hash\ LEN\ 4\ TTL\ 不过期\ ENC\ listpack\ MEM\ [0-9.]+B\ RENAME\ DELETE$ ]]' || info
chip_click TTL
check "点 TTL：状态栏问「TTL（秒，或 90m；空 = 不过期）：」" eval '[[ $(prompt) == "TTL（秒，或 90m；空 = 不过期）：" ]]' || prompt
e2e_type 90m; key Enter; sleep 1.5
check "写 90m：服务端上 hash:user 的 TTL 是 5390–5400 秒，信息行的 TTL 是 1h29m5x s 这样的人话" eval 't=$(rc ttl hash:user); (( t >= 5390 && t <= 5400 )) && [[ $(info) =~ TTL\ 1h29m5[0-9]s\ ENC || $(info) =~ TTL\ 1h30m00s\ ENC ]]' || { rc ttl hash:user; info; }
chip_click TTL; key Enter; sleep 1.5
check "再点 TTL、什么都不写 ↵：TTL 取消，信息行回到「不过期」" eval '[[ $(rc ttl hash:user) == -1 && $(info) =~ TTL\ 不过期\ ENC ]]' || { rc ttl hash:user; info; }
chip_click RENAME
check "点 RENAME：状态栏问「新名字：hash:user」" eval '[[ $(prompt) == "新名字：hash:user" ]]' || prompt
for i in $(seq 1 9); do key BSpace; done; e2e_type "hash:renamed"; key Enter; sleep 1.5
check "改成 hash:renamed：服务端上旧名没了、新名在；tab 的标题跟着变成 hash:renamed；树里 user 变成 renamed" eval '[[ $(rc exists hash:user) == 0 && $(rc hget hash:renamed age) == 30 && $(ptitle) == "hash:renamed" ]] && side | grep -qx " *renamed"' || { ptitle; side | sed -n 8,16p; }
check "改名之后表格和信息行还在（同一个键的内容：四行）" eval 'rowsare "field value
1 age 30
2 email ann@example.com
3 meta {\"vip\":true}
4 name Ann"' || true
sleep 3; chip_click RENAME
for i in $(seq 1 12); do key BSpace; done; e2e_type "str:plain"; key Enter; sleep 1
check "改成已经有的 str:plain：弹确认「str:plain 已经有了，覆盖它？」" eval 'confirm_open && e2e_plain | grep -q "str:plain 已经有了，覆盖它？"' || e2e_plain | sed -n 14,30p | cut -c30-130
key y; sleep 1.5
check "y 覆盖：str:plain 现在是 hash（TYPE hash），hash:renamed 没了；tab 标题 str:plain" eval '[[ $(rc type str:plain) == hash && $(rc exists hash:renamed) == 0 && $(ptitle) == "str:plain" ]]' || { rc type str:plain; ptitle; }
quit_app; _e2e_kill

# ---- DELETE（confirm_dangerous 时先问）
reseed; DIR=$D/k; sstart rd
expand list; open_key '^queue$'
chip_click DELETE
check "点 DELETE：弹确认「删除键 list:queue？」，服务端上还没动" eval 'confirm_open && e2e_plain | grep -q "删除键 list:queue？" && [[ $(rc exists list:queue) == 1 ]]' || e2e_plain | sed -n 14,30p | cut -c30-130
key n; sleep 0.6
check "n 取消：键还在" eval '! confirm_open && [[ $(rc exists list:queue) == 1 ]]'
chip_click DELETE; sleep 0.5; key y; sleep 1.5
check "y 删除：服务端上 list:queue 没了，toast「已删除键 list:queue」，树里 list 下面只剩 dups" eval '[[ $(rc exists list:queue) == 0 && $(toast) == *"已删除键 list:queue"* ]] && ! side | grep -qx " *queue"' || { toast; side | sed -n 8,18p; }
quit_app; _e2e_kill; DIR=

# ---- R 刷新整行（外面改了键）、键没了、类型变了
reseed; sstart rd
expand list; expand hash; open_key '^queue$'
rc rpush list:queue sixth >/dev/null; rc expire list:queue 725 >/dev/null
key R; sleep 1.5
check "外面 RPUSH 了一个、设了 725 秒的 TTL：R 之后 LEN 6、TTL 是 12m0x s、表格 6 行" eval '[[ $(info) =~ LEN\ 6\ TTL\ 12m0[0-9]s\ ENC ]] && rowsare "index value
1 0 first
2 1 second
3 2 third
4 3 fourth
5 4 fifth
6 5 sixth"' || { info; grid_rows; }
open_key '^user$'
M0=$(info | grep -oE "MEM [0-9.]+[KMG]?B")
rc hset hash:user big "$(python3 -c 'print("x" * 3000)')" >/dev/null
key R; sleep 1.5
check "外面往 hash:user 里塞了一个 3000 字节的值：R 之后 MEM 变大了（原来 $M0，现在 KB 级），LEN 5" eval '[[ $(info) =~ LEN\ 5 && $(info) =~ MEM\ [0-9.]+KB && $(info | grep -oE "MEM [0-9.]+[KMG]?B") != "$M0" ]]' || info
rc del hash:user >/dev/null; key R; sleep 1.5
check "外面把键删了、R：错误栏「键不存在」，表格留着上次的页" eval 'e2e_plain | grep -q "键不存在"' || e2e_plain | tail -8 | cut -c34-120
rc set hash:user plain >/dev/null; key R; sleep 1.5
check "外面把它换成了字符串：R 之后 TYPE string、LEN 5，表头成了 value、一行 plain" eval '[[ $(info) =~ ^TYPE\ string\ LEN\ 5 ]] && rowsare "value
1 plain"' || { info; grid_rows; }
quit_app; _e2e_kill

# ---- MATCH 过滤（≤ load_all 的整取、本地过滤）
reseed; sstart rd
expand hash; expand set; expand zset; open_key '^user$'
key /; e2e_type "e*"; key Enter; sleep 1
check "hash:user 的 MATCH 里写 e*：只剩 field 以 e 开头的一行 email" eval 'rowsare "field value
1 email ann@example.com"' || grid_rows
key /; for i in 1 2; do key BSpace; done; e2e_type "*m*"; key Enter; sleep 1
check "改成 *m*：email、meta、name 三行（age 没有 m）" eval 'rowsare "field value
1 email ann@example.com
2 meta {\"vip\":true}
3 name Ann"' || grid_rows
open_key '^rank$'
key /; e2e_type "*o*"; key Enter; sleep 1
check "zset:rank 的 MATCH *o*：member 里有 o 的 bob、carol" eval 'rowsare "member score
1 bob 2.5
2 carol 3"' || grid_rows
open_key '^tags$'
key /; e2e_type "r*"; key Enter; sleep 1
check "set:tags 的 MATCH r*：只有 red" eval 'rowsare "member
1 red"' || grid_rows
quit_app; _e2e_kill

# ---- 超过 load_all（默认 5000）的 hash：HSCAN，不整取；页脚的总页数；MATCH 走 HSCAN … MATCH；表头不能排序
reseed
rc monitor >"$D/mon.txt" 2>&1 & MON=$!; sleep 0.5
sstart d4
expand hash; open_key '^big$'
check "hash:big（6000 个 field）：信息行 LEN 6000，第一页（PAGE 1/60，每页 100 行，屏幕上放得下的都在）" eval '[[ $(info) =~ ^TYPE\ hash\ LEN\ 6000 ]] && e2e_text 36 $PR 4 | grep -q "LIMIT 100 .*PAGE 1/60" && (( $(grid_rows | wc -l) > 25 ))' || { info; e2e_text 36 $PR 4; grid_rows | wc -l; }
P1=$(grid_rows | sed -n 2p)
key ']'; sleep 1.2; key ']'; sleep 1.2
check "] ] 翻到第 3 页（PAGE 3/60），行号从 201 起，内容和第 1 页不同" eval 'e2e_text 36 $PR 4 | grep -q "PAGE 3/60" && [[ $(grid_rows | sed -n 2p) == 201\ * && $(grid_rows | sed -n 2p) != "$P1" ]]' || { e2e_text 36 $PR 4; grid_rows | sed -n 2p; }
key '['; sleep 1.2
check "[ 回到第 2 页（游标记着每一页的开头）" eval 'e2e_text 36 $PR 4 | grep -q "PAGE 2/60" && [[ $(grid_rows | sed -n 2p) == 101\ * ]]' || e2e_text 36 $PR 4
key /; e2e_type "e*"; sleep 0.5
check "（F18.10）/ 后输入 e*：MATCH 那一行（信息行下面一行）有 e*，光标在它后面（同一行），芯片行（LIMIT / PAGE）整行还在、没被输入框盖住" eval 'x=$(e2e_find "e*" 3 | cut -d" " -f1); [[ -n $x && $(e2e_flag cursor_y) == 2 && $(e2e_flag cursor_x) == $((x + 1)) ]] && e2e_text 36 $PR 4 | grep -qE "LIMIT 100 .*PAGE [0-9]+/60" || { e2e_text 36 $PR 3; e2e_text 36 $PR 4; echo "  cursor $(e2e_flag cursor_x),$(e2e_flag cursor_y)"; false; }'
key BSpace; key BSpace; sleep 0.2
e2e_type "field12*"; key Enter; sleep 1.5
check "MATCH field12*：走 HSCAN … MATCH，页脚是 PAGE 1/?（总数不知道），每行的 field 都以 field12 开头" eval 'e2e_text 36 $PR 4 | grep -q "PAGE 1/?" && [[ -z $(grid_rows | sed -n "2,\$p" | awk "\$2 !~ /^field12/") ]] && (( $(grid_rows | wc -l) > 5 ))' || { e2e_text 36 $PR 4; grid_rows | head -5; }
kill $MON 2>/dev/null; wait $MON 2>/dev/null
check "服务端上看到的是 HSCAN（带 MATCH field12* 和 COUNT），没有 HGETALL" eval 'grep -qi "\"hscan\" \"hash:big\"" "$D/mon.txt" && grep -qi "\"match\" \"field12\\*\"" "$D/mon.txt" && ! grep -qi "\"hgetall\" \"hash:big\"" "$D/mon.txt"' || grep -i "hash:big" "$D/mon.txt" | head -4
key g s; sleep 0.5
check "HSCAN 的页上按列排序（gs）：不排（Sortable 为空），toast 说清楚" eval '[[ -n $(toast) ]]' || true
quit_app; _e2e_kill

# ---- 小的键强制走 HSCAN（[redis] load_all = 3）：一页超过 LIMIT 时只给 LIMIT 行，PAGE 能翻
reseed; DIR=$D/l; sstart rd
expand hash; open_key '^user$'
key g l; sleep 0.3; e2e_type 2; key Enter; sleep 1.5
check "hash:user 有 4 个 field、load_all 是 3 所以走 HSCAN；LIMIT 设成 2：第一页是 2 行（不是 4 行）" eval '[[ $(grid_rows | wc -l | tr -d " ") == 3 ]]' || grid_rows
key ']'; sleep 1.5
check "] 翻到第 2 页：行号 3、4 的两个 field（两页合起来是四个 field 不重不漏）" eval '[[ $(grid_rows | sed -n 2p) == 3\ * && $(grid_rows | wc -l | tr -d " ") == 3 ]]' || grid_rows
quit_app; _e2e_kill; DIR=

# 页的边界（load_all = 3、4 个 field）：LIMIT 1 / 3 / 4，每个 field 只出现一次，最后一页没有下一页；MATCH 加翻页；DDL 视图只列 load_all 项
reseed; DIR=$D/l; sstart rd
expand hash; open_key '^user$'
fields_over_pages() { local seen= rows prev= i; for i in 1 2 3 4 5 6 7 8; do rows=$(grid_rows | sed 1d | awk '{ print $2 }' | tr '\n' ' '); [[ $rows == "$prev" ]] && break; seen+=" $rows"; prev=$rows; key ']'; sleep 1.2; done; echo $seen | tr ' ' '\n' | sort | uniq -c | awk '{ print $2 ":" $1 }' | paste -sd' ' -; }   # every field of every page from here on, counted: pages go till one more ] changes nothing
key g l; sleep 0.3; e2e_type 3; key Enter; sleep 1.5
check "LIMIT 3（页在一批中间截断）：第一页 3 行，翻到底四个 field（age email meta name）各出现一次，最后一页是 PAGE 2/2" eval '[[ $(fields_over_pages) == "age:1 email:1 meta:1 name:1" ]] && e2e_text 36 $PR 4 | grep -q "PAGE 2/2"' || { fields_over_pages; e2e_text 36 $PR 4; }
key g l; sleep 0.3; e2e_type 4; key Enter; sleep 1.5
check "LIMIT 4（页正好等于一批）：一页 4 行，PAGE 1/1，没有第二页" eval '[[ $(fields_over_pages) == "age:1 email:1 meta:1 name:1" ]] && e2e_text 36 $PR 4 | grep -q "PAGE 1/1"' || { fields_over_pages; e2e_text 36 $PR 4; }
key g l; sleep 0.3; e2e_type 1; key Enter; sleep 1.5
check "LIMIT 1：每页一行，PAGE 4/4 到底，四个 field 各一次" eval '[[ $(fields_over_pages) == "age:1 email:1 meta:1 name:1" ]] && e2e_text 36 $PR 4 | grep -q "PAGE 4/4"' || { fields_over_pages; e2e_text 36 $PR 4; }
key g g; key /; e2e_type "*m*"; key Enter; sleep 1.5
check "LIMIT 1 加 MATCH *m*：翻页翻出 email、meta、name 各一次（age 没有 m），页脚是 PAGE …/? 到最后一页" eval '[[ $(fields_over_pages) == "email:1 meta:1 name:1" ]]' || { fields_over_pages; e2e_text 36 $PR 4; }
key /; for i in 1 2 3; do key BSpace; done; key Enter; sleep 1
pal ">表的 DDL"; key Enter; sleep 1
check "DDL 视图（load_all = 3）：第一行 # 只列出前 3 项，一共 4 项，一条 HSET hash:user 带三对（没有被 LIMIT 截成别的）" eval 'e2e_plain | grep -q "# 只列出前 3 项，一共 4 项" && e2e_plain | grep -E "HSET hash:user" | head -1 | grep -oE "(age|email|meta|name) " | sort -u | wc -l | tr -d " " | grep -qx 3' || e2e_plain | sed -n 8,14p | cut -c34-150
key Escape; sleep 0.4
quit_app; _e2e_kill; DIR=

# ---- JSON 视图（gj）和 DDL 视图（命令面板「表的 DDL」）：每种类型是它自己的样子
reseed; sstart rd
expand hash; expand list; expand set; expand zset; expand stream; expand str
json_text() { e2e_plain | python3 -c "
import re, sys
out = []
for l in sys.stdin:
    t = re.sub('[\ue000-\uf8ff]', '', l)[35:$PR + 1].rstrip('│ \n').strip()
    m = re.match(r'^\d+ (.*)\$', t)
    if m: out.append(m.group(1))
print(' '.join(out))"; }
open_key '^user$'; key g j; sleep 1
check "hash 的 JSON 视图：一个 object（field → value，值都是字符串）" eval '[[ $(json_text) == "{ \"name\": \"Ann\", \"age\": \"30\", \"email\": \"ann@example.com\", \"meta\": \"{\\\"vip\\\":true}\" }" || $(json_text) == *"\"age\": \"30\""* && $(json_text) == "{"*"}" ]]' || json_text
key g j; sleep 0.5
open_key '^queue$'; key g j; sleep 1
check "list 的 JSON 视图：一个数组 [first, second, third, fourth, fifth]" eval '[[ $(json_text | tr -d " ") == "[\"first\",\"second\",\"third\",\"fourth\",\"fifth\"]" ]]' || json_text
key g j; sleep 0.5
open_key '^rank$'; key g j; sleep 1
check "zset 的 JSON 视图：[member, score] 对的数组" eval '[[ $(json_text | tr -d " ") == "[[\"alice\",1],[\"bob\",2.5],[\"carol\",3],[\"dave\",3],[\"erin\",10]]" ]]' || json_text
key g j; sleep 0.5
open_key '^events$'; key g j; sleep 1
check "stream 的 JSON 视图：{id, fields} 的数组（三条，第一条 id 1700000000000-1）" eval '[[ $(json_text) == *"1700000000000-1"* && $(json_text) == *"\"fields\""* && $(json_text | grep -o "\"id\"" | wc -l | tr -d " ") == 3 ]]' || json_text
key g j; sleep 0.5
ddl_text() { e2e_plain | python3 -c "
import re, sys
out = []
for l in sys.stdin:
    m = re.search(r'│ ([A-Z]{2,} [^│]*?) *│', re.sub('[\ue000-\uf8ff]', '', l))
    if m: out.append(m.group(1).strip())
print('\n'.join(out))"; }
open_key '^queue$'; pal ">表的 DDL"; key Enter; sleep 1
check "list 的 DDL 视图：重建它的命令 RPUSH list:queue first second third fourth fifth" eval 'ddl_text | grep -qx "RPUSH list:queue first second third fourth fifth"' || ddl_text
key Escape; sleep 0.4
open_key '^rank$'; pal ">表的 DDL"; key Enter; sleep 1
check "zset 的 DDL 视图：ZADD zset:rank 1 alice 2.5 bob 3 carol 3 dave 10 erin" eval 'ddl_text | grep -qx "ZADD zset:rank 1 alice 2.5 bob 3 carol 3 dave 10 erin"' || ddl_text
key Escape; sleep 0.4
open_key '^ttl$'; pal ">表的 DDL"; key Enter; sleep 1
check "带 TTL 的 string 的 DDL 视图：SET str:ttl expires，最后一行 PEXPIRE str:ttl <毫秒>" eval 'ddl_text | grep -qx "SET str:ttl expires" && ddl_text | tail -1 | grep -qE "^PEXPIRE str:ttl [0-9]+$"' || ddl_text
key Escape; sleep 0.4
quit_app; _e2e_kill

# ---- layout：重启后回到打开着的键 tab；WATCH 冲突；MULTI；只读；不支持的操作
reseed; ST=$D/st; sstart rd
expand hash; expand list; open_key '^user$'; open_key '^queue$'
check "（准备）打开了 hash:user 和 list:queue 两个键 tab（当前是 list:queue）" eval '[[ $(ptitle) == "list:queue" ]] && e2e_plain | grep -q "hash:user"'
quit_app; _e2e_kill
check "退出后 last.toml 里记着两个 key tab：kind = \"key\"、connection = rd、key = hash:user / list:queue（db 是 0 就不写）" eval 'f=$(ls "$ST"/sqlmux/last.toml) && grep -c "kind = \"key\"" "$f" | grep -qx 2 && grep -q "key = \"hash:user\"" "$f" && grep -q "key = \"list:queue\"" "$f"' || cat "$ST/sqlmux/last.toml" | head -40
sstart
sleep 1.5
check "重启（不带连接名，按 last.toml 恢复布局）：布局回来了，当前 tab 是 list:queue，表格里是 LRANGE 的内容，信息行是 TYPE list LEN 5" eval '[[ $(ptitle) == "list:queue" && $(info) =~ ^TYPE\ list\ LEN\ 5 ]] && rowsare "index value
1 0 first
2 1 second
3 2 third
4 3 fourth
5 4 fifth"' || { ptitle; info; grid_rows; }
key g T; sleep 1
check "gT 回到另一个 tab：hash:user，内容在" eval '[[ $(ptitle) == "hash:user" ]] && grid_rows | grep -q "email ann@example.com"' || { ptitle; grid_rows; }
quit_app; _e2e_kill; ST=

# 写回走 WATCH + MULTI + EXEC；WATCH：改了格子，保存之前别人动了这个键 → 行数据已变化
reseed
rc monitor >"$D/mon2.txt" 2>&1 & MON=$!; sleep 0.5
sstart rd
expand hash; open_key '^user$'
key g g; key l; set_cell 32; key C-s; saved
check "改 age = 32、C-s：写进去了" eval '[[ $(rc hget hash:user age) == 32 ]]'
kill $MON 2>/dev/null; wait $MON 2>/dev/null
check "服务端上看到的写回：WATCH hash:user、MULTI、HSET、EXEC（一批包进事务）" eval 'grep -qi "\"watch\" \"hash:user\"" "$D/mon2.txt" && grep -qi "\"multi\"" "$D/mon2.txt" && grep -qi "\"exec\"" "$D/mon2.txt"' || grep -ai "watch\|multi\|exec" "$D/mon2.txt" | head -5
key g g; key l; set_cell 35
rc hset hash:user age 99 >/dev/null
key C-s; sleep 1.5
check "再改 age = 35，保存之前别人把 age 改成了 99：C-s 报「field = age 的行数据已变化或行不存在，已回滚」，库里还是 99（没有覆盖）" eval 'e2e_plain | grep -q "field = age 的行数据已变化或行不存在，已回滚" && [[ $(rc hget hash:user age) == 99 ]]' || { toast; rc hget hash:user age; }
quit_app; _e2e_kill

# 只读连接：改了格子 C-s 被拒、没有发 MULTI；信息行上的 TTL / RENAME / DELETE 也拒
reseed
rc monitor >"$D/mon3.txt" 2>&1 & MON=$!; sleep 0.5
sstart ro
expand hash; open_key '^user$'
key g g; key l; set_cell 41; key C-s; sleep 1.2
check "只读连接 ro 上改格子、C-s：toast 说是只读连接，库里 age 还是 30" eval '[[ $(toast) == *只读* && $(rc hget hash:user age) == 30 ]]' || { toast; rc hget hash:user age; }
sleep 3; chip_click TTL; sleep 0.4
check "点 TTL：toast「ro 是只读连接」，没有问 TTL 的输入框" eval '[[ $(toast) == *"ro 是只读连接"* && $(prompt) != TTL* ]]' || { toast; prompt; }
sleep 3; chip_click RENAME; sleep 0.4
check "点 RENAME：同样被拒，状态栏没有「新名字」" eval '[[ $(toast) == *"ro 是只读连接"* && $(prompt) != *新名字* ]]' || { toast; prompt; }
sleep 3; chip_click DELETE; sleep 0.4
check "点 DELETE：同样被拒，没有确认框，键还在" eval '[[ $(toast) == *"ro 是只读连接"* ]] && ! confirm_open && [[ $(rc exists hash:user) == 1 ]]' || { toast; }
kill $MON 2>/dev/null; wait $MON 2>/dev/null
check "整个过程服务端上没见过 MULTI（只读在应用里拒了，没往服务端发）" eval '! grep -qi "\"multi\"" "$D/mon3.txt"' || grep -ai multi "$D/mon3.txt" | head -3
quit_app; _e2e_kill

# 键 tab 上不支持的操作：* 按值过滤、y i 复制为 INSERT
reseed; sstart rd
expand hash; open_key '^user$'
key g g; key l; key "*"; sleep 0.5
check "键 tab 上 *（按值过滤）：toast「这一列的类型不支持按值过滤」" eval '[[ $(toast) == *不支持按值过滤* ]]' || toast
sleep 3; key y; sleep 0.3; key i; sleep 0.5
check "键 tab 上 y i（复制为 INSERT）：toast「只有表里的行能复制为 INSERT」" eval '[[ $(toast) == *只有表里的行能复制为* ]]' || toast
quit_app; _e2e_kill

e2e_done
