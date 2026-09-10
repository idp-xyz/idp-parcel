# 29 择优结果 → 面单交易七类依据引用的翻译适配器：候选标识译成 `Establish` 要的账号 / 持有人 / 服务方 / 结算相对方 / 合同 / 费率 / 责任依据

Category: enhancement
Status: ready-for-agent——2026-09-10 17:5x 通道 1 推送方对四条「要裁的」逐条裁（全取默认）并补强两条（「择优结果」对象由本票定义并产出；PC 取不到的那格落为新对象的字段），三条越权风险点记「裁决」尾供 owner 复核；通道 3 照写（task-8ede4724），「做法」按裁决写实，本票再无待裁问题。此前 draft——通道 3 于 2026-09-10 17:3x 按通道 1 裁决（[`28`](./28-channel-selection-composition-root-and-call-entry.md)「要裁的」2 另立，task-bc04bfc2）立票，取证锚远端 main `062f5228`；只写票面，未动代码；「七类引用各从 PC 哪个对象取」以代码为准逐格取证（见「取证」），四格答得出、三格答不出
Blocked by: 无（`28` Blocked by 本票；本票定义「择优结果」对象，`28` 只接线——不成环）

## 从哪里来

`28` 立票时顺带量到：择优编排 `SelectChannelCandidateHandler.Handle` 交回的是**一个候选标识** `ChannelCandidateID`，而票 [`06`](./06-label-transaction-write-side-executors.md) 的 `EstablishLabelTransactionCommand` 要七类依据引用（`ChannelAccount` / `AccountHolder` / `ServiceProvider` / `SettlementCounterparty` / `Contract` / `Rate` / `ResponsibilityBasis`），中间没有适配器。通道 1 裁另立：地盘不同（PS `adapters/partycommercial/` vs `cmd/` 组合根），且它读的全是 PC 对象、写开时会冒 B 类问题，不该挡组合根的形状。

按 [ADR-0025](../../../docs/adr/0025-cross-context-adapters-live-on-the-consumer-side.md) 落 PS `internal/parcelshipment/adapters/partycommercial/`，与同目录 `channel_candidate_assembly.go`（票 `12`）、`commercial_basis.go`（商业闭包）同族：两套词汇之间的翻译只许在这一层发生，编排住应用层、门禁不许它导入 PC。

## 取证（`062f5228` 上量，逐符号名）

**候选标识是什么**：`channel_candidate_assembly.go` 的 `AssembleChannelCandidates` 对映射 `CandidatesAllowedBy` 交回的每个 `pcdomain.ChannelProductReference` 做 `psdomain.NewChannelCandidateID(channel.String())`——候选标识的字面就是 PC 的**渠道产品引用**。它「指向渠道服务方提供的服务……既不是运营企业自己的服务产品，也不是任何一次运输的实际承运商」（`service_product.go` 头注）；渠道本体不在 PC 预造（ADR-0072），PC 里持有它的只有三处：`ServiceProduct` / `ProductChannelBinding` / `ChannelAccountUseAuthorization.channel`。

**七类引用各从哪取**（PS 侧类型见 `label_transaction.go` 各头注）：

