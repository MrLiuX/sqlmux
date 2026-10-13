#!/usr/bin/env bash
# F15.3 签名提示（specs/m15-pg-databases/task.md F15.3；tech-design §11「签名提示与值补全」）
# 光标所在的值是哪个字段的：INSERT 的 VALUES（有列清单按清单、没有按表的列序、多行 VALUES 每个括号重数、字符串里的逗号不算、
# INSERT … SELECT 不提示）、UPDATE 的 SET、条件的右边、用户函数的调用括号；提示行画在光标行的上一行（第一行时在下一行），当前字段
# 亮（focus 色、粗体）、其余 dim，放不下两头 …；光标移动跟着换、出了上下文就消失；[query] hints = false 关掉。
# 上下文识别的细节（引号、注释、嵌套）由 sqlkit 的单测覆盖；这里测真实终端里画出来的行、颜色、位置和跟着光标换。
. "$(dirname "$0")/lib.sh"
e2e_build || exit 1

D=$(mktemp -d "${TMPDIR:-/tmp}/sqlmux-e2e-cfg.XXXXXX"); trap 'e2e_stop; rm -rf "$D"' EXIT
e2e_own_db || exit 1
psql "$E2E_DB" -q -v ON_ERROR_STOP=1 -c "create function add_tax(price numeric(10,2), rate real default 0.2) returns numeric language sql as 'select 1'" || exit 1
printf '[[connection]]\nname = "doraemon"\nengine = "postgres"\ndsn = "%s"\n' "$E2E_DB" >"$D/connections.toml"
mkdir -p "$D/off"; cp "$D/connections.toml" "$D/off/"; printf '[query]\nhints = false\n' >"$D/off/config.toml"

