# 11 包裹终局的跨交易判断没有承接执行器

Category: enhancement
Status: resolved——MCP-3（2026-09-04，基线 `acb4975`；实现 `0e5a4ea`，机制清点 `754c868`；见文末 `## Answer`）
Blocked by: 06（已 resolved，阻塞解除）

## 缺口

`internal/parcelshipment/domain/label_transaction.go` 的聚合注释点名：包裹终局的判断**不在
聚合内**——它要跨该包裹的全部相关交易，以及实际承运商的收寄事实。今天没有任何执行器承接它。

聚合能答的只有单笔交易的定案（`Finalized()` 是派生谓词）。一个包裹可能经历重试与替代
（`EstablishPriorLabelTransactionLink` 在新交易出生时建立前后关系），**「这个包裹到底成没成」
不是任何一笔交易能单独回答的问题**。

## 为什么被 `06` 阻塞

跨交易判断的输入是那条链上各笔交易的结果，而写侧执行器链（`06`）就是产生这些结果的地方。
在它之前，跨交易判断只能对着一组永远为空的交易作答。

## 做什么

> 2026-09-04 按 PS `CONTEXT.md` 改写（`unresolved-review-20260904/report.md` A 组核实：票面原问的「与既有终局是不是
> 同一个」CONTEXT 已正面答）。原文只有一句「定跨交易判断」，下面是按 CONTEXT 三处原文落成的形状。

**同一个概念，不同的形成规则集。** CONTEXT 词条「终局服务结果」：「网络服务和面单渠道服务可以具有不同终局结果，实际
交付不是所有服务形态的统一完成条件」。面单渠道服务的形成规则集由 CONTEXT 规则「首个面单渠道服务产品默认跨同一包裹
的全部相关面单交易判断终局……」与生命周期节三条转换给出，本票把它们落成一个执行器：

1. **首次有效收寄即终局**（非取消终局结果）——「除已经形成的有效取消结果外，实际承运商首次有效收寄即形成终局」；
   生命周期节「`transport-fulfillment` 提供实际承运商首次有效收寄事件 → 面单渠道服务非取消终局结果：该服务不统一
   等待实际交付」。输入是 TF 的外部承运轨迹事实（票 `16`），**只引用不复制**。
2. **受控关闭路径 · 终局失败结果**——「当前受控关闭已经生效且未被重开，全部相关面单交易均已定案为明确失败，并且
   不存在有效或结果待确认的面单结果 → 终局失败结果」。
3. **受控关闭路径 · 终局服务结果**——「当前受控关闭已经生效且未被重开，全部相关面单交易均已定案，不存在仍可使用或
   结果待确认的面单结果，并且已有成功结果均已成功作废或依据接受时固定的规则不可逆失效 → 终局服务结果」。
4. **不形成**的各格照 CONTEXT 逐句：「交易级失败不能推导包裹失败，单笔交易中的包裹级失败、渠道作废或失效也不自动形成
   包裹终局」；「受控关闭只将包裹级继续尝试判断派生为不允许新增尝试……不直接形成包裹终局」；「渠道退款和对账不作为
   包裹终局的前置条件」；「任何已经实际形成且归属该包裹的面单结果都必须参与终局判断」（含违反截断边界的边界后交易）。
5. 「客户合同明确的其他面单服务终局边界成立 → 非取消终局服务结果」是合同实例（`PAR-COM-17`），不在本票的默认规则集里
   ——它走既有 `FinalRuleView` 的合同声明，本票只留位置不填值。

输入三样都已存在：该包裹全部相关面单交易（票 `06`/`10` 的 `LabelTransactionRepository` 登记册）、包裹级继续尝试判断
（票 `10` 的 `ContinuedAttemptRegister.Judge`——「受控关闭已经生效且未被重开」就是它的`受控关闭`一格，本票不另判）、
TF 首次有效收寄事实（票 `16`，`ExternalCarrierTrackingFact`，PS 只持引用与版本）。

## 与既有终局的关系（工程选择及理由）

**结论：复用 `FormParcelFinalHandler`，加两种来源种类；跨聚合谓词由并列的判断执行器在上游算出，不塞进 `FinalRuleView`。**

- **为什么必须落进同一个终局**：委托完成派生（`deriveCompletion`）、取消前的终局核验（UC-PS-006「重新核验当前有效收寄
  及终局事实」）与包裹级继续尝试判断（`Judge(currentFinalPresent)`）都从 `FinalOutcomeStore.FindCurrentFinal` 读「当前
  有效终局」。面单渠道服务的终局若另存一处，这三处都看不见它——一件已经首次收寄的面单包裹会让委托永远停在部分完成、
  会被取消、会被重开。CONTEXT 也没有第二个词：两种服务形态的产物都叫「终局服务结果」。
