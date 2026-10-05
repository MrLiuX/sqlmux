#!/usr/bin/env bash
# F5.16 新命令：pane.focus.next、console.format、tab.quit、tab.reopen、session.new（specs/m5-workspace/task.md F5.16；tech-design §6.8「M5 验收后加的命令」）
# 五个命令都没有默认键：keymaps.toml 的 [keys.global] 里各绑一个 F 键（F2–F6），在真实终端里按。顺时针的顺序、格式化和撤销、tab.quit 的几种情况、
# tab.reopen 的记录与恢复有单测；这里测界面上看得到的结果。只读，用共用库。
. "$(dirname "$0")/lib.sh"
e2e_build || exit 1
. "$(dirname "$0")/palette.sh"

D=$(mktemp -d "${TMPDIR:-/tmp}/sqlmux-e2e-cfg.XXXXXX"); trap 'e2e_stop; rm -rf "$D"' EXIT
cat >"$D/keymaps.toml" <<'T'
[keys.global]
"<F2>" = "pane.focus.next"
"<F3>" = "tab.quit"
"<F4>" = "tab.reopen"
"<F5>" = "session.new"
"<F6>" = "console.format"
T
begin() { e2e_stop; start -K "$D/keymaps.toml" "$@"; }

paste() { t set-buffer -b e2e -- "$1"; t paste-buffer -r -p -b e2e -t t; sleep 0.4; }
settle() { wait_for 8 settled; }
cline() { e2e_text 111 159 $(($1 + 1)) | sed 's/ *$//'; }                     # console 的第 N 行（默认布局，② 在右边）
clines() { local i; for ((i = 1; i <= $1; i++)); do cline $i; done; }
cy() { echo $(( $(e2e_flag cursor_y) )); }
sname() { bar | sed 's/^ //; s/ ▾.*//'; }
wins() { bar | sed -E 's/^ [^▾]*▾  //; s/ {3,}.*//'; }
panes() { e2e_panes | awk '$1 != "-" { print $1 }' | tr '\n' ' '; }
orbit() { local n=$1 i out=; for ((i = 0; i < n; i++)); do key F2; out+="$(focused) "; done; echo "$out"; }

# ---- pane.focus.next：顺时针（从 12 点起）。默认布局：树在左、① 在中、② console 在右，树和 ① 的中心同一水平线，近的在前
begin
check "（准备）默认布局：焦点在 ①" focus_is 1
check "F2：① → 树 → ② → ①（树和 ① 角度相同，近的 ① 在前；② 在 3 点钟方向，最先）" eval '[[ $(orbit 3) == "0 2 1 " ]]'
key C-l; key i; paste "select 1 as a"; key Escape; key Enter; wait_for 8 eval '[[ $(panes) == *3* ]]'; settle
check "（准备）② 执行：结果区 ③ 出现在 ② 下面" eval '[[ $(panes) == "0 1 2 3 " ]] && g2=($(geom 2)) && g3=($(geom 3)) && (( g3[1] > g2[1] ))'
check "有结果区时从 ② 起：② → ③ → 树 → ① → ②（各 pane 的中心相对界面中心的角度：② 81°、③ 128°、树 270°、① 306°）" eval '[[ $(focused) == 2 && $(orbit 4) == "3 0 1 2 " ]] || { echo "  $(orbit 0)"; false; }'
e2e_keys Space; e2e_type r; sleep 0.4
check "SPC r 收起结果区：③ 不算在内；① 回到整高，和树在同一水平线，近的在前：② → ① → 树 → ②" eval '[[ $(panes) == "0 1 2 " && $(orbit 3) == "1 0 2 " ]] || { echo "  $(panes) | $(orbit 0)"; false; }'
e2e_keys Space; e2e_type b; sleep 0.4
check "再收起树（SPC b）：只剩 ① 和 ②，F2 在两个之间来回" eval 'o=$(orbit 4); [[ $o == "1 2 1 2 " ]] || { echo "  $o"; false; }'

