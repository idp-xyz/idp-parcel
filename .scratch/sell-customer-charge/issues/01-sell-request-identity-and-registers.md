# 01 结算：SELL 评价请求身份、包裹计费时点采用册与费用项目映射册

Category: enhancement
Status: resolved——2026-10-19 进 main（至 `c18e694c`）。
Blocked by: 无
地盘：`internal/settlementaccounting` 的 domain、ports、application（只登记那几只）、adapters/postgres、adapters/registrationjson、adapters/identity；`migrations/settlement_accounting/` 0037–0039；`cmd/parcel-settlement-register`；`internal/architecture` 里分区主体、标识前缀两本登记的对应行。
出处：[ADR-0190](../../../docs/adr/0190-a-parcel-billing-moment-requests-a-sell-evaluation-that-forms-the-estimated-customer-charge.md) 决定一、三、五（映射册那一半）。

## 做什么

1. **SELL 评价请求**（决定一）：领域 `SellEvaluationRequest`。
   - 铸造标识，新前缀，过标识前缀门禁。自然键（租户、包裹、计费时点）。
   - 字段：租户、包裹、结算账户、结算币种（登记时从不可变的结算账户行快照）、计价范围、计费时点、来源事实引用、计价基准时点。
   - 册 `sell_evaluation_request`（迁移 0037；号被占就取下一个空号），状态格留给形成那一半写：已提交、已形成、待判断、不可计价、冲突；已形成时另记那笔预估客户费用的标识。
   - 端口：登记（同自然键答已存在）、按标识读、按包裹读当前请求。
   - Outbox 交接：信封 `settlement-accounting.sell-evaluation-request.submitted`，载荷只有（租户、请求标识），ID 走 `outboxintent.FingerprintEventID`，分区键（租户/包裹）。
2. **SELL 计费时点采用册**（决定三，迁移 0038）：键（租户、结算账户），值（计费时点、计价范围），不可变元组，照受理前估价采用册的先例。计费时点是封闭词：`NETWORK_INTAKE_ADOPTED`、`EFFECTIVE_DELIVERY_FINALIZED`。
3. **费用项目映射册**（决定五，迁移 0039）：键（租户、计价费用代码），值（费用项目、登记依据），只增不改。
4. 两册的登记用例、批文译装与 `parcel-settlement-register` 两条命令（`sell-billing-moment-adoption`、`fee-item-mapping`）。没登记的读答未找到，不给默认。

## 不做

- 不发起请求、不消费任何信封（票 02、04）。不碰计价。

## 完成判据

- 真库用例：三册的登记、幂等、租户隔离、无事务拒（架构门禁要的负向证据）。
- 登记口的译装与路由用例；分区主体、标识前缀两道门禁绿。

## 完成记录

2026-10-19 Claude Code 作者会话在分支 `claude-sell-01` 落地，两笔提交（rebase 到 main 之后）：`a5e7bed3`（请求身份、请求册与交接）、`223447c4`（两本租户取值册与两条命令）。证据级别：隔离合成，只记 `S`。

### 做了什么

- **SELL 评价请求**（决定一）：领域 `SellEvaluationRequest`／`SubmitSellEvaluationRequest`，铸造标识前缀 `SELLREQ`（`adapters/identity` 的 `SellEvaluationRequestIdentities`），自然键（包裹、计费时点），租户在册键上。各格缺一不成。包裹计费时点是封闭词 `ParcelBillingMoment`（`NETWORK_INTAKE_ADOPTED`、`EFFECTIVE_DELIVERY_FINALIZED`）。
- **请求册**（迁移 0037 `sell_evaluation_request`）：端口 `SellEvaluationRequestRegistry`（Save、FindByNaturalKey）与 `SellEvaluationRequestView`（FindByID、FindCurrentByParcel），同一个适配器 `SellEvaluationRequests`。撞主键或撞自然键都答`已存在`、不覆盖。
- **交接**：`OutboxSellEvaluationRequestHandoff`，类型 `settlement-accounting.sell-evaluation-request.submitted`，载荷只有 `tenantId`、`sellEvaluationRequestId`，ID 走 `outboxintent.FingerprintEventID("sell-evaluation-request", 租户, 请求, "submitted")`，分区键 `租户/sell-evaluation-request/包裹`，在分区主体表登「租户/包裹」并带裁段。信封被框架确定性拒收时交回 `ports.ErrSellEvaluationRequestHandoffRejected`。
- **SELL 计费时点采用册**（决定三，迁移 0038）：`SellBillingMomentAdoption`，照受理前估价采用册的不可变元组与账户外键。
- **费用项目映射册**（决定五，迁移 0039）：`FeeItemMapping`（计价费用代码 → 费用项目、登记依据），只增不改。没登记的读答未找到，判待判断归票 04。
- 两本册的登记用例（`application/register_sell_customer_charge_registers.go`）、批文译装（`registrationjson/sell_customer_charge_registers.go`）、`parcel-settlement-register` 的 `sell-billing-moment-adoption` 与 `fee-item-mapping` 两条命令。两本都不交信封：没登记时那一拍已入账不重投，补登之后没有停着等它的东西。

