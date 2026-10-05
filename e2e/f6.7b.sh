#!/usr/bin/env bash
# F6.7 的审查补充（specs/m6-config/task.md F6.7；tech-design §13「危险语句先确认」）：窄终端里确认框的宽度、语句压成一行并截到 60 列、
# 最多列 8 条、点框外等于取消、手动事务模式也问、只读连接先拒绝不弹框、「保存并关闭 tab」含删除行时的确认。写库的部分在自己建的库里做。
. "$(dirname "$0")/lib.sh"
e2e_build || exit 1
. "$(dirname "$0")/palette.sh"

D=$(mktemp -d "${TMPDIR:-/tmp}/sqlmux-e2e-cfg.XXXXXX"); trap 'e2e_stop; rm -rf "$D"' EXIT
e2e_own_db || { echo "e2e: could not create the private database" >&2; exit 1; }
q() { psql "$E2E_DB" -At -c "$1"; }
q "create table t_del (id int primary key, v text); insert into t_del select g, 'r' || g from generate_series(1, 5) g" >/dev/null
mkdir -p "$D/on" "$D/ro"
(umask 077; printf '[[connection]]\nname = "doraemon"\nengine = "postgres"\ndsn = "%s"\n' "$E2E_DB" >"$D/on/connections.toml"
 printf '[[connection]]\nname = "doraemon"\nengine = "postgres"\ndsn = "%s"\nread_only = true\n' "$E2E_DB" >"$D/ro/connections.toml")
printf 'confirm_dangerous = true\n' >"$D/on/config.toml"; printf 'confirm_dangerous = true\n' >"$D/ro/config.toml"
paste() { t set-buffer -b e2e -- "$1"; t paste-buffer -r -p -b e2e -t t; sleep 0.4; }
setsql() { key Escape; key g g; key d G; key i; paste "$1"; key Escape; key g g; }
asked() { e2e_plain | grep -q "危险语句"; }
# 确认框的行（框里 │ 之间的文字）
dbox() { e2e_panes | awk '$1 == "-" { print $2, $3, $4, $5; exit }'; }                  # 浮层的 X Y W H
boxlines() { local g y; g=($(dbox)); [[ -n ${g[0]} ]] || return 0
  for ((y = g[1] + 1; y < g[1] + g[3] - 1; y++)); do e2e_text $((g[0] + 1)) $((g[0] + g[2] - 2)) $y | sed 's/^ *//; s/ *$//'; done; }
boxw() { local g; g=($(dbox)); echo "${g[2]}"; }

start -x 80 -y 35 -C "$D/on"; sleep 0.5; key C-l
setsql $'delete   from\n   t_del;'
key Enter; sleep 0.8
check "（80 列）语句里的换行和连续空白压成一个空格：列的是「delete from t_del」（分号不列）" eval 'asked && boxlines | grep -qx "delete from t_del" || { boxlines; false; }'
check "（80 列）确认框放得下：宽度不超过终端的 80 列，按钮 y 执行 / n 取消在框里" eval '[[ $(boxw) -le 80 ]] && e2e_plain | grep -q "y 执行   n 取消"'
key Escape; sleep 0.3
long=$(printf 'x%.0s' $(seq 1 90))
setsql "delete from t_$long"; key Enter; sleep 0.8
check "很长的语句：截到 60 列、以 … 结尾，框宽不超过 80" eval 'l=$(boxlines | grep "delete from t_x"); [[ $l == *"…" && ${#l} -le 62 && $(boxw) -le 80 ]] || { echo "  [$l] w=$(boxw)"; false; }'
key Escape; sleep 0.3
sql=; for i in $(seq 1 9); do sql+="delete from t_a$i;"$'\n'; done
setsql "$sql"; key V; key G; key Enter; sleep 0.8
check "9 条危险语句：「这次执行有 9 条危险语句：」，列出 8 条，第 9 条写「… 另有 1 条」" eval 'asked && e2e_plain | grep -q "这次执行有 9 条危险语句：" && [[ $(boxlines | grep -c "^delete from t_a") == 8 ]] && boxlines | grep -q "另有 1 条"'
check "点框外（树里一点）等于取消：框关了，没有执行" eval 'e2e_click 5 10; sleep 0.5; ! asked'

# ---- 手动事务模式下也问
key Escape; e2e_keys C-p; sleep 0.3; e2e_type ">手动提交"; sleep 0.5; key Enter; sleep 0.5
setsql "delete from t_del"; key Enter; sleep 0.8
check "手动事务模式：delete from t_del 也弹确认" asked
key n; sleep 0.4
check "n：没执行，t_del 还是 5 行" eval '[[ $(q "select count(*) from t_del") == 5 ]]'

# ---- 「保存并关闭 tab」含删除行：n 不关，y 存完就关
start -x 160 -y 45 -C "$D/on"; key C-h; open_table t_del; wait_for 8 settled
key d; key d
e2e_keys C-p; sleep 0.3; e2e_type ">保存并关闭"; sleep 0.5; key Enter; sleep 0.6
check "保存并关闭 tab（含 1 行删除）：弹「将删除 1 行」" eval 'e2e_plain | grep -q "将删除 1 行"'
key n; sleep 0.5
check "n：不关 tab，库里还是 5 行" eval '[[ $(q "select count(*) from t_del") == 5 ]] && [[ $(e2e_plain | head -1) == *t_del* ]]'
e2e_keys C-p; sleep 0.3; e2e_type ">保存并关闭"; sleep 0.5; key Enter; sleep 0.6; key y; sleep 1.5
check "y：存完就关，库里 4 行，① 变成引导页（或 pane 关了）" eval '[[ $(q "select count(*) from t_del") == 4 ]] && ! e2e_plain | head -1 | grep -q "t_del"'

# ---- 只读连接：先拒绝，不弹框
start -C "$D/ro"; key C-l
setsql "delete from t_del"; key Enter; sleep 0.8
check "只读连接：整次拒绝（日志里是只读提示），不弹危险确认" eval '! asked && screen_has "只读" && [[ $(q "select count(*) from t_del") == 4 ]]'

e2e_done
