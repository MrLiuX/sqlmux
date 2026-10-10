# M9 第二轮使用反馈 · 任务清单

- **状态**：**todo**（2026-10-10 用户在 M8 验收期间提出 10 条，决策者拆成 F9.1–F9.9；另外两条只改用户自己的配置：`SPC p` 改成 session 列表、表格默认转置 `[grid] transpose = true`）。M8 全部 passed 之后开工。
- **目标**：WHERE 历史 / 收藏的几处顺手操作，表格的 `^`、`C-r`、复制为 JSON，缩放提示，DDL 的查看（面板预览可滚动、表格里看列 / 表的定义）。
- **范围**：tech-design §7.5（表格键）、§7.8（pane 标题、WHERE）、§9.6（历史 / 收藏下拉）、§12（DDL 预览）。
- **依赖**：M8 F8.1–F8.23 passed。
- **完成标准**：F9.1–F9.8 passed；tester 在最后一个 commit 上跑完整回归；用户验收通过后打 tag `m9`。
- **审查节点**：
  1. F9.5 之后：F9.1–F9.5、F9.9 一批（WHERE 和表格的小改动，加一个面板入口）；
  2. F9.8 之后：F9.6–F9.8 一批（缩放提示、DDL 查看）。

任务文件的格式和状态约定见 [`../plan.md`](../plan.md)。

---

## F9.1 WHERE 下拉：把选中项填进输入框、不执行 · 状态：todo

用户 2026-10-10：历史 / 收藏下拉里 `↵` 直接执行很好，但还要一个好按的键只把它填进输入框、进 INSERT，方便改变量值再执行。

- **依赖**：F3.31
- **涉及**：`internal/app`、`internal/keymap/default.toml`

**开发**（§9.6）
- [ ] `where.edit`「填入输入框」，默认 `C-e`（`[keys.where]`）：关掉下拉，输入框换成选中项的 WHERE 文字（ORDER / LIMIT 不动——收藏项附带的 ORDER / LIMIT 只在 `↵` 应用时才换），INSERT 模式、光标在末尾；不执行。底部提示加上它。

**验收**
- [ ] 单测：`C-e` 后输入框是那条文字、没执行、模式 INSERT、光标在末尾。
- [ ] e2e：历史下拉里 `C-e` → 输入框填入、改一个值、`↵` 执行新条件。

## F9.2 WHERE 输入框里 `C-f` 收藏当前条件 · 状态：todo

用户 2026-10-10：在输入框里直接 `C-f` 收藏，toast 提示。

- **依赖**：F3.31
- **涉及**：`internal/app`、`internal/keymap/default.toml`

**开发**（§9.6）
- [ ] `where.star` 也绑在 WHERE 输入框（`[keys.input]` 的 `<C-f>`，只在 WHERE 里起作用，同 `<C-r>`）：收藏 / 取消收藏输入框里的文字（连同当前 ORDER / LIMIT，同下拉里的规则），toast「已收藏」/「已取消收藏」；输入为空无事。

**验收**
- [ ] 单测：收藏后 state 里有、再按一次没有；toast 文字。
- [ ] e2e：输入条件 `C-f` → toast，`C-r` 打开下拉在收藏组里看到它。

## F9.3 表格 `^` 到首列 · 状态：todo

用户 2026-10-10：`$` 能到行尾，`^` 不能到行首；要能映射 `_`。

- **依赖**：F1.3
- **涉及**：`internal/keymap/default.toml`

**开发**（§7.5）
- [ ] `[keys.grid]` 加 `"^" = "grid.first"`（同 `0`，转置视图里同样是第一条记录）。用户映射 `[map.grid.normal] _ = "^"` 本来就支持，补一个单测。

**验收**
- [ ] 单测：`^` 到首列；`_` 映射到 `^` 后同样。

## F9.4 表格里 `C-r` 直接进 WHERE 并打开历史 · 状态：todo

用户 2026-10-10：焦点在表格而不在输入框时，`C-r` 也要聚焦到 WHERE 并打开历史下拉。

- **依赖**：F3.31
- **涉及**：`internal/app`、`internal/keymap/default.toml`

**开发**（§9.6）
- [ ] `where.history` 在焦点不在 WHERE 时：先进入 WHERE 输入（INSERT，同点击 `▾` 的做法），再打开下拉；`[keys.grid]` 加 `"<C-r>" = "where.history"`。

**验收**
- [ ] e2e：表格上 `C-r` → WHERE 聚焦、下拉打开；`esc` 关下拉回到输入框，再 `esc` 回表格。

## F9.5 `Y` 把当前行复制成 JSON · 状态：todo

用户 2026-10-10：`yy` 整行、`yl` 单元格之外，加 `Y`：当前行记录复制成 JSON 到系统剪贴板，json / jsonb 字段也要序列化成 JSON（不是字符串）。

- **依赖**：F3.32、F8.7
- **涉及**：`internal/app`、`internal/keymap/default.toml`