| `Establish` 的格 | PS 类型头注说它是什么 | PC（或别处）能取到的对象 | 今天的读口 | 答得出？ |
|---|---|---|---|---|
| `ChannelAccount` | 发起本次渠道业务请求所用的渠道账号 | `ChannelAccountUseAuthorization.account`（`ChannelAccountID`）——该授权的 `channel` 等于候选的渠道产品引用、`grantee` 是运营企业、`scope` / `effective` 覆盖本次 | `ChannelAccountUseAuthorizationRegistry.LoadLatest(tenant, authorizationID)` 点读；`LoadAuthorizedAccountUse(tenant, account)` 按账号列。**没有按渠道产品 + 范围 + 时点反查的读口** | 对象答得出；「哪一条授权」要一个选法（要裁的 1） |
| `AccountHolder` | 渠道账号持有人 | 同一条授权的 `grantor`（`PartyID`） | 随上 | 随上 |
| `ServiceProvider` | 承接本次请求的渠道服务方 | PC 没有渠道产品本体，`ChannelAccountUseAuthorization` 不带服务方 party；唯一带 party 的采购侧对象是 `SupplierAgreement.supplier`（`PartyID`） | `SupplierAgreementContentView.LoadSupplierAgreement(tenant, version)` 点读——要先知道哪一版 | **答不出**：来源归属要裁（要裁的 2），选法要裁（要裁的 1） |
| `SettlementCounterparty` | 合同与结算相对方 | `BUY` 方向的相对方 = `SupplierAgreement.supplier`；协议的 `legalEntity` 是运营企业自己的法人（我方），不是相对方 | 随上 | **答不出**：同上（要裁的 2、1） |
| `Contract` | 本次交易所依据的合同，本体属 PC | `SupplierAgreement.Version()`（`CommercialVersion`，kind `SupplierAgreementObject`）。**PC 今天没有「渠道产品 → 供应商协议」的绑定**：`SupplierAgreement` 只有 supplier / legalEntity / scope / purchasePlan / effective，`ProductChannelBinding` 只带渠道产品引用 | `LoadSupplierAgreement` 存在，选版本的口不存在 | 对象答得出；「哪一版」要选法（要裁的 1） |
| `Rate` | 本次交易适用的费率 | 两处候选：(a) PP `BUY` 评价——`ChannelBuyPlanSource.BuyPlanFor(query, candidate)` 交回 `ppdomain.PlanEvaluationTarget`（评价标识 + 价卡版本），成本取值 `ChannelCandidateCost.evaluation`（`ChannelCostEvaluationReference`）指回评价；(b) PC `SupplierAgreement.PurchasePricingPlan()`（`PricingPlanReference`）——头注明写它「永远不是面向客户的可执行价格」，是方案不是费率 | (a) 在 `adapters/parcelpricing/`，不在本包；(b) 随协议点读 | 对象答得出，**取 (a) 还是 (b) 要裁**（要裁的 3） |
| `ResponsibilityBasis` | 建立时固定下来的渠道角色与责任依据快照 | **PS 自己的**，不是 PC 的。今天 PS 只有接受时的 `CommercialBasisSnapshot`（`CommercialResolutionID` + 规则包 + 视图修订 + 各时点 + 结算条款），**不含映射 / 授权 / 交易角色**；PC CONTEXT「后续发生渠道选择、面单交易……再固定该次决定实际使用的映射和授权依据，并保留它们与委托接受时快照的关系」那半在 PS 领域里没有对象 | 无 | **答不出**（要裁的 4） |

**同目录先例——「这次该用哪一份」一律是消费方的实例半边源**：`ResolutionKeySource.FormResolutionKey`（商业闭包解析键）、`ChannelConstraintSource.ChannelConstraintFor`（渠道约束）、`PricingInputSource.PricingInputFor` 与 `ChannelBuyPlanSource.BuyPlanFor`（`adapters/parcelpricing/`）。四处同形：接口留在适配器包不进 `psports`，第二返回值 `false`（或零值）= 显式未配置，适配器据以停下、不代拟。

## 做法（按 2026-09-10 17:4x 裁决写实）

