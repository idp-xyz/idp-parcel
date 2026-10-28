# ADR-0338：节点监管协作承接是一种运营决定，执行事实是作业事实登记面上的一种事实——登记的现场节点必须就是承接决定里的节点

Status: Accepted（2026-10-28，Claude Code 会话代 NO owner 决（代决）：用户常设授权「你作为业务和系统专家自决」，另于 2026-10-28 答「现在就接线」；票 [launch-wiring/04](../../.scratch/launch-wiring/issues/04-node-collaboration-acceptance-production-face.md)。裁决能力边界：读过 [ADR-0333](./0333-a-node-collaboration-pending-supplement-is-a-stored-round-of-decision-and-the-supplemented-decision-is-the-next-round.md)、[ADR-0151](./0151-unassigned-command-faces-get-their-families.md) 决定一与四、[ADR-0149](./0149-business-command-faces-split-into-frontline-operator-and-integration-client-families.md) 决定二与四、[ADR-0337](./0337-a-regulatory-disposition-acceptance-and-an-alternate-journey-start-are-two-operation-decisions-and-the-acceptance-checks-the-item-body-it-is-handed.md) 全文、UC-NO-001 全文，`AcceptCollaborationHandler`、承接册与事实册及两份 Outbox 意图交付的适配器，`cmd/parcel-api` 现有 NO 运营决定口与作业事实口的装配。没读：UC-CC-008 正文（只核了关务一侧没有协作事项的读口与承接回执的消费者）。越权风险点见文末，待 NO owner 复核，第 1、3 点同时待 CC owner 会签。）<!-- 索引注记 -->｜**入口**：承接编排与执行事实登记编排已实现（含 ADR-0333 的待补充轮次）却没有生产入口。**取法**：承接决定与执行事实都把操作者记在记录上（只进审计）；承接决定挂操作者渠道「运营决定」能力面的一种（节点监管协作承接）；执行事实挂「作业事实登记」能力面、判自己那一种事实（节点监管协作执行）的准入范围；执行事实登记的现场节点须与承接决定里的节点一致。

Date: 2026-10-28

## Context

- `AcceptCollaborationHandler`（UC-NO-001）的 `Accept` 与 `RecordExecution` 在 `cmd/` 下没有调用方；承接册、执行事实册与两份 Outbox 意图交付的 PostgreSQL 适配器都已在，关务一侧的处置执行核对读口（`ExecutionFactView`）以执行事实意图为上游。
- UC-NO-001 的参与者表把承接交给「节点作业责任角色」、把现场事实交给「节点现场执行角色」，权限一节要求「协作承接、任务安排、受控开封、销毁/移交等不可逆执行、事实更正和结果发布分别校验权限」，管理员或设备权限不能互相推导。
- 关务一侧没有协作事项的读口，也没有消费承接意图的下游；协作事项的到达形态今天没有对端。

## Decision

**一、承接决定是「运营决定」能力面上的一种决定。**（产品策略）

- 决定种类 `NODE_COLLABORATION_ACCEPTANCE`，`POST /node-operations/collaboration-acceptances`。按 ADR-0151 定族判据：提交方是管理台上承担节点作业责任的人，授予维度是决定种类。四格（已承接、部分承接、待补充、拒绝）与待补充之后的下一轮都由编排与领域判（ADR-0333），入口只在形状上把关。
- 不与监管运输处置承接、送站交接确认、身份冲突处置共用——那些裁断的是运输责任、交付范围与实物身份；这一种是接受现场监管作业责任。
- 谁持有承接授予是租户取值：本记录不种任何生产授予；演示租户的授予、准入对照与权威区间在种子里登，证据记 `S`。写开关换不了这一口（ADR-0150 决定三）。

**二、执行事实是「作业事实登记」能力面上的一种事实。**（产品策略）

- 事实类型 `NODE_COLLABORATION_EXECUTION`，`POST /node-operations/collaboration-executions`，设备与作业范围三个头，作业范围种类只认 `NODE`。它是不可逆现场动作的记录，进的是关务处置执行核对的依据，所以像送站交接单三口那样在铸造侧判这一种事实的准入范围（ADR-0149 决定四第二条）。
- 授予的粒度等于权力的粒度：能承接的人不因此能替现场登记已做了什么；有设备的现场人员也不因此能决定承接。

**三、登记执行事实的现场节点必须就是承接决定里的节点。**（机制）

`RecordExecutionCommand` 带认证出的作业范围节点；与承接决定的节点不同时按`未受理`答（`SOURCE_NOT_ACCEPTED`），不落事实、不交意图，答复不泄露事项是否存在（UC-NO-001 应用流程步骤 1）。没有这一条，节点信封只是摆设：任一节点的设备都能替承接节点登记。

