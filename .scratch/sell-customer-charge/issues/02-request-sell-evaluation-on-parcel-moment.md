# 02 结算：包裹计费时点到来时发起 SELL 评价请求

Category: enhancement
Status: 完工，待评审与进 main——分支 `claude-sell-02`（2026-10-19 Claude Code 会话立；完工见文末「完成记录」）。
Blocked by: [01](./01-sell-request-identity-and-registers.md)。02、03、04 都改 `cmd/parcel-dispatch` 的路由表，按 02 → 03 → 04 串行做。
地盘：`internal/settlementaccounting` 的 application、adapters/inbox、新目录 adapters/parcelshipment（若无）、adapters/partycommercial；`cmd/parcel-dispatch` 的路由表与装配。
出处：[ADR-0190](../../../docs/adr/0190-a-parcel-billing-moment-requests-a-sell-evaluation-that-forms-the-estimated-customer-charge.md) 决定二、三、六。

## 做什么

1. 编排 `RequestSellEvaluationOnParcelMoment`：
   - 收（租户、包裹、计费时点、来源事实引用、计价基准时点）。
   - 包裹到结算账户走受理前财务控制同一条链（小包托运接受时固定的商业解析 → 参与方与商业闭包的结算政策 → 结算账户册）。
   - 查采用册：没有这一户、或采用的不是这次到来的时点，答未配置，入账不重投。
   - 按自然键登记请求并同一事务交信封；重放答已存在。
2. 两只 inbox 消费者：
   - 收寄采用：`parcel-shipment.network-intake.recorded`。回读采用记录，不采用的略过；基准时点取责任起点。
   - 终局：`parcel-shipment.final-outcome.formed`。回读终局，只认已终局的「有效交付」；基准时点取生效时点。
3. 派发：这两种信封改成扇出，各路记各自的 inbox，各列自己的未决哨兵。未配置、不采用、非有效交付一律入账。

## 完成判据

- 应用层用例：没登记、时点不符、重放、两种时点各一条。
- 真库装配用例：经生产路由表，一封收寄采用信封让一户采用了 `NETWORK_INTAKE_ADOPTED` 的包裹落一份请求、交一封信封；没登记时入账、零行。

## 完成记录

2026-10-19 Claude Code 作者会话在分支 `claude-sell-02`（基于 `claude-sell-01`）落地，三笔提交：`61c971e1`（链与发起编排）、`f99685a6`（两只消费者与派发扇出）、`（rebase 时丢弃，落地时于 tip 上重生成）`（机制清点重生成）。证据级别：隔离合成，只记 `S`。

### 做了什么

- **发起编排** `application/request_sell_evaluation_on_parcel_moment.go`：命令收（租户、包裹、计费时点、来源事实引用、计价基准时点）。顺序：受理 → 包裹到结算账户 → 采用册 → 按自然键找先到者 → 铸造 → 登记 → 交信封。结算币种取账户键上那一格（账户行是不可变元组，按这把键找到的行币种就是它），计价范围取采用册。结果分格：
  - 已发起、已存在：重放交回先到者，再交同一封信封，Outbox 按认领键吞重。
  - 未配置，带原因：闭包没采用结算政策、账户不在册、没登记采用、采用的不是这一拍。
  - 包裹无商业解析：包裹不属任何已接受委托。
  - 未受理：命令缺格。
  - 未决，点名停在哪一口：链上三口、采用册、请求册、铸造口、交接口。
- **包裹到结算账户的链**，与受理前财务控制同一条，三段各一口：
  - `ports.ParcelCommercialResolutionView`，由新包 `adapters/parcelshipment` 的 `ParcelCommercialResolutions` 实现，读 PS 的 `CommercialResolutionReferenceView`。
  - `ports.AdoptedSettlementTermsView`，由 `adapters/partycommercial` 的 `AdoptedSettlementTerms` 实现，读 PC 的 `CommercialResolutionView.LoadResolution`。它与 `CreditBasis` 并列，从闭包取结算政策，法人、相对方、币种取适用范围，政策取对象标识，与 PS 侧 `RegisteredAccountDirectory` 拼账户键的取法一致。
  - SA 自己的 `SettlementAccountView`，方向取应收。新立领域值 `domain.AdoptedSettlementTerms`，四维，`AccountKey(direction)` 拼键。
- **两只 inbox 消费者**（`adapters/inbox`）：
  - `NetworkIntakeRecordedConsumer`，名 `settlement-accounting/request-sell-evaluation-on-network-intake`。
  - `FinalOutcomeFormedConsumer`，名 `settlement-accounting/request-sell-evaluation-on-final-outcome`。
  - 两只都只认采用键四维，缺一即毒丸。
- **两只处理方**（`adapters/parcelshipment/request_on_parcel_moment.go`）：
  - 收寄采用那一路按采用键回读 `IntakeAdoptions.FindByKey`；不采用的入账略过；基准时点取 `Intake.ResponsibilityStart()`；来源事实引用为 `network-intake/<包裹>/<来源类型>/<版本>`。
  - 终局那一路按终局键回读 `FinalOutcomes.FindByKey`；只认已终局且责任结果种类是 `EFFECTIVE_DELIVERY` 的那一份，不认合同给的终局类型引用（那是租户命名）；基准时点取 `Final.EffectiveAt()`；来源事实引用为 `final-outcome/<包裹>/<种类>/<版本>`。
  - 消费两格：已发起、已存在、未配置、包裹无商业解析都入账；未决交续办哨兵。
