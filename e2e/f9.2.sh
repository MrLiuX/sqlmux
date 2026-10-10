#!/usr/bin/env bash
# F9.3 / F9.5 表格 ^ 到首列、Y 把当前行复制成 JSON（specs/m9-feedback2/task.md；tech-design §7.5「复制为 JSON」）
# ^ 同 0，用户映射 _ = "^" 也行；Y 复制一个 JSON 对象到（假）系统剪贴板：键按列顺序，数字 / 布尔 / null 是 JSON 的，json / jsonb 是对象不是字符串，
# 时间等是字符串，2 空格缩进、多行，未保存的修改也算，结果 tab 的行也能复制，整行闪一下。只读，用共用库。
. "$(dirname "$0")/lib.sh"
SOLO=1
e2e_build || exit 1

D=$(mktemp -d "${TMPDIR:-/tmp}/sqlmux-e2e-cfg.XXXXXX"); trap 'e2e_stop; rm -rf "$D"' EXIT
pj() { clip | python3 -c "$1"; }                                         # 剪贴板里的 JSON 交给 python 检查，输出 ok 才算过
jok() { [[ $(pj "$1" 2>&1) == ok ]] || { echo "  clip: $(clip | head -c 400)"; false; }; }

# ---- F9.3：^ 到首列
start; open_table t_order; wait_for 8 settled
key '$'
check "（准备）$ 到最后一列（行 1，列 10）" eval 'pos_is 1,10'
key '^'
check "^：回到首列（同 0）" pos_is 1,1
key '$'; key 0
check "0 还是首列" pos_is 1,1
printf '[map.grid.normal]\n_ = "^"\n' >|"$D/k.toml"
start -K "$D/k.toml"; open_table t_order; wait_for 8 settled
key '$'; key _
check "用户映射 [map.grid.normal] _ = \"^\"：_ 同样到首列" pos_is 1,1
key '$'
check "（映射之后 ^ 本身还在）$ 再 ^" eval 'key "^"; pos_is 1,1'

# ---- F9.5：Y 复制当前行为 JSON
start; open_table t_order; wait_for 8 settled
key j; key Y; sleep 0.6
check "Y（id 2 那一行）：剪贴板是合法 JSON 对象，键按列顺序（id user_id status amount paid meta raw note created_at deleted_at）" eval 'jok "import json,sys; d = json.load(sys.stdin); assert list(d) == [\"id\",\"user_id\",\"status\",\"amount\",\"paid\",\"meta\",\"raw\",\"note\",\"created_at\",\"deleted_at\"], list(d); print(\"ok\")"'
check "bigint 是 JSON 数字（id 2、user_id 3），numeric 也是数字（2.99），布尔是 true，NULL 是 null，enum 和时间是字符串" eval 'jok "import json,sys; d = json.load(sys.stdin); assert d[\"id\"] == 2 and type(d[\"id\"]) is int and d[\"user_id\"] == 3; assert d[\"amount\"] == 2.99 and type(d[\"amount\"]) is float; assert d[\"paid\"] is True; assert d[\"note\"] is None and d[\"deleted_at\"] is None; assert d[\"status\"] == \"done\" and isinstance(d[\"created_at\"], str); print(\"ok\")"'
check "jsonb / json 列是对象，不是字符串（meta = {n: 2, tags: [a, b]}，raw = {n: 2}）" eval 'jok "import json,sys; d = json.load(sys.stdin); assert d[\"meta\"] == {\"n\": 2, \"tags\": [\"a\", \"b\"]} and d[\"raw\"] == {\"n\": 2}; print(\"ok\")"'
check "2 空格缩进、多行，不带结尾换行" eval '[[ $(clip | head -2 | tail -1) == "  \"id\": 2," && $(clip | wc -l | tr -d " ") -ge 10 ]]'

# ---- 转置视图里同样是当前记录；未保存的修改也算
key T; sleep 0.5
key Y; sleep 0.5
check "转置视图里 Y：同样是当前记录（id 2）" eval 'jok "import json,sys; d = json.load(sys.stdin); assert d[\"id\"] == 2 and \"meta\" in d; print(\"ok\")"'
key T; sleep 0.4
key '^'; key 7 l; key Enter; sleep 0.4; e2e_type "zz"; key Enter; sleep 0.5
key Y; sleep 0.5
check "未保存的修改也算：note 列（第 8 列，刚改成 zz，没保存）在 JSON 里是屏幕上显示的 zz" eval 'jok "import json,sys; d = json.load(sys.stdin); assert d[\"note\"] == \"zz\", d[\"note\"]; print(\"ok\")"'

# ---- 结果 tab 的行
SOLO= start; key C-l; key i; e2e_type "select 1 as a, true as b, null as c, '{\"x\": [1, 2]}'::jsonb as d, 'hi' as e, 1.50::numeric as f"; key Escape; key Enter; wait_for 8 eval '[[ $(bar) != *busy* ]]'; sleep 1
g=($(geom 3)); e2e_click $((g[0] + 3)) $((g[1] + 3)); sleep 0.5; key Y; sleep 0.6
check "query 的结果 tab 上 Y：{a: 1, b: true, c: null, d: {x: [1, 2]}, e: hi, f: 1.50→数字}" eval 'jok "import json,sys; d = json.load(sys.stdin); assert d == {\"a\": 1, \"b\": True, \"c\": None, \"d\": {\"x\": [1, 2]}, \"e\": \"hi\", \"f\": 1.5}, d; print(\"ok\")"'
e2e_done
