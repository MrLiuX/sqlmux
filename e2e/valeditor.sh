# Shared by f12.2.sh (PG) and f12.2b.sh (MySQL): F12.2 the value editor on t_val(id, doc json(b), note text) — row 1: doc {"b":1,"a":[1,2],"s":"hi","t":true,"n":null}, note 'plain text';
# row 2: doc NULL, note '{"x":1}'. Needs $D/own/connections.toml (doraemon), $D/ro/connections.toml (the same with read_only = true), and the helpers
# DOC1 (the doc of row 1 in the database, one line of JSON), lib.sh / palette.sh / paging.sh.
# the value editor's box: "X Y W H" of the floating box that is not a pane (e2e_panes lists it with "-"), the widest one with " · " in its title
popbox() { local x y w h; while read -r n x y w h _; do [[ $n == - ]] || continue; [[ $(e2e_text "$x" $((x + w - 1)) "$y") == *" · "* ]] && { echo "$x $y $w $h"; return; }; done < <(e2e_panes); }
pop_open() { [[ -n $(popbox) ]]; }
pop_title() { local b; b=($(popbox)); e2e_text $((b[0] + 1)) $((b[0] + b[2] - 2)) "${b[1]}" | sed 's/[─┌┐]//g; s/^ *//; s/ *$//'; }
pop_text() { local b r; b=($(popbox)); [[ -n ${b[0]} ]] || return 0; for ((r = b[1] + 1; r < b[1] + b[3] - 1; r++)); do e2e_text $((b[0] + 1)) $((b[0] + b[2] - 2)) "$r"; done; }   # the popup's lines
pop_lines() { pop_text | sed -E 's/^ *[0-9]+ ?//; s/ *$//'; }               # the text, without the line numbers
has_line() { pop_lines | grep -qxF -- "$1" || { echo "  popup: $(pop_lines | sed -n 1,14p | tr '\n' '|')"; false; }; }
EDITED=0
toast_has() { e2e_cap | grep -qF -- "$1"; }
jeq() { python3 - "$1" "$2" <<'PY'
import json, sys
sys.exit(0 if json.loads(sys.argv[1]) == json.loads(sys.argv[2]) else 1)
PY
}
cell() { e2e_cap | grep -c . >/dev/null; }

value_body() {
  SOLO=1 start -C "$D/own"; open_table t_val; wait_for 8 settled
  key l; key e; sleep 0.7
  check "e 在 doc 格上：浮窗打开，标题 doc · ${JT}，NORMAL" eval 'pop_open && [[ $(pop_title) == "doc · ${JT}" ]] && mode_is NORMAL'
  check "JSON 打开时格式化成 2 空格缩进（键 a 在前、数组展开）" eval 'has_line "{" && has_line "  \"a\": [" && has_line "    1," && has_line "  \"t\": true" && has_line "}"'
  check "起始光标 1 行，行号栏在（1 { / 2 …）" eval 'pop_text | sed -n 1p | grep -qE "^ *1 \{"'
  key G; key o; e2e_type '"zz": 1'; key Escape; sleep 0.3
  e2e_type ":"; sleep 0.3; e2e_type "q"; key Enter; sleep 0.6
  check ":q 有没写回的改动：浮窗不关，问「放弃修改？」" eval 'pop_open && toast_has "放弃修改"'
  key n; sleep 0.4
  check "回答 n：浮窗还在，改动还在（最后一行 \"zz\": 1）" eval 'pop_open && has_line "\"zz\": 1"'
  e2e_type ":"; sleep 0.3; e2e_type "q"; key Enter; sleep 0.5; key y; sleep 0.5
  check ":q 再问、回答 y：浮窗关了，格子没有修改（没有未保存计数）" eval '! pop_open && ! qb | grep -qE "[0-9]+ 个修改|已修改"'

  # ---- 改成别的 JSON，gq 格式化，:wq 写回并关，C-s 提交，库里是编辑后的值
  key e; sleep 0.7
  key g g; key d G; key i; e2e_type '{"z": [1, 2], "a": {"n": 5}, "s": "新"}'; key Escape; sleep 0.3
  key g q q; sleep 0.5
  check "gq：压成一行的 JSON 格式化成多行（键的顺序照写的：z 在 a 前）" eval 'has_line "{" && has_line "  \"z\": [" && has_line "    \"n\": 5" && has_line "  \"s\": \"新\""'
  e2e_type ":"; sleep 0.3; e2e_type "wq"; key Enter; sleep 0.7
  check ":wq：浮窗关了，格子里是编辑后的值（已修改，截断显示 z 在前）" eval '! pop_open && e2e_plain | grep -q "{↵  \"z\": \[↵"'
  key C-s; sleep 1.2
  check "表格 C-s 之后库里的 doc 是编辑后的值（解析后比较：jsonb 会规范化）" eval 'jeq "$(doc1)" "{\"z\": [1, 2], \"a\": {\"n\": 5}, \"s\": \"新\"}"' || echo "  db doc: $(doc1)"

  # ---- 没改的再开：:q 不问
  key e; sleep 0.7
  check "再 e：打开的是新值（格式化，z 在前，a 在后 PG 规范化后 a 在前亦可）" eval 'pop_open && has_line "{" && pop_lines | grep -qE "^  \"(a|z)\": "'
  e2e_type ":"; sleep 0.3; e2e_type "q"; key Enter; sleep 0.6
  check "没改动 :q：直接关，不问" eval '! pop_open && ! toast_has "放弃修改"'

  # ---- gq{移动}：不论范围都格式化整个值（JSON 是一个文档）
  key e; sleep 0.7
  key g g; key d G; key i; e2e_type '{"k":1,"l":[1,2]}'; key Escape; sleep 0.3
  e2e_type "gq"; sleep 0.2; e2e_type '$'; sleep 0.6
  check "gq$（有移动、范围只是一行的一部分）：整个值格式化成多行" eval 'has_line "{" && has_line "  \"k\": 1," && has_line "  \"l\": ["'
  e2e_type ":"; sleep 0.3; e2e_type "q"; key Enter; sleep 0.5; key y; sleep 0.5
  check "放弃（:q y）：浮窗关了" eval '! pop_open'

  # ---- gq 对非 JSON 不动，toast「不是 JSON」
  key l; key e; sleep 0.7
  check "note 格（纯文本 plain text）：浮窗标题 note · text，内容原样" eval 'pop_open && [[ $(pop_title) == "note · text" ]] && has_line "plain text"'
  key g q q; sleep 0.5
  check "gq 对非 JSON：toast 不是 JSON，文字没动" eval 'toast_has "不是 JSON" && has_line "plain text"'
  # ---- :w 写回不关；之后 :q 不问
  key A; e2e_type "!"; key Escape; sleep 0.3
  e2e_type ":"; sleep 0.3; e2e_type "w"; key Enter; sleep 0.6
  check ":w：浮窗还在，格子里已经是 plain text!（记成这一格的修改）" eval 'pop_open && e2e_plain | grep -q "plain text!"'
  e2e_type ":"; sleep 0.3; e2e_type "q"; key Enter; sleep 0.6
  check ":w 之后 :q：不问，直接关" eval '! pop_open && ! toast_has "放弃修改"'
  key e; sleep 0.7; key A; e2e_type "?"; key Escape; sleep 0.3
  key C-s; sleep 0.7
  check "C-s：写回并关闭，格子是 plain text!?" eval '! pop_open && e2e_plain | grep -q "plain text!?"'
  key r; sleep 0.3

  # ---- 浮窗里的搜索和不跳出
  key e; sleep 0.7
  key G; e2e_type '?'; sleep 0.3; e2e_type 'plain'; key Enter; sleep 0.4
  check "? 向后搜索：光标到 plain 所在行（这个文本只有一行），浮窗还在" eval 'pop_open && has_line "plain text"'
  for k in Space C-p C-l C-h C-j C-k; do key "$k"; done
  check "空格 / C-p / C-l / C-h / C-j / C-k 不会跳出浮窗（还在、焦点没变）" eval 'pop_open && [[ $(pop_title) == "note · text" ]]'
  key g t; key Space; key q; sleep 0.5
  check "gt、SPC q 也不漏到底下：浮窗还在，标题没变" eval 'pop_open && [[ $(pop_title) == "note · text" ]]'
  key Escape; sleep 0.4
  check "搜索高亮亮着时第一下 esc 只清高亮：浮窗还在（M15 F15.9，docs 79fd4e1）" eval 'pop_open'
  key Escape; sleep 0.4
  check "再按 esc（NORMAL，没有改动）：关浮窗，表 tab 还在（gt / SPC q 没有切走或关掉它）" eval '! pop_open && [[ $(tabbar 1) == *t_val* ]]'
}

