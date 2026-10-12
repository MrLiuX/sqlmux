#!/usr/bin/env bash
# F14.6 MySQL 取消的兜底（specs/m14-sqlite/task.md F14.6；tech-design §8.3）
# 一条 select sleep(60) 在跑，KILL QUERY 的临时连接连不通（e2e/killhole.py：有 block 文件时新连接只被接住、不回答）：C-c 之后 5 秒左右驱动的 ctx
# 取消、连接关掉，Worker 空出来，下一条语句重拨之后照常跑。以前 Worker 一直锁到那条语句自己跑完（60 秒）。自建库。
. "$(dirname "$0")/lib.sh"
e2e_build || exit 1
. "$(dirname "$0")/palette.sh"
. "$(dirname "$0")/paging.sh"

D=$(mktemp -d "${TMPDIR:-/tmp}/sqlmux-e2e-cfg.XXXXXX"); HOLEPID=
trap 'e2e_stop; [[ -n $HOLEPID ]] && kill $HOLEPID 2>/dev/null; rm -rf "$D"' EXIT
e2e_own_mydb || { echo "e2e: could not create the private MySQL database" >&2; exit 1; }
read -r UH UP <<<"$(python3 -c 'import sys, urllib.parse as u; p = u.urlsplit(sys.argv[1]); print(p.hostname, p.port)' "$E2E_MYDB")"
PORT=$(python3 -c 'import socket; s = socket.socket(); s.bind(("127.0.0.1", 0)); print(s.getsockname()[1])')
python3 "$E2E_ROOT/e2e/killhole.py" "$PORT" "$UH" "$UP" "$D/block" & HOLEPID=$!; sleep 0.5
DSN=$(python3 -c 'import sys, urllib.parse as u; p = u.urlsplit(sys.argv[1]); print(p._replace(netloc=f"{p.username}:{p.password}@127.0.0.1:{sys.argv[2]}").geturl())' "$E2E_MYDB" "$PORT")
mkdir -p "$D/own"; printf '[[connection]]\nname = "doraemon"\nengine = "mysql"\ndsn = "%s"\n' "$DSN" >"$D/own/connections.toml"; chmod 600 "$D/own/connections.toml"

seven_row() { e2e_plain | grep -qE "│ +7 +│"; }
start -C "$D/own"; key C-l
setsql "select sleep(60)"; key Enter; sleep 2
check "（准备）select sleep(60) 在跑（状态栏 busy）" eval '[[ $(bar) == *busy* ]]'
touch "$D/block"
t0=$(date +%s); key C-c
wait_for 20 eval '[[ $(bar) != *busy* ]]'; took=$(( $(date +%s) - t0 ))
check "KILL 发不出去：C-c 之后 5–10 秒 Worker 空出来（$took 秒，不是等 60 秒）" eval '((took >= 4 && took <= 10))'
rm -f "$D/block"
setsql "select 7 as seven"; key Enter; wait_for 15 eval '[[ $(bar) != *busy* ]]'; sleep 1
check "连接已重拨：接着跑 select 7，结果出来了（第一次可能先报断线，再按一次）" eval 'seven_row || { key Enter; sleep 3; seven_row; }' || e2e_plain | tail -6
e2e_done
