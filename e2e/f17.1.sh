#!/usr/bin/env bash
# F17.1 tab 栏横向滚动（specs/m17-polish-exec/task.md F17.1；tech-design §7.8「tab 栏」）
# 放不下时当前 tab 始终可见，两端是 dim 的 ‹N / N›（N 是那一边看不见的 tab 数），点 ‹N / N› 各滚一屏（不换当前 tab），
# gt / gT / {N}gt / 点击切换后滚到当前 tab，滚动位置按 pane 记、不进布局文件，命令面板的 # 范围列全部 tab。窄屏（100 列）里开 8 个 tab。
. "$(dirname "$0")/lib.sh"
SOLO=1   # ① alone right of the sidebar (lib.sh solo)
e2e_build || exit 1
. "$(dirname "$0")/palette.sh"

D=$(mktemp -d "${TMPDIR:-/tmp}/sqlmux-e2e-cfg.XXXXXX"); trap 'e2e_stop; rm -rf "$D"' EXIT
DB=$D/shop.db; sqlite3 "$DB" <"$E2E_ROOT/testdata/seed/sqlite.sql"
mkdir -p "$D/c"; (umask 077; printf '[[connection]]\nname = "shop"\nengine = "sqlite"\ndsn = "%s"\n' "$DB" >"$D/c/connections.toml")
bar() { tabbar "${1:-1}"; }
title() { e2e_plain | sed -n 1p; }
pal_tab() { e2e_keys C-p; sleep 0.3; e2e_type "@$1"; sleep 0.3; key C-t; sleep 0.8; }   # 新开一个 tab

start -x 100 -C "$D/c" "$E2E_BIN shop"
open_table t_user; pal_tab t_order; pal_tab t_order_item
check "三个 tab 放得下：没有 ‹N / N›" eval '[[ $(bar) == "1:t_user │ 2:t_order │ 3:t_order_item*" || $(bar) == "1:t_user- │ 2:t_order- │ 3:t_order_item*" ]] || [[ $(bar) != *"‹"* && $(bar) != *"›"* ]] || { bar; false; }'
for t in t_sku t_log t_kv t_misc t_note; do pal_tab $t; done
check "八个 tab 放不下：当前（8）在，左边藏了 4 个（‹4），右边没有 ›" eval '[[ $(bar) == "‹4 5:t_log │ 6:t_kv │ 7:t_misc- │ 8:t_note*" ]] || { bar; false; }'
check "标题是当前 tab 的表" eval '[[ $(title) == *"t_note"* ]]'

key 1 g t
check "1gt：滚回开头，当前（1）在，右边藏了 5 个（5›），左边没有 ‹" eval '[[ $(bar) == "1:t_user* │ 2:t_order │ 3:t_order_item │ 5›" ]] || { bar; false; }'
key 4 g t
check "4gt：两边都有（‹1 和 4›），当前（4）在" eval '[[ $(bar) == "‹1 2:t_order │ 3:t_order_item │ 4:t_sku* │ 4›" ]] || { bar; false; }'
key 8 g t
check "8gt：滚到最后，当前（8）在，原来的当前（4）标 -" eval '[[ $(bar) == "‹4 5:t_log │ 6:t_kv │ 7:t_misc │ 8:t_note*" ]] || [[ $(bar) == *"8:t_note*" ]] || { bar; false; }'
key 1 g t
key g T
check "第 1 个 tab 上 gT：绕到最后一个（8），滚到它（‹4）" eval '[[ $(bar) == *"8:t_note*" && $(bar) == "‹4 "* ]] || { bar; false; }'
key g t
check "最后一个上 gt：绕回第 1 个，滚回开头" eval '[[ $(bar) == "1:t_user* │ "* && $(bar) == *"5›" ]] || { bar; false; }'
key 6 g t
check "6gt：6 在（当前 tab 不会被挤出）" eval '[[ $(bar) == *"6:t_kv*"* ]] || { bar; false; }'
key 9 g t
check "9gt（超出）不动" eval '[[ $(bar) == *"6:t_kv*"* ]] || { bar; false; }'

