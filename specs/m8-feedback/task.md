# M8 第一轮使用反馈 · 任务清单

- **状态**：**todo**（2026-10-07 用户照 WALKTHROUGH 走完 M5–M7 的验收版后提出 20 条意见，决策者拆成 F8.1–F8.16）。M7 F7.6（默认英文）先做完，再从这里开工。
- **目标**：把用户实际使用时碰到的 bug 修掉，补齐几处配置项和命令，连接可以修改、删除。
- **范围**：tech-design §5（window 名）、§6.5（前缀）、§6.8（新命令）、§7.5（列宽）、§7.8（WHERE 下拉、连接管理）、§9.7（补全）、§14（配置项）。
- **依赖**：M7 F7.6 passed。
- **完成标准**：F8.1–F8.16 passed；tester 在最后一个 commit 上跑完整回归；用户验收通过后打 tag `m8`（M5–M7 的 tag 和 `v0.1.0` 是否在 M8 之前打，等用户定，见 plan.md）。
- **审查节点**：
  1. F8.4 之后：F8.1–F8.4 一批（console 的补全和格式化）；
  2. F8.10 之后：F8.5–F8.10 一批（表格和布局）；
  3. F8.15 之后：F8.11–F8.15 一批（工作区的命令和配置）；
  4. F8.16 之后：F8.16 单独一批（连接管理，改动面大）。

用户自己配置文件里的改动（Cmd+b、`tn`、HJKL、`S` 排序、`SPC wa/wx/wA`、`keyword_case = "upper"`、`tab_width = 4`、默认 desc、圆角边框）由决策者在对应 feature 通过后改 `~/.config/sqlmux/`，不在清单里。

任务文件的格式和状态约定见 [`../plan.md`](../plan.md)。

---

## F8.1 补全列表打开时 `↵` 只接受，列名和表名也补空格 · 状态：in-review（dac2356）

用户：console 里打完 `id` 候选只剩 `id`，按 `↵` 补全之后还换了行；WHERE 里同样的情况会直接执行（F5.12 已经改过一次，没改全）。

- **依赖**：F5.12
- **涉及**：`internal/app`（补全的接受）、`internal/ui`

**开发**（§9.7「接受」）
- [ ] 补全列表开着时，`↵` 一律只接受选中项，不执行、不换行，就算接受后文字一个字都没变（原来「文字没变就当没接受」的分支删掉，连同「跨过自动配对的右引号」那条例外）。想换行或执行，再按一次 `↵`。
- [ ] 接受后补空格的规则改成按「后面自然接什么」：关键字、表名、列名后面接的是下一个词，补一个空格（光标后已经是空白就只移过去）；值（`null` / `true` / `false` / `is null` / `is not null`）、函数（接 `(`）、枚举值（接右引号）、INSERT 列清单里的列（接 `,` / `)`）不补。光标后面紧跟 `)` `,` `;` `.` 时不补（审查时定：`(stat↵` 不能变成 `(status )`）；WHERE 执行和记历史前去掉首尾空白。
- [ ] 不改文字的键（`i` / `a` / `A` / `o`、方向键）不弹列表（§9.7「输入时自动弹出」，审查时发现原来就弹、只是以前无害）；`C-n` 手动唤起列出该位置的全部候选，不因 F8.4 少掉表和关键字。

**验收**
- [ ] 单测：console 里 `id` 候选只有 `id` 时 `↵` → `id |`、没有换行；WHERE 里同样 → 不执行；`count` → `count`；INSERT 列清单里 `id` → `id`。
- [ ] e2e：console 里 `delete from t_event where id` `↵` 后仍是一行、光标在 `id ` 之后；再按 `↵` 才换行。

## F8.2 `;` 结尾的行之后，新行不继承缩进 · 状态：in-review（fc13ef7）

用户：`  id = 1;` 后按 `↵` 或 `o`，新行缩进到了 `id` 的列；语句已经结束，应该回到第一列。

- **依赖**：无
- **涉及**：`internal/editor`

