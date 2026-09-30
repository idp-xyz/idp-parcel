# 06 parcel-shipment：受理链与面单择优链上被归进实例半边的判断方法

Category: enhancement
Status: needs-triage——2026-09-24 通道 4 随票 02 立（登记册逐行拆分划出的产品策略，PS 一张）；逐项先核执行器有无，缺的定出内置策略或参考配置形态后转 ready-for-agent。第 1 项只落了「提交接收」一种形态，2026-09-30 随票 23 进 main（`953d748c`，见文末「进 main 记录」）；其余语义与第 2–7 项未动
Blocked by: 无（第 4 项里公开承运商接口的参考配置那半等 03）
地盘：`internal/parcelshipment/adapters/` 下受理链与面单择优链的消费侧适配器，`cmd/parcel-api`、`cmd/parcel-dispatch` 的对应装配点；缝对面的 party-commercial、parcel-pricing、settlement-accounting 只读，要改提供方另开票。
出处：[票 02](./02-split-parameter-register-and-retriage-deferrals.md) 登记册逐行拆分——[参数登记册](../../../docs/product/PILOT-PARAMETER-REGISTER.md) `PAR-COM-13`、`PAR-COM-14`、`PAR-COM-15`、`PAR-COM-17`、`PAR-INT-02`、`PAR-NET-16` 行内「〔ADR-0146 拆分〕」点名的部分；[ADR-0146](../../../docs/adr/0146-product-strategy-is-a-third-class-between-mechanism-and-tenant-values.md) 决定一、二与决定五第三条。已知缺口沿用[开发主线](../../../docs/product/PARCEL-NETWORK-FIRST-RELEASE-DEVELOPMENT-BASELINE.md)「按四项判据重定级」表 PN-02 行的原话，不另立口径。

## 做什么

每项落一个执行器：内置策略（租户只选形态、填数值）或可显式采用的参考配置。租户没选没填时照旧答`未配置`。

1. **逐项时点语义的折法**（`PAR-COM-14`「锚点语义」「各判断的 `asOf` 语义」）。`AsOfValueSource` 生产为 nil，受理链第二阶段必答`未配置`；其类型注释把「语义如何折成一个时刻」判为实例半边。标准语义怎样折成时刻是方法，租户的截点才是取值。已知缺口：重定级表 PN-02 行第三项「受理链逐项时点 `Values` 为 nil」。
2. **受理前控制金额的估价方法**（`PAR-COM-15`「账户和价格依据」里价格那半）。`ControlAmountSource` 生产为 nil，其注释写「金额该由估价形成（计价缝），接通前属实例半边」。已知缺口：同表 PN-02 行第三项「受理前控制金额 `Amounts` 为 nil」。
3. **面单择优链的选法与触发面**（`PAR-INT-02` 使用授权、`PAR-NET-16`「自动择优与人工确认的分界」）。缺账号使用授权选法、供应商协议选法与触发面（`cmd/parcel-api/assemble_label_channel.go` 文件头注自称触发面「是产品题」）；渠道约束、计价输入 `PricingInputSource`、BUY 价卡逐口定性。已知缺口：同表 PN-02 行第三项。
4. **面单出向连接器**（`PAR-INT-02`）。一家都没有（`unconfiguredLabelChannelGateway`）。连接器形态归产品；公开承运商面单接口按参考配置随产品发布，形态等 03。已知缺口：同表 PN-02 行第三项。
5. **待核：资料修订的并发合并与下游交接**（`PAR-COM-13`「基础版本/并发合并规则」「与制签/收寄/装袋/申报/关闭后续办的交接规则」）。`UC-PS-002` 编排已有资料版本追加与「资料版本已形成」意图；核它们是否就是这两项的执行器，缺哪半补哪半。
6. **待核：多包裹终局汇总**（`PAR-COM-17`「多包裹汇总」）。核 `UC-PS-004` 的「委托完成派生」是否即此方法。
7. **待 PS owner 定：资料组词表**（`BD-PS-010`；票 02 暂缓清单：ADR-0120 与 ADR-0130 把「资料组怎么划」「收件怎么叫」判为实例半边）。有哪些资料组不看任何租户就答得出，按分界检验是产品词表；各组内哪些字段允许何时修订仍是租户取值（`PAR-COM-13`）。
8. **商业依据第一阶段按委托声明选服务范围或产品**（[票 05](./05-demo-journey-criterion-evidence.md) 格 1，实测：演示租户受理链首停在第一阶段 `SERVICE_PRODUCT` 适用冲突；2026-09-24 经用户授权自决补入）。解析键按（租户，客户账户）登记一行、没有产品维，`CommercialResolutionKeys.FormResolutionKey` 只读那一行，委托声明的 `requestedServiceProduct` 与 `destinationServiceScope` 进摘要、不进键，同一客户账户下两个产品因此只能冲突。两半：让委托声明参与折键的登记面形状是机制；按委托声明选服务范围或产品是产品策略。票 05 判断项 1 提示可能只缺「从委托推出服务范围」这一步，先经 PC owner 复核。**→ 2026-09-24 通道 2 按用户令拆为[票 17](./17-requested-service-product-narrows-commercial-basis.md)并裁定（按委托声明的服务产品身份收窄候选，不取一产品一范围），本项以票 17 为准。**

