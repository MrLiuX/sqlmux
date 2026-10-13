#!/usr/bin/env bash
# F10.6 连接分组：改、清空、删（specs/m10-feedback3/task.md F10.6；tech-design §7.8「分组」「连接管理」、§14）
# 连接管理里改 group：文件里只有那一行变（块级改写，别的块和注释一个字节不动），树里立刻重排；清空 group 删掉那一行；删掉一个组的第一个连接后组的顺序
# 按文件更新；一个连接都没分组时一条分割线都不画；表单的 group 补全：焦点刚进字段不弹，打了字有匹配才弹（不分大小写），弹着时 C-p 照样开面板。
# 画法由 golden 覆盖。用不碰库的连接（只连启动的那条）。
. "$(dirname "$0")/lib.sh"
e2e_build || exit 1
. "$(dirname "$0")/palette.sh"

D=$(mktemp -d "${TMPDIR:-/tmp}/sqlmux-e2e-cfg.XXXXXX"); trap 'e2e_stop; rm -rf "$D"' EXIT
mkdir -p "$D/c"
{ printf '# my conns\n'; for spec in "plain -" "p1 prod" "s1 stage" "p2 prod"; do set -- $spec
    printf '[[connection]]\nname = "%s"\nengine = "postgres"\ndsn = "%s"\n' "$1" "$SQLMUX_TEST_PG"; [[ $2 != - ]] && printf 'group = "%s"\n' "$2"; printf '\n'; done; } >"$D/c/connections.toml"
