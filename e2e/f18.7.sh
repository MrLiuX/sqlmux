#!/usr/bin/env bash
# F18.7 SQL Server（specs/m18-windows-mssql/task.md F18.7 a；tech-design §8.1「SQL Server」、§8.5、§9.2、§9.4）：照 f5.4（MySQL）在真实终端和 SQL Server 2022 上走一遍读的那一半：
# 树里列 database 和 schema（不列系统库）、表格浏览（分页、计数、WHERE、bit 显示 true / false）、面板（表、DDL 预览、快速 SQL 的自动分页、TOP 不再追加分页）、
# query（标题的 库.schema 下拉、GO 分批、临时表在同一个会话里、选中几批一起执行出几个结果 tab）、库切换、C-c 取消 waitfor。
# 共用的 sqlmux 库只读（连 Windows 主机上的容器，SQLMUX_TEST_MSSQL）；写库的在 f18.7b.sh、只读连接在 f18.7c.sh。
. "$(dirname "$0")/lib.sh"
e2e_build || exit 1
. "$(dirname "$0")/palette.sh"

D=$(mktemp -d "${TMPDIR:-/tmp}/sqlmux-e2e-cfg.XXXXXX")
trap 'e2e_stop; rm -rf "$D"' EXIT
[[ -n $SQLMUX_TEST_MSSQL ]] || { echo "e2e: SQLMUX_TEST_MSSQL is not set" >&2; exit 1; }
MS_ADDR=$(python3 -c 'import sys, urllib.parse as u; p = u.urlsplit(sys.argv[1]); print(f"{p.username}@{p.hostname}:{p.port}")' "$SQLMUX_TEST_MSSQL")
mkdir -p "$D/a"
(umask 077; printf '[[connection]]\nname = "ms"\nengine = "mssql"\ndsn = "%s"\n' "$SQLMUX_TEST_MSSQL" >"$D/a/connections.toml")

sidebar() { local y; for y in $(seq 3 43); do e2e_text 1 32 "$y"; done; }
trow() { local y; for y in $(seq 3 43); do [[ $(e2e_text 1 32 "$y") == *" $1 "* ]] && { echo "$y"; return; }; done; }
paste() { t set-buffer -b e2e "$1"; t paste-buffer -r -p -b e2e -t t; sleep 0.4; }
setsql() { key Escape; key g g; key d G; key i; paste "$1"; key Escape; key g g; }   # 焦点所在 query 的内容换成 SQL
done_run() { wait_for 8 eval '[[ $(bar) != *busy* ]]'; sleep 0.4; }
run() { key Enter; done_run; }
rval() { local g; g=($(geom 3)); e2e_text $((g[0] + 1)) $((g[0] + g[2] - 2)) $((g[1] + 3)) | awk -F'│' '{ gsub(/ /, "", $2); print $2 }'; }   # ③ 结果第一行第一列
ctitle() { local g; g=($(geom 2)); e2e_text "${g[0]}" $((g[0] + g[2] - 1)) "${g[1]}"; }
click_title() { local g x; g=($(geom 2)); x=$(e2e_find "$1" "${g[1]}" | tr ' ' '\n' | awk -v l="${g[0]}" '$1 > l { print; exit }'); e2e_click "$x" "${g[1]}"; sleep 0.4; }
pane_has() { local g y; g=($(geom "$1")); for ((y = g[1]; y < g[1] + g[3]; y++)); do e2e_text "${g[0]}" $((g[0] + g[2] - 1)) "$y"; done | grep -qF -- "$2"; }
qbar() { local g; g=($(geom 1)); e2e_text $((g[0] + 1)) $((g[0] + g[2] - 2)) $((g[1] + 2)); }
rtab() { local g; g=($(geom 3)); e2e_text $((g[0] + 1)) $((g[0] + g[2] - 2)) $((g[1] + g[3] - 2)) | sed 's/ *hjkl.*//; s/^ *//'; }   # ③ 的 tab 栏
rrows() { local g y; g=($(geom 3)); for ((y = g[1] + 3; y < g[1] + 8; y++)); do e2e_text $((g[0] + 1)) $((g[0] + g[2] - 2)) $y | sed 's/^ *[0-9]* *│ *//; s/ *│.*//; s/ *$//'; done | tr '\n' ' ' | sed 's/ *$//'; }   # ③ 当前 tab 的前几行第一列

