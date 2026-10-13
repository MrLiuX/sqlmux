#!/usr/bin/env bash
# F15.8b snippet 什么时候结束（docs 79fd4e1）（specs/m15-pg-databases/task.md F15.8；tech-design §11「snippet 与 Select 模式」、§7.8「模板」）
# INSERT / SELECT 下光标出了所有 tabstop，snippet 就结束（o 开新行、A 到行尾以后 Tab 是插入空白）；NORMAL 下移动不结束（再进 INSERT 按 Tab 照跳）；
# BS 删空一个 tabstop 后接着打字、tabstop 里按 ↵ 变成多行，snippet 都还在（Tab 照跳到下一个）。
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
sc() { start -C "$D/own"; sleep 1; open_tmpl "加索引…"; }

sc
key Escape; sleep 0.3; key o; sleep 0.4
check "esc 回 NORMAL 后 o 开新行：INSERT，snippet 结束（visual 标记没了）" eval 'mode_is INSERT && [[ -z $(marks) ]]' || { marks; bar; }
key Tab; e2e_type x; sleep 0.4
check "Tab 在新行里插入空白（不跳占位符）：模式还是 INSERT，没有选中，<column> 没动" eval 'mode_is INSERT && [[ -z $(marks) ]] && [[ $(text) == *"(<column>);" ]]' || { marks; bar; text; }
_e2e_kill

sc
key Escape; sleep 0.3; key A; sleep 0.4; key Tab; e2e_type y; sleep 0.4
check "esc 后 A 到行尾：Tab 是插入空白、不跳去 <column>（模式 INSERT，没有选中）" eval 'mode_is INSERT && ! mode_is SELECT && [[ $(text) == "create index <name> on \"public\".\"cust\" (<column>);"* ]]' || { marks; bar; text; }
_e2e_kill

sc
key Escape; sleep 0.3; key w; key b; key i; sleep 0.3; key Tab; sleep 0.4
check "NORMAL 下移动不结束 snippet：w 出去、b 回到 <name> 里再 i，Tab 照跳到 <column>（SELECT）" eval 'mode_is SELECT' || { marks; bar; text; }
_e2e_kill

sc
e2e_type 12; sleep 0.3; key BSpace; key BSpace; sleep 0.3
e2e_type 3; sleep 0.4
check "BS 把第一个 tabstop 删空再打字：3 就在原位（create index 3 on …），<column> 还是 visual 底" eval '[[ $(text) == "create index 3 on \"public\".\"cust\" (<column>);" ]] && [[ $(marks) == "3<column>" || $(marks) == "<column>" ]]' || { marks; text; bar; }
key Tab; sleep 0.4
check "接着 Tab：跳到 <column>（SELECT），snippet 没有被删空的 tabstop 弄丢" eval 'mode_is SELECT' || { marks; bar; text; }
_e2e_kill

sc
e2e_type 1; key Enter; e2e_type 2; sleep 0.4
key Tab; sleep 0.4
check "tabstop 里按 ↵（变成两行）：snippet 还在，Tab 跳到 <column>（SELECT）" eval 'mode_is SELECT' || { marks; bar; e2e_plain | sed -n 3,10p | cut -c30-120; }
e2e_type id; sleep 0.4
check "在 <column> 打 id 替换它：<column> 没了，1↵2 还在" eval '! e2e_plain | grep -qF "<column>" && e2e_plain | grep -qF "(id);"' || e2e_plain | sed -n 3,10p | cut -c30-120
e2e_done