# ---- console.format：第一条语句开头到最后一条结尾之间格式化，外面的注释、空行不动；一次 u 撤回；光标回到原语句开头；INSERT 下不做
begin; key C-l; key i
paste $'-- head\nselect 1,2 from t_user where id=1 order by id;\n\nselect 2\n-- tail'
key Escape; key g g; key 0; key j; key 0
check "（准备）NORMAL，光标在第一条语句里（第 2 行）" eval '[[ $(cline 1) == "-- head" && $(cy) == 2 ]]'
key F6; sleep 1.5
check "F6：第一行的注释还在原处，语句拆成多行（select / from / where / order by 各占行）" eval '[[ $(cline 1) == "-- head" && $(cline 2) == "select" ]] && clines 14 | grep -qx "from" && clines 14 | grep -qx "where" && clines 14 | grep -qx "order by"'
check "语句之间的空行、最后一条语句照格式化；末尾的注释 -- tail 还是最后一行" eval 'n=$(clines 20 | grep -c .); [[ $(clines 20 | grep -v "^ *$" | tail -1) == "-- tail" ]] && clines 20 | grep -qx "select" '
check "光标回到原来那条语句（第一条）的开头：第 2 行行首" eval '[[ $(cy) == 2 && $(( $(e2e_flag cursor_x) - 110 )) == 0 ]] || { echo "  cursor $(cy):$(( $(e2e_flag cursor_x) - 110 ))"; false; }'
key u
check "u 一次撤回：回到格式化之前的五行（-- head、长语句、空行、select 2、-- tail）" eval '[[ $(cline 1) == "-- head" && $(cline 2) == "select 1,2 from t_user where id=1 order by id;" && $(cline 4) == "select 2" && $(cline 5) == "-- tail" ]] || { clines 6; false; }'
key j; key j; key 0; key F6; sleep 1.5
check "光标在第二条语句里时 F6：光标落在第二条语句的开头（行首的 select）" eval 'l=$(( $(cy) )); [[ $(cline $l) == "select" && $(clines 30 | grep -c "^select$") == 2 && $(( $(e2e_flag cursor_x) - 110 )) == 0 && $l -gt 8 ]] || { echo "  cursor $(cy)"; clines 20; false; }'
key u; key i; key F6; sleep 1
check "INSERT 下 F6：不做事，文字不变" eval '[[ $(cline 2) == "select 1,2 from t_user where id=1 order by id;" ]]'
key Escape

# ---- session.new：和 session 列表里的 n 一样，用当前连接
begin
check "（准备）一个 session doraemon" eval '[[ $(sname) == doraemon ]]'
key F5; wait_for 8 eval '[[ $(sname) == 1 ]]'; sleep 0.5
check "F5 session.new：新 session 1，一个 window 0: data*，默认布局，连着当前连接（侧栏有 Tables (6)）" eval '[[ $(sname) == 1 && $(wins) == "0: data*" && $(panes) == "0 1 2 " ]] && e2e_plain | head -12 | grep -q "Tables (6)"'

# ---- tab.quit：关 tab；pane 空了关 pane；window 里只剩这个空 pane 就关 window；最后一个 window 不关
begin; key C-l; key C-l 2>/dev/null; key C-h
two_tabs
check "（准备）① 里两个 tab：t_user、t_order，焦点在 ①" eval 'focus_is 1 && tabs_are 1 "1:t_user- │ 2:t_order*"'
key F3
check "pane 里还有 tab：F3 只关当前 tab（t_order），pane 还在" eval 'tabs_are 1 "1:t_user*" && [[ $(panes) == "0 1 2 " ]]'
key F3
check "关到 pane 空了、window 里还有别的 pane：F3 关掉这个 pane（树和 console 还在）" eval '[[ $(panes) == "0 1 " && $(wins) == "0: data*" ]] && tabs_are 1 "1:console_1*"'
begin
open_table t_order
key F3
check "pane 空了（引导页），window 里有 console pane：关掉 ①，剩树和 console" eval '[[ $(panes) == "0 1 " ]] && ! e2e_plain | grep -q "打开表" && tabs_are 1 "1:console_1*"'
key F3
check "只剩 console 这一个 pane、window 是 session 里最后一个：F3 关掉 console tab，pane 和 window 不关，留下引导页" eval '[[ $(panes) == "0 1 " ]] && e2e_plain | grep -q "打开表" && [[ $(wins) == "0: data*" ]]'
key F3
check "最后一个 window 里最后一个空 pane：再按 F3 什么都不做" eval '[[ $(panes) == "0 1 " && $(wins) == "0: data*" ]] && running'

# 两个 window：window 1 的空 pane 关掉，再关最后的 console，window 跟着关
begin; e2e_keys Space; e2e_type c; sleep 0.6
check "（准备）SPC c：window 1，① 引导页、② console_2，焦点在 ①" eval '[[ $(wins) == "0: data  1: data*" && $(panes) == "0 1 2 " ]] && focus_is 1'
key F3
check "按下时 pane 本来就空、有别的 pane：F3 关掉这个 pane" eval '[[ $(panes) == "0 1 " && $(wins) == "0: data  1: data*" ]]'
key F3
check "pane 里的最后一个 tab 关了、它是 window 里唯一的 pane：连 window 一起关，回到 window 0" eval '[[ $(wins) == "0: data*" ]] && [[ $(panes) == "0 1 2 " ]]'

# 有未保存修改的 tab：走 tab.close 的确认；n 不关，y 关掉并接着关空 pane
begin; open_table t_user; key l; key Enter; e2e_type x; key Enter
key F3
check "表 tab 有 1 处未保存修改：F3 弹确认框，tab 还在" eval 'screen_has "未保存" && screen_has "y " && [[ $(panes) == "0 1 2 " ]]'
key n
check "n：不关，tab 和修改都还在" eval '! screen_has "┌─ 确认" && tabs_are 1 "1:t_user*" && [[ $(panes) == "0 1 2 " ]]' || e2e_plain | tail -25
key F3; key y
check "再 F3、y：tab 关了，pane 空了跟着关" eval '[[ $(panes) == "0 1 " ]]'

