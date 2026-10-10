#!/usr/bin/env bash
# F9.7 命令面板的 DDL 预览可滚动（specs/m9-feedback2/task.md F9.7；tech-design §12「预览」）
# 预览区放不下时 C-f / C-b 各滚半页，滚到末尾停住，换了候选回到顶部；上面还有行时第一行画 …，下面还有时最后一行画 …；底部提示里有「C-b/C-f 滚动预览」；
# PG 的 DDL 含索引（CREATE INDEX）和表 / 列的注释；没有可滚的预览时 C-u 仍是清输入。自建库，宽表 t_wide（30 列、两个索引、表和列注释）。
. "$(dirname "$0")/lib.sh"
e2e_build || exit 1
. "$(dirname "$0")/palette.sh"

D=$(mktemp -d "${TMPDIR:-/tmp}/sqlmux-e2e-cfg.XXXXXX"); trap 'e2e_stop; rm -rf "$D"' EXIT
e2e_own_db || { echo "e2e: could not create the private database" >&2; exit 1; }
mkdir -p "$D/c"; (umask 077; printf '[[connection]]\nname = "d"\nengine = "postgres"\ndsn = "%s"\n' "$E2E_DB" >"$D/c/connections.toml")
cols=$(python3 -c 'print(", ".join("c%d int" % i for i in range(1, 31)))')
psql "$E2E_DB" -q -c "create table t_wide (id bigint primary key, $cols); create index t_wide_c1_idx on t_wide (c1); create index t_wide_c2_c3_idx on t_wide (c2, c3); comment on table t_wide is 'wide one'; comment on column t_wide.c5 is 'five'; create table t_wide2 (id int, name text)"
box() { local g y; g=($(pbox)); for ((y = g[1]; y < g[1] + g[3]; y++)); do e2e_text $((g[0] + 1)) $((g[0] + g[2] - 2)) "$y" | sed 's/^ *//; s/ *$//'; done; }
prev() { box | awk '/^─+$/ { n++; next } n == 2 { print }'; }                      # 预览区的行（第二条分隔线之后、底部提示之前）
top() { prev | head -1; }
bot() { prev | tail -1; }
hasline() { prev | grep -qF -- "$1"; }

SOLO= start -x 160 -y 50 -C "$D/c"; sleep 0.5
pal "@t_wide"; sleep 1.5
check "预览：第一行 create table public.t_wide (，下面还有行：最后一行是 …，上面没有 …" eval '[[ $(top) == "create table public.t_wide ("* && $(bot) == "…" ]] || { prev | head -3; echo "  bot=[$(bot)]"; false; }'
check "底部提示里有「C-b/C-f 滚动预览」" eval 'box | grep -q "C-b/C-f 滚动预览"'
key C-f; sleep 0.5
check "C-f：滚半页，第一行换成 …（上面还有），create table 看不到了，最后一行还是 …" eval '[[ $(top) == "…" && $(bot) == "…" ]] && ! hasline "create table public.t_wide" || { prev; false; }'
n1=$(prev | sed -n 2p)
for i in 1 2 3; do key C-f; sleep 0.4; done
check "再 C-f 三次（共四次，30 列的宽表）：看到 CREATE INDEX（索引在 DDL 里）" eval 'hasline "CREATE INDEX t_wide_c1_idx"'
for i in 1 2 3 4; do key C-f; sleep 0.3; done
check "滚到末尾：最后一行是表注释 / 列注释，不再有「下面还有」的 …，上面有 …" eval '[[ $(top) == "…" && $(bot) == "comment on column public.t_wide.c5 is '"'five'"';" ]] || { prev | tail -4; false; }'
check "含表的注释和列的注释（comment on table …、comment on column …）" eval 'hasline "comment on table public.t_wide is '"'wide one'"';" && hasline "comment on column public.t_wide.c5"'
snap=$(prev)
key C-f; sleep 0.4
check "到末尾再 C-f：停住，内容不变" eval '[[ $(prev) == "$snap" ]]'
key C-b; sleep 0.4
check "C-b：往回滚半页（下面又有 …）" eval '[[ $(bot) == "…" && $(prev) != "$snap" ]]'
for i in 1 2 3 4 5 6 7 8; do key C-b; sleep 0.3; done
check "一直 C-b：回到开头（第一行 create table，上面没有 …）" eval '[[ $(top) == "create table public.t_wide ("* ]]'

# ---- 换了候选回到顶部
e2e_type "2"; sleep 0.3; key BSpace; sleep 0.3
key C-f; key C-f; sleep 0.5
check "（准备）又滚下去了" eval '[[ $(top) == "…" ]]'
key Down; sleep 1.2
check "换了候选（↓ 到 t_wide2）：预览回到顶部，是 t_wide2 的 DDL（create table public.t_wide2）" eval '[[ $(top) == "create table public.t_wide2 ("* ]] || { prev | head -3; false; }'
key Up; sleep 1
check "换回 t_wide：同样从顶部开始" eval '[[ $(top) == "create table public.t_wide ("* ]]'

# ---- 没有可滚的预览时 C-u 是清输入
clear_input; e2e_type "@t_wide2"; sleep 1.2
check "（准备）t_wide2 的 DDL 很短，没有 …，放得下" eval '[[ $(top) == "create table public.t_wide2 ("* && $(bot) != "…" ]]'
key C-u; sleep 0.4
check "没有可滚的预览：C-u 清输入（输入框里没有 @t_wide2 了）" eval '[[ $(input) != *t_wide2* ]] || { echo "  input: [$(input)]"; false; }'
e2e_done
