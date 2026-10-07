#!/usr/bin/env bash
# F8.7 yi 在 MySQL 上（specs/m8-feedback/task.md F8.7；tech-design §10.6「复制为 INSERT」）：标识符用反引号、值照 MySQL 转义（\ 写成 \\），
# 复制出来的语句执行回去得到同一行；行里有显示成 <binary N bytes> 的格时 yi 和 yy → p 都拒绝（toast「有二进制列，复制不了」）。自建库。
. "$(dirname "$0")/lib.sh"
SOLO=1
e2e_build || exit 1

D=$(mktemp -d "${TMPDIR:-/tmp}/sqlmux-e2e-cfg.XXXXXX"); trap 'e2e_stop; rm -rf "$D"' EXIT
e2e_own_mydb || { echo "e2e: could not create the private MySQL database" >&2; exit 1; }
OWN=sqlmux_e2e_$$
my() { e2e_mysql "$OWN" <<<"$1"; }
my "create table t_b (id int primary key, b varbinary(8), v text, d double); insert into t_b values (1, x'ff00fe', 'a', 1.5), (2, 'abc', concat('it', char(39), 's \\\\ back\"q'), null);"
mkdir -p "$D/c"; (umask 077; printf '[[connection]]\nname = "my"\nengine = "mysql"\ndsn = "%s"\n' "$E2E_MYDB" >"$D/c/connections.toml")
toast_text() { e2e_text 1 160 44; }
rows() { my "select count(*) from t_b"; }
row_text() { my "select id, hex(b), v, d from t_b where id = $1"; }

start -C "$D/c"; open_table t_b; wait_for 8 settled
check "（准备）第 1 行的 b 列显示成 <binary 3 bytes>" eval 'e2e_plain | grep -q "<binary 3 bytes>"'
: >|"$E2E_TMP/clipboard"; key y i; sleep 0.5
check "有 <binary N bytes> 的行：yi 不写剪贴板，toast「有二进制列，复制不了」" eval 'toast_text | grep -q "有二进制列，复制不了" && [[ -z "$(clip)" ]]'
key y y; sleep 0.3; key p; sleep 0.6
check "yy 之后 p 同样拒绝：toast「有二进制列，复制不了」，没有多出新行" eval 'toast_text | grep -q "有二进制列，复制不了" && ! e2e_plain | grep -qE "│ +\\+ │"'

# ---- 第 2 行：能复制；反引号、MySQL 的转义，执行回去得到同一行
key j; : >|"$E2E_TMP/clipboard"; key y i; sleep 0.5
check "yi：insert into 库名.t_b (id, b, v, d) values (2, 'abc', …, null);（标识符是反引号，MySQL 的转义）" eval '[[ "$(clip)" == "insert into \`$OWN\`.\`t_b\` (\`id\`, \`b\`, \`v\`, \`d\`) values (2, '"'abc'"', "* && "$(clip)" == *", null);" ]] || { echo "  clip: $(clip)"; false; }'
want=$(row_text 2); my "delete from t_b where id = 2" >/dev/null; my "$(clip)" >/dev/null
check "把复制出来的语句在 MySQL 上执行回去：id = 2 的行和原来一样（单引号、反斜杠、双引号都没坏）" eval '[[ "$(row_text 2)" == "$want" && -n "$want" ]] || { echo "  want: $want"; echo "  got:  $(row_text 2)"; false; }'
e2e_done
