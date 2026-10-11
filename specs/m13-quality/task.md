# M13 工程质量审计 · 任务清单

- **状态**：**todo**（2026-10-11 用户让 reviewer 做全仓工程质量审计，20 条发给决策者排进清单；M12 全部 passed 之后开工，排在 SQLite 之前，因为前五条是数据安全）。
- **目标**：修掉审计里的数据安全和多进程问题，把浮层的优先级收成一处，其余重复和性能按条做。reviewer 的探针（`aud/src`、`aud-app`、`aud-data`、`aud-core`，名字在各条里）放在 `/Users/ctw/proj/sqlmux-aud/`（worktree 外面、不入库：i18n 的单测会扫到 worktree 里未跟踪的 `.go` 文件；各自是独立 go module，README 写了怎么跑），worker 直接搬进测试。
- **范围**：tech-design §8.2（提交结果未知）、§14（state.json 合并、未知键、布局文件）、§7.4（浮层开着时的滚轮）、§7.9（输入框的 `C-h`）、§9.7（补全列表滚动）；其余是实现层面的改动，不改设计。
- **依赖**：M12 passed。
- **完成标准**：F13.1–F13.19 passed；tester 在最后一个 commit 上跑完整回归；用户验收通过后打 tag `m13`。
- **审查节点**：
  1. F13.6 之后：F13.1–F13.6 一批（必须改：数据安全、多进程、浮层漏键、未知键、快速 SQL 的 rollback）；
  2. F13.13 之后：F13.7–F13.13 一批（浮层收成一处、扫描器、列表绘制、可见性、`C-h`、布局文件、守卫的测试）；
  3. F13.19 之后：F13.14–F13.19 一批（可以改：小的收拾）。

审计里第 20 条（`sqlkit/json.go` 挪不挪）不做；session 列表、连接管理列表的名字截断加 `…` 和注释也不在 M13 做，留给以后的小收拾统一用 listText（审查时定）。每条「探针」是 reviewer 审计时写的复现测试，改完要能让它过。

任务文件的格式和状态约定见 [`../plan.md`](../plan.md)。

---

## F13.1 COMMIT 发出后断线报成「已回滚」 · 状态：todo

审计第 1 条（必须改）。COMMIT 发出去之后断线，报普通错误并拼「，已回滚」，其实已经提交，再存一次写两遍，PG 和 MySQL 都复现；query 里手写 `begin; …; commit` 也拿不到 §8.2 的「提交结果未知」。根因 `internal/db/worker.go:194`：`txLost, sent := w.inTx(), w.conn.calls > 0` 看不见本请求里自己开的事务。

- **依赖**：F8.19
- **涉及**：`internal/db`（worker）、`internal/app`（edit 的 gotSave）

**开发**（§8.2）
- [ ] worker 记下本请求最近一次成功调用后的 TxStatus，`lost()` 用它判 `TxLost{Sent}`；Save 在 `failed == -1` 且 Sent 时返回 `ErrCommitUnknown`；gotSave 遇到它不拼「已回滚」。
- [ ] 提交结果未知时表格按事务丢失那条路自动重载，修改标记保留在上面，toast「<连接名>：连接断开，提交结果未知：已刷新，请核对」，让用户自己核对（决策者定；复审时定为自动重载）。

**验收**
- [ ] 集成（PG、MySQL，自建库）：reviewer 的 `zz_savecommit_probe_test.go` 过；query 手写 commit 后断线报「提交结果未知」；表格保存在 COMMIT 前断线：错误栏是原来的错误加「，已回滚」，toast「连接断开，事务已丢失」，修改标记还在（审查时定，§8.2）。
- [ ] 单测：修改标记在 ErrCommitUnknown 后仍在。

## F13.2 两个进程写同一个 query 文件互相覆盖 · 状态：todo

审计第 2 条（必须改）。两个 sqlmux 打开同一个 query 文件，后 `:w` 的整份覆盖前一个（默认布局两边都开 query_1）。`app/query.go:280` flush、`config/query.go:49`。

