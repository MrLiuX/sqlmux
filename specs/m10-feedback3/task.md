# M10 第三轮使用反馈 · 任务清单

- **状态**：**todo**（2026-10-10 用户在 M9 审查期间提出 9 条，决策者拆成 F10.1–F10.8，之后追加 F10.9、F10.10；翻页键改 `<lt>` / `>`、`c` 开 COLS 只是用户自己的 keymaps.toml，不算 feature）。M9 全部 passed 之后开工。
- **目标**：表格的多选（标记删除、复制 JSON）、flash 式跳转、树去掉 pane 层、连接分组、WHERE 下拉撑满、新建 session 先要名字、console 结果 500 行并翻页、COLS 调列顺序、侧栏去掉键位文字、删开着的 console 直接关 tab。
- **范围**：tech-design §6.4（作用域）、§6.8（键位表）、§6.10（跳转，新）、§7.6（表格选择）、§7.8（树、COLS、session 列表、连接表单）、§9.6（WHERE 下拉）、§11（结果翻页）、§14（`group`、`max_rows`）。
- **依赖**：M9 F9.1–F9.10 passed。
- **完成标准**：F10.1–F10.10 passed；tester 在最后一个 commit 上跑完整回归；用户验收通过后打 tag `m10`。
- **审查节点**：
  1. F10.5 之后：F10.1–F10.5、F10.9、F10.10 一批（小改：WHERE 下拉、session 名字、COLS 顺序、结果翻页、树去掉 pane 层、侧栏的键位文字、删开着的 console）；
  2. F10.8 之后：F10.6–F10.8 一批（连接分组、选择模式、跳转）。

任务文件的格式和状态约定见 [`../plan.md`](../plan.md)。

---

## F10.1 WHERE 历史 / 收藏下拉撑满 · 状态：todo

用户 2026-10-10：候选框太矮，要显示更多的候选项。现状是和补全列表共用 8 行的上限。

- **依赖**：F3.31
- **涉及**：`internal/ui`（CompleteBox）、`internal/app`

**开发**（§9.6）
- [ ] WHERE 下拉的高度：从输入框下一行起，往下撑到 data pane 内容区的底边，留出提示行；条目比这还多时滚动。补全列表（console、WHERE 的补全）仍是 8 行。
- [ ] 细节（已定）：不加配置项。

**验收**
- [ ] 单测：历史 20 条、pane 高 30 行时下拉显示的行数是 pane 能放下的行数，不是 8；补全列表仍是 8。
- [ ] golden：撑满的 WHERE 下拉。

## F10.2 新建 session 先要名字 · 状态：todo

用户 2026-10-10：`session.new`（用户映射 `tC`）先弹输入框输入名字，回车新建。window 不用（序号就行，用户定）。

- **依赖**：F8.11
- **涉及**：`internal/app`

**开发**（§7.8「新建 session」）
- [ ] `session.new` 和 session 列表里的 `n`：先在状态栏单行输入里要名字（同 `session.rename`），预填 tmux 式的最小未用数字；`↵` 用输入的名字（trim 后为空就用预填的），重名 toast 同重命名、输入框留着；`esc` 什么都不建。名字定了之后照旧后台连接、建 session、attach。

**验收**
- [ ] 单测：`session.new` 后出现输入框；输入 `ops` `↵` 建出名为 ops 的 session；空 `↵` 得到数字名；重名 toast；`esc` 不建。
- [ ] e2e：`SPC s` `n` 输入名字 `↵`，状态栏 session 块显示新名字。

## F10.3 COLS 里调列顺序 · 状态：todo

用户 2026-10-10：COLS 能改显隐，再加上下移动排显示顺序。按键用 `J` / `K`，不画按钮（用户定）。

- **依赖**：F1.9
- **涉及**：`internal/app`（cols、grid）、`internal/keymap/default.toml`

**开发**（§7.8「COLS」）
- [ ] `[keys.cols]` 加 `J` = `cols.move.down`「下移」、`K` = `cols.move.up`「上移」：当前项和相邻项换位，表格的列顺序立刻跟着变，光标仍在原来那一列上。过滤框非空时不动。
- [ ] 顺序和隐藏一样按 tab 记，和隐藏列用同一处存法；`yy`、`yi`、`Y`、转置视图都按显示顺序。
- [ ] 底部提示加 `J/K move`（从 keymap 读）。

