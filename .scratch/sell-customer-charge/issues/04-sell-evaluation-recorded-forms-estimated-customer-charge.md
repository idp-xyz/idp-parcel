# 04 结算：SELL 评价已记录经费用项目映射形成预估客户费用

Category: enhancement
Status: 完工，待评审与进 main——分支 `claude-sell-04`（基于 `claude-sell-03`；2026-10-19 Claude Code 会话立，同日作者会话落地，见文末「完成记录」）。
Blocked by: [03](./03-pricing-consumes-sell-evaluation-requests.md)（同改派发路由表，串行）
地盘：`internal/settlementaccounting` 的 application（形成编排外层）、adapters/parcelpricing（SELL 消费门与评价读口）、adapters/inbox；`cmd/parcel-dispatch` 路由与装配。
出处：[ADR-0190](../../../docs/adr/0190-a-parcel-billing-moment-requests-a-sell-evaluation-that-forms-the-estimated-customer-charge.md) 决定五、六；ADR-0169 决定一、二。

## 做什么

1. SELL 消费门不再一律答未登记：按评价回指读回 SELL 请求，按映射册把全部费用行代码映到同一个费用项目；任一行没有映射、或映到多于一个项目，一律答待判断（`AT-SA-174`）。
2. 费用标识由请求标识确定性派生，调用既有的 `FormSellCustomerChargeHandler`。
3. 评价待判断、不可计价、冲突：在请求册上记状态，入账不重投。已形成时在请求册上记费用标识。计价侧没形成评价时收不到信封，请求停在「已提交」（ADR-0190 决定五）。
4. 派发：`parcel-pricing.evaluation.recorded` 改成扇出，BUY 门与 SELL 门各一路、各自的 inbox 与未决哨兵。

## 完成判据

- 应用层用例：映到一个项目形成；任一行没映射、映到多个各一条；重放不多形成。
- 真库装配：用夹具造一份回指 SELL 请求的评价，一封评价已记录信封经生产路由表形成一笔预估客户费用，金额整组出自评价。端到端由 05 实走。

## 完成记录

2026-10-19 Claude Code 作者会话在分支 `claude-sell-04`（基于 `claude-sell-03`）落地。代码三笔，派发接线单独压在最后一笔：

- `9fba63cd` 请求册改记口与由请求派生的费用标识（只动 `internal/settlementaccounting` 的 domain、ports、adapters/postgres）。
- `40e58893` SELL 消费门经费用项目映射形成预估客户费用（只动 `internal/settlementaccounting` 的 ports、application、adapters/parcelpricing、adapters/inbox）。
- `d0b79874` 评价已记录扇出到 BUY 与 SELL 两路（只动 `cmd/parcel-dispatch`：`assemble.go` 与新用例文件）。票 02 同改路由表，合并冲突落在这一笔里。

两笔中间提交各自检出后 build、vet 与 `./internal/settlementaccounting/...` 全绿。没重生成机制清点（留给进 main 时）。证据级别：隔离合成，只记 `S`。

### 做了什么

1. **费用标识派生**（做什么 2）：`SellEvaluationRequest.EstimatedCustomerChargeID()` 取 `SELLCC-<请求标识>`。派生而不铸造，所以重放与并发算出同一个标识，费用库主键当场挡住第二笔。拼接而不取指纹：请求标识由本上下文铸造、长度有界，拼出来仍有界，读的人从费用标识看得出它出自哪份请求。类别段常量名以 `Prefix` 结尾，进全仓标识前缀门禁守唯一。
2. **请求册改记口**（做什么 3，票 01 留下的写口）：
   - 领域 `SellEvaluationRequestConclusion`，状态格与费用标识成对构造。只有`已形成`带费用；`已提交`不是结论。
   - 端口 `SellEvaluationRequestConclusionRecorder.RecordConclusion`，落点分三格：写下、重放、另有结论。
   - 真库实现是 `UPDATE … WHERE state = 'SUBMITTED'`，改不到就读回比内容，不覆盖先到者。`已形成`的费用标识有外键（票 01 迁移 0037），所以费用行必须同一事务先落。
3. **SELL 读口**：
   - 采用快照多带两样：评价回指（照 `SellEvaluationRequestID` 读，没有回指是零值）和全部费用行的计价费用代码（按行序）。
   - 卡没声明金额取整不再报错，交回新的一格 `SellEvaluationAmountUndeclared`，回指与代码照带，不带金额。
   - 「不是 SELL·客户费用」改用端口上的 `ports.ErrNotASellEvaluation`。适配器的同名变量就是它，应用层才分辨得出。