- **依赖**：F8.21
- **涉及**：`internal/app`（query flush）、`internal/config`

**开发**
- [ ] 读写时记 mtime + size，flush 前比对，不同就拒写并 toast（同 vim 的 W11），约 10 行。拒一次、同一操作再来一次就覆盖；自动保存永不覆盖、只提示一次（审查时定，§11）。

**验收**
- [ ] 单测：reviewer 的 `internal/app/zz_twoproc_probe_test.go` 过；外部改过文件后 `:w` 拒写、toast。

## F13.3 多进程各写 state.json 后写者赢 · 状态：todo

审计第 3 条（必须改）。`config/state.go:174-201` 启动读一次之后整份写，WHERE 历史 / 收藏 / 快速 SQL 历史在多窗口下丢。

- **依赖**：F6.3
- **涉及**：`internal/config`（state）

**开发**（§14）
- [ ] 写前读盘按 key 合并：Tables / SQL / Queries 按键合并，Recent 去重（决策者定：用户本来就会开多个窗口）。

**验收**
- [ ] 单测：reviewer 的 `internal/config/zz_twoproc_probe_test.go` 过；两个进程各加一条历史后两条都在。

## F13.4 确认框开着时按键和粘贴漏进底下的输入框 · 状态：todo

审计第 4 条（必须改）。标删一行、`/` 写 WHERE、`C-s` 弹「将删除 N 行」，按 `x` 或粘贴，WHERE 变成 `idxZ`。`app/app.go:688` dispatch 的 switch 漏了 confirm。

- **依赖**：F2.x
- **涉及**：`internal/app`（dispatch）

**开发**
- [ ] `case a.confirm != nil || a.menu != nil || a.ddl != nil:` 一行；F13.7 收成一处时一并进 topOverlay。

**验收**
- [ ] 单测：reviewer 的 `TestZZOverlayConfirmLeaksKeysToWhere` 过。

## F13.5 配置文件拼错的键静默忽略 · 状态：todo

审计第 5 条（必须改）。连接里写 `readonly = true` 读成可写连接、不报错；顶层 `max_rows = 10` 读出 500。`config/config.go:148`、`config/connections.go:79` 没查 `Undecoded()`，主题文件 `ui/theme.go:120` 查了。

- **依赖**：F6.x
- **涉及**：`internal/config`

**开发**（§14）
- [ ] config.toml、connections.toml 读完查 `Undecoded()`，有就在启动后 toast 警告列出键名（`config.toml: 不认识的键 readonly（connection "D"）`），不阻止启动（决策者定：只读这类拼写更该让人看见，但不能因为一个多余的键进不去）；`sqlmux keys --check` 之外不另加命令。

**验收**
- [ ] 单测：reviewer 的 `internal/config/zz_unknownkeys_probe_test.go` 过；e2e：拼错键启动后 toast 列出键名。

## F13.6 快速 SQL 的 rollback 不走 db.End · 状态：todo

审计第 8 条（必须改的收尾）。两个引擎的 Quick 各写了一遍 rollback，都用 `context.Background()`、没走 `db.End`：`db/postgres/quick.go:28`、`db/mysql/quick.go:24`。服务器卡住时取消后 20 秒还占着 Meta，退出后进程不结束。

- **涉及**：`internal/db/postgres`、`internal/db/mysql`

**开发**
- [ ] 各一行 `defer func() { _, _ = db.End(ctx, c, "rollback") }()`。

**验收**
- [ ] 集成：reviewer 的 `db/postgres/zz_quickrollback_probe_test.go` 过。MySQL 不要测试：它的取消靠另开连接 KILL，服务端不可达时 db.End 也兜不住（审查时在真库复跑过），对齐 PG 的兜底是 M14 F14.6。

## F13.7 浮层的集合和顺序收成一处 · 状态：todo

审计第 7 条（应该改）。浮层的集合和顺序在 12 处各写一份（context、dispatch、wheel、paste、run 放行、completing、click 的 KindRow、drawOn 等），成员和顺序各不同；F13.4 就出在这里，滚轮也漏了：COLS / 命令面板 / session 列表 / 表单 / 确认框开着时滚轮滚的是底下的表。

