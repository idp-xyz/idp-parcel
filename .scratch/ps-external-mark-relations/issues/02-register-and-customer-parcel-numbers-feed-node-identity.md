# 02 外部标识关联册落地，客户包裹件号随委托接受形成，节点身份核对接真

Category: enhancement
Status: resolved——2026-10-16 进 main（`cc52e9b6`，分支 `claude-psxm` 快进，远端 main 含 `3e5f7c6f`）；见「进 main 记录」。此前 in-progress——2026-10-16 完工，待进 main；见「完成记录」。此前 in-progress——2026-10-16 Claude Code 会话认领，分支 `claude-psxm`，基线 `d01e4309`。此前 ready-for-agent——2026-10-16 Claude Code 会话立（[ADR-0184](../../../docs/adr/0184-external-identifier-relations-are-an-append-only-register-formed-with-their-source-facts.md) 决定一至六）
Blocked by: 无
父票：[01](./01-external-mark-relations-have-no-model-in-parcel-shipment.md)

## 做什么

1. **PS 领域**：外部标识关联的值类型——号码、标识类型（本票只有客户包裹件号一类有来源；渠道包裹件号随 03 进）、分配方、被标识对象、适用范围、形成依据——与关联本身；委托上的（声明包裹，客户包裹件号）配对，每件至多一个号，指着成员集合外即立不起来；已接受委托交出它该形成的关联（形成依据为接受决定）。
2. **提交路径**：`SubmitShipmentRequestCommand` 带配对；`IsolatedSubmissionIntake` 按逐件 `customerParcelReference` 与现签内部标识一一配上。配对立不起来答`输入未受理`。快照存、重建带回；新提交版本不重报。
3. **PS 持久化**：迁移建关联表与按（租户，号码）查的索引（适用变化表随 03：客户包裹件号没有作废或替代的来源事实）；`ShipmentRequests.Save` 保存已接受委托时与快照同一条事务写入它的关联，按键幂等。
4. **核对读口**：PS 端口按（租户，号码）答此刻适用、标识包裹的关联所指的包裹，逐字比对、去重；真库实现。
5. **NO 防腐适配器**：`internal/nodeoperations/adapters/parcelshipment` 实现 NO 的 `ParcelIdentityView`，包裹译成 `ParcelAssociationReference`；空号码答零个候选。
6. **装配**：`cmd/parcel-api` 与 `cmd/parcel-frontline-import` 换掉 `unconfiguredParcelIdentityView`，两处未配置替身与前线导入开工前那句提示一并退场。

## 不做

- 渠道包裹件号、作废与替代（03）。
- 人工登记、更正、作废口；查阅读面；客户参考号与渠道交易级号码两类（ADR-0184 决定二）。
- 「身份后来被确认」的补关联（`AT-NO-026`）。

## 完成判据

- 委托接受后，客户包裹件号逐字在节点识别出那一件包裹：收寄形成、控制建立，票 operator-channel/25 的服务结果读口随之被问。
- 接受前、已拒绝、已撤回的委托，其号码在节点答零个候选。
- 两件已接受包裹用了同一个号（同客户或跨客户）→ 两个候选 → 身份冲突。
- 已接受委托反复保存不重复形成关联；关联表只插不改。
- 提交时配对指着成员集合外、或一件两个号 → `输入未受理`。
- 真库用例：经生产仓储走「提交 → 接受 → 按号码核对」一条；`cmd/parcel-api` 装配的收寄编排对一件已接受包裹答`节点收寄已形成`。
- 架构门禁绿；机制清点重生成。

## 完成记录（2026-10-16，Claude Code 会话，分支 `claude-psxm`，代码 `cc52e9b6`）