# ---- 点 N›：滚一屏，当前 tab 不换
key 1 g t
click_tab 1 "5›"
check "点 5›：滚一屏（4 到 7 在，‹3 和 1›），当前 tab 还是 1（不在屏上，没有 * 标记），标题仍是 t_user" eval '[[ $(bar) == "‹3 4:t_sku │ 5:t_log │ 6:t_kv- │ 7:t_misc │ 1›" && $(bar) != *"*"* && $(title) == *"t_user"* ]] || { bar; title; false; }'
click_tab 1 "‹3"
check "点 ‹3：滚回一屏（回到开头），当前 tab 1 又在屏上" eval '[[ $(bar) == "1:t_user* │ 2:t_order │ 3:t_order_item │ 5›" ]] || { bar; false; }'
click_tab 1 "5›"
click_tab 1 "5:"
check "滚走之后点屏上的 5：切到 t_log，滚动位置不变，1 成为上一个（不在屏上）" eval '[[ $(bar) == "‹3 4:t_sku │ 5:t_log* │ 6:t_kv │ 7:t_misc │ 1›" && $(title) == *"t_log"* ]] || { bar; title; false; }'
click_tab 1 "1›"
check "点 1›：滚到最后一屏，当前 tab（5）不在屏上" eval '[[ $(bar) == "‹7 8:t_note │ +"* || $(bar) == "‹"*" 8:t_note" ]] || { bar; false; }'
key g t
check "之后 gt（5 → 6）：滚回当前 tab 所在的一屏" eval '[[ $(bar) == *"6:t_kv*"* ]] || { bar; false; }'

# ---- 命令面板的 # 范围列全部 tab（包括栏里看不见的）
key 8 g t
e2e_keys C-p; sleep 0.3; e2e_type "#"; sleep 0.5
rows() { e2e_plain | python3 -c "import re, sys; print(' '.join(re.findall(r'(t_\w+) +shop ›', sys.stdin.read())))"; }
check "# 范围：八个 tab 都列出来" eval '[[ $(rows | wc -w) -eq 8 && " $(rows) " == *" t_user "* && " $(rows) " == *" t_order_item "* ]] || { rows; false; }'
key Escape; sleep 0.3

# ---- 滚动位置按 pane 记：分出第二个 pane，各自滚
key Space %; sleep 1
check "分出第二个 pane：第一个 pane 的栏里当前 tab 还在（变窄了，藏的更多）" eval '[[ $(bar 1) == "‹"* && $(bar 1) == *"8:t_note*" ]] || { bar 1; false; }'
B1=$(bar 1); pal_tab t_user; pal_tab t_sku
check "第二个 pane 自己的两个 tab，放得下，没有 ‹N" eval '[[ $(bar 2) == "1:t_user- │ 2:t_sku*" ]] || { bar 2; false; }'
pal_tab t_kv; pal_tab t_misc; pal_tab t_log
check "第二个 pane 也开到放不下：自己滚（‹N，当前 5 在）" eval '[[ $(bar 2) == "‹"* && $(bar 2) == *"5:t_log*" ]] || { bar 2; false; }'
check "第一个 pane 的栏没动（滚动位置各管各的）" eval '[[ $(bar 1) == "$B1" ]] || { bar 1; echo "want $B1"; false; }'
e2e_type ":"; sleep 0.4; e2e_type qa; key Enter; wait_for 8 exited; sleep 0.5
check "布局文件里没有滚动位置（last.toml 在，没有 scroll / offset）" eval 'L=$(find "$E2E_TMP" -name last.toml | head -1); [[ -f $L ]] && ! grep -qi "scroll\|offset" "$L" || { find "$E2E_TMP" -name "last.toml*"; false; }'
e2e_done