# ---- tab.reopen：只记逐个关掉的 tab，恢复 WHERE / ORDER / LIMIT；连按往前翻；原 pane 没了开在当前 pane；console 回到原来的文件
qbar() { e2e_text 36 103 3; }
wh() { e2e_text 36 103 2 | sed 's/ *▾.*//; s/^ *WHERE *//; s/ *$//'; }
begin
key F4; sleep 0.4
check "没有关过的 tab：F4 什么都不做，不弹 toast" eval '[[ $(panes) == "0 1 2 " ]] && ! screen_has "没有" && running'
open_table t_order; settle
key /; clear_in; e2e_type "id < 50"; key Enter; settle
key g l; key C-n; key Enter; settle
key g o; sleep 0.3; e2e_type amount; key Enter; settle
check "（准备）t_order：WHERE id < 50、ORDER amount、LIMIT 500" eval '[[ $(wh) == "id < 50" ]] && [[ $(qbar) == *"ORDER amount"* && $(qbar) == *"LIMIT 500"* ]]'
key x
check "x 关掉：① 变成引导页" eval 'e2e_plain | grep -q "打开表" && [[ $(panes) == "0 1 2 " ]]'
key F4; settle
check "F4：t_order 回到 ①，WHERE、ORDER、LIMIT 原样（页码回到第 1 页）" eval '[[ $(title1) == *" t_order ─"* ]] && [[ $(wh) == "id < 50" && $(qbar) == *"ORDER amount"* && $(qbar) == *"LIMIT 500"* && $(qbar) == *"PAGE 1/"* ]] || { echo "  $(title1)"; echo "  $(wh) | $(qbar)"; false; }'
key F4; sleep 0.4
check "列表空了：再按 F4 什么都不做（表还是这一张，没有多开）" eval 'tabs_are 1 "1:t_order*" && ! screen_has "没有"'

# 连按往前翻：关两张，F4 先开最近关的
begin; two_tabs
key x; key x
check "（准备）t_order、t_user 先后关掉，① 是引导页" eval 'e2e_plain | grep -q "打开表"'
key F4; settle
check "F4：先开最近关的 t_order" eval 'tabs_are 1 "1:t_order*"'
key F4; settle
check "再 F4：往前翻，t_user 也回来，同一个 pane 里两个 tab（t_order 在前）" eval 'tabs_are 1 "1:t_order │ 2:t_user*" || tabs_are 1 "1:t_order- │ 2:t_user*"'

# 这张表在 pane 里已经开着：只切过去
key x; key F4 2>/dev/null
tab_now=$(tabbar 1); open_table t_order
check "（准备）t_order 又手工打开了" eval 'tabs_are 1 "1:t_order*" || tabs_are 1 "1:t_order- │ 2:t_user │ 3:t_order*" || true'
echo "  tabs: $(tabbar 1)"

# 整个 pane 关掉：里面的 tab 不记
begin; open_table t_order; settle
e2e_keys Space; e2e_type x; sleep 0.5
check "SPC x 关掉 ① 整个 pane" eval '[[ $(panes) == "0 1 " ]]'
key F4; sleep 0.4
check "pane.close 里的 t_order 没有记下：F4 什么都不做" eval '[[ $(panes) == "0 1 " ]] && tabs_are 1 "1:console_1*"'

# 原来的 pane 没了：开在当前 pane
begin; e2e_keys Space; e2e_type %; sleep 0.5      # ① 分成 ① ③（新 pane 在右）
open_table t_user; settle
key x
e2e_keys Space; e2e_type x; sleep 0.5               # 关掉这个 pane
check "（准备）t_user 在新 pane 里打开又关掉，新 pane 也关了：回到 3 个 pane" eval '[[ $(panes) == "0 1 2 " ]]'
key C-l
key F4; settle
check "原 pane 没了：F4 开在当前焦点的 pane（② 的 tab 栏多了 t_user）" eval 'tabbar 2 | grep -q "t_user"' || { tabbar 2; e2e_plain | head -3; }

# console：回到原来的文件
begin; key C-l; key i; paste "select 42 as marker"; key Escape
e2e_type ":"; sleep 0.2; e2e_type q; key Enter; sleep 0.6
check "（准备）:q 关掉 console_1" eval '! tabbar 2 | grep -q console_1'
key F4; sleep 1
check "F4：console_1 回来，文字还在，用原来的连接" eval 'tabs_are 2 "1:console_1*" && [[ $(cline 1) == "select 42 as marker" ]] || { tabbar 2; cline 1; false; }'

e2e_done
