#!/usr/bin/env bash
# F8.4 需要列名的位置自动弹出列名补全（specs/m8-feedback/task.md F8.4；tech-design §9.7「空前缀的列名补全」）
# 上下文识别（哪些词之后、找不到表不弹、多表）由 sqlkit 的单测覆盖；这里在真实终端里测 console 和 WHERE：打了空格就弹，
# esc 关掉后继续打字再弹，不该弹的位置不弹，WHERE 空输入框不弹（删空再 ↵ 是清除过滤）。只读，用共用库。
. "$(dirname "$0")/lib.sh"
e2e_build || exit 1

paste() { t set-buffer -b e2e -- "$1"; t paste-buffer -r -p -b e2e -t t; sleep 0.4; }
pop() { e2e_panes | awk '$1 == "-" { print $2, $3, $4, $5 }' | tail -1; }
items() { local g y; g=($(pop)); [[ -n ${g[0]} ]] || return 0
  for ((y = g[1] + 1; y < g[1] + g[3] - 1; y++)); do e2e_text $((g[0] + 2)) $((g[0] + g[2] - 2)) $y | sed 's/^ *//; s/ *$//'; done; }
names() { items | sed 's/  .*//' | tr '\n' '|'; }
names_are() { [[ $(names) == "$1" ]] || { echo "  candidates: '$(names)', want '$1'"; false; }; }
has() { [[ "|$(names)" == *"|$1|"* ]] || { echo "  candidates: $(names)"; false; }; }
none() { [[ -z $(pop) ]] || { echo "  popup: $(names)"; false; }; }
line() { e2e_text 104 159 $(($1 + 1)) | sed 's/ *$//'; }
fresh() { key Escape; key Escape; key g g; key d G; key i; }
settle() { wait_for 8 settled; }
EVT="id|occurred_at|kind|"      # t_event 的列（按表里的顺序）

start; key C-l; key i

# ---- console：用户的原话 —— delete from t_event where ␠
e2e_type "delete from t_event where "; sleep 1
check "delete from t_event where ␠：不打字母就弹出 t_event 的列，没有「全部列」" names_are "$EVT"
check "注释里有列的类型和所属的表" eval 'items | head -1 | grep -q "bigint · t_event"'
key Escape; sleep 0.3
check "esc：关掉列表（并退出 INSERT）" eval 'none && mode_is NORMAL'
key A; e2e_type "ki"; sleep 0.6
check "继续打字：按前缀过滤，只剩 kind" names_are "kind|"
key Enter; sleep 0.3
check "↵ 接受 kind（F8.1：补空格，不换行）" eval '[[ $(line 1) == "delete from t_event where kind" && -z $(line 2) ]]'

# ---- 其他位置：select、and、or、order by、group by、set、on、having
fresh; e2e_type "select * from t_event where kind = 'a' and "; sleep 1
check "… and ␠：弹出列" names_are "$EVT"
fresh; e2e_type "select * from t_event where kind = 'a' or "; sleep 1
check "… or ␠：弹出列" names_are "$EVT"
fresh; e2e_type "select * from t_event order by "; sleep 1
check "order by ␠：弹出列" names_are "$EVT"
fresh; e2e_type "select * from t_event group by "; sleep 1
check "group by ␠：弹出列" names_are "$EVT"
fresh; e2e_type "update t_event set "; sleep 1
check "update t_event set ␠：弹出列" names_are "$EVT"
fresh; e2e_type "select * from t_event a join t_log b on "; sleep 1
check "join … on ␠：两张表的列都有（多表的排法同打了字母之后）" eval 'has id && has msg && has logged_at'
fresh; e2e_type "select * from t_event group by kind having "; sleep 1
check "having ␠：弹出列" names_are "$EVT"

# ---- select 列表：词后空格弹；还没写 from 时找不到表，不弹
fresh; e2e_type "select "; sleep 0.8
check "select ␠（没有 from，找不到表）：不弹" none
fresh; paste "select  from t_event"; key Escape; key 0; key f; key t; key a; e2e_type " "; sleep 1
check "光标在 select 与 from 之间（语句里有 t_event）：select ␠ 弹出列" eval 'has id && has kind'

# ---- 不该弹的位置
fresh; e2e_type "select * from "; sleep 0.8
check "from ␠：不自动弹（前缀为空，表名不属于这一条）" none
key C-n; sleep 0.6
check "C-n 手动唤起：列出的仍是表名（有 t_event、t_log），不是列" eval 'has t_event && has t_log && ! has kind'
fresh; e2e_type "select * from t_event where kind = 'a',"; sleep 0.8
check "逗号后面没有空格（a,）：不弹（免得打 a,b 闪一下）" none
fresh; e2e_type "select * from t_event where kind = "; sleep 0.8
check "kind = ␠（取值位置，不是列名位置）：不弹列名" eval '! has occurred_at'

# ---- WHERE 输入框
e2e_stop; SOLO=1 start; open_table t_order; settle
key /; clear_in; sleep 0.4
check "WHERE 空输入框（刚按 /）：不弹" none
e2e_type "id > 0 or "; sleep 1
check "WHERE：or ␠ 之后弹出 t_order 的列（有 id、status、paid）" eval 'has id && has status && has paid'
clear_in; e2e_type "id > 0 and "; sleep 1
check "WHERE：and ␠ 之后同样弹出" eval 'has id && has paid'
check "没有关键字候选（前缀为空只列列名）" eval '! has "is null" && ! has and'
key Escape; sleep 0.4
check "esc：关掉列表（并回 NORMAL）" eval 'none && mode_is NORMAL'
# 空输入框 ↵ 是清除过滤，不会被当成接受列名
key Escape; sleep 0.3   # 退出 WHERE 的 NORMAL（在里面 / 是打开历史）
key /; clear_in; e2e_type "paid"; sleep 0.5; key Enter; key Enter; settle
check "（准备）WHERE paid 已执行，行数少于 6000" eval 'mode_is NORMAL && [[ $(cnt) -lt 6000 && $(cnt) -gt 0 ]]'
key /; clear_in; key Enter; sleep 0.5; settle
check "WHERE 删空后 ↵：清除过滤、执行，不被当成接受列名（NORMAL，输入框是空的）" eval 'mode_is NORMAL && [[ -z $(where_in) ]]'
check "结果是整张表（6000 行）" eval '[[ $(cnt) == 6000 ]]'

e2e_done
