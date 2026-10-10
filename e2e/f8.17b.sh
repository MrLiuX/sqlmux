#!/usr/bin/env bash
# F8.17 审查时点名的几处（reviewer 提测：多表列名后到、keymaps 里绑 complete.accept、快速 SQL 无选中时 ↵ 执行）
# 只读，用共用库。
. "$(dirname "$0")/lib.sh"
e2e_build || exit 1

D=$(mktemp -d "${TMPDIR:-/tmp}/sqlmux-e2e-cfg.XXXXXX"); trap 'e2e_stop; rm -rf "$D"' EXIT
printf '[keys.complete]\n"<C-y>" = "complete.accept"\n' >"$D/k.toml"
pop() { e2e_panes | awk '$1 == "-" { print $2, $3, $4, $5 }' | tail -1; }
items() { local g y; g=($(pop)); [[ -n ${g[0]} ]] || return 0
  for ((y = g[1] + 1; y < g[1] + g[3] - 1; y++)); do e2e_text $((g[0] + 2)) $((g[0] + g[2] - 2)) $y | sed 's/^ *//; s/ *$//'; done; }
names() { items | sed 's/  .*//' | tr '\n' '|'; }
SELECT=#364a82
sel() { local g y; g=($(pop)); [[ -n ${g[0]} ]] || return 0
  for ((y = g[1] + 1; y < g[1] + g[3] - 1; y++)); do style_has $((g[0] + g[2] - 2)) $y bg=$SELECT >/dev/null && e2e_text $((g[0] + 2)) $((g[0] + g[2] - 2)) $y | sed 's/  .*//; s/^ *//; s/ *$//'; done; }
line() { e2e_text 104 159 $(($1 + 1)) | sed 's/ *$//'; }
fresh() { key Escape; key Escape; key g g; key d G; key i; }

# ---- 多表：列名是后到的（第一次用到的表要先取列），按下 Tab 之后到的列不能把选中冲掉
start -K "$D/k.toml"; key C-l; key i
e2e_type "select * from t_order "; sleep 1                       # t_order 的列先取回来（整串一次发出去时 Tab 会先于任何列表到达，那时没有候选，Tab 就是插入 Tab 键）
e2e_type "a join t_user b on "; e2e_keys Tab; sleep 2                # on ␠ 时已有 t_order 的列表，Tab 选中第一项；t_user 的列是后到的
check "多表 on ␠ 之后马上 Tab（列表里只有先到的 t_order 的列）：t_user 的列后到，选中还在（有选中项）" eval '[[ -n $(sel) && -n $(names) ]] || { echo "  names=$(names) sel=[$(sel)]"; false; }'
first=$(sel); key Enter; sleep 0.5
check "↵ 接受选中的那一项，补一个空格" eval '[[ $(line 1) == "select * from t_order a join t_user b on $first" || $(line 1) == "select * from t_order a join t_user b on "*"$first" ]] && [[ -z $(pop) ]] || { echo "  [$(line 1)] first=$first"; false; }'

# ---- 用户把 complete.accept 绑到 C-y：无选中时什么都不发生，Tab 之后接受
fresh; e2e_type "select * from t_log wher"; sleep 0.6; key Enter; sleep 1
before=$(line 1)
key C-y; sleep 0.4
check "无选中项按 C-y：什么都不发生（文字不变、列表还开着、没崩）" eval '[[ $(line 1) == "$before" && -n $(names) && -z $(sel) ]] && running || { echo "  [$(line 1)] names=$(names)"; false; }'
key Tab; key C-y; sleep 0.4
check "Tab 之后 C-y：接受 logged_at" eval '[[ $(line 1) == "select * from t_log where logged_at" && -z $(sel) ]] || { echo "  [$(line 1)]"; false; }'
key Escape; key Escape

# ---- 快速 SQL：无选中的列表上 ↵ 直接执行
e2e_keys C-p; sleep 0.4; e2e_type ";select * from t_order where "; sleep 1.2
check "快速 SQL：where ␠ 弹出列名，无选中" eval '[[ -n $(names) && -z $(sel) ]]'
key Enter; sleep 1.5
check "↵：不接受列名，直接执行（输入还是 where 结尾，结果区里有错误）" eval 'e2e_plain | grep -q ";select \* from t_order where" && ! e2e_plain | grep -q "where id" && [[ -z $(sel) ]]'
e2e_done
