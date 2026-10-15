#!/usr/bin/env bash
# F16.10 Redis 的 db 节点按号排（specs/m16-redis/task.md F16.10；tech-design §7.8「Redis 的树」）
# db 节点按号从小到大排（db0 db1 db2 db3 db10，不是按字符串），DSN 指定的那个夹在里面、默认展开，其余收着；原来 DSN 的排最前面（db2 db0 db1 db3）。
# DSN 指定的 db 里没有键也列、也展开；外面加了键之后 R 刷新，新出现的 db 也落在按号的位置上。每个脚本自己起 redis-server（redis.sh）。
. "$(dirname "$0")/lib.sh"
e2e_build || exit 1
. "$(dirname "$0")/palette.sh"
. "$(dirname "$0")/redis.sh"

D=$(mktemp -d "${TMPDIR:-/tmp}/sqlmux-e2e-cfg.XXXXXX"); trap 'e2e_stop; redis_stop; rm -rf "$D"' EXIT
redis_start R || exit 1
for n in 0 1 3 10; do rc -n $n set "k$n" v >/dev/null; done
mkdir -p "$D/c"
(umask 077; : >"$D/c/connections.toml"
for n in 0 2 3 7 10; do printf '[[connection]]\nname = "d%s"\nengine = "redis"\ndsn = "redis://localhost:%s/%s"\n\n' "$n" "$RPORT" "$n" >>"$D/c/connections.toml"; done)
# the db nodes of the sidebar in the order they are drawn: "▾ db2" / "▸ db3", the keys under an open one left out
dbs() { e2e_plain | python3 -c '
import re, sys
for l in sys.stdin:
    m = re.match(r"^│ {3}([▸▾]) +(db\d+)", re.sub("[-] ?", "", l))
    if m: print(m.group(1), m.group(2))' | tr '\n' ',' ; }
side_has() { e2e_plain | python3 -c "import re, sys; sys.exit(0 if re.search(sys.argv[1], re.sub('[-] ?', '', sys.stdin.read())) else 1)" "$1"; }
open() { e2e_start -C "$D/c" "$E2E_BIN $1"; wait_for 8 flag_is alternate_on 1; sleep 2; }
ddrows() { e2e_plain | python3 -c "import re, sys; print(' '.join(re.findall(r'│ (db\\S*) +│', sys.stdin.read())))"; }   # the rows of the open dropdown (gs of a query): db names
quit_app() { e2e_type ":"; sleep 0.4; e2e_type qa; key Enter; wait_for 8 exited; sleep 0.5; e2e_stop; }

# ---- DSN 的 db 里没有键（db2）：按号夹在 db1 和 db3 中间、展开
open d2
check "DSN /2（db2 里没有键），服务端有键的是 0 1 3 10：db0 db1 db2 db3 db10，只有 db2 展开（原来是 db2 排最前面）" eval '[[ $(dbs) == "▸ db0,▸ db1,▾ db2,▸ db3,▸ db10," ]]' || dbs
check "按号，不是按字符串：db3 在 db10 前面（字符串顺序 db10 会在 db1 后面）" eval '[[ $(dbs) == *"db3,▸ db10"* ]]'
quit_app
# ---- DSN 的 db 里有键、在中间（db3）
open d3
check "DSN /3：db0 db1 db3 db10，没有 db2（没有键也不是 DSN 的就不列），db3 展开" eval '[[ $(dbs) == "▸ db0,▸ db1,▾ db3,▸ db10," ]]' || dbs
check "展开的 db3 下面有它的键 k3（展开的就是 DSN 的那个）" eval 'side_has "k3"' 
check "别的 db 的键没读出来（k0 k1 k10 不在屏幕上，收着的 db 不扫描）" eval '! side_has "k0|k1[^0-9]|k10"' || e2e_plain | sed -n 3,14p | cut -c1-40
# ---- 外面加了键：新出现的 db 落在按号的位置上
rc -n 2 set k2 v >/dev/null
y=$(e2e_plain | python3 -c '
import re, sys
for i, l in enumerate(sys.stdin.read().split("\n")):
    if re.search(r"db3( |$)", re.sub("[-] ?", "", l[:30])): print(i + 1); break')
e2e_click 14 "$y"; sleep 0.4; key R; sleep 1.5   # the click folds db3 and leaves the cursor on it
check "db2 里加了键之后在 db3 上 R：db2 出现在 db1 和 db3 中间（db0 db1 db2 db3 db10；db3 是点了折起来的）" eval '[[ $(dbs) == "▸ db0,▸ db1,▸ db2,▸ db3,▸ db10," ]]' || { dbs; e2e_plain | sed -n 3,14p | cut -c1-40; }
quit_app
rc -n 2 del k2 >/dev/null
# ---- DSN 的 db 排在最后（db10）、在最前（db0）
open d10
check "DSN /10：db0 db1 db3 db10，db10 展开（排在最后）" eval '[[ $(dbs) == "▸ db0,▸ db1,▸ db3,▾ db10," ]]' || dbs
quit_app
open d0
check "DSN /0：db0 db1 db3 db10，db0 展开（和原来一样）" eval '[[ $(dbs) == "▾ db0,▸ db1,▸ db3,▸ db10," ]]' || dbs
# ---- DSN 指向空的 db7（排在最后）：树里最后、展开、连着；query 的数据库下拉（gs）也是同一个顺序，打开几次也不重复前缀
rc -n 7 flushdb >/dev/null; rc -n 10 flushdb >/dev/null   # db10 gone: db7 is the biggest number
open d7
check "DSN /7（空，比所有有键的 db 都大）：db0 db1 db3 db7，db7 排在最后、展开" eval '[[ $(dbs) == "▸ db0,▸ db1,▸ db3,▾ db7," ]]' || dbs
key C-l; key g s; sleep 0.8
check "query 的数据库下拉（gs）：db0 db1 db3 db7，同树里的顺序" eval '[[ $(ddrows) == "db0 db1 db3 db7" ]]' || { ddrows; e2e_plain | sed -n 1,10p | cut -c60-165; }
key Escape; sleep 0.3
for pick in db3 db0 db7; do key g s; sleep 0.6; e2e_type "$pick"; sleep 0.3; key Enter; sleep 1.2; done
key g s; sleep 0.8
check "连着选了 db3、db0、db7 再打开：下拉里还是 db0 db1 db3 db7，没有 dbdb 之类重复的前缀" eval '[[ $(ddrows) == "db0 db1 db3 db7" ]] && ! e2e_plain | grep -q "dbdb"' || { ddrows; e2e_plain | sed -n 1,10p | cut -c60-165; }
key Escape; sleep 0.3
check "下拉选过一圈之后，query 的标题是 d7 · db7，树里的顺序没变" eval 'e2e_plain | sed -n 1p | grep -q "d7 · db7" && [[ $(dbs) == "▸ db0,▸ db1,▸ db3,▾ db7," ]]' || { dbs; e2e_plain | sed -n 1p | cut -c60-165; }
e2e_done
