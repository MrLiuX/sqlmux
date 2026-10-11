#!/usr/bin/env bash
# F13.1 COMMIT 发出之后断线，不再报「已回滚」（PG）（specs/m13-quality/task.md F13.1；tech-design §8.2）
# 连接经一个会在 COMMIT 发出后立刻掐断的代理（e2e/commitcut.py）：C-s 保存报「提交结果未知」，修改标记保留，库里已提交；query 里手写 commit 同样。自建库 t_c。
. "$(dirname "$0")/lib.sh"
e2e_build || exit 1
. "$(dirname "$0")/palette.sh"
. "$(dirname "$0")/paging.sh"
. "$(dirname "$0")/commit.sh"

D=$(mktemp -d "${TMPDIR:-/tmp}/sqlmux-e2e-cfg.XXXXXX"); trap 'e2e_stop; cut_stop; rm -rf "$D"' EXIT
e2e_own_db || { echo "e2e: could not create the private database" >&2; exit 1; }
psql "$E2E_DB" -q -c "create table t_c (id int primary key, v text)" -c "insert into t_c values (1,'one'),(2,'two'),(3,'three')" >/dev/null
DBVALS() { psql "$E2E_DB" -At -c "select string_agg(v, ',' order by id) from t_c"; }
DBEXEC() { psql "$E2E_DB" -q -c "$1" >/dev/null; }
setup_cut postgres "$E2E_DB"
commit_body
e2e_done
