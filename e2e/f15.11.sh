#!/usr/bin/env bash
# F15.11 补全的同类小项（specs/m15-pg-databases/task.md F15.11；tech-design §11「同类小项」、§9.7）
# ① INSERT INTO t 之后补全给「(全部列) VALUES ()」骨架，VALUES 括号里一个 tabstop（Tab 到括号之后、结束），generated 的列不在里面；
# ② UPDATE t SET 之后的列补全只列这张表的列；③ 补全插入的标识符需要引号时加引号（大小写混合、有空格、关键字；PG 和 MySQL 的反引号），
# WHERE 输入框也一样。单测覆盖三项，这里测真实终端里的弹出、接受后的文字、光标和 Tab。
. "$(dirname "$0")/lib.sh"
e2e_build || exit 1
. "$(dirname "$0")/palette.sh"

pop() { e2e_panes | awk '$1 == "-" { print $2, $3, $4, $5 }' | tail -1; }
items() { local g y; g=($(pop)); [[ -n ${g[0]} ]] || return 0
  for ((y = g[1] + 1; y < g[1] + g[3] - 1; y++)); do e2e_text $((g[0] + 2)) $((g[0] + g[2] - 2)) $y | sed 's/^ *//; s/ *$//'; done; }
names() { items | sed 's/  .*//' | tr '\n' '|'; }
names_are() { [[ $(names) == "$1" ]] || { echo "  candidates: '$(names)', want '$1'"; false; }; }
none() { [[ -z $(pop) ]] || { echo "  popup: $(names)"; false; }; }
line() { e2e_text 104 159 $(($1 + 1)) | sed 's/ *$//'; }
newest() { local f; f=$(ls -t "$E2E_TMP/data/sqlmux/queries"/*.sql 2>/dev/null | head -1); [[ -n $f ]] && cat "$f"; }   # the query's text on disk: the line on screen scrolls
line_is() { local got; sleep 1.6; got=$(newest | tr -d '\n' | sed 's/ *$//'); [[ $got == "$2" ]] || { echo "  text: '$got', want '$2'"; false; }; }
fresh() { key Escape; key Escape; key g g; key d G; key i; }
at() { fresh; e2e_type "$1"; sleep 1.2; }
wi() { e2e_text 42 96 2 | sed -E 's/ +[^ ]+ C-r.*//; s/ *$//'; }   # the WHERE input's text (the data pane is the left one here); its right end is [icon] C-r ▾
# pick N：选候选里的第 N 项（从 0 起）：候选先不选中，第一个 Down 选第一项
pick() { local i; for ((i = 0; i <= $1; i++)); do key Down; done; key Enter; sleep 0.4; }

D=$(mktemp -d "${TMPDIR:-/tmp}/sqlmux-e2e-cfg.XXXXXX"); trap 'e2e_stop; rm -rf "$D"' EXIT
e2e_own_db || exit 1
psql "$E2E_DB" -q -v ON_ERROR_STOP=1 \
  -c 'create table "Mixed" ("Id" int primary key, "user" text, "created at" timestamptz default now(), plain text, "Status" text)' \
  -c "create table t_gen (id int generated always as identity, a int, b int generated always as (a * 2) stored, c text)" \
  -c "create table t_k (a int, b text)" \
  -c "create table t_onlygen (id int generated always as identity)" || exit 1
printf '[[connection]]\nname = "doraemon"\nengine = "postgres"\ndsn = "%s"\n' "$E2E_DB" >"$D/connections.toml"

start -C "$D"; key C-l; key i

# ---- ① INSERT INTO t 之后的骨架
at "insert into t_user "
check "INSERT INTO t_user 之后：一项骨架（全部列，放不下截断加 …）+ 注释「全部 5 列」" eval 'names_are "(id, name, email, active…) values ()|" && items | head -1 | grep -q "全部 5 列"'
key Enter; sleep 0.3
check "候选先不选中：↵ 是换行（没有接受），骨架没写进去" eval '[[ $(line 1) == "insert into t_user" && -z $(pop) ]]' || line 1
at "insert into t_user "
key Tab; key Enter; sleep 0.4
check "Tab 选中、↵ 接受：列清单是普通文字，括号里 values ()" line_is 1 "insert into t_user (id, name, email, active, created_at) values ()"
# 短一点的 t_k（a, b），整行放得进窗口，光标和 Tab 能直接看屏幕
at "insert into t_k "
key Tab; key Enter; sleep 0.4
check "t_k：接受后 insert into t_k (a, b) values ()，光标在 values 的括号里（) 前），INSERT 模式" eval '[[ $(line 1) == "insert into t_k (a, b) values ()" && $(( $(e2e_flag cursor_x) - 103 )) -eq 31 ]] && mode_is INSERT' || { line 1; echo "  cursor $(( $(e2e_flag cursor_x) - 103 ))"; }
e2e_type "1, 'x15'"; sleep 0.5
check "直接打值：括号里是 1, 'x15'" eval '[[ $(line 1) == "insert into t_k (a, b) values (1, '"'"'x15'"'"')" ]]' || line 1
key Tab; sleep 0.3; e2e_type '\;'; sleep 0.4   # tmux reads a bare ; as the end of a command
check "Tab：到括号之后（最后一个 tabstop 结束），再打 ; 在 ) 后面" eval '[[ $(line 1) == "insert into t_k (a, b) values (1, '"'"'x15'"'"');" ]]' || line 1
key Escape; key Enter; sleep 1.5
check "骨架能直接执行：esc ↵ 在库里插入了这一行" eval '[[ $(psql "$E2E_DB" -Atc "select count(*) from t_k where b = '"'"'x15'"'"'") == 1 ]]'
at "insert into t_gen "
check "generated 的列不在骨架里（id identity always、b 生成列）：(a, c) values ()" names_are "(a, c) values ()|"
at "insert into t_onlygen "
check "只有 generated 列的表：不给骨架" none
at "insert into t_nosuch "
check "catalog 里没有的表：不弹" none
at "insert into t_user (id) "
check "列清单已经写了：不再给骨架" none
at "insert into t_user v"
check "已经打了一个词：不弹骨架（弹的是别的词）" eval '! names | grep -q "values ()"'
at "insert into public.t_user "
check "带 schema 的表名也给骨架" eval 'names_are "(id, name, email, active…) values ()|"'

# ---- ② UPDATE t SET 只列这张表的列
at "update t_user set "
check "UPDATE t_user SET 之后：只有 t_user 的五列，没有别的表的列" names_are "id|name|email|active|created_at|"
at "update t_user set name = 'x', "
check "第二个赋值：同样只有 t_user 的列" names_are "id|name|email|active|created_at|"
at "update t_user set na"
check "打了前缀 na：只有 name" names_are "name|"
at "update t_order set id = 1 where "
check "WHERE 之后不是 SET：列还是那张表的，但这是普通的列补全" eval 'names | grep -q "^id|" && ! names | grep -q "email"'
fresh; e2e_type "update t_order set x from t_user"; key Escape; key 0; key f x; key s; sleep 0.3; e2e_type e; sleep 1
check "SET 后面有 FROM t_user：打 e 只补 t_order 的列（没有 t_user 的 email）" eval '! names | grep -q "email"'
key Escape; key Escape

# ---- ③ 需要引号的名字加引号（PG）
at 'select * from "Mixed" where '
check "候选的文字是库里的名字（不带引号）：Id user created at plain Status" names_are "Id|user|created at|plain|Status|"
pick 1
check "选 user（PG 的保留字）：写成 \"user\"" line_is 1 'select * from "Mixed" where "user"'
at 'select * from "Mixed" where '
pick 0
check "选 Id（有大写）：写成 \"Id\"" line_is 1 'select * from "Mixed" where "Id"'
at 'select * from "Mixed" where '
pick 2
check "选 created at（有空格）：写成 \"created at\"" line_is 1 'select * from "Mixed" where "created at"'
at 'select * from "Mixed" where '
pick 3
check "选 plain（不用引号）：原样 plain" line_is 1 'select * from "Mixed" where plain'
at 'select * from "Mixed" where us'
key Enter; sleep 0.4
check "打了前缀 us 再 ↵：整个词换成 \"user\"（不留下 us）" line_is 1 'select * from "Mixed" where "user"'
at "select * from Mi"
key Enter; sleep 0.4
check "表名大小写混合：补全写成 \"Mixed\"" line_is 1 'select * from "Mixed"'
at "select * from t_us"
key Enter; sleep 0.4
check "普通的表名 t_user 不加引号" line_is 1 'select * from t_user'
at 'select "Mixed".'
pick 1
check "限定名之后的列：\"Mixed\".\"user\"" line_is 1 'select "Mixed"."user"'
at 'update "Mixed" set '
pick 4
check "UPDATE 的 SET 里的列：\"Status\"" line_is 1 'update "Mixed" set "Status"'
at 'insert into "Mixed" '
check "INSERT 的骨架：显示的是原名（Id, user, created at, plain, Status）" eval 'names_are "(Id, user, created at, p…) values ()|"'
key Tab; key Enter; sleep 0.4
check "接受骨架：每个要引号的列都带引号，plain 不带" line_is 1 'insert into "Mixed" ("Id", "user", "created at", plain, "Status") values ()'
at 'insert into "Mixed"('
check "INSERT 的列清单的补全：全部列也带引号（选第一项全部列）" eval 'names | grep -q "^Id, user, created at, plain, Status|"'
key Enter; sleep 0.4
check "接受全部列：\"Id\", \"user\", \"created at\", plain, \"Status\"" line_is 1 'insert into "Mixed"("Id", "user", "created at", plain, "Status")'

# ---- WHERE 输入框（表 tab 的筛选）也一样
start -C "$D"
open_table Mixed; wait_for 8 settled
key /; sleep 0.4; e2e_type us; sleep 1
key Tab; key Enter; sleep 0.4
check "WHERE 输入框：us 补全成 \"user\"" eval '[[ $(wi) == "\"user\"" ]]' || wi
e2e_type "= 'x' and cre"; sleep 1
key Tab; key Enter; sleep 0.4
check "WHERE 输入框：and 之后的 cre 补全成 \"created at\"" eval '[[ $(wi) == "\"user\" = '"'"'x'"'"' and \"created at\"" ]]' || wi
key Escape; key Escape
e2e_stop

# ---- MySQL：反引号；auto_increment 可以写，生成列不在骨架里
e2e_own_mydb || exit 1
e2e_mysql "sqlmux_e2e_$$" <<<'create table mx (`Key` int, `a b` int, Mixed int, plain int, g int as (plain + 1) stored); create table `order` (id int, v text)' || exit 1
printf '[[connection]]\nname = "my"\nengine = "mysql"\ndsn = "%s"\n' "$E2E_MYDB" >"$D/connections.toml"
start -C "$D"; key C-l; key i
at "insert into mx "
check "MySQL：骨架显示原名，generated 的 g 不在里面" names_are "(Key, a b, Mixed, plain) values ()|"
key Tab; key Enter; sleep 0.4
check "MySQL：接受后：Key（关键字）和 a b（有空格）加反引号，Mixed / plain 不加" line_is 1 'insert into mx (`Key`, `a b`, Mixed, plain) values ()'
at "select * from mx where "
pick 0
check "MySQL：WHERE 里选 Key：\`Key\`" line_is 1 'select * from mx where `Key`'
at "select * from mx where a"
key Enter; sleep 0.4
check "MySQL：前缀 a 补全 a b：\`a b\`（整个词换掉）" line_is 1 'select * from mx where `a b`'
at "select * from ord"
key Enter; sleep 0.4
check "MySQL：表名 order 是关键字：\`order\`" line_is 1 'select * from `order`'
at "update mx set pl"
key Enter; sleep 0.4
check "MySQL：UPDATE 的 SET：plain 不加引号" line_is 1 'update mx set plain'
at "update t_order o join t_user u on u.id = o.user_id set st"
check "MySQL：update … join … set st：第一张表 t_order 的列（status）" names_are "status|"
at "update t_order o join t_user u on u.id = o.user_id set em"
check "MySQL：update … join … set em：第二张表 t_user 的列（email）也列（两张都列，docs e6600d4）" names_are "email|"
at "update mx set "
check "MySQL：UPDATE mx SET：只列 mx 的列" eval 'names_are "Key|a b|Mixed|plain|g|"'

e2e_done
