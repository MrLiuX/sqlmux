# Shared by f10.4.sh (PG) and f10.4b.sh (MySQL): F10.4 query result paging on a 1203-row table t_page(id 1..1203) in the
# script's own database. Needs $D/own/connections.toml with a connection called doraemon, and the helpers below.
paste() { t set-buffer -b e2e "$1"; t paste-buffer -r -p -b e2e -t t; sleep 0.4; }
setsql() { key Escape; key g g; key d G; key i; paste "$1"; key Escape; key g g; }
RP=3
rtitle() { local g; g=($(geom $RP)); e2e_text "${g[0]}" $((g[0] + g[2] - 1)) "${g[1]}" | noicon; }
rrow() { local g; g=($(geom $RP)); e2e_text $((g[0] + 1)) $((g[0] + g[2] - 2)) $((g[1] + $1)) | sed 's/^ //; s/ *$//'; }
rtabs() { tabbar $RP | sed -E 's/ {3,}.*//; s/- │/ │/g; s/-$//'; }
done_run() { wait_for 8 eval '[[ $(bar) != *busy* ]]'; sleep 0.3; }
run() { key Enter; done_run; }
firstid() { rrow 3 | awk -F'│' '{ gsub(/ /, "", $2); print $2 }'; }              # 结果第一行的 id
title_has() { [[ $(rtitle) == *"$1"* ]] || { echo "  title: $(rtitle)"; false; }; }
paged() { key "$1"; wait_for 8 eval "[[ \$(rtitle) == *'$2'* ]]"; sleep 0.4; }   # KEY 之后等标题里出现 TEXT

paging_body() {
  start -C "$D/own"
  key C-l
  setsql "select id from t_page order by id"
  run
  check "第 1 页：标题 500+ 行，第一行 id 1" eval 'title_has "500+ 行" && [[ $(firstid) == 1 ]]'
  tabs0=$(rtabs)
  key C-j
  check "[ 在第 1 页：无事（标题、第一行不变）" eval 'key "["; sleep 0.8; title_has "500+ 行" && [[ $(firstid) == 1 ]]'
  paged "]" "501–1000+ 行"
  check "] 翻到第 2 页：标题 501–1000+ 行，第一行 id 501" eval 'title_has "501–1000+ 行" && [[ $(firstid) == 501 ]]'
  check "只换了这一个 tab：结果区的 tab 栏不变（${tabs0}）" eval '[[ $(rtabs) == "$tabs0" ]]'
  # ---- 第 2 页导出 CSV：文件名加 -p2（F10.4 导出加页号），内容是这一页的 500 行
  seq_n=$(rtitle | grep -oE '#[0-9]+' | tr -d '#')
  read -r x y <<<"$(g=($(geom $RP)); echo $((g[0] + g[2] - 9)) "${g[1]}")"
  e2e_click "$x" "$y"; sleep 0.8; key C-k; key C-j
  check "第 2 页点导出：当前目录下是 query_1-${seq_n}-p2.csv，500 行数据 + 表头，第一行数据是 501" eval 'f=$E2E_TMP/query_1-${seq_n}-p2.csv; [[ -f $f && $(wc -l <"$f" | tr -d " ") == 501 && $(sed -n 2p "$f" | tr -d "\r\" ") == 501 ]] || { ls $E2E_TMP | head; false; }'
  paged "]" "1001–1203 行"
  check "再 ]：第 3 页（最后一页）：标题 1001–1203 行，第一行 id 1001" eval 'title_has "1001–1203 行" && [[ $(firstid) == 1001 ]]'
  check "最后一页再按 ]：无事" eval 'key "]"; sleep 0.8; title_has "1001–1203 行" && [[ $(firstid) == 1001 ]]'
  paged "[" "501–1000+ 行"
  check "[ 回第 2 页：id 501" eval '[[ $(firstid) == 501 ]]'
  paged "[" "500+ 行"
  check "[ 回第 1 页：标题 500+ 行（不是 1–500），id 1" eval 'title_has "500+ 行" && [[ $(firstid) == 1 ]] && [[ $(rtitle) != *"1–500"* ]]'
  paged "]" "501–1000+ 行"; key R; wait_for 8 eval '[[ $(rtitle) == *"500+ 行"* ]]'; sleep 0.4
  check "第 2 页上按 R：重跑回第 1 页（500+ 行，id 1）" eval '[[ $(firstid) == 1 ]]'

  # ---- 自己写了 LIMIT 的语句没有翻页
  key C-k
  setsql "select id from t_page order by id limit 600"
  run
  key C-j; t0=$(rtitle)
  check "带 LIMIT 的语句：] 无事（标题不变）" eval 'key "]"; sleep 0.8; [[ $(rtitle) == "$t0" ]] && [[ $(firstid) == 1 ]]'
  check "带 LIMIT 的语句：[ 也无事" eval 'key "["; sleep 0.8; [[ $(rtitle) == "$t0" ]]'
  key C-k
}
