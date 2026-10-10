#!/usr/bin/env bash
# F9.6 缩放中的 pane 要看得出来（specs/m9-feedback2/task.md F9.6；tech-design §5「缩放」、§7.1、§7.2）
# 缩放中的 pane：标题的编号后面有 zoom 图标（ascii 时 [Z]，[icon] 可覆盖），边框用主题的 zoom 色（默认 = warn #e0af68，主题里写了就用它），
# 退出缩放恢复。布局由 golden 覆盖，这里在真实终端里看颜色和图标。只读，用共用库。
. "$(dirname "$0")/lib.sh"
e2e_build || exit 1

D=$(mktemp -d "${TMPDIR:-/tmp}/sqlmux-e2e-cfg.XXXXXX"); trap 'e2e_stop; rm -rf "$D"' EXIT
WARN=#e0af68 FOCUS=#9ece6a
zoom() { e2e_keys Space; e2e_type z; sleep 0.8; }
title() { e2e_text 1 60 1; }
hasz() { e2e_text 1 40 1 | python3 -c 'import sys; print(sys.stdin.read().count(chr(0xf065)))'; }   # 标题里缩放图标（默认 U+F065 nf-fa-expand）的个数
conf() { rm -rf "$D/c"; mkdir -p "$D/c/themes"; printf '%b' "$1" >"$D/c/config.toml"; [[ -n $2 ]] && printf '%b' "$2" >"$D/c/themes/x.toml"; true; }

# ---- 默认：warn 色边框，标题里有图标
SOLO= start; open_table t_order; wait_for 8 settled

check "（准备）没缩放：边框是 focus 色，标题里 t_order，没有缩放图标" eval 'style_has 34 1 fg=$FOCUS && [[ $(title) == *"t_order"* && $(hasz) == 0 ]]'
zoom
check "SPC z 缩放：pane 占满整个窗口，边框（四角和边）是 warn 色 #e0af68" eval 'style_has 1 1 fg=$WARN && style_has 1 10 fg=$WARN && style_has 80 1 fg=$WARN && style_has 160 10 fg=$WARN'
check "标题里编号 ① 后面有 zoom 图标（U+F065）" eval '[[ $(hasz) == 1 && $(title) == "┌─ ①"* ]] || { echo "  title=[$(title)]"; false; }'
check "zoom 图标是 warn 色" eval 'z=$(e2e_text 1 12 1 | python3 -c "import sys; print(sys.stdin.read().index(chr(0xf065)) + 1)"); style_has "$z" 1 fg=$WARN'
zoom
check "再 SPC z 退出缩放：边框恢复 focus 色，缩放图标没了，侧栏回来" eval 'style_has 34 1 fg=$FOCUS && [[ $(hasz) == 0 ]] && e2e_plain | sed -n 1p | grep -q "«"'

# ---- icons = "ascii"：[Z]
conf 'icons = "ascii"\n'; SOLO= start -C "$D/c"; open_table t_order; wait_for 8 settled
zoom
check "icons = ascii：缩放时标题里是 [Z]" eval '[[ $(title) == *"[Z]"* ]] || { echo "  [$(title)]"; false; }'
check "（ascii 下边框也是 warn 色）" eval 'style_has 1 1 fg=$WARN'
zoom
check "退出缩放：[Z] 没了" eval '[[ $(title) != *"[Z]"* ]]'

# ---- [icon] 覆盖
conf 'theme = "x"\nicons = "ascii"\n' '[icon]\nzoom = { text = "ZM", fg = "#00ff00" }\n'; SOLO= start -C "$D/c"; open_table t_order; wait_for 8 settled
zoom
check "主题文件的 [icon] zoom = { text = \"ZM\" }：标题里是 ZM" eval '[[ $(title) == *"ZM"* ]] || { echo "  [$(title)]"; false; }'
check "覆盖的图标颜色 #00ff00" eval 'c=$(e2e_find ZM 1 | cut -d" " -f1); style_has "$c" 1 fg=#00ff00'
zoom

# ---- 主题里写了 zoom：边框用它
conf 'theme = "x"\n' 'zoom = "#ff00ff"\n'; SOLO= start -C "$D/c"; open_table t_order; wait_for 8 settled
zoom
check "主题 x 里 zoom = #ff00ff：缩放时边框是 #ff00ff（不是 warn）" eval 'style_has 1 1 fg=#ff00ff && style_has 1 10 fg=#ff00ff'
zoom

# ---- 缩放中的 query / 表 tab 一样；退出缩放的几个途径
SOLO= start; open_table t_order; wait_for 8 settled
zoom; e2e_keys Space; e2e_type %; sleep 0.8
check "缩放中分割：退出缩放（分割、关闭都会退出缩放），边框恢复" eval '! style_has 1 1 fg=$WARN'
e2e_done