- **「择优结果」对象——本票定义并产出，`28` 只接线**（补强 ①）。落 PS `domain`，值对象，名字作者定（立票时的提名：`SelectedChannelBasis`，「选定渠道的依据」；实施改名写一句理由）。内容三段：候选 `ChannelCandidateID`；七类依据引用，与 `EstablishLabelTransactionCommand` 七格同型；评价痕迹引用 `ChannelCostEvaluationReference`，可缺席（没登记价卡的候选没经过评价）。构造走 Spec 结构体（≥5 入参，本包分界线），七格任一空白即拒；它就是裁决 (4) 说的那份**新造的「渠道角色与责任依据快照」**——载体是面单交易上的七格引用，不另起表、不复制 PC 正文。
- **七格各从哪取**（裁决 (1)(2)(3)(4) 落实）：
  - `ChannelAccount` / `AccountHolder` ← 实例半边源 `ChannelAccountUseSource.AuthorizationFor(query, candidate) (pcdomain.ChannelAccountUseAuthorizationID, bool, error)` 答「哪一条」→ `ChannelAccountUseAuthorizationRegistry.LoadLatest` 取回 → 核 `channel` 等于候选、`grantee` 是运营企业、`scope` 与 `effective` 覆盖 `query.At`、状态未撤销——任一不满足即停、不换一条 → `account` / `grantor` 译成 PS 引用。
  - `Contract` / `ServiceProvider` / `SettlementCounterparty` ← 实例半边源 `SupplierAgreementSource.AgreementFor(query, candidate) (pcdomain.CommercialVersion, bool, error)` 答「哪一版」→ `SupplierAgreementContentView.LoadSupplierAgreement` 取回 → 核 `Effective()` 覆盖 `query.At`、未终止 → `Version()` 译 `Contract`；`Supplier()` **同一个 `PartyID` 填服务方与结算相对方两格**，两格的头注都写明「同源是首发的实例事实，不是类型上的同义——PC CONTEXT 要求分别表达，代理 / 分包场景下会不同，届时 PC 立第二格、这里改取处」（裁决 (2)）。
  - `Rate` ← **由择优步带出，本票不问 PP**（裁决 (3)）：择优编排交回的不再只是 `ChannelCandidateID`，还带选中候选的评价目标（`PlanEvaluationTarget`：评价标识 + 价卡版本）——评价标识落评价痕迹引用那格，价卡版本落 `Rate`。择优编排的交回值怎么拓（另加一个方法、或交回一个带两格的小结构），作者定，走 expand 不改旧签名——`Handle` 的调用点今天只有测试与 `28` 将写的前置步。
  - `ResponsibilityBasis` ← **接受时商业解析回指**（裁决 (4) 的读法，通道 3 照句读）：这一格填委托接受时 `CommercialBasisSnapshot` 的 `CommercialResolutionID`，让新快照「与委托接受时快照的关系」由它保留；前六格 + 候选就是「该次决定实际使用的映射和授权依据」。它**不是**把接受时快照当作责任依据快照——那份不含映射 / 授权 / 角色，顶替不了；它是新快照七格里的回指一格。接受时解析标识从哪来：`query` 今天没带委托引用（`ChannelSelectionSubject` 头注原句「编排的入参里没有面单交易或包裹的引用」），所以由第三个实例半边源 `AcceptanceResolutionSource.ResolutionFor(query) (psdomain.CommercialResolutionID, bool, error)` 答，未配置即停——这一格与前两源同形，是本票量到的第三处「这次该用哪一份」。`ResponsibilityBasisSnapshotReference` 的头注随笔改一句（「回指接受时商业解析；快照本体是本交易七格」），只改注释不改类型。
- **端口**（PS `ports`，编排看得见）：`ChannelSelectionBasisTranslator.TranslateSelectedCandidate(ctx, query psports.ChannelSelectionQuery, selected <择优交回的候选 + 评价目标>) (psdomain.SelectedChannelBasis, error)`。停下的格各自具名、不折成一格：授权未配置 / 协议未配置 / 接受时解析未配置 / 授权渠道不等于候选 / 授权不在有效期或已撤销 / 协议不在有效期或已终止 / 译不过去（照 `ErrUntranslatableQuery` / `ErrUntranslatableAnswer` 的方向区分：查询出不去 vs 答复进不来）。
- **适配器**（`adapters/partycommercial/channel_selection_basis.go`）：把 `psports.ChannelSelectionQuery` 与候选译成 PC 键（照 `providerKeysOf`）；三个实例半边源接口留在本包不进 `psports`（ADR-0025 协作者接口留在适配器包内，同 `ResolutionKeySource`）；只走 `pcports` 读口，不读 PC `adapters/postgres`。
- **不做的**：不在本包判候选该不该赢（票 `01` 裁决）；不复制映射、授权或协议的任何规则；不读凭据（ADR-0039）；不在 PC 加反查读口或绑定（裁决 (1)：PC 的 B 类题，不在本票开）。

