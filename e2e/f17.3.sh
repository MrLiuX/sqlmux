#!/usr/bin/env bash
# F17.3 命令行直接给 redis://（specs/m17-polish-exec/task.md F17.3；tech-design §14「启动」）
# 位置参数是 redis:// 或 rediss:// URL 时开一个临时 Redis 连接，名字 <db>@<主机>（没有 db 用 db0），规则同 F14.1 的 URL：
# 不写 connections.toml、不存钥匙串、不存 last.toml。每个脚本自己起 redis-server（一个有密码）。
. "$(dirname "$0")/lib.sh"
e2e_build || exit 1
. "$(dirname "$0")/palette.sh"
. "$(dirname "$0")/redis.sh"

D=$(mktemp -d "${TMPDIR:-/tmp}/sqlmux-e2e-cfg.XXXXXX"); trap 'e2e_stop; redis_stop; rm -rf "$D"' EXIT
redis_start R || exit 1
redis_start A --requirepass pw1 || exit 1
redis_seed; rca set pwkey v >/dev/null
RDB=127.0.0.1
rstart() { e2e_start "$E2E_BIN $1"; wait_for 8 flag_is alternate_on 1; sleep 2.5; }
quit_app() { e2e_type ":qa"; e2e_keys Enter; wait_for 8 exited; sleep 0.5; }
screen_has() { e2e_plain | grep -qF -- "$1"; }
side() { e2e_plain | noicon | sed 's/^│//; s/│.*//' | sed 's/^ *//; s/ *$//'; }                # the sidebar's rows
conns_file() { echo "$E2E_TMP/config/sqlmux/connections.toml"; }
untouched() { grep -q doraemon "$(conns_file)" && ! grep -q "$RDB" "$(conns_file)" && [[ -z $(find "$E2E_TMP/state" "$E2E_TMP/data" "$E2E_TMP/config" -name "last.toml*" 2>/dev/null) && -z $(ls "$E2E_TMP/keychain") ]]; }

# ---- redis://主机:端口/2：名字 db2@127.0.0.1，树里是它一个连接，db2 展开（里面没有键）
rstart "redis://$RDB:$RPORT/2"
check "窗口标题和树里的名字是 db2@127.0.0.1（<db>@<主机>）" eval 'e2e_plain | sed -n 1p | noicon | grep -qE "⓪ +db2@$RDB " && side | grep -qE "^▾ +db2@$RDB" || { e2e_plain | head -6 | cut -c1-80; false; }'
check "树：db0、db1 收着，db2（URL 里的）展开" eval '[[ $(side | grep -E "^[▸▾] +db[0-9]+( |$)" | sed "s/  *[0-9]*$//" | tr -s " " | tr "\n" ",") == "▸ db0,▸ db1,▾ db2," ]] || { side | head -8; false; }'
quit_app
check "退出后：connections.toml 里没有它、没有 last.toml、钥匙串（假的）里没东西" eval 'untouched' || { cat "$(conns_file)"; find "$E2E_TMP" -name "last.toml*"; ls "$E2E_TMP/keychain"; }
_e2e_kill

# ---- 没有 db：db0；连得上、键读得出来；快速 SQL 在这个临时连接上能跑
rstart "redis://$RDB:$RPORT"
check "没有 db：名字是 db0@127.0.0.1，db0 展开、键读出来了（app 命名空间在树里）" eval 'e2e_plain | sed -n 1p | noicon | grep -qE "⓪ +db0@$RDB " && side | grep -qE "^▾ +db0 " && side | grep -qE "^▸ +app " || { side | head -8; false; }'
key Escape; key C-h; sleep 0.3; e2e_type ";GET str:plain"; sleep 1.2; [[ -n $(pop) ]] && { key Escape; sleep 0.3; }; key Enter; sleep 1.8
check "临时连接是活的：快速 SQL 跑 GET str:plain → hello" eval '[[ $(grid | tr "\n" /) == "hello/" ]] || { title; grid; false; }'
quit_app
_e2e_kill

# ---- 带密码的 URL：用 URL 里的密码连上（读得到 pwkey），密码不上屏、不进钥匙串
rstart "redis://:pw1@$RDB:$APORT/0"
check "带密码：连上了，pwkey 在树里；名字里没有密码" eval 'side | grep -qE "^▸ +pwkey|pwkey" && e2e_plain | sed -n 1p | noicon | grep -qE "⓪ +db0@$RDB " && ! screen_has pw1 || { side | head -8; false; }'
quit_app
check "带密码的 URL 退出后：配置里没有它、没有 last.toml、密码没进钥匙串" eval 'untouched' || { cat "$(conns_file)"; ls "$E2E_TMP/keychain"; }
_e2e_kill
rstart "redis://:wrong@$RDB:$APORT/0"
check "（对照）密码错：读不到 pwkey（上面那条不是没认证也读得到）" eval '! side | grep -q pwkey'
_e2e_kill

# ---- rediss://（TLS）：同样按 URL 开一个临时连接，名字也是 <db>@<主机>；服务端没开 TLS，连不上，但不崩、不写配置
rstart "rediss://$RDB:$RPORT/3"
check "rediss://：窗口标题 db3@127.0.0.1，程序还在（没有退出）" eval 'e2e_plain | sed -n 1p | noicon | grep -qE "⓪ +db3@$RDB " && ! screen_has "[e2e-exit" || { e2e_plain | head -4 | cut -c1-100; false; }'
quit_app
check "rediss:// 退出后：配置里没有它、没有 last.toml" eval 'untouched' || { cat "$(conns_file)"; find "$E2E_TMP" -name "last.toml*"; }
_e2e_kill
# ---- db 的另外两种写法：?db=3、路径 /02 → 都按数字：db3@、db2@，树也在对应的 db 上
rstart "redis://$RDB:$RPORT?db=3"
check "?db=3：名字 db3@127.0.0.1，树里 db3 展开、db0 收着" eval 'e2e_plain | sed -n 1p | noicon | grep -qE "⓪ +db3@$RDB " && side | grep -qE "^ *▾ +db3( |$)" && side | grep -qE "^ *▸ +db0( |$)"' || { e2e_plain | head -3 | cut -c1-80; side | head -8; false; }
quit_app
_e2e_kill
rstart "redis://$RDB:$RPORT/02"
check "/02：名字 db2@127.0.0.1（前导零按数字），树里 db2 展开" eval 'e2e_plain | sed -n 1p | noicon | grep -qE "⓪ +db2@$RDB " && side | grep -qE "^ *▾ +db2( |$)"' || { e2e_plain | head -3 | cut -c1-80; side | head -8; false; }
quit_app
_e2e_kill
e2e_done
