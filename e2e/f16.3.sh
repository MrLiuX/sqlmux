#!/usr/bin/env bash
# F16.3 键 tab（一）：各类型的键怎么打开、信息行、写回（specs/m16-redis/task.md F16.3；tech-design §5「KeyTab」、§8.7「键 tab」）
# 树里 ↵ 打开一个键：标题是键名，查询条上方一行信息行（TYPE LEN TTL ENC MEM），表格按类型出列（string value、hash field/value、list index/value、
# set member、zset member/score、stream id+字段）；改格子、加行、删行、改 field / member，C-s 写回（WATCH + MULTI），校验都用 redis-cli 回读。
# 信息行上的操作、过滤、HSCAN、JSON / DDL 视图、layout 恢复、只读、并发改在 f16.3b.sh。没装 RedisJSON，json 类型不测（CI 的 redis-stack 跑集成测试）。
. "$(dirname "$0")/lib.sh"
e2e_build || exit 1
. "$(dirname "$0")/palette.sh"
. "$(dirname "$0")/redis.sh"

D=$(mktemp -d "${TMPDIR:-/tmp}/sqlmux-e2e-cfg.XXXXXX"); trap 'e2e_stop; redis_stop; rm -rf "$D"' EXIT
redis_start R || exit 1
mkdir -p "$D/c"
(umask 077; printf '[[connection]]\nname = "rd"\nengine = "redis"\ndsn = "redis://localhost:%s/0"\n' "$RPORT" >"$D/c/connections.toml")
reseed() { rc flushall >/dev/null; redis_seed; }
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
open_key() { to_row "$1" "$2"; key Enter; sleep 1.2; }   # ROW-REGEX [N]: the tree's cursor on the Nth row matching, ↵
toast() { e2e_text 1 "$(e2e_flag pane_width)" "$(( $(H) - 1 ))" | python3 -c 'import sys; print(sys.stdin.read().strip("└─┘│ \n"))'; }
quit_app() { e2e_type ":"; sleep 0.4; e2e_type qa; key Enter; wait_for 8 exited; sleep 0.5; }
sstart() { e2e_start $SX -C "${DIR:-$D/c}" "$E2E_BIN $1"; wait_for 8 flag_is alternate_on 1; sleep 2; }
set_cell() { key Enter; e2e_type "$1"; sleep 0.2; key Enter; }
# the key tab in ① (x 35..97): its title, the info row, the grid rows (squeezed, the line numbers kept)
ptitle() { e2e_plain | sed -n 1p | python3 -c "import re, sys; t = sys.stdin.read(); print(re.sub('^[\u2460-\u2473\u24ea] *', '', re.sub('[\ue000-\uf8ff] ?', '', t[34:97]).strip('┌─┐ ')).strip(' ─'))"; }
info() { e2e_text 36 96 2 | python3 -c 'import re, sys; print(re.sub(r"\s+", " ", re.sub("[\ue000-\uf8ff]", "", sys.stdin.read().strip("│ \n"))))'; }
grid_rows() { local y0 y; y0=$(grid_y); [[ -n $y0 ]] || return 1; for ((y = y0 - 1; y < y0 + 30; y++)); do [[ $y == $((y0)) ]] && continue; e2e_text 35 97 "$y"; done | python3 -c "
import re, sys
on = True
for l in sys.stdin:
    t = re.sub(r'\\s+', ' ', re.sub('[\ue000-\uf8ff]', '', l.replace('│', ' ')).strip())
    if not t or t.startswith('键') or 'hjkl' in t or t.startswith('1:'): on = False
    if on: print(t)"; }   # the header row, then the rows, squeezed
rowsare() { local got; got=$(grid_rows); [[ $got == "$1" ]] || { echo "  rows:"; echo "$got" | sed 's/^/    /'; false; }; }
hex() { xxd -p | tr -d '\n'; }

# ---- 打开各类型的键：标题、信息行（TYPE LEN TTL ENC MEM 都和 redis-cli 一致）、表格
info_is() { local t=$1 key=$2 len=$3 ttl=${4:-不过期}; [[ $(info) =~ ^TYPE\ $t\ LEN\ $len\ TTL\ $ttl\ ENC\ $(rc object encoding "$key")\ MEM\ [0-9.]+[KMG]?B ]] || { echo "  info: [$(info)] want $t $len $(rc object encoding "$key")"; false; }; }
reseed; sstart rd
expand str; expand hash; expand list; expand set; expand zset; expand stream
open_key '^plain$'
check "string str:plain：标题是键名，信息行 TYPE string LEN 5 TTL 不过期 ENC embstr MEM…，一列 value，一行 hello" eval '[[ $(ptitle) == "str:plain" ]] && info_is string str:plain 5 && rowsare "value
1 hello"'
open_key '^number$'
check "string str:number：一行 42；LEN 2" eval 'info_is string str:number 2 && rowsare "value
1 42"'
open_key '^unicode$'
check "string str:unicode：你好 world（LEN 是字节数 12）" eval 'info_is string str:unicode 12 && rowsare "value
1 你好 world"'
open_key '^binary$' 3
check "string str:binary（\\x00\\x01\\xff\\xfe）：格子里是占位符 <binary 4 bytes>，不是乱码" eval 'rowsare "value
1 <binary 4 bytes>"'
open_key '^ttl$'
check "string str:ttl（十年的 TTL）：信息行 TTL 是「3649d23h59m」这样的人话" eval '[[ $(info) =~ TTL\ 36[0-9][0-9]d[0-9]+h[0-9]+m\ ENC ]]' || info
open_key '^user$'
check "hash hash:user：标题 hash:user，LEN 4，表头 field value，四行按 field 排：age email meta name" eval '[[ $(ptitle) == "hash:user" ]] && info_is hash hash:user 4 && rowsare "field value
1 age 30
2 email ann@example.com
3 meta {\"vip\":true}
4 name Ann"'
open_key '^binary$'
check "hash hash:binary：二进制的值是占位符，另一个 field 正常（raw <binary 3 bytes> / text ok）" eval 'rowsare "field value
1 raw <binary 3 bytes>
2 text ok"'
open_key '^queue$'
check "list list:queue：表头 index value，index 从 0 起，顺序和 LRANGE 一致" eval 'info_is list list:queue 5 && rowsare "index value
1 0 first
2 1 second
3 2 third
4 3 fourth
5 4 fifth"'
open_key '^dups$'
check "list list:dups（x y x y x）：五行，相同的值各占一行" eval 'rowsare "index value
1 0 x
2 1 y
3 2 x
4 3 y
5 4 x"'
open_key '^tags$'
check "set set:tags：表头 member，三行 blue green red" eval 'info_is set set:tags 3 && rowsare "member
1 blue
2 green
3 red"'
open_key '^binary$' 2
check "set set:binary：二进制的成员是占位符 <binary 2 bytes>，另一个是 plain" eval 'rowsare "member
1 <binary 2 bytes>
2 plain"'
open_key '^rank$'
check "zset zset:rank：表头 member score，五行，分数 1 2.5 3 3 10" eval 'info_is zset zset:rank 5 && rowsare "member score
1 alice 1
2 bob 2.5
3 carol 3
4 dave 3
5 erin 10"'
open_key '^events$'
check "stream stream:events：表头 id 加字段的并集（target type user），缺的格子是 NULL；三行" eval 'info_is stream stream:events 3 && rowsare "id target type user
1 1700000000000-1 <null> login ann
2 1700000000000-2 button click ann
3 1700000000001-1 <null> logout ann"'
check "打开过的键都是树工作区下面的 tab（同一个 pane 里按打开的顺序：str:plain … stream:events）" eval 'e2e_plain | grep -c "str:plain\|str:number\|stream:events" | grep -qE "^[3-9]"' || e2e_plain | sed -n 38,45p | cut -c34-160
quit_app; _e2e_kill


# ---- 写回：每种类型改格子、加行、删行、改 field / member，C-s，用 redis-cli 回读（WATCH + MULTI / EXEC）
saved() { wait_for 5 eval '[[ $(e2e_text 36 96 3) == *已保存* ]]'; }   # the query bar says 已保存 N 行
hall() { rc hgetall "$1" | paste -sd' ' -; }
reseed; sstart rd
expand str; expand hash; expand list; expand set; expand zset; expand stream

open_key '^ttl$'
key g g; set_cell "new text"; key C-s; saved
check "string str:ttl：改成 new text、C-s：GET 是 new text，TTL 还在（SET … KEEPTTL，十年没变短）" eval '[[ $(rc get str:ttl) == "new text" ]] && (( $(rc ttl str:ttl) > 300000000 ))' || { rc get str:ttl; rc ttl str:ttl; }

open_key '^user$'
key g g; key l; set_cell 31
key j; key j; key j; key h; set_cell nick
key k; key d; key d
key o; set_cell city; key l; set_cell Paris
check "（准备）hash:user 四处改动都标出来了：查询条上保存按钮的计数是 4" eval 'e2e_text 36 96 4 | grep -qE "PAGE 1/1[^0-9]+4([^0-9]|$)"' || e2e_text 36 96 4
key C-s; saved
check "hash:user：age 改成 31、name 改名 nick（HDEL + HSET）、meta 删掉、新增 city = Paris：一次写回后 HGETALL 是 age 31 / email / nick Ann / city Paris，没有 name、meta" eval '[[ $(rc hget hash:user age) == 31 && $(rc hget hash:user nick) == Ann && $(rc hget hash:user city) == Paris && $(rc hexists hash:user name) == 0 && $(rc hexists hash:user meta) == 0 && $(rc hlen hash:user) == 4 ]]' || hall hash:user
check "写回之后表格重读：四行 age city email nick（按 field 排），没有标记了" eval 'rowsare "field value
1 age 31
2 city Paris
3 email ann@example.com
4 nick Ann"'

open_key '^binary$'   # hash:binary: the binary value is not written back, the other field is
key g g; key j; key l; set_cell fine; key C-s; saved
check "hash:binary：改 text 的值 → fine、C-s：text 变了，raw 的二进制（00 01 ff）原样没动" eval '[[ $(rc hget hash:binary text) == fine && $(rc --no-raw hget hash:binary raw | head -1) == "\"\\x00\\x01\\xff\"" ]]' || { rc --no-raw hget hash:binary raw; }

open_key '^dups$'
key g g; key l; set_cell X
key j; key j; key d; key d
key G; key o; key l; set_cell z
key C-s; saved
check "list:dups（x y x y x）：第 0 行 → X、删第 2 行（LSET 哨兵 + LREM）、末尾加 z（RPUSH）：LRANGE 是 X y y x z，相同的值没有被误删" eval '[[ $(rc lrange list:dups 0 -1 | paste -sd" " -) == "X y y x z" ]]' || rc lrange list:dups 0 -1 | paste -sd" " -
check "写回之后的表格：index 从 0 重排 0..4" eval 'rowsare "index value
1 0 X
2 1 y
3 2 y
4 3 x
5 4 z"'

open_key '^tags$'
key g g; set_cell azure
key j; key d; key d
key G; key o; set_cell purple
key C-s; saved
check "set:tags：blue → azure（SREM + SADD）、删 green、加 purple：SMEMBERS 是 azure purple red" eval '[[ $(rc smembers set:tags | sort | paste -sd" " -) == "azure purple red" ]]' || rc smembers set:tags

open_key '^rank$'
key g g; key l; set_cell 7
key j; key h; set_cell bobby
key j; key d; key d
key G; key o; set_cell frank; key l; set_cell 4
key C-s; saved
check "zset:rank：alice 的分改成 7、bob 改名 bobby（分数 2.5 带过去）、删 carol、加 frank 4：ZRANGE WITHSCORES 对" eval '[[ $(rc zrange zset:rank 0 -1 withscores | paste -sd" " -) == "bobby 2.5 dave 3 frank 4 alice 7 erin 10" ]]' || rc zrange zset:rank 0 -1 withscores | paste -sd" " -

open_key '^events$'
key g g; key Enter; sleep 0.4
check "stream：格子不能改，toast「stream 的条目不能改，只能删除」" eval '[[ $(toast) == *"stream 的条目不能改，只能删除"* ]]' || toast
key Escape; sleep 3; key o; sleep 0.4
check "stream：不能加行，toast「stream 的条目用 XADD 加，这里只能删除」" eval '[[ $(toast) == *"stream 的条目用 XADD 加，这里只能删除"* ]]' || toast
key j; key d; key d; key C-s; saved
check "stream：删第 2 行（XDEL）：XLEN 2，剩 …-1 和 …001-1" eval '[[ $(rc xlen stream:events) == 2 && $(rc xrange stream:events - + | grep -c "^17000000000") == 2 && $(rc exists stream:events) == 1 ]] && ! rc xrange stream:events - + | grep -q "1700000000000-2"' || rc xrange stream:events - +
quit_app; _e2e_kill

e2e_done
