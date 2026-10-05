# M6 配置与持久化 · 任务清单

- **状态**：todo（2026-10-05 决策者按 worker 的草稿定稿）。M5 节点 5 通过后开工；M5 和 M6 一起验收（用户 2026-10-05 定），验收通过后先打 `m5` 再打 `m6`。
- **目标**：完成键位配置、主题和持久化的收尾工作；界面文案支持中英文切换（用户在 M3 期间提出，2026-09-29）；布局文件：退出时保存工作现场、启动时恢复、多个命名布局（用户在 M2 / M3 验收期间提出，2026-09-30）；vim 模式可以关掉（同日提出）。
- **范围**：
  - PRD：第 6 章（6.3）、第 8 章（主题可切换）
  - tech-design：§4（i18n 包）、§6.7、§6.9（非 vim 模式）、§7.3、§14（界面语言、布局文件）
- **依赖**：M5 F5.1–F5.19 全部 passed。
- **已经有的**（worker 起草时清点，清单据此缩短）：state.json 已是原子写入（临时文件 → rename、0600、带版本号）、读一次、坏文件挪开；`sqlmux keys` 已导出全部作用域；主题文件已能加载；`keymap.New` 已返回冲突、只是 `main.go` 丢掉了；代码里搜不到写死的键位文字。
- **完成标准**：
  - F6.1–F6.6 全部 passed；
  - 全量 e2e 回归通过；
  - 用户验收通过后打 tag `m6`。
- **审查节点**（2026-10-05 按 worker 的提议改为四个）：
  1. F6.3 之后：F6.1–F6.3，键位收尾、主题切换、持久化的测试；
  2. F6.4 之后：F6.4 单独一批（改动面广，要扫遍所有包的文案）；
  3. F6.5 之后：F6.5 单独一批（动数据模型和启动）；
  4. F6.6b 之后：F6.6a、F6.6b 一批（非 vim 模式）。

任务文件的格式和状态约定见 [`../plan.md`](../plan.md)。

---

## F6.1 键位配置收尾 · 状态：todo

- **依赖**：F5.9
- **涉及**：`cmd/sqlmux`（把 `keymap.New` 返回的冲突接住）、`internal/app`（启动浮层）、`internal/ui`

**开发**
- [ ] 启动时 keymaps.toml 有冲突（§6.7 的两种）就用浮层列出来，一行一条，`↵` / `esc` 关掉；复用确认框，它能画多行就用它，不行再做一个小浮层。程序照常启动，出问题的行跳过，和 `keymap.New` 现在一样。
- [ ] 「界面上的键位文字都从 keymap 读取」做成单测，不是一次性的搜索：扫 `internal/app`、`internal/ui` 的非测试 Go 文件，字符串字面量里出现 `C-x`、`SPC`、`esc`、`↵`、`Tab` 这类键位写法的报出来，带一张允许名单。已知的一条：console INSERT 下的 `C-n` 写在代码里（`ponytail:`，keymap 没有 console 的 INSERT 作用域），留着、进允许名单。
- 细节（worker 提议，已定）：`ui.Confirm` 改成能画多行文字（每行一行，框跟着长），`No` 为空时只画一个按钮；冲突浮层就是它：标题「keymaps.toml 有 N 处问题，这些行没有生效：」，一行一条 `keymap.Problem.String()`，按钮显示 `confirm.yes` 的键（默认 `y`，同其他确认框，不另加按键取显示文字的接口），`↵` / `y` / `esc` 都关（审查时定）。只在第一帧出现一次、在警告 toast 之前，两者都有就都显示。审计测试用 go/parser 扫非测试文件，规则是字面量里出现 `\b(C|M|S)-[a-z]\b`、`SPC`、`esc`、`↵`、`Tab`，允许名单是「文件:片段 → 理由」，只放真正的例外。

**验收**
- [ ] 单测：keymaps.toml 里写一对冲突，启动后浮层列出它们，关掉后界面正常。
- [ ] e2e：keymaps.toml 里改一个键，pane 标题、tab 栏、命令面板、which-key、状态栏上显示的都是新键。

## F6.2 切换主题 · 状态：todo

- **依赖**：F0.12
- **涉及**：`internal/app`（命令）、`internal/ui`（主题重载）

**开发**
- [ ] 命令 `theme.switch`「切换主题…」：用现有的下拉框列出 `themes/*.toml` 和内置主题，当前的标出来，选中立即重绘。只对这次运行有效，不写任何文件（决策者定：config.toml 只有用户写；写进 state.json 会盖过 config.toml 的 `theme`，用户改了 config 没反应会莫名其妙）。选完 toast 一行「写进 config.toml 的 theme 才会保留」。