- **派发**（`cmd/parcel-dispatch/assemble.go`）：
  - `parcel-shipment.network-intake.recorded` 改为扇出：NR 路由复核在前，SA 在后。
  - `parcel-shipment.final-outcome.formed` 改为扇出：VE 投影在前，SA 在后。
  - 两路各列各的未决哨兵名单：可见性那一枚，加 `ErrSellEvaluationRequestUndecided`。
  - 生产装配全接真读口与真册，不放替身。

### 用例

- 应用层：两种时点各发起一份，各格取对来源；没登记、时点不符（两个方向）答未配置，零痕迹；重放答已存在，不另铸、不另登、信封不翻倍；并发败者读回赢家；链上三格的「没有」各归其格；各口故障答未决并点名；确定性拒收整笔报错；缺格未受理；缺依赖构造拒。
- 适配器：
  - PC 那一段走真库：落库读回四维，政策取对象不带版本；没采用结算政策答 false；坏回指、他租户、空回指答 error。
  - PS 回指那一段：三格原样；答「有」却给空回指，判提供方违约。
  - 两只处理方：主径命令各格；不采用的收寄、不采用的终局、退运完成的终局都入账不问编排；读不回与读口故障进可见性哨兵；键对不上、集外来源类型判硬失败；编排答案落成消费两格，确定性拒收不是续办。
  - 两只消费者走真库：只处理一次、四维缺一毒丸、失败回滚重投、互不认对方类型。
- 真库装配（`cmd/parcel-dispatch/sell_evaluation_on_parcel_moment_test.go`）：
  - 用 SYN-V0 已接受委托，种带结算政策的 `SYN-RES-01` 闭包、`synRReceivableAccount` 账户；采用记录与信封经 PS 真仓储、真交接同一事务落。
  - 采用了 `NETWORK_INTAKE_ADOPTED` 时：SA 那一路 inbox `PROCESSED`，`sell_evaluation_request` 恰一行。账户、币种、计价范围、来源事实、基准时点、状态逐格对；请求信封恰一封，ID 是这份请求的指纹；重拍不翻倍。
  - 没登记采用时：入账 `PROCESSED`，零行零信封。
  - 另两条：两种信封经路由表扇出、两路各一行 inbox；两路未决名单只收续办哨兵、硬失败一枚不在。
  - 拿掉扇出里 SA 那一路，两条装配用例变红（变异实测）。

### 偏离与取舍

- **交接失败答`未决`而不是 BUY 那只的「已发起 + 续办引用」**：本编排跑在消费门事务里，留续办引用的形状会让请求行提交而信封缺席，没有任何东西再推它；答未决让整笔回滚重来。
- **信封被确定性拒收是硬失败（`publish_failed`），不是「入账不重投」**：票 01 完成记录预期它「入账不重投」，但入账就得提交事务，请求行会没有信封独自留下；照 CC 侧 `ErrDutyVerificationHandoffRejected`（sa-cc/29）的先例，回滚并落硬失败让人动手。
- **`包裹无商业解析`单列一格，不并入`未配置`**：它不是租户取值，是包裹不在任何已接受委托上（今天含谱系未建模的拆分合并包裹），恢复动作在小包托运那一侧；同样入账不重投。
- **链上任一口读出错一律`未决`**，含 PC 适配器把「闭包查无」判成 error 那一格：沿 `ApplyPreAcceptanceControlHandler` 对商业侧读口出错一律答「不可用」的先例，不折成未配置。代价是一份坏回指会重试到失败预算耗尽、落成失败记录，而不是静默入账。
- **机制清点顺手重生成**：票 01 没重生成，这一笔把票 01 的新增件一起数进去了。

### 验证

DSN `postgres://parcel:parcel@127.0.0.1:55432/postgres?sslmode=disable`：

- `go test -p 1 -count=1 ./internal/settlementaccounting/... ./internal/parcelshipment/... ./cmd/parcel-dispatch/ ./internal/architecture/` 全部 `ok`，新增真库用例逐条 `PASS`，没有 `SKIP`。
- `go build ./...` 退 0；`go vet` 上述各包退 0；`gofmt -l` 空。
- 第一笔单独检出（临时 detached worktree）也能 build、vet，本笔各包测试绿。

## 评审记录（2026-10-19，隔离子代理一次过两轴）

- 唯一阻断是机制清点没重生成，落地时在这一批的 tip 上重生成。
- 已修的非阻断：端口里拒收哨兵的头注与实现对齐（整笔回滚、硬失败，不是入账不重投）；「包裹无商业解析」补进 ADR-0190 越权风险点 8；两处注释不再数别处的东西；完成记录的 SHA。
- 记一笔：链上确定性的译不动（闭包查无等）也归未决，重试到预算耗尽才落失败，与受理前财务控制同先例。终局那一路没有真库端到端用例，重放用例没断言重交信封，留给后续。
- 与票 03、04 合成一批进 main。
