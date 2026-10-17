#!/usr/bin/env bash
# F17.7 / F17.8 exec 连接的配置检查和连接管理（specs/m17-polish-exec/task.md F17.7 d、F17.8 b；tech-design §8.8「exec 连接只在 connections.toml 里手写，连接管理器不列它」）
# 缺 dialect / command / {sql}、redis 方言缺单独一项 {args...}、别的引擎写了 dialect / command：启动时报错退出，说清哪一个连接和为什么；
# 能用的 exec 连接不出现在连接管理的列表里（表单没有 command / dialect 字段，删、改名在文件里做）。后端不用：SQLite 文件、假命令。
. "$(dirname "$0")/lib.sh"
e2e_build || exit 1
. "$(dirname "$0")/palette.sh"

D=$(mktemp -d "${TMPDIR:-/tmp}/sqlmux-e2e-cfg.XXXXXX"); trap 'e2e_stop; rm -rf "$D"' EXIT
DB=$D/shop.db; sqlite3 "$DB" <"$E2E_ROOT/testdata/seed/sqlite.sql"
mkdir -p "$D/c"
squash() { e2e_plain | tr -d ' \n'; }                      # the screen without its spaces and line ends: a message wrapped at the window's edge reads on
bad() { # TOML WANT...：这份 connections.toml 启动就报错退出，信息里有每一个 WANT（空格不计）
  local toml=$1; shift
  (umask 077; printf '%b' "$toml" >"$D/c/connections.toml")
  e2e_start -C "$D/c" "$E2E_BIN"; wait_for 8 screen_has "[e2e-exit"; sleep 0.3
  local w; for w in "$@"; do squash | grep -qF -- "$w" || { echo "  no '$w' in: $(squash | cut -c1-300)"; _e2e_kill; return 1; }; done
  squash | grep -qF "[e2e-exit1]" || { echo "  did not exit 1"; _e2e_kill; return 1; }
  _e2e_kill
}
screen_has() { e2e_plain | grep -qF -- "$1"; }

check "exec 没写 dialect：报「dialect = \"\"：exec 只认 postgres 和 redis」，退出码 1" eval 'bad "[[connection]]\nname = \"x\"\nengine = \"exec\"\ncommand = [\"echo\", \"{sql}\"]\n" "第1个连接" "exec只认postgres和redis"'
check "exec 没写 command：报「exec 的 command 是 argv 数组，里面要有 {sql}」" eval 'bad "[[connection]]\nname = \"x\"\nengine = \"exec\"\ndialect = \"postgres\"\n" "exec的command是argv数组" "要有{sql}"'
check "exec 的 command 里没有 {sql}：同一句" eval 'bad "[[connection]]\nname = \"x\"\nengine = \"exec\"\ndialect = \"postgres\"\ncommand = [\"echo\", \"hi\"]\n" "exec的command是argv数组" "要有{sql}"'
check "redis 方言的 {args...} 不是单独一项（{args...}x）：报「redis 的 command 要有单独一项 {args...}」" eval 'bad "[[connection]]\nname = \"x\"\nengine = \"exec\"\ndialect = \"redis\"\ncommand = [\"echo\", \"{cmd}\", \"{args...}x\"]\n" "redis的command要有单独一项{args...}"'
check "redis 方言只有 {args...}、没有 {cmd}：报 command 里要有 {cmd}（argv 数组那句）" eval 'bad "[[connection]]\nname = \"x\"\nengine = \"exec\"\ndialect = \"redis\"\ncommand = [\"echo\", \"{args...}\"]\n" "exec的command是argv数组" "要有{"'
check "postgres 连接写了 command：报「dialect 和 command 只对 exec 连接有用」" eval 'bad "[[connection]]\nname = \"x\"\nengine = \"postgres\"\ndsn = \"postgres://u@h/d\"\ncommand = [\"echo\", \"{sql}\"]\n" "dialect和command只对exec连接有用"'
check "postgres 连接写了 dialect：同一句" eval 'bad "[[connection]]\nname = \"x\"\nengine = \"postgres\"\ndsn = \"postgres://u@h/d\"\ndialect = \"postgres\"\n" "dialect和command只对exec连接有用"'

# ---- 连接管理不列 exec 连接
(umask 077; cat >"$D/c/connections.toml" <<TOML
[[connection]]
name = "plain"
engine = "sqlite"
dsn = "$DB"

[[connection]]
name = "exe"
engine = "exec"
dialect = "postgres"
command = ["echo", "{sql}"]
TOML
)
e2e_start -C "$D/c" "$E2E_BIN plain"; wait_for 8 flag_is alternate_on 1; sleep 2
pal ">conn.manage"; key Enter; sleep 1
check "连接管理打开了，列表里有 plain、没有 exe（exec 连接只在文件里管）" eval 'e2e_plain | grep -q "┌─ 连接" && e2e_plain | noicon | grep -qE "│ +plain +│" && ! e2e_plain | noicon | sed -n "/┌─ 连接/,\$p" | grep -qE "│ +exe +│" || { e2e_plain | sed -n 8,24p | cut -c25-120; false; }'
key Escape; sleep 0.4
e2e_done
