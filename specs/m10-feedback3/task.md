# M10 第三轮使用反馈 · 任务清单

- **状态**：**完成**（F10.1–F10.12 passed；最后一个 commit 08d9009 上完整回归 2982 项加 f10.11 全过，2026-10-11；`go install` 2026-10-11 在 v0.2.0 上确认。原文：2026-10-10 用户在 M9 审查期间提出 9 条，决策者拆成 F10.1–F10.8，之后追加 F10.9–F10.12；翻页键改 `<lt>` / `>`、`c` 开 COLS 只是用户自己的 keymaps.toml，不算 feature）。M9 全部 passed（2026-10-10，93e9d8c），可以开工。节点 1 passed（2026-10-11，ae3dd16；e2e 3c23daa，全量 2900 过，18 败全是节点 2 的脚本）。节点 2 passed（2026-10-11，c5d6abe；e2e 508b591，全量 2987 项全过）。节点 3 passed（2026-10-11，08d9009；e2e 6a24c4f）。
- **目标**：表格的多选（标记删除、复制 JSON）、flash 式跳转、树去掉 pane 层、连接分组、WHERE 下拉撑满、新建 session 先要名字、console 结果 500 行并翻页、COLS 调列顺序、侧栏去掉键位文字、删开着的 console 直接关 tab；最后把 console 改名叫 query。
- **范围**：tech-design §6.4（作用域）、§6.8（键位表）、§6.10（跳转，新）、§7.6（表格选择）、§7.8（树、COLS、session 列表、连接表单）、§9.6（WHERE 下拉）、§11（结果翻页）、§14（`group`、`max_rows`）。
- **依赖**：M9 F9.1–F9.10 passed。
- **完成标准**：F10.1–F10.12 passed；tester 在最后一个 commit 上跑完整回归；用户验收通过后打 tag `m10`。
- **审查节点**：
  1. F10.5 之后：F10.1–F10.5、F10.9、F10.10、F10.12 一批（小改：WHERE 下拉、session 名字、COLS 顺序、结果翻页、树去掉 pane 层、侧栏的键位文字、删开着的 console、module 路径）；
  2. F10.8 之后：F10.6–F10.8 一批（连接分组、选择模式、跳转）；
  3. F10.11 之后：F10.11 单独一批（console 改名 query，纯改名，单独审、单独测）。

任务文件的格式和状态约定见 [`../plan.md`](../plan.md)。

---

## F10.1 WHERE 历史 / 收藏下拉撑满 · 状态：passed（ae3dd16；e2e 3c23daa）

用户 2026-10-10：候选框太矮，要显示更多的候选项。现状是和补全列表共用 8 行的上限。

- **依赖**：F3.31
- **涉及**：`internal/ui`（CompleteBox）、`internal/app`

**开发**（§9.6）
- [x] WHERE 下拉的高度：取「data pane 内容区底边放得下的」和「原来的 min(条目数, 8, 窗口剩余)」中的大者，不超过条目数（pane 矮时不比现在少）；CompleteBox 加行数上限参数，补全列表（console、WHERE 的补全）仍传 8。
- [x] 细节（已定）：不加配置项。

**验收**
- [x] 单测：历史 20 条、pane 高 30 行时下拉显示的行数是 pane 能放下的行数，不是 8；补全列表仍是 8。
- [x] golden：撑满的 WHERE 下拉。

## F10.2 新建 session 先要名字 · 状态：passed（ae3dd16；e2e 3c23daa）

用户 2026-10-10：`session.new`（用户映射 `tC`）先弹输入框输入名字，回车新建。window 不用（序号就行，用户定）。

- **依赖**：F8.11
- **涉及**：`internal/app`

**开发**（§7.8「新建 session」）
- [x] `session.new` 和 session 列表里的 `n`：先在状态栏单行输入里要名字（同 `session.rename`），预填 tmux 式的最小未用数字；`↵` 用输入的名字（trim 后为空就用预填的），重名 toast 同重命名、输入框留着；`esc` 什么都不建。名字定了之后照旧后台连接、建 session、attach。

**验收**
- [x] 单测：`session.new` 后出现输入框；输入 `ops` `↵` 建出名为 ops 的 session；空 `↵` 得到数字名；重名 toast；`esc` 不建。
- [x] e2e：`SPC s` `n` 输入名字 `↵`，状态栏 session 块显示新名字。

## F10.3 COLS 里调列顺序 · 状态：passed（ae3dd16；e2e 3c23daa）

用户 2026-10-10：COLS 能改显隐，再加上下移动排显示顺序。按键用 `J` / `K`，不画按钮（用户定）。