**验收**
- [ ] 单测：`K` 把第三列移到第二；JSON 的键顺序、INSERT 的列顺序跟着变；过滤中 `J` 不动。
- [ ] golden：移动后的表格表头。

## F10.4 console 结果默认 500 行并翻页 · 状态：todo

用户 2026-10-10：1000 太多，500 够了；结果也要像表格一样翻页（DataGrip 的 `501-1,000 / 1,001+`）。

- **依赖**：F3.14、F9.4 的自动 LIMIT（§9.4）
- **涉及**：`internal/app`（result）、`internal/sqlkit`（自动 LIMIT 加 OFFSET）、`internal/config`、`internal/keymap/default.toml`

**开发**（§11、§14）
- [ ] `[console] max_rows` 默认 500。
- [ ] 翻页只对被自动 LIMIT 的语句生效（顶层没写 LIMIT / FETCH 的读语句）：`[keys.result]` 加 `]` = `result.page.next`、`[` = `result.page.prev`，在结果 tab 所属 console 的连接上重跑同一条语句，追加 `LIMIT max_rows+1 OFFSET 页号×max_rows`（走 §9.4 的追加，不包子查询）；结果替换这个 tab 的内容。下一页只在截断时有效，上一页只在第 2 页起有效，其余按了无事。自己写了 LIMIT 的和写语句没有翻页。
- [ ] 标题和日志的行数文字：第 1 页照旧 `500+ 行`；之后 `501–1000+ 行`（还有更多）/ `1001–1203 行`（最后一页）。`R` 重跑回第 1 页。固定的 tab 同样能翻。
- [ ] 已知上限（写进 §11）：没有 ORDER BY 时两页可能重叠或漏行，同 DataGrip。

**验收**
- [ ] 单测：截断的结果 `]` 发出的 SQL 末尾是 `LIMIT 501 OFFSET 500`；第 1 页 `[` 无事；带 LIMIT 的语句 `]` 无事；行数文字三种格式。
- [ ] 集成（PG 和 MySQL）：1203 行的表 `select * from t order by id`，三页的 id 连续不重叠。
- [ ] e2e：console 执行、`]` 翻到第 2 页、标题显示 `501–1000+`。

## F10.5 树里去掉 pane 层 · 状态：todo

用户 2026-10-10：工作区里 `pane-x` 这一层价值不高，层级交错不好读。tab 直接挂在 window 下，按 pane 顺序排，每个 tab 标上所属 pane。

- **依赖**：F3.30、F5.15
- **涉及**：`internal/app`（tree）

**开发**（§7.8「工作区节点」）
- [ ] window 节点下面直接是 tab 节点：按 pane 编号分组、组内按 tab 顺序；每行 `<带圈编号> <图标> 名字  摘要`，编号同 pane 标题（`① ②`，ascii 下 `⟨1⟩`），`dim` 色；结果区的 tab 也这样列（日志 tab 照旧 `log` 图标）。没有 tab 的 pane 不出现。
- [ ] pane 节点的展开 / 折叠、`↵`、单击随之删掉；tab 节点的 `↵`、单击、`focus` 色高亮照旧。命令面板的 pane 范围不变。

**验收**
- [ ] 单测（替换 F3.30 的树形期望）：两个 pane 各两个 tab 时工作区是 `▾ data` 下四行，前缀 `①①②②`；空 pane 不列。
- [ ] golden：工作区节点。

## F10.6 连接分组 · 状态：todo

用户 2026-10-10：要按项目或环境区分连接，但不要文件夹多一层；用分割线（像浏览器书签栏的分隔符）。暂不上色。

- **依赖**：F5.1、F8.16
- **涉及**：`internal/config`（connections）、`internal/app`（tree、form、连接管理）、`internal/ui`

