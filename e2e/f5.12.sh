#!/usr/bin/env bash
# F5.12 补全：接受关键字补空格，补齐 DML / DDL 关键字（specs/m5-workspace/task.md F5.12；tech-design §9.7「接受关键字时补一个空格」）
# 补空格和光标的细节有单测；这里测真实终端里 console、WHERE、快速 SQL 三处的接受、↵ 不执行 / 照常执行，以及 DML / DDL 候选。只读，用共用库。
. "$(dirname "$0")/lib.sh"
e2e_build || exit 1
. "$(dirname "$0")/palette.sh"

paste() { t set-buffer -b e2e "$1"; t paste-buffer -r -p -b e2e -t t; sleep 0.4; }
pop() { e2e_panes | awk '$1 == "-" { print $2, $3, $4, $5 }' | tail -1; }
items() { local g y; g=($(pop)); [[ -n ${g[0]} ]] || return 0
  for ((y = g[1] + 1; y < g[1] + g[3] - 1; y++)); do e2e_text $((g[0] + 2)) $((g[0] + g[2] - 2)) $y | sed 's/^ *//; s/ *$//'; done; }
names() { items | sed 's/  .*//' | tr '\n' '|'; }          # 候选名（可能带空格），以 | 分隔
has() { [[ "|$(names)" == *"|$1|"* ]] || { echo "  candidates: $(names)"; false; }; }
line() { e2e_text 111 159 $(($1 + 1)) | sed 's/ *$//'; }
cc() { echo "$(e2e_flag cursor_y):$(( $(e2e_flag cursor_x) - 110 ))"; }      # 光标所在行（0 起）和它前面的字符数（第一行）
fresh() { key Escape; key Escape; key g g; key d G; key i; }
settle() { wait_for 8 settled; }

start; key C-l; key i

# ---- console：selec ↵ → "select "，光标落在空格后（状态栏的列号 = 文字长度 + 1）
e2e_type "selec"; sleep 0.5
check "console 输入 selec：弹出 select" has select
key Enter
check "↵ 接受：select + 一个空格，光标在空格后（前面 7 个字符），列表关了，仍在 INSERT" eval '[[ $(line 1) == "select" && $(cc) == 1:7 && -z $(pop) ]] && mode_is INSERT'
fresh; e2e_type "select 1 from t_user wher"; sleep 0.5; key Enter
check "console：wher ↵ → where + 空格（前面 27 个字符），没有换行" eval '[[ $(line 1) == "select 1 from t_user where" && $(cc) == 1:27 && $(line 2) == "" ]]'
fresh; e2e_type "select * from t_user u wher"; sleep 0.5; key Enter; e2e_type "u.i"; sleep 0.5
check "补空格之后接着打：where u.i 弹出列 id（列不补空格的前提）" has id
key Enter
check "接受列名 id：不补空格（光标就在 id 后）" eval '[[ $(line 1) == "select * from t_user u where u.id" ]] && [[ $(cc) == 1:33 ]]'
fresh; e2e_type "select * from t_us"; sleep 0.5; key Enter
check "接受表名 t_user：不补空格" eval '[[ $(line 1) == "select * from t_user" ]]'
fresh; e2e_type "select * from t_user where id is nul"; sleep 0.5; key Enter
check "接受值 null：不补空格，没有换行" eval '[[ $(line 1) == "select * from t_user where id is null" && $(line 2) == "" ]]'
fresh; e2e_type "select coun"; sleep 0.5; key Enter
check "接受函数 count：不补空格" eval '[[ $(line 1) == "select count" ]]'

# ---- 光标后已经是空白：文字不变、光标移过去，↵ 不换行
fresh; e2e_type " x"; key Escape; key 0; key i; e2e_type "selec"; sleep 0.5
check "（准备）selec| x：光标在 selec 后，弹出 select" has select
key Enter
check "光标后已有空格：不多补，文字还是 'select x'，光标移到空格后（7）" eval '[[ $(line 1) == "select x" && $(cc) == 1:7 ]]'