## 要裁的（各带默认；派单方一句裁下即可）

1. **「这个候选适用哪条账号使用授权、哪一版供应商协议」两问怎么答。** (a) 消费方实例半边源，照同目录四处先例：`ChannelAccountUseSource.AuthorizationFor(query, candidate) (pcdomain.ChannelAccountUseAuthorizationID, bool, error)` 与 `SupplierAgreementSource.AgreementFor(query, candidate) (pcdomain.CommercialVersion, bool, error)`，未配置即停，再用 PC 既有 `LoadLatest` / `LoadSupplierAgreement` 点读并核；(b) PC 加反查读口（按渠道产品 + 范围 + 时点选授权）或加一个「渠道产品 → 供应商协议」绑定对象——PC 地盘，B 类。**默认 (a)**：今天没有租户，选法本来就是实例半边；(b) 等真有第二个消费方要同一问时再议。
2. **`ServiceProvider` 与 `SettlementCounterparty` 是否都取所选协议的 `Supplier()`。** PC CONTEXT「渠道账号持有人、渠道服务方、合同与结算相对方……必须分别表达」——分别表达不等于必然不同值；首发用同一 `PartyID` 填两格是最省的，但服务方另有来源（渠道产品本体）时会错。**默认：两格都取 `Supplier()`，头注写明「同源是首发的实例事实、不是类型上的同义」**；B 类，因为它定的是两个角色的来源归属。
3. **`Rate` 取 PP 评价的价卡版本还是 PC 协议的采购方案。** **默认取 (a)**：`PlanEvaluationTarget` 的价卡版本——那是这笔交易真正按之出价的费率，方案不是费率；且择优时 `ChannelBuyPlanSource` 已经问过一遍，不必再问。后果：本票的翻译跨了 PC 与 PP 两个提供方，按 ADR-0025 拆成两个适配器包（`adapters/partycommercial/` 出六格、`adapters/parcelpricing/` 出 `Rate`）由组合根拼，或让 `28` 的「择优结果」对象在择优那一步就带上评价目标、本票不再问 PP——**后者更省**，但要 `28` 判据 1 的对象多带一格价卡版本引用。
4. **`ResponsibilityBasis` 是什么。** (a) 复用委托接受时 `CommercialBasisSnapshot` 的 `CommercialResolutionID`（今天唯一现成的 PS 快照标识）；(b) 06 建立时新造一份 PS「渠道角色与责任依据快照」（映射 + 授权 + 六类引用 + 与接受快照的关系）并签发引用——PC CONTEXT 那句要的是 (b)，PS 领域今天无此对象。**默认 (b) 的形状由 `28` 判据 1 的「择优结果」对象承担一半**：对象本身就是「该次决定实际使用的映射和授权依据」，本票只需给它一个可回指的标识；与接受快照的关系怎么留，B 类，报 owner。

### 裁决（通道 1 推送方代裁 · 2026-09-10 17:4x · 用户 17:0x 经队列授权「你自决」；由通道 3 照写，task-8ede4724）

1. **选法 → 取默认**：消费方实例半边源（照同目录四处先例；ADR-0025 协作者接口留在适配器包内），显式未配置 → 诚实停点。PC 加反查读口 / 绑定是 PC 的 B 类题，**不在本票开**。
2. **服务方与结算相对方都取 `SupplierAgreement.Supplier()` → 取默认**，头注写明同源非同义。
3. **`Rate` → 取默认 PP 价卡版本，且由择优步带出。** 与 28-1 的形状约束一致：择优结果对象携带评价痕迹引用，费率引用就是那一格，29 不再问 PP。
4. **`ResponsibilityBasis` → 新造快照，不拿接受时 `CommercialResolutionID` 顶替；快照的载体就是面单交易上的七类依据引用（引用不复制正文），其中一格是接受时商业解析回指——与接受快照的关系由它保留。** 判据不是推的，是 PC CONTEXT Rules 原句：「后续发生渠道选择、面单交易或继续尝试决定时，再固定该次决定实际使用的映射和授权依据，并保留它们与委托接受时快照的关系」——两半都在那一句里。

