#!/usr/bin/env bash
# F5.17 终端翻译过来的修饰键（specs/m5-workspace/task.md F5.17；tech-design §6.2「终端翻译过来的键」）
# 推荐的送法：tmux 用 send-keys -H 写原始字节 ESC [ <码点十进制> ; <1+修饰> u（shift 1、alt 2、ctrl 4、super 8）。
# <C-S-w> <C-S-[> <C-S-t> <D-w> 都绑成 pane.zoom（看得见：只剩焦点所在的 pane / 恢复；焦点不动，不会把输入框里的光标带走），在表格、树、console 的 NORMAL / INSERT、WHERE 的 INSERT
# 下各按两次，看命令执行了、输入框里没有多出字符；没绑的 <D-x> 在 console 的 INSERT 下不会输入 x。只读，用共用库。
. "$(dirname "$0")/lib.sh"
e2e_build || exit 1
. "$(dirname "$0")/palette.sh"

D=$(mktemp -d "${TMPDIR:-/tmp}/sqlmux-e2e-cfg.XXXXXX"); trap 'e2e_stop; rm -rf "$D"' EXIT
cat >"$D/keymaps.toml" <<'T'
[keys.global]
"<C-S-w>" = "pane.zoom"
"<C-S-[>" = "pane.zoom"
"<C-S-t>" = "pane.zoom"
"<D-w>" = "pane.zoom"
T
sed "s/pane.zoom/tree.toggle/" "$D/keymaps.toml" >"$D/keymaps-tree.toml"   # 树上放大不起作用，树的那一轮改用树的开关
hexs() { printf '%s' "$1" | xxd -p | sed 's/../& /g'; }
csi() { local cp; cp=$(printf '%d' "'$1"); t send-keys -t t -H 1b 5b $(hexs "$cp") 3b $(hexs "$2") 75; sleep 0.4; }   # CHAR MOD：CSI <码点> ; <1+修饰> u
C_S=6 SUPER=9
shown() { [[ $(panes_n) -ge 3 ]]; }
hidden() { [[ $(panes_n) == 1 ]]; }
panes_n() { e2e_panes | awk '$1 != "-"' | grep -c .; }
paste() { t set-buffer -b e2e -- "$1"; t paste-buffer -r -p -b e2e -t t; sleep 0.4; }
cline() { e2e_text 104 159 $(($1 + 1)) | sed 's/ *$//'; }

# trial KEYDESC CHAR MOD：按两次，第一次放大（只剩一个 pane）、第二次恢复；返回是否都对
trial() { csi "$2" "$3"; hidden || { echo "  $1 第一次：没有放大"; return 1; }; csi "$2" "$3"; shown || { echo "  $1 第二次：没有恢复"; return 1; }; }
all_keys() { trial "<C-S-w>" w $C_S && trial "<C-S-[>" '[' $C_S && trial "<C-S-t>" t $C_S && trial "<D-w>" w $SUPER; }

start -K "$D/keymaps.toml"; open_table t_order; settle() { wait_for 8 settled; }; settle
check "（准备）三个 pane，焦点在 ① 表格" eval 'shown && focus_is 1'
check "表格：四个键各按两次，放大又恢复" all_keys

key C-l; key C-l
check "console 的 NORMAL：四个键都执行" eval 'focus_is 2 && mode_is NORMAL && all_keys'
key i; paste "abc"
check "console 的 INSERT：四个键都执行，文字还是 abc，没有字符漏进去" eval 'mode_is INSERT && all_keys && [[ $(cline 1) == "abc" ]] || { echo "  line1: $(cline 1)"; false; }'
csi x $SUPER
check "没绑的 <D-x>：INSERT 下不输入 x，文字还是 abc，没有放大" eval '[[ $(cline 1) == "abc" ]] && shown'
key Escape; key C-h
key /; clear_in; e2e_type "id > 0"
check "WHERE 的 INSERT：四个键都执行，输入框里还是 id > 0，没有多出字符" eval 'mode_is INSERT && all_keys && [[ $(e2e_text 36 95 2 | sed -E "s/ {2,}(([^ ]+) ){0,2}▾ *\$//; s/^ *WHERE *//; s/ *\$//") == "id > 0" ]] || { echo "  WHERE: $(e2e_text 36 95 2)"; false; }'
key Escape

# ---- 焦点在树上：绑成 tree.toggle（收起 / 展开）
e2e_stop; start -K "$D/keymaps-tree.toml"; key C-h
shown() { [[ -n $(geom 0) ]]; }; hidden() { [[ -z $(geom 0) ]]; }
check "（准备）焦点在树上" focus_is 0
check "树（焦点在树上）：四个键都执行（树收起又展开）" all_keys

e2e_done