# 提示行的屏幕行：光标在第 N 行（从 1 起）时，编辑区第 N 行在屏幕第 N+1 行，提示在它上面（屏幕第 N 行）；第一行时在下面（第 3 行）。
srow() { (( $1 == 1 )) && echo 3 || echo "$1"; }
# sig N：提示行的字段和亮着的那个，"全部|亮的"；提示行是 #292e42 底的格子，亮的是 focus 色（#9ece6a）粗体；没有提示行时是 "|"
# 提示行是从编辑区左边那一格起、dim 色或亮色（focus 粗体）的一段；它后面露出的是底下那行 SQL 自己的颜色
sig() { e2e_cap -e -N | python3 -I -c '
import sys; sys.path.insert(0, sys.argv[1]); import cells
g = cells.grid(sys.stdin.read()); r = g[int(sys.argv[2]) - 1]
dim = lambda c: c[1] == "#565f89" or (c[1] == "#9ece6a" and "bold" in c[3])
n = 0
while 103 + n < len(r) and dim(r[103 + n]): n += 1
cs = r[103:103 + n]
print("".join(c[0] for c in cs).strip() + "|" + "".join(c[0] for c in cs if c[1] == "#9ece6a").strip())' "$E2E_ROOT/e2e" "$(srow "$1")"; }
# dims N：提示行里 dim 色的字
dims() { e2e_cap -e -N | python3 -I -c '
import sys; sys.path.insert(0, sys.argv[1]); import cells
g = cells.grid(sys.stdin.read()); r = g[int(sys.argv[2]) - 1]
print("".join(c[0] for c in r[103:159] if c[1] == "#565f89").strip())' "$E2E_ROOT/e2e" "$(srow "$1")"; }
sig_is() { local got; got=$(sig "$1"); [[ $got == "$2" ]] || { echo "  提示行（光标第 $1 行）'$got'，要 '$2'"; false; }; }
# 在第 2 行打（第 1 行空着）：提示在第 1 行的位置，补全浮窗在光标下面，两个不重叠
fresh() { key Escape; key Escape; key g g; key d G; key i; key Enter; }
at() { fresh; e2e_type "$1"; sleep 1.2; }

start -C "$D"; key C-l; key i

at "insert into t_user(name, email) values ("
check "INSERT 有列清单：第一个值，提示行是清单里的列和类型，亮的是 name" sig_is 2 "name text, email text|name text"
at "insert into t_user(name, email) values ('a,b', "
check "字符串里的逗号不算：第二个值亮 email" sig_is 2 "name text, email text|email text"
check "没亮的字段（name text 和逗号）是 dim 色 #565f89" eval '[[ $(dims 2) == "name text," ]] || { echo "  dim: $(dims 2)"; false; }'
at "insert into t_user values (1, 'x', "
check "没有列清单：按表的列序，第三个值亮 email" sig_is 2 "id bigint, name text, email text, active boolean, …|email text"
at "insert into t_user(id, name) values (1, 'x', 2, "
check "值比列多：没有可名的字段，提示消失" sig_is 2 "|"
at "insert into t_user select 1, "
check "INSERT … SELECT：不提示" sig_is 2 "|"
at "select * from t_user where 1 = "
check "不是列的比较（1 = ）：不提示" sig_is 2 "|"

# 多行 VALUES：每个括号重新数；提示在光标行的上一行
fresh; e2e_type "insert into t_user(name, email) values ('a', 'b'),"; key Enter; e2e_type "('c', "; sleep 1.2
check "第三行的第二个括号：重新数，第二个值亮 email，提示画在第二行的位置" sig_is 3 "name text, email text|email text"
key Escape; key k; sleep 0.8
check "光标 k 回到第二行（在 insert 开头，出了括号）：提示消失" sig_is 2 "|"
key j; key '$'; sleep 0.8
check "光标 j 回到括号那行（$ 在 ) 上）：提示又回来，亮的是第二个值" sig_is 3 "name text, email text|email text"

# 光标在编辑器的第一行：提示画在下一行
fresh; key BSpace; e2e_type "insert into t_user(name, email) values ("; sleep 1
check "光标在第一行：提示在第一行的下面（屏幕第 3 行），不是上面" eval '[[ $(sig 1) == "name text, email text|name text" ]]'

# 光标移动跟着换
fresh; e2e_type "insert into t_user(name, email) values ('a', 'b')"; key Escape; sleep 0.6
check "Esc 之后光标在 ) 上：亮 email" sig_is 2 "name text, email text|email text"
key F ,; sleep 0.6
check "F, 光标在逗号上：还在第一个值里，亮 name" sig_is 2 "name text, email text|name text"
key h; key h; sleep 0.6
check "光标回到第一个值里（'a' 的 a 上）：亮 name" sig_is 2 "name text, email text|name text"
key l l l l; sleep 0.6
check "光标移过逗号进到第二个值：亮 email" sig_is 2 "name text, email text|email text"
key '0'; sleep 0.6
check "光标到行首（在 insert 上）：提示消失" sig_is 2 "|"
fresh; e2e_type "insert into t_user(name, email) values ('a', 'b'); select 1"; sleep 0.8
check "分号之后的另一条语句：提示消失" sig_is 2 "|"

# UPDATE 的 SET 和条件的右边：一个字段
at "update t_user set name = 'x', active = "
check "UPDATE 的 SET：亮的是正在填的列 active boolean，只有这一个字段" sig_is 2 "active boolean|active boolean"
at "select * from t_user where email = 'e' and name in ('a', "
check "条件 IN (…) 里：name text" sig_is 2 "name text|name text"
at "select * from t_user u where u.active = "
check "条件带别名的列：active boolean" sig_is 2 "active boolean|active boolean"
at "select * from t_user where active "
check "还没打 = ：不提示" sig_is 2 "|"

# 用户函数的调用括号：参数表来自 Functions 的同一份 catalog
at "select add_tax("
check "用户函数 add_tax(：参数表 price numeric, rate real，亮第一个" eval '[[ $(sig 2) == "price numeric, rate real|price numeric" ]] || { echo "  $(sig 2)"; false; }'
at "select add_tax(1, "
check "第二个参数亮 rate real" eval '[[ $(sig 2) == *"|rate real" ]] || { echo "  $(sig 2)"; false; }'
at "select lower("
check "内置函数没有签名：不提示" sig_is 2 "|"
at "select nosuch("
check "catalog 里没有的函数：不提示" sig_is 2 "|"

# 放不下：只显示当前字段前后几个，两头 …
at "insert into t_order values (1, 2, 'pending', 4, true, "
check "t_order 十列，提示行放不下：两头都有 …，亮的是第六个值 meta" eval '[[ $(sig 2) == "…, "*", …|meta jsonb" ]] || { echo "  $(sig 2)"; false; }'
at "insert into t_order values ("
check "第一个值：只有后面有 …，亮 id" eval '[[ $(sig 2) == "id bigint, "*", …|id bigint" && $(sig 2) != "…,"* ]] || { echo "  $(sig 2)"; false; }'
key Escape; key Escape

# 不用唤起也不用关：Esc 关了补全浮窗，提示还在；换了上下文 / 光标出了语句就没有
e2e_stop
# ---- hints = false 关掉
start -C "$D/off"; key C-l; key i
at "insert into t_user(name, email) values ('a', "
check "[query] hints = false：同样的位置没有提示行" sig_is 2 "|"
at "update t_user set name = "
check "hints = false：UPDATE 的 SET 也没有" sig_is 2 "|"
e2e_stop

# ---- MySQL：列的类型是 MySQL 的写法
e2e_own_mydb || exit 1
printf '[[connection]]\nname = "my"\nengine = "mysql"\ndsn = "%s"\n' "$E2E_MYDB" >"$D/connections.toml"
start -C "$D"; key C-l; key i
at "insert into t_user(name, email) values ("
check "MySQL：INSERT 的列清单，亮第一列 name" eval '[[ $(sig 2) == "name varchar(64), email varchar(128)|name varchar(64)" ]] || { echo "  $(sig 2)"; false; }'
at "insert into t_user(name, email) values ('a', "
check "MySQL：第二个值亮 email" eval '[[ $(sig 2) == *"|email varchar(128)" ]] || { echo "  $(sig 2)"; false; }'
at "update t_user set active = "
check "MySQL：UPDATE 的 SET" eval '[[ $(sig 2) == "active tinyint(1)|active tinyint(1)" || $(sig 2) == "active boolean|active boolean" ]] || { echo "  $(sig 2)"; false; }'
at "select * from t_order where status = "
check "MySQL：条件的右边，enum 列显示完整类型" eval '[[ $(sig 2) == "status enum("*"|status enum("* ]] || { echo "  $(sig 2)"; false; }'

e2e_done
