#!/usr/bin/env bash
# F12.6 补全列表的排序（specs/m12-grid/task.md F12.6；tech-design §9.7「排序」）
# 先按匹配层（完全相同 → 候选开头的前缀 → 词首前缀 → 其余模糊），层内按候选组顺序，组内按得分；语句开头处候选组是 关键字 → 表 → 列，别处是 列 → 表 → 关键字。
# query、WHERE、快速 SQL 同一套排序。自建库有表 pap_secretary_problem_event 和 oa_workflow_instance（用户报的两个例子）。
. "$(dirname "$0")/lib.sh"
e2e_build || exit 1
. "$(dirname "$0")/palette.sh"

D=$(mktemp -d "${TMPDIR:-/tmp}/sqlmux-e2e-cfg.XXXXXX"); trap 'e2e_stop; rm -rf "$D"' EXIT
e2e_own_db || { echo "e2e: could not create the private database" >&2; exit 1; }
psql "$E2E_DB" -q -c "create table pap_secretary_problem_event (id int primary key, status text)" -c "create table oa_workflow_instance (id int primary key, state text)" -c "create table oa_workflow_instance_log (id int primary key)" -c "create table oa_workflow_instance_node (id int primary key)" >/dev/null
mkdir -p "$D/own"; printf '[[connection]]\nname = "doraemon"\nengine = "postgres"\ndsn = "%s"\n' "$E2E_DB" >"$D/own/connections.toml"; chmod 600 "$D/own/connections.toml"
pop() { e2e_panes | awk '$1 == "-" { print $2, $3, $4, $5 }' | tail -1; }
items() { local g y; g=($(pop)); [[ -n ${g[0]} ]] || return 0
  for ((y = g[1] + 1; y < g[1] + g[3] - 1; y++)); do e2e_text $((g[0] + 4)) $((g[0] + g[2] - 2)) $y | sed 's/^ *//; s/ *$//'; done; }
names() { items | sed 's/  .*//' | grep -v '^$' | tr '\n' '|'; }
first() { names | cut -d'|' -f1; }
fresh() { key Escape; key Escape; key g g; key d G; key i; }
pos_of() { names | tr '|' '\n' | grep -nxF "$1" | cut -d: -f1; }   # the 1-based place of NAME in the list

start -C "$D/own"; key C-l; key i
e2e_type "sele"; sleep 0.6
check "query 里打 sele：第一项是 select（不是 pap_secretary_problem_event）" eval '[[ $(first) == select ]] || { echo "  candidates: $(names)"; false; }'
fresh; e2e_type "inse"; sleep 0.6
check "打 inse：第一项是 insert into，oa_workflow_instance* 在它后面" eval '[[ $(first) == "insert into" ]] || { echo "  candidates: $(names)"; false; }'
check "oa_workflow_instance 这几张表仍在列表里（只是排在关键字后面）" eval 'n=$(names); [[ $n == *oa_workflow_instance* ]] || { echo "  candidates: $n"; false; }'
fresh; e2e_type "sec"; sleep 0.6
check "语句开头打 sec：pap_secretary_problem_event（词首前缀）在列表里" eval '[[ -n $(pos_of pap_secretary_problem_event) ]] || { echo "  candidates: $(names)"; false; }'
fresh; e2e_type "select * from pap_secretary_problem_event where sta"; sleep 0.6
check "from … where sta：列 status 排第一（列 → 表 → 关键字，不在语句开头）" eval '[[ $(first) == status ]] || { echo "  candidates: $(names)"; false; }'
fresh; e2e_type "select * from t_o"; sleep 0.6
check "select * from t_o（不在语句开头）：表 t_order 排在关键字前，第一项是 t_order" eval '[[ $(first) == t_order ]] || { echo "  candidates: $(names)"; false; }'
fresh; e2e_type "select 1; sele"; sleep 0.6
check "第二条语句开头（; 之后）打 sele：第一项仍是 select" eval '[[ $(first) == select ]] || { echo "  candidates: $(names)"; false; }'
key Escape; key Escape; key Escape; sleep 0.3

# ---- 快速 SQL：同一套排序
e2e_keys C-p; sleep 0.3; e2e_type ";sele"; sleep 0.6
check "快速 SQL 输入 ;sele：第一项是 select" eval '[[ $(first) == select ]] || { echo "  candidates: $(names)"; false; }'
key Escape; key Escape; sleep 0.3

# ---- WHERE：列在前（不在语句开头）
_e2e_kill
SOLO=1 start -C "$D/own"; open_table t_order; wait_for 8 settled
key /; clear_in; e2e_type "sta"; sleep 0.6
check "WHERE 输入框里打 sta：第一项是列 status" eval '[[ $(first) == status ]] || { echo "  candidates: $(names)"; false; }'
e2e_done
