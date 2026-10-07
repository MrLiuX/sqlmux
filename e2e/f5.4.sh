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
y=$(trow agentable); e2e_click "$(e2e_find agentable "$y" | cut -d' ' -f1)" "$y"; sleep 0.5
pal; sql "select count(*) as n from agent"
check "树的光标移到 agentable 上：快速 SQL 先 USE 当前连接的 Schema（agentable），select count(*) from agent 是 3" eval '[[ $(grid | head -1) == 3 ]] || { echo "  $(title) / $(line1) / $(grid | head -1)"; false; }'
key Escape

# ---- console：/*! */、选 database、USE 之后重设
key C-l; key C-l
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
pal "@t_order"
check "只输入表名：同名的表同分，当前连接的 Schema（自建库）那张排第一（68337a7，测试时定）" eval '[[ $(selected) == "t_order  my.$OWN  表" ]] || { echo "  selected: $(selected)"; false; }'
key Escape
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
bad=; for q in "flush tables" "kill 1" "set global max_connections = 200" "set persist max_connections = 200" "reset persist" "optimize table t_log" "repair table t_log" "xa start 'x'"; do
  setsql "$q"; run; [[ $(tabbar 3) == "1:日志*"* ]] && pane_has 3 "$q" || bad+=" [$q]"; done
check "flush、kill、SET GLOBAL / PERSIST、reset persist、optimize、repair、xa：都拦下" eval '[[ -z $bad ]] || { echo "  not blocked:$bad"; false; }'
setsql 'select 1; /*! SET @a = 1 /*! , transaction_read_only = 0 */ , @b = 2'; key V; key Enter; sleep 0.8
check "/*! SET @a = 1 /*! , transaction_read_only = 0 */ , @b = 2（嵌套的 /*! 也是代码）：拦下" eval 'pane_has 3 "/*! SET @a = 1 /*! , transaction_read_only = 0 */ , @b = 2" && [[ $(tabbar 3) == "1:日志*"* ]]'
setsql "check table t_log"; run
check "CHECK TABLE 放行：结果第一列是 $OWN.t_log" eval '[[ $(rval) == "$OWN.t_log" ]] || { echo "  got: $(rval)"; false; }'
key C-h; CONN=ro open_own t_user; key l; key Enter
check "ro 的表格上 ↵ 编辑：toast「ro 是只读连接」" eval 'toast_is "ro 是只读连接" && mode_is NORMAL'

# ---- 表单：engine 用空格切成 mysql
CT=$E2E_TMP/config/sqlmux/connections.toml
pal ">conn.new"; key Enter; form_dsn m2 "$MY_DSN" "" mysql
check "新建连接 engine 切成 mysql：追加的一段是 engine = \"mysql\"，树里多出 m2（mysql 图标、未连接）" eval 'grep -q "^engine = \"mysql\"" <(tail -5 "$CT") && [[ $(e2e_text 1 32 "$(trow m2)") == *"▸ $MYSQL_ICON m2 "* ]]'
y=$(trow m2); e2e_click "$(e2e_find m2 "$y" | cut -d' ' -f1)" "$y"; wait_for 6 eval '[[ $(e2e_text 1 32 "$(trow m2)") == *"▾ "* ]]'
check "单击 m2：连上并展开" eval '[[ $(e2e_text 1 32 "$(trow m2)") == *"▾ $MYSQL_ICON m2 "* ]]'

# ---- 审查修复（d99727f）：值照原文显示和保存、BIT 只读、enum 里的反斜杠、latin1 / gbk、只读的表、新行全默认
my "$(cat <<'SQL'
create table t_types (id int primary key, c_double double, c_float float(10,2), c_year year, c_zerofill int(3) zerofill, c_bit bit(8), c_enum enum('a\\b','c'), c_latin1 varchar(10) character set latin1, c_gbk varchar(10) character set gbk);
insert into t_types values (1, 1500000, 1.5, 0, 7, b'101', 'a\\b', 'café', '中文');
create table t_bin (id binary(16) primary key, v int);
insert into t_bin values (unhex('00112233445566778899aabbccddeeff'), 1);
create table t_isam (id int primary key, v int) engine = MyISAM;
insert into t_isam values (1, 1);
create table t_def (id bigint auto_increment primary key, note varchar(10) default 'x');
create procedure p42() select 42 as v
SQL
)"
opts_box() { e2e_panes | awk '$1 == "-" { print $2, $3, $4, $5; exit }'; }
opts() { local g y; g=($(opts_box)); [[ -n ${g[0]} ]] || return 0
  for ((y = g[1] + 1; y < g[1] + g[3] - 1; y++)); do e2e_text $((g[0] + 2)) $((g[0] + g[2] - 2)) $y | tr -s ' ' | sed 's/^ //; s/ $//'; done | tr '\n' /; }
