#!/usr/bin/env bash
# F15.10 值补全（specs/m15-pg-databases/task.md F15.10；tech-design §11「签名提示与值补全」）
# 光标在一个列的值的位置（INSERT 的一行、UPDATE 的 SET、条件的右边）：弹这一列的候选——枚举的值（带引号）、布尔的 true / false、
# 时间的 now() / current_timestamp、可空的 null、有默认值的 default（只在 INSERT / UPDATE 里；条件里 null 和 default 都没有，docs e6600d4）；其余不弹。只在值开始的地方弹（( , = <> != 之后），
# 不在值打完之后；字符串里按整串匹配；候选先不选中，Tab 选第一项、↵ 接受。单测覆盖每一类，这里测真实终端里的弹出、接受和不弹的位置（PG 和 MySQL）。
. "$(dirname "$0")/lib.sh"
e2e_build || exit 1

pop() { e2e_panes | awk '$1 == "-" { print $2, $3, $4, $5 }' | tail -1; }
items() { local g y; g=($(pop)); [[ -n ${g[0]} ]] || return 0
  for ((y = g[1] + 1; y < g[1] + g[3] - 1; y++)); do e2e_text $((g[0] + 4)) $((g[0] + g[2] - 2)) $y | sed 's/^ *//; s/ *$//'; done; }
