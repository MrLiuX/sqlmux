#!/usr/bin/env bash
# F5.4 MySQL（specs/m5-workspace/task.md F5.4；tech-design §5、§7.7、§8.1–§8.6、§9.1、§10.2、§10.3、§12、§13）
# 连接节点、表单的画法由 golden 覆盖，SQL 的拼法、逐类型的旧值校验、只读解除的清单由集成测试覆盖；这里在真实终端和 MySQL 8.4 上
# 端到端地走一遍：树里列 database、表格浏览、面板（表、DDL 预览、快速 SQL）、console（/*! */、选 database、USE 之后重设、
# KILL QUERY 取消）、编辑保存（布尔写 1 / 0、别人改过时报错、只差大小写也查得出）、事务状态与手动模式、只读连接、表单选 mysql。
# 共用的 sqlmux 库只读；写库、开事务都在自建库 sqlmux_e2e_<pid>（lib.sh e2e_own_mydb）。
. "$(dirname "$0")/lib.sh"
e2e_build || exit 1
. "$(dirname "$0")/palette.sh"

D=$(mktemp -d "${TMPDIR:-/tmp}/sqlmux-e2e-cfg.XXXXXX")
trap 'e2e_stop; rm -rf "$D"' EXIT
[[ $SQLMUX_TEST_MYSQL =~ ^([^:]*):([^@]*)@tcp\(([^:]*):([0-9]+)\)/(.*)$ ]] || { echo "e2e: SQLMUX_TEST_MYSQL is not set" >&2; exit 1; }
MY_DSN=mysql://${BASH_REMATCH[1]}:${BASH_REMATCH[2]}@${BASH_REMATCH[3]}:${BASH_REMATCH[4]}/${BASH_REMATCH[5]}
MY_ADDR=${BASH_REMATCH[1]}@${BASH_REMATCH[3]}:${BASH_REMATCH[4]}
e2e_own_mydb || { echo "e2e: could not create the private MySQL database" >&2; exit 1; }
OWN=sqlmux_e2e_$$ MYSQL_ICON=$(printf '\xee\x9c\x84') WARN=#e0af68
mkdir -p "$D/a" "$D/b" "$D/ro"
(umask 077
 printf '[[connection]]\nname = "my"\nengine = "mysql"\ndsn = "%s"\n\n[[connection]]\nname = "doraemon"\nengine = "postgres"\ndsn = "%s"\n' "$MY_DSN" "$SQLMUX_TEST_PG" >"$D/a/connections.toml"
 printf '[[connection]]\nname = "my"\nengine = "mysql"\ndsn = "%s"\n' "$E2E_MYDB" >"$D/b/connections.toml"
 printf '[[connection]]\nname = "ro"\nengine = "mysql"\ndsn = "%s"\nread_only = true\n' "$E2E_MYDB" >"$D/ro/connections.toml")

my() { e2e_mysql "$OWN" <<<"$1"; }                                         # 在自建库上执行一句，输出不带表头
sidebar() { local y; for y in $(seq 3 43); do e2e_text 1 32 "$y"; done; }
trow() { local y; for y in $(seq 3 43); do [[ $(e2e_text 1 32 "$y") == *" $1 "* ]] && { echo "$y"; return; }; done; }
paste() { t set-buffer -b e2e "$1"; t paste-buffer -r -p -b e2e -t t; sleep 0.4; }
setsql() { key Escape; key g g; key d G; key i; paste "$1"; key Escape; key g g; }   # 焦点所在 console 的内容换成 SQL
done_run() { wait_for 8 eval '[[ $(bar) != *busy* ]]'; sleep 0.4; }
run() { key Enter; done_run; }
rval() { local g; g=($(geom 3)); e2e_text $((g[0] + 1)) $((g[0] + g[2] - 2)) $((g[1] + 3)) | awk -F'│' '{ gsub(/ /, "", $2); print $2 }'; }   # ③ 结果第一行第一列
ctitle() { local g; g=($(geom 2)); e2e_text "${g[0]}" $((g[0] + g[2] - 1)) "${g[1]}"; }
click_title() { local g x; g=($(geom 2)); x=$(e2e_find "$1" "${g[1]}" | tr ' ' '\n' | awk -v l="${g[0]}" '$1 > l { print; exit }'); e2e_click "$x" "${g[1]}"; sleep 0.4; }
qbar() { local g; g=($(geom 1)); e2e_text $((g[0] + 1)) $((g[0] + g[2] - 2)) $((g[1] + 2)); }
pane_has() { local g y; g=($(geom "$1")); for ((y = g[1]; y < g[1] + g[3]; y++)); do e2e_text "${g[0]}" $((g[0] + g[2] - 1)) "$y"; done | grep -qF -- "$2"; }
txstate() { bar | grep -oE '事务中|事务出错'; }
sleeping() { e2e_mysql <<<"select count(*) from information_schema.processlist where info like 'select sleep(30)%'"; }