- **落点**：PS 领域 `external_identifier.go`（号码、类型、分配方、适用范围、形成依据、关联、客户包裹件号配对；委托的 `ExternalIdentifierRelationsFormedByAcceptance`）；委托与重建面各带一格 `customerParcelNumbers`，重建门禁反射名单同笔登记；提交命令与 `IsolatedSubmissionIntake` 带配对；迁移 `0024` 关联表；`ShipmentRequests.Save` 同笔写关联；读面 `ExternalIdentifiers.FindParcelsByExternalIdentifier`；PS 端口 `ParcelExternalIdentifierView`；NO 适配器 `ParcelIdentityView`；`cmd/parcel-api` 的 `buildParcelIdentityView` 与 `cmd/parcel-frontline-import` 的 `parcelIdentityViewOf` 换掉两处未配置替身。
- **补的格**：领域——接受时每件给了号的成员形成一条（类型、分配方、范围、依据、形成时刻逐格对）；已提交与已拒绝不形成；配对指着成员外或一件两号即拒、两件同号不拒；号码逐字；新提交版本带过配对；重建带回、坏配对拒收。应用——配对随委托落下，坏配对答`输入未受理`、不建单、来源照常保全。Intake——客户参考按次序配上现签内部成员，内部标识不被客户参考顶替。真库——接受那一笔才写、再保存不重复成行；跨客户同号交回两件、大小写不同与他租户查不到；拒绝不留行；关联写失败接受整笔回滚。NO 适配器——号码原样交 PS、多件即多候选、空号码零候选不问 PS、读不动原样交回。装配——`cmd/parcel-api` 经真提交编排带号建单、接受前同号答待识别、经真仓储接受后同号唯一识别并形成收寄与控制。
- **改了预期的旧用例**：`TestTheWiredReceptionAnswersHonestlyAgainstARealDatabase`、`TestIsolatedReceptionAnswersBusinessOutcomesAgainstARealDatabase`、前线导入两条纵向用例——号码不在册里时明确接收答待识别并真实落库（原断言是未配置缝下停在`收寄待确认`、不落库）；前线导入因此三行全落、退出码 0，冲突那一轮退出码 2。
- **判据**：✅ 接受后逐字识别出那一件，收寄形成、控制建立（服务结果读口在同一编排里被问，读得动才走得到`节点收寄已形成`）；✅ 接受前与已拒绝零候选——已撤回只能发生在决定之前、从未接受过，结构上不会形成关联，没有另立用例；✅ 同号两件交回两件（三分支据此答身份冲突，由 NO 编排既有用例覆盖）；✅ 反复保存不重复；✅ 坏配对`输入未受理`；✅ 真库「提交 → 接受 → 核对」一条；✅ `internal/architecture` 绿，机制清点 `d270ad1e`（消费缝 `nodeoperations → parcelshipment` 由一个文件变两个，`ParcelIdentityView` 退出虚低名单）。PS、NO、`cmd/parcel-api`、`cmd/parcel-frontline-import`、`cmd/parcel-dispatch`、`tests/`、`internal/platform` 带 DSN 全绿；`go build ./...`、`go vet ./...` 退 0，`gofmt -l .` 无输出。
- **评审中改的**：关联访问器改成包内惯用的多行写法；ADR-0184 后果节与票 02、03 对齐「适用变化表随 03 进来」——本票没有任何来源事实会产生作废或替代，先建表只会让读口对一张永远空的表做排除。
- **开发主线**：补记一条「PN-03 第一项身份核对缝这一格已过时」，上表单元格不改写。
- **评审**：作者自审，不算非作者评审——已登进 [operator-channel/26](../../operator-channel/issues/26-non-author-review-backlog-for-claude-code-landings.md) 的评审清单。证据层级 `S`。

### 进 main 记录（2026-10-16）

- 分支 `claude-psxm` 四笔 `5714a137`（建模与 ADR-0184）/ `cc52e9b6`（代码）/ `d270ad1e`（清点）/ `3e5f7c6f`（记录）叠在远端 main `98934a17` 之上，快进进 main、推的就是这个 SHA。等全量期间另一会话两度推了管理台与 `/operator-binding` 的票，本分支两度重放；第二次重放之前核过对方改动的文件，与本票只在生成物 `MECHANISM-INVENTORY.md` 上重叠（重放后重生成），但对方动了同一个 Go 包 `cmd/parcel-api`，所以在新 tip 上重跑了全量，没有沿用上一次的结果。
- 推送方全量：钉 `3e5f7c6f` 的干净 detached 检出，带 DSN，`go test -p 1 -count=1 -v ./...` 退 0——有测试的包 142 个 ok、0 FAIL，`--- SKIP` 1（`TestHelperTemplateOwnerProcess`，照设计跳过）；同一检出上 `gofmt -l .` 无输出，`go build ./...`、`go vet ./...` 退 0。
- 评审：只有作者自审，没有非作者评审（本机无空闲的非作者通道；仓规 2026-09-16 不用子代理），已登进 operator-channel/26。

## Comments

