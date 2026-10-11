# Shared by f12.1.sh (PG) and f12.1b.sh (MySQL): F12.1 multi-column ORDER on t_ord(id 1..12, source a/b/c = 1 + id % 3 → a: 3 6 9 12, b: 1 4 7 10, c: 2 5 8 11, n = id * 10)
# in the script's own database. Needs $D/own/connections.toml with a connection called doraemon, and lib.sh / palette.sh helpers.
save_layout() { e2e_keys C-p; sleep 0.3; e2e_type ">save layout"; sleep 0.2; for i in $(seq 20); do e2e_keys BSpace; done; e2e_type ">保存布局"; sleep 0.5; key Enter; sleep 0.4; e2e_type "$1"; key Enter; sleep 0.6; }
ids() { local y n=${1:-12} out=""; for ((y = $(grid_y) + 1; y <= $(grid_y) + n; y++)); do out+="$(e2e_text 35 159 "$y" | awk -F'│' '{ gsub(/ /, "", $2); print $2 }'),"; done; echo "$out"; }   # first N ids on screen
ids_are() { local got; got=$(ids "$(echo "$1" | tr -cd , | wc -c)"); [[ $got == "$1" ]] || { echo "  ids [$got], want [$1]"; false; }; }
# the dropdown (under the ORDER chip): items from row 7 on, "1↓ source" / "id" …; the cursor row has the select background
DD_Y=7 SELBG=#364a82
dd_items() { local y t; for ((y = DD_Y; y < DD_Y + 8; y++)); do t=$(e2e_text 36 56 "$y" | sed 's/[│┌┐└┘─┼]//g; s/^ *//; s/ *$//'); [[ $t =~ ^([0-9][↑↓]\ )?[A-Za-z默认_][^\ ]*$ ]] && echo "$t" || break; done; }
dd_has() { dd_items | grep -qxF "$1" || { echo "  dropdown: $(dd_items | tr '\n' '|')"; false; }; }
dd_cursor() { local y n=0; for ((y = DD_Y; y < DD_Y + 8; y++)); do [[ " $(e2e_style 40 "$y") " == *" bg=$SELBG "* ]] && { echo "$n"; return; }; n=$((n + 1)); done; echo -1; }
dd_goto() {   # NAME: move the cursor to this item (C-n / C-p); the dropdown opens with the cursor on the current sort column
  local want i=0 cur n=0 item; while read -r item; do [[ ${item##* } == "$1" ]] && want=$n; n=$((n + 1)); done < <(dd_items)
  cur=$(dd_cursor); [[ -n $want && $cur -ge 0 ]] || { echo "  dd_goto $1: want=$want cur=$cur"; return 1; }
  while ((cur < want)); do e2e_keys C-n; sleep 0.12; cur=$((cur + 1)); done
  while ((cur > want)); do e2e_keys C-p; sleep 0.12; cur=$((cur - 1)); done
  sleep 0.2
}
sp() { dd_goto "$1" && { key Space; sleep 0.7; }; }   # NAME: space on this item
go_open() { key g o; sleep 0.5; }
marks() { dd_items | grep -E '^[0-9][↑↓]' | tr '\n' '|'; }

order_body() {
  SOLO=1 start -C "$D/own"; open_table t_ord; wait_for 8 settled
  check "默认：chip 是 id ↑，按 id 升序" eval 'qb_has "ORDER id $ASC" && ids_are "1,2,3,4,5,6,7,8,9,10,11,12,"'

  # ---- go → source → space：追加（升序）；再 space 翻转
  go_open; sp source
  check "go，到 source，space：下拉里 source 前面是 1↑（只有它带标记），chip 是 source ↑（单列用方向图标）" eval '[[ $(marks) == "1↑ source|" ]] && qb_has "ORDER source $ASC" || { echo "  marks: $(marks)"; false; }'
  key Escape; sleep 0.5
  check "esc 后：按 source 升序、同 source 里 id 升序（a 3 6 9 12、b 1 4 7 10、c 2 5 8 11）" eval 'ids_are "3,6,9,12,1,4,7,10,2,5,8,11,"'
  go_open; sp source
  check "再 space：翻转，1↓ source，chip source ↓" eval '[[ $(marks) == "1↓ source|" ]] && qb_has "ORDER source $DESC"'
  key Escape; sleep 0.5
  check "esc 后：c 在前（2 5 8 11）、再 b、再 a" eval 'ids_are "2,5,8,11,1,4,7,10,3,6,9,12,"'

  # ---- 到 id，space：追加到末尾，chip 变成 source ↓, id ↑；再 space 翻转
  go_open; sp id
  check "到 id，space：id 前面是 2↑，source 还是 1↓，chip 是 source ↓, id ↑" eval '[[ $(marks) == "2↑ id|1↓ source|" ]] && qb_has "ORDER source ↓, id ↑"'
  sp id
  check "再 space：id 翻转成 2↓，chip source ↓, id ↓" eval '[[ $(marks) == "2↓ id|1↓ source|" ]] && qb_has "ORDER source ↓, id ↓"'
  key Escape; sleep 0.5
  check "esc 关下拉：排序留着，第一行是 source 最大（c）里 id 最大的记录（11）：11 8 5 2 | 10 7 4 1 | 12 9 6 3" eval 'qb_has "ORDER source ↓, id ↓" && ids_are "11,8,5,2,10,7,4,1,12,9,6,3," && ! e2e_plain | grep -q "2↓ id"'

  # ---- 第三次 space 去掉 id：只剩 source ↓，id 作 tiebreaker（升序）
  go_open; sp id
  check "第三次 space：id 从排序里去掉，只剩 1↓ source，chip source ↓" eval '[[ $(marks) == "1↓ source|" ]] && qb_has "ORDER source $DESC"'
  key Escape; sleep 0.5
  check "esc 后：同 source 里 id 升序（行标识列作 tiebreaker）：2 5 8 11 | 1 4 7 10 | 3 6 9 12" eval 'ids_are "2,5,8,11,1,4,7,10,3,6,9,12,"'

  # ---- 非行标识列 n：追加、翻转、去掉
  go_open; sp n; sp n
  check "n 上 space 两次：2↓ n、1↓ source，chip source ↓, n ↓" eval '[[ $(marks) == "1↓ source|2↓ n|" ]] || [[ $(marks) == "2↓ n|1↓ source|" ]]; qb_has "ORDER source ↓, n ↓"'
  sp n
  check "第三次：n 去掉，回到只有 source ↓" eval '[[ $(marks) == "1↓ source|" ]] && qb_has "ORDER source $DESC"'
  key Escape; sleep 0.4

  # ---- ↵ 在别的列上：清掉列表，只剩这一列（方向按 [grid] order，默认 asc）
  go_open; sp id; sp n
  check "（准备）两列：id ↑、n ↑ 在 source ↓ 之后" eval 'qb_has "ORDER source ↓, id ↑, n ↑"'
  dd_goto id; key Enter; sleep 0.8
  check "↵ 在 id 上（别的列，下拉关闭）：列表清成只有 id（方向按默认 asc），chip id ↑，按 id 升序" eval 'qb_has "ORDER id $ASC" && ids_are "1,2,3,4,5,6,7,8,9,10,11,12,"'
  go_open
  check "再开下拉：只有 id 带标记（1↑）" eval '[[ $(marks) == "1↑ id|" ]]'
  dd_goto source; key Enter; sleep 0.8
  check "↵ 在 source 上：只剩 source ↑（id 的标记没了），按 source 升序" eval 'qb_has "ORDER source $ASC" && ids_are "3,6,9,12,1,4,7,10,2,5,8,11,"'

  # ---- 唯一排序列上 ↵：翻转（原来的规则不变）
  go_open; dd_goto source; key Enter; sleep 0.8
  check "唯一排序列 source 上 ↵：翻转成 ↓" eval 'qb_has "ORDER source $DESC" && ids_are "2,5,8,11,1,4,7,10,3,6,9,12,"'

  # ---- 过滤框非空时 space 是字符
  go_open; e2e_type "so"; sleep 0.3; key Space; sleep 0.5
  check "过滤框里有字（so）时 space 是字符：过滤框是 'so '，排序没变（chip 还是 source ↓）" eval 'e2e_plain | grep -q "so " && qb_has "ORDER source $DESC"'
  key Escape; key Escape; sleep 0.4
  check "esc 两次关下拉，排序没变" eval 'qb_has "ORDER source $DESC" && ids_are "2,5,8,11,1,4,7,10,3,6,9,12,"'

  # ---- 回默认：下拉里选「默认」
  go_open; dd_goto 默认; key Enter; sleep 0.8
  check "选「默认」：chip 回到 id ↑，按 id 升序" eval 'qb_has "ORDER id $ASC" && ids_are "1,2,3,4,5,6,7,8,9,10,11,12,"'

  # ---- 布局文件：存成数组，--layout 读回；旧的 order = "x" + desc = true 读成一列
  go_open; sp source; sp source; sp id; key Escape; sleep 0.5    # source ↓, id ↑
  check "（准备）source ↓, id ↑" eval 'qb_has "ORDER source ↓, id ↑"'
  save_layout o
  L=$E2E_TMP/config/sqlmux/layouts/o.toml
  check "layout.save o：表 tab 的 order 是数组，两项：source（desc = true）、id（升序）" eval 'python3 - "$L" <<PY
import sys, tomllib
d = tomllib.load(open(sys.argv[1], "rb"))
t = d["session"][0]["window"][0]["pane"][0]["tab"][0]
o = t["order"]
sys.exit(0 if isinstance(o, list) and [x["col"] for x in o] == ["source", "id"] and o[0].get("desc") is True and not o[1].get("desc") else 1)
PY' || sed -n '/tab.order/,$p' "$L"
  mkdir -p "$D/keep"; cp -R "$E2E_TMP/config/sqlmux/layouts" "$D/keep/"
  e2e_type ":"; sleep 0.3; e2e_type qa; key Enter; sleep 1.2
  python3 - "$D/keep/layouts/o.toml" "$D/keep/layouts/old.toml" <<'PY'
import re, sys
s = open(sys.argv[1]).read()
s = re.sub(r'\n *\[\[session\.window\.pane\.tab\.order\]\]\n( *(col|desc) = .*\n)+', '\n', s)
s = s.replace('        table = "t_ord"\n', '        table = "t_ord"\n        order = "source"\n        desc = true\n')
open(sys.argv[2], "w").write(s)
PY
  check "（准备）旧格式的布局文件：order = \"source\" 加 desc = true，没有 order 数组" eval 'grep -q "^ *order = \"source\"" "$D/keep/layouts/old.toml" && grep -q "^ *desc = true" "$D/keep/layouts/old.toml" && ! grep -q "tab.order" "$D/keep/layouts/old.toml"'
  mkdir -p "$D/c2"; cp -Rp "$D/own"/. "$D/c2/"; cp -R "$D/keep/layouts" "$D/c2/"
  e2e_start -C "$D/c2" -S "$D/st" -D "$D/data" "$E2E_BIN --layout o"; wait_for 6 flag_is alternate_on 1; sleep 2; wait_for 8 settled
  check "--layout o（新格式）：chip source ↓, id ↑，第一行 c 里 id 最小（2）" eval 'qb_has "ORDER source ↓, id ↑" && ids_are "2,5,8,11,1,4,7,10,3,6,9,12,"'
  e2e_type ":"; sleep 0.3; e2e_type qa; key Enter; sleep 1.2
  e2e_start -C "$D/c2" -S "$D/st2" -D "$D/data2" "$E2E_BIN --layout old"; wait_for 6 flag_is alternate_on 1; sleep 2; wait_for 8 settled
  check "--layout old（order = \"source\" + desc = true）：读成一列，chip source ↓，c 在前，同 source 里 id 升序" eval 'qb_has "ORDER source $DESC" && ids_are "2,5,8,11,1,4,7,10,3,6,9,12,"'
  e2e_type ":"; sleep 0.3; e2e_type qa; key Enter; sleep 1.2
  check "退出写的 last.toml：order 是新格式（数组），只有 source（desc = true），没有旧的 desc 键" eval 'python3 - "$D/st2/sqlmux/last.toml" <<PY
import sys, tomllib
d = tomllib.load(open(sys.argv[1], "rb"))
t = d["session"][0]["window"][0]["pane"][0]["tab"][0]
o = t["order"]
sys.exit(0 if isinstance(o, list) and [x["col"] for x in o] == ["source"] and o[0].get("desc") is True and "desc" not in t else 1)
PY' || { ls "$D/st2/sqlmux" 2>&1; sed -n '/kind = "table"/,$p' "$D/st2/sqlmux/last.toml" 2>&1 | head -12; }
}