**开发**（§7.9 INSERT 的「`↵` 换行并保持上一行的缩进」）
- [ ] `↵`（INSERT）和 `o` / `O` 开新行时，上一行去掉尾部空白后以 `;` 结尾的，新行缩进为 0；其余照旧继承。`O` 看的是它上面那一行（也就是光标行的上一行）。
- [ ] nvim 本身没有这条规则（它的 sql indent 来自 indentexpr），所以 `cases.txt` 不加这组用例，用编辑器自己的单测。

**验收**
- [ ] 单测：`  id = 1;` 后 `↵`、`o` 都得到缩进 0 的新行；`  id = 1` 后仍继承 2 格。

## F8.3 `console.format` 格式化整个文件，光标不动 · 状态：in-review（6833640；两条都格式化本来就对，只改了光标）

用户：NORMAL 下 `SPC ll` 看起来只格式化了光标所在的语句块，而且光标跳到了块头；预期是整个 console 文件，光标留在原处。VISUAL 下的 `gq` 照旧只格式化选区。

- **依赖**：F5.16
- **涉及**：`internal/app`（`console.format`）

**开发**（§6.8 `console.format`）
- [ ] 先复现：`select … where id = 1;` 和 `delete from t_log;` 两条，光标在第一条上按 `SPC ll`，看第二条有没有被格式化。只处理了一条的话是 bug；两条都处理了（第二条本来就是格式化后的样子）就只改光标。
- [ ] 格式化后光标**留在原来的行号和列号**（超出时夹到文件末尾 / 行尾），不再移到语句开头。§6.8 同步改。

**验收**
- [ ] 单测：两条语句都被格式化；光标行列不变。

## F8.4 需要列名的位置自动弹出列名补全 · 状态：todo

用户：`insert into t (` 后面能弹出全部列很好；`select `、`where `、`delete from t where ` 这些要写列名的地方也想直接看到列名，不用先打一个字母。

- **依赖**：F5.13（INSERT 列清单的做法）、§9.7 的补全上下文
- **涉及**：`internal/sqlkit`（上下文）、`internal/app`（补全的触发）

**开发**（§9.7「空前缀的列名补全」，新增）
- [ ] 光标前一个词是 `select`、`where`、`and`、`or`、`on`、`set`、`having`、`by`（`order by` / `group by`），或者光标在 select 列表、`set` 列表、`order by` 列表里的 `,` 后面，且这条语句里能找到表（`from` / `update` / `delete from` / `insert into` / `join` 的表，含别名），就在打空格之后自动弹出列名列表，前缀为空；候选是这些表的列（多表时带 `表.` 或别名前缀排在各自的列后面，同现在打了字母之后的做法），不含「全部列」项。找不到表就不弹。`esc` 关掉，继续打字就按前缀过滤，和现在一样。
- [ ] WHERE 输入框里同样：光标在 `and ` / `or ` 后面时弹出这张表的列（空输入框不弹，见细节）。
- [ ] 先看 lazysql `components/sql_completer.go` 怎么判断「这里要列名」，不要自己从头猜；本项目已有的上下文解析（§9.7）能复用的就复用。
- [ ] 审查时定：触发词加 `distinct`；`↵` 接受关键字补出的空格不触发（`where↵↵` 要能换行）；快速 SQL 共用同一套、行为一致，tester 也覆盖；`between … and `、MySQL `on duplicate key update ` 记为已知上限。
- [ ] 细节（worker 2026-10-07 提议，已定）：只在光标紧跟空白后面时弹（`select ` 弹，`a,` 不弹，免得打 `a,b` 闪一下、换行要先 esc）；多表沿用打了字母之后的排法（所在层的表在前、外层在后，注释「类型 · 表名」），不新增 `表.列` / `别名.列` 形式的候选；`by` / `on` / `set` 只看前一个词、不分上下文（`partition by`、`on conflict … set` 同样弹，找不到表就不弹）；WHERE 输入框只在 `and ` / `or ` 之后弹，**空输入框不弹**（删空条件按 `↵` 是清除过滤，一弹出来 `↵` 就变成接受列名）。lazysql 的 completer 前缀为空直接返回 nil，没有可借的，在 `sqlkit.CompletionContext` 上加 `WantsColumn`。