**四、承接决定的时刻是提交方填的业务时刻；操作者作为操作证据落库，待遇照 ADR-0303。**（机制）

载荷 `decidedAt` 必填，执行事实不得早于它（领域门）。承接册记 `decided_by`、事实册记 `recorded_by`（NO 迁移 `0025`），取自认证信封，写法「发行方#sub」，可空、不回填、给了不得是空白；只进审计，不进内容摘要，重放与冲突、任何门都不读，回执不回显。同一事项换一位操作者重放，答已有，在册的仍是先到的那一位。

**五、响应与事务。**（机制）

首次落决定（含待补充一轮）与首次登记事实答 201；重放、冲突、拒接或待补充的事项、越出承接范围、未受理答 200 带 `outcome`（业务负向格的恢复动作分别是重新承接、补齐缺口、扩范围，重发不会变好，设备据此出队）；册读写失败答 5xx `NO_ANSWER_FORMED`（ADR-0186 决定三）。`continuationReference` 非空说明发布意图还没交出去，重放会重发同一份。两口各包一笔事务，记录与意图同笔落地（ADR-0043）。

## 候选与反方

- **执行事实也走运营决定。** 否决：现场事实由设备在节点当场登记，授予维度是设备与作业范围，不是一种「决定」；挂在运营决定上会让能管理台点按钮的人不经设备册就写现场事实。
- **承接也走作业事实。** 否决：承接是责任角色的判断，不是现场当场发生的事。
- **等关务给出读口再接线。** 否决：用户 2026-10-28 要求现在接线；协作事项的几格由承接人填进载荷（节点、事项引用、范围），范围校验归领域，关务有读口后由消费侧适配器供（ADR-0025）。

## Consequences

- UC-NO-001 状态行改成现行，立 `AT-NO-133`、`AT-NO-134`。
- 承接意图与执行事实意图随记录入 Outbox，等关务一侧建消费者；`ExecutionFactView` 以执行事实意图为上游。两份意图无订阅者，派发答 `dispatch.no_subscriber` 直到预算耗尽，是认下的代价（同 ADR-0202 决定六先例）。

## 越权风险点

1. **协作事项的存在与当前有效性不核**（NO、CC owner）。关务没有读口，承接人可能对一件不存在或已取消的事项作承接；缓解只有范围校验与待补充缺口。关务给出读口后应在承接前核事项。
2. **操作者列不回填，读回的记录不带它**（NO owner）。存量行为空；`FindByKey` 读回的记录不带操作者，审计查询直接读两列。UC-NO-001 审计要求的「决定角色」由操作者加 `authority` 引用共同承担。
3. **承接后没有任务形成**（NO、CC owner）。UC-NO-001 的「作业任务」今天无聚合；执行事实直接依附承接范围。任务派生与完成状态不在本票。
4. **迁移与种子取号**。access_identity `0031`、NO `0025`、种子 `28`/`29`/`65`/`66`/`484`、`AT-NO-133`/`134`、本记录 `0338` 取自 `2d0193336` 后的下一个空号；并行分支若同样取号，由推送方在进 main 时重排。

## Links

- [ADR-0333](./0333-a-node-collaboration-pending-supplement-is-a-stored-round-of-decision-and-the-supplemented-decision-is-the-next-round.md)：待补充轮次；本记录为其编排接上生产入口
- [ADR-0151](./0151-unassigned-command-faces-get-their-families.md)、[ADR-0149](./0149-business-command-faces-split-into-frontline-operator-and-integration-client-families.md)：能力面与定族判据
- [ADR-0337](./0337-a-regulatory-disposition-acceptance-and-an-alternate-journey-start-are-two-operation-decisions-and-the-acceptance-checks-the-item-body-it-is-handed.md)：同形的 TF 运营决定口
- [ADR-0202](./0202-transport-commission-booking-and-carrier-answer-entry-families-and-supplier-agreement-check.md)：无订阅者意图的先例
- [ADR-0303](./0303-transport-fulfillment-operation-decisions-record-the-submitting-operator-as-operation-evidence.md)：操作证据只进审计
- [ADR-0043](./0043-publish-intent-claimed-by-result-identity.md)、[ADR-0025](./0025-cross-context-adapters-live-on-the-consumer-side.md)、[ADR-0150](./0150-synthetic-tenant-is-treated-as-a-real-tenant-and-isolated-form-retires-per-face.md)、[ADR-0186](./0186-node-reception-records-physical-facts-when-the-service-result-read-fails.md)
- [UC-NO-001](../application/node-operations/UC-NO-001-ACCEPT-AND-EXECUTE-CUSTOMS-NODE-COLLABORATION.md)
- 票 `.scratch/launch-wiring/issues/04-node-collaboration-acceptance-production-face.md`