- **依赖**：F13.4
- **涉及**：`internal/app`（app.go、view.go）

**开发**（§7.4）
- [ ] 一个 `topOverlay()` 按 context() 现有顺序返回最上层浮层，context / dispatch / overlaid / wheel / paste 都问它，dispatch 删掉约 10 个分支；drawOn 保留显式顺序、注释写明是它的倒序。不引入框架。
- [ ] 模态浮层开着时滚轮不滚底下的 pane（决策者定）；有自己 top 的列表让它滚自己留到以后。

**验收**
- [ ] 单测：reviewer 的 `TestZZOverlayWheelUnderCols` / `UnderOthers` 过；现有浮层测试全过。

## F13.8 扫描器对注释文本是平方的 · 状态：todo

审计第 6 条。`sqlkit/scan.go:75` 行注释 `strings.IndexByte(s[i:]+"\n", '\n')` 每次复制剩余全文：16000 行带 `--` 注释的文本 1.13s；`ui.Query.Draw` 每帧扫两遍，1 万行 1.1s / 帧。

- **涉及**：`internal/sqlkit`

**开发**
- [ ] 直接找换行、找不到到末尾，一行改动；`ui/query.go:21` 的 ponytail 上限按实际改。

**验收**
- [ ] 单测 / benchmark：reviewer 的 `zz_scanquad_probe_test.go`、`zz_querydraw_probe_test.go` 过，sqlkit 单测全过。

## F13.9 下拉和补全的列表行绘制合成一个 · 状态：todo

审计第 9 条。`ui/dropdown.go:96-101` 和 `ui/complete.go:72-77` 两份几乎一样：名字截断不加 `…`（§7.8 要求加），注释比框宽时画到框左边外面。

- **涉及**：`internal/ui`

**开发**
- [ ] 合成一个行绘制，用 TruncateMatch；注释比剩下的空间宽时先丢注释保名字，同树（§7.8，审查时定）。

**验收**
- [ ] 单测：reviewer 的 `ui/zz_listcut_probe_test.go` 过；golden 只有 `…` 的差别。

## F13.10 「让选中项保持可见」只留一个 · 状态：todo

审计第 10 条。约 10 遍、两种行为：palette / cols / dropdown / sessions 视图停住；where.go、edit.go、form.go 把选中项钉在最底行。

- **涉及**：`internal/ui`、`internal/app`

**开发**（§9.7）
- [ ] 只留 `ui.InView(top, sel, rows)`；补全 / 单元格选项 / 表单分组的状态里存 top；行为照 nvim 的补全弹窗：选中项在视图里就不动（决策者定）。列表移动的两行重复（cols.go、dropdown.go、palette.go、sessions.go）一起收。

**验收**
- [ ] 单测：reviewer 的 `app/zz_listtop_probe_test.go` 过。

## F13.11 单行输入框把 `C-h` 当退格 · 状态：todo

审计第 11 条。`app/tree.go:689` editInput（面板、树过滤、COLS、PAGE、单元格编辑、表单、下拉过滤共用）不认 `C-h`，命令行和 query 的 INSERT 认。

- **涉及**：`internal/app`

**开发**（§7.9）
- [ ] `case "<BS>", "<C-h>":`；editInput 搬出 tree.go（F13.15 之后可以进 ui）。

**验收**
- [ ] 单测：reviewer 的 `app/zz_inputkeys_probe_test.go` 过。

## F13.12 布局文件对坏输入前后不一 · 状态：todo

审计第 12 条。order 里一个坏值整份报错，`split = "H"` 却把一个 pane 连同 tab 静默丢掉。`app/layout_file.go:186-201`。

- **涉及**：`internal/app`

**开发**（§14）
- [ ] 有 `a` / `b` 两半就是分割：没写 `split` 按 h 不 warning，值不认识照「不认识的 tab」给 warning；只有 `pane` 的是叶子，多带 `split` 也不丢（§14，审查时定）。