**验收**
- [ ] 单测：`select |`、`select distinct |`、`select * from t where |`、`update t set |`、`delete from t where |`、`… where a = 1 and |` 都弹出 t 的列；`select |` 在还没写 `from` 时不弹（找不到表）；`from |` 弹的仍是表名。
- [ ] e2e：console 里打 `delete from t_event where ` 弹出列名；`esc` 关掉后继续打字再弹。

## F8.5 短列不收缩：`[grid] full_width` · 状态：todo

用户：`<null>`、`pending`、`t`、时间这些短值被缩成 `<nu…`、`pendi…`，看不出是什么；长的（json）截断可以。

- **依赖**：F1.3
- **涉及**：`internal/ui`（表格列宽）、`internal/config`

**开发**（§7.5「列宽」）
- [ ] 列宽规则改成：期望宽度（**本页最宽的值**，不再是 p90，审查时定：p90 会把只有一行非空的时间列压到表头宽；不小于表头）不超过 `full_width` 的列不收缩；总宽度超出时只在比它宽的列之间按比例收缩，收到 `full_width` 为止；还放不下就横向滚动，同转置视图现在的做法。`full_width` 默认 **22**（审查时定：放得下 `2026-09-01 00:01:00+00`），写在 config.toml 的 `[grid]` 表里。
- [ ] 转置视图不变。

**验收**
- [ ] golden：t_order 全部列在 160 列宽下，`<null>`、`pending`、`2026-09-01 00:01:00` 完整显示，json 列截断。
- [ ] 单测：列宽分配（都短 / 一长多短 / 全长）。

## F8.6 表格里 `Tab` / `S-Tab` 左右移格 · 状态：todo

用户：新增的黄色行里，只有编辑单元格时 `Tab` 才跳下一格；希望在新增行上任何时候 `Tab` 都跳下一格。

- **依赖**：F3.33
- **涉及**：`internal/keymap/default.toml`

**开发**
- [ ] `[keys.grid]` 加 `<Tab> = grid.right`、`<S-Tab> = grid.left`，所有行都生效（不只新增行：没有理由区分，决策者定）。编辑中的 `[keys.newrow]` 照旧。
- [ ] §6.8 的默认键表同步。

**验收**
- [ ] e2e：新增行上 NORMAL 按 `Tab` 光标右移一格，`S-Tab` 左移；编辑中 `Tab` 照旧提交并右移。

## F8.7 `yi`（复制为 INSERT）没反应 · 状态：todo

用户：`yy`、`yl` 都能用，`yi` 看不到效果。

- **依赖**：F3.32
- **涉及**：`internal/app`