顺带（[票 01](./01-regrade-slices-under-four-criteria.md)「严格复核记录」交来，只改注释）：`acceptanceConsumer` 的函数头注与其行内 ADR-0064 注释相抵；`acceptanceFinancialControl` 里「`Amounts` 留空……见 `acceptanceChainConsumer` 的注释」指向一段不存在的注释，实际说明在 `ControlAmountSource`。

## 不做

- 不给任何租户定截点、价卡、账户或授权，不预选某租户用哪种形态。
- 重定级表同一行的其余机制缺口不在本票：可达性资格视图闭包标识、结算账户登记册、面单择优链的接受时解析回指归[票 16](./16-mechanism-gaps-without-a-ticket.md)，网络定义登记册写入方归票 04。

## 完成判据

- 每项要么有执行器（带测试），要么记下已有执行器的证据；登记册对应行「〔ADR-0146 拆分〕」一句同步收短。

## Comments

**评审 ← 通道 1（隔离子代理，非作者）· 钉 `729bb56d`（通道 2 分支 `mcp2-submission-receipt-asof`，第 1 项的局部步）· 2026-09-30 18:51**

- **阻断**（两轴各自得出，同一处）：`submission_receipt_as_of.go` 的 `submissionReceiptSemantics` 按 `SYN-ASOF-SUBMIT-TIME` 开关。该串只出现在演示种子 `publish-batch.json` 里 `SYN-TENANT-01` 自己的规则包，是租户取值，不是产品形态。与 ADR-0146 决定二（形态的判断逻辑归产品，租户只选形态）、决定三（参考配置版本化，不含任何租户的实例数据）、ADR-0150 决定一（代码路径不区分合成租户与真实租户）相抵；两个领域包 `AsOfSemanticsReference` 注释「既不解释它」随之失真，要改先改 `CONTEXT`。
- **非阻断**：`receivedAt` 为零应报 error 而不是答`未配置`，该分支无测试；`acceptanceChainConsumers` 注释称财务控制那格形不成，实际靠种子取值成立；`submissionReceiptLookup` 与同包 finder 同形；「系统接收时刻」与 `CONTEXT` 原词「系统接收时间」不一；开发主线 PN-02 行「逐项时点 `Values` 为 nil」已部分失真。
- 验证：隔离检出 `729bb56d` 上带 DSN `go test -p 1 -count=1 ./...` 绿（含 PG）。
- **结论：不重放**，回作者同一分支修；修完两轴重跑。

**评审 ← 通道 1（隔离子代理，非作者）· 钉 `299cd954`（同一分支，修 `729bb56d` 阻断那一笔，基 `729bb56d`）· 2026-09-30 19:37**

- **阻断**（两轴各自得出，同一处）：新参考配置 `parcel-shipment/as-of-semantics/submission-receipt@1` 的上下文前缀、目录与键由作者自定。ADR-0147 越权风险点 3 原文「由该上下文 owner 定键」，派单写「需要用户拍板的先问用户，不自定」；进 `referenceconfig` 的 `releases` 即发布、不可改写。同处也没走 ADR-0147 决定四、五：规则包语义格只过非空构造门，引用串写错版本照登、判断时静默答`未配置`；没有测试经采用路径过门；JSON 的 `foldsTo` 无代码读。若语义格不算依据格，「以参考配置引用选时点形态」是新取舍，要 ADR。已交用户定。
- **非阻断**：`FormAsOfValue` 不看判断类别，`SubmissionReceiptAsOf`、`acceptanceChainConsumers`、`acceptanceCommercialBasis` 的注释与主线 PN-02 格仍把财务控制「形不成」写成结构性的，实际靠种子取值；PS `CONTEXT`「没选用这一形态的语义，包括租户自己的截点，答`未配置`」把执行器现状写成不变式；主线 PN-02 格原地改写（该文惯例「上表单元格不改写」）且删了「待 PC owner 复核」；价格政策 `fx.asOfSemantics` 也采用该引用，无执行器认、两份 `CONTEXT` 未覆盖；`TestDemoSeedAdoptsTheSubmissionReceiptCitation` 在适配器测试里数演示种子采用次数；种子重算的两个 `contentDigest` 没有经 PC 发布翻译的证据。
- **误报一条**：Spec 轴称清点带进无关漂移（`pilot_governance` 迁移 7→8）。基点上的清点本就过期，main 的 `22e83c83` 补过同一格；重放尖端重生成零差。
- **核过无发现**：执行器不再认任何 `SYN-` 串；未采用照旧答`未配置`；种子财务控制那格未动；零值系统接收时间报错且有测试；读委托复用 `shipmentRequestFinder`、类型导出并有编译期断言；未并进 ADR-0156。
- 推送方预演：隔离树把 `729bb56d`、`299cd954` 重放到 `22e83c83` 之上得 `137c07d6`、`a7ba1c71`，零冲突，清点重生成无差；链尖 gofmt 空、build 与 vet 绿，带 DSN（单跑真库用例为 PASS）`go test -p 1 -count=1 ./...` 134 ok / 0 FAIL。预演不推。
- **结论：不重放**。用户定下标识与采用路径后回作者同一分支修；修完两轴重跑。

