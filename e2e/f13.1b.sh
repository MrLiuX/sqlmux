#!/usr/bin/env bash
# F13.1 COMMIT 发出之后断线，不再报「已回滚」（MySQL）（specs/m13-quality/task.md F13.1）：同 f13.1.sh，在 MySQL 8.4 的自建库上。
. "$(dirname "$0")/lib.sh"
e2e_build || exit 1
. "$(dirname "$0")/palette.sh"
. "$(dirname "$0")/paging.sh"
. "$(dirname "$0")/commit.sh"

D=$(mktemp -d "${TMPDIR:-/tmp}/sqlmux-e2e-cfg.XXXXXX"); trap 'e2e_stop; cut_stop; rm -rf "$D"' EXIT
e2e_own_mydb || { echo "e2e: could not create the private MySQL database" >&2; exit 1; }
e2e_mysql "sqlmux_e2e_$$" <<<"create table t_c (id int primary key, v varchar(20)); insert into t_c values (1,'one'),(2,'two'),(3,'three')"
DBVALS() { e2e_mysql "sqlmux_e2e_$$" <<<"select group_concat(v order by id) from t_c"; }
DBEXEC() { e2e_mysql "sqlmux_e2e_$$" <<<"$1"; }
setup_cut mysql "$E2E_MYDB"
commit_body
e2e_done
