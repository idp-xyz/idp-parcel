# 07 只因歧义信号落下的身份冲突，在那份声明随委托被拒绝或撤回之后自动解开

Category: enhancement
Status: in-progress——2026-10-19 会话 `idp-parcel-9b` 立并代决、认领（ADR 占号 0211），分支 `claude-nri07`
Blocked by: 无
父票：[04](./04-identity-conflicts-have-a-disposition-and-release.md)
出处：ADR-0210 决定一「不做自动解除……记作后续方向」；ADR-0195 歧义信号。
地盘：
- docs/adr/0211-*.md（新）、docs/adr/README.md；ADR-0210、ADR-0199 文末复核记录与前向指针（部分停用照 adr/README 约定）
- docs/domain/node-operations/CONTEXT.md、docs/domain/parcel-shipment/CONTEXT.md、docs/domain/CONTEXT-MAP.md（PS → NO 新交接一条）、UC-NO-002 / UC-NO-004 相应格
- internal/parcelshipment/adapters/postgres/shipment_request.go 及其测试（委托离开`已提交`而未接受时交声明撤除信封；信封装在新文件）
- internal/nodeoperations/**（冲突来由一格；重核编排认「只因歧义信号」的冲突；inbox 消费门）
- migrations/node_operations/（冲突来由一列，下一个空号）
- cmd/parcel-dispatch/assemble.go（路由表加一行；装配进已有的 assemble_node_identity_recheck.go）——另一会话也在追加，只加行
- internal/architecture/partition_subject_registry_test.go（新分区键一行）
- docs/product/MECHANISM-INVENTORY.md、.scratch/at-coverage-inventory.md

代决依据：用户 2026-10-16 授权会话 `idp-parcel-9b`「你作为业务和系统专家你来自决」；2026-10-19 用户答「需要」。

## 为什么

ADR-0195 让「号码另有一份`已提交`委托声明」的唯一命中落成身份冲突（歧义信号）。那份委托后来被拒绝或撤回，歧义就不在了，这件实物其实可以唯一关联；可 ADR-0210 只给了人工处置一条解开的路，ADR-0199 决定三又规定记冲突的不重核——于是每一件都要人去点，信号来得快去得也快的场景（客户先报错号、马上撤单重报）会堆出一串人工活。

## 裁定（2026-10-19，会话 idp-parcel-9b 代决）

1. **冲突落下时记来由。** 收寄与重核落身份冲突时，在判断上记封闭的冲突来由：只因歧义信号（恰一个候选带信号）、多候选、一码两物（ADR-0196）。既有冲突行来由缺席、不回填——它们只经人工处置解开。
2. **只有「只因歧义信号」的冲突可以自动解开，且只经重核。** 重核对象放宽到「当前有效版本记冲突、来由是只因歧义信号」的待识别；重核照旧走与收寄同一份身份判断（含 ADR-0195 的 `Unique()`、ADR-0196 的按包裹认与包裹锁）。结果唯一且无信号 → 追加关联版（业务时间原收寄时刻、不另起控制起点、意图一封），冲突随链头解开；信号仍在 → 不追加；变成多候选或一码两物 → 照现有追加冲突版、来由随之更新。
3. **触发：PS 在声明消失时交门铃。** 一份带客户包裹件号的委托由`已提交`转为`已拒绝`或`已撤回`（含补充期满形成的拒绝）时，与状态变化同一笔事务，按它声明的每个号各交一封只带（租户，号码）的信封；NO 用已有的重核消费者接，按号找来由为「只因歧义信号」的冲突行重核。委托被接受走的是已有的「外部标识关联已形成」门铃，重核时那件会成为多候选或唯一（看另一侧），本票不另起触发。人工重核口（`PENDING_IDENTITY_RECHECK`）对这类冲突行同样放开。
4. **与人工处置的关系**：已经人工处置过的链，链头不再记冲突，不在重核对象里；自动解开不撤销任何处置。
5. 写 ADR-0211：部分停用 ADR-0210「不做自动解除」与 ADR-0199 决定三「记身份冲突的……不重核」两处（限只因歧义信号这一来由），照 adr/README 约定加前向指针、追加复核记录、README 标注。

越权风险点：代 NO 与 PS owner 定了「撤单即自动解开」——客户撤单重报同号的场景下，那件实物会在没有人看过的情况下关联到第一份委托的包裹；owner 复核时看是否要求这类自动解开也进复核目录供人事后核。

## 完成判据

- [ ] ADR-0211 与 CONTEXT / CONTEXT-MAP / UC 同步（先 CONTEXT 后 UC）；ADR-0210、0199 部分停用照约定。
- [ ] 冲突来由在收寄、重核、ADR-0196 冲突版、处置版上都记对（领域与编排用例）；迁移只加列不回填。
- [ ] PS：委托拒绝、撤回各交声明撤除信封，接受不交；信封 ID 用 `outboxintent.FingerprintEventID`，分区键登记。
- [ ] 真库端到端（经生产派发装配）：号码被另一份已提交委托声明 → 送站落冲突（来由只因歧义信号）→ 那份委托被拒绝 → 门铃 → 重核追加关联版，身份一格成立，意图一封；另有一格「拒绝之后仍有第三份已提交委托声明同号」不解开；一格「一码两物的冲突」不被这道门铃解开。修前先红。
- [ ] Go 门禁：`gofmt -l .` 无输出，`go build ./...`、`go vet ./...` 退 0；带 DSN 跑 `internal/parcelshipment/...`、`internal/nodeoperations/...`、`cmd/parcel-api`、`cmd/parcel-dispatch`、`migrations/...`、`internal/architecture` 全绿；机制清点重生成。
- [ ] 走 Go 那条路（[并行会话](../../../docs/agents/parallel-sessions.md)）。
