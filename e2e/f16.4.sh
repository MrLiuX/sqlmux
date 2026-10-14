#!/usr/bin/env bash
# F16.4 Redis 控制台（specs/m16-redis/task.md F16.4；tech-design §9.8、§11「Redis 控制台」、§13）
# query tab 在 Redis 连接上是控制台：一行一条命令（# 是注释，引号照 redis-cli），↵ 跑光标行；回复按形状成表（标量 value、数组一列、map field / value、
# 等长二维数组按列、更深的嵌套是格子里的 JSON、nil 是 NULL）；整份回复在内存，按 500 行翻页，「全部」不重跑；只读连接拒绝写命令，@dangerous 走确认；
# 阻塞命令取消 = 关连接重拨；历史；SELECT n；非 SQL 连接上 gq、快速 SQL、DDL 的入口不出现。补全和签名提示在 f16.5.sh。
. "$(dirname "$0")/lib.sh"
e2e_build || exit 1
. "$(dirname "$0")/palette.sh"
. "$(dirname "$0")/redis.sh"

D=$(mktemp -d "${TMPDIR:-/tmp}/sqlmux-e2e-cfg.XXXXXX"); trap 'e2e_stop; redis_stop; rm -rf "$D"' EXIT
redis_start R || exit 1
mkdir -p "$D/c" "$D/dt"
(umask 077; cat >"$D/c/connections.toml" <<TOML
[[connection]]
name = "rd"
engine = "redis"
dsn = "redis://localhost:$RPORT/0"

[[connection]]
name = "ro"
engine = "redis"
dsn = "redis://localhost:$RPORT/0"
read_only = true
TOML
)
printf 'confirm_dangerous = true\n' >"$D/c/config.toml"
reseed() { rc flushall >/dev/null; redis_seed; }
toast() { e2e_text 1 "$(e2e_flag pane_width)" "$(( $(H) - 1 ))" | python3 -c 'import sys; print(sys.stdin.read().strip("└─┘│ \n"))'; }
quit_app() { e2e_type ":"; sleep 0.4; e2e_type qa; key Enter; wait_for 8 exited; sleep 0.5; }
sstart() { e2e_start -C "$D/c" -D "$D/dt" "$E2E_BIN $1"; wait_for 8 flag_is alternate_on 1; sleep 2; }
# runl TEXT: focus the query, replace its text with TEXT (one line), esc, ↵ runs the cursor's line
runl() { key C-l; key g g; key d G; key i; e2e_type "$1"; key Escape; key Enter; sleep 1.3; }
# the result pane (③ under the query): its title's right end ("N 行 · Xms"), its rows squeezed
rtitle() { local g; g=($(geom 3)); e2e_text $((g[0] + 1)) $((g[0] + g[2] - 2)) "${g[1]}" | python3 -c 'import re, sys; print(re.sub("[\ue000-\uf8ff] ?", "", sys.stdin.read()).strip("┌─┐│ \n"))'; }
rrows() { local g y; g=($(geom 3)); for ((y = g[1] + 1; y < g[1] + g[3] - 2; y++)); do e2e_text $((g[0] + 1)) $((g[0] + g[2] - 2)) "$y"; done | python3 -c "
import re, sys
for l in sys.stdin:
    t = re.sub(r'\\s+', ' ', re.sub('[\ue000-\uf8ff]', '', l.replace('│', ' ').replace('┼', ' ')).strip())
    if not t or set(t) <= set('─ '): continue
    print(t)"; }   # the result tab's table: header, rows, squeezed (the pane's last row is its tab bar)
rows_are() { local got; got=$(rrows); [[ $got == "$1" ]] || { echo "  rows:"; echo "$got" | sed 's/^/    /'; false; }; }
logline() { local g y; g=($(geom 3)); for ((y = g[1] + 1; y < g[1] + g[3] - 1; y++)); do e2e_text $((g[0] + 1)) $((g[0] + g[2] - 2)) "$y"; done | sed 's/ *│ *$//; s/^ *│ *//' | grep -a "$1" | tail -1; }
confirm_open() { e2e_plain | grep -q "y 执行   n 取消"; }

# ---- 回复的形状
reseed; rc set cnt0 0 >/dev/null; sstart rd
runl "SET foo bar"
check "SET foo bar：标量回复，一列 value、一行 OK，标题 1 行" eval 'rows_are "value
1 OK" && [[ $(rtitle) == *"1 行"* ]] && [[ $(rc get foo) == bar ]]' || { rrows; rtitle; }
runl "GET foo"
check "GET foo：value / bar" eval 'rows_are "value
1 bar"' || rrows
runl "GET nokey"
check "GET 一个不存在的键：nil 是 NULL（<null>）" eval 'rows_are "value
1 <null>"' || rrows
runl "ZRANGE zset:rank 0 -1 WITHSCORES"
check "ZRANGE … WITHSCORES：成对的数组按列：member / score，五行（1 alice 1 … 5 erin 10）" eval 'rows_are "member score
1 alice 1
2 bob 2.5
3 carol 3
4 dave 3
5 erin 10" && [[ $(rtitle) == *"5 行"* ]]' || rrows
runl "HGETALL hash:user"
check "HGETALL（RESP3 的 map）：两列 field / value，四行" eval 'rows_are "field value
1 age 30
2 email ann@example.com
3 meta {\"vip\":true}
4 name Ann"' || rrows
runl "LRANGE list:queue 0 -1"
check "LRANGE 0 -1：一列 value，五行 first … fifth" eval 'rows_are "value
1 first
2 second
3 third
4 fourth
5 fifth"' || rrows
runl "XRANGE stream:events - +"
check "XRANGE：等长二维数组按列（1、2），第 2 列是格子里的 JSON（[\"type\",\"login\",\"user\",\"ann\"]）" eval 'rrows | sed -n 2p | grep -qF "1 1700000000000-1 [\"type\",\"login\",\"user\",\"ann\"]" && [[ $(rrows | wc -l | tr -d " ") == 4 ]]' || rrows
runl "EXISTS str:plain nokey"
check "整数回复：EXISTS str:plain nokey → value / 1" eval 'rows_are "value
1 1"' || rrows
runl "EVAL \"return {1,{2,3},'x'}\" 0"
check "更深的嵌套：数组 {1,{2,3},'x'} 一列，第二行是格子里的 JSON [2,3]" eval 'rows_are "value
1 1
2 [2,3]
3 x"' || rrows
runl "LPUSH str:plain x"
check "错误回复：对字符串键 LPUSH 报 WRONGTYPE，日志里是服务端的原话（Operation against a key holding the wrong kind of value），错误栏是 [WRONGTYPE] …，不当成表" eval '[[ -n $(logline "wrong kind of value") ]] && e2e_plain | grep -q "\[WRONGTYPE\]"' || { rtitle; rrows; }
runl "NOSUCH x"
check "不认识的命令：日志里是 unknown command 'NOSUCH'" eval '[[ -n $(logline "unknown command") ]]' || rrows

# ---- 引号、注释、多行：一行一条命令，↵ 跑光标那一行
runl "SET \"k 1\" \"a \\\"q\\\" b\""
check "双引号带转义：SET \"k 1\" \"a \\\"q\\\" b\" → 键 k 1，值 a \"q\" b" eval '[[ $(rc get "k 1") == "a \"q\" b" ]]' || rc get "k 1"
runl "SET 'k 2' 'single quoted'"
check "单引号：SET 'k 2' 'single quoted'" eval '[[ $(rc get "k 2") == "single quoted" ]]' || rc get "k 2"
runl "SET k3 \"tab\\there\\x41\""
check "\"…\" 里的 \t 和 \x41 照 redis-cli 转义：值是 tab、here、A（十六进制 746162 09 68657265 41）" eval '[[ $(rc get k3 | tr -d "\n" | xxd -p) == 74616209686572654* ]]' || rc get k3 | xxd -p
runl "SET k5 \"abc\"def"
check "引号后有字符（\"abc\"def）：整行不发，日志写「引号后有字符」（k5 没有）" eval '[[ $(rc exists k5) == 0 && -n $(logline "引号后有字符") ]]' || logline SET
runl "SET k4 \"abc"
check "没闭合的引号（§9.8：整行报「引号没有闭合」，这一行不发）：k4 没有，日志写「引号没有闭合」" eval '[[ $(rc exists k4) == 0 && -n $(logline "引号没有闭合") ]]' || { rc get k4; logline SET; }
key C-l; key g g; key d G; key i; e2e_type "SET m1 a"; key Enter; e2e_type "# a comment line"; key Enter; e2e_type "SET m2 b"; key Enter; e2e_type "SET m3 c"; key Escape; key g g; key j; key j; key Enter; sleep 1.3
check "三行里光标在第 3 行（SET m2 b，中间隔着一行 # 注释）按 ↵：只跑这一行：m2 = b，m1、m3 没有" eval '[[ $(rc get m2) == b && $(rc exists m1 m3) == 0 ]]' || { rc exists m1 m2 m3; }
key g g; key Enter; sleep 1.3
check "光标移到第 1 行 ↵：m1 = a（# 注释行本身不是命令）" eval '[[ $(rc get m1) == a ]]' || rc get m1
key g g; key V; key G; key Enter; sleep 2
check "选中全部（ggVG）↵：每一行按顺序都跑，m3 = c" eval '[[ $(rc get m3) == c ]]' || { rc exists m1 m2 m3; rrows; }
quit_app; _e2e_kill

# ---- 高亮：命令名（服务端 COMMAND 里有的）是 keyword 色，字符串是 sql_string 色，认不得的名字是普通色，# 开头的行是注释色
reseed; sstart rd
key C-l; key i; e2e_type 'GET str:plain "x y"'; key Enter; e2e_type 'NOSUCH a'; key Enter; e2e_type '# a note'; key Escape; sleep 0.5
gx=$(e2e_find GET 2 | cut -d' ' -f1); sx=$(e2e_find '"x y"' 2 | cut -d' ' -f1); nx=$(e2e_find NOSUCH 3 | cut -d' ' -f1); cx=$(e2e_find '# a note' 4 | cut -d' ' -f1)
check "命令名 GET 是 keyword 色（#bb9af7），\"x y\" 的引号字符串是 #9ece6a，认不得的 NOSUCH 是普通前景 #c0caf5，# 开头的整行是注释色 #565f89" eval '[[ $(e2e_style $gx 2) == fg=#bb9af7* && $(e2e_style $((sx + 1)) 2) == fg=#9ece6a* && $(e2e_style $nx 3) == fg=#c0caf5* && $(e2e_style $((cx + 3)) 4) == fg=#565f89* ]]' || { e2e_style $gx 2; e2e_style $((sx+1)) 2; e2e_style $nx 3; e2e_style $((cx+3)) 4; }
quit_app; _e2e_kill

# ---- 整份回复在内存：500 行一页，翻页和「全部」不重跑命令（EVAL 里 INCR 一个计数验证）
reseed; rc EVAL "for i=1,1200 do redis.call('RPUSH','bl','e'..i) end" 0 >/dev/null; sstart rd
runl "EVAL \"redis.call('INCR','cnt'); return redis.call('LRANGE','bl',0,-1)\" 0"
check "回复 1200 个元素：结果 tab 标题是「500+ 行」，第一页从 e1 起；命令只跑了一次（cnt = 1）" eval '[[ $(rtitle) == *"500+ 行"* && $(rrows | sed -n 2p) == "1 e1" && $(rc get cnt) == 1 ]]' || { rtitle; rrows | head -3; rc get cnt; }
key C-j; sleep 0.4; key ']'; sleep 1.2
check "] 翻到下一页：标题是 501–1000+ 行，页内行号重新从 1 起，第一行是 e501；命令没有重跑（cnt 还是 1）" eval '[[ $(rtitle) == *"501–1000+ 行"* && $(rrows | sed -n 2p) == "1 e501" && $(rc get cnt) == 1 ]]' || { rtitle; rrows | head -3; rc get cnt; }
key ']'; sleep 1.2
check "再翻一页：最后一页（1001–1200）从 e1001 起；命令还是只跑过一次" eval '[[ $(rtitle) == *"1001–1200"* && $(rrows | sed -n 2p) == "1 e1001" && $(rc get cnt) == 1 ]]' || { rtitle; rrows | head -3; rc get cnt; }
key '['; sleep 1.2
check "[ 回到第 2 页（e501 起），cnt 仍是 1" eval '[[ $(rrows | sed -n 2p) == "1 e501" && $(rc get cnt) == 1 ]]' || { rrows | head -3; rc get cnt; }
quit_app; _e2e_kill

# ---- 只读连接：写命令（有 write 标记的、EVAL、CONFIG SET 这类）整次不执行，读命令照跑
reseed; sstart ro
runl "SET foo 1"
check "只读连接 ro：SET foo 1 整次不执行，日志写「ro 是只读连接，整次都没有执行」，库里没有 foo" eval '[[ -n $(logline "ro 是只读连接，整次都没有执行") && $(rc exists foo) == 0 ]]' || logline SET
runl "GET str:plain"
check "读命令 GET 照跑：value / hello" eval 'rows_are "value
1 hello"' || rrows
runl "EVAL \"return redis.call(\\\"set\\\", \\\"viaeval\\\", 1)\" 0"
check "EVAL 没有 write 标记也被拦（服务端的脚本能写）：整次不执行，库里没有 viaeval" eval '[[ -n $(logline "ro 是只读连接，整次都没有执行") && $(rc exists viaeval) == 0 ]]' || logline EVAL
runl "CONFIG SET maxmemory-policy allkeys-lru"
check "CONFIG SET（@admin）：整次不执行，服务端的 maxmemory-policy 没变" eval '[[ $(rc config get maxmemory-policy | sed -n 2p) != allkeys-lru ]]' || rc config get maxmemory-policy
runl "FLUSHALL"
check "FLUSHALL：只读连接上直接拒，没有确认框，库还在（29 个键）" eval '! confirm_open && [[ $(rc dbsize) == 29 ]]' || { rc dbsize; }
quit_app; _e2e_kill

# ---- @dangerous 走确认（confirm_dangerous）：FLUSHALL、KEYS
reseed; sstart rd
runl "FLUSHALL"
check "FLUSHALL：弹确认「这次执行有 1 条危险语句：FLUSHALL」，库里还没动" eval 'confirm_open && e2e_plain | grep -q "这次执行有 1 条危险语句" && e2e_plain | grep -q "FLUSHALL" && [[ $(rc dbsize) == 29 ]]' || e2e_plain | sed -n 14,30p | cut -c30-130
key n; sleep 0.6
check "n 取消：库里 29 个键都在" eval '! confirm_open && [[ $(rc dbsize) == 29 ]]'
runl "KEYS str:*"
check "KEYS（服务端的 @dangerous 名单里有它）：也要确认" eval 'confirm_open && e2e_plain | grep -q "KEYS str:\*"' || e2e_plain | sed -n 14,30p | cut -c30-130
key y; sleep 1.3
check "y 执行：回复是匹配的 7 个键（一列 value）" eval '[[ $(rrows | wc -l | tr -d " ") == 8 && $(rrows | sed -n 1p) == value ]]' || rrows
runl "FLUSHALL"; key y; sleep 1.3
check "FLUSHALL 确认 y：库清空（dbsize 0）" eval '[[ $(rc dbsize) == 0 ]]' || rc dbsize
quit_app; _e2e_kill

# ---- 阻塞命令：BLPOP 不带超时，C-c 取消 = 关连接重拨，之后连接还能用
reseed; sstart rd
key C-l; key g g; key d G; key i; e2e_type "BLPOP nolist 0"; key Escape; key Enter; sleep 1
check "BLPOP nolist 0：在等（状态栏 busy）" eval '[[ $(bar) == *busy* ]]' || bar
key C-c; sleep 0.8
check "C-c：2 秒内回来了（不 busy），日志里是已取消" eval 'wait_for 2 eval "[[ \$(bar) != *busy* ]]"' || bar
runl "GET str:plain"
check "取消之后下一条命令照跑（连接重拨了）：GET str:plain → hello" eval 'rows_are "value
1 hello"' || rrows
check "服务端上没有挂着的 BLPOP 客户端（旧连接被关了）" eval '[[ -z $(rc client list | grep "cmd=blpop") ]]' || rc client list | grep blpop
quit_app; _e2e_kill

# ---- 控制台里的 SELECT 只改这条连接的 db；标题的 db 段下拉换子连接；历史带 db
reseed; rc -n 3 set marker3 three >/dev/null; sstart rd
runl "GET marker3"
check "db0 里没有 marker3：GET marker3 → <null>" eval 'rows_are "value
1 <null>"' || rrows
runl "SELECT 3"; runl "GET marker3"
check "SELECT 3 之后 GET marker3 → three（只改了这条 Main 的 db）；query 的标题还是 db0" eval 'rows_are "value
1 three" && e2e_plain | sed -n 1p | grep -q "rd · db0"' || { rrows; e2e_plain | sed -n 1p | cut -c98-165; }
key C-l; key g s; sleep 0.6; e2e_type db3; sleep 0.3; key Enter; sleep 1.2
check "gs 选 db3：标题的 db 段变成 db3（换成 db3 的子连接）" eval 'e2e_plain | sed -n 1p | grep -q "rd · db3"' || e2e_plain | sed -n 1p | cut -c98-165
runl "GET marker3"
check "在 db3 的子连接上 GET marker3 → three" eval 'rows_are "value
1 three"' || rrows
quit_app
check "历史：history.jsonl 里记着跑过的命令（GET marker3、SELECT 3），db3 子连接上跑的那条带 \"db\":\"3\"，db0 的不带 db（默认库不写）" eval 'f=$(ls "$D/dt"/sqlmux/history.jsonl) && grep -q "\"sql\":\"SELECT 3\"" "$f" && [[ $(grep -c "\"sql\":\"GET marker3\"" "$f") == 3 ]] && [[ $(grep -c "\"db\":\"3\"" "$f") == 1 ]]' || { tail -4 "$D/dt"/sqlmux/history.jsonl; }
_e2e_kill

# ---- 非 SQL 连接上：gq 无事、格式化和（对象）DDL 的命令不列、快速 SQL 说没有
reseed; sstart rd
key C-l; key i; e2e_type "  get   str:plain  "; key Escape; sleep 0.3; key g q; sleep 0.6
check "gq：什么都没发生（文字原样，没有格式化成别的样子）" eval 'e2e_plain | sed -n 2p | grep -q "▶  1   get   str:plain"' || e2e_plain | sed -n 2p
pal ">格式化"; sleep 0.5
check "命令面板里没有「格式化整个 query」" eval '! e2e_plain | grep -q "格式化整个 query"' || true
key Escape; sleep 0.3
pal ">快速"; sleep 0.5
check "命令面板 > 里没有「快速 SQL」（非 SQL 连接上面板里不列，task F16.4 f）" eval '! e2e_plain | grep -q "palette.sql"' || e2e_plain | grep -a "快速"
key Escape; sleep 0.3
pal ";select 1"; key Enter; sleep 1
check "面板的 SQL（快速 SQL）范围里执行：toast「这个连接没有快速 SQL」" eval '[[ $(toast) == *"这个连接没有快速 SQL"* ]]' || toast
key Escape; sleep 0.3
quit_app; _e2e_kill

# ---- 带密码的行（AUTH、CONFIG SET requirepass）不进历史
reseed; sstart rd
runl "AUTH hunter2"
runl "CONFIG SET requirepass \"\""; confirm_open && { key y; sleep 1; }
runl "GET str:plain"
quit_app
check "AUTH hunter2 和 CONFIG SET requirepass 的行不在 history.jsonl 里（没有 hunter2、没有 requirepass），GET str:plain 在" eval 'f=$(ls "$D/dt"/sqlmux/history.jsonl) && ! grep -q "hunter2" "$f" && ! grep -qi "requirepass" "$f" && grep -q "GET str:plain" "$f"' || { tail -5 "$D/dt"/sqlmux/history.jsonl; }
_e2e_kill

e2e_done