- **评审 ← 会话 idp-parcel-9b 起的隔离子代理 · 钉 `cc52e9b6`（基线 `98934a17`；两笔：建模 `5714a137`、代码 `cc52e9b6`）· 2026-10-16 10:0x–10:2x UTC**（票 [operator-channel/26](../../operator-channel/issues/26-non-author-review-backlog-for-claude-code-landings.md) 序 0 补评。非作者：代码出自会话 `idp-parcel-c8`。隔离只读 detached 检出；这一行两轴各起一个子代理分评。由会话 `idp-parcel-9b` 代落。）
  - **Spec**：**阻断 无**。**非阻断 4**：
    (1) `internal/parcelshipment/adapters/http/isolated_write_intake.go` · `customerParcelNumbers`：号码取客户包裹参考，连带既有纪律——去两端空白后才进册键；同一委托两件同号，在摘要规范化那一步就答 4xx。端到端看，与 ADR-0184 越权风险点 2「…不进登记册的键」、点 4「…同一个号不拒收」相反，票面与 ADR 都没写明。评审会话复核：去空白来自本票之前就有的 `customerDeclaredReferences`，册键与委托记下的声明值一致，属判断题，交 PS owner 裁。
    (2) 迁移 `0024_external_identifier_relation.sql` 的主键（决定一的键）`(tenant_id, identifier_kind, allocator, identifier_number, object_ref)` 配上 `insertExternalIdentifierRelations` 的 `ON CONFLICT … DO NOTHING`，与 PS CONTEXT 生命周期「同一号码后来重新标识同一对象时形成新的关联，不复活旧的」相抵：同键的新关联被静默吞掉。本票不可达（还没有作废），票 03（作废与替代）动手前须裁。评审会话复核属实，并当面告知了在做 03 的会话。
    (3) 决定六「核对时不按范围收窄」与 UC-NO-002「收寄和身份核对必须在明确集团租户、客户账户…范围内执行」相抵；ADR 没读那一节，也没列为风险点。
    (4) `cc52e9b6` 原地改写了已接受 ADR-0184 的后果节，违反「不改写已接受 ADR 历史」。
  - **Standards**：**阻断 无**（(1) 经评审会话复核、定为判断题之后定级）。**非阻断 4**：
    (1) 同一个 `customerParcelNumbers`：号码取 `customerDeclaredReferences` 去两端空白后的结果，「去两端空白」因此进了登记册的键，与 ADR-0184 越权风险点 2「本记录一种折算都不内置……加在核对读口一处，不进登记册的键」及 PS CONTEXT「不做大小写、空白或校验位折算」相左。注释自称这是「草案的形状纪律」；判断题，交 PS owner 裁。
    (2) ADR-0184 Consequences：`5714a137` 已标 Accepted，`cc52e9b6` 又原地把「PS 迁移一份」改成「两份」，违反「改文档」的「不改写已接受 ADR 历史」。同日 ADR-0183 的做法是追加「复核记录（决定原文不动）」。Decision 未动、改动发生在合入前的同一分支上，判非阻断；补一条带日期的复核记录即可。
    (3) `internal/nodeoperations/adapters/parcelshipment/service_results.go` 包头注「两件」：数的是本包跨文件的适配器，违反「计数与行号同构」。
    (4) `docs/application/node-operations/UC-NO-002-RECEIVE-CUSTOMER-DELIVERED-PARCEL.md`「权限、隔离与审计」：该节与 ADR-0184 决定六「两个客户账户恰好用了同一个号，就是两个候选」相左；ADR 自述没读该节，UC 也未同步。按「改文档」，应在改 CONTEXT 之后「再改引用它的 `UC-*`」。
  - **无发现**：分层（领域与端口只引 `errors`、`time`、`context`）；所有权（NO 经 PS 端口读，不碰 PS 表）；事务写法（`Save` 用同一 executor 同一事务写册，`ON CONFLICT DO NOTHING` 与 `acceptance_judgments.go` 同形）；注释全中文、无行号。
  - **结论**：两轴 0 阻断，不立修复票。钉评时 main `4dd937eb` 上各条都未修。其后票 [03](./03-channel-parcel-numbers-from-label-results-void-and-replacement.md) 进 main（至 `d3caf1b0`），接走两条：Spec (2) 由 `58c4c496` 处理（关联键加入形成依据，[ADR-0185](../../../docs/adr/0185-external-identifier-relation-identity-includes-its-formation-basis.md)）；风险点 4「同号不拒」的事实更正由 `83ca983b` 记进 ADR-0184 复核记录（决定原文不动）。这两处修法本次未评。仍待 PS owner 裁：Spec (1) / Standards (1) 去空白进册键；Spec (3) / Standards (4) 决定六与 UC-NO-002 的范围冲突；Spec (4) / Standards (2) 后果节原地改写要不要补一条复核记录。Standards (3) 的计数随下一笔碰到该文件时改。
- **2026-10-18 · 票 [operator-channel/29](../../operator-channel/issues/29-node-reception-records-physical-facts-when-cross-context-reads-fail.md)**：本票评审 Standards (3) 那条（`service_results.go` 包头注「两件」计数）随票 29 去掉。身份核对接真之后同事务读 PS 的失败面，一并按 [ADR-0186](../../../docs/adr/0186-node-reception-records-physical-facts-when-the-service-result-read-fails.md) 处理：身份核对读不动不形成答案（5xx、不落册），服务结果读口经 savepoint 隔离。
- **2026-10-19 · 票 [04](./04-ticket-02-review-follow-ups-records-and-pinned-behavior.md)**：上面评审「仍待 PS owner 裁」的三格——Spec (1) / Standards (1) 去两端空白进册键、Spec (3) / Standards (4) 决定六与 UC-NO-002「权限、隔离与审计」、Spec (4) / Standards (2) 已接受 ADR-0184 后果节被原地改写——已于 2026-10-16 由会话 `idp-parcel-9b` 据用户授权代 PS owner 裁：维持代码与决定六、不立新 ADR，后果节保留现行文字并补记。规则见 [ADR-0184](../../../docs/adr/0184-external-identifier-relations-are-an-append-only-register-formed-with-their-source-facts.md) 复核记录「越权风险点 2 的事实更正」「决定六与 UC-NO-002「权限、隔离与审计」」「后果节在接受后被原地改写的补记」，UC-NO-002 同步两处；唯一误关联另立 [05](./05-a-unique-hit-in-someone-elses-number-space-is-not-enough.md)，一码两物另立 no-reception-identity/01。