**开发**（§7.8「连接节点」「新建连接表单」「连接管理」、§14）
- [ ] `connections.toml` 每个 `[[connection]]` 可选 `group = "prod"`；表单和连接管理的修改按 F8.16 的块级改写加 / 删这一行。
- [ ] 树的根：没分组的连接按文件顺序排最前面、不画标题；然后每个组一段，组按文件里第一次出现的顺序，组内按文件顺序，段前一行分割线 `── prod ──────`（`sep` 色线、`dim` 色字，宽到侧栏右边）；最后一个连接和 Workspace 之间一条没字的分割线。分割线不是节点：`j` / `k` / `gg` / `G` 跳过，不能点，过滤（`/`）时不画。
- [ ] 表单加「分组」字段（name 下一行；图标 `group`，nf-fa-tags，U+F02C）：单行输入，留空就是没分组；有已有组名时提供补全（复用 `complete` 列表，前缀匹配，`Tab` / `↵` 接受）。连接管理的列表画同样的分割线。
- [ ] 细节（已定）：组名的字符规则同连接名；组的顺序只能改文件。

**验收**
- [ ] 单测：三个连接 `group` 为空、`prod`、`prod` 时树的行序和分割线位置；`j` 跳过分割线；表单保存写出 `group = "prod"`；清空后那一行删掉。
- [ ] golden：带两个组的侧栏。
- [ ] e2e：新建连接填分组，树里出现在那条分割线下。

## F10.7 表格选择模式 · 状态：todo

用户 2026-10-10：照 yazi：`v` 进入后 `j` / `k` 扩选，`esc` 确认范围；也要非连续多选（比如 id 1 和 3），选中后 `d` 标记删除（`C-s` 才真删）、`y` 复制选中记录的 JSON；高亮序号列，转置时是顶部的记录号行。

- **依赖**：F3.33（`dd`）、F9.5（`Y`）
- **涉及**：`internal/app`（grid）、`internal/ui`（grid 绘制）、`internal/keymap/default.toml`

**开发**（§6.4 `select` 作用域、§6.8、§7.6「选择」）
- [ ] 选择的单位是记录（竖直视图一行，转置视图一列），选中集按 tab 记。`space`（`grid.select.toggle`）切换光标所在记录，不进 VISUAL。`v` / `V`（`grid.select.visual`）进 VISUAL：锚点是当前记录，移动改变范围，范围内的记录和 `space` 选的合并算选中；`esc` 或再按 `v` 退出 VISUAL，范围并入选中集（yazi）。模式块显示 VISUAL。
- [ ] `select` 作用域：有选中或在 VISUAL 时生效，叠在 `grid` 之上。`d`（`grid.select.delete`）：对选中的记录做 `dd` 的标记删除，全部已标记时全部取消；`y`（`grid.select.yank`）：复制为 JSON 数组（按显示顺序，格式同 `Y`，复制后闪一下序号格）；两者之后清空选中、退出 VISUAL。`esc`（`grid.select.clear`）在 NORMAL 有选中时清空。`d`、`y` 立即执行，不因为下层 `grid` 的 `dd` / `yy` 等 `timeoutlen`（上层的完整绑定优先）。
- [ ] 高亮：选中记录的序号格（竖直：左侧行号；转置：顶部记录号）用 `visual` 底；VISUAL 里的范围同样。chip 行的计数后面加 ` · 3 selected`（i18n）。
- [ ] 换页、刷新、改 WHERE / ORDER / LIMIT 后清空选中；转置不清。
- [ ] 细节（已定）：没有鼠标选择；`o` 换锚点不做。

**验收**
- [ ] 单测：`space` 在 1、3 行，`d` 后两行都标删除、选中清空；`v` `2j` `esc` 后三行选中、模式回 NORMAL；`v` `j` `y` 剪贴板是两条记录的 JSON 数组；有选中时 `d` 不等超时；无选中时 `dd` 照旧；转置下 `v` `l` 选两条记录。
- [ ] golden：竖直和转置两种高亮。
- [ ] e2e：`space` `j` `j` `space` `d` `C-s` 后两行真删。

## F10.8 flash 式跳转 · 状态：todo

用户 2026-10-10：模拟 flash.nvim（folke）：`s` 后打字，屏幕上每个匹配处出现标签，按标签跳到那里；凡是光标在 NORMAL 下能停的字符都能跳。行为和 flash.nvim 一致。

