#!/usr/bin/env bash
# F15.1 PG 数据库层级（specs/m15-pg-databases/task.md F15.1；tech-design §7.8「数据库节点」、§8.4、§5）
# 树是 连接 → 数据库 → schema：DSN 里的库排第一、默认展开，其余按名字、收着，点开才连；closed 的库（allow_connections false）、模板库不列，没有 CONNECT 权限的库对那个角色不列；
# 表 tab 的状态栏地址只在非默认库时带库名；query 的标题是 <库>.<schema>，query.schema 的下拉第一级列库、选了就切库；layout 重启后 query 还在那个库。
# 自建库：own（DSN 里的库，seed 的副本）、own_b（有一张表 bt）、own_c（不接受连接）、own_l（对 PUBLIC 撤了 CONNECT）、角色 r（只能连前两个）。
. "$(dirname "$0")/lib.sh"
SOLO=1
e2e_build || exit 1
. "$(dirname "$0")/palette.sh"

D=$(mktemp -d "${TMPDIR:-/tmp}/sqlmux-e2e-cfg.XXXXXX")
ADMIN() { PGOPTIONS="-c client_min_messages=warning" psql "$SQLMUX_TEST_PG" -qAt "$@"; }
OWN=sqlmux_e2e_$$; B=${OWN}_b; C=${OWN}_c; L=${OWN}_l; R=sqlmux_f151_r_$$
cleanup() { e2e_stop; ADMIN -c "drop database if exists ${B} with (force)" -c "drop database if exists ${C} with (force)" -c "drop database if exists ${L} with (force)" -c "drop role if exists ${R}" >/dev/null; rm -rf "$D"; }
trap cleanup EXIT
e2e_own_db || { echo "e2e: could not create the private database" >&2; exit 1; }
ADMIN -c "create database ${B}" -c "create database ${C} allow_connections false" -c "create database ${L}" -c "revoke connect on database ${L} from public" -c "create role ${R} login password 'x'" >/dev/null || exit 1
psql "${E2E_DB}" -q -c "grant select on all tables in schema public to ${R}" >/dev/null
psql "${E2E_DB/${OWN}/${B}}" -q -c "create table bt (id int primary key, v text)" -c "insert into bt values (1, 'b-row')" >/dev/null
mkdir -p "$D/own" "$D/lim" "$D/data" "$D/st" "$D/data2" "$D/st2"
printf '[[connection]]\nname = "doraemon"\nengine = "postgres"\ndsn = "%s"\n' "$E2E_DB" >"$D/own/connections.toml"
printf '[[connection]]\nname = "limited"\nengine = "postgres"\ndsn = "%s"\n' "${E2E_DB/sqlmux:sqlmux@/${R}:x@}" >"$D/lim/connections.toml"
chmod 600 "$D"/{own,lim}/connections.toml
SELECT=#364a82
conns() { ADMIN -c "select count(*) from pg_stat_activity where datname = '$1'"; }          # the sessions open on database $1
dbrows() { e2e_plain | python3 -c "
import re, sys
for l in sys.stdin:
    m = re.match(r'│ {3}[▸▾]\W*?(\w+)', l)
    if m: print(m.group(1))"; }   # the database nodes (the second level of the tree), in screen order
tsel() { e2e_rows 1 32 3 43 4 | python3 -c "
import re, sys
for l in sys.stdin:
    t, _, bg = l.rstrip('\n').rpartition('|')
    if bg.strip().lower() == '$SELECT': print(re.sub(r'^[^\w]+', '', t.strip()).rstrip('│ ')); break"; }
tree_focus() { local i; for ((i = 0; i < 6; i++)); do [[ $(focused) == 0 ]] && return 0; key C-h; done; }
tree_to() { local i; tree_focus; key g g; for ((i = 0; i < 90; i++)); do [[ $(tsel) == "$1" || $(tsel) == "$1 "* ]] && return 0; key j; done; return 1; }
side() { e2e_plain | python3 -c 'import re, sys; [print(re.sub("[\ue000-\uf8ff] ?", "", l[:34])) for l in sys.stdin.read().split("\n")]'; }   # the sidebar's columns of each screen row, the icons off
addr() { bar | grep -oE "[a-z]+@localhost:[0-9]+[^ ]*"; }                                   # the status bar's address
ptitle() { e2e_plain | sed -n 1p | python3 -c "import sys; print(sys.stdin.read()[33:])"; }                                             # the top border of the pane at the right
qtitle() { ptitle | grep -oE "(Tx: [^ ]+ ▾ )?[^ ]+ · [^ ]+ ▾" | sed 's/^Tx: [^ ]* ▾ //'; }   # the query's "<conn> · <db>.<schema> ▾"
run() { key Enter; wait_for 8 eval '[[ $(bar) != *busy* ]]'; sleep 0.6; }
loaded() { wait_for 25 eval '! e2e_plain | grep -qE "(^|[^0-9])0 tables"'; sleep 0.5; }   # the connection is dialed and its catalog read
quit_app() { e2e_type ":"; sleep 0.4; e2e_type qa; key Enter; wait_for 8 exited; sleep 0.5; }

start -C "$D/own" -D "$D/data" -S "$D/st"; loaded
DBS=$(dbrows | tr '\n' ' ')
check "树是 连接 → 数据库：doraemon 下第一个库是 DSN 里的库（${OWN}），展开着，schema public 在它下面" eval '[[ $(dbrows | head -1) == "${OWN}" ]] && side | sed -n 4,12p | grep -q "▾ ${OWN}" && side | sed -n 4,14p | grep -q "public"' || { echo "  dbs: $DBS"; e2e_plain | sed -n 3,14p | cut -c1-40; }
check "其余的库按名字排、收着（▸）：postgres、sqlmux、…_b、…_l 在，模板库和不接受连接的库（_c）不在" eval '[[ " $DBS" == *" postgres "* && " $DBS" == *" sqlmux "* && " $DBS" == *" ${B} "* && " $DBS" == *" ${L} "* && " $DBS" != *" ${C} "* && " $DBS" != *template* ]] && [[ $(dbrows | tail -n +2) == "$(dbrows | tail -n +2 | LC_ALL=C sort)" ]]' || echo "  dbs: $DBS"
check "（别的库点开才连）展开之前服务端 ${B} 上一个连接都没有" eval '[[ $(conns "${B}") == 0 ]]'
tree_to "${B}"; key l; sleep 1.5
check "光标在 ${B} 上 l：展开，下面是它的 schema public、Tables (1)、表 bt（别的库也默认展开到表）" eval 'side | sed -n 4,43p | python3 -c "
import sys
l = [x.strip(\"│ \\n\") for x in sys.stdin]
i = [n for n, x in enumerate(l) if x.endswith(\"${B}\")][0]
sys.exit(0 if any(x.endswith(\"public\") for x in l[i+1:i+3]) and any(\"Tables (1)\" in x for x in l[i+1:i+4]) and any(\"bt\" in x for x in l[i+1:i+6]) else 1)"' || e2e_plain | sed -n 4,43p | cut -c1-40
check "展开之后服务端 ${B} 上有连接了（展开才拨号）" eval '[[ $(conns "${B}") -ge 1 ]]'

tree_to t_user; key Enter; sleep 2
check "默认库的表 t_user：状态栏地址没有库名（… sqlmux@localhost:<端口>）" eval '[[ $(addr) == sqlmux@localhost:* && $(addr) != */* ]]' || { addr; bar; }
tree_to bt; key Enter; sleep 2
check "别的库 ${B} 的表 bt：打开，状态栏地址带库名（…/${B}）" eval '[[ $(addr) == *"/${B}" ]] && ptitle | grep -q " bt "' || { addr; ptitle; }
check "表里是那个库的数据（b-row）" eval 'e2e_plain | grep -qF "b-row"'
pal ">新建 query"; sleep 0.5; key Enter; sleep 1.5
check "在这张表的 tab 上新建 query：标题 <库>.<schema> 是 ${B}.public" eval '[[ $(qtitle) == "doraemon · ${B}.public ▾" ]]' || { ptitle; qtitle; }
key i; e2e_type "select current_database()"; key Escape; run
check "select current_database() 是 ${B}" eval 'e2e_plain | grep -qE "│ ${B} +│"' || e2e_plain | sed -n 3,12p
key g s; sleep 0.8
check "gs：下拉第一级列的是库（${OWN}、postgres、sqlmux、${B} 都在）" eval 'e2e_plain | sed -n 1,30p | python3 -c "
import sys
t = sys.stdin.read()
sys.exit(0 if all(w in t for w in [\"${OWN}\", \"postgres\", \"sqlmux\", \"${B}\"]) else 1)"' || e2e_plain | sed -n 1,20p
e2e_type "${OWN}"; sleep 0.4; key Enter; sleep 1.2
check "选 ${OWN}：标题成了 ${OWN}.public" eval '[[ $(qtitle) == "doraemon · ${OWN}.public ▾" ]]' || { ptitle; qtitle; }
run
check "再跑 select current_database()：是 ${OWN}" eval 'e2e_plain | grep -qE "│ ${OWN} +│"' || e2e_plain | sed -n 3,12p
key g s; sleep 0.6; e2e_type "${B}"; sleep 0.4; key Enter; sleep 1.2
check "再 gs 选回 ${B}：标题 ${B}.public（留着给重启用）" eval '[[ $(qtitle) == "doraemon · ${B}.public ▾" ]]' || { ptitle; qtitle; }
quit_app
check "退出后 state.json 里 query_1 记着库：db = ${B}、schema = public" eval 'python3 - "$D/st/sqlmux/state.json" "${B}" <<PY
import json, sys
q = json.load(open(sys.argv[1])).get("queries", {}).get("query_1", {})
sys.exit(0 if q.get("db") == sys.argv[2] and q.get("schema") == "public" else 1)
PY' || cat "$D/st/sqlmux/state.json" | cut -c1-400
_e2e_kill

start -C "$D/own" -D "$D/data" -S "$D/st"; loaded
check "重启：layout 里的 query_1 回到 ${B}（标题 ${B}.public，布局里有 bt 和 query_1 两个 tab）" eval '[[ $(qtitle) == *"${B}.public ▾" ]] && e2e_plain | grep -q "bt" && e2e_plain | grep -q "query_1"' || { ptitle; qtitle; e2e_plain | tail -4 | cut -c1-120; }
quit_app
_e2e_kill

# ---- 没有 CONNECT 权限的库不列出来：角色 ${R} 连不上 ${L}
start -C "$D/lim" -D "$D/data2" -S "$D/st2"; loaded
DBS=$(dbrows | tr '\n' ' ')
check "角色 ${R} 的树：能连的库在（${OWN}、${B}），它没有 CONNECT 权限的 ${L}、不接受连接的 ${C} 都不列" eval '[[ " $DBS" == *" ${OWN} "* && " $DBS" == *" ${B} "* && " $DBS" != *" ${L} "* && " $DBS" != *" ${C} "* ]]' || { echo "  dbs: $DBS"; e2e_plain | sed -n 3,14p | cut -c1-40; }
e2e_done
