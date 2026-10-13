#!/usr/bin/env bash
# F15.1b PG 数据库层级：同名表、命令面板、删表、连接下拉、事务退出、连不上的库（审查提测的重点；F15.1 的 e2e 在 f15.1.sh）
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
cleanup() { e2e_stop; ADMIN -c "drop database if exists ${B} with (force)" -c "drop database if exists ${OWN}_d with (force)" -c "drop database if exists ${C} with (force)" -c "drop database if exists ${L} with (force)" -c "drop role if exists ${R}" >/dev/null; rm -rf "$D"; }
trap cleanup EXIT
e2e_own_db || { echo "e2e: could not create the private database" >&2; exit 1; }
ADMIN -c "create database ${B}" -c "create database ${OWN}_d" -c "create database ${C} allow_connections false" -c "create database ${L}" -c "revoke connect on database ${L} from public" -c "create role ${R} login password 'x'" >/dev/null || exit 1
psql "${E2E_DB}" -q -c "grant select on all tables in schema public to ${R}" >/dev/null
psql "${E2E_DB/${OWN}/${B}}" -q -c "create table t_user (id int primary key, v text)" -c "insert into t_user values (1, 'b-row')" -c "create table bt (id int primary key, v text)" -c "insert into bt values (1, 'bt-row')" >/dev/null
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