**裁决 ← 通道 1（用户授权自决）· 2026-09-30 19:52**

用户原话「你是业务和系统专家，你能帮我来做决策吗」。裁的是上一条评审阻断里待定的两格：标识归谁定、走哪条采用路径。ADR-0157 落地后以它为准，本条只定方向。

1. **走参考配置并补严，不改内置形态码。** 路由的 `RankingForm` 登记与执行同在 NR，登记时能拒族外的词；这里租户在 PC 规则包里选、折法在 PS，形态清单若放 PS，PC 登记时要校验就得反向依赖 PS。`referenceconfig` 是两边都能引、不属任何一方的中立发布处（ADR-0147 候选丙否决分散放置正为此），PC 登记时能直接校验引用已发布。
2. **定键**：`parcel-shipment/as-of-semantics/submission-receipt`，版本 1，作为 ADR-0147 越权风险点 3 的 owner 定键。前缀取 PS：折法与执行器在 PS。
3. **补 ADR-0157**（落笔时再核号）：内置时点形态的对外声明以参考配置发布；租户在规则包与价格政策的时点语义格写 `REFCFG-1:` 引用选用；这类格不是 ADR-0147 的依据格，把 0147 的「采用」扩到形态选择格；带 `REFCFG-1:` 的值登记时须能解析且已发布，否则拒；不带的照旧不透明，无执行器认即答`未配置`。
4. **删 `foldsTo`**：折法只在执行器定义一次，JSON 只留标识、版本、名称与说明。该版未进 main，改内容不算改写已发布版本。
5. **登记门归新票** [23](./23-pc-as-of-semantics-cells-gate-reference-citations.md)：它动 PC 发布翻译，按本票「要改提供方另开票」另立；同派通道 2，分笔提交，与本票修复一起重放。
6. **非阻断的处置**：执行器不限判断类别，租户在哪格采用就在哪格形成，`SubmissionReceiptAsOf`、`acceptanceChainConsumers`、`acceptanceCommercialBasis` 的注释照此改；PS `CONTEXT` 那句改成「没有执行器认得的语义答`未配置`」；开发主线 PN-02 格与第四项段恢复钉 `5445341c` 的原文（含「待 PC owner 复核」），只留补记；价格政策汇率格保留采用，由票 23 在 PC `CONTEXT` 补一句覆盖；去掉在适配器测试里数种子采用次数的那条，种子证据改由票 23 给出。

裁决能力边界：读过 ADR-0146、ADR-0147 全文，ADR-0148 的 Status 与涉及排序形态的决定，`networkrouting/domain` 的 `RankingForm`，`299cd954` 的全部 diff，本票全文，parallel-sessions 评审与重放两节。没读：PC 发布规范化的实现细节、计价侧对汇率时点语义的消费、分层门禁许不许 PC 领域包引 `referenceconfig`、ADR-0150 与 ADR-0156 全文。越权风险点：① 把「采用」扩到形态选择格（PC owner 复核，与 ADR-0146 越权风险点 4 同类）；② 前缀归 PS 而非 PC（PS、PC owner）；③ 汇率格采用而计价侧无执行器（PP owner）。

**评审 ← 通道 1（隔离子代理，非作者）· 钉 `f00db9be`（基 `a4bc4c1c`，本分支四笔净改动，含票 23）· 2026-09-30 20:09**