**验收**
- [ ] 单测：reviewer 的 `internal/app/zz_layoutcheck_probe_test.go` 过。

## F13.13 有风险的守卫补测试 · 状态：todo

审计第 13 条。`app/app.go:654` paste() 里 WHERE 分支的 `!a.overlaid()` 删掉测试全过；`app/app.go:466` 点连接管理的一行后 `a.form.list = true` 没测试。

- **涉及**：`internal/app`（测试）

**开发**
- [ ] 搬 reviewer 的 `TestZZOverlayPasteGoesToPaletteOverWhere`；给 `form.list = true` 加一个测试。

**验收**
- [ ] 两个变异（删守卫）都红。

## F13.14 键位帮助被命令面板盖住 · 状态：todo

审计第 14 条。Ctrl leader 时在命令面板里打开键位帮助，帮助画在面板下面，键却给了帮助。`app/view.go:134、194`。

- **开发**：帮助画在面板、表单、session 列表、COLS 之后；标题照别处显示打开它的键（`?` / `C-a ?`），这些浮层的上下文没有 Focus 时也取（审查时发现露出来后标题是空的）。
- **验收**：帮助开着时屏幕最上面是帮助、键给帮助、标题非空。审计探针 `TestZZOverlayKeyHelpUnderPalette` 断言的是相反的行为，作废。

## F13.15 键位记法的解析挪进 keymap · 状态：todo

审计第 15 条。`config.ParseKeys` / `config.Binding`（约 35 行）放在 config，于是 keymap 的测试链接 goja、ui 没法引入 keymap；`keymap.go:27` tables 和 `:228` when 是两份平行列表。

- **开发**：那 35 行挪进 keymap，`keymap.New` 直接收 leader / vim / timeoutlen / bindings；两份列表合成一个有序的 `[]struct{name, when}`。§4 的包依赖说明跟着改（决策者改）。
- **验收**：`go list -deps ./internal/keymap` 不含 goja；`just check` 绿。

## F13.16 postgres.Conn 多余的导出字段 · 状态：todo

审计第 16 条。`Addr` / `Database` / `SearchPath` 只用来在 Connect 里拼 `db.Info`。

- **开发**：照 mysql.Connect 直接返回 `db.Info`，删掉三个字段。
- **验收**：`just check` 绿。

## F13.17 小的近似拷贝 · 状态：todo

审计第 17 条。`showToast(s, ttl)` 85 处调用 84 处传 toastTTL；query.go 和 value.go 设编辑器的四行一样。

- **开发**：`toast(s)`，退出提示单独传；四行抽一个函数。
- **验收**：`just check` 绿，golden 不变。

## F13.18 edit.go 按职责拆文件 · 状态：todo

审计第 18 条。`app/edit.go` 1116 行混着单元格编辑、保存、退出前确认、模拟 PG 输入校验。

- **开发**：只搬文件、不改行为：退出前确认和输入校验各自一个文件。
- **验收**：`git diff --stat` 只有移动；`just check` 绿。

## F13.19 多语句格式化逐条跑 · 状态：todo

审计第 19 条。goja 格式化 7KB 一次要 4s 或撞 5s 超时，同样 48 条语句逐条格式化总共 1.38s、输出一致。

- **涉及**：`internal/sqlkit`（Format）、`internal/app`（gq / query.format）

**开发**（§9.5）
- [ ] 多语句的 `gq` 和 `query.format` 逐条格式化再拼回，输出和整段一次格式化一致（每条从上一个 `;` 之后切到自己的 `;`，语句前的注释随这条交给 sql-formatter；不是原样保留，审查时定）；一次 `gq` 共用一个 5s 截止时间，每条拿剩下的时间；单条仍一次。
- [ ] 细节（已定）：不换格式化库；VM 常驻照旧。

**验收**
- [ ] benchmark：reviewer 的 `TestZZGojaPerStmt` 的数字；单测：12 条的输出和整段格式化一致（48 条整段必超时、没法比）；4 条 1.5KB 的 insert 总时长过 5s 要提示超时。