# ---- 树：连接下面一层是 database（不列系统库），连上的库展开 schema 和 Tables / Views / Functions
start -C "$D/a"; sleep 2
check "树的根 ms 连上并展开" eval '[[ $(e2e_text 1 32 "$(trow ms)") == *"▾ "*" ms "* ]]'
check "ms 下面是 database：agentable、sqlmux（展开：dbo、Tables (6)、Views (1)、Functions (2)）；master、tempdb、model、msdb 不列" eval 's=$(sidebar); [[ $s == *" agentable "* && $s == *"▾ "*" sqlmux "* && $s == *" dbo "* && $s == *"Tables (6)"* && $s == *"Views (1)"* && $s == *"Functions (2)"* && $s != *" master "* && $s != *" tempdb "* && $s != *" model "* && $s != *" msdb "* ]] || { echo "$s" | head -20; false; }'
check "状态栏的地址块：ms · $MS_ADDR" eval '[[ $(bar) == *" ms · $MS_ADDR "* ]]'

# ---- 表格：取数、分页、计数、WHERE；bit 显示 true / false
open_table t_order
check "打开 t_order：6000 行，PAGE 1/60，第一行 id 1" eval '[[ $(qbar) == *"PAGE 1/60"* ]] && e2e_plain | grep -qE "│ +1 │ +1 │"'
check "paid 是 bit：前三行显示 true / false / <null>" eval 'e2e_plain | grep -E "│ +[123] │ +[123] │" | cut -c40-110 | tr -s " " | tr "\n" ";" | grep -q "true.*false.*<null>" || { e2e_plain | sed -n 5,8p | cut -c34-120; false; }'
key ']'; wait_for 5 eval '[[ $(qbar) == *"PAGE 2/60"* ]]'
check "]：PAGE 2/60，第一行 id 101（OFFSET 100 ROWS FETCH NEXT 100 ROWS ONLY）" eval '[[ $(qbar) == *"PAGE 2/60"* ]] && e2e_plain | grep -qE "│ +101 │ +101 │"'
key /; e2e_type "status = 'done'"; key Enter; wait_for 5 eval '[[ $(qbar) == *"PAGE 1/15"* ]]'
check "WHERE status = 'done'：1500 行，PAGE 1/15" eval '[[ $(qbar) == *"PAGE 1/15"* ]]'

# ---- 面板：表的所在位置是 ms.dbo；DDL 预览是 T-SQL 的 create table；快速 SQL
pal "@t_order"; sleep 0.8
check "@t_order：所在位置 ms.dbo；预览是 create table [dbo].[t_order] (…" eval 'list | grep -q "^t_order  ms.dbo  表|" && preview | head -1 | grep -qF "create table [dbo].[t_order] (" && preview | grep -qF "[status] nvarchar(16) not null default ('"'"'pending'"'"')"'
clear_input; sql "select * from t_order"
check "快速 SQL select * from t_order：自动加 OFFSET 0 ROWS FETCH NEXT 101 ROWS ONLY，结果「100+ 行」" eval '[[ $(title) == "100+ 行"* ]] || { echo "  $(title)"; false; }'
sql "select top 3 id from t_order order by id desc"
check "快速 SQL select top 3 …：顶层带 TOP 不追加分页，3 行 6000 / 5999 / 5998" eval '[[ $(title) == "3 行"* && $(grid | tr "\n" " ") == "6000 5999 5998 " ]] || { echo "  $(title) / $(grid | tr "\n" " ")"; false; }'
sql "select distinct status from t_order"
check "快速 SQL select distinct：不追加分页也不报错，4 种状态" eval '[[ $(title) == "4 行"* ]] || { echo "  $(title) / $(line1)"; false; }'
sql "delete from t_log"
check "快速 SQL delete from t_log：写语句不执行" eval '[[ $(line1) == "写语句不在这里执行"* ]]'
key Escape
y=$(trow agentable); e2e_click "$(e2e_find agentable "$y" | cut -d' ' -f1)" "$y"; sleep 1
pal; sql "select count(*) as n from dbo.agent"
check "树的光标移到 agentable 上：快速 SQL 在 agentable 库里，select count(*) from dbo.agent 是 3" eval '[[ $(grid | head -1) == 3 ]] || { echo "  $(title) / $(line1) / $(grid | head -1)"; false; }'
key Escape