# ---- DML / DDL 候选
fresh; e2e_type "ins"; sleep 0.5
check "console 输入 ins：有 insert into" has "insert into"
fresh; e2e_type "upd"; sleep 0.5
check "console 输入 upd：有 update" has update
fresh; e2e_type "del"; sleep 0.5
check "console 输入 del：有 delete from" has "delete from"
fresh; e2e_type "ins"; sleep 0.5; key Enter
check "接受 insert into：补一个空格（带空格的整条候选）" eval '[[ $(line 1) == "insert into" && $(cc) == 1:12 ]]'
fresh; e2e_type "crea"; sleep 0.5
check "crea：有 create table / create index / create view" eval 'has "create table" && has "create index" && has "create view"'
fresh; e2e_type "trunc"; sleep 0.5
check "trunc：有 truncate" has truncate
fresh; e2e_type "roll"; sleep 0.5
check "roll：有 rollback" has rollback
fresh; e2e_type "inner"; sleep 0.5
check "inner：有 inner join（词首规则：空格后的 j 不算词首，但整串匹配开头）" eval 'has "inner join" || { echo "  (inner join 没出现)"; false; }'
fresh; e2e_type "retur"; sleep 0.5
check "PG：有 returning" has returning
fresh; e2e_type "on"; sleep 0.5
check "PG：on 有 on conflict" has "on conflict"
fresh; e2e_type "repl"; sleep 0.5
check "PG 没有 replace into（MySQL 才有）" eval '! has "replace into" 2>/dev/null'
fresh; e2e_type "on dup"; sleep 0.5
check "PG 没有 on duplicate key update" eval '! has "on duplicate key update" 2>/dev/null'
# ---- 审查补的：输入的大小写保留；和输入完全相同的候选排第一；多词关键字不重复前面的词；DO / KEY / FOR 后面的 UPDATE 不是表名位置
fresh; e2e_type " b"; key Escape; key 0; key i; e2e_type "AND"; sleep 0.5; key Enter
check "大写：AND| b ↵：光标移到空格后（4），文字仍是 AND b（没有改成小写）" eval '[[ $(line 1) == "AND b" && $(cc) == 1:4 ]]'
fresh; e2e_type "SELECT"; sleep 0.5; key Enter
check "打完 SELECT ↵：得到 \"SELECT \"（大小写保留，补一个空格）" eval '[[ $(line 1) == "SELECT" && $(cc) == 1:7 ]]'
fresh; e2e_type "select 1 where x is NULL"; sleep 0.5; key Enter
check "NULL ↵：不改成 null，也没有补空格（值），console 换行" eval '[[ $(line 1) == "select 1 where x is NULL" && $(cc) == 2:0 ]]'
fresh; e2e_type "update t_order set"; sleep 0.5; key Enter
check "update t_order set ↵：得到 set + 空格（完全相同的候选排第一，不会换成别的表名）" eval '[[ $(line 1) == "update t_order set" && $(cc) == 1:19 ]]'
fresh; e2e_type "alter table t rename to"; sleep 0.5; key Enter
check "alter table t rename to ↵：得到 rename to + 空格" eval '[[ $(line 1) == "alter table t rename to" && $(cc) == 1:24 ]]'
fresh; e2e_type "insert in"; sleep 0.5
for _ in 1 2 3; do key Down; done; key Enter
check "insert in，↓ 三次选中 insert into ↵：只有一个 insert into（不重复 insert）" eval '[[ $(line 1) == "insert into" && $(cc) == 1:12 ]] || { echo "  [$(line 1)] $(cc) cands: $(names)"; false; }'
fresh; e2e_type "values (1) on conflict (id) do update set"; sleep 0.5; key Enter
check "PG：do update set ↵：补空格（do 后面的 update 不当成表名的位置）" eval '[[ $(line 1) == "values (1) on conflict (id) do update set" && $(cc) == 1:42 ]] || { echo "  $(line 1) $(cc)"; false; }'
fresh; e2e_type "full"; sleep 0.5
check "PG：full 有 full join" has "full join"
fresh; e2e_type "ifn"; sleep 0.5
check "PG：没有 ifnull（MySQL 才有）" eval '! has ifnull 2>/dev/null'
key Escape; key Escape

