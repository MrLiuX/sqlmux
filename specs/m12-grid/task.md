# M12 表格日常 · 任务清单

- **状态**：**todo**（2026-10-11 用户规划 M12–M14 时定；M11 F11.4 passed 之后开工）。
- **目标**：多列排序、值编辑器（长文本和 JSON）、按单元值过滤和按列排序加表格的右键菜单、导出 CSV / JSON；连接连不上不退出、表单的测试按钮、tab 移到相邻 pane。用户 2026-10-11 对照 DataGrip 提出，决策者分析后拆成 M12（表格日常）、M13（SQLite 与历史）、M14（PG 数据库层级）三个里程碑，一个 sprint 不超过两个审查节点。
- **范围**：tech-design §5（`Query.Order`、启动连接）、§6.4（`value` 作用域、`menu`）、§6.8（键位表）、§7.6（ORDER、表头、按值过滤、按列排序、右键菜单、导出、工具按钮）、§10（值编辑器）、§11（结果导出）、§14（布局文件的 `order`、启动时连不上）、§7.8（表单的测试、tab 移动）。
- **依赖**：M11 F11.4 passed。
- **完成标准**：F12.1–F12.5 passed；tester 在最后一个 commit 上跑完整回归；用户验收通过后打 tag `m12`。
- **审查节点**：
  1. F12.2 之后：F12.1–F12.2 一批（多列排序、值编辑器）；
  2. F12.5 之后：F12.3–F12.5 一批（过滤 / 排序 / 右键菜单、导出、连接与 tab 的小项）。

任务文件的格式和状态约定见 [`../plan.md`](../plan.md)。

---

## F12.1 多列排序 · 状态：todo

用户 2026-10-11：ORDER 只支持一个字段一个方向是严重问题，应该多个字段、各自指定方向，像 DataGrip 的 `ORDER BY source DESC, id asc`。

- **依赖**：F1.4（ORDER chip）、F6.5（布局文件）
- **涉及**：`internal/app`（grid、query 拼装、布局文件）、`internal/ui`（下拉框、chip）

**开发**（§5、§7.6「ORDER」、§14）
- [ ] `Query.Order` 从一列改成有序的 `[]OrderCol{Col, Desc}`，空是默认排序；SQL `ORDER BY <列1> <方向1>, …, <行标识列>`，行标识列作 tiebreaker，列表里已有的不重复。
- [ ] 下拉框：已在排序里的列前面标 `1↓` `2↑`；`↵` 保持原来单列的语义（唯一排序列上翻转，别的列上换成只按这一列）；`space`（`dropdown.toggle`，过滤框为空时）把光标所在列循环：追加（方向按 `[grid] order`）→ 翻转 → 去掉，下拉框不关；过滤框非空时 `space` 是字符。
- [ ] chip 显示整个列表 `source ↓, id ↑`，放不下照 §10.3 中间截短；点方向图标翻转第一列。
- [ ] 布局文件 `order` 改成 `[{col, desc}]` 数组，旧的 `order` 字符串加 `desc` 照读成一列，写回新格式。换了排序回到第 1 页，同原来。
- [ ] 细节（已定）：默认排序的 chip 显示和「选行标识列得到降序」的规则不变（§7.6）。

**验收**
- [ ] 单测：`space` 两列后 SQL 是 `ORDER BY source DESC, id ASC`（id 是行标识列时不再追加）；`↵` 在别的列上清掉列表只剩这一列；`space` 第三次去掉；旧布局文件读成一列、写回是数组。
- [ ] golden：两列的 ORDER chip。
- [ ] e2e：`go`、移到 source、`space`、移到 id、`space` `space`、`esc` 后第一行是 source 最大、同 source 里 id 最大的记录（PG 和 MySQL 各一次）。

## F12.2 值编辑器 · 状态：todo

用户 2026-10-11：长文本和 JSON 看不见，要像 DataGrip 的值编辑器：弹窗里支持 vim、修改、格式化，JSON 自动格式化并高亮；和 query 的编辑器差不多。记录视图不另做，转置视图 `T` 就是。

- **依赖**：F3.1（编辑器）、F9.8（DDL 浮窗）
- **涉及**：`internal/app`（grid、浮窗）、`internal/editor`（只读模式、JSON 着色）、`internal/ui`、`internal/keymap/default.toml`