- **为什么 `FinalRuleView` 装不下跨聚合谓词**：它的形状是（来源身份，一份责任结果）→ 该产品/合同下算不算终局、什么类型、
  哪版规则。「全部相关面单交易均已定案且不存在有效/待确认面单结果」是对一组交易聚合加一册决定的判断，不是对一份结果的
  判断；硬塞进去等于让 PC 的规则声明替 PS 做 PS 自己的判断（CONTEXT 规则：「`parcel-shipment` 必须跨该包裹全部相关交易
  及实际承运商收寄事实形成包裹级判断」）。所以谓词是 PS 的判断执行器 `JudgeLabelServiceFinalHandler` 的事；它把判断结果
  作为一份 `ResponsibilityOutcome` 交给既有采用编排，`FinalRuleView` 保持原形状，只答「这一格在该产品下算不算终局」。
- **两种新来源种类**（`ResponsibilityOutcomeKind` 封闭集加两行，词取 CONTEXT 生命周期节原文）：`LABEL_SERVICE_OUTCOME`
  （面单渠道服务非取消终局结果——首次有效收寄，或关闭路径下成功结果均已作废/失效）与 `LABEL_SERVICE_FAILURE`（终局失败
  结果——关闭路径下全部明确失败）。两份引用都是真引用：**决定** = PS 自己的面单服务终局判断（由包裹、触发格与证据定值的
  判断引用——CONTEXT 把这一判断判给 PS），**执行证据** = TF 首次有效收寄事实（引用@版本）或生效的受控关闭决定；**来源版本**
  = 证据的版本。幂等键因此仍是（租户+包裹+来源种类+来源版本），同一事实/同一关闭决定重放返原。
- **不改既有终局的定义**：CONTEXT 一字不动；`ParcelFinalOutcome` 的形状不动；改的是 UC-PS-004 的来源矩阵（加面单渠道两行）
  与库表 `final_outcome_kind_closed` 的封闭集（新迁移），属「改行为 → 改 UC」，不是改定义，不走 ADR。
- **PC 半边今天答不出这两格**：`FinalRuleView` 的适配器（`adapters/partycommercial/service_stage_rules.go`）把责任结果
  译成 PC 的 `DeclaredResponsibilityOutcome`，而那个封闭集只有网络服务四行。面单渠道服务的两格在适配器里如实答
  「终局规则未配置」（found=false → 编排未决 `FINAL_RULE_UNCONFIGURED`），不错报为「此产品下不形成终局」也不吸收成
  某个网络格。判断本身（机制半边）已可测可用；形成终局等 PC 声明词汇表加两行（PC 地盘，另立票）。这与首发基线
  「独立面单渠道服务不进入首发生产」一致。

## 红线

