#!/usr/bin/env bash
# F12.10 命令面板的「表」范围列出 query（specs/m12-grid/task.md F12.10；tech-design §12）
# @ 范围在表之后列出 Queries 节点下的全部 query（文件加开着的）：名字、记下的连接名、右端 Query；@que 匹配到它并排在表后面；
# ↵：开着的 query 聚焦过去，没开着的在当前 data pane 新开 tab。预览区显示文件前几行。只读，用共用库。
. "$(dirname "$0")/lib.sh"
e2e_build || exit 1
. "$(dirname "$0")/palette.sh"

D=$(mktemp -d "${TMPDIR:-/tmp}/sqlmux-e2e-cfg.XXXXXX"); trap 'e2e_stop; rm -rf "$D"' EXIT
mkdir -p "$D/data/sqlmux/queries"; printf 'select 42 as answer\n' >"$D/data/sqlmux/queries/mine.sql"
start -D "$D/data" -S "$D/st"; sleep 1.2
e2e_keys C-p; sleep 0.4; e2e_type "@"; sleep 0.6
check "面板 @ 列表里表在前、query 在后：最后几项里有 query_1（连接 doraemon，右端 Query）" eval 'list | grep -q "^query_1  doraemon  Query|"'
check "表仍排在 query 前面（第一项是表，query_1 在 t_user 之后）" eval 'n=$(list | grep -n "^t_user " | cut -d: -f1); q=$(list | grep -n "^query_1 " | cut -d: -f1); [[ -n $n && -n $q && $n -lt $q ]]'
e2e_type "que"; sleep 0.5
check "@que：匹配到 query_1（Query 类），没有表混进来" eval '[[ $(list | grep -c "Query|") -ge 1 && $(list | grep -c "  表|") == 0 ]] || { list; false; }'
check "没有打开过的 mine.sql 也在列表里（@mine）" eval 'for i in 1 2 3 4 5 6; do e2e_keys BSpace; done; e2e_type "mine"; sleep 0.5; list | grep -q "^mine .*Query|"'
# ---- ↵：开着的聚焦过去
for i in 1 2 3 4 5 6; do e2e_keys BSpace; done; e2e_type "query_1"; sleep 0.5
check "（准备）焦点在 ① data pane，query_1 开在 ②" eval 'palette_open && true'
key Enter; sleep 0.8
check "↵ 选 query_1（已在 ② 开着）：焦点聚焦到 ②，② 的当前 tab 是 query_1" eval 'focus_is 2 && [[ $(tabbar 2) == *query_1* ]]'
key C-h; sleep 0.3
# ---- ↵：没开着的在当前 data pane 新开 tab
e2e_keys C-p; sleep 0.4; e2e_type "@mine"; sleep 0.6
check "预览区显示文件的前几行（select 42 as answer）" eval 'e2e_plain | grep -q "select 42 as answer"'
key Enter; sleep 1
check "↵ 选 mine（没开着）：在当前 data pane（①）新开 tab，焦点在 ①，tab 栏有 mine" eval 'focus_is 1 && [[ $(tabbar 1) == *mine* ]]'
e2e_done
