# AGENTS

给人与 Agent 共用的工作方式入口。产品与领域规则以 `docs/` 权威文档为准；本文件只规定**怎么开工、怎么改、何时用 skill**，不重述限界上下文内容。

## 交付模式

2026-11-12 用户裁定，本节优先于本文件其余各节与 `docs/agents/` 下各文。起因：上线线已达成（见[上线线与冻结清单](./.scratch/launch-line-freeze-2026-10-27.md)），而冻结之后票与 ADR 照旧在长——流程里每一步的产出都是下一步的输入，没有哪一步会说「够了」。

**能开工的只有三种**，其余一律不开工，也不立票：

1. 冻结清单「限量生产前必须」一节里还没进 main 的。
2. 真实租户实际撞上的问题。
3. 用户点名的。

在途分支不自动豁免：继续还是搁置，由用户逐条点名。

**做的过程中只产出代码、测试和这张票自己的票面。** 具体四条：

- **ADR 只记难逆转的取舍。** 换掉它要迁数据、改对外契约或动上下文边界的才写；某个字段、某个口、某条分支的行为留在代码与测试里，要改口径就改 `CONTEXT.md` 或 `UC-*` 那一句。
- **评审只评一轮，只修挡上线的。** 口径用冻结清单「上线线」一节那句；不挡的在冻结清单「记录」追加一行，不立后续票，不开第二轮评审，修完由作者重跑评审的探针即可。
- **不主动做审计、盘点、复核与文档自洽扫描。** 「再查一遍还缺什么」永远有答案，它不是交付。
- **同时在写的会话不超过两个。** 超过之后花在协调上的比省下的多。

本节的解除条件：首个租户上线，或用户点名。

## 开工顺序

对任何功能或缺陷，按序加载材料；每步完成后再进入下一步。

1. **定切片** — 读 [首发开发主线](./docs/product/PARCEL-NETWORK-FIRST-RELEASE-DEVELOPMENT-BASELINE.md)，确认所属 PN 切片与主链/关务旁路。  
   **完成**：能说出 PN 编号、主责上下文、是否关务专项。
2. **定边界** — 读 [CONTEXT-MAP](./docs/domain/CONTEXT-MAP.md) 与目标上下文的 `CONTEXT.md`；跨术语读 [GLOSSARY](./docs/domain/GLOSSARY.md)。  
   **完成**：说清本上下文拥有/不拥有，以及会引用的邻接上下文。
3. **定行为** — 读对应 `UC-*`（见 [application/README](./docs/application/README.md)）与相关 ADR（见 [adr/README](./docs/adr/README.md)）。  
   **完成**：用例输入/结果/失败边界与未确认 `BD-*` 已列出。
