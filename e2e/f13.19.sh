#!/usr/bin/env bash
# F13.19 多语句格式化逐条跑（specs/m13-quality/task.md F13.19；tech-design §9.5）
# 几条语句的文本 gqG：每条都格式化成多行，语句之间的注释原样保留；48 条语句一次 gqG 在限时内完成（以前整段一次调用会超时）。只读，用共用库。
. "$(dirname "$0")/lib.sh"
e2e_build || exit 1
. "$(dirname "$0")/palette.sh"
. "$(dirname "$0")/paging.sh"

D=$(mktemp -d "${TMPDIR:-/tmp}/sqlmux-e2e-cfg.XXXXXX"); trap 'e2e_stop; rm -rf "$D"' EXIT
F=$D/data/sqlmux/queries/query_1.sql
wcmd() { e2e_type ":"; sleep 0.3; e2e_type "w"; key Enter; sleep 0.6; }
start -D "$D/data" -S "$D/st"; sleep 1; key C-l
setsql "$(printf 'select a,b from t1 where x=1;\n-- keep me\n\nselect c from t2;')"
key g g; e2e_type "gq"; sleep 0.2; e2e_type G; sleep 3; wcmd
check "两条语句 gqG：都格式化成多行（有单独的 from / where 行）" eval 'grep -qx "from" "$F" && grep -qx "where" "$F" && [[ $(grep -c "^select$" "$F") == 2 ]] || { cat "$F"; false; }'
check "语句之间的注释 -- keep me 原样还在" eval 'grep -qx -- "-- keep me" "$F"'

# ---- 48 条语句
sql=$(python3 -c 'print("\n".join("select a%d, b from t%d where x = %d;" % (i, i, i) for i in range(48)))')
setsql "$sql"
t0=$(date +%s); key g g; e2e_type "gq"; sleep 0.2; e2e_type G
for i in $(seq 1 60); do sleep 0.5; wcmd; [[ $(grep -cx "where" "$F") -ge 48 ]] && break; done
took=$(( $(date +%s) - t0 ))
check "48 条语句 gqG：全部格式化（48 个 where 行），用时 ${took}s，没有超时报错" eval '[[ $(grep -cx "where" "$F") == 48 ]] && ! screen_has "超时" && ! screen_has "timeout"' || { grep -cx where "$F"; e2e_cap | tail -3; }
check "用时在 10 秒内（以前整段一次调用 7KB 要 4 秒或撞 5 秒超时；逐条 1.4 秒）" eval '(( took <= 10 ))'
e2e_done
