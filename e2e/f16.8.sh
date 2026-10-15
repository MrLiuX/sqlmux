#!/usr/bin/env bash
# F16.8 JSON 视图的 s 跳转、F16.9 JSON 视图的 Y 复制整个 JSON（specs/m16-redis/task.md F16.8 / F16.9；tech-design §6.10、§6.4 jsonview、§7.6「JSON 视图」）
# JSON 视图里 s = jump.start：可跳区域多了视图的文字，标签贴在匹配后面一个字符上，按标签后终端光标（编辑器光标）落在匹配的第一个字符上；
# esc 退出 JUMP 光标不动、仍在 JSON 视图；↵ 跳到离光标最近的；视图的文字和树的节点互跳（焦点跟着走）；没有匹配的字符直接退出；chip 行、标题不算。
# Y = json.yank：整个视图的 JSON 文本（当前页，不截断）到系统剪贴板（假 pbcopy），不是 vim 的 Y（一行）；整段闪 yank 色；命令面板里有。
# 用 SQLite 文件（t_user 3 行，t_order 6000 行）：不占 PG / MySQL。
. "$(dirname "$0")/lib.sh"
e2e_build || exit 1
. "$(dirname "$0")/palette.sh"
. "$(dirname "$0")/paging.sh"

D=$(mktemp -d "${TMPDIR:-/tmp}/sqlmux-e2e-cfg.XXXXXX"); trap 'e2e_stop; rm -rf "$D"' EXIT
DB=$D/shop.db; sqlite3 "$DB" <"$E2E_ROOT/testdata/seed/sqlite.sql"
WARN=#e0af68
cur() { t display -p -t t '#{cursor_x},#{cursor_y}'; }                                  # the terminal's cursor, 0-based: the editor's in a JSON view
labels() { e2e_bgcells "$WARN" | awk '{ print $3 }' | tr '\n' ' '; }
nlabels() { e2e_bgcells "$WARN" | wc -l | tr -d ' '; }
label_xy() { e2e_bgcells "$WARN" | awk -v c="$1" '$3 == c { print $1, $2; exit }'; }   # 1-based, the cell after the match
in_json() { e2e_plain | sed -n 3p | grep -qF "JSON"; }
jump_is() { mode_is JUMP; }
REC=$D/rec; rec() { : >"$REC"; e2e_record "$REC"; }; unrec() { t pipe-pane -t t; }
flashed() { local n; for n in $(seq 20); do LC_ALL=C grep -q '48;2;224;175;104' "$REC" 2>/dev/null && return 0; sleep 0.05; done; false; }   # yank 底色在录下的输出里
# jump_to PATTERN N: s, the pattern, the Nth label (1 = the nearest); the cursor must then sit on the first character of the match the label was on
jump_to() { local xy l; key s; e2e_type "$1"; sleep 0.4; l=$(labels | awk -v n="$2" '{ print $n }'); xy=($(label_xy "$l")); [[ -n ${xy[0]} ]] || { echo "  no label $2"; return 1; }
  e2e_type "$l"; sleep 0.5; WANT="$((xy[0] - ${#1} - 1)),$((xy[1] - 1))"; [[ $(cur) == "$WANT" ]] || { echo "  cursor $(cur), want $WANT"; false; }; }

e2e_start "$E2E_BIN $DB"; wait_for 5 flag_is alternate_on 1; sleep 1; solo
open_table t_user; wait_for 8 settled
key g j; sleep 1
check "（准备）JSON 视图开着，编辑器光标在第 1 行行首（终端光标 39,3），状态栏格子光标 1,1" eval 'in_json && [[ $(cur) == 39,3 ]] && pos_is 1,1'

