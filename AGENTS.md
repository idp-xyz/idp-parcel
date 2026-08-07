# AGENTS

给人与 Agent 共用的工作方式入口。产品与领域规则以 `docs/` 权威文档为准；本文件只规定**怎么开工、怎么改、何时用 skill**，不重述限界上下文内容。

## 开工顺序

对任何功能或缺陷，按序加载材料；每步完成后再进入下一步。

1. **定切片** — 读 [首发开发主线](./docs/product/PARCEL-NETWORK-FIRST-RELEASE-DEVELOPMENT-BASELINE.md)，确认所属 PN 切片与主链/关务旁路。  
   **完成**：能说出 PN 编号、主责上下文、是否关务专项。
2. **定边界** — 读 [CONTEXT-MAP](./docs/domain/CONTEXT-MAP.md) 与目标上下文的 `CONTEXT.md`；跨术语读 [GLOSSARY](./docs/domain/GLOSSARY.md)。  
   **完成**：说清本上下文拥有/不拥有，以及会引用的邻接上下文。
3. **定行为** — 读对应 `UC-*`（见 [application/README](./docs/application/README.md)）与相关 ADR（见 [adr/README](./docs/adr/README.md)）。  
   **完成**：用例输入/结果/失败边界与未确认 `BD-*` 已列出。
4. **定交接** — 读该 PN 的 `docs/design/*handoff*`（无真实参数时优先合成任务包，如 [PN02-SYN](./docs/design/pn-02-synthetic-business-contract-development-task-pack.md)）。  
   **完成**：知道当前只允许骨架 / 隔离 `S` / 还是可提交 PN-08 候选。
5. **再编码** — 技术切片以 [Go 首个消费者决策简报](./docs/design/parcel-go-first-consumer-slice-decision-brief.md) 与包布局为准；默认主刀为 `UC-PS-001`「来源保全 → 生产归属 → 已提交」。  
   **完成**：变更落在正确 `internal/<context>/` 层；领域包不依赖 HTTP/`pgx`。

全库文档索引与阅读顺序：[docs/README.md](./docs/README.md)。`docs/archive/` 仅追溯，不是现行规则。

## 红线

用正向目标约束实现；下列为硬门禁。

| 目标 | 门禁 |
|---|---|
| 只实现已确认规则 | 未确认参数与 `BD-*` 保持可配置或显式未决；不写死为生产默认 |
| 证据层级诚实 | 隔离合成 `S` 只记为 `S`；生产通过只来自登记册证据 + PN-08 `Go` |
| 所有权清晰 | 限界上下文表达语言与数据所有权；不等于微服务、进程或库表共享许可 |
| 单一权威 | 一决策一处定义；用例与交接只引用，不复制第二套口径 |
| 敏感实例外置 | 仓库只登脱敏标识与证据索引；真实映射留在约定的受限存储 |

冲突时：产品基线 / 试点范围 / `CONTEXT*` / ADR 优先于用例与设计交接；交接优先于临时代码注释。

## 改文档

- 改领域语言、不变量或生命周期 → 改对应 `CONTEXT.md`（必要时 CONTEXT-MAP / GLOSSARY / SCENARIOS），再改引用它的 `UC-*`。
- 改难逆转技术或产品取舍 → 新 ADR 或 supersede；不改写已接受 ADR 历史。
- 改试点实例状态 → [参数登记册](./docs/product/PILOT-PARAMETER-REGISTER.md)，不写入领域文档当长期事实。
- 新增权威文档 → 在 [docs/README.md](./docs/README.md) 补入口与职责一句。

## Skills 路由

