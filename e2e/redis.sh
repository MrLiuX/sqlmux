# Redis for the e2e scripts of M16: a server of its own per script (redis-server on a free port), so that what a script does to the
# keyspace, the clients and the ACL is its own. Source it after lib.sh. The seed is testdata/seed/redis.txt, db0 and db1.
#   redis_start PREFIX [redis-server args…]   starts one: ${PREFIX}PORT, ${PREFIX}DIR, ${PREFIX}PID; PREFIX R is the main one, A the one with a password
#   rc [redis-cli args…]                      redis-cli on the R server (rca: on the A server, authenticated)
#   redis_seed                                the seed without its two EVALs into db0 and db1 of the R server (the tree reads all of it)
#   redis_big N                               the two EVALs, a hash of 6000 fields and 12000 keys, into db N of the R server
#   redis_stop                                every server the script started; the EXIT trap calls it
# A machine with no redis-server / redis-cli skips the script (a summary of nothing).
command -v redis-server >/dev/null && command -v redis-cli >/dev/null || { echo "e2e: no redis-server / redis-cli on this machine, skipped"; echo "== 0 passed, 0 failed"; exit 0; }
SEED=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)/testdata/seed/redis.txt
RPORT= APORT= RPID= APID= RDIR= ADIR=
redis_start() {
  local p=$1 dir port i; shift
  dir=$(mktemp -d "${TMPDIR:-/tmp}/sqlmux-e2e-redis.XXXXXX")
  port=$(python3 -c 'import socket; s = socket.socket(); s.bind(("127.0.0.1", 0)); print(s.getsockname()[1])')
  redis-server --port "$port" --bind 127.0.0.1 --save "" --appendonly no --dir "$dir" "$@" >"$dir/server.log" 2>&1 & local pid=$!
  printf -v "${p}PORT" %s "$port"; printf -v "${p}DIR" %s "$dir"; printf -v "${p}PID" %s "$pid"
  for ((i = 0; i < 50; i++)); do redis-cli -p "$port" ping 2>&1 | grep -qE "PONG|NOAUTH" && return 0; sleep 0.1; done
  echo "e2e: redis-server did not start" >&2; cat "$dir/server.log" >&2; return 1
}
rc() { redis-cli -p "$RPORT" "$@"; }
rca() { redis-cli -p "$APORT" -a pw1 --no-auth-warning "$@"; }
redis_seed() { grep -v '^EVAL' "$SEED" | rc >/dev/null; }
redis_big() { grep '^EVAL' "$SEED" | rc -n "$1" >/dev/null; }
redis_stop() {
  local p
  for p in R A; do local pid=${p}PID dir=${p}DIR; [[ -n ${!pid} ]] && { kill "${!pid}" 2>/dev/null; wait "${!pid}" 2>/dev/null; }; [[ -n ${!dir} ]] && rm -rf "${!dir:?}"; done
  RPID= APID= RDIR= ADIR=
}