- **依赖**：F1.9
- **涉及**：`internal/app`（cols、grid）、`internal/keymap/default.toml`

**开发**（§7.8「COLS」）
- [x] `[keys.cols]` 加 `J` = `cols.move.down`「下移」、`K` = `cols.move.up`「上移」：当前项和相邻项换位，表格的列顺序立刻跟着变，光标仍在原来那一列上。过滤框非空时不动。
- [x] 顺序和隐藏一样按 tab 记，布局文件加 `order_cols`（§14）；刷新后新出现的列排最后，已删除的列去掉；`yy`、`yi`、`Y`、转置视图都按显示顺序。
- [x] 底部提示加 `J/K move`（从 keymap 读）。

**验收**
- [x] 单测：`K` 把第三列移到第二；JSON 的键顺序、INSERT 的列顺序跟着变；过滤中 `J` 不动。
- [x] golden：移动后的表格表头。

## F10.4 console 结果默认 500 行并翻页 · 状态：passed（ae3dd16；e2e 3c23daa）

用户 2026-10-10：1000 太多，500 够了；结果也要像表格一样翻页（DataGrip 的 `501-1,000 / 1,001+`）。

- **依赖**：F3.14、F9.4 的自动 LIMIT（§9.4）
- **涉及**：`internal/app`（result）、`internal/sqlkit`（自动 LIMIT 加 OFFSET）、`internal/config`、`internal/keymap/default.toml`

**开发**（§11、§14）
- [x] `[console] max_rows` 默认 500。
- [x] 翻页只对被自动 LIMIT 的语句生效（顶层没写 LIMIT / FETCH 的读语句）：`[keys.result]` 加 `]` = `result.page.next`、`[` = `result.page.prev`，在结果 tab 所属 console 的连接上重跑同一条语句，追加 `LIMIT max_rows+1 OFFSET 页号×max_rows`（走 §9.4 的追加，不包子查询）；结果替换这个 tab 的内容。下一页只在截断时有效，上一页只在第 2 页起有效，其余按了无事。自己写了 LIMIT 或 OFFSET 的、写语句、快速 SQL 送来的结果没有翻页。翻页只换这一个 tab（结果 tab 记语句序号、offset、是否被自动 LIMIT；把 run 里在 Main 上跑的那段抽成函数复用），兄弟 tab 和固定的 tab 不动，取回期间保留旧内容，完成后写一行日志。
- [x] 标题和日志的行数文字：第 1 页照旧 `500+ 行`；之后 `501–1000+ 行`（还有更多）/ `1001–1203 行`（最后一页）。`R` 重跑回第 1 页。固定的 tab 同样能翻。
- [x] 已知上限（写进 §11）：没有 ORDER BY 时两页可能重叠或漏行，同 DataGrip。

**验收**
- [x] 单测：截断的结果 `]` 发出的 SQL 末尾是 `LIMIT 501 OFFSET 500`；第 1 页 `[` 无事；带 LIMIT 的语句 `]` 无事；行数文字三种格式。
- [x] 集成（PG 和 MySQL）：1203 行的表 `select * from t order by id`，三页的 id 连续不重叠。
- [x] e2e：console 执行、`]` 翻到第 2 页、标题显示 `501–1000+`。

## F10.5 树里去掉 pane 层 · 状态：passed（ae3dd16；e2e 3c23daa）

用户 2026-10-10：工作区里 `pane-x` 这一层价值不高，层级交错不好读。tab 直接挂在 window 下，按 pane 顺序排，每个 tab 标上所属 pane。

- **依赖**：F3.30、F5.15
- **涉及**：`internal/app`（tree）

**开发**（§7.8「工作区节点」）
- [x] window 节点下面直接是 tab 节点：按 pane 编号分组、组内按 tab 顺序；每行 `<带圈编号> <图标> 名字  摘要`，编号同 pane 标题（`① ②`，ascii 下 `⟨1⟩`），`dim` 色；结果区的 tab 也这样列（日志 tab 照旧 `log` 图标）。没有 tab 的 pane 不出现。
- [x] pane 节点的展开 / 折叠、`↵`、单击随之删掉；tab 节点的 `↵`、单击、`focus` 色高亮照旧。命令面板的 pane 范围不变。

**验收**
- [x] 单测（替换 F3.30 的树形期望）：两个 pane 各两个 tab 时工作区是 `▾ data` 下四行，前缀 `①①②②`；空 pane 不列。
- [x] golden：工作区节点。

## F10.6 连接分组 · 状态：passed（c5d6abe；e2e 508b591）