**验收**
- [ ] 单测：切换后 token 换成新主题的，缺的 token 沿用 tokyonight-storm。
- [ ] e2e：切换主题后立即重绘；重启后回到 config.toml 的主题。

## F6.3 持久化 · 状态：todo

- **依赖**：F1.5
- **涉及**：`internal/config`（测试）

**开发**
- [ ] 代码已经是 §14 写的样子（原子写、0600、版本号、坏文件挪开），这里只补测试和核对 §14 的清单：最近使用、WHERE 历史与收藏、快速 SQL 历史、console 的连接和 schema。

**验收**
- [ ] 单测：写一轮再读回来，上述内容都在。
- [ ] 单测：起一个子进程写 state.json，在写完临时文件、rename 之前把它杀掉，原文件完好。

## F6.4 界面多语言 · 状态：todo

- **依赖**：F6.1（键位文字的审计先做完，文案和键位文字分得开）
- **涉及**：新包 `internal/i18n`（§4）、所有画界面文字的包（`internal/app`、`internal/ui`、`internal/config`、`cmd`）

**开发**（§14「界面语言」）
- [ ] `internal/i18n`：`T(zh string, args ...any) string`，中文原文是 key，查 `en` 表、查不到用原文，再 `fmt.Sprintf` 填参数；语言是包级变量，启动时设一次，测试里按用例设。不把翻译器传来传去。
- [ ] 存在表里的文案（Action 标题、范围标签、类型标签、状态栏的模式文字）在画的时候翻译，不在定义处翻译；命令面板按翻译后的标题加 action id 搜索。
- [ ] 扫描单测：用 go/ast 过一遍所有包，含汉字的字符串字面量不是 `T` 的参数、不在注释和 `_test.go` 里、不在小的允许名单（数据库错误前缀等）里的，报出来；每个 `T` 的 key 在 `en` 表里都有。
- [ ] 分两个 commit：先机制、扫描测试和 `internal/app` 的文案，再 `internal/ui`、`internal/config`、`cmd`（现在约 350 行中文字面量）。
- [ ] `config.toml` 的 `language = "zh" | "en"`，默认 `zh`，写错启动报错、带路径。
- 细节（worker 提议，已定）：API 是 `T(zh, args...)`、`N(zh)`（同 gettext 的 N_，表里的文案在定义处标、在画的时候 `T(变量)`）、`Set(lang)`；`en` 表是 `internal/i18n/en.go` 里一个按区域分组的 map 字面量，带参数的文案两边动词同序，有测试查；拼接出来的句子改成一条带 `%s` 的 `T`。扫描测试：含汉字的字面量只能出现在 `T(` / `N(` 里，允许名单只有 CLI 的用法说明和 flag 错误，以及读 config.toml / keymaps.toml / 主题本身时的错误（那时还不知道语言，照 zh）；再查 `T` / `N` 的每个 key 都在 `en` 表里、`en` 表的每个 key 都有人用。`sqlmux keys` 的输出和 config 读完之后的启动错误跟着语言。实际合成了一个 commit（a0eda0f）：扫描测试要全部包都转完才能过，临时允许名单反而让 diff 更大；审查时接受，不再拆。

**验收**
- [ ] 单测：扫描测试通过；`en` 表覆盖全部文案。
- [ ] golden：`en` 下的命令面板、确认框、查询条和结果区、引导页各一张。
- [ ] e2e：`language = "en"` 启动，toast、日志都是英文；`language` 写错时启动报错。

## F6.5 布局文件 · 状态：todo

- **依赖**：M5（一个 session 多个连接、多 window）、F6.3
- **涉及**：`internal/app`（工作现场的序列化与恢复）、`internal/config`（布局文件的读写与路径）、`cmd/sqlmux`（`--layout`）