chmod 600 "$D/c/connections.toml"; cp "$D/c/connections.toml" "$D/orig.toml"
SELECT=#364a82
roots() { local y; for y in $(seq 4 41); do e2e_text 1 33 "$y"; done | python3 -c '
import re, sys
out = []
for l in re.sub(r"[-]", "", sys.stdin.read()).split("\n"):
    if "工作区" in l: break
    body = re.sub(r"^│|│\s*$", "", l).strip()
    m = re.match(r"^│ [▸▾]\s+(\S+)", l)
    if body.startswith("──"):
        name = body.strip("─ ")
        out.append("── " + name if name else "──")
    elif m:
        out.append(m[1])
print(",".join(out))'; }
roots_is() { [[ $(roots) == "$1" ]] || { echo "  roots: '$(roots)'"; echo "  want:  '$1'"; false; }; }
ct() { echo "$E2E_TMP/config/sqlmux/connections.toml"; }
box() { e2e_panes | awk '$1 == "-" { print $2, $3, $4, $5; exit }'; }
frows() { local g y; g=($(box)); for ((y = g[1] + 1; y < g[1] + g[3] - 1; y++)); do e2e_text $((g[0] + 28)) $((g[0] + g[2] - 2)) "$y" | noicon | sed 's/^ *//; s/ *$//'; done; }
fval() { frows | awk -v k="$1" '$1 == k { if (NF == 1) print ""; else { sub(/^[^ ]+ /, ""); print }; exit }'; }
lrows() { local g y; g=($(box)); for ((y = g[1] + 1; y < g[1] + g[3] - 1; y++)); do e2e_text $((g[0] + 2)) $((g[0] + 26)) "$y" | noicon | sed 's/^ *//; s/ *$//'; done; }
lsel() { local g y; g=($(box)); for ((y = g[1] + 1; y < g[1] + g[3] - 1; y++)); do style_has $((g[0] + 20)) "$y" bg=$SELECT >/dev/null && { e2e_text $((g[0] + 2)) $((g[0] + 26)) "$y" | noicon | sed 's/^ *//; s/ *$//'; return; }; done; }
cmdp() { pal ">$1"; key Enter; sleep 0.7; }
clr() { local i; for ((i = 0; i < 40; i++)); do e2e_keys BSpace; done; sleep 0.2; }
edit_group() { key Tab; key Tab; clr; [[ -n $1 ]] && e2e_type "$1"; key C-s; sleep 1.2; }   # 焦点在列表上选好了连接：进表单、改 group、保存

SOLO= start -C "$D/c" -S "$D/st"; sleep 0.5
check "树：plain 在最前，prod 段（p1、p2），stage 段（s1），最后一条线" roots_is "plain,── prod,p1,p2,── stage,s1,──"
cmdp conn.manage
check "连接管理的列表：同样的分割线（prod、stage 和「新建」上面一条）" eval 'l=$(lrows | sed "/^$/d" | tr "\n" ","); [[ $l == plain,*──\ prod*,p1,p2,*──\ stage*,s1,*──*,新建, ]] || { echo "  list: $l"; false; }'
key j
check "j：跳过分割线，选中 p1，表单里 group 是 prod" eval '[[ $(lsel) == p1 && $(fval group) == prod ]]'

# ---- 改 group：p1 prod → stage（p1 在文件里排第二，stage 的第一次出现在 p1，所以 stage 段挪到 prod 前面）
edit_group stage
check "保存：文件里只有 p1 的 group 那一行变成 stage，别的一个字节不动" eval 'python3 - "$D/orig.toml" "$(ct)" <<PY
import sys
a = open(sys.argv[1]).read().replace("name = \"p1\"\nengine = \"postgres\"\ndsn = \"" + __import__("os").environ["SQLMUX_TEST_PG"] + "\"\ngroup = \"prod\"", "name = \"p1\"\nengine = \"postgres\"\ndsn = \"" + __import__("os").environ["SQLMUX_TEST_PG"] + "\"\ngroup = \"stage\"")
sys.exit(0 if a == open(sys.argv[2]).read() else 1)
PY'
key Escape; sleep 0.5
e2e_plain | grep -qF "┌─ 连接 ─" && { key Escape; sleep 0.5; }   # M15 F15.1: the longer tree shows the dialog still closing under load
check "关掉管理：树里 stage 段（p1、s1）在 prod 段（p2）前面（组的顺序按文件第一次出现）" roots_is "plain,── stage,p1,s1,── prod,p2,──"

# ---- 清空 group：删掉那一行
cmdp conn.manage; key j; key j                                              # plain → p1 → s1
check "（准备）选中 s1，group 是 stage" eval '[[ $(lsel) == s1 && $(fval group) == stage ]]'
edit_group ""
check "清空 group 保存：s1 那一块没有 group 行了" eval '! grep -A5 "name = \"s1\"" "$(ct)" | head -5 | grep -q "^group"'
key Escape; sleep 0.5
check "树里 s1 回到没分组的那段（plain 后面）" roots_is "plain,s1,── stage,p1,── prod,p2,──"

# ---- 删掉一个组的第一个连接：prod 里只剩 p2，组的顺序按文件
cmdp conn.manage; key j; key j; key j                                       # plain → s1 → p1 → p2
check "（准备）选中 p2，group 是 prod" eval '[[ $(lsel) == p2 ]]'
key Escape; sleep 0.3; key Escape; sleep 0.3
cmdp conn.manage; key j; key j                                              # plain → s1 → p1
key x; sleep 0.4; key y; sleep 1; key Escape; sleep 0.5
check "删 p1（stage 的唯一成员、在文件里排 p1、s1 之后）：stage 段没了，树里 plain、s1、prod 段（p2）" roots_is "plain,s1,── prod,p2,──"

# ---- 一个都没分组：一条分割线都不画
cmdp conn.manage; key j; key j                                              # plain → s1 → p2
edit_group ""; key Escape; sleep 0.5
check "清空最后一个 group：树里一条分割线都没有（含「工作区」前的那条）" roots_is "plain,s1,p2"
cmdp conn.manage
check "连接管理的列表也没有分割线" eval '! lrows | grep -q "──"'
key Escape; sleep 0.3

# ---- 表单的 group 补全
{ printf '[[connection]]\nname = "plain"\nengine = "postgres"\ndsn = "%s"\n\n[[connection]]\nname = "p1"\nengine = "postgres"\ndsn = "%s"\ngroup = "prod"\n\n[[connection]]\nname = "s1"\nengine = "postgres"\ndsn = "%s"\ngroup = "Prod-eu"\n' "$SQLMUX_TEST_PG" "$SQLMUX_TEST_PG" "$SQLMUX_TEST_PG"; } >"$D/c/connections.toml"
e2e_stop; SOLO= start -C "$D/c" -S "$D/st2"; sleep 0.5
cmdp conn.new; e2e_type neo; key Tab; sleep 0.5
check "焦点刚进 group：不弹列表" eval '! e2e_plain | grep -qE "│ (prod|Prod-eu) *│"'
e2e_keys C-p; sleep 0.5
check "焦点刚进 group（列表没弹）：C-p 开命令面板，没有被列表截走" palette_open
key Escape; sleep 0.3
e2e_type PR; sleep 0.5
check "打了 PR（不分大小写）：弹出 prod 和 Prod-eu" eval 'e2e_plain | grep -qE "│ prod *│" && e2e_plain | grep -qE "│ Prod-eu *│"'
key Tab; sleep 0.3
check "Tab 接受第一项：group 变成 prod" eval '[[ $(fval group) == prod ]]'
e2e_done
