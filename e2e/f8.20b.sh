#!/usr/bin/env bash
# F8.20 SQLMUX_PPROF（specs/m8-feedback/task.md F8.20；tech-design §15「性能」）
# 设成 localhost:<端口> 时 /debug/pprof/ 能取到页面；端口被占或地址不对：进界面之前报 sqlmux: SQLMUX_PPROF: … 退出 1；
# 不设时不监听任何端口。只读，用共用库。
. "$(dirname "$0")/lib.sh"
e2e_build || exit 1

D=$(mktemp -d "${TMPDIR:-/tmp}/sqlmux-e2e-cfg.XXXXXX"); trap 'e2e_stop; [[ -n $HOLD ]] && kill "$HOLD" 2>/dev/null; rm -rf "$D"' EXIT
free_port() { python3 -c 'import socket; s = socket.socket(); s.bind(("127.0.0.1", 0)); print(s.getsockname()[1])'; }
spid() { ps -axo pid=,command= | awk -v b="$E2E_BIN" '$2 == b { print $1; exit }'; }
HOLD=

P=$(free_port)
e2e_start -x 160 "SQLMUX_PPROF=localhost:$P $E2E_BIN"; wait_for 6 flag_is alternate_on 1; sleep 1
check "SQLMUX_PPROF=localhost:${P}：进了界面（没有退出）" running
check "curl http://localhost:$P/debug/pprof/ 取到 pprof 的索引页" eval 'curl -s --max-time 5 "http://localhost:$P/debug/pprof/" | grep -q "profile"'
check "取 heap profile（/debug/pprof/heap?debug=1）有内容" eval 'curl -s --max-time 5 "http://localhost:$P/debug/pprof/heap?debug=1" | grep -q "heap profile"'
check "取 1 秒 CPU profile（/debug/pprof/profile?seconds=1）是二进制数据，不是空的" eval '[[ $(curl -s --max-time 10 "http://localhost:$P/debug/pprof/profile?seconds=1" | wc -c | tr -d " ") -gt 100 ]]'

# ---- 端口被占
P2=$(free_port); python3 -c 'import socket, sys, time; s = socket.socket(); s.bind(("127.0.0.1", int(sys.argv[1]))); s.listen(1); time.sleep(120)' "$P2" & HOLD=$!; sleep 0.5
e2e_start -x 160 "SQLMUX_PPROF=localhost:$P2 $E2E_BIN"; sleep 2
check "端口 ${P2} 被占：进界面之前报「sqlmux: SQLMUX_PPROF: …」，退出码 1" eval 'exited && e2e_plain | grep -q "sqlmux: SQLMUX_PPROF:" && e2e_plain | grep -q "\[e2e-exit 1\]" || { e2e_plain | head -4; false; }'
check "没有进过界面（屏幕上没有侧栏）" eval '! e2e_plain | grep -q "SPC b"'

# ---- 地址不对
e2e_start -x 160 "SQLMUX_PPROF=not-an-address $E2E_BIN"; sleep 2
check "地址不对（not-an-address）：同样报「sqlmux: SQLMUX_PPROF: …」，退出码 1" eval 'exited && e2e_plain | grep -q "sqlmux: SQLMUX_PPROF:" && e2e_plain | grep -q "\[e2e-exit 1\]"'

# ---- 不设：不监听
e2e_start -x 160 "$E2E_BIN"; wait_for 6 flag_is alternate_on 1; sleep 1; PID=$(spid)
check "不设 SQLMUX_PPROF：sqlmux 进程（pid ${PID}）没有监听任何 TCP 端口" eval '[[ -n $PID ]] && [[ -z $(lsof -nP -a -p "$PID" -iTCP -sTCP:LISTEN 2>/dev/null) ]]'
e2e_done
