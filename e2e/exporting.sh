# Shared by f12.4.sh (PG) and f12.4b.sh (MySQL): F12.4 export CSV / JSON on t_x(id, name 'n,<id>"q"', qty id * 1.5, tag NULL when id % 3 = 0) with 1203 rows,
# in the script's own database ($D/own/connections.toml, doraemon); t_huge (2M rows) for the cancel. Files land in the app's cwd, $E2E_TMP.
toast_has() { e2e_cap | grep -qF -- "$1"; }
export_pick() { key g e; sleep 0.6; local i; for ((i = 0; i < $1; i++)); do e2e_keys C-n; sleep 0.12; done; e2e_keys Enter; }   # N: 0 CSV·页 1 CSV·全部 2 JSON·页 3 JSON·全部
has4() { local t; for t in "CSV · 当前页" "CSV · 全部" "JSON · 当前页" "JSON · 全部"; do e2e_plain | grep -qF "$t" || { echo "  lacks $t"; return 1; }; done; }
exported() { wait_for 20 eval 'toast_has "已导出"'; }
fcount() { ls "$E2E_TMP" | grep -cE "$1"; }
py() { python3 - "$@"; }
json_ok() { py "$1" "$2" "$3" <<'PY'
import json, sys
d = json.load(open(sys.argv[1]))
n, first = int(sys.argv[2]), int(sys.argv[3])
ok = isinstance(d, list) and len(d) == n and d[0]["id"] == first and list(d[0].keys())[:2] == ["id", "name"]
sys.exit(0 if ok else 1)
PY
}

