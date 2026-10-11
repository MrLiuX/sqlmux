#!/usr/bin/env bash
# F12.11 表格的 O、fv、fc、fe（specs/m12-grid/task.md F12.11；tech-design §6.8、§7.6）
# O 也开值编辑器（e 保留）、fv 按这个值过滤（* 保留）、fe 导出（ge 保留）、fc 隐藏这一列（同 COLS 里取消勾选；转置下作用于光标所在字段；结果 tab 同样能用）；which-key 的 f 组多出三项。自建库 t_f。
. "$(dirname "$0")/lib.sh"
SOLO=1
e2e_build || exit 1
. "$(dirname "$0")/palette.sh"
. "$(dirname "$0")/paging.sh"
. "$(dirname "$0")/ordering.sh"
. "$(dirname "$0")/cellops.sh"
. "$(dirname "$0")/valeditor.sh"
. "$(dirname "$0")/exporting.sh"

D=$(mktemp -d "${TMPDIR:-/tmp}/sqlmux-e2e-cfg.XXXXXX"); trap 'e2e_stop; rm -rf "$D"' EXIT
e2e_own_db || { echo "e2e: could not create the private database" >&2; exit 1; }
psql "$E2E_DB" -q -c "create table t_f (id int primary key, name text, qty int, tag text)" -c "insert into t_f values (1,'ann',30,'x'),(2,'bob',10,null),(3,'o''neil',20,'x'),(4,'ann',40,'y'),(5,'cat',10,null)" >/dev/null
QUOTE='"'
mkdir -p "$D/own"; printf '[[connection]]\nname = "doraemon"\nengine = "postgres"\ndsn = "%s"\n' "$E2E_DB" >"$D/own/connections.toml"; chmod 600 "$D/own/connections.toml"
hdr() { e2e_text 35 159 "$(hy)" | sed 's/│/ /g; s/  */ /g; s/^ //; s/ $//'; }

start -C "$D/own"; open_table t_f; wait_for 8 settled
check "（准备）表头 id name qty tag，COLS 4/4" eval '[[ $(hdr) == *"id name qty tag"* ]] && qb_has "COLS 4/4"'
# ---- which-key 的 f 组
key f; sleep 1
check "f：which-key 列出 h H v c e（fv 过滤、fc 隐藏列、fe 导出，加原来的 fh / fH）" eval 'for k in "v →" "c →" "e →" "h →" "H →"; do e2e_plain | grep -qF "$k" || { echo "  lacks $k"; false; }; done'
key Escape; sleep 0.3
# ---- O 开值编辑器
key l; key O; sleep 0.7
check "O：值编辑器开了（标题 name · text）" eval 'pop_open && [[ $(pop_title) == "name · text" ]]'
key Escape; sleep 0.4
key e; sleep 0.7
check "e 仍然开值编辑器" eval 'pop_open && [[ $(pop_title) == "name · text" ]]'
key Escape; sleep 0.4
# ---- fv 按这个值过滤
key f v; wait_for 8 settled; sleep 0.5
check "fv：按这个值过滤（WHERE $(q name) = 'ann'，id 1、4）" eval 'where_is "$(q name) = '"'ann'"'" && ids_are "1,4,"'
clear_where
# ---- fe 导出
key f e; sleep 0.7
check "fe：导出下拉开了（4 项）" eval 'has4'
key Escape; sleep 0.4
# ---- fc 隐藏这一列
key g g; key 0; key l; key l; key f c; sleep 0.6
check "fc（光标在 qty 列）：表头少了 qty，COLS 3/4，光标挪到相邻的可见列" eval '[[ $(hdr) == *"id name tag"* && $(hdr) != *qty* ]] && qb_has "COLS 3/4"'
key g c; sleep 0.6
check "gc 里 qty 是未勾选的（[ ] qty）" eval 'e2e_plain | grep -qE "\\[ \\] qty"'
key Escape; sleep 0.4
# ---- 转置下 fc：作用于光标所在字段
key T; sleep 0.5; key g g; key j; sleep 0.3      # 转置：第一列是字段名；光标在 name 字段
key f c; sleep 0.6
check "转置视图下 fc：隐藏光标所在的字段（name 行不见了），COLS 2/4" eval '! e2e_plain | grep -qE "│ name +│" && qb_has "COLS 2/4"'
key T; sleep 0.4
check "转回竖直：表头只有 id tag" eval '[[ $(hdr) == *"id tag"* && $(hdr) != *name* ]]'
_e2e_kill

# ---- 结果 tab 上的 fc
start -C "$D/own"; key C-l
setsql "select * from t_f order by id"; run
key C-j; sleep 0.5; key l; key l
key f c; sleep 0.6
g=($(geom 3)); res_hdr() { e2e_text $((g[0] + 1)) $((g[0] + g[2] - 2)) $((g[1] + 2)) | sed 's/│/ /g; s/  */ /g'; }
check "结果 tab 上 fc：表头少了光标所在的列（qty），id name tag 还在" eval 'h=$(res_hdr); [[ $h == *name* && $h == *tag* && $h != *qty* ]] || { echo "  [$h]"; false; }'
e2e_done
