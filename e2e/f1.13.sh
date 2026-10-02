#!/usr/bin/env bash
# F1.13 键位配置的完整导出（specs/m1-browse/task.md F1.13；tech-design §6.7「导出」）；F5.9 起导出的是一份完整的 keymaps.toml：
# 和默认相同的绑定是注释行（值是默认键），用户改过的写成生效的行
# sqlmux keys 是命令行，不需要终端；「有标题的 Action」取自命令面板的命令范围（它列的就是带标题的 Action，
# 只在浮层 / 输入态里用的除外，§12），和导出结果对照。
. "$(dirname "$0")/lib.sh"
e2e_build || exit 1
. "$(dirname "$0")/palette.sh"

D=$(mktemp -d "${TMPDIR:-/tmp}/sqlmux-e2e-keys.XXXXXX")
trap 'e2e_stop; rm -rf "$D"' EXIT
keys() { local c=$1; shift; mkdir -p "$D/$c/sqlmux" "$D/st"; XDG_CONFIG_HOME=$D/$c XDG_STATE_HOME=$D/st XDG_DATA_HOME=$D/st "$E2E_BIN" keys "$@"; }   # CONFIG_DIR ARGS…

# ---- 导出的 toml 原样放进 keymaps.toml：sqlmux keys 的输出与没有配置时完全一致
keys none >"$D/default.md"; keys none --format toml >"$D/export.toml"
mkdir -p "$D/rt/sqlmux"; cp "$D/export.toml" "$D/rt/sqlmux/keymaps.toml"
check "默认键位导出 → 放进 keymaps.toml：--check 通过" keys rt --check
check "… sqlmux keys 的输出与没有配置时完全一致" eval 'keys rt | cmp -s - "$D/default.md" || { keys rt | diff "$D/default.md" - | head; false; }'
check "没有用户配置时：每一条绑定都是注释行（新版本改了默认键不会被旧导出盖住）" eval '! grep -qE "^\"" "$D/export.toml" && grep -qE "^# \"<C-p>\" = \"palette.open\"" "$D/export.toml"'
# 用户改过键（改绑、解绑、[map]）时：改过的写成生效的行，导出的仍是当前生效的键位
mkdir -p "$D/mine/sqlmux"; printf '[keys.grid]\n"x" = ""\n"X" = "tab.close"\n[keys.tree]\n"o" = "tree.open"\n' >"$D/mine/sqlmux/keymaps.toml"
keys mine >"$D/mine.md"; mkdir -p "$D/rt2/sqlmux"; keys mine --format toml >"$D/rt2/sqlmux/keymaps.toml"
check "改过的三行（解绑 x、改绑 X、加 o）在导出里是生效的行" eval 'grep -qx "\"x\" = \"\".*" "$D/rt2/sqlmux/keymaps.toml" && grep -qE "^\"X\" = \"tab.close\"" "$D/rt2/sqlmux/keymaps.toml" && grep -qE "^\"o\" = \"tree.open\"" "$D/rt2/sqlmux/keymaps.toml" || { grep -nE "^\"" "$D/rt2/sqlmux/keymaps.toml"; false; }'
# after_defaults FILE PATTERN：这一行紧跟在默认键的注释行后面，下一行不再是默认键（用户新加的排在作用域的默认之后）
after_defaults() { local n; n=$(grep -n -- "$2" "$1" | head -1 | cut -d: -f1); [[ -n $n && $(sed -n "$((n - 1))p" "$1") == '# "'[!\"]* && $(sed -n "$((n + 1))p" "$1") != '# "'[!\"]* ]] || { echo "  $2 at line $n"; false; }; }
check "位置：解绑的 x 就在默认那一行（# \"x\" = \"tab.close\"）的位置，新加的 X、o 排在各自作用域的默认之后，别的行都不变" eval 'n=$(grep -n "^\"x\" = \"\"" "$D/rt2/sqlmux/keymaps.toml" | cut -d: -f1); [[ $(sed -n "${n}p" "$D/export.toml") == "# \"x\" = \"tab.close\""* && $(diff "$D/export.toml" "$D/rt2/sqlmux/keymaps.toml" | grep -c "^[<>]") == 4 ]] && after_defaults "$D/rt2/sqlmux/keymaps.toml" "^\"X\" = " && after_defaults "$D/rt2/sqlmux/keymaps.toml" "^\"o\" = " || { diff "$D/export.toml" "$D/rt2/sqlmux/keymaps.toml"; false; }'
check "没有写 [map.*] 时导出里没有生效的 [map.*] 表头" eval '! grep -q "^\[map\." "$D/export.toml" "$D/rt2/sqlmux/keymaps.toml"'
check "改绑、解绑之后导出 → 再放进 keymaps.toml：键位与原配置一致（x 仍是解绑的）" eval 'keys rt2 | cmp -s - "$D/mine.md" && ! grep -q "| grid | \`x\` |" "$D/mine.md" || { keys rt2 | diff "$D/mine.md" - | head; false; }'

# ---- 片段的格式：每个作用域一节，节前一行注释；注释掉的 [map.*] 示例
check "每个 [keys.<作用域>] 前一行都是注释" eval 'awk "/^\[keys\./ { if (prev !~ /^# /) bad = bad \$0 \" \" } { prev = \$0 } END { if (bad) { print \"  no comment above: \" bad; exit 1 } }" "$D/export.toml"'
check "附有注释掉的 [map.<上下文>.<模式>] 示例" eval 'grep -qE "^# \[map\.[a-z]+\.[a-z]+\]" "$D/export.toml"'

# ---- 有标题的 Action 都在：有绑定的出现在绑定行里，没有绑定的恰好一行注释
bound=$(grep -E '^(# )?"([^"\\]|\\.)+" = "[a-z_.]+"' "$D/export.toml" | sed -E 's/^(# )?"([^"\\]|\\.)+" = "([a-z_.]+)".*/\3/' | sort -u)   # 默认键是注释行；键可能带转义的引号，如 "<Leader>\""
unbound=$(grep -E '^# "" = "[a-z_.]+"' "$D/export.toml" | sed -E 's/^# "" = "([a-z_.]+)".*/\1/' | sort)
check "没有绑定的 Action 各恰好一行注释，也不出现在任何绑定行里" eval '[[ -n $unbound && -z $(uniq -d <<<"$unbound") && -z $(comm -12 <(echo "$bound") <(echo "$unbound")) ]]'
check "一个 Action 绑了多个键时有多行（palette.up：↑ 和 C-p）" eval '(( $(grep -c "= \"palette.up\"" "$D/export.toml") == 2 ))'
start; pal ">"; sleep 0.3
ids=; last=; same=0
for ((i = 0; i < 150 && same < 3; i++)); do          # 从第一项往下走，读选中行的「所在位置」（action id）
  id=$(selected | awk -F'  ' '{ print $2 }')
  [[ $id == "$last" ]] && same=$((same + 1)) || { same=0; ids="$ids$id"$'\n'; }
  last=$id; e2e_keys Down; sleep 0.12
done
cmds=$(sort -u <<<"$ids" | grep .)
check "命令面板的每个命令（带标题的 Action，共 $(grep -c . <<<"$cmds") 个）都在导出里：绑定行或注释行" eval 'miss=$(comm -23 <(echo "$cmds") <(printf "%s\n%s\n" "$bound" "$unbound" | sort -u)); [[ $(grep -c . <<<"$cmds") -gt 20 && -z $miss ]] || { echo "  missing: $miss"; false; }'

e2e_done