**开发**（§14「布局文件」，2026-10-05 按 worker 的提议定）
- [ ] 格式：TOML，`version = 1`，`[[session]]` → `[[session.window]]`（名字、分割树、焦点、缩放、侧栏、结果区高度）→ `[[session.window.pane]]`（tab 列表和当前 tab；表 tab 记连接、schema、表、WHERE / ORDER / 方向 / LIMIT / PAGE / 隐藏列 / 转置，console tab 记编号、连接、schema）。不存结果集、正在跑的查询、未保存的修改、引导页。
- [ ] 恢复：第一个 session 的启动连接照现在连；其余用到的连接在后台连，连上后表 tab 才取数；连不上的连接整个丢掉它的 tab、一条 toast；没有的表、没了的 console 文件、不存在的连接跳过那一项、一条 toast。console 编号被别的 session 占着时换下一个空号。
- [ ] 保存：`last.toml` 只在正常退出时写，`ponytail:` 标出被杀掉会丢；不管怎么启动，退出都覆盖。
- [ ] 命令 `layout.save`「保存布局为…」（状态栏单行输入要名字，同 `window.rename`；含 `/` 或 `.` 开头拒绝，同名覆盖）、`layout.open`「打开布局」（下拉框列出 `layouts/*.toml` 和 `last`，替换正在跑的全部 session，有未保存的先确认）；`--layout` 走同一段代码。
- 细节（worker 开工时提议，已定，写进 §14）：分割树由程序写成子表、手写内联表也能读；恢复的表 tab 挂起到连接的 catalog 到了再取数；console tab 先建在启动连接上、把布局里的连接和 schema 当作想要的，连上后挪过去；其他 session 的启动连接也在启动时后台连。
- [ ] 命令行：不带参数恢复 `last`；给连接名就默认布局、不恢复；`--layout <名字或路径>`；两个都给报错退出。`last.toml` 读不了：toast、默认布局、改名 `.broken`。

**验收**
- [ ] 单测：序列化再反序列化一个有两个 session、分割、各种 tab 的工作现场，相等；坏文件、缺项的处理。
- [ ] e2e：退出再打开，window、pane、tab、各 tab 的查询条件和 console 都和退出前一样；保存两个命名布局，`--layout` 分别打开；布局里的表被删掉后打开，其余正常恢复、有提示；连接名和 `--layout` 一起给时报错。

## F6.6a 非 vim 模式：普通编辑器 · 状态：todo

- **依赖**：F3.4、F3.39
- **涉及**：`internal/editor`（无模式的一层）、`internal/app`（console、WHERE）、`internal/config`（`vim`）

**开发**（§6.9 第 1 条）
- [ ] `config.toml` 的 `vim = true | false`，默认 `true`。
- [ ] `vim = false` 时 console 和 WHERE 用无模式编辑器：一直可输入，方向键移动，`S-方向键` 和鼠标拖动选择（`visual` 底，再输入替换选区），`C-a` / `C-e` 行首行尾，`C-w` 删词，`Backspace` / `Delete`，`C-z` / `C-y` 撤销重做，`Tab` 插空格，`C-c` 取消选区或关补全。console `↵` 换行；WHERE `↵` 执行、`C-r` 历史、`esc` 回表格。补全同 vim 模式，`esc` 只关列表。状态栏不画模式块。编辑器里没有 NORMAL 上下文，leader、`gt`、`?`、`[map.*]` 不生效。
- [ ] 和 vim 层共用缓冲区、撤销栈、绘制；用普通单测，不做 nvim 差分。

**验收**
- [ ] 单测：输入、选择替换、`C-w`、撤销重做、选区执行的范围。
- [ ] e2e：`vim = false` 下写两条 SQL、用 `S-→` 选一段、`C-e` 执行选区（`C-e` 在 F6.6b 绑，这条验收放到节点 4 一起跑）。

## F6.6b 非 vim 模式：叠加的默认键位 · 状态：todo

- **依赖**：F6.6a、F6.1
- **涉及**：`internal/keymap`（`default-novim.toml`、叠加、`keys` 导出）、`cmd/sqlmux`

**开发**（§6.9 第 2 条）
- [ ] `default-novim.toml`，`vim = false` 时叠在 `default.toml` 上：grid / tree 的方向键；grid 的 `PgUp` / `PgDn` 翻页、`Delete` 标记删除行；console 的 `<CR>` 解绑、`<C-e>` 执行。同一套解析、冲突检查；`sqlmux keys` 按配置导出叠加后的结果。
- [ ] 「只用处处可用的键」的单测对这一层放开 PgUp / PgDn / Delete，其余限制照旧。

**验收**
- [ ] 单测：叠加后 grid 的 `↓` 是 `grid.down`、console 的 `↵` 没有绑定、`C-e` 是 `console.run`；`vim = true` 时这些都不在。
- [ ] e2e：`vim = false` 时，只用方向键、`↵`、`Delete`、`PgDn`、`C-e` 和鼠标，完成浏览、翻页、筛选、编辑保存、写 SQL 并执行，全程不按 vim 键。