names() { items | sed 's/  .*//' | tr '\n' '|'; }
names_are() { [[ $(names) == "$1" ]] || { echo "  candidates: '$(names)', want '$1'"; false; }; }
none() { [[ -z $(pop) ]] || { echo "  popup: $(names)"; false; }; }
line() { e2e_text 104 159 $(($1 + 1)) | sed 's/ *$//'; }
# 选中的是哪一项（#364a82 底）：第几项（从 1 起），没有选中 0
sel() { local g y n=0; g=($(pop)); [[ -n ${g[0]} ]] || { echo 0; return; }
  for ((y = g[1] + 1; y < g[1] + g[3] - 1; y++)); do n=$((n + 1)); [[ $(e2e_style $((g[0] + 2)) "$y") == *bg=#364a82* ]] && { echo $n; return; }; done; echo 0; }
fresh() { key Escape; key Escape; key g g; key d G; key i; }
at() { fresh; e2e_type "$1"; sleep 1.2; }

D=$(mktemp -d "${TMPDIR:-/tmp}/sqlmux-e2e-cfg.XXXXXX"); trap 'e2e_stop; rm -rf "$D"' EXIT
e2e_own_db || exit 1
printf '[[connection]]\nname = "doraemon"\nengine = "postgres"\ndsn = "%s"\n' "$E2E_DB" >"$D/connections.toml"

start -C "$D"; key C-l; key i

# ---- 枚举：t_order.status order_status not null default 'pending'（标签按 enumsortorder）
at "insert into t_order(status) values ("
check "枚举列的 VALUES 里弹出枚举的标签（按 enumsortorder：queued 在 running 前），后面是 default（列有默认值），没有 null（not null）" names_are "pending|queued|running|done|failed|default|"
check "候选先不选中（↵ 仍是换行，不会误接受）" eval '[[ $(sel) == 0 ]]'
key Tab; sleep 0.3
check "Tab：选中第一项" eval '[[ $(sel) == 1 ]]'
key Enter; sleep 0.4
check "↵ 接受：枚举值带引号写进括号，列表关了" eval '[[ $(line 1) == "insert into t_order(status) values ('"'"'pending'"'"')" ]] && none' || line 1
at "insert into t_order(status) values ('ru"
check "字符串里按整串匹配：只剩 running，已选中" eval 'names_are "running|" && [[ $(sel) == 1 ]]'
key Enter; sleep 0.4; e2e_type X; sleep 0.4
check "↵：接受的 running 盖过原来的收尾引号（'running' 只有一对引号），光标在引号之后" eval '[[ $(line 1) == "insert into t_order(status) values ('"'"'running'"'"'X)" ]]' || line 1
at "insert into t_order(status) values ('"
check "空字符串里：只列枚举的标签，不列 default / null" names_are "pending|queued|running|done|failed|"
at "insert into t_order(paid, status) values (true, "
check "第二个值：枚举（paid 是布尔，这里是 status）" names_are "pending|queued|running|done|failed|default|"

# ---- 布尔、可空、时间、默认值
at "insert into t_order(id, paid) values (1, "
check "布尔且可空：true / false / null" names_are "true|false|null|"
at "insert into t_order(id, paid) values (1, t"
check "布尔打了一个字母 t：true 排第一并选中（值在前、再是别处的词）" eval '[[ $(names) == true\|* && $(sel) == 1 ]] || { echo "  $(names) sel $(sel)"; false; }'
at "insert into t_user(name, email) values ('x', "
check "可空的文本列（email）：只有 null" names_are "null|"
at "insert into t_user(name, email) values ("
check "not null 又没有默认值的文本列（name）：不弹" none
at "insert into t_user(name, email, created_at) values ('x', null, "
check "时间列（not null、默认 now()）：now() / current_timestamp / default，没有 null" names_are "now()|current_timestamp|default|"
key Tab; key Enter; sleep 0.4
check "接受 now()：直接写进括号（不加引号）" eval '[[ $(line 1) == *"values ('"'"'x'"'"', null, now())" ]]' || line 1
at "insert into t_order(id) values ("
check "有默认值的 id（bigserial）：只有 default" names_are "default|"
at "insert into t_order(note) values ("
check "可空文本 note：null" names_are "null|"

# ---- UPDATE 的 SET 和条件
at "update t_order set status = "
check "UPDATE 的 SET：枚举的标签 + default" names_are "pending|queued|running|done|failed|default|"
at "update t_order set created_at = "
check "UPDATE 的 SET 时间列：now() / current_timestamp / default" names_are "now()|current_timestamp|default|"
at "update t_order set id = "
check "UPDATE 的 SET 有默认值的列：default" names_are "default|"
at "update t_order set status = 'done', paid = "
check "SET 的第二个赋值：布尔 true / false / null（没有 default：paid 没有默认值）" names_are "true|false|null|"
at "select * from t_order where status = "
check "条件的右边：枚举的标签，没有 default" names_are "pending|queued|running|done|failed|"
at "select * from t_order o where o.status = "
check "带别名的列：同样的枚举标签" names_are "pending|queued|running|done|failed|"
at "select * from t_order where status in ('done', "
check "IN (…) 的第二个：枚举标签" names_are "pending|queued|running|done|failed|"
at "select * from t_order where created_at = "
check "条件里的时间列：now() / current_timestamp，没有 null（not null）、没有 default" names_are "now()|current_timestamp|"
at "select * from t_order where status = 'done' and paid = "
check "and 之后的条件：paid 的 true / false，没有 null（条件里不给 null 和 default，docs e6600d4）" names_are "true|false|"
at "select * from t_user where email = "
check "条件里的可空文本列（email）：不弹（条件里没有 null）" none
at "select * from t_order where id = "
check "条件里的 not null 数字列：不弹（default 不在条件里）" none
at "select * from t_order where id = 1 and "
check "条件里的列名不是值的位置：弹的是列，不是值" eval '! names | grep -q "^default\|^null" && names | grep -q "id|"' || names

# ---- 不弹的位置
at "insert into t_order(id, status) values (1 "
check "一个值打完（空格之后）：不弹" none
at "insert into t_order(id, status) values (1, 5"
check "数字打了一半：不弹" none
at "update t_order set status = 'done' "
check "字符串打完之后：不弹" none
at "insert into t_order(status) values (lower("
check "函数调用的括号里：不弹" none
at "insert into t_order(status) values ((" 
check "行里的嵌套括号：不弹" none
at "insert into t_order(id) values (1, "
check "值比列多：不弹" none
at "insert into t_order(status) select "
check "INSERT … SELECT：弹的是列，不是值" eval '! names | grep -q "pending"'
at "insert into nosuch(a) values ("
check "catalog 里没有的表：不弹" none
at "insert into t_order(status) values (1), ("
check "多行 VALUES 的第二个括号：重新数，枚举标签" names_are "pending|queued|running|done|failed|default|"

# ---- C-n：值的候选和别处的词一起列
at "insert into t_order(id, status) values (1, "
key Escape; key a; key C-n; sleep 0.8
check "C-n：先是这一列的枚举标签，后面是别处的词（列名 id 在里面）" eval '[[ $(names) == pending\|queued\|running\|done\|failed\|* ]] && names | grep -q "|id|"' || names
key Escape; key Escape
e2e_stop

# ---- MySQL：enum('…')、tinyint(1)、datetime；"…" 在 MySQL 里是字符串，不弹
e2e_own_mydb || exit 1
printf '[[connection]]\nname = "my"\nengine = "mysql"\ndsn = "%s"\n' "$E2E_MYDB" >"$D/connections.toml"
start -C "$D"; key C-l; key i
at "insert into t_order(status) values ("
check "MySQL：enum 的标签（建表的顺序）+ default" names_are "pending|queued|running|done|failed|default|"
key Tab; key Enter; sleep 0.4
check "MySQL：接受 pending：'pending'（单引号）" eval '[[ $(line 1) == "insert into t_order(status) values ('"'"'pending'"'"')" ]]' || line 1
at "insert into t_order(status) values ('ru"
check "MySQL：单引号的字符串里按整串匹配：running" names_are "running|"
at 'insert into t_order(status) values ("'
check 'MySQL："…" 在 MySQL 里也是字符串，候选是单引号的字面量、塞进去会多一个引号：双引号里不弹' none
at "insert into t_user(name, active) values ('x', "
check "MySQL：boolean 是 tinyint(1)：true / false / default" names_are "true|false|default|"
at "insert into t_user(name, email, created_at) values ('x', null, "
check "MySQL：datetime not null default current_timestamp：now() / current_timestamp / default" names_are "now()|current_timestamp|default|"
at "update t_order set paid = "
check "MySQL：UPDATE 的 SET 可空 boolean：true / false / null" names_are "true|false|null|"
at "select * from t_order where status = "
check "MySQL：条件的右边：enum 的标签，没有 default" names_are "pending|queued|running|done|failed|"
at "insert into t_order(id) values ("
check "MySQL：auto_increment 的 id：default" names_are "default|"

e2e_done