Skill 权威源是 [`idp-xyz/idp-skills`](https://github.com/idp-xyz/idp-skills)（私有）；本机 `~/.cursor/skills` 只是它的安装副本，改 skill 正文回上游仓库改，不要就地编辑。以下名单取该仓 `engineering`/`misc`/`productivity` 分类中与本仓相关者；细节以各 skill 正文为准。写作类、TS/Obsidian/教学类与 `personal`、`deprecated` 分类已排除，勿当本仓默认路径。

工程 skills 的 tracker / 标签 / 领域布局见下方 [Agent skills](#agent-skills)。

### Skill 可见执行

Skill 是 Agent 作业流程，不是 shell 命令；Cursor 不一定显示 skill 名。**本仓要求**在用户触发 skill（`/name`、@ 引用或口述「按某 skill 执行」）时，让人类能盯到进度：

1. **开场** — 首条回复写明：`按 /skill-name · Step N：…`（N 取该 skill 正文进程表第一项），并一句点出本 skill 目的。
2. **换步** — 每次进入 skill 的下一进程点，单独起一行：`按 /skill-name · Step N：标题`；该步的**完成标准**用半句标出（与 skill 正文一致，不另造阶段）。
3. **等人** — 需要选择/确认时写：`⏸ 等待你决定（Section/Step …）`；收到答复后写：`▶ 继续 Step N`。
4. **收工** — 结束时写：`✓ /skill-name 完成`，并列产物路径或「无变更」；若卡在 skill 中途收口，写：`◼ /skill-name 暂停于 Step N` 与恢复条件。
5. **嵌套** — 父 skill 调用子 skill（如 `/implement` → `/tdd`）时，父子名都写上：`按 /implement · Step … › /tdd · red`。

禁止只默默读写文件却不点名 skill；禁止自造与 skill 正文冲突的 Step 编号——skill 无编号则用 skill 内原有小节标题。

### 本仓主路径

| 意图 | Skill |
|---|---|
| 不知用哪个 | `/which-skill` |
| 磨想法并落 `CONTEXT`/ADR | `/grill-with-docs` |
| 术语/边界/不变量/生命周期 | `/ubiquitous-language` → `/domain-modeling` |
| 多会话：对话收成规格 | `/to-spec` |
| 多会话：规格拆成带阻塞边的票 | `/to-tickets` |
| 按票或明确行为实现（含 TDD + 双轴评审） | `/implement` |
| 只要测试先行的小行为 | `/tdd` |
| 相对固定点评审分支/PR | `/code-review` |

默认：边界清晰 → `/implement`（或 `/tdd`）→ 必要时再 `/code-review`。  
多会话：`/grill-with-docs` → `/to-spec` → `/to-tickets` → **每票清上下文**再 `/implement`。

### 入口与旁路

| 意图 | Skill |
|---|---|
| 外来 bug/需求堆（非 to-tickets 产物） | `/triage` |
| 难复现缺陷、回归 | `/diagnosing-bugs` |
| 大到看不清路的探索地图 | `/wayfinder`（产出决策，不直接交付代码） |
| 需可运行证据再定案 | `/prototype`（扔弃；结论经 `/handoff` 回主线） |
| 会话将满或换线程保上下文 | `/handoff` |
| 加深模块缝与接口形状 | `/improve-codebase-architecture`、`/codebase-design` |

### 偶发工具

| 意图 | Skill |
|---|---|
| 查一手资料并落引用笔记 | `/research` |
| 解决进行中的 merge/rebase 冲突 | `/resolving-merge-conflicts` |
| 走手工第三方配置/密钥步骤 | `/wizard`（上游仍属 `in-progress`，产出需人工复核） |
| 装本仓 pre-commit | `/setup-pre-commit` |
| 改本文件或 agent 可读文档写法 | `/writing-for-agents` |

## 当前默认切片

在另有明确 ticket 之前，编码默认推进：

- 用例：[UC-PS-001](./docs/application/parcel-shipment/UC-PS-001-SUBMIT-SHIPMENT-REQUEST.md)
- 技术边界：[parcel-go-first-consumer-slice-decision-brief.md](./docs/design/parcel-go-first-consumer-slice-decision-brief.md)
- 范围：来源保全、显式生产归属、进入`已提交`；不形成接受/拒绝/财务控制生产结果

## Agent skills

### Issue tracker

Issues and specs live as markdown under `.scratch/<feature>/`. See [`docs/agents/issue-tracker.md`](./docs/agents/issue-tracker.md).

### Triage labels

Default category/state role strings (`bug`, `ready-for-agent`, …). See [`docs/agents/triage-labels.md`](./docs/agents/triage-labels.md).

### Domain docs

Multi-context map at `docs/domain/CONTEXT-MAP.md`; shared ADRs in `docs/adr/`. See [`docs/agents/domain.md`](./docs/agents/domain.md).
