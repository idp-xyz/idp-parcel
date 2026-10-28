# ADR-0337：监管运输处置承接与替代/退运旅程启动各是一种运营决定——承接口核提交方抄录的协作事项本体并答带原因的待补充，两口都记提交操作者

Status: Accepted（2026-10-28，Claude Code 会话代 TF owner 决：用户 2026-10-20 原话「要我裁定的部分，你作为业务和系统专家，请你自决吧」；票 [docs-consistency/code/43](../../.scratch/docs-consistency/code/43-regulatory-disposition-acceptance-and-alternate-journey-have-no-production-face.md)。裁决能力边界：读过 [ADR-0265](./0265-a-regulatory-disposition-supplement-required-is-an-answer-not-a-recorded-decision.md) 全文、[ADR-0151](./0151-unassigned-command-faces-get-their-families.md) 决定一与四、[ADR-0303](./0303-transport-fulfillment-operation-decisions-record-the-submitting-operator-as-operation-evidence.md) 决定一至三、UC-TF-001 与 UC-TF-007 全文，`AcceptRegulatoryDispositionHandler`、`StartAlternateJourneyHandler`、两个登记册与三条 Outbox 意图交付的适配器，`cmd/parcel-api` 现有 TF 运营决定口的装配与授予形状。没读：UC-CC-008 正文与 CC CONTEXT 的协作事项词条（只核了 `internal/customscompliance` 下没有协作事项的聚合或读口）。越权风险点见文末，待 TF owner 复核，第 1、2、4 点同时待 CC owner 会签。）<!-- 索引注记 -->｜**入口**：承接编排与旅程启动编排都已实现却没有生产入口。**取法**：两口各挂操作者渠道「运营决定」能力面的一种决定（监管运输处置承接、替代或退运旅程启动），按 ADR-0151 定族判据；承接口核载荷里据关务协作事项抄录的载运对象、移动动作、起止责任，缺则答待补充并指名缺哪一项（ADR-0265 决定四）；两口都把提交操作者作为操作证据记在决定记录上，只进审计。

Date: 2026-10-28

## Context

- 取证于 main `282af38da`。`application.AcceptRegulatoryDispositionHandler`（UC-TF-001）与 `application.StartAlternateJourneyHandler`（UC-TF-007）在 `cmd/` 下没有接线；读一侧已经接上（派发进程把旅程启动信封投给全程追踪投影，实际履约发生项按旅程册挂旅程），生产上没有写入方。承接册、旅程册与三条 Outbox 意图交付（承接回执、监管来路的处置执行、异常链）的 PostgreSQL 适配器都已在。
- ADR-0265 决定四要求承接编排读协作事项并核对象、动作、起止责任与当前有效性，不足时答带原因的待补充；票 code/43 把「协作事项交进来的形态」留作要裁的事：关务推信封，还是本上下文按事项引用去读。
- 关务一侧没有协作事项的聚合、读口或出站信封：`internal/customscompliance` 下没有 `UC-CC-008` 的协作事项实现，承接回执信封 `transport-fulfillment.regulatory-acceptance.recorded` 也没有消费者。两种形态今天都没有对端。

## Decision

**一、两口各是「运营决定」能力面上的一种决定。**（产品策略）

- 监管运输处置承接（`REGULATORY_DISPOSITION_ACCEPTANCE`），`POST /transport-fulfillment-regulatory-disposition-acceptances`；替代或退运旅程启动（`ALTERNATE_JOURNEY_START`），`POST /transport-fulfillment-alternate-journey-starts`。理由按 ADR-0151 定族判据：提交方是管理台上的运营人员，授予维度是决定种类，既不是登记册配置写也不是作业事实。
- 两种不共用，也不与运输委托提交共用：接受监管移动责任、开一条新旅程、向服务提供方提出运输范围是三种不同的权力，授予的粒度等于权力的粒度。写开关换不了这两口（ADR-0150 决定三）。
- 谁持有、谁被授予是租户取值：本记录不种任何生产授予；演示租户的授予、准入对照与权威区间在 `scripts/demo-seeds` 一节里登，证据记 `S`。

**二、两口都把提交操作者记在决定记录上，待遇照 ADR-0303。**（机制）

承接决定记 `decided_by`，旅程记 `started_by`（TF 迁移 `0056`），写法「发行方#sub」，可空、不回填、给了不得是空白；只进审计，不进重放比对，也不进任何门；回执不回显。同一事项（同一处置）换一位操作者重放，答已有，在册的仍是先到的那一位。

**三、协作事项本体由提交的运营人员据关务协作事项抄录进承接载荷，承接口核它，不读关务。**（机制）

- 载荷带 `itemObjects`（事项指名的载运对象）、`movementAction`、`origin`、`destination`（起止责任）；它们是承接的前提，不进决定记录与内容摘要。
- 核对次序，报第一项缺口，答 `SUPPLEMENT_REQUIRED` 并带 `supplementReason`（封闭四值，ADR-0265 决定四）：`MOVEMENT_ACTION_MISSING`（声称有起止而没有动作）、`OBJECT_MISSING`、`RESPONSIBILITY_MISSING`（缺起点或终点）、`MOVEMENT_AUTHORITY_MISSING`（承接或部分承接缺移动授权，领域门）。动作与起止都为空仍是 `NODE_ONLY_DISPOSITION`。待补充仍不落决定、不交回执。
- 已承接的对象不得超出事项指名的对象：超出答 `NOT_ACCEPTED`（运输方不扩大移动范围）。
- 事项本体的核对排在「同一事项是否已有决定」之后：已落的决定不因事项后来怎么变而变，改决定走事项方的更正或替代。
- 当前有效性不核：没有来源可核，见越权风险点 2。