picked() { local g y; g=($(opts_box)); for ((y = g[1] + 1; y < g[1] + g[3] - 1; y++)); do style_has $((g[0] + g[2] - 2)) $y bg=#364a82 >/dev/null && e2e_text $((g[0] + 2)) $((g[0] + g[2] - 2)) $y | tr -s ' ' | sed 's/^ //; s/ $//'; done; }
start -x 240 -C "$D/b"
open_own t_types
check "值照服务端原文显示：double 1500000、float(10,2) 1.50、year 0000、zerofill 007、bit 显示成数字 5、enum a\\b、latin1 café、gbk 中文" eval '[[ $(cell c_double 1) == 1500000 && $(cell c_float 1) == 1.50 && $(cell c_year 1) == 0000 && $(cell c_zerofill 1) == 007 && $(cell c_bit 1) == 5 && $(cell c_enum 1) == "a\\b" && $(cell c_latin1 1) == café && $(cell c_gbk 1) == 中文 ]] || { for c in c_double c_float c_year c_zerofill c_bit c_enum c_latin1 c_gbk; do printf "%s=%s " $c "$(cell $c 1)"; done; echo; false; }'
goto c_double 1; key Enter; e2e_type 1500001; key Enter
goto c_float 1; key Enter; e2e_type 2.50; key Enter
goto c_latin1 1; key Enter; e2e_type naïve; key Enter
goto c_gbk 1; key Enter; e2e_type 汉字; key Enter
key C-s; wait_for 8 eval '[[ $(qbar) == *已保存* || -n $(errbar 1) ]]'
check "改 double、float、latin1、gbk 后 C-s：「已保存 1 行」，库里都是新值（旧值校验按原文对得上）" eval '[[ $(qbar) == *"已保存 1 行"* && $(my "select concat_ws(\"/\", c_double, c_float, convert(c_latin1 using utf8mb4), convert(c_gbk using utf8mb4)) from t_types") == "1500001/2.50/naïve/汉字" ]] || { echo "  $(qbar) / $(errbar 1) / $(my "select concat_ws(\"/\", c_double, c_float, convert(c_latin1 using utf8mb4), convert(c_gbk using utf8mb4)) from t_types")"; false; }'
goto c_bit 1; key Enter
check "改 BIT 列：toast「c_bit 是 bit(8)，只读」，不进编辑" eval 'toast_is "c_bit 是 bit(8)，只读" && mode_is NORMAL'
goto c_enum 1; key Enter
check "enum 的选项照标签原文显示：a\\b、c" eval '[[ $(opts) == "a\\b 值/c 值/"* ]] || { echo "  $(opts)"; false; }'
e2e_type c; key Enter; key C-s; wait_for 8 eval '[[ $(my "select c_enum from t_types") == c ]]'
goto c_enum 1; key Enter; key C-n; key Enter; key C-s; wait_for 8 eval '[[ $(my "select hex(c_enum) from t_types") == 615C62 ]]'
check "先存成 c，再选回 a\\b 存：库里是 a\\b（反斜杠原样，hex 615C62）" eval '[[ $(my "select hex(c_enum) from t_types") == 615C62 ]]'
open_own t_bin; key l; key Enter
check "行标识列是 binary(16) 的表：toast「t_bin 的行标识列 id 是 binary(16)，只读」" eval 'toast_is "t_bin 的行标识列 id 是 binary(16)，只读" && mode_is NORMAL'
open_own t_isam; key l; key Enter
check "MyISAM 的表：toast「t_isam 的引擎 MyISAM 不支持事务，只读」" eval 'toast_is "t_isam 的引擎 MyISAM 不支持事务，只读" && mode_is NORMAL'
open_own t_def; key o; key 0; key Enter
check "auto_increment 列在新行上提供 DEFAULT" eval '[[ $(opts) == *DEFAULT* ]] || { echo "  $(opts)"; false; }'
for i in 1 2 3 4 5; do [[ $(picked) == DEFAULT* ]] && break; key C-n; done; key Enter; key C-s; wait_for 8 eval '[[ $(my "select count(*) from t_def") == 1 ]]'
check "一格都没改（id 选 DEFAULT）的新行：保存成功，插进一行，note 是默认的 x" eval '[[ $(my "select concat(count(*), \"/\", max(note)) from t_def") == 1/x ]]'
goto id 1; key Enter
check "已有的行上 auto_increment 列不提供 DEFAULT" eval '[[ $(opts) != *DEFAULT* ]] || { echo "  $(opts)"; false; }'
key Escape; key Escape

# ---- console：call、嵌套的 /*!、整条 /*! … */
key C-l
setsql "call p42()"; run
check "call 返回的行能显示：42" eval '[[ $(rval) == 42 ]] || { echo "  got: $(rval)"; false; }'
setsql "SELECT 1 /*! +1 /*! +2 */"; run
check "SELECT 1 /*! +1 /*! +2 */：嵌套的 /*! 也是代码，结果 4" eval '[[ $(rval) == 4 ]] || { echo "  got: $(rval)"; false; }'
setsql "/*! SELECT 1 /* c */ */"; run
check "/*! SELECT 1 /* c */ */：整条发出去，结果 1" eval '[[ $(rval) == 1 ]] || { echo "  got: $(rval)"; false; }'
setsql "rename table t_def to t_def2"; run; wait_for 5 eval '[[ -n $(trow t_def2) ]]'
tcat() { local y t; for y in $(seq 3 43); do t=$(e2e_text 1 32 "$y"); [[ $t == *" 工作区 "* ]] && return; [[ $t == *" $1 "* ]] && { echo "$y"; return; }; done; }   # 只在 catalog 那部分找（工作区里有同名的 tab）
check "RENAME TABLE 之后 catalog 重新加载：树里是 t_def2，没有 t_def" eval '[[ -n $(tcat t_def2) && -z $(tcat t_def) ]]'

# ---- 取消：一次执行多条写语句，C-c 之后后面的不再执行
setsql "$(printf "update t_log set msg = 'c1' where msg = 'plain';\nselect sleep(5);\nupdate t_log set msg = 'c3' where msg = 'c1'")"; key V; key G; key Enter; sleep 1.5
key C-c; wait_for 5 eval '[[ $(bar) != *busy* ]]'; sleep 0.5
check "执行到 sleep 时 C-c：第一条写进去了（c1），第三条没执行（没有 c3）" eval '[[ $(my "select count(*) from t_log where msg = '"'c1'"'") == 1 && $(my "select count(*) from t_log where msg = '"'c3'"'") == 0 ]]'

# ---- 手动事务：DDL 隐式提交后重新补 BEGIN；rollback to savepoint 仍在事务里；SET TRANSACTION ISOLATION 能执行
click_title "Tx:"; e2e_type 手动; sleep 0.3; key Enter
setsql "update t_order set amount = 11.11 where id = 11"; run
setsql "create table t_x (i int)"; run
check "手动模式下 update 之后执行 DDL：服务端隐式提交了，状态记为不在事务里（11.11 已提交）" eval '[[ -z $(txstate) && $(my "select amount from t_order where id = 11") == 11.11 ]]'
setsql "update t_order set amount = 12.12 where id = 12"; run
check "再 update：重新补了 BEGIN，「事务中」" eval '[[ $(txstate) == 事务中 && $(my "select amount from t_order where id = 12") == 12.99 ]]'
setsql "savepoint s"; run; setsql "rollback work to savepoint s"; run
check "rollback work to savepoint s：仍在事务里" eval '[[ $(txstate) == 事务中 ]]'
click_title "↺"; wait_for 5 eval '[[ -z $(txstate) ]]'
check "↺：DDL 之后那条撤掉了（12.99），DDL 之前的撤不掉（11.11）" eval '[[ $(my "select amount from t_order where id = 12") == 12.99 && $(my "select amount from t_order where id = 11") == 11.11 ]]'
setsql "set transaction isolation level read committed"; run
check "手动模式下 SET TRANSACTION ISOLATION LEVEL：能执行，没有报错" eval '[[ -z $(errbar 2) ]] || { errbar 2; false; }'

# ---- 手动模式下保存、取数：不报 SAVEPOINT 不存在；✓ 提交
key C-h; open_own t_order
goto amount 13; key Enter; e2e_type 13.13; key Enter; key C-s; wait_for 8 eval '[[ $(qbar) == *已保存* || -n $(errbar 1) ]]'
check "手动模式下 C-s：「已保存 1 行（未提交）」，没有错误栏，库里还是 13.99" eval '[[ $(qbar) == *"已保存 1 行（未提交）"* && -z $(errbar 1) && $(my "select amount from t_order where id = 13") == 13.99 ]] || { echo "  $(qbar) / $(errbar 1)"; false; }'
key C-l; click_title "✓"; wait_for 5 eval '[[ -z $(txstate) ]]'
check "✓：提交，库里 13.13" eval '[[ $(my "select amount from t_order where id = 13") == 13.13 ]]'

# ---- 死锁：toast「死锁，整个事务已回滚」，事务记为结束
# 另一条连接先改 302 行、锁住 id 22，3 秒后再去改 id 21；sqlmux（手动模式）先改 21、再改 22 等它的锁。InnoDB 回滚改得少的那个，就是 sqlmux
setsql "$(printf 'update t_order set amount = 21.12 where id = 21;\nupdate t_order set amount = 22.12 where id = 22')"   # 先写好两行，按键要赶在另一条连接的 sleep 之内
e2e_mysql "$OWN" >/dev/null 2>&1 <<'SQL' &
begin;
update t_order set note = 'dl' where id between 100 and 400;
update t_order set amount = 22.22 where id = 22;
select sleep(4);
update t_order set amount = 21.21 where id = 21;
commit;
SQL
DL=$!; sleep 0.5
run; key j; key Enter                                                         # 改 21，再去改 22（等另一条连接的锁）
wait "$DL"; wait_for 5 eval '[[ $(bar) != *busy* ]]'; sleep 0.3
check "死锁、sqlmux 这边被回滚：toast「死锁，整个事务已回滚」，没有「事务中」，另一边提交了（21.21、22.22）" eval 'toast_is "死锁，整个事务已回滚" && [[ -z $(txstate) && $(my "select concat(amount) from t_order where id in (21, 22) order by id" | tr "\n" " ") == "21.21 22.22 " ]]'

# ---- 驱动的日志不写到终端：Main 被服务端断开之后，屏幕上没有 [mysql] 开头的行
click_title "Tx:"; e2e_type 自动; sleep 0.3; key Enter
setsql "select connection_id()"; run; cid=$(rval)
my "kill $cid"; sleep 0.5; setsql "select 1"; run; sleep 1
check "连接被服务端 kill 之后再执行：屏幕上没有驱动的 [mysql] 日志" eval '[[ -n $cid ]] && ! e2e_plain | grep -qF "[mysql]"'

# ---- DSN 里带 parseTime、columnsWithAlias：值照样是服务端原文，列名不带表名
mkdir -p "$D/p"; (umask 077; printf '[[connection]]\nname = "my"\nengine = "mysql"\ndsn = "%s?parseTime=true&columnsWithAlias=true"\n' "$E2E_MYDB" >"$D/p/connections.toml")
start -x 200 -C "$D/p"; key C-l
setsql "select created_at from t_user where id = 1"; run
check "parseTime=true、columnsWithAlias=true：列名是 created_at（不带 t_user.），值是 2026-01-02 00:00:00" eval 'g=($(geom 3)); [[ $(e2e_text $((g[0] + 1)) $((g[0] + g[2] - 2)) $((g[1] + 1))) == *" created_at "* && $(e2e_text $((g[0] + 1)) $((g[0] + g[2] - 2)) $((g[1] + 1))) != *t_user.* ]] && e2e_text $((g[0] + 1)) $((g[0] + g[2] - 2)) $((g[1] + 3)) | grep -q "2026-01-02 00:00:00"'

e2e_done