# ---- WHERE：打完整个 and ↵ → "and "，不执行、没有报错（用户验收时的复现）
e2e_stop; SOLO=1 start; open_table t_order; settle
key /; clear_in; e2e_type "status = 'done' and"; sleep 0.5
check "WHERE 输入 …and：弹出 and" has and
key Enter; sleep 0.8
check "↵：文字变成 \"… and \"，光标在末尾，不执行（还在 WHERE 的 INSERT）、没有报错" eval '[[ $(where_in) == "status = '"'done'"' and" ]] && mode_is INSERT && [[ $(bar) == *"editing WHERE"* ]] && ! screen_has "ERROR"'
check "光标在 and 后一格（前面 20 个字符）" eval '[[ $(( $(e2e_flag cursor_x) - 41 )) == 20 ]]'
e2e_type "id > 0"; sleep 0.3; key Enter; settle
check "接着打 id > 0 再 ↵：整句执行，回到表格" eval 'mode_is NORMAL && settled && [[ $(where_in) == *"and id > 0" ]]'
key /; clear_in; e2e_type "note is nul"; sleep 0.5; key Enter; sleep 0.3
check "WHERE：note is nul ↵ 接受 is null（不补空格）" eval '[[ $(where_in) == "note is null" ]]'
key Enter; settle
check "再 ↵：文字没变，照常执行" eval 'mode_is NORMAL && [[ $(where_in) == "note is null" ]]'
key /; clear_in; e2e_type "note is null"; sleep 0.5
check "WHERE：打完整个 null，列表里是 null" has null
key Enter; settle
check "打完 null ↵：文字不变，照常执行（不补空格）" eval 'mode_is NORMAL && [[ $(where_in) == "note is null" ]]'
key /; clear_in; e2e_type "status = 'done"; sleep 0.5; key Enter; settle
check "WHERE：status = 'done（右引号是自动配对补的，光标后就是它）↵：不算接受，照常执行查询，回到表格" eval 'mode_is NORMAL && [[ $(where_in) == "status = '"'"'done'"'"'" ]]'
key /; clear_in; e2e_type " b"; key Escape; key 0; key i; e2e_type "and"; sleep 0.5
check "WHERE：and| b：光标在 and 后、后面是空格，弹出 and" has and
key Enter; sleep 0.5
check "光标后已有空格：文字不变（and b），光标移到空格后（4），不执行，还在 WHERE 的 INSERT" eval '[[ $(where_in) == "and b" && $(( $(e2e_flag cursor_x) - 41 )) == 4 ]] && mode_is INSERT && [[ $(bar) == *"editing WHERE"* ]]'
key Escape; key Escape

# ---- 快速 SQL：;selec ↵ 也补空格
e2e_keys C-p; sleep 0.3; e2e_type ";selec"; sleep 0.5
check "快速 SQL 输入 ;selec：弹出 select" has select
key Enter; sleep 0.3
check "快速 SQL：↵ 接受 select + 空格（光标在 ;select 后一格），面板还开着" eval 'palette_open && c=$(e2e_find ";select" $(e2e_plain | awk "/;select/ { print NR; exit }") | cut -d" " -f1) && [[ $(e2e_flag cursor_x) == $(( c - 1 + 8 )) ]] || { echo "  cursor_x=$(e2e_flag cursor_x) c=$c"; false; }'
key Escape; key Escape


# ---- MySQL：replace into、on duplicate key update 另有；PG 的 returning 没有（共用库，只读）
[[ $SQLMUX_TEST_MYSQL =~ ^([^:]*):([^@]*)@tcp\(([^:]*):([0-9]+)\)/(.*)$ ]]
D=$(mktemp -d "${TMPDIR:-/tmp}/sqlmux-e2e-cfg.XXXXXX"); trap 'e2e_stop; rm -rf "$D"' EXIT
printf '[[connection]]\nname = "my"\nengine = "mysql"\ndsn = "mysql://%s:%s@%s:%s/%s"\n' "${BASH_REMATCH[1]}" "${BASH_REMATCH[2]}" "${BASH_REMATCH[3]}" "${BASH_REMATCH[4]}" "${BASH_REMATCH[5]}" >"$D/connections.toml"
e2e_stop; start -C "$D"; key C-l; key i
e2e_type "repl"; sleep 0.8
check "MySQL：repl 有 replace into" has "replace into"
fresh; e2e_type "on"; sleep 0.5
check "MySQL：on 有 on duplicate key update，没有 on conflict" eval 'has "on duplicate key update" && ! has "on conflict" 2>/dev/null'
fresh; e2e_type "retur"; sleep 0.5
check "MySQL：retur 没有 returning" eval '! has returning 2>/dev/null'
fresh; e2e_type "ifn"; sleep 0.5
check "MySQL：有 ifnull" has ifnull
fresh; e2e_type "full"; sleep 0.5
check "MySQL：没有 full join（PG 才有）" eval '! has "full join" 2>/dev/null'
fresh; e2e_type "insert into t_log values (1) on duplicate key update st"; sleep 0.8
check "MySQL：on duplicate key update 之后的 st：不是只有表名（UPDATE 前面是 KEY，不是表名的位置）" eval '! [[ $(names) == "t_order|t_sku|t_user|" ]]' 
fresh; e2e_type "des"; sleep 0.5
check "MySQL：des 有 describe" has describe
fresh; e2e_type "repl"; sleep 0.5; key Enter
check "MySQL：接受 replace into 补一个空格" eval '[[ $(line 1) == "replace into" && $(cc) == 1:13 ]]'
key Escape

e2e_done
