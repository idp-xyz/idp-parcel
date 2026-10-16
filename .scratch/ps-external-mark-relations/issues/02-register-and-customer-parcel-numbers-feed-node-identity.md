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
