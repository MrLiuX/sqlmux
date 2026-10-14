#!/usr/bin/env bash
# F6.6 非 vim 模式（specs/m6-config/task.md F6.6a / F6.6b；tech-design §6.9）
# vim = false：query 和 WHERE 是没有模式的普通编辑器，表格、树多一层方向键，grid 有 PgUp / PgDn / Delete，query 用 C-e 执行。
# 编辑器的各项行为有单测，叠加的键位有单测；这里在真实终端里只用「非 vim 用户手边的键」走一遍：写 SQL、选区、替换、撤销、执行选区、
# 补全里 esc 只关列表、WHERE、表格浏览 / 翻页 / 删除 / 编辑保存、树。写库的部分在自己建的库里做。
. "$(dirname "$0")/lib.sh"
e2e_build || exit 1
. "$(dirname "$0")/palette.sh"

D=$(mktemp -d "${TMPDIR:-/tmp}/sqlmux-e2e-cfg.XXXXXX"); trap 'e2e_stop; rm -rf "$D"' EXIT
e2e_own_db || { echo "e2e: could not create the private database" >&2; exit 1; }
mkdir -p "$D/c"; printf 'vim = false\n' >"$D/c/config.toml"
(umask 077; printf '[[connection]]\nname = "doraemon"\nengine = "postgres"\ndsn = "%s"\n' "$E2E_DB" >"$D/c/connections.toml")
cl() { e2e_text 104 159 $(($1 + 1)) | sed 's/ *$//'; }                         # query 的第 N 行
VISUAL_BG=#2d3f76
modeless() { [[ $(bar) != *NORMAL* && $(bar) != *INSERT* && $(bar) != *VISUAL* && $(bar) != *COMMAND* ]]; }
sel_bg() { [[ $(e2e_style "$1" "$2") == *"bg=$VISUAL_BG"* ]]; }
done_run() { wait_for 8 eval '[[ $(bar) != *busy* ]]'; sleep 0.5; }
rval() { local g; g=($(geom 3)); e2e_text $((g[0] + 1)) $((g[0] + g[2] - 2)) $((g[1] + 3)) | awk -F'│' '{ gsub(/ /, "", $2); print $2 }'; }   # ③ 结果第一行第一列
logs() { local g; g=($(geom 3)); local y; for ((y = g[1] + 1; y < g[1] + g[3] - 2; y++)); do e2e_text $((g[0] + 1)) $((g[0] + g[2] - 2)) $y | sed 's/^ //; s/ *$//'; done | sed '/^$/d'; }

start -C "$D/c"; key C-l
check "状态栏不画模式块（没有 NORMAL / INSERT）" modeless
e2e_type "select 111\;"; key Enter; e2e_type "select 222\;"; sleep 0.4
check "一直可输入：不用按 i；↵ 换行，两条语句两行" eval '[[ $(cl 1) == "select 111;" && $(cl 2) == "select 222;" ]] || { echo "  [$(cl 1)] [$(cl 2)]"; false; }'
e2e_type ":x gt ?"; sleep 0.3
check ": 空格 g t ? 都是要打的字：面板、which-key 没有打开，文字进了缓冲区" eval '[[ $(cl 2) == "select 222;:x gt ?" ]] && ! is_open && ! screen_has "which-key"' || cl 2
for i in 1 2 3 4 5 6 7; do key BSpace; done
# ---- 选择与替换、撤销重做
key S-Left; key S-Left; key S-Left; key S-Left
check "S-← 选择：选区用 visual 底（#2d3f76），选中了第 2 行末尾的 222;" eval 'sel_bg 111 3 && sel_bg 114 3 && ! sel_bg 110 3 && modeless'
e2e_type "9"; sleep 0.3
check "再输入就替换选区：select 之后是 9" eval '[[ $(cl 2) == "select 9" ]]'
key C-z
check "C-z 撤销：回到选区替换之前" eval '[[ $(cl 2) == "select 222;" ]]'
key C-y
check "C-y 重做" eval '[[ $(cl 2) == "select 9" ]]'
key C-z
key C-a; e2e_type "X"; sleep 0.3
check "C-a 行首：X 打在第 2 行最前面" eval '[[ $(cl 2) == "Xselect 222;" ]]'
key BSpace
check "Backspace 删前一个字符" eval '[[ $(cl 2) == "select 222;" ]]'
key Right; key Right; key Delete
check "Delete 删光标后的一个字符" eval '[[ $(cl 2) == "seect 222;" ]]'
key C-w
check "C-w 删到词首" eval '[[ $(cl 2) == "ect 222;" ]] || { echo "  [$(cl 2)]"; false; }'

