---
paths:
  - e2e/**
---
# tmux 黑盒测试（e2e/）

- **一律通过 `e2e/lib.sh`**：worker 在 main 上、tester 在 e2e worktree 里会同时跑同一套脚本。`lib.sh` 每次运行用独立的 socket `sqlmux-e2e-<pid>`，退出时 kill-server 并删 socket 文件。不要写死 socket 名，否则一方的 kill-server 或 resize 会打到另一方；也不要碰用户自己的 tmux。
- **只编译一次 sqlmux**，各脚本共用（`just e2e`；`lib.sh` 有跳过编译的开关），跑完删掉临时目录里的二进制和日志。全量一轮 40 分钟以上，后台任务 30 分钟会被杀，长跑用 `nohup`。
- **隔离用户数据**：把 `XDG_CONFIG_HOME`、`XDG_STATE_HOME`、`XDG_DATA_HOME` 指向临时目录。剪贴板也一样：`lib.sh` 在临时目录放假的 `pbcopy` / `pbpaste`，放在 sqlmux 进程 `PATH` 最前面；测 OSC 52 后备时把 `PATH` 设成找不到它们。`tmux new-session -e PATH=…` 到不了 pane 里的命令，要在 pane 的 sh 里 `export PATH=…` 并 `unset DISPLAY WAYLAND_DISPLAY`，手动起 tmux 也一样（写法不对会调到真的 pbcopy，覆盖用户剪贴板）。sqlmux 退出后落到的 shell 在临时目录里运行，`HISTFILE=/dev/null`。
- **手动操作时**照 `lib.sh`：`tmux -L sqlmux-e2e-<唯一后缀> -f /dev/null new-session -d -x 160 -y 45 '<命令>'`；`send-keys` 发按键；`send-keys -l $'\e[<0;X;YM\e[<0;X;Ym'` 单击第 X 列第 Y 行（从 1 起）；`capture-pane -p` 读屏幕，要颜色加 `-e`。
- **数据库**：按 AGENTS.md「集成测试环境」直接连主工作区的 PG / MySQL；要锁表或写库就建自己的库 `sqlmux_e2e_<pid>`，退出时 drop。
