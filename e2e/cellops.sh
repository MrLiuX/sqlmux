# Shared by f12.3.sh (PG) and f12.3b.sh (MySQL): F12.3 filter by cell value, sort by header, the grid's right-click menu on t_f(id, name, qty, tag):
# 1 ann 30 x | 2 bob 10 NULL | 3 o'neil 20 x | 4 ann 40 y | 5 cat 10 NULL, in the script's own database ($D/own/connections.toml, doraemon).
# QUOTE is the identifier quote of the engine (" or `); needs lib.sh / palette.sh / paging.sh / ordering.sh helpers.
rclick() { _sgr 2 "$1" "$2" M; _sgr 2 "$1" "$2" m; }
sclick() { _sgr 4 "$1" "$2" M; _sgr 4 "$1" "$2" m; }
find_xy() { e2e_plain | python3 -c '
import sys, unicodedata
for y, line in enumerate(sys.stdin.read().split("\n"), 1):
    i = line.find(sys.argv[1])
    if i >= 0:
        x = sum(2 if unicodedata.east_asian_width(c) in "WF" else 1 for c in line[:i]) + 1
        print(x, y); break' "$1"; }
menu_all() { local t; for t in "$@"; do menu_has "$t" || return 1; done; }
menu_has() { find_xy "$1" | grep -q . || { echo "  menu lacks '$1'"; false; }; }
menu_click() { local xy; xy=$(find_xy "$1"); [[ -n $xy ]] || { echo "  no '$1' on screen"; return 1; }; e2e_click $((${xy% *} + 1)) "${xy#* }"; sleep 0.8; }
colx() { e2e_find "$1" "$(hy)" | tr ' ' '\n' | awk '$1 > 34 { print; exit }'; }   # the column of a header's text
cellxy() { echo "$(( $(colx "$1") + 1 )) $(( $(grid_y) + $2 ))"; }                  # NAME ROW: a cell of that column (1-based row)
where_is() { local got; got=$(where_in | sed 's/ *$//'); [[ $got == "$1" ]] || { echo "  WHERE [$got], want [$1]"; false; }; }
q() { echo "${QUOTE}$1${QUOTE}"; }
clear_where() { key /; clear_in; key Enter; wait_for 8 settled; sleep 0.4; }
ALL=1,2,3,4,5,

cell_body() {
  SOLO=1 start -C "$D/own"; open_table t_f; wait_for 8 settled
  check "（准备）t_f 5 行，id 升序" eval 'ids_are "$ALL" && [[ $(cnt) == 5 ]]'
  # ---- * 按这个值过滤
  key l; key "*"; wait_for 8 settled; sleep 0.5
  check "* 在 name 格（ann）：WHERE $(q name) = 'ann'，两行（id 1、4）" eval 'where_is "$(q name) = '"'ann'"'" && ids_are "1,4," && [[ $(cnt) == 2 ]]'
  key l; key "*"; wait_for 8 settled; sleep 0.5
  check "再 * 在 qty 格（30，数字不加引号）：已有条件时 AND 接上，只剩 id 1" eval 'where_is "$(q name) = '"'ann'"' AND $(q qty) = 30" && ids_are "1,"'
  key "*"; wait_for 8 settled; sleep 0.5
  check "同一格再 *：相同条件不重复，WHERE 不变" eval 'where_is "$(q name) = '"'ann'"' AND $(q qty) = 30"'
  key C-r; sleep 0.5
  check "进了 WHERE 历史：C-r 的列表里有这条条件" eval 'e2e_plain | grep -qF "AND $(q qty) = 30"'
  key Escape; key Escape; key Escape; sleep 0.3
  clear_where
  key j; key l; key "*"; wait_for 8 settled; sleep 0.5      # row 2, tag NULL
  check "* 在 NULL 格（id 2 的 tag）：WHERE $(q tag) IS NULL，两行（id 2、5）" eval 'where_is "$(q tag) IS NULL" && ids_are "2,5,"'
  clear_where
  key g g; key j; key j; key h; key h; key "*"; wait_for 8 settled; sleep 0.5   # row 3, name o'neil
  check "* 在带单引号的值（o'neil）：引号翻倍 'o''neil'，只剩 id 3" eval 'where_is "$(q name) = '"'o''neil'"'" && ids_are "3,"'
  clear_where

  # ---- 表头点击：只按这一列 → 翻转 → 默认
  x=$(colx qty); e2e_click $((x + 1)) "$(hy)"; wait_for 8 settled; sleep 0.5
  check "点 qty 表头：ORDER qty ↑，第一行是最小值（10：id 2、5 先，再 20、30、40）" eval 'qb_has "ORDER qty $ASC" && ids_are "2,5,3,1,4,"'
  e2e_click $((x + 1)) "$(hy)"; wait_for 8 settled; sleep 0.5
  check "再点：翻转，ORDER qty ↓，最大值（40：id 4）在前" eval 'qb_has "ORDER qty $DESC" && ids_are "4,1,3,2,5,"'
  e2e_click $((x + 1)) "$(hy)"; wait_for 8 settled; sleep 0.5
  check "第三次点：回到默认排序 ORDER id ↑" eval 'qb_has "ORDER id $ASC" && ids_are "$ALL"'
  # ---- S-click：同下拉框的 space（追加 → 翻转 → 去掉）
  sclick $((x + 1)) "$(hy)"; wait_for 8 settled; sleep 0.5
  check "S-click qty 表头：追加（ORDER qty ↑）" eval 'qb_has "ORDER qty $ASC" && ids_are "2,5,3,1,4,"'
  sclick $((x + 1)) "$(hy)"; wait_for 8 settled; sleep 0.5
  check "再 S-click：翻转（ORDER qty ↓）" eval 'qb_has "ORDER qty $DESC"'
  sclick $((x + 1)) "$(hy)"; wait_for 8 settled; sleep 0.5
  check "第三次 S-click：去掉，回到默认" eval 'qb_has "ORDER id $ASC" && ids_are "$ALL"'
  # ---- gs / gS
  key g g; key 0; key l; key l; key g s; wait_for 8 settled; sleep 0.5
  check "gs（光标在 qty 列）：只按这一列，ORDER qty ↑" eval 'qb_has "ORDER qty $ASC" && ids_are "2,5,3,1,4,"'
  key h; key g S; wait_for 8 settled; sleep 0.5
  check "h 到 name 列，gS：加入排序，ORDER qty ↑, name ↑（qty 同为 10 的按 name：bob 2、cat 5）" eval 'qb_has "ORDER qty ↑, name ↑" && ids_are "2,5,3,1,4,"'
  key g o; sleep 0.5; dd_goto 默认; key Enter; wait_for 8 settled; sleep 0.5
  check "（还原）下拉里选「默认」" eval 'qb_has "ORDER id $ASC"'
  # ---- 转置下表头不响应
  key T; sleep 0.5
  e2e_click $((x + 1)) "$(hy)"; sleep 0.8
  check "转置视图：点表头（记录号）不动排序" eval 'qb_has "ORDER id $ASC"'
  key T; sleep 0.5

  # ---- 右键菜单：格
  read -r cx cy <<<"$(cellxy qty 3)"
  rclick "$cx" "$cy"; sleep 0.6
  check "格上右键：光标先移到那一格（3,3），菜单有 9 项" eval 'pos_is 3,3 && menu_all "在值编辑器中打开" "按这个值过滤" "按这一列排序 ↑" "按这一列排序 ↓" "加入排序" "隐藏这一列" "复制这一格" "复制行为 JSON" "导出…"'
  menu_click "复制这一格"
  check "菜单「复制这一格」：剪贴板里是 20" eval '[[ $(clip) == 20 ]] || { echo "  clipboard [$(clip)]"; false; }'
  rclick "$cx" "$cy"; sleep 0.6; menu_click "复制行为 JSON"
  check "菜单「复制行为 JSON」：剪贴板里是这一行的 JSON 对象" eval 'python3 - "$(clip)" <<PY
import json, sys
sys.exit(0 if json.loads(sys.argv[1]) == {"id": 3, "name": "o'"'"'neil", "qty": 20, "tag": "x"} else 1)
PY' || echo "  clipboard [$(clip)]"
  rclick "$cx" "$cy"; sleep 0.6; menu_click "按这个值过滤"; wait_for 8 settled
  check "菜单「按这个值过滤」：WHERE $(q qty) = 20，只剩 id 3" eval 'where_is "$(q qty) = 20" && ids_are "3,"'
  clear_where
  rclick "$cx" "$cy"; sleep 0.6; menu_click "按这一列排序 ↓"; wait_for 8 settled; sleep 0.4
  check "菜单「按这一列排序 ↓」：ORDER qty ↓，id 4 在前" eval 'qb_has "ORDER qty $DESC" && ids_are "4,1,3,2,5,"'
  rclick "$cx" "$cy"; sleep 0.6; menu_click "加入排序"; wait_for 8 settled; sleep 0.4
  check "菜单「加入排序」：qty 已在排序里（↓），再循环一次就是去掉，回到默认 ORDER id ↑" eval 'qb_has "ORDER id $ASC" && ids_are "$ALL"'
  # ---- 表头上右键
  rclick $((x + 1)) "$(hy)"; sleep 0.6
  check "表头上右键：菜单 4 项（排序 ↑ ↓、加入排序、隐藏这一列），没有「复制」" eval 'menu_all "按这一列排序 ↑" "按这一列排序 ↓" "加入排序" "隐藏这一列" && ! menu_has "复制这一格" >/dev/null 2>&1'
  menu_click "隐藏这一列"
  check "菜单「隐藏这一列」：表头少了 qty 列，COLS 3/4" eval '[[ $(e2e_text 35 159 "$(hy)") != *" qty "* ]] && qb_has "COLS 3/4"'
  _e2e_kill

  # ---- 结果 tab 的格：只有 值编辑器 / 复制 / 导出
  start -C "$D/own"; key C-l
  setsql "select * from t_f order by id"; run
  key C-j; sleep 0.5
  g=($(geom 3)); rclick $((g[0] + 8)) $((g[1] + 4)); sleep 0.6
  check "结果 tab 格上右键：菜单只有「在值编辑器中打开」「复制这一格」「导出…」，没有过滤 / 排序 / 隐藏" eval 'menu_has "在值编辑器中打开" && menu_has "复制这一格" && menu_has "导出…" && ! menu_has "按这个值过滤" >/dev/null 2>&1 && ! menu_has "隐藏这一列" >/dev/null 2>&1'
  menu_click "复制这一格"
  check "结果 tab：复制这一格，剪贴板里有值" eval '[[ -n $(clip) ]]'
}