### 评审途中按协调方口径做的调整

协调方转来 ADR-0190 评审结果，本票照做，ADR 文字由协调方在 `claude-sell01` 上改：

1. 请求加一格**结算币种**，登记时从不可变的结算账户行（ADR-0158）快照，计价建快照时取它，不再另读账户册。
2. 请求册的**状态格**登记时是 `SUBMITTED`，形成那一半改记为 `FORMED`、`PENDING_JUDGMENT`、`UNRATABLE`、`CONFLICT` 之一；另一列 `customer_charge_id` 只在 `FORMED` 时非空（CHECK 配对，外键指 `customer_charge`），按包裹读就能找到当前有效的预估客户费用。Save 只收`已提交`、不带费用标识的记录。领域侧状态词表 `SellEvaluationRequestState` 已立，改记的写口归票 04。
3. 映射册值保持（费用项目、登记依据）；读口查不到答未找到，不在册这一层答未配置。

### 偏离与取舍

- **复用而不新立**：SELL 请求 Save 的落点复用 BUY 的 `ports.EvaluationRequestSaveOutcome`（写入代数同一条）；两本册的落点复用 `ports.CatalogueRegistrationEffect`，没有照受理前估价采用册另立一个三值类型。计价范围复用 `domain.EstimatePricingScope`（ADR-0190 决定四与受理前估价同一口径，已改它的头注），来源事实引用复用 `domain.SourceFactReference`。
- **新增的拒收哨兵** `ErrSellEvaluationRequestHandoffRejected` 票面没写：票 02 的消费者要靠它分出「确定性拒收，入账不重投」与「依赖故障，重试」。
- **分区键带段**：主体是包裹，但字符串带 `/sell-evaluation-request/` 段，不与 PS 采用链、VE 各口的包裹分区共队。
- 请求册对结算账户暂不设外键，账户存在由票 02 的链保证。评审指出：BUY 请求册本没有账户列，不是这里的先例；带账户列的册都设了外键。外键（或带币种的组合外键，先要在结算账户册上加对应唯一键）留作后续，请求册与票 02 的用例到时要先登账户。
- `ports/settlement_catalogue.go` 里 `CatalogueRegistrationEffect` 头注原写着一个已失效的册数，顺手去掉了数。
- 迁移号 0037–0039 提交时在本机各分支上都空着，未改号。
- rebase 之后 `internal/architecture/envelope_id_bound_gate_test.go` 已在基线上，本票交接口调用 `outboxintent.FingerprintEventID`，那道门禁实跑通过。

### 裁定（协调方，2026-10-19）

不立「评价未成形」那一格。计价侧没形成评价时不发评价已记录信封，结算这边什么都收不到，请求诚实地停在「已提交」。结算能看到的只有已记录的评价：待判断、不可计价、冲突。ADR-0190 决定五与票 04 的措辞随之对齐。

### 验证

DSN `postgres://parcel:parcel@127.0.0.1:55432/postgres?sslmode=disable`，在 rebase 前的 tip 上：

- `go test -p 1 -count=1 ./internal/settlementaccounting/... ./cmd/parcel-settlement-register/ ./internal/architecture/ ./migrations/...` 全部 `ok`。本票新增的真库用例逐条 `PASS`，没有 `SKIP`。
- `go vet` 上述各包退 0，`gofmt -l` 空，`go build ./...` 退 0；第一笔单独检出也能 build、vet。
- 新增用例：领域（时点封闭、请求各格缺一不成、自然键只取包裹与时点、状态词表与「只有已形成带费用」、采用元组、映射三格）；真库请求册（三口往返、同自然键另一 ID 答已存在且先到者不被覆盖、换时点另一份且按包裹读答最晚的、租户隔离、无事务拒、键与对象不符拒、登记冒充已形成拒、状态格与费用标识的 CHECK 和外键）；真库交接（契约字面、请求行与信封同生同灭、回滚两样都不留、重发不翻倍、无事务拒、按包裹同区、超长确定性拒收、缺件响亮报错）；两本册真库（登记、重放、冲突不覆盖、账户不在册上外键拒、租户隔离、无事务拒）；译装正反例；用例层透传；CLI 命令表与真库登记全链。

## 评审与进 main 记录（2026-10-19）

- 隔离子代理一次过两轴，结论如下：
  - 代码轴无阻断。
  - 唯一的阻断是机制清点没重生成，已在干净检出上重生成并随分支提交。那笔提交信息里钉的是 rebase 前的 `cc3a7485`；之后的 rebase 只跨过文档，代码树不变。
  - 已修的非阻断：完成记录里的 SHA、外键先例与门禁说法；「评价未成形」不另立一格的裁定；分区键头注的理由。
  - 留作后续：外键、参数登记册指针（随票 05）、登记口的冲突答复。
- 全量 `go test -p 1 -count=1 ./...`（带 55432 DSN）在干净游离检出上钉 `230fe987`：143 包 ok，0 FAIL，0 SKIP。之后只有注释与文档修订，加上 rebase 跨过的文档，没重跑。
- 快进推到 main：`92dc263e..c18e694c`。