D_=${OWN}_d
tsel_y() { e2e_rows 1 32 3 43 4 | python3 -c "
import sys
for y, l in enumerate(sys.stdin, 3):
    if l.rpartition('|')[2].strip().lower() == '$SELECT': print(y); break"; }
rclick() { _sgr 2 "$1" "$2" M; _sgr 2 "$1" "$2" m; sleep 0.6; }
pop() { e2e_panes | awk '$1 == "-" { print $2, $3, $4, $5 }' | tail -1; }
mclick() { local g y; g=($(pop)); for ((y = g[1] + 1; y < g[1] + g[3] - 1; y++)); do [[ $(e2e_text $((g[0] + 1)) $((g[0] + g[2] - 2)) $y) == *"$1"* ]] && { e2e_click $((g[0] + 3)) $y; sleep 1; return; }; done; echo "  no menu item $1"; return 1; }
tree_nth() { local i n=0 cur; tree_focus; key g g; for ((i = 0; i < 120; i++)); do cur=$(tsel); [[ $cur == "$1" || $cur == "$1 "* ]] && ((++n == ${2:-1})) && return 0; key j; done; return 1; }   # the Nth tree node named $1, down from the top
tabs() { e2e_plain | sed -n 43p | python3 -c "import sys, re; print(re.sub('[-]', '', sys.stdin.read()[33:]).split('  ')[0])"; }   # the data pane's tab bar, up to the key hints
popt() { local g y; g=($(pop)); [[ -n ${g[0]} ]] || return 0; for ((y = g[1]; y < g[1] + g[3]; y++)); do e2e_text "${g[0]}" $((g[0] + g[2] - 1)) "$y"; done; }   # the open popup box (a dropdown), row by row
psel() { list | awk -F'|' '$2 == 1 { print $1 }'; }

# ---- 两个库里同名的表 t_user：各开一个 tab、标题不同；命令面板列出两条，写 连接/库.schema
start -C "$D/own" -D "$D/data" -S "$D/st"; loaded
tree_to "${B}"; key l; sleep 1.5
tree_nth t_user 1; key Enter; sleep 2
tree_nth t_user 2; key Enter; sleep 2
check "两个库里同名的 t_user 各开一个 tab：默认库的叫 t_user，${B} 的叫 ${B}.public.t_user" eval '[[ $(tabs) == *"1: t_user"*"2: ${B}.public.t_user"* ]]' || { tabs | cat -v | head -3; }
check "第二个 tab 里是 ${B} 的数据（b-row），不是默认库的 50 行" eval 'e2e_plain | grep -qF "b-row" && [[ $(cnt) == 1 ]]' || cnt
pal "@t_user"; sleep 0.6
check "面板 @t_user：两条，当前库 ${B} 的在前写 doraemon/${B}.public，默认库的写 doraemon.public（默认库不写库）" eval '[[ $(list | cut -d"|" -f1 | tr -s " " | tr "\n" ";") == "t_user doraemon/${B}.public 表;t_user doraemon.public 表;" ]]' || list
key Escape; sleep 0.3
key g T; sleep 0.8
pal "@bt"; sleep 0.6
check "当前在默认库的 t_user tab 上，面板 @bt：bt 只在 ${B} 里，排第一，写 doraemon/${B}.public" eval '[[ $(list | head -1 | cut -d"|" -f1 | tr -s " ") == "bt doraemon/${B}.public 表" ]]' || list
key Enter; sleep 2
check "↵ 打开的是 ${B} 的 bt：tab 叫 ${B}.public.bt，里面是 bt-row" eval '[[ $(tabs) == *"${B}.public.bt"* ]] && e2e_plain | grep -qF "bt-row"' || tabs
# 删 ${B} 的 t_user：确认框和「已删除」写全名，只关这个库的 tab
tree_nth t_user 2; y=$(tsel_y); rclick 12 "$y"; mclick "删除表…"
check "删表确认框写全名：删除表 ${B}.public.t_user，不能撤销" eval 'e2e_plain | grep -qF "删除表 ${B}.public.t_user，不能撤销"' || e2e_plain | sed -n 8,30p
key y; sleep 1.5
check "y：toast「已删除 ${B}.public.t_user」，服务端 ${B} 里 t_user 没了、默认库的还在" eval 'e2e_plain | grep -qF "已删除 ${B}.public.t_user" && [[ $(psql "${E2E_DB/${OWN}/${B}}" -At -c "select count(*) from pg_tables where tablename = '"'"'t_user'"'"'") == 0 && $(psql "$E2E_DB" -At -c "select count(*) from pg_tables where tablename = '"'"'t_user'"'"'") == 1 ]]' || e2e_plain | sed -n 40,45p
check "只关了 ${B} 的 t_user 的 tab：默认库的 t_user 和 ${B} 的 bt 还在" eval '[[ $(tabs) == *"t_user"* && $(tabs) == *"${B}.public.bt"* && $(tabs) != *"${B}.public.t_user"* ]]' || tabs
_e2e_kill

# ---- 非默认库里的 query：连接下拉在当前连接上 ↵ 什么都不做；开着事务退出，问句列出 连接/库
start -C "$D/own"; loaded
tree_to "${B}"; key l; sleep 1.5
tree_nth bt 1; key Enter; sleep 2
pal ">新建 query"; sleep 0.5; key Enter; sleep 1.5
check "（准备）在 ${B} 的表上新建 query：标题 ${B}.public" eval '[[ $(qtitle) == "doraemon · ${B}.public ▾" ]]' || { ptitle; qtitle; }
pal ">切换连接"; sleep 0.5; key Enter; sleep 0.8
check "切换连接…：下拉里是 doraemon" eval 'popt | grep -q doraemon' || e2e_plain | sed -n 1,12p
key Enter; sleep 1
check "在当前连接 doraemon 上 ↵：什么都不做，query 还在 ${B}（标题不变）" eval '[[ $(qtitle) == "doraemon · ${B}.public ▾" ]]' || { ptitle; qtitle; }
key i; e2e_type "begin"; key Escape; run
check "begin：状态栏事务中" eval '[[ $(bar) == *事务中* ]]' || bar
e2e_type ":"; sleep 0.4; e2e_type qa; key Enter; sleep 1
check "开着事务 :qa：问句提到 doraemon/${B}" eval 'e2e_plain | grep -qF "doraemon/${B}"' || e2e_plain | sed -n 8,30p
_e2e_kill

# ---- 子库连不上（库被删了）：节点标红、toast 带 连接/库
start -C "$D/own"; loaded
ADMIN -c "drop database ${D_} with (force)" >/dev/null
tree_to "${D_}"; y=$(tsel_y); key l; sleep 2.5
x=$(e2e_find "${D_}" "$y" | cut -d" " -f1)
check "展开一个已经被删掉的库：节点名 error 色（#f7768e）" eval '[[ $(e2e_style "$x" "$y") == *fg=#f7768e* ]]' || { e2e_style "$x" "$y"; e2e_plain | sed -n 1,30p | head -3; }
check "toast 带 doraemon/${D_}" eval 'e2e_plain | grep -qF "doraemon/${D_}"' || e2e_plain | tail -5
e2e_done
