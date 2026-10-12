#!/usr/bin/env bash
# F13.4 确认框开着时按键和粘贴不漏进底下的输入框（specs/m13-quality/task.md F13.4）
# 标删一行，/ 开 WHERE 打 id，C-s 弹「将删除 1 行」的确认框：按 x、粘贴 Z 都不该进 WHERE（以前 WHERE 变成 idxZ）；确认框回答之后 WHERE 还是 id。自建库 t_d。
. "$(dirname "$0")/lib.sh"
e2e_build || exit 1
. "$(dirname "$0")/palette.sh"

D=$(mktemp -d "${TMPDIR:-/tmp}/sqlmux-e2e-cfg.XXXXXX"); trap 'e2e_stop; rm -rf "$D"' EXIT
e2e_own_db || { echo "e2e: could not create the private database" >&2; exit 1; }
psql "$E2E_DB" -q -c "create table t_d (id int primary key, v text)" -c "insert into t_d select g, 'v' || g from generate_series(1, 5) g" >/dev/null
mkdir -p "$D/own"; printf '[[connection]]\nname = "doraemon"\nengine = "postgres"\ndsn = "%s"\n' "$E2E_DB" >"$D/own/connections.toml"; chmod 600 "$D/own/connections.toml"
paste() { t set-buffer -b e2e "$1"; t paste-buffer -r -p -b e2e -t t; sleep 0.4; }
rows() { psql "$E2E_DB" -At -c "select count(*) from t_d"; }

printf "confirm_dangerous = true\n" >"$D/own/config.toml"   # 确认框要 confirm_dangerous（lib.sh 默认关）
SOLO=1 start -C "$D/own"; open_table t_d; wait_for 8 settled
key d d; sleep 0.4
key /; sleep 0.3; e2e_type "id"; sleep 0.3
e2e_keys C-s; sleep 0.8
check "（准备）WHERE 里有 id，C-s 弹出确认框（将删除）" eval 'screen_has "将删除" && [[ $(where_in) == id ]]'
e2e_type x; sleep 0.4
check "确认框开着时按 x：WHERE 还是 id（不是 idx）" eval '[[ $(where_in) == id ]] && screen_has "将删除"' || where_in
paste "Z"
check "确认框开着时粘贴 Z：WHERE 还是 id（不是 idxZ）" eval '[[ $(where_in) == id ]]' || where_in
key C-p; sleep 0.4
check "确认框开着时 C-p：不开命令面板（§6.4 confirm 只放行 confirm.* 和取消）" eval '! palette_open && screen_has "将删除" && [[ $(where_in) == id ]]'
e2e_type "jl"; sleep 0.3
check "确认框还在（x、粘贴、别的键都没有把它关掉）" eval 'screen_has "将删除"'
key Escape; sleep 0.6
check "esc 放弃：确认框关了，WHERE 还是 id，库里 5 行都还在（没删）" eval '! screen_has "将删除" && [[ $(rows) == 5 ]] && [[ $(where_in) == id ]]' || { rows; where_in; }
e2e_done
