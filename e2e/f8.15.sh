#!/usr/bin/env bash
# F8.15 既是绑定又是前缀的键：下一个键接不上更长的序列时，立刻执行已有的绑定（specs/m8-feedback/task.md F8.15；tech-design §6.5「序列规则」）
# 用户的 [keys.landing] "tc" = "palette.window" 和引导页自带的 t（打开表）并存：t 是完整绑定又是 tc 的前缀。
# 序列的单测在 keymap 包里；这里在真实终端里测时序：t 单独按要等 timeoutlen，t 后面接 x 立刻执行 t 再处理 x，t c 是 tc。只读，用共用库。
. "$(dirname "$0")/lib.sh"
e2e_build || exit 1
. "$(dirname "$0")/palette.sh"

D=$(mktemp -d "${TMPDIR:-/tmp}/sqlmux-e2e-cfg.XXXXXX"); trap 'e2e_stop; rm -rf "$D"' EXIT
printf '[keys.landing]\n"tc" = "palette.window"\n' >|"$D/k.toml"
printf 'timeoutlen = 2500\n' >|"$D/config.toml"
opened() { screen_has "命令面板"; }   # 等下一个键的时候会弹出 which-key 小框，不是命令面板

SOLO= start -K "$D/k.toml" -c "$D/config.toml"; sleep 0.5
check "启动时 keymaps.toml 的提示（F6.1）：[keys.landing] t 是 tc 的前缀" eval 'screen_has "[keys.landing] t" && screen_has "tc 的前缀"'
key Enter; sleep 0.3   # 关掉提示（它会吃掉第一个键）
check "（准备）焦点在 ①（空 pane，用引导页的键）" focus_is 1
# t 单独：等 timeoutlen（这里 2.5 秒）
key t; sleep 0.8
check "只按 t：还没到 timeoutlen（2.5 秒），面板没有打开（在等下一个键）" eval '! opened'
sleep 2.2
check "超时之后：t 的 Action 执行了（面板打开，输入框是 @）" eval 'opened && input_is "@"'
key Escape; sleep 0.3
# t 后面接一个接不上的键：立刻执行 t，再处理 x
key t; key z; sleep 0.5
check "t 后立刻按 z（t 没有 tz）：不用等 2.5 秒，t 先执行（面板打开），z 作为新序列的开头，落到面板输入里（@z）" eval 'opened && input_is "@z"'
key Escape; key Escape; sleep 0.3
# t c = tc
key t; key c; sleep 0.5
check "t c：是用户的 tc（palette.window，输入框是 &），不是 t 加 c" eval 'opened && input_is "&"'
key Escape; sleep 0.3
e2e_done
