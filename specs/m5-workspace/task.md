# M5 工作现场与 MySQL · 任务清单

- **状态**：M2–M4 验收通过（2026-10-02）。先做 M4 验收意见 F5.5–F5.9（todo，可以开工）；F5.1–F5.4 仍是 draft：§5「一个 session 多个连接」已重写（2026-10-02，用户定），worker 按它补 F5.1–F5.4 的开发清单（可以重新拆分 feature），一并列出 §8.2、§8.6、§11、§12、§13、§14 要改的条目，决策者确认后改为 todo。
- **目标**：
  - 支持多个 session、多个 window；
  - 支持只读 session 和两种事务模式；
  - 支持 MySQL。
- **范围**：
  - PRD：S-01~S-04、W-01~W-02、第 9 章中的 MySQL
  - tech-design：§8（MySQL 部分）、§13
- **依赖**：M1–M4 全部 passed。
- **设计变更**（M2/M3 验收时用户提出，2026-10-02 定，已写进 §5「一个 session 多个连接」）：
  - session 变成工作区，每个 session 能看到 connections.toml 里的全部连接，目录树的根是这些连接，用到时才连；多个 session 照样保留。
  - **每个 console 独立一条连接**，事务、search_path、取消互不影响；console 标题上可以选连接和 schema，有未提交的事务时不能换连接。
  - 事务模式按 console：标题上 `Tx: 自动 ▾`，手动时有提交 ✓、回滚 ↺（照 DataGrip，用户给的截图）；表格的保存总是自己一个事务。
  - 只读按连接：connections.toml 的 `read_only = true`。
- **完成标准**：
  - F5.1–F5.9 全部 passed；
  - 前面各阶段的集成测试在 MySQL 上也全部通过；
  - 用户验收通过后，打 tag `m5`。
- **审查节点**：
  0. F5.9 之后：F5.5–F5.9，M4 验收意见，先做；
  1. F5.3 之后：F5.1–F5.3，多 session、多 window、只读与事务模式；
  2. F5.4 之后：MySQL 单独一批。

任务文件的格式和状态约定见 [`../plan.md`](../plan.md)。

---

## F5.1 多 session · 状态：draft

- **session 列表浮层**：支持 S-02、S-03 规定的全部按键（`j/k`、`l/h`、`↵`、`n`、`x`、`$`）。
- **新建连接**：通过表单新建，结果写入 `connections.toml`。
- **管理 session**：attach、关闭、重命名；点击状态栏的 session 块也能打开列表。
- **命令面板的会话范围 `$`**（从 M4 挪来，§12）：列出 session，`↵` attach。

**验收**
- [ ] e2e：
  - 新建连接后，`connections.toml` 中出现这条连接，且不会破坏文件中已有的内容；
  - 在两个 session 之间切换后，各自的界面状态保持不变。

## F5.2 多 window · 状态：draft

- **按键**：`SPC n` / `SPC p` / `SPC l` 切到下一个 / 上一个 / 上次用的 window，`SPC c` 新建；重命名、关闭默认不绑键，在命令面板里执行（§6.8）。不提供按编号切换 window。
- **鼠标**：点击状态栏上的 window 列表可切换。
- **独立性**：每个 window 有自己的布局和结果区。

**验收**
- [ ] e2e：
  - 新建、切换、重命名、关闭 window 都正确；
  - 各 window 的布局互不影响。
- [ ] 补回 M0 里因 F1.1 去掉第二个 window 而删掉的 3 项 e2e（tester 在 F1.1 结论里列出，2026-09-24）：f0.5 其余 window 为 dim 字、无底色；窄宽度下省略非当前 window（含省略顺序单调性检查里的这一环）；f0.14 window 范围 `↵`（`%report`）。

## F5.3 只读 session 与事务模式 · 状态：draft

