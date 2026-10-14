#!/usr/bin/env bash
# F16.4 Redis 控制台（二）：连接丢了之后的重跑规则（specs/design/08-data.md §8.7「连接」、§8.2「重试」）
# 一次执行里每一行的命令都带服务端 COMMAND 的 readonly flag 才重跑（「已发出」也重跑）；否则照旧报原错误，toast「已重连，请重新执行」。
# 不重跑的有 PUBLISH、EXEC、PING、INFO、写命令、服务端没列出的；MULTI → SET → 断线 → EXEC 不重跑 EXEC。丢连接用杀隧道（假 ssh）和 CLIENT KILL 两种。
# 面板的命令列表在 Redis 连接上没有「快速 SQL」和「格式化」，在 SQL 连接上照旧。
. "$(dirname "$0")/lib.sh"
e2e_build || exit 1
. "$(dirname "$0")/palette.sh"
. "$(dirname "$0")/redis.sh"
command -v nc >/dev/null || { echo "e2e: no nc"; exit 1; }

D=$(mktemp -d "${TMPDIR:-/tmp}/sqlmux-e2e-cfg.XXXXXX"); trap 'e2e_stop; redis_stop; rm -rf "$D"' EXIT
redis_start R || exit 1
mkdir -p "$D/c" "$D/bin"
cat >"$D/bin/ssh" <<'SH'
#!/bin/sh
printf '%s\n' "$*" >>"$SSHLOG"
echo $$ >>"$SSHPIDS"
exec nc localhost "${2##*:}"
SH
chmod +x "$D/bin/ssh"; export SSHLOG=$D/ssh.log SSHPIDS=$D/pids; : >"$SSHLOG"; : >"$SSHPIDS"
(umask 077; cat >"$D/c/connections.toml" <<TOML
[[connection]]
name = "rd"
engine = "redis"
dsn = "redis://localhost:$RPORT/0"

[[connection]]
name = "viassh"
engine = "redis"
dsn = "redis://redisbox.internal:$RPORT/0"
ssh = "me@bastion"

[[connection]]
name = "pg"
engine = "postgres"
dsn = "$SQLMUX_TEST_PG"
TOML
)
reseed() { rc flushall >/dev/null; redis_seed; }
nlog() { wc -l <"$SSHLOG" | tr -d ' '; }
quit_app() { e2e_type ":"; sleep 0.4; e2e_type qa; key Enter; wait_for 8 exited; sleep 0.5; }
sstart() { e2e_start -C "$D/c" -P "$D/bin:$PATH" "$E2E_BIN $1"; wait_for 8 flag_is alternate_on 1; sleep 2; }
runl() { key C-l; key g g; key d G; key i; e2e_type "$1"; key Escape; key Enter; sleep 1.5; }   # the console: replace its text with one line, run it
rrows() { local g y; g=($(geom 3)); for ((y = g[1] + 1; y < g[1] + g[3] - 2; y++)); do e2e_text $((g[0] + 1)) $((g[0] + g[2] - 2)) "$y"; done | python3 -c "
import re, sys
for l in sys.stdin:
    t = re.sub(r'\\s+', ' ', re.sub('[\ue000-\uf8ff]', '', l.replace('│', ' ').replace('┼', ' ')).strip())
    if not t or set(t) <= set('─ '): continue
    print(t)"; }
logline() { local g y; g=($(geom 3)); for ((y = g[1] + 1; y < g[1] + g[3] - 1; y++)); do e2e_text $((g[0] + 1)) $((g[0] + g[2] - 2)) "$y"; done | grep -a "$1" | tail -1; }
lastlines() { local g y; g=($(geom 3)); for ((y = g[1] + 1; y < g[1] + g[3] - 1; y++)); do e2e_text $((g[0] + 1)) $((g[0] + g[2] - 2)) "$y"; done | sed 's/ *│ *$//; s/^ *│ *//' | grep -aE "^ *[0-9]{2}:[0-9]{2}:[0-9]{2} " | tail -"$1"; }   # the newest N lines of the log tab
toast() { e2e_text 1 "$(e2e_flag pane_width)" "$(( $(H) - 1 ))" | python3 -c 'import sys; print(sys.stdin.read().strip("└─┘│ \n"))'; }
cut_tunnel() { kill $(cat "$SSHPIDS") 2>/dev/null; : >"$SSHPIDS"; sleep 1; }   # the ssh processes die
cut_client() { rc client kill type normal skipme yes >/dev/null; sleep 1; }    # the server closes the connections
runq() { key C-l; key g g; key d G; key i; e2e_type "$1"; key Escape; e2e_keys Enter; local i; TOAST=; for i in 1 2 3 4 5 6 8; do sleep 0.3; TOAST=$(toast); [[ -n $TOAST ]] && break; done; sleep 1.2; }   # run one line; TOAST is what the status bar says first (it goes in 2–3 s)
RERAN="viassh：已重连，请重新执行"

# ---- 经 ssh：杀掉隧道。readonly 命令自动重拨、重跑，结果回来；别的不重跑，toast「已重连，请重新执行」
reseed; sstart viassh
key C-l; key C-l; runl "GET str:plain"
for c in "GET str:plain|1 hello" "TTL str:ttl|1 3153599" "DBSIZE|1 29" "SCAN 0 COUNT 5|1 "; do
  cmd=${c%%|*}; want=${c#*|}; N0=$(nlog); cut_tunnel; runl "$cmd"
  check "杀掉隧道之后跑 ${cmd}（readonly）：自动起新的 ssh、重跑，结果回来（第一行是「${want}…」）" eval '(( $(nlog) > N0 )) && [[ $(rrows | sed -n 2p) == "$want"* ]]' || { echo "  log $N0 -> $(nlog)"; rrows | head -3; logline "${cmd%% *}"; }
done
rc subscribe ch >"$D/sub.txt" 2>&1 & SUB=$!; sleep 0.5
for cmd in "PING" "INFO server" "SET zz 1" "PUBLISH ch m"; do
  N0=$(nlog); cut_tunnel; runq "$cmd"
  check "杀掉隧道之后跑 ${cmd}（不在可重跑的名单里）：不重跑，toast「已重连，请重新执行」，日志里是原来的断线错误" eval '[[ $TOAST == "$RERAN" && $(logline "${cmd%% *}") == *"ERROR: ssh me@bastion: signal: terminated"* ]]' || { echo "  toast [$TOAST]"; logline "${cmd%% *}"; }
done
check "SET zz 1 没有被写进去（zz 不存在），PUBLISH ch m 订阅端一次也没收到（不重跑，不会重复发）" eval '[[ $(rc exists zz) == 0 && $(grep -c "^m$" "$D/sub.txt") == 0 ]]' || { rc exists zz; cat "$D/sub.txt"; }
runl "PUBLISH ch m2"; sleep 0.5
check "照提示重新执行 PUBLISH ch m2：订阅端收到一次" eval '[[ $(grep -c "^m2$" "$D/sub.txt") == 1 ]]' || cat "$D/sub.txt"
kill $SUB 2>/dev/null; wait $SUB 2>/dev/null
runl "MULTI"; runl "SET mm 1"; cut_tunnel; runq "EXEC"
check "MULTI → SET mm 1 → 杀隧道 → EXEC：不重跑 EXEC（mm 没有写），toast「已重连，请重新执行」，日志是原来的断线错误，不是「EXEC without MULTI」" eval '[[ $TOAST == "$RERAN" && $(rc exists mm) == 0 && $(logline EXEC) == *"signal: terminated"* && $(logline EXEC) != *"without MULTI"* ]]' || { echo "  toast [$TOAST]"; rc exists mm; logline EXEC; }
# 一次执行好几行（选中多行再 ↵）
key C-l; key g g; key d G; key i; e2e_type "GET str:plain"; key Escape; key o; e2e_type "TTL str:ttl"; key Escape
N0=$(nlog); cut_tunnel; key g g; key V; key G; key Enter; sleep 2.5
key C-j; key g T; sleep 0.6   # the log tab of ③
check "两行都是 readonly（GET、TTL），隧道断了，选中两行一起执行：自动重拨（日志里多了一次 ssh），两条都重跑成功（日志最新的两行是 GET 和 TTL，都是「1 行」，没有 ERROR）" eval '(( $(nlog) > N0 )) && [[ $(lastlines 2 | grep -ac "1 行") == 2 && $(lastlines 2 | grep -ac "ERROR") == 0 && $(lastlines 1) == *"TTL str:ttl"* ]]' || { echo "  log $N0 -> $(nlog)"; lastlines 3; }
key C-k   # ② again (the focus is in ③)
key C-l; key g g; key d G; key i; e2e_type "GET str:plain"; key Escape; key o; e2e_type "SET zz2 1"; key Escape
cut_tunnel; key g g; key V; key G; e2e_keys Enter
for i in 1 2 3 4 5 6 8; do sleep 0.3; TOAST=$(toast); [[ -n $TOAST ]] && break; done; sleep 1.5
check "里面有一行写命令（GET、SET zz2 1），隧道断了，选中两行一起执行：整次不重跑，toast「已重连，请重新执行」，zz2 没有写进去" eval '[[ $TOAST == "$RERAN" && $(rc exists zz2) == 0 ]]' || { echo "  toast [$TOAST]"; rc exists zz2; }
quit_app; _e2e_kill

# ---- 服务端关了连接（CLIENT KILL）：readonly 命令同样自动重跑
reseed; sstart rd
key C-l; key C-l; runl "GET str:plain"
cut_client; runl "GET str:number"
check "CLIENT KILL 之后跑 GET：结果回来（42）" eval '[[ $(rrows | sed -n 2p) == "1 42" ]]' || { rrows | head -3; logline GET; }
cut_client; runl "DBSIZE"
check "CLIENT KILL 之后跑 DBSIZE：结果回来（29）" eval '[[ $(rrows | sed -n 2p) == "1 29" ]]' || { rrows | head -3; logline DBSIZE; }
quit_app; _e2e_kill

# ---- 面板的命令列表：Redis 连接上没有「快速 SQL」和「格式化」，SQL 连接上照旧
sstart rd
pal ">快速"; sleep 0.5
check "Redis 连接：面板 >快速 里没有「快速 SQL」" eval '! e2e_plain | grep -q "palette.sql"' || e2e_plain | grep -a 快速
key Escape; sleep 0.3
pal ">格式化"; sleep 0.5
check "Redis 连接：面板 >格式化 里没有「格式化整个 query」" eval '! e2e_plain | grep -q "格式化整个 query"' || true
key Escape; sleep 0.3
quit_app; _e2e_kill
sstart pg
pal ">快速"; sleep 0.5
check "SQL 连接（pg）：面板 >快速 里有「快速 SQL」" eval 'e2e_plain | grep -q "palette.sql"' || e2e_plain | sed -n 1,16p | cut -c34-150
key Escape; sleep 0.3
pal ">格式化"; sleep 0.5
check "SQL 连接（pg）：面板 >格式化 里有「格式化整个 query」" eval 'e2e_plain | grep -q "格式化整个 query"' || e2e_plain | sed -n 1,16p | cut -c34-150
key Escape; sleep 0.3
quit_app; _e2e_kill

e2e_done
