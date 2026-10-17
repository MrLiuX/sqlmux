#!/usr/bin/env bash
# F15.3 / F15.10 / F15.11 的补充（specs/m15-pg-databases/task.md F15.3、F15.10、F15.11；复审点名的位置）：
# 签名提示只在聚焦的 query 里画；VALUES 里 array[1, 2]、块注释、行注释里的逗号不算；内置函数（lower( 之类）退回外层，显示 INSERT 的列；
# 命令面板 SQL 范围（quick SQL）的输入框里接受骨架后光标在括号里；PG 的非默认库（F15.1 的子连接）里，提示、值候选和骨架用的是那个库的 catalog。
. "$(dirname "$0")/lib.sh"
e2e_build || exit 1
. "$(dirname "$0")/palette.sh"

srow() { (( $1 == 1 )) && echo 3 || echo "$1"; }
sig() { e2e_cap -e -N | python3 -I -c '
import sys; sys.path.insert(0, sys.argv[1]); import cells
g = cells.grid(sys.stdin.read()); r = g[int(sys.argv[2]) - 1]
dim = lambda c: c[1] == "#565f89" or (c[1] == "#9ece6a" and "bold" in c[3])
n = 0
while 103 + n < len(r) and dim(r[103 + n]): n += 1
cs = r[103:103 + n]
print("".join(c[0] for c in cs).strip() + "|" + "".join(c[0] for c in cs if c[1] == "#9ece6a").strip())' "$E2E_ROOT/e2e" "$(srow "$1")"; }
sig_is() { local got; got=$(sig "$1"); [[ $got == "$2" ]] || { echo "  提示行（光标第 $1 行）'$got'，要 '$2'"; false; }; }
pop() { e2e_panes | awk '$1 == "-" { print $2, $3, $4, $5 }' | tail -1; }
items() { local g y; g=($(pop)); [[ -n ${g[0]} ]] || return 0
  for ((y = g[1] + 1; y < g[1] + g[3] - 1; y++)); do e2e_text $((g[0] + 4)) $((g[0] + g[2] - 2)) $y | sed 's/^ *//; s/ *$//'; done; }
names() { items | sed 's/  .*//' | tr '\n' '|'; }
names_are() { [[ $(names) == "$1" ]] || { echo "  candidates: '$(names)', want '$1'"; false; }; }
fresh() { key Escape; key Escape; key g g; key d G; key i; key Enter; }
at() { fresh; e2e_type "$1"; sleep 1.2; }
palline() { e2e_plain | grep -m1 ' ;insert' | sed 's/.* ;insert/;insert/; s/ *│.*//'; }   # the quick SQL's input

D=$(mktemp -d "${TMPDIR:-/tmp}/sqlmux-e2e-cfg.XXXXXX"); trap 'e2e_stop; rm -rf "$D"' EXIT
e2e_own_db || exit 1
B=sqlmux_e2e_$$_b
psql "$E2E_DB" -q -v ON_ERROR_STOP=1 -c "create table t_k (a int, b text)" || exit 1
psql "$SQLMUX_TEST_PG" -q -v ON_ERROR_STOP=1 -c "create database $B" || exit 1
trap 'psql "$SQLMUX_TEST_PG" -qAt -c "drop database if exists $B with (force)" >/dev/null 2>&1; e2e_stop; rm -rf "$D"' EXIT
psql "${E2E_DB/sqlmux_e2e_$$/$B}" -q -v ON_ERROR_STOP=1 -c "create type mood as enum ('sad', 'ok', 'happy')" -c "create table bt (x int, y text not null, m mood, n int)" || exit 1
printf '[[connection]]\nname = "doraemon"\nengine = "postgres"\ndsn = "%s"\n' "$E2E_DB" >"$D/connections.toml"

start -C "$D"; key C-l; key i

# ---- 只在聚焦的 query 里画
at "insert into t_user(name, email) values ("
check "聚焦的 query：有提示行" sig_is 2 "name text, email text|name text"
key Escape; key l; sleep 0.6
check "esc l：光标在 ) 上，还是亮 name" sig_is 2 "name text, email text|name text"
key C-h; sleep 0.6
check "焦点移到别的 pane：这个 query 里的提示行没有了" sig_is 2 "|"
key C-l; sleep 0.6
check "焦点回来：提示行又在（光标没动，亮 name）" sig_is 2 "name text, email text|name text"
key i

# ---- 逗号不算的地方
at "insert into t_user(name, email) values (array[1, 2], "
check "array[1, 2] 里的逗号不算：第二个值亮 email" sig_is 2 "name text, email text|email text"
at "insert into t_user(name, email) values (/* a, b */ 'x', "
check "块注释里的逗号不算：第二个值亮 email" sig_is 2 "name text, email text|email text"
at "insert into t_user(name, email) values ('x' -- a, b"
key Enter; e2e_type ", "; sleep 1
check "行注释里的逗号不算：换行后的第二个值亮 email" sig_is 3 "name text, email text|email text"

# ---- 内置函数的括号：退回外层
at "insert into t_user(name, email) values (lower("
check "values (lower( ：内置函数没有签名，显示 INSERT 的列，亮第一个 name" sig_is 2 "name text, email text|name text"
at "insert into t_user(name, email) values ('a', lower("
check "('a', lower( ：亮第二个 email" sig_is 2 "name text, email text|email text"

# ---- quick SQL（命令面板 ;）的输入框：骨架接受后光标在括号里
key Escape; key Escape
pal '\;'; sleep 0.5; e2e_type "insert into t_k "; sleep 1.2
check "quick SQL 输入框里 insert into t_k 之后：骨架 (a, b) values () 弹出" eval 'e2e_plain | grep -qF "(a, b) values ()  全部 2 列"'
key Tab; key Enter; sleep 0.5
check "接受：输入框里是 insert into t_k (a, b) values ()" eval '[[ $(palline) == ";insert into t_k (a, b) values ()" ]]' || palline
e2e_type "1, 'z'"; sleep 0.5
check "光标在括号里（输入框没有 tabstop，落在第一处）：直接打值，值进了括号" eval '[[ $(palline) == ";insert into t_k (a, b) values (1, '"'"'z'"'"')" ]]' || palline
key Escape; key Escape

# ---- 非默认库：${B} 里的 query 用那个库的 catalog
key C-l; key i; key Escape; key g s; sleep 0.8; e2e_type "$B"; sleep 0.4; key Enter; sleep 3
at "insert into bt(x, y) values ("
check "非默认库：提示行是 ${B}.bt 的列和类型，亮 x" sig_is 2 "x integer, y text|x integer"
at "insert into bt(x, y) values (1, "
check "第二个值亮 y text" sig_is 2 "x integer, y text|y text"
at "insert into bt(m) values ("
check "枚举 mood（只在 ${B} 里有）：sad ok happy，m 可空所以还有 null" names_are "sad|ok|happy|null|"
at "insert into bt "
check "骨架用 ${B}.bt 的列：(x, y, m, n) values ()" names_are "(x, y, m, n) values ()|"
at "update bt set m = "
check "UPDATE 的 SET 也是 ${B} 的：枚举标签" eval 'names | grep -q "^sad|ok|happy|"'

e2e_done
