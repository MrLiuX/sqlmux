#!/usr/bin/env bash
# F10.4 console 结果默认 500 行并翻页（PG）（specs/m10-feedback3/task.md F10.4；tech-design §11「结果翻页」、§9.4）
# 1203 行的表：第 1 页 500+ 行，] 翻到 501–1000+，再 ] 到 1001–1203，最后一页 ] 无事，[ 往回翻，R 回第 1 页，带 LIMIT 的语句没有翻页。
# SQL 的拼法（LIMIT 501 OFFSET 500）和三页 id 连续在集成测试里；这里看真实终端里的按键、标题和翻页后的内容。自建库。
. "$(dirname "$0")/lib.sh"
e2e_build || exit 1
. "$(dirname "$0")/palette.sh"
. "$(dirname "$0")/paging.sh"

D=$(mktemp -d "${TMPDIR:-/tmp}/sqlmux-e2e-cfg.XXXXXX")
trap 'e2e_stop; rm -rf "$D"' EXIT
e2e_own_db || { echo "e2e: could not create the private database" >&2; exit 1; }
psql "$E2E_DB" -q -c "create table t_page as select g as id from generate_series(1, 1203) g" >/dev/null
mkdir -p "$D/own"; printf '[[connection]]\nname = "doraemon"\nengine = "postgres"\ndsn = "%s"\n' "$E2E_DB" >"$D/own/connections.toml"; chmod 600 "$D/own/connections.toml"
paging_body

# ---- 手动事务模式：翻页就是在同一个事务里再执行一条语句（先 begin）
_e2e_kill; start -x 200 -C "$D/own"; sleep 1.5; key C-l                      # 宽一点的终端：console 标题放得下「Tx: 手动」
pal ">手动提交事务"; sleep 0.6; key Enter; sleep 0.6
setsql "select id from t_page order by id"; run
idle_tx() { psql "$E2E_DB" -At -c "select count(*) from pg_stat_activity where datname = current_database() and state like 'idle in transaction%'"; }
check "（准备）手动模式：执行后服务端有一个 idle in transaction（begin 之后的 select），第 1 页 500+ 行" eval 'title_has "500+ 行" && [[ $(idle_tx) == 1 ]] || { echo "  idle=$(idle_tx)"; false; }'
key C-j; paged "]" "501–1000+ 行"
check "手动事务模式下 ]：第 2 页（id 501 起），还是同一个事务里的一条语句：服务端仍只有一个 idle in transaction" eval '[[ $(firstid) == 501 && $(idle_tx) == 1 ]] || { echo "  first=$(firstid) idle=$(idle_tx)"; false; }'
e2e_done