export_body() {
  SOLO=1 start -C "$D/own"; open_table t_x; wait_for 8 settled
  key g e; sleep 0.6
  check "ge：下拉列出 CSV · 当前页 / CSV · 全部 / JSON · 当前页 / JSON · 全部" eval 'has4'
  key Escape; sleep 0.3
  x=$(e2e_find "$(printf '\xef\x80\x99')" 3 | cut -d' ' -f1)      # 查询条第二行的导出按钮（U+F019，转置那一组里）
  e2e_click "$x" 3; sleep 0.6
  check "点查询条的导出按钮：开同一个下拉（4 项）" eval '[[ -n $x ]] && has4'
  key Escape; sleep 0.3
  export_pick 3; exported; sleep 0.3
  check "JSON · 全部：toast 带文件名 t_x.json 和 1203 行" eval 'toast_has "t_x.json" && toast_has "1203"'
  check "文件 t_x.json：python3 -m json.tool 能解析，1203 个对象，第一个 id 1，键顺序 id、name 打头（和表格的列一致）" eval 'python3 -m json.tool "$E2E_TMP/t_x.json" >/dev/null && json_ok "$E2E_TMP/t_x.json" 1203 1'
  check "JSON 的类型：id 是数字、tag 为 NULL 的是 null（id 3）、name 里的逗号和引号原样" eval 'py "$E2E_TMP/t_x.json" <<PY
import json, sys
d = json.load(open(sys.argv[1]))
sys.exit(0 if isinstance(d[0]["id"], int) and d[2]["tag"] is None and d[0]["tag"] == "t" and d[4]["name"] == "n,5\"q\"" else 1)
PY'
  sleep 3.2
  export_pick 1; exported; sleep 0.3
  check "CSV · 全部：t_x.csv，1203 行加表头；用 csv 模块解析：表头 id,name,qty,tag（…），NULL 是空，逗号和引号的值原样" eval 'py "$E2E_TMP/t_x.csv" <<PY
import csv, sys
rows = list(csv.reader(open(sys.argv[1], newline="", encoding="utf-8")))
h = rows[0]
ok = len(rows) == 1204 and h[:4] == ["id", "name", "qty", "tag"] and rows[1][0] == "1" and rows[3][3] == "" and rows[5][1] == "n,5\"q\"" and rows[-1][0] == "1203"
sys.exit(0 if ok else 1)
PY'
  sleep 3.2
  export_pick 0; exported; sleep 0.3
  check "CSV · 当前页：文件已存在，写成 t_x-2.csv（100 行加表头，是这一页）" eval '[[ -f $E2E_TMP/t_x-2.csv && $(wc -l <"$E2E_TMP/t_x-2.csv") -ge 101 ]] && py "$E2E_TMP/t_x-2.csv" <<PY
import csv, sys
rows = list(csv.reader(open(sys.argv[1], newline="", encoding="utf-8")))
sys.exit(0 if len(rows) == 101 and rows[-1][0] == "100" else 1)
PY'
  sleep 3.2
  export_pick 2; exported; sleep 0.3
  check "JSON · 当前页：t_x-2.json，100 个对象" eval '[[ -f $E2E_TMP/t_x-2.json ]] && json_ok "$E2E_TMP/t_x-2.json" 100 1'

  # ---- 隐藏的列不导出，排序和 WHERE 照旧
  sleep 3.2
  key g c; sleep 0.5; key j; key j; key j; key Space; key Escape; sleep 0.5      # 隐藏 tag（第 4 列）
  export_pick 0; exported; sleep 0.3
  check "隐藏 tag 后导出 CSV · 当前页：t_x-3.csv 的表头没有 tag" eval 'py "$E2E_TMP/t_x-3.csv" <<PY
import csv, sys
h = next(csv.reader(open(sys.argv[1], newline="", encoding="utf-8")))
sys.exit(0 if "tag" not in h and h[:3] == ["id", "name", "qty"] else 1)
PY'
  key /; sleep 0.3; e2e_type "id < 11"; key Enter; wait_for 8 settled; sleep 0.4
  key g o; sleep 0.5; dd_goto id; key Enter; wait_for 8 settled; sleep 0.4   # 当前排序列 id 上 ↵：翻成 ↓
  sleep 3.0
  export_pick 3; exported; sleep 0.3
  check "WHERE id < 11 和 ORDER id ↓ 之后 JSON · 全部：t_x-3.json 10 个对象，第一个 id 10，没有 tag 键（隐藏的列）" eval 'py "$E2E_TMP/t_x-3.json" <<PY
import json, sys
d = json.load(open(sys.argv[1]))
sys.exit(0 if len(d) == 10 and d[0]["id"] == 10 and "tag" not in d[0] else 1)
PY'
  _e2e_kill

  # ---- 结果 tab：右键菜单 → 导出…；全部 = 同一条语句去掉自动 LIMIT 重跑
  start -C "$D/own"; key C-l
  setsql "select id, name from t_x order by id"; run
  key C-j; sleep 0.5
  g=($(geom 3)); rclick $((g[0] + 8)) $((g[1] + 4)); sleep 0.6; menu_click "导出…"
  for i in 1; do e2e_keys C-n; sleep 0.12; done; e2e_keys Enter; exported; sleep 0.3
  check "结果 tab：右键 → 导出… → CSV · 全部：文件名 query_1-<序号>.csv，1203 行加表头（不是 500）" eval 'f=$(ls "$E2E_TMP" | grep -E "^query_1-[0-9]+\.csv$" | head -1); [[ -n $f ]] && [[ $(python3 -c "import csv, sys; print(len(list(csv.reader(open(sys.argv[1], newline=\"\", encoding=\"utf-8\")))))" "$E2E_TMP/$f") == 1204 ]] || { echo "  files: $(ls "$E2E_TMP" | tr "\n" " ")"; false; }'
  sleep 3.2
  g=($(geom 3)); rclick $((g[0] + 8)) $((g[1] + 4)); sleep 0.6; menu_click "导出…"; e2e_keys Enter; exported; sleep 0.3
  check "结果 tab：CSV · 当前页：已有同名则 -2，里面是这一页的 500 行（加表头）" eval 'f=$(ls "$E2E_TMP" | grep -E "^query_1-[0-9]+-2\.csv$" | head -1); [[ -n $f ]] && [[ $(python3 -c "import csv, sys; print(len(list(csv.reader(open(sys.argv[1], newline=\"\", encoding=\"utf-8\")))))" "$E2E_TMP/$f") == 501 ]] || { echo "  files: $(ls "$E2E_TMP" | tr "\n" " ")"; false; }'
  _e2e_kill

  # ---- C-c 取消：删掉写了一半的文件
  start -C "$D/own"; open_table t_huge; wait_for 15 settled
  export_pick 1; sleep 0.4
  e2e_keys C-c; sleep 1.5
  check "导出 200 万行时 C-c：toast「导出已取消」，半截文件被删掉（目录里没有 t_huge.csv）" eval 'toast_has "导出已取消" && [[ $(fcount "^t_huge") == 0 ]] || { echo "  files: $(ls "$E2E_TMP" | tr "\n" " ")"; false; }'
}
