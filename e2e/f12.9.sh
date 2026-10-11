#!/usr/bin/env bash
# F12.9 ORDER 浮层改成 COLS 式的列表（specs/m12-grid/task.md F12.9；tech-design §7.6「ORDER」、§6.4）
# go 打开就是列表（焦点在列表）：j / k 移动、不是往过滤框打字；space 只加入 / 翻转，a 移出，A 清空回默认；↵ 只按这一列并关闭；/ 进过滤框；提示行写着这些键；
# 没有「默认」这一项。多列、space 的细节在 f12.1 里。自建库 t_ord（12 行）。
. "$(dirname "$0")/lib.sh"
SOLO=1
e2e_build || exit 1
. "$(dirname "$0")/palette.sh"
. "$(dirname "$0")/ordering.sh"

D=$(mktemp -d "${TMPDIR:-/tmp}/sqlmux-e2e-cfg.XXXXXX"); trap 'e2e_stop; rm -rf "$D"' EXIT
e2e_own_db || { echo "e2e: could not create the private database" >&2; exit 1; }
psql "$E2E_DB" -q -c "create table t_ord (id int primary key, source text, n int)" -c "insert into t_ord select g, (array['a','b','c'])[1 + g % 3], g * 10 from generate_series(1, 12) g" >/dev/null
mkdir -p "$D/own"; printf '[[connection]]\nname = "doraemon"\nengine = "postgres"\ndsn = "%s"\n' "$E2E_DB" >"$D/own/connections.toml"; chmod 600 "$D/own/connections.toml"

start -C "$D/own"; open_table t_ord; wait_for 8 settled
go_open
check "go：浮层是列表（右上角 3/3），列出 id / source / n 和类型，没有「默认」这一项" eval 'list_open && [[ $(dd_items | sed -E "s/^[0-9][↑↓] //; s/ .*//" | tr "\n" " ") == "id source n " ]] && ! e2e_plain | grep -q "默认"'
check "提示行：j/k 移动 · SPC 方向 · a 移出 · A 清空 · ↵ 只按这一列 · / 搜索" eval 'e2e_plain | grep -qF "j/k 移动 · SPC 方向 · a 移出 · A 清空 · ↵ 只按这一列 · / 搜索"'
check "打开时光标在第一项（id），状态栏 COMMAND" eval '[[ $(dd_cursor) == 0 ]] && mode_is COMMAND'
key j; key j; sleep 0.3
check "j j：光标移到第三项（n），没有把 jj 打进过滤框（列表的计数还是 3/3）" eval '[[ $(dd_cursor) == 2 ]] && [[ $(list_n) == 3/3 ]]'
key k; sleep 0.3
check "k：光标回到第二项（source）" eval '[[ $(dd_cursor) == 1 ]]'
key Space; sleep 0.8
check "space：source 追加（1↑），列表不关，chip source ↑" eval 'dd_has "1↑ source" && qb_has "ORDER source $ASC"'
key Space; sleep 0.8
check "space 再按：翻转 1↓，不是去掉（F12.9）" eval 'dd_has "1↓ source" && qb_has "ORDER source $DESC"'
key a; sleep 0.8
check "a：移出排序，标记没了，chip 回到默认 id ↑" eval '[[ -z $(marks) ]] && qb_has "ORDER id $ASC"'
key Space; sleep 0.6; key j; key Space; sleep 0.6
check "（准备）source ↑、n ↑ 两列：1↑ source、2↑ n" eval '[[ $(marks) == "1↑ source|"* ]] && dd_has "2↑ n"'
key A; sleep 0.8
check "A：清空，标记全没了，chip 回到默认 id ↑（列表还开着）" eval '[[ -z $(marks) ]] && qb_has "ORDER id $ASC" && list_open'
key Enter; sleep 0.8
check "↵（光标在 n）：只按 n 并关闭浮层，chip n ↑" eval '! list_open && qb_has "ORDER n $ASC"'
go_open; dd_goto source; key Enter; sleep 0.8
check "再开，光标到 source ↵：换成只按 source（n 的标记没了）" eval 'qb_has "ORDER source $ASC"'
go_open
check "再开：只有 source 带标记" eval '[[ $(marks) == "1↑ source|" ]]'
key Escape; sleep 0.4
check "esc：关浮层" eval '! list_open'
e2e_done