# ---- query：库.schema 的标题、GO 分批、临时表、选中几批一起执行
key C-l; key C-l
check "② query 标题：ms · sqlmux.dbo ▾" eval '[[ $(ctitle) == *" ms · sqlmux.dbo ▾ "* ]] || { echo "  $(ctitle)"; false; }'
setsql "select 5 + 1 as x"; run
check "query select 5 + 1：6（一条普通语句，结果在 ③）" eval '[[ $(rval) == 6 ]] || { echo "  got: $(rval)"; false; }'
setsql "$(printf 'select 10 as a\ngo\ncreate table #t (x int)\ninsert into #t values (7), (8)\ngo\nselect 20 as b\ngo\nselect x from #t order by x')"
key V; key G; run
check "选中全部（V G）↵：GO 切成几批依次执行，三条 select 出三个结果 tab（query_1 #N、#N·2、#N·3）" eval '[[ $(rtab) == *"query_1 #"*"·2"*"query_1 #"*"·3"* ]] || { echo "  $(rtab)"; false; }'
key C-j; key g t; key g t; sleep 0.4
check "第 3 个结果 tab：select x from #t，8 行以内是 7、8——#t 在批与批之间还在（同一个会话）" eval '[[ $(rrows) == "7 8" ]] || { echo "  $(rtab) / $(rrows)"; g=($(geom 3)); e2e_plain | sed -n "${g[1]},$((g[1] + 8))p" | cut -c34-100; false; }'
key C-k; key g g; run
check "光标在第 1 句上 ↵：只执行 select 10 as a，不连带 GO 后面的批" eval '[[ $(rval) == 10 ]] || { echo "  got: $(rval)"; false; }'
setsql "select * from no_such_table"; run
check "不存在的表：错误是 SQL Server 的 [208] Invalid object name 'no_such_table'" eval 'e2e_plain | grep -q "\[208\] Invalid object name .no_such_table" || { e2e_plain | sed -n 1,45p | cut -c34-160 | grep -i "invalid\|error"; false; }'
click_title "sqlmux.dbo"; e2e_type agentable; sleep 0.3; key Enter
check "点标题的下拉选 agentable：标题 ms · agentable.dbo ▾" eval '[[ $(ctitle) == *" ms · agentable.dbo ▾ "* ]] || { echo "  $(ctitle)"; false; }'
setsql "select count(*) from dbo.agent"; run
check "agentable 里 select count(*) from dbo.agent 是 3" eval '[[ $(rval) == 3 ]] || { echo "  got: $(rval)"; false; }'
setsql "select db_name()"; run
check "select db_name()：agentable（query 的库是它自己的，不是树的）" eval '[[ $(rval) == agentable ]] || { echo "  got: $(rval)"; false; }'

# ---- 取消：C-c 把 waitfor delay 取消掉，服务端上它停了，之后能继续执行
setsql "waitfor delay '00:00:30'"; key Enter; sleep 1
check "（准备）waitfor delay 在跑，状态栏 busy" eval '[[ $(bar) == *busy* ]]'
SECONDS=0; key C-c; wait_for 5 eval '[[ $(bar) != *busy* ]]'; took=$SECONDS
check "C-c：1 秒左右（≤ 3 秒）回来，不再 busy" eval '[[ $took -le 3 && $(bar) != *busy* ]] || { echo "  took $took s"; false; }'
check "服务端上没有残留的 waitfor（sys.dm_exec_requests 里没有）" eval 'n=$(e2e_mssql master <<<"select count(*) from sys.dm_exec_requests where command = '"'"'WAITFOR'"'"'"); [[ $n == 0 ]] || { echo "  $n"; false; }'
setsql "select 41 + 1 as x"; run
check "取消之后下一次执行照常（连接重拨，结果 42）" eval '[[ $(rval) == 42 ]] || { echo "  got: $(rval) / $(bar)"; false; }'
e2e_done