**开发**（§10「值编辑器」、§6.4 `value`）
- [ ] `e`（`grid.value`「在值编辑器中打开」，grid 和 result）打开居中浮窗，宽高同表 DDL 浮窗，标题 `<列名> · <类型>`，内容是 `internal/editor` 的编辑器，作用域 `value`：`C-s` 写回并关闭，其余交给 vim；`[map.normal]` / `[map.visual]` 照常。模式块显示编辑器的模式。
- [ ] JSON：列类型 json / jsonb / json，或文字 `json.Valid`，打开时格式化成 2 空格缩进；着色用一个小的 JSON 词法（键名 `sql_column`、字符串 `string`、数字 `number`、true / false / null `keyword`、标点 `sql_operator`）；`gq` 格式化 JSON，不合法 toast「不是 JSON」。
- [ ] 写回：`:w` 记成这一格的修改（规则同行内输入框：没改不记；空是空字符串），不关；`C-s` / `:wq` 写回并关；`esc`（NORMAL）/ `:q` 不写，有改动先确认「放弃修改？」。JSON 列写回原文含缩进。
- [ ] 只读：进不了编辑的格和结果 tab 也能开，改动的键 toast「只读」，`y` 照常，`esc` / `q` 关。编辑器需要一个只读开关（最小改动：改动的操作统一拦在一处）。
- [ ] 细节（已定）：不做 DataGrip 的「设置高亮显示语言」，只认 JSON；不做记录视图。

**验收**
- [ ] 单测：json 列打开后文字是格式化的 JSON、着色的 token 类型正确；`:w` 后格的修改是编辑后的文字；没改 `:q` 不留修改；只读格里 `x` 不改文字且 toast；`gq` 对非 JSON 不动。
- [ ] golden：JSON 值的浮窗。
- [ ] e2e：表格上 `e`，`gq`，`:wq`，`C-s` 后库里的 json 列是编辑后的值（PG 的 jsonb 会规范化，比对解析后的值）。

## F12.3 按值过滤、按列排序、表格右键菜单 · 状态：todo

用户 2026-10-11：DataGrip 列上右键按单元值过滤要做；每列的漏斗和上下按钮直接按列排序要评估。决策者：漏斗不做（右键菜单加 WHERE 就够），表头点击排序和右键菜单做。

- **依赖**：F12.1、F8.23（树的右键菜单）、F10.3（COLS）
- **涉及**：`internal/app`（grid、where、菜单）、`internal/ui`（表头命中、菜单）、`internal/keymap/default.toml`

**开发**（§7.6「表头」「按单元值过滤」「按列排序」「右键菜单」、§6.8）
- [ ] `*`（`grid.filter.cell`「按这个值过滤」）：`col = <字面量>`（按列类型加引号和转义，NULL 是 `IS NULL`）追加进 WHERE（已有条件 ` AND ` 接），相同条件不重复，重新取数，进 WHERE 历史。结果 tab 不做。
- [ ] 表头命中目标 `head`：单击只按这一列 → 翻转 → 默认；`S-click` 同 ORDER 下拉的 `space`；转置下不响应。键：`gs`（`grid.order.cycle`「按这一列排序」）、`gS`（`grid.order.add`「加入排序」）。
- [ ] 右键菜单（复用树的 `menu` 作用域和画法）：格上是 值编辑器 / 按这个值过滤 / 排序 ↑ / 排序 ↓ / 加入排序 / 隐藏这一列 / 复制这一格 / 复制行为 JSON / 导出…；表头上是 排序 ↑ / ↓ / 加入排序 / 隐藏这一列；结果 tab 的格只有 值编辑器 / 复制 / 导出。右键先把光标移到那一格。
- [ ] 细节（已定）：不做 DataGrip 的本地筛选（不发 SQL 的过滤）；`S-click` 终端不传 shift 位时就没有这个功能，键照样能用。

**验收**
- [ ] 单测：字符串、数字、NULL 三种格的 `*` 拼出的 WHERE；已有条件时的 ` AND `；重复不加；表头单击三次回默认；`gS` 两列。
- [ ] golden：表格右键菜单。
- [ ] e2e：格上 `*` 后行数变成那个值的行数；表头单击后第一行是最小值；右键菜单「隐藏这一列」后表头少一列。

## F12.4 导出 CSV / JSON · 状态：todo

用户 2026-10-11：表顶部要一个导出按钮，先支持 CSV 和 JSON（DataGrip 的导出对话框有提取程序和输出文件）。决策者：下拉选格式和范围，文件写当前目录，不做剪贴板。

- **依赖**：F3.x（结果导出 CSV）、F9.5（`Y` 的 JSON 规则）
- **涉及**：`internal/app`（导出、流式取数）、`internal/db`（不带 LIMIT 的流式读）、`internal/ui`（按钮）