# ---- Tab 插空格；执行：光标所在语句 / 选区
start -C "$D/c"; key C-l
e2e_type "select 111\;"; key Enter; e2e_type "select 222\;"; key Enter; e2e_type "select 333\;"; key Up; key Up; key C-a; key Tab; sleep 0.3
check "Tab 插入空格（不是切焦点），光标还在 query" eval '[[ $(cl 1) == "  select 111;" ]] && focus_is 2 || { echo "  [$(cl 1)] focus $(focused)"; false; }'
key BSpace; key BSpace
key C-e; done_run
check "没有选区：C-e 执行光标所在的语句（第 1 行），结果值 111" eval '[[ $(rval) == 111 ]] || { echo "  result: [$(rval)]"; false; }'
key S-Down; key S-Down; key C-e; done_run
check "有选区（第 1、2 行，光标停在第 3 行行首）：C-e 只执行选区，两条语句、两个结果 tab（#2、#2·2），第 3 行没有执行" eval 'tb=$(tabbar 3); [[ $tb == *"query_1 #2"* && $tb == *"query_1 #2·2"* && $tb != *"·3"* ]] || { echo "  tabs: $(tabbar 3)"; false; }'

# ---- 补全：esc 只关列表；C-c 取消选区
start -C "$D/c"; key C-l
e2e_type "select * from t_us"; sleep 0.6
check "输入 t_us：自动弹出补全，没有模式块" eval 'screen_has "t_user" && modeless'
key Escape; sleep 0.3
check "esc 只关列表：文字还在，仍在 query 里（没有模式可退）" eval '! screen_has "┌─────" || true; [[ $(cl 1) == "select * from t_us" && $(focused) == 2 ]]'
key S-Left; key S-Left; key C-c; sleep 0.3
check "C-c 取消选区（不算退出提示的第一次）：选区没了，也没有「再按一次」" eval '! sel_bg 123 2 && ! screen_has "再按一次"' || true
# ---- WHERE：/ 进入（表格的键），一直可输入，↵ 执行，C-r 历史，esc 回表格
saved() { [[ $(qb) == *已保存* ]]; }
qb() { e2e_text 36 95 3; }
editing() { [[ $(bar) == *"-- editing WHERE --"* ]]; }
start -C "$D/c"; open_table t_order; wait_for 8 settled
key /; e2e_type "id < 5 gt ?"; sleep 0.3
check "WHERE：/ 进入，不分模式，gt 和 ? 是字符，状态栏没有模式块、有 editing WHERE" eval 'editing && modeless && [[ $(e2e_text 36 95 2) == *"id < 5 gt ?"* ]]'
for i in 1 2 3 4 5; do key BSpace; done; sleep 0.3
key Enter; wait_for 8 settled
check "↵ 执行：回到表格，一页（id < 5 只有 4 行）" eval '! editing && [[ $(e2e_text 36 95 2) == *"id < 5"* ]] && [[ $(qb) == *"PAGE 1/1"* ]]'
key /; key C-r; sleep 0.5
check "C-r：历史下拉打开" eval '[[ -n $(e2e_panes | awk "\$1 == \"-\"") ]]'
key Escape; key Escape; sleep 0.3
check "esc 先关下拉；再 esc 回表格（输入框恢复成生效的条件）" eval '! editing && [[ $(e2e_text 36 95 2) == *"id < 5"* ]]'