**四、旅程启动口只核形状，来路由提交方显式标记。**（机制）

载荷带 `purpose`（`ALTERNATE`、`RETURN`）与 `basisKind`（`SERVICE_DISPOSITION`、`REGULATORY_DISPOSITION`），取值不在封闭集合里是坏报文；其余形状由领域答未受理。监管来路同笔入处置执行与异常链两份意图，非监管来路只入异常链（UC-TF-007 `AT-TF-080` 分流）。

**五、事务边界归装配点；`未决`整笔回滚。**（机制）

两口的写入与 Outbox 入队同一笔事务（ADR-0043）；编排答`未决`时回滚再交回那一份答复。

## 候选与反方

- **关务推一封协作事项信封进来。** 否决（今天）：关务一侧没有出站信封也没有协作事项聚合，推送方与消费方都得先造；而承接是运营人员对着一件事项作的决定，让这个人把事项的几格带进来，比先造一条尚无对端的通路少一层不确定。
- **本上下文按事项引用去读关务。** 否决（今天）：同上，没有提供方可读；留着消费侧适配器（ADR-0025）的位置，等关务给出读口，把 `itemObjects`、`movementAction`、`origin`、`destination` 四格从载荷里撤下、改由适配器供，编排的核对次序与原因不变。
- **两口合成一种决定。** 否决：见决定一。

## Consequences

- UC-TF-001、UC-TF-007 的状态行与「给开发的交接」改成现行；`AT-TF-367` 至 `AT-TF-370` 落点名。
- ADR-0265 决定四「接线时随读协作事项一起补上」一句由本记录决定三改为「核提交方抄录的事项本体」，其余各条不变；该记录 Status 与 Links 各加一条前向指针。
- 承接回执没有消费者（`.scratch/outbox-handoff-consumption-map/report.md` 里该行仍记「应有但未开」）：回执意图随决定入 Outbox，等关务一侧建消费者。

## 越权风险点

1. **抄录的事项本体是持授予者的断言，不是读回的事实**（TF、CC owner）。运营人员可能抄错或编造对象与起止；承接不会被拦。缓解只有一层：已承接对象不得超出抄录的对象。关务给出读口后，抄录的四格应撤下。
2. **当前有效性不核**（TF、CC owner）。事项是否仍当前有效、是否已被更正或取消，本上下文不知道；ADR-0265 决定四点名的这一项未做。
3. **旅程启动口不核监管来路的旅程有对应的承接决定**（TF owner）。持「替代或退运旅程启动」授予的人可以开一条 `REGULATORY_DISPOSITION` 来路的旅程，成员也不核是否在已承接对象内，旅程会流向关务的处置执行链。最小补法是启动编排按依据引用查承接册、成员须在已承接对象内；本票未做，因为 UC-TF-007 把备用与退运统归「授权业务角色」的采用决定，监管来路的关联规则要 TF owner 另定。
4. **关务一侧自己记住哪件事项在等补、消费承接回执**（CC owner）。ADR-0265 越权风险点 1、4 的关务一半仍未建。
5. **授予按两种拆开，租户要分别登两行**（TF owner）。若 owner 认为承接与旅程启动本是同一批运输运营，可合并为一种；那要新记录停用决定一的「不共用」。
6. **迁移与种子取号**。access_identity `0029`、TF `0056`、种子 `24`/`25`/`61`/`62`/`482` 取自 `282af38da` 后的下一个空号；并行分支若同样取号，由推送方在进 main 时重排。

## Links

- [ADR-0265](./0265-a-regulatory-disposition-supplement-required-is-an-answer-not-a-recorded-decision.md)：待补充是答复；本记录落地其决定四并改其读事项一句
- [ADR-0151](./0151-unassigned-command-faces-get-their-families.md)：运营决定能力面与定族判据
- [ADR-0303](./0303-transport-fulfillment-operation-decisions-record-the-submitting-operator-as-operation-evidence.md)：操作证据只进审计
- [ADR-0043](./0043-publish-intent-claimed-by-result-identity.md)、[ADR-0025](./0025-cross-context-adapters-live-on-the-consumer-side.md)、[ADR-0150](./0150-synthetic-tenant-is-treated-as-a-real-tenant-and-isolated-form-retires-per-face.md)
- [UC-TF-001](../application/transport-fulfillment/UC-TF-001-ACCEPT-AND-FULFILL-REGULATORY-TRANSPORT-DISPOSITION.md)、[UC-TF-007](../application/transport-fulfillment/UC-TF-007-START-ALTERNATE-OR-RETURN-JOURNEY.md)
- 票 `.scratch/docs-consistency/code/43-regulatory-disposition-acceptance-and-alternate-journey-have-no-production-face.md`