4. **定交接** — 读该 PN 的 `docs/design/*handoff*`（无真实参数时优先合成任务包，如 [PN02-SYN](./docs/design/pn-02-synthetic-business-contract-development-task-pack.md)）。  
   **完成**：每个待定格已按三类归好（三类见[红线](#红线)一节「软件产品」那段）：机制与产品策略由本票做出执行器或参考配置，租户取值留空答`未配置`；证据记 `S` 还是可提交 PN-08 候选已判明。
5. **再编码** — 技术切片以 [Go 首个消费者决策简报](./docs/design/parcel-go-first-consumer-slice-decision-brief.md) 与包布局为准；默认主刀为 `UC-PS-001`「来源保全 → 生产归属 → 已提交」。  
   **完成**：变更落在正确 `internal/<context>/` 层；领域包不依赖 HTTP/`pgx`。

全库文档索引与阅读顺序：[docs/README.md](./docs/README.md)。`docs/archive/` 仅追溯，不是现行规则。

## 红线

用正向目标约束实现；下列为硬门禁。

| 目标 | 门禁 |
|---|---|
| 租户取值留给租户（旧称「只实现已确认规则」） | 租户取值保持可配置或显式未决，不写死为生产默认；未确认的 `BD-*` 先按三类归，归租户取值的同此办 |
| 证据层级诚实 | 隔离合成 `S` 只记为 `S`；**某租户实例**进入生产只来自登记册证据 + PN-08 `Go` |
| 所有权清晰 | 限界上下文表达语言与数据所有权；不等于微服务、进程或库表共享许可 |
| 单一权威 | 一决策一处定义；用例与交接只引用，不复制第二套口径 |
| 敏感实例外置 | 仓库只登脱敏标识与证据索引；真实映射留在约定的受限存储 |

冲突时：产品基线 / 试点范围 / `CONTEXT*` / ADR 优先于用例与设计交接；交接优先于临时代码注释。

**本仓开发的是卖给物流企业的软件产品，开发方不是运营企业。** 文档里的「运营企业」指购买产品的租户，锚点货主客户是租户的客户。产品开发由开发方独立做完：缺真实参数、真源或租户证据时，先把那一格归进三类，再动手——

- **机制**与**产品策略**（判断方法、内置策略、连接器形态、随产品发布的参考配置）现在就做完。外部系统对着公开规范、沙箱或模拟源接，用演示租户加 `SYN-` 数据走通，证据记 `S`；演示租户就是 `scripts/demo-seeds` 灌的 `SYN-TENANT-01`，开发与演示共用这一份，代码按真实租户对待它（[ADR-0150](./docs/adr/0150-synthetic-tenant-is-treated-as-a-real-tenant-and-isolated-form-retires-per-face.md)）。
- **租户取值**（某个租户的具体数值、对象、人与阶段决定）留空并拒绝默认值，那一格如实答`未配置`。它属租户上线时的实施，票照常收口。

立票与分诊时，`Blocked by` 与 `needs-info` 只指向另一张票、一篇待接受的 ADR 或一项用户决定。三类的分界检验与产品就绪判据见[开发主线](./docs/product/PARCEL-NETWORK-FIRST-RELEASE-DEVELOPMENT-BASELINE.md#切片的机制半边与实例半边)，取舍见 [ADR-0146](./docs/adr/0146-product-strategy-is-a-third-class-between-mechanism-and-tenant-values.md)；两处是唯一权威。

## 写代码注释

**注释一律用中文。** 本仓的领域语言是中文，`CONTEXT.md` 与 `UC-*` 都用中文表达不变量；注释里换成英文会在术语上多一次翻译，读的人还得自己对回去。

注释只写代码本身讲不出的东西——某条取舍为何如此、哪条 `CONTEXT` 规则要求它、换个写法会破坏什么。不写「这一行做了什么」，也不写变更说明。引用领域术语时用文档里的原词（`已提交`、`适用冲突`、`预计承诺`），不要自造译法。

注释里引另一个文件，照 [改文档](#改文档) 那一条办。规则与理由都在那里，此处不复述——复述的那半会在那条改动时留在这里变旧，而这一句自己声称过不重复。

改中文源文件时不要用 `Set-Content`，它会写成乱码让 `go build` 报 `illegal UTF-8 encoding`；原因与替代写法见 [workflow.md 的本机环境](./docs/agents/workflow.md#本机环境)。

## 改文档

- 改领域语言、不变量或生命周期 → 改对应 `CONTEXT.md`（必要时 CONTEXT-MAP / GLOSSARY / SCENARIOS），再改引用它的 `UC-*`。
- 改难逆转技术或产品取舍 → 新 ADR 或 supersede（够不够得上「难逆转」照[交付模式](#交付模式)那条判，够不上的不写）；不改写已接受 ADR 历史。新 ADR 只改掉前一份的某几句时是部分停用：被停 ADR 的 `Status` 行索引注记与 Links 各加一条前向指针，正文不动，写法见 [adr/README](./docs/adr/README.md) 顶部；同一笔把 `CONTEXT.md` 里被停的句子改成现行规则。
- `CONTEXT.md` 写成现行、代码尚未到的规则 → 规则句上不挂票号；规则与实现的差距写在各 UC 的「给开发的交接」，`CONTEXT.md` 头部只放一句同形的固定指针。
- 改试点实例状态 → [参数登记册](./docs/product/PILOT-PARAMETER-REGISTER.md)，不写入领域文档当长期事实。
- 新增权威文档 → 在 [docs/README.md](./docs/README.md) 补入口与职责一句。
- 引另一个文件的某句或某段 → 优先用符号名，无符号可指时用引文，一律**不用行号，也不用计数**；跨文件的序数（「第三条」）与位置词（「上一条」「上节」）同属，同此；已接受 ADR 的「决定几」编号不在此列——正文冻结，编号不会漂。行号失效时仍然指得很稳，只是指错了，没有任何东西会报；引文至少失配可见，但中文注释一折行就搜不中。**计数与行号同构**：「另一个包里有几道校验」「本包另三套门禁」数的都是别处的东西，被数那一侧增减一项是完全正当的改动，而改的人不会路过你这句话，于是它无声地变错——同一条理由，同一种守不住。只有一个例外：**数本身就是论点时**（举证「这几处各不相同」）必须写，且必须锚住取证时的 SHA，否则那几处被修好之后举证自己成了假话。ADR 尤其要守——它引的常是它自己要改的那份代码，Context 会在决定落地那一刻失效。**Go 注释里的跨文件引用同受此约束**，理由一字不差地成立：全仓抽验六处行号引用四处已经指错（实测于 `1dbc680`），而往被引文档头部插五行之后 `go build`、`go vet` 与全仓 `go test` 零信号——`internal/` 下没有任何测试读过 `.md` 的内容，这一类**结构上守不了**，只能靠不写。

## Skills 路由

交付模式下只用这几个；别的 skill 不因为「这一步通常该跑」而跑，用户点名时才用。

| 意图 | Skill |
|---|---|
| 按票或明确行为实现 | `/implement`（小行为直接 `/tdd`） |
| 难复现缺陷、回归 | `/diagnosing-bugs` |
| 落 main 前那一轮评审 | `/code-review` |
| 会话将满或换线程 | `/handoff` |
| 进行中的 merge/rebase 冲突 | `/resolving-merge-conflicts` |

停用到交付模式解除：`/grill-with-docs`、`/ubiquitous-language`、`/domain-modeling`、`/to-spec`、`/to-tickets`、`/triage`、`/wayfinder`、`/prototype`、`/improve-codebase-architecture`、`/codebase-design`、`/research`。

用 skill 时首条回复点名它（`按 /skill-name`），收工时列产物路径或「无变更」；中间不必逐步播报。Skill 权威源是 [`idp-xyz/idp-skills`](https://github.com/idp-xyz/idp-skills)（私有），调不到时的读法与进 main 的步骤见 [workflow.md](./docs/agents/workflow.md)；前端切片按其「前端切片」一节。

## 当前默认切片

没有默认切片。无票可做时不自己找活：对着[交付模式](#交付模式)那三种查一遍，都没有就停下来告诉用户。

## Agent skills

### Issue tracker

Issues and specs live as markdown under `.scratch/<feature>/`. See [`docs/agents/issue-tracker.md`](./docs/agents/issue-tracker.md).

### Triage labels

Default category/state role strings (`bug`, `ready-for-agent`, …). See [`docs/agents/triage-labels.md`](./docs/agents/triage-labels.md).

### Domain docs

Multi-context map at `docs/domain/CONTEXT-MAP.md`; shared ADRs in `docs/adr/`. See [`docs/agents/domain.md`](./docs/agents/domain.md).

### Flow binding

How the generic skill flow maps onto this repo's PN slices, handoff work packages, and evidence levels. See [`docs/agents/workflow.md`](./docs/agents/workflow.md).

### Parallel sessions

At most two writing sessions at a time ([交付模式](#交付模式)). Territory, committing, and reporting when several sessions write one working tree. Single-session work can skip its territory, numbering and roll-call sections, but the steps into `main` bind every session: see [`docs/agents/workflow.md`「进 main 的步骤」](./docs/agents/workflow.md#进-main-的步骤). See [`docs/agents/parallel-sessions.md`](./docs/agents/parallel-sessions.md). 多会话时开工第一眼看 `scripts/branch-state.ps1` 算出来的状态页（main 与 origin、各树未提交、在途分支），派单前带 `-Path <地盘>` 查半成品；不读任何人的自报。**diff 全在 `apps/admin-web/**` 的票不走它**：一人在 `main` 上直接做，落法见 [workflow.md「前端切片」](./docs/agents/workflow.md#前端切片一人在-main-上直接做)。