**开发**（§7.6「导出」「工具按钮」、§11）
- [ ] 查询条加导出按钮（`export` 图标，转置那一组），`ge`（`grid.export`「导出…」）和右键菜单都开同一个下拉：`CSV · 当前页`、`CSV · 全部`、`JSON · 当前页`、`JSON · 全部`；结果区的导出按钮和 `result.export`（标题改「导出…」）也开它。
- [ ] 当前页写内存里这一页；「全部」用同一条取数语句不带 LIMIT / OFFSET 流式读写文件（列同显示的列和顺序），`C-c` 取消并删掉半截文件；结果 tab 的「全部」去掉自动 LIMIT 重跑，自己写了 LIMIT 的和快速 SQL 的就是当前页。
- [ ] 文件名 `<表名>.csv` / `.json`、结果 `query_1-42.csv`（第 2 页起 `-p2`），已存在加 `-2`；完成 toast 路径和行数。CSV 同 §11，JSON 是对象数组、每个对象同 `Y`。
- [ ] 细节（已定）：不做剪贴板、TSV、INSERT、XML；不做导出对话框。

**验收**
- [ ] 单测：四个选项的文件内容（CSV 表头、NULL 为空；JSON 的类型和键顺序）；已存在时 `-2`；隐藏的列不导出。
- [ ] 集成（PG 和 MySQL）：1203 行的表「全部」导出 1203 行，和 `R` 看到的顺序一致。
- [ ] e2e：`ge` 选 `JSON · 全部`，toast 路径，文件能 `python3 -m json.tool`。

## F12.5 连接连不上不退出、测试按钮、tab 移到相邻 pane · 状态：todo

用户 2026-10-11：`sqlmux` 启动时 play 连接超时直接报错退出（`failed to connect … context deadline exceeded`），应该进界面再 toast，让用户能在树里建新连接；新建连接要有 DataGrip 那样的测试按钮；要 `SPC wn` / `SPC wp` 把当前 tab 移到下一个 / 上一个 pane；dsn 的格式怕新用户看不懂。

- **依赖**：F6.5（布局恢复）、F8.16（连接管理）、F5.2（pane 顺序）
- **涉及**：`cmd/sqlmux`、`internal/app`（启动、form、tab）、`internal/db`（测试连接）、`internal/keymap/default.toml`

**开发**（§5、§14「恢复」、§7.8「新建连接表单」「把 tab 移到相邻 pane」）
- [ ] 启动连接连不上（`sqlmux`、`sqlmux <连接名>`、`--layout` 三条路都是）不再是启动错误：进界面，节点标失败、toast 驱动的错误；用它的挂起 tab 不关（用户 2026-10-11 补：DataGrip 那样留着显示报错），内容区居中画 `error` 色的错误页（连接名加驱动的错误原文，折行），`R`、展开节点、连上都重试，连上后照常取数；query tab 本来就留着。启动错误只剩配置本身的问题。
- [ ] 表单加「测试」按钮（`form.test`，`C-t`）：校验通过后用当前值真连一次（10s 超时），`测试中…`，结果写在底栏上方一行：`已连接 · <引擎 版本> · <耗时>` 或驱动的错误；不保存、测完断开。连接管理里同样。
- [ ] dsn 字段空时显示 `dim` 色的示例（按 engine）；别的字段照旧不预填。
- [ ] `tab.move.next` / `tab.move.prev`（默认 `<Leader>wn` / `<Leader>wp`）：当前 tab 挪到顺时针下一个 / 上一个普通 pane 的末尾并成为当前 tab，焦点跟过去；来源空了留引导页；只有一个普通 pane 或引导 tab 时无事。
- [ ] 细节（已定）：测试不把连接留在树里；MySQL 的 dsn 两种写法（`mysql://` URL 和驱动的 `user:pass@tcp(host:port)/db`）本来就都认（§8），示例只写 URL 那种。

**验收**
- [ ] 单测：启动连接拨号失败时 App 照常建出 session、节点状态是失败、toast 有错误、表 tab 还在且内容是错误页，连接恢复后 `R` 取到数据；`form.test` 成功和失败两种结果行；`tab.move.next` 后两个 pane 的 tab 列表和焦点；单 pane 无事。
- [ ] golden：带「测试」按钮和结果行的表单。
- [ ] e2e：把 play 的端口改成没人听的，`sqlmux` 能进界面、树里 play 变红、表 tab 居中显示错误、`conn.manage` 能建新连接；表单填一个错端口 `C-t` 结果行是错误、改对后是「已连接」；`SPC wn` 后 tab 在右边的 pane。