- **依赖**：F0.6（命中表）
- **涉及**：`internal/app`、`internal/ui`（Frame 的后处理）、`internal/keymap/default.toml`

**开发**（§6.10）
- [ ] `s` 在 grid / result / tree / console NORMAL 进 JUMP（`jump.start`；console 里覆盖 vim 的 `s`，`cl` 照旧）。作用域 `jump`：`esc` 退出、`backspace` 删一个字符、`↵` 跳到当前匹配（离光标最近的那个），其余可打印字符追加到模式。模式块显示 JUMP（`info` 色）。
- [ ] 匹配：模式非空后，在整个屏幕的可跳区域里找精确子串，smartcase；可跳区域 = 命中表里的单元格（含结果区）、console 的文字、树的节点行；同一个目标只取第一处匹配。
- [ ] 标签：字母表 `asdfghjklqwertyuiopzxcvbnm`，去掉任一匹配后面紧跟的那个字符（继续打字不会被当成标签），小写用完用大写；焦点 pane 里的匹配按离光标的距离先分配，其余按屏幕顺序；分不到标签的匹配只高亮。标签画在匹配之后一格（覆盖那个字符），样式同 `yank`（`warn` 底、`bg` 字）；匹配的字符 `match` 色；其余内容前景降成 `dim`。只有一个匹配也不自动跳。
- [ ] 按标签：执行那个位置的点击动作（单元格 = 聚焦 pane 并 `grid.goto`；树 = 光标到那个节点，不展开不打开；console = 聚焦并把光标放到那个字符，同单击），退出 JUMP。
- [ ] 细节（已定）：不做 operator-pending（`ds…`）和远程操作；不做 treesitter 模式。

**验收**
- [ ] 单测：模式 `cm` 时标签不含 `m` 后面紧跟的字符；按标签后光标到对应单元格 / 节点 / 字符；焦点换到那个 pane；`backspace` 到空仍在 JUMP；`esc` 退出且光标不动；smartcase。
- [ ] golden：JUMP 中的屏幕（backdrop、匹配、标签）。
- [ ] e2e：表格里 `s` `p` `e` 再按一个标签，光标到 `pending` 那格。

## F10.9 侧栏去掉键位文字 · 状态：todo

用户 2026-10-10：折叠后的细栏竖排着 `schema · SPC b`，不好看，去掉；展开时标题右侧的 `SPC b` 换成一个收起图标。能点、能映射就够了，不用更醒目的提示。

- **依赖**：F0.9
- **涉及**：`internal/ui`（侧栏）、`internal/app`

**开发**（§7.8「侧栏」）
- [ ] 折叠后的细栏只在顶部画 `»`（可点击，等于 `tree.toggle`），下面不画文字。
- [ ] 展开时标题右侧画 `«`（`dim` 色，可点击，等于 `tree.toggle`），代替键位文字 `SPC b`；放不下时先去掉图标再截短 session 名。

**验收**
- [ ] golden：折叠和展开两种侧栏（替换原来带 `SPC b` 的场景）。
- [ ] 单测：点 `«` 折叠，点 `»` 展开。

## F10.10 删除开着的 console 直接关掉它的 tab · 状态：todo

用户 2026-10-10：树里删 console 时，它开着就 toast「Close its tab first」拒绝；照 DataGrip 不限制，直接删，开着的 tab 自动关掉。

- **依赖**：F8.21、F8.23
- **涉及**：`internal/app`（tree、tab）

**开发**（§7.8「Consoles 节点」）
- [ ] `tree.delete` 和右键菜单的「删除…」对开着的 console 文件：确认后删文件和 state.json 的记录，关掉它在所有 session 里的 tab（规则同 F8.23 删表关 tab：没落盘的内容随文件丢弃，不另提示；pane 空了留引导页）；去掉「先关掉它的 tab」的分支和文案。

**验收**
- [ ] 单测：两个 session 各开着这个 console，删除后两处 tab 都没了、文件没了、state.json 没有它。
- [ ] e2e：树里 `dd` 删开着的 console，`y` 后 tab 消失、没有 toast。