**开发**（§7.5「复制为 JSON」）
- [ ] `grid.yank.json`「复制为 JSON」，默认 `Y`：一个对象，键是列名、按列顺序（含隐藏的列）；值按列类型：数字类型 → JSON 数字（文本不是合法 JSON 数字时退回字符串），布尔 → `true` / `false`（MySQL 的 `1` / `0` 也转），NULL → `null`，json / jsonb → 原文嵌入，其余（含时间、数组、uuid）→ 字符串；2 空格缩进、多行；值取显示的（含未保存修改），成功后整行闪一下，同 `yy`；结果 tab 的行也能复制（有列类型就行）。转置视图里同样是当前记录。
- [ ] 用 `encoding/json`，不手拼。

**验收**
- [ ] 单测：含 int、bool、null、jsonb、text、timestamptz 的一行 → 期望的 JSON；非法数字文本退回字符串。
- [ ] e2e：`Y` 后假 pbcopy 里是合法 JSON，jsonb 列是对象不是字符串。

## F9.6 缩放中的 pane 要看得出来 · 状态：todo

用户 2026-10-10：`pane.zoom` 放大后容易忘了，以为只开了一个 pane；边框换个颜色或双层提醒一下。

- **依赖**：F0.4
- **涉及**：`internal/ui`（Frame 的 pane 边框）、`internal/app`

**开发**（§5「缩放」、§7.1）
- [ ] 缩放中的 pane：标题的编号后面加 `zoom` 图标（nf-md-arrow_expand_all，ascii `[Z]`，可在 `[icon]` 覆盖），边框用主题的 `zoom` 色（新 token，默认 = `warn`，§7.2 的表里加一行；焦点 / 非焦点都用它）。退出缩放恢复。同 tmux 在 window 名后标 `Z`。

**验收**
- [ ] golden：缩放中的 pane。
- [ ] 单测：`zoom` token 缺省时等于 `warn`。

## F9.7 命令面板的 DDL 预览可滚动 · 状态：todo

用户 2026-10-10：预览区要能像 pane 一样看完整个 DDL（滚动），并且含索引语句，同 DataGrip。

- **依赖**：F4.2
- **涉及**：`internal/app`（面板）、`internal/ui`（预览区）

**开发**（§12「预览」）
- [ ] 预览区加滚动：`palette.preview.down` / `palette.preview.up`，默认 `C-d` / `C-u`（`[keys.palette]`），各滚半页；滚到末尾停住；换了候选回到顶部；底部提示加上。不做「聚焦到预览」：面板里没有别的键和它冲突，滚动就够（决策者定：少一种焦点状态）。
- [ ] 上面还有行时第一行画成 `…`，下面还有时最后一行画成 `…`（现在只有下面的 `…`）。
- [ ] 索引：PG 的 DDL 已含 `pg_get_indexdef` 的索引（§8.4，F4.2），MySQL 的 `SHOW CREATE TABLE` 自带；核对一遍，缺的补上（约束背后的索引不另列）。

**验收**
- [ ] 单测：滚动到底停住、换候选回顶。
- [ ] e2e：长 DDL 的表，`C-d` 两次看到 `create index`，`C-u` 回来。

## F9.8 表格里看列和表的 DDL · 状态：todo

用户 2026-10-10：表格里一个键显示当前字段的定义，另一个键显示整张表的 DDL 含索引，浮窗内部滚动，居中或在当前单元格下方都行；他自己配 `fh` / `fH`。

- **依赖**：F9.7（DDL 文本和它的着色）、F6.1（keyhelp 的滚动浮层）
- **涉及**：`internal/ui`（只读滚动文本浮层）、`internal/app`

**开发**（§7.5「查看 DDL」）
- [ ] `grid.ddl.column`「列的定义」：当前单元格下方（放不下在上方）的小浮窗：第一行 `列名 类型 [not null] [default …]`（从 DDL 里取这一列那一行），下面是含这一列的索引语句、列的注释（有的话）；`esc` / 任意移动键关。
- [ ] `grid.ddl.table`「表的 DDL」：居中浮窗，宽取窗口的 2/3、高取 2/3，内容同面板预览（含索引），着色同面板；作用域 `ddl`：`j` / `k` / `↓` / `↑` 一行、`C-d` / `C-u` 半页、`gg` / `G`、`esc` 关；复用 keyhelp 那套滚动浮层的绘制，不另写。
- [ ] 两个都不带默认键，面板里能用；DDL 从面板预览的同一份缓存取（§12），没取过就先取、浮窗先显示「…」。

**验收**
- [ ] golden：列定义浮窗、表 DDL 浮窗。
- [ ] 单测：列定义浮窗的内容（有 default、有索引、有注释各一例）；浮窗滚动。
- [ ] e2e：keymaps 绑 `fh` / `fH`，表格上按下 → 浮窗，`esc` 关。

## F9.9 `palette.session`：直接打开面板的 session 范围 · 状态：todo

用户 2026-10-10：`SPC p` 想切 session，旧的 session 列表（M5）只有 `j` / `k`、不能搜，面板的 session 范围更合适。

- **依赖**：F5.19、F8.13
- **涉及**：`internal/app`

**开发**（§6.8）
- [ ] `palette.session`「命令面板：session」= 打开面板并输入 `$`，同 `palette.window` / `palette.tab` / `palette.table`。不带默认键。旧的 session 列表（`session.list`，`SPC s`）保留，以后要不要去掉由决策者另定。

**验收**
- [ ] 单测：执行后面板打开、范围是 session。