ro_body() {   # read_only 连接 / 结果 tab：能开，改动的键 toast「只读」，esc / q 关，y 照常
  SOLO=1 start -C "$D/ro"; open_table t_val; wait_for 8 settled
  key l; key e; sleep 0.7
  check "只读连接：e 能开（标题 doc · ${JT}）" eval 'pop_open && [[ $(pop_title) == "doc · ${JT}" ]]'
  key G; key o; sleep 0.4
  check "o（改动）：toast 只读，文字没变（没有 zz）" eval 'toast_has "只读" && ! pop_lines | grep -q zz'
  key x; sleep 0.4
  check "x（删除一个字符）：toast 只读，文字没变（还有第一行 {）" eval 'toast_has "只读" && has_line "{"'
  sleep 3.3; key g g; key y y; sleep 0.5
  check "yy（不改文字的键）照常：没有「只读」toast，浮窗还在" eval 'pop_open && ! toast_has "只读"'
  key Escape; sleep 0.4
  check "esc 关浮窗，不问" eval '! pop_open'
  key e; sleep 0.6; key q; sleep 0.4
  check "q 也关浮窗（只读）" eval '! pop_open'
  _e2e_kill
  # 没有主键的表：进不了编辑，也只读
  SOLO=1 start -C "$D/own"; open_table t_nopk; wait_for 8 settled
  key e; sleep 0.7
  check "没有行标识列的表：e 能开（标题 doc · ${JT}）" eval 'pop_open && [[ $(pop_title) == "doc · ${JT}" ]]'
  key x; sleep 0.4
  check "没有主键的表里 x：toast 只读，文字没变" eval 'toast_has "只读" && has_line "{"'
  key Escape; sleep 0.4
  check "esc 关浮窗" eval '! pop_open'
  _e2e_kill
  # 结果 tab
  start -C "$D/own"; key C-l
  setsql "select * from t_val order by id"; run
  key C-j; sleep 0.5; key l; key e; sleep 0.7
  check "结果 tab：e 能开（标题 doc · ${JT}）" eval 'pop_open && [[ $(pop_title) == "doc · ${JT}" ]]'
  key x; sleep 0.4
  check "结果 tab 里 x：toast 只读，文字没变" eval 'toast_has "只读" && has_line "{"'
  key q; sleep 0.4
  check "q 关浮窗" eval '! pop_open'
}
