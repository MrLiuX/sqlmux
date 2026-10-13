#!/usr/bin/env bash
# F15.8 snippet 式的占位符和 Select 模式（specs/m15-pg-databases/task.md F15.8；tech-design §11「snippet 与 Select 模式」、§7.8「模板」）
# 右键「加索引…」开出 create index <name> on … (<column>);：<name> 在 SELECT 模式下选中（模式块 SELECT）、打字即替换并进 INSERT，Tab / S-Tab 跳下一个 / 上一个（跳到的又是 SELECT），
# 剩下的占位符 visual 底；最后一个上 Tab 结束 snippet；补全列表开着时 Tab 归补全；SELECT 里 esc 回 NORMAL、BS / Del 删掉选区（nvim：进 NORMAL）、粘贴替换选区进 INSERT；
# vim = false 时同样选中、打字替换、Tab 跳。自建库 cust：最后真的执行模板，索引建出来。
. "$(dirname "$0")/lib.sh"
SOLO=1
e2e_build || exit 1
. "$(dirname "$0")/palette.sh"

D=$(mktemp -d "${TMPDIR:-/tmp}/sqlmux-e2e-cfg.XXXXXX"); trap 'e2e_stop; rm -rf "$D"' EXIT
e2e_own_db || { echo "e2e: could not create the private database" >&2; exit 1; }
psql "$E2E_DB" -q -c "create table cust (id int primary key, nm text)" >/dev/null
mkdir -p "$D/own" "$D/novim"
printf '[[connection]]\nname = "doraemon"\nengine = "postgres"\ndsn = "%s"\n' "$E2E_DB" >"$D/own/connections.toml"; chmod 600 "$D/own/connections.toml"
cp "$D/own/connections.toml" "$D/novim/connections.toml"; printf 'vim = false\n' >"$D/novim/config.toml"
psql_n() { psql "$E2E_DB" -At -c "$1"; }
rclick() { _sgr 2 "$1" "$2" M; _sgr 2 "$1" "$2" m; sleep 0.6; }
srow() { local y; for y in $(seq 3 43); do [[ $(e2e_text 1 32 "$y") == *" $1 "* ]] && { echo "$y"; return; }; done; }
pop() { e2e_panes | awk '$1 == "-" { print $2, $3, $4, $5 }' | tail -1; }
mclick() { local g y; g=($(pop)); for ((y = g[1] + 1; y < g[1] + g[3] - 1; y++)); do [[ $(e2e_text $((g[0] + 1)) $((g[0] + g[2] - 2)) $y) == *"$1"* ]] && { e2e_click $((g[0] + 3)) $y; sleep 1.2; return; }; done; echo "  no menu item $1"; return 1; }
newest() { local f; f=$(ls -t "$E2E_TMP/data/sqlmux/queries"/*.sql 2>/dev/null | head -1); [[ -n $f ]] && cat "$f"; }
paste() { t set-buffer -b e2e "$1"; t paste-buffer -r -p -b e2e -t t; sleep 0.5; }
VISUAL_BG=#2d3f76
marks() { e2e_bgcells "$VISUAL_BG" | awk '{ printf "%s", $3 }'; }        # the characters on the visual background, in screen order: what is selected and the tabstops marked
text() { e2e_plain | grep -m1 -oE "(create index|alter table).*;" | sed 's/ *$//'; }   # the statement as it is on screen
tmpl() { sleep 1.2; text; }
open_tmpl() { rclick 6 "$(srow cust)"; mclick "$1"; sleep 1; }

start -C "$D/own"; sleep 1
open_tmpl "加索引…"
check "加索引…：模式块 SELECT，<name> 选中、<column> 也在 visual 底（marks = <name><column>）" eval '[[ $(tmpl) == "create index <name> on \"public\".\"cust\" (<column>);" ]] && mode_is SELECT && [[ $(marks) == "<name><column>" ]]' || { text; marks; echo; bar; }
e2e_type ix_nm; sleep 0.4
check "打字替换：ix_nm 换掉 <name>，进 INSERT，剩下的 <column> 还是 visual 底（没有选中的）" eval '[[ $(text) == "create index ix_nm on \"public\".\"cust\" (<column>);" ]] && mode_is INSERT && [[ $(marks) == "<column>" ]]' || { text; marks; echo; bar; }
key Tab; sleep 0.4
check "Tab：<column> 选中（模式块 SELECT），<name> 的位置（ix_nm）也标出来" eval 'mode_is SELECT && [[ $(marks) == "ix_nm<column>" ]]' || { marks; echo; bar; }
key BTab; sleep 0.4
check "S-Tab：回到 ix_nm，选中（SELECT）" eval 'mode_is SELECT && [[ $(marks) == "ix_nm<column>" ]]' || { marks; echo; bar; }
e2e_type ix_cust_nm; sleep 0.4
check "回到第一个再打字：整个 ix_nm 被替换（ix_cust_nm），INSERT" eval '[[ $(text) == "create index ix_cust_nm on \"public\".\"cust\" (<column>);" ]] && mode_is INSERT' || text
key Tab; sleep 0.3; e2e_type nm; sleep 0.4
check "Tab 再打 nm：<column> 换成 nm，语句是 create index ix_cust_nm on \"public\".\"cust\" (nm);，没有占位符了" eval '[[ $(text) == "create index ix_cust_nm on \"public\".\"cust\" (nm);" ]]' || text
key Tab; sleep 0.4
check "最后一个上按 Tab：snippet 结束（visual 标记都没了），INSERT，光标在模板文字的末尾（nvim 的 \$0）" eval 'mode_is INSERT && [[ -z $(marks) ]]' || { marks; bar; }
e2e_type Z; sleep 0.3
check "再打的字接在模板末尾（; 后面），不是替换什么：statement 末尾是 ;Z" eval 'e2e_plain | grep -qF "(nm);Z"' || text
key BSpace; key Escape; sleep 0.5
check "（还原）删掉 Z、esc：模板还是完整的一条" eval '[[ $(text) == "create index ix_cust_nm on \"public\".\"cust\" (nm);" ]]'
key Enter; sleep 1.5
check "执行（NORMAL 下 ↵）：索引 ix_cust_nm 真的建在 cust(nm) 上，语句正确" eval '[[ $(psql_n "select indexdef from pg_indexes where indexname = '"'"'ix_cust_nm'"'"'") == *"(nm)"* ]]' || { psql_n "select indexname, indexdef from pg_indexes where tablename = 'cust'"; e2e_plain | tail -6 | cut -c1-150; }
_e2e_kill

# ---- esc / BS / Del / 粘贴：SELECT 里的键
start -C "$D/own"; sleep 1
open_tmpl "加列…"
key Escape; sleep 0.4
check "SELECT 里 esc：回 NORMAL，文字不变（<name> <type> 都在）" eval 'mode_is NORMAL && [[ $(text) == "alter table \"public\".\"cust\" add column <name> <type>;" ]]' || { text; bar; }
_e2e_kill
start -C "$D/own"; sleep 1
open_tmpl "加列…"
key BSpace; sleep 0.4
check "SELECT 里 BS：删掉选区（<name> 没了），进 NORMAL（nvim 的 v_<BS> 是 d），<type> 还在" eval 'mode_is NORMAL && [[ $(text) == "alter table \"public\".\"cust\" add column  <type>;" ]]' || { text; bar; }
_e2e_kill
start -C "$D/own"; sleep 1
open_tmpl "加列…"
key DC; sleep 0.4
check "SELECT 里 Del：同 BS，删掉选区进 NORMAL" eval 'mode_is NORMAL && [[ $(text) == "alter table \"public\".\"cust\" add column  <type>;" ]]' || { text; bar; }
_e2e_kill
start -C "$D/own"; sleep 1
open_tmpl "加列…"
paste "age"
check "SELECT 里粘贴：粘贴的文字替换选区，进 INSERT" eval 'mode_is INSERT && [[ $(text) == "alter table \"public\".\"cust\" add column age <type>;" ]]' || { text; bar; }
key Tab; sleep 0.3; e2e_type int; sleep 0.4
check "接着 Tab 到 <type> 打 int：alter table … add column age int;，没有占位符了（打 int 弹出了补全，所以不再按 Tab）" eval '[[ $(text) == "alter table \"public\".\"cust\" add column age int;" ]]' || { text; marks; }
_e2e_kill

# ---- 补全列表开着时 Tab 归补全：不跳占位符
start -C "$D/own"; sleep 1
open_tmpl "加索引…"
e2e_type ix_a; key Tab; sleep 0.3; e2e_type n; sleep 0.8
check "（准备）在 <column> 里打 n：补全列表开着，snippet 还在（ix_a 标着）" eval '[[ $(e2e_plain | grep -c " 关键字\| 列") -ge 1 ]] && [[ $(marks) == "ix_a" ]]' || { marks; e2e_plain | sed -n 3,12p | cut -c30-120; }
key Tab; sleep 0.5
check "补全开着时 Tab：归补全（移选中项），没有跳到末尾结束 snippet（ix_a 的标记还在，模式 INSERT）" eval 'mode_is INSERT && [[ $(marks) == "ix_a" ]]' || { marks; bar; }
_e2e_kill

# ---- vim = false：同样选中、打字替换、Tab 跳
start -C "$D/novim"; sleep 1
open_tmpl "加索引…"
e2e_type ix_p; sleep 0.4
check "vim = false：打字替换第一个占位符（ix_p），<column> 还在" eval '[[ $(text) == "create index ix_p on \"public\".\"cust\" (<column>);" ]]' || text
key Tab; sleep 0.3; e2e_type id; sleep 0.4
check "vim = false：Tab 跳到 <column>，选中，打 id 替换" eval '[[ $(text) == "create index ix_p on \"public\".\"cust\" (id);" ]]' || text
key Tab; sleep 0.3; e2e_type Z; sleep 0.3
check "vim = false：最后的 Tab 结束 snippet，再打字接在末尾（;Z）" eval 'e2e_plain | grep -qF "(id);Z"' || text
e2e_done