# ---- 树：连接下面一层是 database（不列系统库），连上的展开当前 database 和 Tables
start -C "$D/a"
check "树的根 my 是 mysql 图标（U+E704），连上并展开" eval '[[ $(e2e_text 1 32 "$(trow my)") == *"▾ $MYSQL_ICON my "* ]]'
check "my 下面是 database：agentable、sqlmux（展开，Tables (6)、Views (1)）；information_schema、mysql、performance_schema、sys 不列" eval 's=$(sidebar); [[ $s == *" agentable "* && $s == *"▾ "*" sqlmux "* && $s == *"Tables (6)"* && $s == *"Views (1)"* && $s != *information_schema* && $s != *performance_schema* && $s != *" sys "* && $s != *" mysql "* ]]'
check "状态栏的地址块：my · $MY_ADDR" eval '[[ $(bar) == *" my · $MY_ADDR "* ]]'

# ---- 表格：取数、分页、计数、WHERE
open_table t_order
check "打开 t_order：6000 行，PAGE 1/60，第一行 id 1" eval '[[ $(qbar) == *"PAGE 1/60"* ]] && e2e_plain | grep -qE "│ +1 │ +1 │"'
key ']'; wait_for 5 eval '[[ $(qbar) == *"PAGE 2/60"* ]]'
check "]：PAGE 2/60，第一行 id 101" eval '[[ $(qbar) == *"PAGE 2/60"* ]] && e2e_plain | grep -qE "│ +101 │ +101 │"'
key /; e2e_type "status = 'done'"; key Enter; wait_for 5 eval '[[ $(qbar) == *"PAGE 1/15"* ]]'
check "WHERE status = 'done'：1500 行，PAGE 1/15" eval '[[ $(qbar) == *"PAGE 1/15"* ]]'

# ---- 面板：表的所在位置是 my.sqlmux；DDL 预览是 SHOW CREATE TABLE；快速 SQL
pal "@t_order"; sleep 0.8
check "@t_order：所在位置 my.sqlmux；预览是 SHOW CREATE TABLE 的原文" eval 'list | grep -q "^t_order  my.sqlmux  表|" && preview | head -1 | grep -qF "CREATE TABLE \`t_order\` (" && preview | grep -qF "\`status\` enum('"'"'pending'"'"','"'"'queued'"'"'"'
clear_input; sql "select * from t_order"
check "快速 SQL select * from t_order：自动 LIMIT，结果「100+ 行」" eval '[[ $(title) == "100+ 行"* ]] || { echo "  $(title)"; false; }'
sql "select 5 /*! +1 */ as x"
check "快速 SQL select 5 /*! +1 */：MySQL 执行 /*! */ 里的代码，结果是 6" eval '[[ $(grid | head -1) == 6 ]] || { echo "  $(grid | head -1)"; false; }'
sql "delete from t_log"
check "快速 SQL delete from t_log：写语句不执行" eval '[[ $(line1) == "写语句不在这里执行"* ]]'
key Escape

# ---- console：/*! */、选 database、USE 之后重设
key C-l
check "② console 标题：my · sqlmux ▾（MySQL 只有 database 一段）" eval '[[ $(ctitle) == *" my · sqlmux ▾ "* ]]'
setsql "select 5 /*! +1 */ as x"; run
check "console 里 select 5 /*! +1 */：6" eval '[[ $(rval) == 6 ]] || { echo "  got: $(rval)"; false; }'
setsql "$(printf '/*!40101 select 1 as a */;\nselect 2 as b')"; run
check "第 1 行 /*!40101 select 1 as a */; 上 ↵：它是一条语句（不当注释），结果 1" eval '[[ $(rval) == 1 ]] || { echo "  got: $(rval)"; false; }'
key j; run
check "第 2 行上 ↵：只执行 select 2，结果 2" eval '[[ $(rval) == 2 ]] || { echo "  got: $(rval)"; false; }'
click_title "sqlmux ▾"; e2e_type agentable; sleep 0.3; key Enter
check "点标题的 database 下拉选 agentable：标题 my · agentable ▾" eval '[[ $(ctitle) == *" my · agentable ▾ "* ]]'
setsql "select count(*) from agent"; run
check "执行前先 USE agentable：select count(*) from agent 是 3" eval '[[ $(rval) == 3 ]] || { echo "  got: $(rval)"; false; }'
setsql "use sqlmux"; run; setsql "select database()"; run
check "一次执行里 USE 了别的库：记下的值作废，下次执行又 USE 回 console 的 agentable" eval '[[ $(rval) == agentable ]] || { echo "  got: $(rval)"; false; }'