**两条补强**：① 「择优结果」对象**由本票定义并产出**（PS `ports` 或 `domain`，类型名作者定；内容 = 候选 + 七类依据引用 + 评价痕迹引用），`28` 只接线不定义——否则 `28` Blocked by 本票而本票又等 `28` 定形状，成环。② 七类引用里从 PC 取不到的（「责任依据快照 PS 无对象」那格）按 (4) 落为新对象的字段，不留空也不填默认。

**越权风险点（供 owner 复核）**：(a) PC owner——要不要立「渠道产品 → 供应商协议」绑定与按（渠道产品 + 范围 + 时点）反查授权的读口；(b) PC owner——服务方与结算相对方何时会不同（代理 / 分包场景），不同时要在 PC 立第二格；(c) PC owner——要不要在 PC 一侧另记一份「该次决定实际使用的」（PS 引用之外）。

## 红线

- 不填任何账号、持有人、供应商、协议版本、价卡、范围、接受时解析标识的取值（实例半边 `PAR-INT-02` / `PAR-COM-10` / `PAR-SET-03`）；三个实例半边源未配置即停，不改成默认放行、不拿「最新一条」顶替「适用的那条」。
- 授权的 `channel` 不等于候选、`grantee` 不是运营企业、`scope` / `effective` 不覆盖 `query.At`、状态已撤销——任一不满足即停并具名，不换一条授权继续；ADR-0039：凭据可达不顶替业务授权。
- 不复制 PC 的任何规则到本包；不读 PC `adapters/postgres`，只走 `pcports` 的读口（同目录先例）。
- 翻译停下时不产生任何面单交易、不写决定记录（择优留痕在择优那一步已写完，本票不再写）。

## 完成判据

1. **「择优结果」对象**在 PS `domain` 落地：候选 + 七类引用 + 可缺席的评价痕迹引用；Spec 构造、七格任一空白即拒、评价痕迹缺席合法（各一条测试）；头注写明「系统择优与日后人工择优都产同一个对象，`28` 的编排不问它从哪来」与「它就是该次决定实际使用的映射和授权依据快照，载体是本交易七格」。
2. **择优步带出评价目标**：择优编排能交出选中候选的评价标识 + 价卡版本（可缺席）；走 expand，`Handle` 旧签名不动；一条测试证选中候选的评价目标与 `Costs` 交回的那一份同源。
3. **翻译适配器**对每一格停点各一条测试（合成替身、合成串）：授权未配置 / 协议未配置 / 接受时解析未配置 / 授权渠道不等于候选 / 授权 `grantee` 不是运营企业 / 授权不在有效期 / 授权已撤销 / 协议不在有效期 / 协议已终止 / 译不过去；一条正路：三源配上 → 七格齐 → 与 `Establish` 的聚合构造门逐格过（`ServiceProvider` 与 `SettlementCounterparty` 同值）。停点各自具名，测试断言到具名错误不看字符串。
4. `ResponsibilityBasisSnapshotReference` 头注改口一句（回指接受时商业解析；快照本体是本交易七格），只改注释。
5. 架构门禁：本包只导入 `psdomain` / `psports` / `pcdomain` / `pcports`，不导入 PP、不导入 PC `adapters/postgres`；`./internal/architecture/...` 全绿（接线基线若因新端口变动，按其头注重数、带 SHA）。
6. `gofmt -l` 空、`go build ./...` / `go vet ./...` 退 0；`go test -count=1` PS `domain` + `application` + `adapters/partycommercial` + `./internal/architecture/...`；不动 postgres 不带 DSN。
7. 完成记录逐笔 SHA、逐格写「从哪个 PC 对象、经哪个读口取」与本票「取证」表对照，写明对象的最终类型名与择优步带出评价目标的形状。