- **只读**（S-04）：应用层拦截与数据库层设置两层同时生效；SET 语句按 §13 的规则区分放行和拦截。
- **只读 session 禁止编辑单元格**（§10.1）：M2 没做，随 `connections.toml` 的 `read_only` 一起加（M1 F1.1 时去掉了这个字段）。
- **事务模式**：在 auto 和 manual 之间切换；提供 Commit 和 Rollback 命令；manual 模式下保存修改不自动提交。有未提交的事务时，退出前需要确认。
- **console 标题上的事务控件**（M2/M3 验收时用户要求，照 DataGrip）：`Tx: 自动 ▾` 下拉切换自动 / 手动；手动时旁边显示提交 ✓、回滚 ↺ 按钮，点击即执行，也有对应的命令。M3 已知上限「在 console 里自己 begin 之后不提交」在这里一并处理：状态栏显示事务中。
- **状态显示**：状态栏和查询条显示当前事务模式。

**验收**
- [ ] 集成测试：
  - 只读 session 中，写语句被拦截；
  - 解除只读的 SET 被拦截，`SET search_path` 等可以执行。
- [ ] e2e：manual 模式下保存后不提交；执行 Rollback 后修改被撤销。

## F5.4 MySQL · 状态：draft

- **驱动**：对应 §8.1–§8.3 中 MySQL 的部分：
  - 走文本协议，开启 `interpolateParams`；
  - 取消执行用 `KILL QUERY`。
- **catalog**：取 §8.4 表格中 MySQL 那一列的数据：
  - `TABLE_ROWS`、`STATISTICS`；
  - 从 `COLUMN_TYPE` 解析出 enum 的可选值；
  - DDL 用 `SHOW CREATE TABLE` 获取。
- **方言**：
  - 标识符用反引号；
  - 空值安全比较用 `<=>`；
  - 布尔值写入 1 / 0。
- **种子数据**：`testdata/seed/mysql.sql`，覆盖的情况与 PG 的种子数据相同。
- **扫描器**：MySQL 会执行的注释 `/*! … */`（如 `SELECT 5 /*! +1 */` 返回 6，dump 开头的 `/*!40101 SET NAMES utf8 */;`）M3 的扫描器当作注释，分句和读写判定会错；M5 接 MySQL 时处理（M3 审查时发现）。

**验收**
- [ ] M1–M4 的集成测试在 MySQL 上全部通过。
- [ ] 用 `KILL QUERY` 取消长查询有效。

---

以下为 M2–M4 用户验收的意见（2026-10-02），在 F5.1 之前做，审查节点 0。

## F5.5 工具按钮共用一条底色、竖线分隔 · 状态：todo

- **依赖**：F3.26
- **涉及**：`internal/ui`（querybar）

**开发**
- [ ] 按 §7.8「工具按钮」：整排按钮共用一条 `sep` 底，组之间用 `border` 色的 `│` 分隔（参考 DataGrip 的工具栏，用户嫌一组一块底色有割裂感），转置也在这条底里；悬停只亮那一个按钮；让位顺序不变。
- [ ] 细节（worker 提议，已定）：`│` 占 1 列、前后不另留空，组间距从 2 列变 1 列；`│` 是 `sep` 底、没有命中区；某组让位后 `│` 只画在仍显示的组之间；`ui.ButtonGroup.Bare` 没人用了，删掉。

**验收**
- [ ] golden：几种宽度下的按钮条，含悬停。

## F5.6 tab 上悬停的 `×` 底色画满 · 状态：todo

- **依赖**：F3.35
- **涉及**：`internal/ui`（tab 栏）

**开发**
- [ ] 悬停时 `×` 的 `select` 底只盖住了一半（用户截图）：nerd 图标在终端里会画出格子外，按 §7.7「图标后面留空格」，`×` 后面跟一个空格、一起画底色，命中区同样覆盖这两格；悬停时 tab 栏仍不移位。原因和做法由 worker 确认后在提交说明里写明。
- [ ] 细节（worker 提议，已定）：根因是 U+F00D 按字素宽度算 1 格，终端却画进右边一格。有标记（`*` / `-`）的 tab，`×` 替换标记，和后面的空格两格一起亮、一起做命中区；没有标记的 tab，悬停时把右边的 `│` 画成亮着的空格并算进命中区，结果区最后一个 tab 后面直接画一个亮着的空格。平时和悬停时布局都不变，不给每个 tab 预留标记位。

**验收**
- [ ] golden：悬停时 `×` 和它后面的空格都是 `select` 底。