# ---- KILL QUERY：取消长查询，服务端上的查询停掉，连接还是原来那条
setsql "select connection_id()"; run; cid=$(rval)
setsql "select sleep(30)"; key Enter; sleep 1
check "（准备）select sleep(30) 在服务端跑着，状态栏 busy" eval '[[ $(sleeping) == 1 && $(bar) == *busy* ]]'
key C-c; wait_for 3 eval '[[ $(sleeping) == 0 ]]'
check "C-c：toast「查询已取消」，服务端的 sleep 停了，不再 busy" eval 'toast_is 查询已取消 && [[ $(sleeping) == 0 && $(bar) != *busy* ]]'
setsql "select connection_id()"; run
check "取消用的是 KILL QUERY：之后执行还在同一条连接上（connection_id 没变）" eval '[[ -n $cid && $(rval) == "$cid" ]] || { echo "  $cid → $(rval)"; false; }'

# ---- 编辑保存（自建库）：布尔写 1 / 0，别人改过时报错，只差大小写也查得出
start -x 200 -C "$D/b"
col_x() { e2e_find "$1" "$(hy)" | tr ' ' '\n' | awk '$1 > 34 { print; exit }'; }
row_y() { echo $(( $(grid_y) + $1 )); }
cell() { local x; x=$(col_x "$1"); e2e_text "$x" $((x + ${3:-10})) "$(row_y "$2")" | sed 's/ *│.*//; s/^ *//; s/ *$//'; }
goto() { key g g; key 0; [[ $2 -gt 1 ]] && key $(($2 - 1)) j; local n; n=$(e2e_text 35 $(( $(col_x "$1") - 1 )) $(hy) | tr -cd '│' | wc -m); n=$((n - 1)); [[ $n -gt 0 ]] && key "$n" l; true; }
saves() { local q s; s=$(printf '\xef\x83\x87'); q=$(qbar); q=${q#*"$s"}; q=${q%%[!\ 0-9]*}; echo $q; }
# open_own TABLE：从面板打开自建库里的表。别的库里有同名的表（共用的 sqlmux、别人的 sqlmux_rev_* …），
# 光输入表名时同分的短路径排前面，会打开共用库的表；选中的不是自建库的就退出，绝不在别的库上写
open_own() { key C-p; e2e_type "@$1 $OWN"; sleep 0.4
  [[ $(selected) == "$1  ${CONN:-my}.$OWN  表" ]] || { echo "e2e: palette picked '$(selected)', not $OWN.$1; stopping" >&2; e2e_stop; exit 1; }
  key Enter; wait_for 8 eval '[[ -n $(grid_y) && $(e2e_plain | head -1) == *" $1 ─"* ]]'; }
open_own t_order
goto amount 1; key Enter; e2e_type 9.99; key Enter
goto paid 1; key Enter; key C-n; key Enter
check "paid（tinyint(1)）选 true：第 1 行 paid 显示要写的 1，两处修改" eval '[[ $(cell paid 1) == 1* && $(saves) == 2 ]] || { echo "  paid $(cell paid 1), saves $(saves)"; false; }'
goto paid 5; key Enter; key C-n; key C-n; key Enter
check "第 5 行 paid 原值是 0，选 false：和原值相同，不记修改" eval '[[ $(saves) == 2 ]] || { echo "  saves $(saves)"; false; }'
key C-s; wait_for 8 eval '[[ $(qbar) == *已保存* ]]'
check "C-s：「已保存 1 行」，库里 id 1 的 amount 是 9.99、paid 写成 1" eval '[[ $(qbar) == *"已保存 1 行"* && $(my "select concat(amount, \"/\", paid) from t_order where id = 1") == "9.99/1" ]] || { echo "  $(qbar) / $(my "select concat(amount, \"/\", paid) from t_order where id = 1")"; false; }'
wait_for 5 eval '[[ $(cell amount 2) == 2.99 && -z $(saves) ]]'; sleep 0.5        # 保存之后的重取回来了，再让别人改
my "update t_order set amount = 7.77 where id = 2"
goto amount 2; key Enter; e2e_type 8.88; key Enter; key C-s; sleep 1
check "别人把 id 2 改成了 7.77：保存报「id = 2 的行数据已变化或行不存在，已回滚」，库里还是 7.77" eval '[[ $(errbar 1) == "id = 2 的行数据已变化或行不存在，已回滚"* && $(my "select amount from t_order where id = 2") == 7.77 ]] || { echo "  $(errbar 1)"; false; }'
open_own t_user
my "update t_user set name = 'USER_3' where id = 3"
goto name 3; key Enter; e2e_type x3; key Enter; key C-s; sleep 1
check "别人把 user_3 改成 USER_3（排序规则下相等）：旧值按字节比，照样报行数据已变化，库里还是 USER_3" eval '[[ $(errbar 1) == "id = 3 的行数据已变化或行不存在，已回滚"* && $(my "select name from t_user where id = 3") == USER_3 ]] || { echo "  $(errbar 1)"; false; }'

# ---- 事务：按执行过的语句记状态；手动模式下不提交，✓ 提交
key C-l
setsql "begin"; run
check "console 里 begin：状态栏「事务中」" eval '[[ $(txstate) == 事务中 ]]'
setsql "delete from t_log"; run
check "（准备）事务里删了 t_log：别的连接看到的还是 3 行" eval '[[ $(my "select count(*) from t_log") == 3 ]]'
setsql "select 1 from no_such_table"; run
check "MySQL 出错不中止事务：仍是「事务中」，不会是「事务出错」" eval '[[ $(txstate) == 事务中 ]]'
setsql "rollback"; run
check "rollback：事务块消失，t_log 还是 3 行" eval '[[ -z $(txstate) && $(my "select count(*) from t_log") == 3 ]]'
click_title "Tx:"; e2e_type 手动; sleep 0.3; key Enter
setsql "update t_order set amount = 5.55 where id = 5"; run
check "手动模式下 update：没提交（别的连接看到 5.99），「事务中」" eval '[[ $(my "select amount from t_order where id = 5") == 5.99 && $(txstate) == 事务中 ]]'
click_title "✓"; wait_for 5 eval '[[ -z $(txstate) ]]'
check "点 ✓ 提交：库里变成 5.55，事务块消失" eval '[[ $(my "select amount from t_order where id = 5") == 5.55 && -z $(txstate) ]]'

# ---- 只读连接：数据库层 SET SESSION TRANSACTION READ ONLY，应用层整次不发，表格拦下
start -C "$D/ro"; key C-l
setsql "select @@transaction_read_only"; run
check "ro 的 Main：@@transaction_read_only 是 1" eval '[[ $(rval) == 1 ]] || { echo "  got: $(rval)"; false; }'
setsql "delete from t_log"; run
check "delete from t_log：整次不发，日志「ro 是只读连接，整次都没有执行」，t_log 还是 3 行" eval '[[ $(tabbar 3) == "1:日志*"* ]] && pane_has 3 "ro 是只读连接，整次都没有执行" && [[ $(my "select count(*) from t_log") == 3 ]]'
setsql "set session transaction read write"; run
check "SET SESSION TRANSACTION READ WRITE：拦下" eval 'pane_has 3 "set session transaction read write"'
setsql "start transaction read write"; run
check "START TRANSACTION READ WRITE：拦下" eval 'pane_has 3 "start transaction read write"'
setsql "$(printf 'use agentable;\nselect count(*) from agent')"; key V; key j; key Enter; done_run
check "USE 放行：use agentable 之后 select count(*) from agent 是 3" eval '[[ $(rval) == 3 ]] || { echo "  got: $(rval)"; false; }'
key C-h; CONN=ro open_own t_user; key l; key Enter
check "ro 的表格上 ↵ 编辑：toast「ro 是只读连接」" eval 'toast_is "ro 是只读连接" && mode_is NORMAL'

# ---- 表单：engine 用空格切成 mysql
CT=$E2E_TMP/config/sqlmux/connections.toml
pal ">conn.new"; key Enter; e2e_type m2; key Tab; key Space; key Tab; e2e_type "$MY_DSN"; key Enter
check "新建连接 engine 切成 mysql：追加的一段是 engine = \"mysql\"，树里多出 m2（mysql 图标、未连接）" eval 'grep -q "^engine = \"mysql\"" <(tail -5 "$CT") && [[ $(e2e_text 1 32 "$(trow m2)") == *"▸ $MYSQL_ICON m2 "* ]]'
y=$(trow m2); e2e_click "$(e2e_find m2 "$y" | cut -d' ' -f1)" "$y"; wait_for 6 eval '[[ $(e2e_text 1 32 "$(trow m2)") == *"▾ "* ]]'
check "单击 m2：连上并展开" eval '[[ $(e2e_text 1 32 "$(trow m2)") == *"▾ $MYSQL_ICON m2 "* ]]'

e2e_done