## 地盘

`internal/parcelshipment/domain/`（新值对象一件 + `ResponsibilityBasisSnapshotReference` 头注一句）；`internal/parcelshipment/ports/`（一个端口，只加不改）；`internal/parcelshipment/application/select_channel_candidate.go`（择优交回值 expand，不改旧签名）；`internal/parcelshipment/adapters/partycommercial/`（新文件 `channel_selection_basis.go` + 测试）。**不动**：PC 任何包（裁决 (1)）、PP 任何包、`operate_label_transaction.go` 与 `cmd/`（`28` 的地盘——`28` 接的是本票产出的对象）。

## 参照

票 `28`（组合根；其判据 1 的「择优结果」对象是本票的输出形状）；票 `06`（`EstablishLabelTransactionCommand` 七格）；票 `12`（`channel_candidate_assembly.go`——候选标识的来历与 `providerKeysOf` 的翻译手法）；票 `13`（`ChannelBuyPlanSource` / `PlanEvaluationTarget`）；票 `14`（`ChannelCostEvaluationReference`）；`commercial_basis.go`（`ResolutionKeySource` 先例与 `CommercialBasisSnapshot`）；PC `channel_account_use_authorization.go` / `supplier_agreement.go` / `ports.go`（`ChannelAccountUseAuthorizationRegistry` / `SupplierAgreementContentView`）；PC CONTEXT「渠道账号使用授权」词条与「委托被接受时固定……再固定该次决定实际使用的映射和授权依据」「必须分别表达」两句；PS CONTEXT「面单交易」词条；ADR-0025 / ADR-0039 / ADR-0072。

## Comments

- 2026-09-10 17:3x · 通道 3（task-bc04bfc2，取证锚远端 main `062f5228`；分支 `mcp3-lc28`）：立票。**只写票面，未动代码。** 能力边界：读过 `channel_candidate_assembly.go` 全文、`label_transaction.go` 七个引用类型头注、`EstablishLabelTransactionCommand`、`cost_source.go` 两个取数口、`channel_candidate_cost.go` 的 `evaluation` 格、`acceptance_basis.go` 的 `CommercialBasisSnapshot` 字段、PC `ChannelAccountUseAuthorization` 与 `SupplierAgreement` 的字段与构造门、PC `ports.go` 的 `ChannelAccountUseAuthorizationRegistry` / `SupplierAgreementContentView` / `ProductChannelMappingRegistry` / `CommercialPublicationView`、PC 与 PS CONTEXT 相关词条；**没读** `commercial_basis.go` 的三阶段正文、`label_transaction.go` 的聚合构造门正文、PP `PricingPlanVersion` 的标识形状、TF 侧「运输委托实际采用的协议快照」的写法（要裁的 4 若取 (b) 可照它的形，开工时再读）。「渠道产品 → 供应商协议在 PC 无绑定」这一条是 `grep ChannelProductReference` 在 PC 领域非测试文件只命中三处（`service_product.go` / `product_channel_registration.go` / `channel_account_use_authorization.go`）得出的，`supplier_agreement.go` 不在其中。
- 2026-09-10 17:5x · 通道 3（task-8ede4724）：照写通道 1 四条裁决 + 两条补强 + 三条越权风险点（见「要裁的」下「裁决」），「做法」「完成判据」「地盘」按裁决写实，Status draft → ready-for-agent。**只写票面，未动代码。** 两处是通道 3 对裁决的读法、不是裁决原句，评审时请对：(i) 裁决 (4)「其中一格是接受时商业解析回指」读作 `ResponsibilityBasis` 那格填接受时 `CommercialResolutionID`、其余六格 + 候选是「实际使用的映射和授权依据」——因此多出第三个实例半边源 `AcceptanceResolutionSource`（`query` 今天不带委托引用）；(ii) 裁决 (3)「费率引用就是那一格」读作择优步带出 `PlanEvaluationTarget`、评价标识与价卡版本各落一格，择优编排交回值要 expand。