# ---- F16.8 s
key s
check "s：模式块 JUMP，还没有标签，仍在 JSON 视图" eval 'jump_is && [[ $(nlabels) == 0 ]] && in_json'
e2e_type name; sleep 0.5
check "打 name：三个对象的 \"name\"（第 4 / 11 / 18 行）各一个标签，a s d" eval '[[ $(nlabels) == 3 && "$(labels)" == "a s d " ]]' || labels
xy=($(label_xy a))
check "标签 a 在最近的匹配（第 4 行，屏幕第 7 行）上，贴在 name 后面一个字符（第 49 列，原来是引号）" eval '[[ ${xy[0]} == 49 && ${xy[1]} == 7 ]]' || echo "  a at ${xy[*]}"
check "标签不是 name 后面紧跟的字符（引号）：屏幕上 name 后面那格是标签字母" eval '[[ $(e2e_text 49 49 7) == a ]]'
e2e_type s; sleep 0.5
check "按 s：退出 JUMP 回 NORMAL，标签没了，光标落在第 11 行的 name 的 n 上（终端光标 44,13），仍在 JSON 视图" eval 'mode_is NORMAL && [[ $(nlabels) == 0 && $(cur) == 44,13 ]] && in_json' || { cur; bar; }
check "格子光标没动（光标行不同步）：1,1" pos_is 1,1
key j
check "跳过去之后 vim 的 j 照常：下一行（44,14 → 光标在 y 14，x 取不超过行长）" eval '[[ $(cur) == *,14 ]]' || cur
key g g; key 0
key s; e2e_type name; key Escape; sleep 0.3
check "esc 退出 JUMP：NORMAL、光标不动（39,3）、标签没了，还在 JSON 视图（esc 没有把表格叫回来）" eval 'mode_is NORMAL && [[ $(cur) == 39,3 && $(nlabels) == 0 ]] && in_json' || { cur; bar; }
key j; key j
key s; e2e_type name; key BSpace; key BSpace; key BSpace; key BSpace; sleep 0.3
check "backspace 删到空：仍在 JUMP，标签没了" eval 'jump_is && [[ $(nlabels) == 0 ]]'
e2e_type name; key Enter; sleep 0.4
check "↵：跳到离光标最近的匹配（光标在第 3 行，最近的是第 4 行的 name：44,6）" eval 'mode_is NORMAL && [[ $(cur) == 44,6 ]]' || cur
key g g; key 0
key s; e2e_type N; sleep 0.4
check "smartcase：含大写的 N 区分大小写（文字里只有小写的 name），没有匹配：直接回 NORMAL，光标不动，这个字符丢掉" eval 'mode_is NORMAL && [[ $(nlabels) == 0 && $(cur) == 39,3 ]]'
key s; e2e_type P; sleep 0.4
check "chip 行的 PAGE（有大写 P）不是可跳区域：直接回 NORMAL，光标不动" eval 'mode_is NORMAL && [[ $(nlabels) == 0 && $(cur) == 39,3 ]]'
key s; e2e_type alice; sleep 0.4
check "alice：第 4 行的 \"alice\" 和第 5 行的 alice@example.com 各一个标签（同一行取第一处）" eval '[[ $(nlabels) == 2 ]]' || labels
e2e_type s; sleep 0.5
check "按 s：光标在第 5 行 alice@ 的 a 上（目标是匹配起点，不是行首）" eval '[[ $(cur) == "$(( $(e2e_find "alice@" 8 | cut -d" " -f1) - 1 )),7" ]]' || { cur; e2e_find "alice@" 8; }
key g g
# ---- 带视图文字的另一个 pane / 树：互跳，焦点跟着走
key s; e2e_type t_aud; sleep 0.4
check "JSON 视图里 s、t_aud：树里 t_aud 节点出现标签" eval '[[ $(nlabels) -ge 1 ]]'
e2e_type a; sleep 0.5
check "按 a：焦点到树（⓪），回 NORMAL" eval 'mode_is NORMAL && focus_is 0'
key s; e2e_type bob; sleep 0.4
xy=($(label_xy a))
check "树里 s、bob：JSON 视图里的 bob 出现标签（在 ① 里）" eval '[[ -n ${xy[0]} ]] && (( ${xy[0]} > 34 ))'
e2e_type a; sleep 0.5
check "按 a：焦点回 ①，光标落在 bob 的 b 上（52,13）" eval 'focus_is 1 && [[ $(cur) == 52,13 ]] && in_json' || { cur; focused; }
# ---- 命令面板里执行 jump.start
pal ">jump"; key Enter; sleep 0.5
check "命令面板里执行 jump.start（跳转到屏幕上的字符）：进 JUMP" eval 'jump_is'
key Escape

# ---- 滚动过的视图：标签在可见行上，光标跟着屏幕位置对
key Escape; sleep 0.3
open_table t_order; wait_for 8 settled
key g j; sleep 1; wait_for 5 settled; key G; sleep 0.5
check "（准备）t_order 的 JSON 视图 G 到末行（滚过了）" eval 'in_json && [[ $(cur) == *,* ]]'
jump_to created_at 3; sleep 0.2
check "滚过的视图里 s + created_at + 第三个标签：光标正好在那个标签所在行、匹配的第一个字符上（上面 jump_to 里比过）；不是第一行的位置" eval '[[ $(cur) == "$WANT" ]] && ! [[ $(cur) == *,3 ]]'
key g g
jump_to '"id"' 1
check "gg 之后：光标行在可见的第 1 个对象的 id（\"id\" 离光标最近），jump_to 对上" eval '[[ $(cur) == "$WANT" ]]'

