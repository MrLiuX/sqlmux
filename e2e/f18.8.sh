#!/usr/bin/env bash
# F18.8 命令面板预览 query 时当前连接是 Redis 就崩溃（specs/m18-windows-mssql/task.md F18.8；tech-design §12「query」）
# Redis 连接上开面板、选中一个 query 项：预览画出文件的前几行（Redis 按 PG 的方言着色），程序还在，不 panic。每个脚本自己起 redis-server。
. "$(dirname "$0")/lib.sh"
e2e_build || exit 1
. "$(dirname "$0")/palette.sh"
. "$(dirname "$0")/redis.sh"

D=$(mktemp -d "${TMPDIR:-/tmp}/sqlmux-e2e-cfg.XXXXXX"); trap 'e2e_stop; redis_stop; rm -rf "$D"' EXIT
redis_start R || exit 1
redis_seed
mkdir -p "$D/c" "$D/data/sqlmux/queries"
(umask 077; printf '[[connection]]\nname = "rd"\nengine = "redis"\ndsn = "redis://localhost:%s/0"\n' "$RPORT" >"$D/c/connections.toml")
printf 'select id, name\nfrom t_user\nwhere id > 10;\n' >"$D/data/sqlmux/queries/report.sql"

e2e_start -C "$D/c" -D "$D/data" "$E2E_BIN"; wait_for 8 flag_is alternate_on 1; sleep 2
pal "@repo"; sleep 0.8
check "Redis 连接上面板 @repo：列出 query report（右端 Query）" eval 'list | grep -q "^report .*Query|"' || list
check "选中 report：预览画出文件的前几行，程序还在（不 panic）" eval 'running && preview | grep -qF "select id, name" && preview | grep -qF "where id > 10;"' || { preview; false; }
key Down; sleep 0.3; key Up; sleep 0.3
check "上下移动选中项之后程序还在，预览还在" eval 'running && preview | grep -qF "from t_user"'
key Escape; sleep 0.3
check "esc 关掉面板，程序还在" eval 'running && closed'
e2e_done