- 不新造领域语言：终局的词与格数取 PS `CONTEXT.md` 与既有 `form_parcel_final.go` 的原词。
- 承运商收寄事实的所有权不在本上下文，**只引用不复制**。
- 若发现要改既有终局的定义，停下走[改文档](../../../AGENTS.md#改文档)，不在实现票里改。

## 完成判据

判断有执行器与测试；与既有终局的关系有明确答复并写在票面；`gofmt -l` 空、
`go build`/`go vet` 退 0、`go test -count=1 ./...` 绿并注明含不含真库。

## 参照

[能力形状盘点](../capability-shape-inventory.md)第一段；ADR-0084；票 `06`。

## Answer

2026-09-04，MCP-3。实现 `0e5a4ea`（父 `21749ef`），机制清点重生成 `754c868`。

**落了什么**

- `domain/label_service_final.go`：`JudgeLabelServiceFinal` 跨全部相关面单交易、登记册的生效关闭与 TF 首次有效收寄事实
  引用形成封闭五格判断（`FINAL_BY_FIRST_PICKUP` / `FINAL_OUTCOME_BY_CLOSURE` / `FINAL_FAILURE_BY_CLOSURE` / `NOT_FINAL` 带
  三种原因 / `CANCELLATION_STANDS`），规则逐句取 CONTEXT；`CarrierFirstEffectivePickup` 只持 TF 事实的引用@版本与有效时间。
  `ResponsibilityOutcomeKind` 加 `LABEL_SERVICE_OUTCOME` / `LABEL_SERVICE_FAILURE` 两行；登记册开 `StandingClosure` 读法。
- `application/judge_label_service_final.go`：`JudgeLabelServiceFinalHandler` 读四个只读口（按包裹整册取交易、登记册只读半边、
  取消视图、有效期规则），判出终局的格译成责任结果交既有 `FormParcelFinalHandler`；不形成与取消在先不走采用。
- `ports`：`LabelTransactionsByParcelView`、`LabelValidityRuleView`、`ContinuedAttemptRegisterView`。
- `adapters/postgres`：`LabelTransactions.ListByCoveredParcel`（快照 coveredParcels 的 jsonb 包含查）；迁移 `parcel_shipment/0014`
  放宽 `final_outcome_kind_closed` 两值并建 GIN 索引。
- `adapters/partycommercial/service_stage_rules.go`：面单渠道两格如实答「终局规则未配置」（PC 词汇表没有面单行）。
- `docs/application/parcel-shipment/UC-PS-004`：来源矩阵加面单渠道两行、「面单渠道服务的终局形成」一节、`AT-PS-094..100`。

**完成判据逐条**

- 判断有执行器与测试：领域层五格与三种不形成原因、多包裹交易按本包裹结果判、作废范围不外溢、重开解除关闭、异包裹输入拒；
  应用层收寄采用（同版本重放返原）、关闭路径三支、取消在先、规则未配置停未决、四读口故障分格、封闭原因集、依赖无写口；
  真库用例按包裹整册取交易（不筛、租户隔离）与面单渠道来源的终局落库并作为当前有效终局读回、集合外种类被 CHECK 拒。
- 与既有终局的关系：**同一个终局**（`FinalOutcomeStore` 同一处当前有效终局），不同的形成规则集——理由在上文「与既有终局的
  关系」一节；`FinalRuleView` 形状不动，跨聚合谓词由判断执行器在上游算。
- 验证：`0e5a4ea` 的 detached 检出上 `gofmt -l` 空、`go build ./...` / `go vet ./...` 退 0；`go test -count=1 -p 1
  ./internal/parcelshipment/... ./internal/architecture/ ./migrations/ ./internal/platform/migrate/ ./cmd/...` 全绿（含真库，
  DSN 已设，PS postgres 包耗时数十秒且新用例 `-v` 为 `PASS`）；机制清点在同一检出上重生成为 `754c868`。全仓 `./...` 由推送方
  在 tip 兑底。

**不在本票（各有去处，都是接线的下一层）**

- **谁调 `JudgeLabelServiceFinalHandler`**：三个触发点今天都没有调用方——TF 首次有效收寄事实到达（`external-carrier-tracking`
  信封的 PS 侧 inbox 消费者，`adapters/inbox/*` 不在本票地盘）、面单交易定案（票 `06` 的 `operate_label_transaction.go` 记下
  结果那一拍）、受控关闭/重开决定生效（票 `10` 的决定口，那口自身还没有生产写入方）。三处各自一张接线票；命令形状已定，
  调用方只需带来源身份、委托、包裹与（收寄那一路）事实引用。
- **PC 半边**：`DeclaredResponsibilityOutcome` 加面单渠道两行并在 `service_stage_rules.go` 补逐格翻译（删掉本票那一格）。
  PC 地盘，建议另立票；落地前面单渠道终局的采用一律停在 `FINAL_RULE_UNCONFIGURED`。
- **有效期规则读口的适配器**：接受时固定的面单有效期规则是实例登记面，无 PAR 编号，随首个面单渠道产品的实例登记一起立。
- `cmd/parcel-api` / `cmd/parcel-dispatch` 无装配：与上面第一条同落。

## Comments

- 2026-09-10 · 通道 3（task-b5dba034，取证锚 `c7e3522c`；只写票面，未动代码，本票 Status 不改）：**「不在本票」第一条的三张接线票已立**——[`25`](./25-external-carrier-first-pickup-triggers-label-final-judgment.md)（TF 首次有效收寄到达）、[`26`](./26-label-transaction-settlement-beat-triggers-label-final-judgment.md)（面单交易定案那一拍）、[`27`](./27-controlled-close-reopen-decision-triggers-label-final-judgment.md)（受控关闭 / 重开决定生效）；第四条「`cmd/` 无装配」与 lc/12 的组合根一并归 [`28`](./28-channel-selection-composition-root-and-call-entry.md)。**一处口径更正**：本 Answer 把第一路写成「`external-carrier-tracking` 信封的 PS 侧 inbox 消费者」，票 `25` 量出那封信（`transport-fulfillment.external-carrier-tracking.judged`）按 TF CONTEXT「外部承运轨迹事实」Rules **不构成**实际承运商首次有效收寄，TF 今天也没有收寄那一类事实与信封——PS 侧消费者等的信要 TF 先立，读到本 Answer 那句的人以 `25` 为新。第三条「有效期规则读口的适配器」已由 ps-port-remainder/01 承接（PC 半边 pc-gaps/09 已进 main，PS 半边 2026-09-10 派通道 6），本 Answer「无 PAR 编号」那句以 ps-port-remainder/01 的 `PAR-COM-17` 口径为准（那张票 Comments 已记）。