# ---- 表格：方向键移动、↵ 编辑、C-s 保存；PgDn / PgUp 翻页；Delete 标记 / 取消删除
# F8.9：160 宽的 ① 只有 63 列，查询条里保存 / 刷新组排在 COLS、PAGE、LIMIT 之后才让位，所以修改数还看得到
start -C "$D/c"; open_table t_order; wait_for 8 settled
key Down; key Right; key Right; key Right; key Enter; sleep 0.3; e2e_type 77.5; key Enter; sleep 0.3
key C-s; wait_for 8 eval '[[ $(psql "$E2E_DB" -At -c "select amount from t_order where id = 2") == 77.50 ]]'
check "方向键 ↓ → → → 移到第 2 行 amount，↵ 编辑、输入 77.5、↵ 提交、C-s 保存：库里 id = 2 的 amount 是 77.50" eval '[[ $(psql "$E2E_DB" -At -c "select amount from t_order where id = 2") == 77.50 ]]'
key PageDown; wait_for 8 settled; sleep 0.4
check "PgDn：下一页（PAGE 2/…）" eval '[[ $(qb) == *"PAGE 2/"* ]]'
key PageUp; wait_for 8 settled; sleep 0.4
check "PgUp：上一页（PAGE 1/…）" eval '[[ $(qb) == *"PAGE 1/"* ]]'
pend() { qb | sed -E 's/.*PAGE [^ ]+ *(COLS [0-9]+\/[0-9]+)?//; s/[^0-9]+/ /g; s/^ +//; s/ +$//'; }   # F8.9：COLS 先让位，数字在 PAGE（和 COLS）之后   # 查询条上保存按钮的修改数（图标之后的数字）
key Delete; sleep 0.3
check "Delete：标记删除这一行，保存按钮上计数 1" eval '[[ $(pend) == 1 ]] || { echo "  [$(qb)]"; false; }'
key Delete; sleep 0.3
check "再 Delete：取消标记，计数没了" eval '[[ -z $(pend) ]] || { echo "  [$(qb)]"; false; }'
key C-h; sleep 0.3
tsel() { local y; for y in $(seq 3 43); do [[ $(e2e_style 4 "$y") == *bg=#364a82* ]] && { e2e_text 1 32 "$y"; return; }; done; }
for i in $(seq 1 14); do [[ $(tsel) == *"Views"* ]] && break; key Down; done
check "↓ 在树里移动：光标走到 Views (2)，它是折叠的（▸）" eval '[[ $(tsel) == *"▸"*"Views"* ]] || { echo "  [$(tsel)]"; false; }'
key Right; sleep 0.3
check "→ 展开" eval '[[ $(tsel) == *"▾"*"Views"* ]] || { echo "  [$(tsel)]"; false; }'
key Left; sleep 0.3
check "← 折叠" eval '[[ $(tsel) == *"▸"*"Views"* ]] || { echo "  [$(tsel)]"; false; }'
key Up; sleep 0.3
check "↑ 往上移了一行" eval '[[ $(tsel) != *Views* ]]'

# ---- 鼠标拖动选择（query）
start -C "$D/c"; key C-l; e2e_type "select 111\;"; key Enter; e2e_type "select 222\;"; sleep 0.3
e2e_drag 111 2 114 2; sleep 0.3
check "鼠标在 query 里拖动：选区用 visual 底" eval 'sel_bg 111 2 && sel_bg 113 2 && ! sel_bg 110 2'
e2e_type "7"; sleep 0.3
check "再输入替换选区：select 7" eval '[[ $(cl 1) == "select 7" || $(cl 1) == "select 7;" ]] || { echo "  [$(cl 1)] [$(cl 2)]"; false; }'

# ---- sqlmux keys 按配置导出叠加后的键位
mkdir -p "$D/k-novim/sqlmux" "$D/k-vim/sqlmux"; printf 'vim = false\n' >"$D/k-novim/sqlmux/config.toml"
kk() { XDG_CONFIG_HOME=$D/$1 XDG_STATE_HOME=$D/st XDG_DATA_HOME=$D/st "$E2E_BIN" keys; }
check "vim = false：sqlmux keys 里 grid 的 ↓ 是 grid.down、PgDn 是 grid.page.next、Delete 是 grid.row.delete，query 的 C-e 是 query.run，↵ 没有绑定" eval 'k=$(kk k-novim); grep -qE "\| grid \| .(↓|<Down>|Down). \| grid.down" <<<"$k" && grep -q "grid.page.next" <<<"$k" && grep -q "grid.row.delete" <<<"$k" && grep -qE "\| query \| .(C-e|<C-e>). \| query.run" <<<"$k" && ! grep -qE "\| query \| .(↵|<CR>). \|" <<<"$k" || { grep -E "grid.down|query.run" <<<"$k"; false; }'
check "vim = true（默认）：方向键两种模式都有（F16.6），PgDn、Delete 和 query 的 C-e 不在" eval 'k=$(kk k-vim); grep -qE "\| grid \| .(↓|<Down>|Down). \| grid.down" <<<"$k" && grep -qE "\| tree \| .(→|<Right>|Right). \| tree.expand" <<<"$k" && ! grep -E "grid.page.next|grid.row.delete" <<<"$k" | grep -qE "PgDn|PageDown|PgUp|Del" && ! grep -qE "\| query \| .(C-e|<C-e>). \| query.run" <<<"$k"'

e2e_done