# ---- F16.9 Y
key Escape; sleep 0.3
open_table t_user; wait_for 8 settled
key Escape; sleep 0.3
key g j; sleep 1
key j; key j; key j; key l
rm -f "$E2E_TMP/clipboard"; rec y1
key Y; sleep 0.5; flashed; f=$?; unrec; Y1=$(clip)
SQL=$(sqlite3 -json "$DB" "select * from t_user order by id")
check "Y（光标在第 4 行中间）：剪贴板里是整个 JSON（不是那一行）：和库里 t_user 三行逐值相等（键按列顺序、值按类型：id 数字、email 有 null）" eval 'python3 -c "
import json, sys
a, b = json.loads(sys.argv[1]), json.loads(sys.argv[2])
assert a == b, (a, b)
assert [list(o) for o in a] == [list(o) for o in b]" "$Y1" "$SQL"' || echo "$Y1" | head -5
check "文本和视图里看到的一样：2 空格缩进、多行、第一行 [、最后一行 ]、没有结尾换行" eval '[[ $(echo "$Y1" | head -1) == "[" && "$(echo "$Y1" | tail -1)" == "]" && $(echo "$Y1" | sed -n 3p) == "    \"id\": 1," && $(printf %s "$Y1" | wc -l | tr -d " ") == 22 ]]'
check "整段闪一下 yank 色" eval '(( f == 0 ))'
check "还在 JSON 视图、光标没动（Y 不改光标）" eval 'in_json && [[ $(cur) == 40,6 ]]' || cur
rm -f "$E2E_TMP/clipboard"; e2e_type '"+yy'; sleep 0.5
check "vim 的 \"+yy 还是复制一行（编辑器里的 y 只进寄存器，F3.38）：第 4 行 \"name\": \"alice\"，不是整段" eval '[[ "$(clip | sed "s/^ *//")" == "\"name\": \"alice\"," ]]' || clip
rm -f "$E2E_TMP/clipboard"; pal ">复制整个"; sleep 0.3
check "命令面板里有「复制整个 JSON」" eval 'row_has "复制整个 JSON"' || list
key Enter; sleep 0.5
check "面板里执行它：剪贴板里同样是整个 JSON" eval '[[ "$(clip)" == "$Y1" ]]'
# 第 2 页
key Escape; sleep 0.3
open_table t_order; wait_for 8 settled
key g j; sleep 1; wait_for 5 settled; key "]"; sleep 1; wait_for 5 settled
rm -f "$E2E_TMP/clipboard"; key Y; sleep 0.6
check "t_order 第 2 页的 JSON 视图 Y：整页 100 个对象，id 101..200（当前页，不是整张表，也没有截断）" eval 'python3 -c "
import json, sys
a = json.loads(sys.argv[1])
assert [o[\"id\"] for o in a] == list(range(101, 201)), [o[\"id\"] for o in a][:3]" "$(clip)"' || clip | head -3
# 表格里的 Y 还是记录
key Escape; sleep 0.5; rm -f "$E2E_TMP/clipboard"; key Y; sleep 0.5
check "回表格后 Y 还是复制那一条记录（一个 JSON 对象，以 { 开头，只有 id 101 那条），不是整页数组" eval '[[ "$(clip)" == "{"* && "$(clip)" != *"\"id\": 102"* && "$(clip)" == *"\"id\": 101"* ]]' || clip
# 键位
kk() { XDG_CONFIG_HOME=$D/$1 XDG_STATE_HOME=$D/st XDG_DATA_HOME=$D/st "$E2E_BIN" keys; }
mkdir -p "$D/k/sqlmux"; K=$(kk k)
check "sqlmux keys：jsonview 作用域有 s = jump.start、Y = json.yank" eval 'grep -E "\| jsonview \| .s. \| jump.start" <<<"$K" >/dev/null && grep -E "\| jsonview \| .Y. \| json.yank" <<<"$K" >/dev/null' || grep jsonview <<<"$K"
e2e_done