用户 2026-10-10：要按项目或环境区分连接，但不要文件夹多一层；用分割线（像浏览器书签栏的分隔符）。暂不上色。

- **依赖**：F5.1、F8.16
- **涉及**：`internal/config`（connections）、`internal/app`（tree、form、连接管理）、`internal/ui`

**开发**（§7.8「连接节点」「新建连接表单」「连接管理」、§14）
- [x] `connections.toml` 每个 `[[connection]]` 可选 `group = "prod"`；表单和连接管理的修改按 F8.16 的块级改写加 / 删这一行。
- [x] 树的根：没分组的连接按文件顺序排最前面、不画标题；然后每个组一段，组按文件里第一次出现的顺序，组内按文件顺序，段前一行分割线 `── prod ──────`（`sep` 色线、`dim` 色字，宽到侧栏右边）；最后一个连接和 Workspace 之间一条没字的分割线。分割线不是节点：`j` / `k` / `gg` / `G` 跳过，不能点，过滤（`/`）时不画。
- [x] 表单加「分组」字段（name 下一行；图标 `group`，nf-fa-tags，U+F02C）：单行输入，留空就是没分组；有已有组名时提供补全（复用 `complete` 列表，前缀匹配，`Tab` / `↵` 接受）。连接管理的列表画同样的分割线。
- [x] 细节（已定）：组名的字符规则同连接名；组的顺序只能改文件。

**验收**
- [x] 单测：三个连接 `group` 为空、`prod`、`prod` 时树的行序和分割线位置；`j` 跳过分割线；表单保存写出 `group = "prod"`；清空后那一行删掉。
- [x] golden：带两个组的侧栏。
- [x] e2e：新建连接填分组，树里出现在那条分割线下。

## F10.7 表格选择模式 · 状态：passed（c5d6abe；e2e 508b591）

用户 2026-10-10：照 yazi：`v` 进入后 `j` / `k` 扩选，`esc` 确认范围；也要非连续多选（比如 id 1 和 3），选中后 `d` 标记删除（`C-s` 才真删）、`y` 复制选中记录的 JSON；高亮序号列，转置时是顶部的记录号行。

- **依赖**：F3.33（`dd`）、F9.5（`Y`）
- **涉及**：`internal/app`（grid）、`internal/ui`（grid 绘制）、`internal/keymap/default.toml`

**开发**（§6.4 `select` 作用域、§6.8、§7.6「选择」）
- [x] 选择的单位是记录（竖直视图一行，转置视图一列），选中集按 tab 记。`m`（`grid.select.toggle`，标记）切换光标所在记录，不进 VISUAL（不用 yazi 的 `space`：它是 leader，单按要等 `timeoutlen`，`space j` 又是用户的 `SPC j`）。`v` / `V`（`grid.select.visual`）进 VISUAL：锚点是当前记录，移动改变范围，范围内的记录和 `m` 标记的合并算选中；`esc` 或再按 `v` 退出 VISUAL，范围并入选中集（yazi）。模式块显示 VISUAL。
- [x] `select` 作用域：有选中或在 VISUAL 时生效，叠在 `grid` 之上。`d`（`grid.select.delete`）：对选中的记录做 `dd` 的标记删除，全部已标记时全部取消；`y`（`grid.select.yank`）：复制为 JSON 数组（按显示顺序，格式同 `Y`，复制后闪一下序号格）；两者之后清空选中、退出 VISUAL。`esc`（`grid.select.clear`）在 NORMAL 有选中时清空。`d`、`y` 立即执行，不因为下层 `grid` 的 `dd` / `yy` 等 `timeoutlen`：解析器加「完整绑定优先」的作用域标记，只对 `select` 生效（§6.4）。
- [x] 高亮：选中记录的序号格（竖直：左侧行号；转置：顶部记录号）用 `visual` 底；VISUAL 里的范围同样。chip 行的计数后面加 ` · 3 selected`（i18n）。
- [x] 换页、刷新、改 WHERE / ORDER / LIMIT 后清空选中；转置不清。
- [x] 细节（已定）：没有鼠标选择；`o` 换锚点不做。

**验收**
- [x] 单测：`m` 在 1、3 行，`d` 后两行都标删除、选中清空；`v` `2j` `esc` 后三行选中、模式回 NORMAL；`v` `j` `y` 剪贴板是两条记录的 JSON 数组；有选中时 `d` 不等超时；无选中时 `dd` 照旧；转置下 `v` `l` 选两条记录。
- [x] golden：竖直和转置两种高亮。
- [x] e2e：`m` `j` `j` `m` `d` `C-s` 后两行真删。