4. **形成编排外层**（做什么 1–3）：`application.FormEstimatedCustomerChargeHandler`。
   - 按回指读 SELL 请求。没有回指，或回指不在 SELL 请求册上（例如 BUY 请求标识），答「不归本编排」，零写入。
   - 请求已有结论就答「早有结论」：重放在这里落定，不再碰映射册与费用库。
   - 评价已完成时：全部代码去重后逐个查映射册。都有映射且只映到一个费用项目，才以派生标识调既有的 `FormSellCustomerChargeHandler`，再在请求册记`已形成`和费用标识，与费用行同一事务。
   - 下列各格都记`待判断`，不建占位费用：任一代码没映射、映到多个项目、评价没有费用行（`AT-SA-174`），金额未声明，评价结算币种对不上请求快照（`AT-SA-177`）。
   - 评价待判断、不可计价、冲突照记对应那一格。评价失败则请求停在`已提交`。
5. **消费门**：`FormOnSellEvaluationRecordedAdapter` 接形成编排，各定局入账；唯一的未决哨兵 `ErrSellChargeFormationUndecided`。ADR-0169 时代的 `ErrSellChargeReferencesUnrecorded`（对每份 SELL 评价都答缺费用项目）随之退役。
6. **inbox 与派发**（做什么 4）：
   - `sainbox.SellEvaluationRecordedConsumer`，名 `settlement-accounting/form-estimated-customer-charge`。事件类型直接引 `BuyEvaluationRecordedEventType`，两路认同一种信封，只有一处定义。
   - 派发路由表上 `parcel-pricing.evaluation.recorded` 改为 `FanOut(BUY, SELL)`，各包各的未决名单 `sellEvaluationRecordedUndecidedSentinels`。

### 金额取整未声明：记`待判断`，不按未决重投

卡上的金额取整声明是租户取值（ADR-0190 三类）。缺它时评价已完成，但金额换写不成最小币单位（ADR-0107 决定五）。选`待判断`、入账不重投，理由：

- 与既有处理一致。BUY 那一路把 `ErrAmountPrecisionUndeclared` 留在未决名单之外，理由是「这份评价按现卡永远采用不了，要改的是卡、再评价一份新的，不是重投这一封」。评价不可变，重投不会变。
- 与 ADR-0190 决定五一致：评价采用不了时「入账，不重投，状态记在请求册上」。缺映射行（同为租户取值）答的也是`待判断`。UC-SA-002 的「规则…依据不足 → 待判断」与 `AT-SA-050` 是同一个口径。
- 若按 BUY 那样报错落 `publish_failed`，请求会停在`已提交`。可裁定说的是「`已提交` = 计价没形成评价」，SA 其实已经收到了评价，停在那里就是说假话。

为此读口不再对这一格报错。报错时连回指一起丢了，就没法在请求上记状态。

### 用例

- 领域：
  - 派生标识只依请求标识，换包裹或范围不变，换请求就变。
  - 结论成对构造：`已形成`必带费用；其余三格必不带；`已提交`与零值拒。
- 真库请求册改记口（`sell_evaluation_request_conclusion_test.go`）：
  - `已提交` → `待判断`写下；同一份再记答重放；另一份结论不覆盖。
  - `已形成`与费用行同一事务落下，按包裹读得到；另一笔费用不覆盖。
  - 费用行没先落时外键拒、整笔回滚。
  - 无事务拒；另一个租户的键、不在册的请求、零值结论都答错。
- 应用层（`form_estimated_customer_charge_test.go`，依赖全替身）：
  - 映到一个项目就形成：标识派生；项目取映射；金额、形成时刻分别出自评价与时钟；重复代码只问一次映射册。
  - 记`待判断`的各一条：一行没映射；映到两个项目；映射册空；没有费用行；金额未声明；币种对不上。
  - 待判断、不可计价、冲突各记一格；失败停在`已提交`、不改记。
  - 不归本编排的两条：没有回指；回指一份 BUY 请求标识。两条都零写入。
  - 重放不多形成：请求已`已形成`时不碰映射与费用库；费用已在库上时答已存在，仍只一笔、照记`已形成`。
  - 四个依赖答不出各是未决、指名原因；另有结论与封闭集外结果是错误；缺依赖拒。
  - 变异实测：把「没映射 / 多个项目」两格改成放行，对应两条转红。
- SELL 读口（`sell_evaluation_test.go`，真算 SYN 卡，带一条固定附加费）：
  - 回指与两条代码按行序带出，105 元换写 10500 分。
  - 没有回指是零值。金额未声明交回那一格，回指和代码照带，不带金额。
  - 分区落空是待判断，回指照带、没有代码。BUY 评价答端口哨兵。别的租户的评价读不到。
