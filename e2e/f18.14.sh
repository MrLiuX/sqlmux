#!/usr/bin/env bash
# F18.14 树上删连接：dd（tree.delete）和右键菜单（specs/m18-windows-mssql/task.md F18.14；tech-design §7.8「右键菜单」「连接管理」、§6.4 tree）
# 连接节点上 dd 弹 F18.12 的确认框（没 tab 是「删除连接 lite？」，有 tab 写几个），n 什么都不动，y 后树里和文件里都没了；
# 右键菜单：PG「选择库… / 编辑… / 删除…」，MySQL「选择 schema… / 编辑… / 删除…」，SQLite 和 Redis（连不上也一样）「编辑… / 删除…」；
# 「编辑…」打开连接管理并选中这个连接，「删除…」等于 dd。
. "$(dirname "$0")/lib.sh"
e2e_build || exit 1
. "$(dirname "$0")/palette.sh"

D=$(mktemp -d "${TMPDIR:-/tmp}/sqlmux-e2e-cfg.XXXXXX"); trap 'chmod -R u+w "$D" 2>/dev/null; e2e_stop; rm -rf "$D"' EXIT
python3 -c 'import sqlite3, sys; c = sqlite3.connect(sys.argv[1]); c.execute("create table t1 (id integer primary key)"); c.commit()' "$D/lite.db"
mkdir -p "$D/c"
(umask 077; printf '[[connection]]\nname = "play"\nengine = "postgres"\ndsn = "%s"\n\n[[connection]]\nname = "my"\nengine = "mysql"\ndsn = "%s"\n\n[[connection]]\nname = "lite"\nengine = "sqlite"\ndsn = "%s"\n\n[[connection]]\nname = "rd"\nengine = "redis"\ndsn = "redis://localhost:1/0"\n' "$SQLMUX_TEST_PG" "$SQLMUX_TEST_MYSQL" "$D/lite.db" >"$D/c/connections.toml")
ct() { echo "$E2E_TMP/config/sqlmux/connections.toml"; }
rclick() { _sgr 2 "$1" "$2" M; _sgr 2 "$1" "$2" m; sleep 0.6; }
side() { e2e_plain | python3 -c 'import sys; [print(l[1:33].rstrip("│ ")) for l in sys.stdin.read().split("\n")]'; }
srow() { local y t; for y in $(seq 3 43); do t=$(e2e_text 1 32 "$y" | noicon | tr -s ' ' | sed 's/[ │]*$//'); [[ $t == *" $1" ]] && { echo "$y"; return; }; done; }   # 侧栏里以 NAME 结尾的行（连接节点）
has_node() { side | noicon | tr -s " " | grep -qE "^ [▾▸] $1$"; }
pop() { e2e_panes | awk '$1 == "-" { print $2, $3, $4, $5 }' | tail -1; }
mitems() { local g y; g=($(pop)); [[ -n ${g[0]} ]] || return 0; for ((y = g[1] + 1; y < g[1] + g[3] - 1; y++)); do e2e_text $((g[0] + 1)) $((g[0] + g[2] - 2)) $y | sed 's/^ *//; s/^[^ ]* //; s/ *$//'; done | tr '\n' '|'; }
menu_is() { [[ $(mitems) == "$1" ]] || { echo "  menu: [$(mitems)], want [$1]"; false; }; }
tsel() { local y; for y in $(seq 3 43); do [[ $(e2e_style 4 "$y") == *bg=#364a82* ]] && { e2e_text 1 32 "$y" | noicon | tr -s ' ' | sed 's/^[│ ]*[▾▸]* *//; s/[ │]*$//'; return; }; done; }
tree_focus() { local i; for ((i = 0; i < 6; i++)); do [[ $(focused) == 0 ]] && return 0; key C-h; done; }
tree_to() { local i; tree_focus; key gg; for ((i = 0; i < 60; i++)); do [[ $(tsel) == "$1" ]] && return 0; key j; done; return 1; }
dialog() { e2e_plain | grep -o "删除连接 [^ ]*？[^│]*" | head -1 | sed 's/ *$//'; }
mgr_open() { e2e_plain | grep -q "┌─ 连接"; }
box() { e2e_panes | awk '$1 == "-" { print $2, $3, $4, $5; exit }'; }
lsel() { local g y; g=($(box)); for ((y = g[1] + 1; y < g[1] + g[3] - 1; y++)); do style_has $((g[0] + 20)) "$y" bg=#364a82 >/dev/null && { e2e_text $((g[0] + 2)) $((g[0] + 26)) "$y" | noicon | sed 's/^ *//; s/ *$//'; return; }; done; }

SOLO= start -C "$D/c" -S "$D/st"; sleep 2
check "（准备）树里有四个连接节点 play、my、lite、rd" eval 'has_node play && has_node my && has_node lite && has_node rd || { side | noicon | head -30; false; }'

# ---- 右键菜单
rclick 5 "$(srow play)"
check "右键 PG 连接 play：选择库… / 编辑… / 删除…（编辑和删除排在选择库后面）" menu_is "选择库…|编辑…|删除…|"
key Escape
rclick 5 "$(srow my)"
check "右键 MySQL 连接 my：选择 schema… / 编辑… / 删除…" menu_is "选择 schema…|编辑…|删除…|"
key Escape
rclick 5 "$(srow lite)"
check "右键 SQLite 连接 lite：编辑… / 删除…" menu_is "编辑…|删除…|"
key Escape
rclick 5 "$(srow rd)"
check "右键 Redis 连接 rd（连不上的）：编辑… / 删除…" menu_is "编辑…|删除…|"
check "（对照）菜单里「编辑…」「删除…」没有键位提示" eval '! mitems | grep -qE "dd|^D"'
key Enter; sleep 1
check "「编辑…」：打开连接管理，选中 rd（右边表单的 name 是 rd）" eval 'mgr_open && [[ $(lsel) == rd ]] && e2e_plain | grep -qE "name +rd" || { echo "  mgr $(mgr_open && echo open) sel [$(mgr_open && lsel)]"; e2e_plain | head -30 | cut -c1-120; false; }'
key Escape; sleep 0.4
check "esc 关掉连接管理" eval '! mgr_open'

# ---- dd：没 tab 的 lite
cp "$(ct)" "$D/orig.toml"
tree_to lite; sleep 0.2
check "（准备）光标在 lite 节点上" eval '[[ $(tsel) == lite ]] || { echo "  tsel [$(tsel)]"; false; }'
key d; key d; sleep 0.5
check "dd：确认框「删除连接 lite？」，没有 tab 的话不带「它开着的…」" eval '[[ $(dialog) == "删除连接 lite？" ]] || { echo "  dialog [$(dialog)]"; false; }'
key n; sleep 0.4
check "n：什么都不动——文件没变、树里还有 lite" eval 'cmp -s "$(ct)" "$D/orig.toml" && has_node lite && [[ -z $(dialog) ]]'
key d; key d; sleep 0.5; key y; sleep 1.2
check "y：树里没有 lite 了，connections.toml 里也没有，别的连接原样" eval '! has_node lite && ! grep -q "lite" "$(ct)" && grep -q "name = \"play\"" "$(ct)" && grep -q "name = \"rd\"" "$(ct)" && has_node play || { side | noicon | head -6; false; }'

# ---- 右键菜单的「删除…」等于 dd；有 tab 的连接：确认框写几个 tab
rclick 5 "$(srow rd)"; key j; key Enter; sleep 0.6
check "右键「删除…」：同一个确认框「删除连接 rd？」" eval '[[ $(dialog) == "删除连接 rd？" ]] || { echo "  dialog [$(dialog)]"; false; }'
key y; sleep 1.2
check "y：rd 删掉了（树里和文件里）" eval '! has_node rd && ! grep -q "\"rd\"" "$(ct)"'
open_table t_user; wait_for 8 settled
tree_to play; key d; key d; sleep 0.5
check "dd 在开着 t_user 和 query_1 的 play 上：确认框「删除连接 play？它开着的 2 个 tab 会一起关掉」" eval '[[ $(dialog) == "删除连接 play？它开着的 2 个 tab 会一起关掉" ]] || { echo "  dialog [$(dialog)]"; false; }'
key y; sleep 1.5
check "y：play 删掉了，它的 tab 都关了（t_user 不在了），my 还在" eval '! has_node play && ! e2e_plain | grep -q "t_user" && has_node my && ! grep -q "name = \"play\"" "$(ct)"'
check "程序没崩" running
e2e_done