**开发**
- [ ] 根因（worker 2026-10-07 查明）：`grid.yank.insert` 从 F3.32 起就是个没有 Run 的占位，从来没实现过。从头做，细节（worker 提议，已定）：
  - 一行：`insert into "public"."t_order" ("id", "status") values (1, 'done');`，表名和列名走 `Engine.Quote`（PG pgx.Identifier、MySQL 反引号）；关键字（insert into / values / null）跟 `keyword_case`（preserve 当 lower，决策者加：用户配了 upper）。
  - 列：这张表目录里的全部列（不只 COLS 显示的），去掉 `Generated` 的；值取当前显示的，含没保存的修改（同 `yy`）。
  - 字面量：NULL 写 null；数字类型（含 MySQL 的 BIT）不加引号；其余加单引号，`'` 写成 `''`，MySQL 另把 `\` 写成 `\\`（`NO_BACKSLASH_ESCAPES` 下会错，`ponytail:` 标上限）；布尔不特判。转义放 `sqlkit.Literal(s, dialect)`，单测在那里。
  - 只复制当前一行，不支持计数；成功后整行闪一下（同 `yy`）。结果 tab 没有表可写：toast「只有表里的行能复制为 INSERT」，不写剪贴板；已标删除和新增未保存的行照常复制。审查时定：行里有 `<binary N bytes>` 占位的格时 `yi` 不写剪贴板、toast「有二进制列，复制不了」，`yy` → `p` 同样拒绝（F3.32 的老路径，补一个测试）；新增行每格都是 DEFAULT 时写 `insert into t default values;` / MySQL `insert into t () values ();`（db/save.go 已有这两种写法），不能什么都不做。
  - lazysql 的 INSERT 带参数（`drivers/utils.go`），没有可借的转义；PG / MySQL 集成测试各把复制出来的语句原样执行回去一遍。

**验收**
- [ ] e2e：表格上 `yi`，假 pbcopy 收到 `insert into … values (…);`，行闪一下。

## F8.8 默认排序方向和默认转置可配置 · 状态：todo

用户：经常倒序看 id / created_at 的最新数据，想默认 desc；转置也给一个默认值。

- **依赖**：F1.4
- **涉及**：`internal/config`、`internal/app`

**开发**（§14 config.toml）
- [ ] `[grid] order = "asc" | "desc"`（默认 asc）：打开表时默认排序（行标识列）的方向；在 ORDER 下拉里选别的列时第一次也按这个方向。
- [ ] `[grid] transpose = false`：新打开的表 tab 是否转置。
- [ ] 两项都只影响新打开的 tab；布局文件里记的 `desc` / `transpose` 照旧优先；`tab.reopen` 重开的 tab 保留关掉时的转置状态（closedTab 记上 transpose，审查时定）。

**验收**
- [ ] 单测：`order = "desc"` 时打开表的 SQL 带 `order by id desc`，chip 显示 `id ↓`；`transpose = true` 时新 tab 是转置视图。

## F8.9 左右 pane 没有均分 · 状态：todo

用户：左右拆分后两个 pane 宽度明显不等，像是侧栏折叠后的细栏和分隔线被算进了左边。

- **依赖**：F0.4
- **涉及**：`internal/app`（布局树）

**开发**（§5 / §7.1 布局）
- [ ] 代价（审查时确认，接受）：160 列下 ① 从 70 列变成 63 列，查询条少显示一组按钮、note 列移出可见区，200 列下都放得下。
- [ ] worker 2026-10-07 探过：拆分出来的一对本来就均分（侧栏和缝都没算进去，最多差 1 列），不等宽的是默认布局 data : console = 5 : 9（用户截图正是这个两 pane 的默认布局）。决策者定：默认布局改成 1 : 1（§5），拆分规则不动；用户要是指别的情形再说。

**验收**
- [ ] golden：默认布局 data 和 console 等宽（差 ≤ 1 列），侧栏展开 / 折叠各一张。

## F8.10 边框样式：`border = "single" | "rounded" | "double" | "bold" | "hidden"` · 状态：todo

用户：pane 边框想学 lazygit，可以配单线、双线、粗线、圆角。

- **依赖**：F0.4
- **涉及**：`internal/config`、`internal/ui`（Frame 的边框字符）

**开发**（§7.1、§14）
- [ ] config.toml 顶层 `border`，取值同 lazygit 的 `gui.border`：`single`（默认，现在的样子）、`rounded`（四角 `╭ ╮ ╰ ╯`）、`double`、`bold`（粗线，四角是方的：Unicode 没有粗圆角，所以粗和圆角二选一）、`hidden`（不画边框，标题照画）。焦点 / 非焦点只换颜色，不换样式。
- [ ] 侧栏、结果区、浮层（面板、表单、确认框、下拉）都用同一套字符。
- [ ] 不加 `focus_border` 之类的第二个选项（lazygit 也只有一个）。

**验收**
- [ ] golden：`rounded`、`bold`、`double`、`hidden` 各一张默认布局。
- [ ] 单测：写错的值启动报错，报错是英文（F7.6 起读 config 前的文字都是英文）。

## F8.11 window 名不重复、默认名带序号；session 和 console 也能重命名 · 状态：todo

用户：新建的 window 都叫 `data`，状态栏上三个 `data` 分不清；只有 window 能改名，session 和 console 也要能改。

- **依赖**：F5.15
- **涉及**：`internal/app`、`internal/config`（console 文件、布局文件）

**开发**（§5、§6.8、§11）
- [ ] 新 window 的默认名：第一个 `data`，之后 `data-2`、`data-3`……取同一 session 里最小的没用过的序号。`window.rename` 改成同 session 里已有的名字时 toast 拒绝、输入框留着。
- [ ] `session.rename`「重命名 session…」：在任何地方可用（面板、可绑键），用状态栏的单行输入（同 `window.rename`）；名字和已有 session 重复时拒绝。session 列表里的 `$` 照旧。
- [ ] `console.rename`「重命名 console…」：同上；焦点不在 console 上时什么都不做、不 toast（审查时定，同其他只对 console 起作用的命令）。console 的名字就是文件名：`consoles/<名字>.sql`，默认 `console_<n>`，改名就是改文件名（目标已存在时拒绝；名字规则同连接名：不含 `/`、`\`，不以 `.` 开头）；tab、pane 标题、工作区树、面板都显示新名字；state.json 的 `consoles` 键和布局文件里的 console 项改成记名字（`n` 改成 `name`，读到旧的 `n` 当 `console_<n>`，不写迁移代码）。改名后另一个 session 正开着同名文件的情况不会出现（打开时就互斥，§11）。

**验收**
- [ ] 单测：连开三个 window 得 `data`、`data-2`、`data-3`；重命名成已有名字被拒绝；console 改名后文件改名、state.json 键改名、布局文件写 `name`，旧布局的 `n` 能读。
- [ ] e2e：状态栏上三个 window 名字各不相同；`console.rename` 后 tab 名变了，重启后内容还在。

## F8.12 `session.close` 在任何地方可用 · 状态：todo

用户要在 `SPC w…` 下放关 pane（`pane.close`，已有）、关 window（`window.close`，已有）、关 session；关 session 现在只有 session 列表里的 `x`。

- **依赖**：F5.1
- **涉及**：`internal/app`

**开发**（§6.8）
- [ ] `session.close`「关闭 session」：关掉当前 session，确认规则同列表里的 `x`（有未保存的修改或没提交的事务先问，§10.5）；关掉最后一个 session 等于退出。不带默认键。

**验收**
- [ ] 单测：两个 session 时关掉当前的，attach 到另一个；只剩一个时走退出流程。

## F8.13 `palette.table`：直接打开面板的表范围 · 状态：todo

用户要一个键打开「所有连接的全部表」的搜索。面板的表范围本来就列出所有已连上的连接的表（§12），缺的只是直接进这个范围的命令。

- **依赖**：F5.19
- **涉及**：`internal/app`

**开发**
- [ ] `palette.table`「命令面板：表」= 打开面板并输入 `@`，同 `palette.window` / `palette.tab`。不带默认键。

**验收**
- [ ] 单测：执行后面板打开、范围是表、候选含两个连接的表。

## F8.14 WHERE 下拉：历史在上、收藏在下、中间一条线；入口写上键位 · 状态：todo

用户：历史和收藏要用横线分开，历史在上；WHERE 行右端提示打开历史的键。

- **依赖**：F3.31
- **涉及**：`internal/ui`、`internal/app`

**开发**（§9.6「WHERE 的 `C-r` 历史 / 收藏下拉」、§7.8 查询条）
- [ ] 列表顺序改成历史在前、收藏在后，两组之间画一行 `sep` 色的横线（两组都非空时才画）；图标照旧，不画组标题。
- [ ] WHERE 行右端的 `▾` 前面加 `history` 图标和打开下拉的键（从 keymap 读 `where.history` 在 `input` 作用域的键，默认 `C-r`），`dim` 色，如 `󰋚 C-r ▾`；输入框留得下 16 列才显示，放不下时只留 `▾`；键位文字那一段也能点开下拉（worker 定，审查时接受）。
- [ ] 下拉底部一行提示：移动 · 应用 · 收藏 / 取消收藏 · 关闭，键位从 keymap 读（同面板底栏的做法）；已有的话只核对顺序。

**验收**
- [ ] golden：有历史和收藏时的下拉（历史在上、横线、底部提示）；只有历史时没有横线；WHERE 行右端的键位文字。
- [ ] e2e：keymaps.toml 里把 `where.history` 改成别的键，WHERE 行右端显示新键。

## F8.15 既是绑定又是前缀的键：下一个键不匹配时立即执行 · 状态：todo

用户：引导页的 `t`（打开表）和他自己的 `tc` / `tC` / `tt` 并存时，`t` 要等 `timeoutlen` 才生效。nvim 的做法是：后续键不能接成更长的序列时，立刻执行已有的绑定，再处理那个键。

- **依赖**：F0.9
- **涉及**：`internal/keymap`

**开发**（§6.5「序列规则」）
- [ ] 既是完整绑定、又是更长序列前缀的节点：等 `timeoutlen` 超时执行（已有）；或者下一个键接不上任何更长的序列时，先执行已有的绑定，再把这个键当成新序列的开头处理（nvim 行为）。已经是这样的就只补单测。
- [ ] §6.5 同步改，并去掉「默认键位不会产生这种歧义节点」后面的假设：用户的 `t` + `tc` 就是这种节点。

**验收**
- [ ] 单测：`t` 和 `tc` 都绑定时，`t` `x` → 先 `t` 的 Action 再 `x`；`t` `c` → `tc` 的 Action；`t` 等超时 → `t` 的 Action。

## F8.16 连接管理：修改和删除连接 · 状态：todo

用户：现在只能新增，要能改、能删；参考 DataGrip 的数据源面板，左边连接列表、右边属性。

- **依赖**：F7.4（表单）、F7.3（钥匙串）
- **涉及**：`internal/app`、`internal/ui`、`internal/config`（connections.toml 的局部改写）

**开发**（§7.8「连接管理」，新增；§14「新建连接」改成「新建 / 修改 / 删除」）
- [ ] 命令 `conn.manage`「管理连接…」：居中浮层，左边是 connections.toml 顺序的连接列表（图标 + 名字，只读的带 `lock`，当前选中项 `select` 底），右边是 F7.4 的表单，填着选中连接的值（`dsn` 写法的连接 host 等字段从 dsn 拆出来显示；`${VAR}` 和 `password_cmd` 原样显示在高级里，不展开）；`j` / `k` / 点击切换，`Tab` 进表单，`↵` / `C-s` 保存，`esc` 关闭。列表最后一项是 `＋ 新建`，`conn.new` 和侧栏的 `＋` 都打开这个浮层并选中它（表单空白），不再另有一个单独的新建表单。
- [ ] 保存：改写 connections.toml 里这一个 `[[connection]]` 块的 `key = value` 行（改值的改、新加的键加在这一块最后一个键后面、清空的键删掉），文件其他部分（注释、空行、别的块）一个字节不动；密码照 F7.3 走钥匙串。`name` 改了时 state.json 里按连接名记的东西（`recent`、`tables` 的键）跟着改。已连上的连接改了之后不重连，toast「下次连接时生效」。
- [ ] 删除：列表上 `x`（`[keys.conns]` 作用域）或表单下的「删除」按钮，确认框「删除连接 <名字>？」；有 session 正在用它（有它的 tab 或它是启动连接）时拒绝、toast「先关掉用它的 tab」；删掉文件里这一块（它上面紧挨着的注释行一起删）；钥匙串里的密码一并删（`security delete-generic-password` / `secret-tool clear`）。
- [ ] 细节（worker 2026-10-07 提议，已定；lazysql 整文件重写、pgtui 存自己的库，都不能借）：
  - **块的边界**：从 `[[connection]]` 行到下一个以 `[` 开头的行或文件末尾；块上面紧挨着的连续 `#` 行算它的（删除一起删，改值不动）。改值只动 `key = value` 单行，缺的键加在最后一个键行后，清空的删行；块里有多行值、点号键、内联表这类写法时拒绝保存，表单里说「connections.toml 里这一项写法太特别，请手工改」。
  - **`dsn` 写法的连接**（决策者选了小的做法）：host / port / user / database 从 dsn 拆出来只读显示（灰），只能改 `dsn` 一栏和 password / read_only / name；不做「改字段写回 dsn 对应段」（`ponytail:` 标上限：用户要时再做）。新建仍照 F7.4 拼 dsn。
  - **改名**：state.json 里 `recent`、`tables` 的键、`consoles` 的 `conn` 跟着改；layouts/*.toml 和 last.toml 里的 `connection = "旧名"` 不改（恢复时本来就报「没有叫 X 的连接」并跳过），§14 提一句。名字不能和别的连接重复。
  - **钥匙串**：只认我们自己模板的 `password_cmd`（`security find-generic-password -a sqlmux -s …` / `secret-tool lookup sqlmux <name>`），删除时用命令里的服务名删，用户自己写的不碰；改名时我们的项跟着搬到新名字（见下面第 2 条）；表单里 password 非空就按当前名字存一条新的、换掉 password_cmd，旧项在文件写成功后才删（第 3 条的顺序），留空 = 不改密码。
  - **正在用**：有任何 tab 挂在它上面或它是某 session 的启动连接就拒绝删除；只是连着没 tab 的可以删，内存里留到退出。改了已连上的 toast「下次连接时生效」，各 session 的 Spec 换成新读出来的。
  - **界面**：`ui.ConnManager`，宽 100、左 26 列列表、右边复用 `ui.Form` 的字段绘制（拆出来共用，不另写）；新作用域 `[keys.conns]`：`conns.up` / `conns.down` / `conns.form`（`Tab` / `l` / `→` 进表单）/ `conns.delete`（`x`）/ `conns.close`；表单里 `esc` 回列表、再 `esc` 关；F7.4 的独立表单删掉，零连接启动也开这个浮层并选中「＋ 新建」；F7.4 的 golden 重生成。
  - `internal/config` 加 `UpdateConnection(old, new)` / `DeleteConnection(name)`，单测含「块在文件末尾没有换行」。
  - **审查时定（11 条，都照 reviewer 的倾向）**：
    1. 块结束于它最后一个键行加后面的空行，不是下一个表头：后面单独成段的注释（注释掉的旧连接、`# ---- legacy ----`、`[defaults]` 上面的注释）不属于它；紧贴表头上方的 `#` 行仍归这一块，上一块末尾紧贴下一块表头的注释算下一块的。
    2. 改名时，我们模板的 `password_cmd` 跟着搬：用旧命令读出密码、按新名字存一份、改写 `password_cmd`，文件写成功后再删旧项（否则两个连接共用一项，新建同名再删会把密码删掉）。
    3. 顺序：先预检文件（算出新文本、解析通过）→ 存新钥匙串项 → 写文件 → 服务名变了才删旧项；任何一步失败前面的都不做，不会出现「旧项删了文件还指着它」。
    4. 打开面板和切换选中项时重读文件；保存只写表单里相对打开时改过的键；写完把这一块读回来作为 Spec（外部改过的 dsn、加的 password_env 不被盖掉）。
    5. 写盘：EvalSymlinks 找到真实文件，在同目录写临时文件、Chmod 回原权限、rename；文件不可写时拒绝。不丢内容、不改权限（F7.5）、不破坏符号链接。
    6. 不按写法拒绝（点号键、内联表改写结果本来是对的）；改成写完把这一块读回来和要写的连接比对，不一致就拒绝并提示「写法太特别，请手工改」，多行值、带引号的键都被这条挡住。
    7. secret-tool 的模板是 F7.3 实际写的 `secret-tool lookup sqlmux <name>`（spec 原来写错）。
    8. 改名撞上 state.json 里新名字的旧数据（以前删掉的同名连接）：被改名的这个连接的数据为准，recent 去重。
    9. 新建时的 `esc`：从列表走到「＋ 新建」进表单的，`esc` 回列表；从 `conn.new`、侧栏 ＋、零连接启动直接进表单的，`esc` 直接关。
    10. 列表有焦点时 `↵` 也进表单（同 DataGrip），`C-s` 绑 `form.save`、不能落到全局 `save` 把背后表格的修改存了；列表的提示用 `conns` 作用域的键，从 keymap 读。
    11. 已连上的连接改了 `read_only`：建连时的值记在 Conn 上，界面（编辑、拦截、lock 图标）和数据库都等下次连接再变，和 toast「下次连接时生效」一致。

**验收**
- [ ] 单测：改一个连接的 host 后，文件里只有那一行变了，注释都在；删除后那一块和它的注释没了，其他块不动；改名后 state.json 的键跟着改。
- [ ] e2e：打开管理面板，改 seed 的 read_only，保存后文件变了；删除一个没在用的连接，树里消失；删正在用的被拒绝。
