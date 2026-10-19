# 03 第二期：待识别实物按存下的号重核（AT-NO-026）——PS 关联册新形成关联时交出信封、复核页人工发起，结果唯一即追加关联版

Category: enhancement
Status: ready-for-agent——2026-10-19 会话 `idp-parcel-9b` 的作者子代理照票 01 拆出（代决）
Blocked by: 02 进 main（重核拿的是 02 存下的来源文档；追加版要指名前版）
父票：[01](./01-node-reception-preserves-the-delivery-source.md)
出处：票 01 裁定分期之「二、按存下的号重核」与「重核由什么触发」那一问；ADR-0199 决定三。`AT-NO-026` 在 `.scratch/at-coverage-inventory.md` 里是机制候选，今天没有票、也没有人工关联口。
地盘：
- internal/parcelshipment/adapters/postgres/external_identifier.go、shipment_request.go、label_transaction.go 及其测试（关联形成同一笔事务入队信封）
- PS 侧信封的传输形状与事件类型常量（落点照 `OutboxNodeIntakeHandoff` 那一类的先例，实施时定）
- internal/nodeoperations/application/（新重核编排；与收寄步骤 4 共用的身份判断抽出来，不另写一套）及其测试
- internal/nodeoperations/ports/ports.go
- internal/nodeoperations/adapters/postgres/reception.go 及其测试（按号找待重核的待识别）
- migrations/node_operations/（按外部标识观察、只覆盖待识别行的索引）
- cmd/parcel-dispatch（NO 第一个 inbox 消费者的装配）
- internal/nodeoperations/adapters/http/ 与 cmd/parcel-api（人工重核命令口）
- internal/nodeoperations/ports/catalogue_read.go、adapters/postgres/review_catalogue.go、adapters/http/query_node_operations_records.go（待识别行带外部标识观察）
- apps/admin-web/src/pages/operations/**（待识别实物页的外部标识观察列与「重核」动作）
- docs/product/MECHANISM-INVENTORY.md（重生成）
- .scratch/at-coverage-inventory.md

代决依据：用户 2026-10-16 授权会话 `idp-parcel-9b`「你作为业务和系统专家你来自决」。

## 现象

零候选的待识别实物落册后，PS 后来形成了这个号的关联，NO 没有任何路径再问一次：号码不在册上（票 02 之前），也没有触发。`AT-NO-026`「身份后来被 `parcel-shipment` 确认｜建立版本化关联，原待识别记录和既有事实保留；业务时间取原接收时刻，不另起控制起点，发布意图一封」没有机制。

## 做什么

照 ADR-0199 决定三：

1. **PS 交出信封**。`insertExternalIdentifierRelations` 的调用方在关联真插入的同一笔事务里经 Outbox 交出只带（租户，号码）的信封，信封身份由关联的键认领（ADR-0185 的键），经 `outboxintent.EnqueueOnce`；适用终态不交。信封不带包裹、分配方、标识类型或委托。
2. **NO 消费者**住 `cmd/parcel-dispatch`：按（租户，号码）找当前有效版本是待识别、不带关联、不记身份冲突、来源文档的外部标识观察与号码逐字相等的判断，逐件重核。匹配不到即什么也不做。
3. **重核编排**：拿来源文档里的外部标识观察走收寄步骤 4 同一份身份判断（含 ADR-0196 的按包裹认与 no-reception-identity/03 的（租户、包裹）锁），按 ADR-0199 决定三分格追加：唯一且包裹无别的实物关联 → 追加`节点收寄已形成`（接收时刻与控制照原判断、前版是被重核那一版、照 ADR-0186 取服务结果、意图一封）；唯一但包裹已关联别的实物 → 照 ADR-0196；多候选 → 追加冲突版；零候选 → 不追加（被重核那一版是「未核对」时追加「待识别 · 已核对」，票 05 之前不会出现）；读不动 → 不追加、交回 error。追加版的来源标识由被重核那一版的判断版本派生，带 `IDENTITY-RECHECK/` 前缀，撞键即重放。
4. **人工重核命令口**：指名一件待识别实物发起重核，对象条件同上。归族照 ADR-0151 决定一（管理台上作判断的口归「运营决定」能力面），实施时核对；对冲突行、无来源文档的旧行答哪一格照 ADR-0022 与 ADR-0186 的分格定，写进票面。
5. **复核目录与管理台**：待识别行带来源文档里的外部标识观察（旧行如实写「来源未保全」）；管理台待识别实物页加这一列与「重核」动作（走 Go 那条路，不走前端切片的 main 直做）。
6. **AT 点名**：`AT-NO-026`。

## 不做

- 后台扫表（裁定否决）；PS 读不动期间落下的未核对行在号码关联早已形成时怎么自动续上（ADR-0199 越权风险点，等用户决定，挂在票 05）。
- 身份冲突的处置：记身份冲突的待识别不重核。
- 票 02 之前落下、没有来源文档的旧行。
- 第一期落地到本票落地之间已形成的关联不补发信封（ADR-0199 能力边界），那段时间的零候选待识别靠人工重核。

## 完成判据

- [ ] 编排用例：零候选待识别在关联形成后重核成`节点收寄已形成`：带关联、接收时刻等于原收寄时刻、控制起点与控制依据照原判断、前版是原待识别、意图一封；同一信封重投、同一件再人工重核都不再追加（派生键撞键即重放）。
- [ ] 编排用例，非唯一各格：多候选追加冲突版；唯一但包裹已关联别的实物照 ADR-0196 两边追加；零候选不追加；身份读不动不追加且交回 error。
- [ ] 编排用例：记身份冲突的待识别、没有来源文档的旧行都不重核。
- [ ] 真库用例（经生产装配）：零候选收寄 → PS 委托接受形成这个号的关联 → 信封入队 → `cmd/parcel-dispatch` 消费 → NO 追加关联版 → 收寄意图一封，载荷指回的收寄业务时间是原收寄时刻；`JudgeIdentityBasis` 对这件实物判成立（`AT-NO-026`）。
- [ ] 真库用例：人工发起重核与信封触发落下同一结果；对冲突行与无来源旧行的答法照票面写定的那一格。
- [ ] 并发：重核与同一包裹另一件实物的收寄并发，经（租户、包裹）锁串行，后到者答身份冲突或已有结果，不出现两件都关联成立。
- [ ] PS 侧适配器用例：关联重放（ON CONFLICT 不插）不重复入队；适用终态不入队。
- [ ] admin-web 三道门退 0（口径见 [workflow 本机环境](../../../docs/agents/workflow.md#本机环境)）。
- [ ] Go 门禁：`go build ./...`、`go vet ./...` 退 0，`gofmt -l .` 无输出；带 DSN 跑 `internal/nodeoperations/...`、`internal/parcelshipment/...`、`cmd/parcel-api`、`cmd/parcel-dispatch`、`cmd/parcel-frontline-import`、`internal/architecture/...`、`migrations/...` 全绿；机制清点在本笔的干净检出上重生成。
- [ ] 走 Go 那条路（[并行会话](../../../docs/agents/parallel-sessions.md)）：分支、作者自验、非作者评审、推送方在干净检出上带 DSN 跑全量。