## F5.7 转置视图左对齐 · 状态：todo

- **依赖**：F1.3
- **涉及**：`internal/ui`（grid）

**开发**
- [ ] 转置视图里所有值一律左对齐，数字也是（§7.6：一列里混着各种类型，右对齐的数字看着乱）；不转置时照旧。

**验收**
- [ ] golden：转置后的 t_order，`id`、`amount` 左对齐。

## F5.8 `:` 打开「所有」范围 · 状态：todo

- **依赖**：F0.13
- **涉及**：`internal/keymap/default.toml`、`internal/app`（palette 的排序）

**开发**
- [ ] `[keys.grid]`、`[keys.tree]`、`[keys.landing]` 里的 `:` 改绑 `palette.open`（「所有」范围，同 `C-p`）；在「所有」范围里，输入和某个 ex 别名完全相同时这条命令也排第一，`:q↵`、`:qa↵`、`:w↵` 照旧（§12「ex 别名」）。想要原来的命令范围，把 `:` 绑到 `palette.command`（不另加配置项）。
- [ ] 细节（worker 提议，已定）：`palette.command` 保留标题、不再有默认键，导出时是 normal 节里的 `# "" = "palette.command"`；ex 别名在「所有」和 `>` 范围都生效，去掉首尾空白后完全相同才算，排在最近用过的前面。

**验收**
- [ ] e2e：表格里按 `:` 打开的是「所有」；`:q↵` 关掉 tab、`:w↵` 保存。

## F5.9 键位挪到 keymaps.toml · 状态：todo

- **依赖**：F1.13
- **涉及**：`internal/config`、`internal/keymap`、`cmd/sqlmux`（`keys --format toml`）

**开发**
- [ ] `[keys]`（含 leader）、`[keys.*]`、`[map.*]` 从 config.toml 挪到 `~/.config/sqlmux/keymaps.toml`（§14）；config.toml 里还写着它们时启动报错，提示挪到 keymaps.toml。
- [ ] `sqlmux keys --format toml` 输出一份完整的 keymaps.toml（§6.7）：各作用域都有，和默认相同的绑定写成注释行、值是默认键，后面注释标题；没有默认键的写 `# "" = …`；`leader` 和各层 `[map.*]` 的示例也注释着放进去。用户去掉 `#` 才生效，新版本改了默认键不会被旧文件盖住。用户改过的（改键、解绑、leader、映射）照旧写成生效的行（tester 提问后定，保留 F1.13「导出的是当前生效的键位」的往返）。`--check` 读 keymaps.toml。
- [ ] 细节（worker 提议，已定）：config.toml 里有顶层 `keys` / `map` 时报 `<路径>/config.toml: [keys] / [map] 要写在 keymaps.toml 里（sqlmux keys --format toml 生成一份完整的参考）`，走现有的启动报错、退出码 1，`keys --check` 同样报；keymaps.toml 里写了别的顶层键时报 `<路径>/keymaps.toml: <键>：这里只能写 [keys] 和 [map.*]`，文件不存在就当没有改动；config 包加 `ParseKeys`，`default.toml` 也走它，不加新类型。导出：`[keys]` 和各作用域的表头不注释，leader 是默认值时写 `# leader = …`；每个作用域按 default.toml 的顺序，相同的写注释行、改了的和解绑的在原位置写生效的行，然后是用户新加的绑定，最后是没绑定的 `# "" = …`；`[map.*]` 只出用户写了的表，末尾照旧附注释掉的示例；空作用域不出，文件头不加说明。
- [ ] 通过后决策者把用户的 config.toml 里的键位挪出去：生成 keymaps.toml，用户的 `Y` 映射保留为生效的行，config.toml 里只留主题等常规选项（各留备份）。

**验收**
- [ ] 单测：没有用户改动时，导出的全是注释，原样加载后和默认完全一样；有改动时导出再加载，生效的键位不变（往返）；去掉一行的 `#` 改了键后生效；config.toml 里写 `[keys]` 时报错。
- [ ] e2e：只有 keymaps.toml、里面改了一个键时，界面上的键位提示跟着变。