- 消费门（`form_on_sell_evaluation_recorded_test.go`）：真读口加真编排、内存册。
  - 形成，重放后仍只一笔。
  - BUY 评价与没有回指的评价入账、零写入。
  - 金额未声明入账，请求记`待判断`。
  - 评价读不回、请求册答不出都是那枚未决哨兵。另有结论原样上抛、不是未决。空白引用答引用坏了。
- inbox 真库（`sell_evaluation_recorded_consumer_test.go`）：
  - 恰处理一次。同一封信 BUY 那一路收过之后，这一路照样收一次。
  - 缺键是毒丸；失败回滚、重投成功；外来类型响亮拒；nil 依赖拒。
- 派发真库（`cmd/parcel-dispatch/sell_evaluation_recorded_formation_test.go`），经生产路由表一拍：
  - 准备：SA 真请求册落两份演示租户请求；真映射册把 `BASE_FREIGHT`、`REMOTE_AREA_SURCHARGE` 映到同一个合成项目；PP 真评价库与真交接落两份回指它们的合成评价，各交一封信。
  - 结果：两封都 PUBLISHED，两路 inbox 各一行。映射齐的那一份形成恰一笔 `SELLCC-SELLREQ-SYN-MAPPED`，预估阶段、CNY 10500（评价合计 105 按 0.01 换写），请求 FORMED 并带这笔费用。带没映射代码的那一份请求 PENDING_JUDGMENT。租户下费用行恰一行。
  - 变异实测：去掉扇出时，正例红在 SELL 那一路的 inbox。
  - 评价用夹具真算、不经演示卡：演示卡今天没声明金额取整（demo-chain-gaps/09，票 05 补），走它只会落`待判断`。端到端归票 05。
  - 既有的 BUY 评价真库装配用例未改，扇出之后仍绿。

### 验证

在 `d0b79874` 上，DSN `postgres://parcel:parcel@127.0.0.1:55432/postgres?sslmode=disable`：

- `go test -p 1 -count=1 ./internal/settlementaccounting/... ./internal/parcelpricing/... ./cmd/parcel-dispatch/ ./internal/architecture/ ./migrations/...` 全部 `ok`。本票新增的真库用例逐条 `PASS`，没有 `SKIP`。
- `go vet` 两处地盘退 0，`gofmt -l` 空，`go build ./...` 退 0。

### 偏离、取舍与待裁

1. **评价失败**：请求停在`已提交`，入账不重投；票面没写这一格。计价对失败评价照样入册并交信封，所以 SA 会收到它。按协调方「不立未形成那一格」的裁定，请求留在`已提交`。评价不可变，计价按回指幂等也不会再形成第二份，重投无益。若要它响亮，可以改成上抛落 `publish_failed`，只动外层的那一个分支。
2. **结算币种核对**（`AT-SA-177`）：票面没写，我加了。评价合计币种与请求快照的结算币种不同时记`待判断`，不拿原币顶结算币。
3. **ADR-0169 决定四「消费门指名缺这一引用」**：缺映射如今是在请求册上记`待判断`，入账、不报错。请求册没有原因列，缺的是哪个代码没有落在任何地方，要重算才知道：拿评价的费用代码对一遍映射册。要不要给请求册加原因列，留给 SA owner；我没有为此另开迁移。
4. **`AT-SA-175`「保存排除依据与评价引用」**：请求册只记`不可计价`，不另存评价引用。评价自带回指，计价侧按回指能读到它。没加列。
5. **矛盾按错误处理，不按未决**：两格都上抛、落 `publish_failed`，整笔回滚。
   - 请求册上另有结论：同一份评价算出的结论是确定的，撞上另一份只可能是映射册在两拍之间变了且两拍并发。
   - 刚读到`已完成`的评价，形成时答未受理：例如合计为零，形成门要求金额大于零。
6. **内层形成编排未改**：`FormSellCustomerChargeHandler` 仍把评价待判断、失败映成未决。生产路径上，外层只在已完成且映射齐时才调它，那几格走不到。外层与内层的口径因此不同，后继若直接调内层要留意。
7. **扇出顺序**：先 BUY、后 SELL。两路互不依赖到达顺序，排序只为与既有扇出「既有一路在前」的写法一致。
8. 没改 `docs/`，也没重生成机制清点（留给进 main 时）。可补：ADR-0190 决定五与 UC-SA-002 可写明失败评价、金额未声明、币种不符三格的去向；`CONTEXT-MAP` 的 `parcel-pricing → settlement-accounting` 一段可补扇出。