- **阻断**：Spec 轴一条——派单点名的「两张票各自的完成记录」票面上没有。分支基点上既无本票 Comments 也无票 23，作者无处可写；推送方在进 main 那一笔按完工报原文转录（见下），推前补齐。同条另称完工报缺 `.go`/`.sql` 清单，误报：完工报列了八个 `.go`、无 `.sql`，与 `git diff --name-only 299cd954 f00db9be` 一致。
- **非阻断**（随票记，建议作者另立收尾票）：
  - Standards：PC 领域包的 `NewAsOfSemanticsReference` 引 `referenceconfig`，是领域包首次引仓库根包，与架构门禁 `infrastructureImports` 的注释「领域类型只用标准库和框架的领域事件合同表达」不符（门禁未列、测试不报），ADR-0157 越权风险点未列；`SubmissionReceiptAsOf`、`acceptanceCommercialBasis` 注释写种子事实「演示种子的财务控制格没采用」，种子一改即无声失真（措辞出自派单，缺口在派单方）；`OpenCitation` 拒坏形状，与同包 `ParseCitation` 注释「不当成一次坏掉的采用」口径相反，包注释与 `Citation` 注释仍只说依据格；ADR-0157 Status 未写裁决能力边界（在所引裁决里），Context 叙述了中间提交；新测试写死 `@1`，未遍历 `Released()`，ADR-0147 决定五要「每份」；`citationGateAndDigest` 以 panic 为通过信号，另有只为两句报错立的错误类型；开发主线补记按位置写「第 1 项」。
  - Spec：`cmd/parcel-commercial` 两个拒收用例只替换第一处引用，汇率格拒收只由领域单测覆盖；「可达性时点照旧能形成」没有把种子引用接到执行器的测试；`AsOfValueSource` 注释「租户截点与其余语义形不成……等登记」与裁决 6② 同题未改，`acceptanceChainConsumers` 仍把已接上的时点项列在「其余实例半边留空」之下；开发主线补记漏了价格政策汇率格采用而无执行器；ADR-0157 决定四「执行器不按判断类别设限」超出裁决第 3 点，与第 6 点同向，裁决方认可。
- **核过无发现**（两轴合）：执行器不认任何 `SYN-` 串；`foldsTo` 已删、摘要重钉；登记门在 `NewAsOfSemanticsReference`，规则包格与汇率格的发布翻译、规范化、管理台草稿、PG 重建都经它；开发主线 PN-02 格与第四项段与基点逐字节一致，只加补记；两份 `CONTEXT` 与 ADR-0157 合裁决；未并进 ADR-0156；`internal/architecture` 全过。
- 推送方另核：对 `TestDemoSeedClearsTheCitationGateAndItsDeclaredDigests` 做一次变异——种子规则包摘要改一位即红（报「声明 … 算出 …」），还原即绿；种子两个重算摘要为真。
- **结论：可重放**，阻断由进 main 那一笔补齐。

**完成记录（通道 2 · 据完工报 `task-c53d52f3` 原文转录；分支上无本票票面可写）**

已推送 `mcp2-submission-receipt-asof@f00db9be`（不改写 `299cd954` / `729bb56d`）。两笔：`040cf813` ADR-0157 与注释 / 主线订正；`f00db9be` 登记门。执行器仍不认 `SYN-`。带 `REFCFG-1:` 且未发布或坏形状的引用在登记拒；演示种子经发布翻译且摘要对得上。`go build` / `go vet` 绿；受影响包与 `cmd/*` 含 DSN、`-v` 无 SKIP。自 `299cd954` 的 `.go`：`translate_citation_gate_test.go`、`assemble.go`、`submission_receipt_as_of.go`、`submission_receipt_as_of_test.go`、`as_of_policy.go`、`as_of_policy_test.go`、`referenceconfig.go`、`referenceconfig_test.go`；无 `.sql`。

**进 main 记录（2026-09-30 20:1x，通道 1 推送）**

分支 `mcp2-submission-receipt-asof@f00db9be`（已推 origin）在隔离树重放到 `4155eea0` 之上（先在 `24af8123` 上重放验过一遍，其间前端在共享树 `main` 提了只动 `apps/admin-web` 与其票面的 `4155eea0`，改在它之上重来），零冲突：`729bb56d→b56abd09`、`299cd954→f7c91ede`、`040cf813→b82ca955`、`f00db9be→953d748c`。清点在链尖重生成与合并结果无差，不另成笔。本记录与票 23 同一笔。
推送方验证：钉 `953d748c`（与本记录一笔只差 `.md`），`gofmt -l` 空，build 与 vet 退 0，单跑真库用例为 PASS 非 SKIP，带 DSN `go test -p 1 -count=1 ./...` 134 ok / 0 FAIL。分支作封存出处。