## F10.8 flash 式跳转 · 状态：passed（c5d6abe；e2e 508b591）

用户 2026-10-10：模拟 flash.nvim（folke）：`s` 后打字，屏幕上每个匹配处出现标签，按标签跳到那里；凡是光标在 NORMAL 下能停的字符都能跳。行为和 flash.nvim 一致。

- **依赖**：F0.6（命中表）
- **涉及**：`internal/app`、`internal/ui`（Frame 的后处理）、`internal/keymap/default.toml`

**开发**（§6.10）
- [x] `s` 在 grid / result / tree / console NORMAL 进 JUMP（`jump.start`；console 里覆盖 vim 的 `s`，`cl` 照旧）。作用域 `jump`：`esc` 退出、`backspace` 删一个字符、`↵` 跳到当前匹配（离光标最近的那个），其余可打印字符追加到模式。模式块显示 JUMP（`info` 色）。
- [x] 匹配：模式非空后，在整个屏幕的可跳区域里找精确子串，smartcase；可跳区域 = 命中表里的单元格（含结果区）、console 的文字、树的节点行；同一个目标只取第一处匹配。
- [x] 标签：字母表 `asdfghjklqwertyuiopzxcvbnm`，去掉任一匹配后面紧跟的那个字符（继续打字不会被当成标签），小写用完用大写；焦点 pane 里的匹配按离光标的距离先分配，其余按屏幕顺序；分不到标签的匹配只高亮。标签画在匹配之后一格（覆盖那个字符），样式同 `yank`（`warn` 底、`bg` 字）；匹配的字符 `match` 色；其余内容前景降成 `dim`。只有一个匹配也不自动跳。
- [x] 按标签：执行那个位置的点击动作（单元格 = 聚焦 pane 并 `grid.goto`；树 = 光标到那个节点，不展开不打开；console = 聚焦并把光标放到那个字符，同单击），退出 JUMP。
- [x] 细节（已定）：不做 operator-pending（`ds…`）和远程操作；不做 treesitter 模式。

**验收**
- [x] 单测：模式 `cm` 时标签不含 `m` 后面紧跟的字符；按标签后光标到对应单元格 / 节点 / 字符；焦点换到那个 pane；`backspace` 到空仍在 JUMP；`esc` 退出且光标不动；smartcase。
- [x] golden：JUMP 中的屏幕（backdrop、匹配、标签）。
- [x] e2e：表格里 `s` `p` `e` 再按一个标签，光标到 `pending` 那格。

## F10.9 侧栏去掉键位文字 · 状态：passed（ae3dd16；e2e 3c23daa）

用户 2026-10-10：折叠后的细栏竖排着 `schema · SPC b`，不好看，去掉；展开时标题右侧的 `SPC b` 换成一个收起图标。能点、能映射就够了，不用更醒目的提示。

- **依赖**：F0.9
- **涉及**：`internal/ui`（侧栏）、`internal/app`

**开发**（§7.8「侧栏」）
- [x] 折叠后的细栏只在顶部画 `»`（可点击，等于 `tree.toggle`），下面不画文字。
- [x] 展开时标题右侧画 `«`（`dim` 色，可点击，等于 `tree.toggle`），代替键位文字 `SPC b`；放不下时先去掉图标再截短 session 名。

**验收**
- [x] golden：折叠和展开两种侧栏（替换原来带 `SPC b` 的场景）。
- [x] 单测：点 `«` 折叠，点 `»` 展开。

## F10.10 删除开着的 console 直接关掉它的 tab · 状态：passed（ae3dd16；e2e 3c23daa）

用户 2026-10-10：树里删 console 时，它开着就 toast「Close its tab first」拒绝；照 DataGrip 不限制，直接删，开着的 tab 自动关掉。

- **依赖**：F8.21、F8.23
- **涉及**：`internal/app`（tree、tab）

**开发**（§7.8「Consoles 节点」）
- [x] `tree.delete` 和右键菜单的「删除…」对开着的 console 文件：确认后删文件和 state.json 的记录，关掉它在所有 session 里的 tab（规则同 F8.23 删表关 tab：没落盘的内容随文件丢弃，不另提示；pane 空了留引导页）；去掉「先关掉它的 tab」的分支和文案。

**验收**
- [x] 单测：两个 session 各开着这个 console，删除后两处 tab 都没了、文件没了、state.json 没有它。
- [x] e2e：树里 `dd` 删开着的 console，`y` 后 tab 消失、没有 toast。

## F10.11 console 改名叫 query · 状态：passed（08d9009；e2e 6a24c4f）

