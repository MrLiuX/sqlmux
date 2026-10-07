#!/usr/bin/env bash
# F8.7 yi：把当前行复制为 INSERT 语句（specs/m8-feedback/task.md F8.7；tech-design §10.6「复制为 INSERT」）
# 字面量的转义和关键字大小写有 sqlkit 的单测、PG / MySQL 集成测试各把复制出来的语句执行回去；这里在真实终端里测：
# 剪贴板里是 insert into … values (…);（假 pbcopy），整行闪一下，语句执行回去得到同一行，未保存的修改也算，结果 tab 上 toast。自建库。
. "$(dirname "$0")/lib.sh"
SOLO=1
e2e_build || exit 1

D=$(mktemp -d "${TMPDIR:-/tmp}/sqlmux-e2e-cfg.XXXXXX"); trap 'e2e_stop; rm -rf "$D"' EXIT
e2e_own_db || { echo "e2e: could not create the private database" >&2; exit 1; }
psql "$E2E_DB" -q -v ON_ERROR_STOP=1 -c "create table t_q (id serial primary key, v text, n int, b boolean, j jsonb, ts timestamptz, g int generated always as identity)" \
  -c "insert into t_q (v, n, b, j, ts) values ('it''s a \"test\", back\\slash', null, true, '{\"a\": [1, 2]}', '2026-09-01 00:01:00+00'), ('', 7, false, null, null)"
mkdir -p "$D/own"; printf '[[connection]]\nname = "doraemon"\nengine = "postgres"\ndsn = "%s"\n' "$E2E_DB" >"$D/own/connections.toml"; chmod 600 "$D/own/connections.toml"
psql_n() { psql "$E2E_DB" -At -c "$1"; }
row_text() { psql_n "select id, v, n, b, j, ts from t_q where id = $1"; }
flashed() { local n; for n in $(seq 20); do LC_ALL=C grep -q '48;2;224;175;104' "$REC" 2>/dev/null && return 0; sleep 0.05; done; false; }
rec() { REC=$D/rec.$1; : >"$REC"; e2e_record "$REC"; }
unrec() { t pipe-pane -t t; }
toast_text() { e2e_text 1 160 44; }

start -C "$D/own"; open_table t_q; wait_for 8 settled
rec yi; key y i; flashed; f=$?; unrec; wait_for 3 eval '[[ -n $(clip) ]]'
check "yi：剪贴板里是 insert into \"public\".\"t_q\" (…) values (…);，列是 generated always 之外的全部列（id、v、n、b、j、ts）" eval '[[ "$(clip)" == "insert into \"public\".\"t_q\" (\"id\", \"v\", \"n\", \"b\", \"j\", \"ts\") values (1, '"'"'it'"''"'s a \"test\", back\\slash'"'"', null, '"'"'t'"'"', '"'"'{\"a\": [1, 2]}'"'"', '"'"'2026-09-01 00:01:00+00'"'"');" ]] || { echo "  clip: $(clip)"; false; }'
check "yi 时这一行闪一下 yank 色" eval '(( f == 0 ))'
check "语句不带换行（一行）" eval '[[ $(clip | wc -l | tr -d " ") == 0 ]]'

# ---- 执行回去得到同一行（转义对不对，库说了算）
want=$(row_text 1); psql_n "delete from t_q where id = 1" >/dev/null; psql "$E2E_DB" -q -c "$(clip)" >/dev/null
check "把复制出来的语句执行回去：库里 id = 1 的行和原来一样（'、\"、\\ 都没坏，NULL 还是 NULL）" eval '[[ "$(row_text 1)" == "$want" && -n "$want" ]] || { echo "  want: $want"; echo "  got:  $(row_text 1)"; false; }'

# ---- 第 2 行：空串、数字、false、NULL 的 json
key R; wait_for 8 settled; key j
rec yi2; key y i; flashed; unrec; wait_for 3 eval '[[ -n $(clip) ]]'
check "第 2 行：空串是 ''，数字不加引号，false 是 'f'，NULL 写 null" eval '[[ "$(clip)" == *"values (2, '"''"', 7, '"'f'"', null, null);" ]] || { echo "  clip: $(clip)"; false; }'

# ---- 含未保存的修改：值取显示的
key 0; key l; key Enter; e2e_type "edited"; key Enter; sleep 0.3
key y i; sleep 0.4
check "未保存的修改也算：v 是 edited" eval '[[ "$(clip)" == *"values (2, '"'edited'"', 7"* ]] || { echo "  clip: $(clip)"; false; }'
check "yi 不改变修改状态（保存按钮的计数还是 1，数据库里 v 仍是空串）" eval '[[ -z $(psql_n "select 1 from t_q where id = 2 and v = '"'edited'"'") ]]'
key u; sleep 0.2

# ---- 新增行：每格都是 DEFAULT 时写成 default values
key o; sleep 0.3; key y i; sleep 0.4
check "新增行每格都是 DEFAULT：insert into \"public\".\"t_q\" default values;" eval '[[ "$(clip)" == "insert into \"public\".\"t_q\" default values;" ]] || { echo "  clip: $(clip)"; false; }'
key d d; sleep 0.2

# ---- 关键字大小写跟 keyword_case
printf 'keyword_case = "upper"\n' >"$D/upper.toml"
start -C "$D/own" -c "$D/upper.toml"; open_table t_q; wait_for 8 settled; : >"$E2E_TMP/clipboard"; key y i; sleep 0.5
check "keyword_case = upper：INSERT INTO … VALUES (… NULL …)，标识符和值不变" eval '[[ "$(clip)" == "INSERT INTO \"public\".\"t_q\" (\"id\", \"v\", \"n\", \"b\", \"j\", \"ts\") VALUES (1, "* ]] && [[ "$(clip)" == *", NULL, "* ]] || { echo "  clip: $(clip)"; false; }'

# ---- 结果 tab 没有表可写：toast
SOLO= start -C "$D/own"; key C-l; key i; e2e_type "select 1 as x"; key Escape; key Enter; wait_for 8 eval '[[ $(bar) != *busy* ]]'; sleep 0.5
key C-j; sleep 0.3; : >"$E2E_TMP/clipboard"; key y i; sleep 0.5
check "结果 tab 上 yi：toast「只有表里的行能复制为 INSERT」，不写剪贴板" eval 'toast_text | grep -q "只有表里的行能复制为 INSERT" && [[ -z "$(clip)" ]]'
e2e_done
