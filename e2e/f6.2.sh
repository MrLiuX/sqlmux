#!/usr/bin/env bash
# F6.2 切换主题（specs/m6-config/task.md F6.2；tech-design §7.3）
# theme.switch 在命令面板里执行，下拉框列出内置主题和 themes/*.toml，当前的在选中位置；选中立即重绘，只对这次运行有效、不写文件，
# toast「写进 config.toml 的 theme 才会保留」；重启回到 config.toml 的主题。token 的合并有单测。只读，用共用库。
. "$(dirname "$0")/lib.sh"
e2e_build || exit 1
. "$(dirname "$0")/palette.sh"

D=$(mktemp -d "${TMPDIR:-/tmp}/sqlmux-e2e-cfg.XXXXXX"); trap 'e2e_stop; rm -rf "$D"' EXIT
mkdir -p "$D/c/themes" "$D/plain"
printf 'pane_bg = "#101010"\nrow = "#202020"\n' >"$D/c/themes/x.toml"; printf 'theme = "x"\n' >"$D/c/config.toml"
printf 'pane_bg = "#303a52"\n' >"$D/c/themes/y.toml"
bg() { e2e_style 40 20 | grep -oE 'bg=#[0-9a-f]+' | cut -c4-; }          # ① 里一个空白格的背景
dd_rows() { local b; b=($(e2e_panes | awk '$1 == "-" { print $2, $3, $4, $5; exit }')); local y
  for ((y = b[1] + 1; y < b[1] + b[3] - 1; y++)); do printf '%s|%s\n' "$(e2e_text $((b[0] + 2)) $((b[0] + b[2] - 3)) $y | sed 's/ *$//')" "$(e2e_style $((b[0] + 3)) $y | grep -oE 'bg=#[0-9a-f]+' | cut -c4-)"; done; }
switch() { e2e_keys C-p; sleep 0.3; e2e_type ">切换主题"; sleep 0.5; key Enter; sleep 0.5; }

# 默认主题的背景：不带配置启动时 ① 里同一格的颜色
start; open_table t_order; DEFAULT_BG=$(bg); e2e_stop

start -C "$D/c"; open_table t_order
check "（准备）config.toml 的 theme = x：背景 #101010" eval '[[ $(bg) == "#101010" ]]'
before=$(cat "$D/c/config.toml")
switch
check "命令「切换主题…」打开下拉框，列出内置的 tokyonight-storm 和 themes 里的 x、y，当前的 x 选中" eval 'r=$(dd_rows | cut -d"|" -f1 | tr "\n" ","); [[ $r == *tokyonight-stor* && $r == *x,* && $r == *y* ]] && [[ $(dd_rows | awk -F"|" "\$2 == \"#364a82\" { print \$1 }") == x ]] || { dd_rows; false; }'
key Down; key Enter; sleep 0.6
check "选中另一个（y）：立即重绘，背景变成 y 的 #303a52" eval '[[ $(bg) == "#303a52" ]]'
check "toast：写进 config.toml 的 theme 才会保留" eval 'screen_has "写进 config.toml 的 theme 才会保留"'
switch; key Up; key Up; key Enter; sleep 0.6
check "再切到内置的 tokyonight-storm：背景是默认主题的（缺的 token 沿用它）" eval '[[ $(bg) == "$DEFAULT_BG" ]] || { echo "  $(bg) vs $DEFAULT_BG"; false; }'
check "不写任何文件：config.toml 没变，state.json 里没有主题" eval '[[ $(cat "$D/c/config.toml") == "$before" ]] || { echo "  config changed"; false; }; ! grep -rq "\"theme\"" "$E2E_TMP/state" || { echo "  state has theme"; false; }'
e2e_stop; start -C "$D/c"; open_table t_order
check "重启：回到 config.toml 的主题 x（#101010）" eval '[[ $(bg) == "#101010" ]]'

e2e_done