用户 2026-10-10：新版 DataGrip 已经不用 console 这个词，目录叫 queries、文件叫 `Query_1.sql`；等前面的都做完，单独一个 feature 改名、单独测。纯改名，不改任何行为。

- **依赖**：F10.1–F10.10 passed
- **涉及**：全仓库：`internal/app`、`internal/ui`、`internal/config`、`internal/keymap/default.toml`、`internal/i18n`、golden、e2e（tester 改）；spec 由决策者在开工时一次改完

**开发**（tech-design 的用词决策者已在开工前改完：`design/11-console.md` → `11-query.md`，各章 console → query，`consoles` 键 → `queries`；task.md 前面各 feature 的名字是历史，不改）
- [x] 用户看得见的词全部从 console 改成 query：树的 `Consoles` 节点 → `Queries`；tab 和 pane 标题；命令标题（「新建 console」→「新建 query」等）和 Action ID（`console.*` → `query.*`，`console.new` / `console.format` / `console.schema` / `console.conn` / `console.rename` 等）；keymap 作用域 `[keys.console]` → `[keys.query]`、`[map.console.visual]` → `[map.query.visual]`；config `[console] max_rows` → `[query] max_rows`；state.json 和布局文件里的 tab 类型和键；i18n 的中英文文案；toast 和确认框文案；`sqlmux keys` 的输出。
- [x] 文件：目录 `consoles/` → `queries/`，默认文件名 `console_<n>.sql` → `query_<n>.sql`。启动时一次性迁移：没有 `queries/` 而有 `consoles/` 就把目录改名，目录里 `console_<n>.sql` 改成 `query_<n>.sql`（用户改过名的文件不动），state.json 里的键跟着改；布局文件里旧的类型名照新的读。
- [x] 不留旧配置键的别名和兼容读取（`console.*` 动作、`[keys.console]`、`[map.console.*]`、`[console]`、`[icon] console` 都按未知键的现有处理走）：v0.1.0 推送了但没有外部用户（`go install` 都装不了）；用户自己的 keymaps.toml 和 config.toml 由决策者改。`[icon]` 的 `console` 键改 `query`，字形不变。迁移只做目录、文件名、state.json 的键、布局文件里旧的 kind 和 `console_<n>` 名字（读时转换，不回写）；`queries/` 已存在就不动、不合并，目标文件已存在就跳过那一个；「用户改过名的文件不动」按名字前缀 `console_<数字>` 判定。代码里的标识符（`consoleTab` 这类）一并改，commit 说明注明是 rename。
- [x] 细节（已定）：不改 `internal/editor` 的包名和编辑器行为；commit 前缀、e2e 脚本文件名随 tester。

**验收**
- [x] `just check` 全绿，golden 只有文字变化；单测：从只有 `consoles/console_2.sql` 的数据目录启动后，`queries/query_2.sql` 存在、树里 Queries 节点列出它、state.json 里的键是新名字。
- [x] e2e：原有 console 相关脚本改用新词后全过；旧的 `[keys.console]` 配置段启动时报未知作用域（和今天写错段名一样）。

## F10.12 module 路径改成 `github.com/kackerx/sqlmux` · 状态：passed（ae3dd16；e2e 3c23daa）

2026-10-10 推送 v0.1.0 后发现：go.mod 的 module 是 `sqlmux`，`go install github.com/kackerx/sqlmux/cmd/sqlmux@v0.1.0` 装不了，而「`go install` 能装」是 plan.md 里定的发布目标。纯机械改动，放节点 1。

- **依赖**：无
- **涉及**：`go.mod`、全部 Go 文件的 import（281 处 `sqlmux/internal/…`）、README 的安装说明

**开发**
- [x] `go.mod` 的 `module github.com/kackerx/sqlmux`，所有 import 改成 `github.com/kackerx/sqlmux/internal/…`（`gofmt -r` 或 sed 一次改完），`go mod tidy`。
- [x] 细节（已定）：二进制名、配置目录、`sqlmux keys` 等用户可见的东西都不变；README 的「Build」节由决策者加一行 `go install github.com/kackerx/sqlmux/cmd/sqlmux@latest`（下一个 tag 起才有效）。

**验收**
- [x] `just check` 全绿；在仓库外的临时目录 `GOFLAGS=-mod=mod go install github.com/kackerx/sqlmux/cmd/sqlmux@<这个 commit 的 sha>` 能装（决策者 2026-10-11 在 `v0.2.0` 上确认：装出的二进制 `keys --check` 通过；推送后几分钟内 sum.golang.org 还没收录，先用 `GONOSUMDB` 验、之后走 proxy.golang.org 带校验又装了一次）。
