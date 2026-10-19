# 37 受控关闭与重开：「代货主」那一对与关闭责任来源词表收归 application 一处，装配头注去计数，票 28 遗留头注改口

Category: enhancement
Status: ready-for-agent——2026-10-18 会话 `idp-parcel-9b` 立（票 26 序 1 评审遗留；用户授权代决）
Blocked by: operator-channel/34 进 main（同改 internal/parcelshipment/adapters/http/operator_decision_intake.go 及其测试，串行免撞）
父票：[26](./26-non-author-review-backlog-for-claude-code-landings.md)。被评的是 [15](./15-operation-decision-faces-take-operator-intake.md) 之六（代码 `0e3bb7df`），规则出处是 [ADR-0183](../../../docs/adr/0183-operator-channel-addresses-controlled-closure-and-reopening-by-parcel-and-derives-the-shipper-from-the-accepted-request.md)。
出处：
- 票 15 Comments「评审 ← 会话 idp-parcel-9b 起的隔离子代理 · 钉 `0e3bb7df`」那一条的 Standards 非阻断 (1)(2)(3)；
- 票 [28](./28-controlled-closure-and-reopening-have-no-replay-algebra.md) 落地后不再成立的头注。28 已 resolved，由本票接手。
地盘：
- internal/parcelshipment/application/form_continued_attempt_decision.go
- internal/parcelshipment/application/form_continued_attempt_decision_internal_test.go
- internal/parcelshipment/application/form_continued_attempt_decision_test.go
- internal/parcelshipment/adapters/http/operator_decision_intake.go
- internal/parcelshipment/adapters/http/operator_decision_intake_test.go
- internal/parcelshipment/ports/continued_attempt_decision.go
- internal/parcelshipment/adapters/identity/identity.go
- cmd/parcel-api/assemble_operator_decisions.go
- docs/product/MECHANISM-INVENTORY.md（只在重生成有差时）
- .scratch/operator-channel/issues/15-operation-decision-faces-take-operator-intake.md（只追加 Comments）
- .scratch/operator-channel/issues/<本票>.md

代决依据：用户 2026-10-16 授权会话 `idp-parcel-9b`「你作为业务和系统专家你来自决」；裁定经隔离子代理研判、对抗核验与合成（研判轮 review26-followups-decide）。

## 为什么

- **请求方在适配器拼、在 application 比。**
  - `adapters/http` 的 `shipperOf` 拼出 `CUSTOMER-ACCOUNT/<客户账户标识>`；application 的 `shipperReauthorizationGate` 拿 `command.ShipperAccount` 与 `closure.Requester()` 比相等。
  - ADR-0183 复核记录要求「日后另开写入路径（例如客户渠道，ADR-0139）时，照决定二写同一种带种类段的引用」，到时就会有第二处拼串。
  - 两处写法一不同，货主本人就会被重开门挡在外面。研判探针实测：关闭请求方是 `CUSTOMER-ACCOUNT/SYN-CUSTOMER-1`、重开货主账户是 `SYN-CUSTOMER-1`，答 `INPUT_NOT_ACCEPTED / SHIPPER_ACCOUNT_MISMATCH`。
- **货主指令关闭的责任主体也在适配器推。**
  - `IntakeControlledClosure` 直接写 `identity.CustomerAccountID().String()`。
  - 它与请求方出自 ADR-0183 决定三同一句「请求方与责任来源的主体都取第二条那个客户账户」。
  - 主体进落册的`关闭责任来源`，因此进票 28 的重放比对（`RecordedRequest` 比的是整串）。核验在 origin/main 上实测：其余内容相同、只换主体，答 `FORMED`，册上变成两条。
- **同一份封闭词表列了两遍。** 适配器的 `parseClosureResponsibilitySource` 与 application 的 `closureResponsibilitySourceKindOf` 各遍历一次种类。今天逐词一致，是重复，还没有漂移。
- **装配头注数别包的口。** `buildOperatorDecisionIntakes` 头注「委托侧按委托标识寻址的三口用 targets，受控关闭与重开两口……」数的是 shipmenthttp 包里的口，违反 AGENTS「改文档」的「计数与行号同构」，以及「Go 注释里的跨文件引用同受此约束」。
- **票 28 之后有几句头注不再成立。**
  - `ports.ContinuedAttemptDecisionIdentity` 与 adapters/identity 的 `ContinuedAttemptDecisions` 都写「关过—重开—再关是三条各自的决定，内容可以逐字相同」。28 之后，逐字相同的再关按重放认：在 origin/main 上实测答 `EXISTING_RESULT`，交回首关，册上判断为 `OPEN`。
  - 同文件 `ports.ContinuedAttemptDecisionAuthorizationQuery` 头注「关闭这一格允许缺席」，自 ADR-0183 决定四起已不实：没带货主新授权的重开同样没有请求方。

## 裁定

取候选 A（最小挪动），并把货主指令关闭的责任主体一并收进 application。各项都是机制整理，行为不变，不出 ADR。

- **「代货主」那一对只在 application 推一次。**
  - 导出 `ShipperRequesterOf`：请求方与货主账户的唯一合成处，重开门比相等的两边都由它给出。
  - 导出 `ShipperInstructionSubjectOf`：货主指令关闭责任主体的唯一推导处。
  - 任何渠道都调这两个函数，不另拼。
- **依据如实写。** lc/30 裁决 ②「合成 / 解析收在一对函数」管的是关闭责任来源；请求方没有解析那一半。把它挪进 application 是按以下几处类推的：ADR-0183 决定二「与`关闭责任来源`的 `<种类>/<主体>` 同形」、上面那句复核记录，以及 AGENTS 的单一权威。同形先例是票 28 的 `HasShipperAuthorizationEvidence`：application 导出判据给接入层用，判据只在一处定义。
- **认词只在 application 一处。** 导出 `ParseClosureResponsibilitySourceKind`，认不出就不默认任何一格（lc/30 裁决 ②）。合成函数 `closureResponsibilitySourceOf` 仍不导出，守住 lc/36 条 8 包内用例头注那句「导出只为测试会让调用方拿到一个不该由它拼的串」。
- **装配头注改成点名符号。** lc/36 那句「数的是同文件的结构体字段，不在本票」是在划范围，不是裁定这类计数可以留。
- **票 28 的遗留头注由本票改口。** 写法以 ADR-0183「复核记录二」为准。
- **业务后果写在这里。** 逐字相同的「再关」答`已有结果`，交回的可能是一条已不生效的关闭。日后接这两口的管理台不得用前一条决定的生效时间预填，重提交时重算生效时间就是另一次请求。

## 做什么

1. `internal/parcelshipment/application/form_continued_attempt_decision.go`
   - 紧挨 `HasShipperAuthorizationEvidence` 与 `shipperReauthorizationGate`，导出两个函数：
     - `ShipperRequesterOf(identity domain.SourceIdentity) (domain.RequesterReference, error)`
     - `ShipperInstructionSubjectOf(identity domain.SourceIdentity) string`
   - 头注写明：两者出自 ADR-0183 决定三同一句；主体进落册的`关闭责任来源`，因此进重放比对；任何渠道都调这里。
   - 种类段写成不导出常量 `customerAccountRequesterKind = "CUSTOMER-ACCOUNT"`。名字不得以 Prefix 结尾：`internal/architecture` 的 `collectIdentityPrefixes` 会收以 Prefix 结尾的常量，`TestNoIdentityPrefixCarriesTheSeparator` 会拒带横线的值。
   - `closureResponsibilitySourceSeparator` 改名为 `kindSegmentSeparator`。头注写「`关闭责任来源`与「代货主」请求方引用同形（ADR-0183 决定二），共用这一个分隔符」，不带数。
   - 导出 `ParseClosureResponsibilitySourceKind(word string) ClosureResponsibilitySourceKind`：
     - 种类的枚举遍历只放在这里；
     - 认不出交 `ClosureResponsibilitySourceKindInvalid`；
     - `closureResponsibilitySourceKindOf` 改为先 `strings.Cut`，再调它。
2. `internal/parcelshipment/adapters/http/operator_decision_intake.go`
   - 删掉 `shipperOf`。
   - `IntakeControlledClosure` 的货主指令分支改调 `application.ShipperRequesterOf` 与 `application.ShipperInstructionSubjectOf`；`IntakeReopening` 改调 `application.ShipperRequesterOf`。错误照旧包原前缀「parcel shipment http: shipper requester」。
   - `parseClosureResponsibilitySource` 只把 Invalid 译成 `ErrMalformedRequest`，报文原样带出认不出的那个词；头注改指 application 的认词函数。
3. `cmd/parcel-api/assemble_operator_decisions.go`：`buildOperatorDecisionIntakes` 头注改为「装 `operatorDecisionIntakes` 各格的防腐认证方与 Intake。`shipmenthttp.OperatorDecisionIntake` 按委托标识寻址时用 targets，受控关闭与重开按包裹寻址、用 parcels（ADR-0183 决定一）。」
4. `internal/parcelshipment/ports/continued_attempt_decision.go` 与 `internal/parcelshipment/adapters/identity/identity.go`：
   - `ContinuedAttemptDecisionIdentity` / `ContinuedAttemptDecisions` 头注改为：标识由本上下文铸，不从内容派生，铸在认重放之后——同类同内容的请求按重放认、交回原决定、不铸新号（ADR-0183「复核记录二」）；关过—重开—再关的再关是另一次请求，带它自己形成时固定的生效时间。
   - `ContinuedAttemptDecisionAuthorizationQuery` 头注「关闭这一格允许缺席，重开与关闭同型只为一口」改为：请求方可缺席（「如有」）——关闭不是货主指令形成的、重开没带货主新授权时，都没有请求方（ADR-0183 决定三、四）。
5. 用例
   - application 外部测试包：`ParseClosureResponsibilitySourceKind` 对每个有名字的种类都认回自己；对空串、`OTHER`、小写词、带主体的整串都交 Invalid。
   - application：
     - `ShipperRequesterOf` 合成 `CUSTOMER-ACCOUNT/<客户账户标识>`，`ShipperInstructionSubjectOf` 交回客户账户标识；
     - 关闭的请求方与主体、重开的货主账户都经它们从同一来源身份推出时，同内容关闭重投答 `EXISTING_RESULT`，重开能过门。
   - 包内往返用例随分隔符一起改名。
   - http：`TestControlledClosureTakesTheShipperFromTheAcceptedRequestOfTheParcel` 与 `TestReopeningTakesTheShipperAccountFromTheAcceptedRequestOfTheParcel` 里的字面串 `CUSTOMER-ACCOUNT/SYN-ACCOUNT-01` 原样保留，作为线上契约的钉；认不出的 `responsibilitySource` 仍答 400。
6. 记录
   - 票 15 Comments 记下钉 `0e3bb7df` 评审 Standards (1)(2)(3) 的去向。
   - 本票「完成记录」写明接手了票 28 的遗留头注。

参考原型（只读取证、未提交）：`/tmp/claude-1000/-home-tops-workspace-idp-parcel/39d8f2f8-cde5-4dd1-bb34-aef8ce7c660c/scratchpad/decide/C5-final/c5-final-q2-only.patch`。在 origin/main `c68e4342` 上 dry-run 干净；挂 overlay 带 DSN 跑 application、http、identity、domain、cmd/parcel-api、architecture，全 ok。

## 不做

- 不把种类抬进 `domain/**` 成类型。lc/30 判断题 (c) 与 lc/36 条 8 都写了「不做」，归 PS owner。
- 不导出 `closureResponsibilitySourceOf`。
- 不改命令契约。候选 B「编排自推请求方」已否决：它会改票 28 刚定的命令面，还会把 ADR-0183 决定四的渠道判据搬进编排。
- 不改任何结果代数与行为。
- `OperatorDecisionIntake` 类型头注里的口数，等 C5-Q1 改写那段头注时一并去掉，不进本笔。
- 不碰请求证据与操作证据落册，那是 C5-Q1。

## 完成判据

- [ ] 非测试 Go 源码里，`CUSTOMER-ACCOUNT` 只出现在 application 的 `customerAccountRequesterKind`（用 `git grep -n 'CUSTOMER-ACCOUNT' -- '*.go' ':!*_test.go'` 核）。
- [ ] 关闭责任来源种类的枚举遍历，在非测试源码里只在 `ParseClosureResponsibilitySourceKind`。
- [ ] 适配器里不再出现 `identity.CustomerAccountID()`。
- [ ] 上列新用例都在；http 那两条线上契约用例原样绿。
- [ ] `git grep -n '内容可以逐字相同\|关闭这一格允许缺席' -- internal/parcelshipment` 零命中；`buildOperatorDecisionIntakes` 头注不再数别包的口。
- [ ] `gofmt -l .` 无输出；`go build ./...`、`go vet ./...` 退 0。
- [ ] 带 DSN 跑 `go test -p 1 -count=1 ./internal/parcelshipment/... ./cmd/parcel-api/ ./internal/architecture/...`，全 ok，其中包括真库用例 `TestClosureAndReopeningThroughTheOperatorChannelAgainstARealDatabase`。
- [ ] 机制清点在本笔的干净检出上重生成；有差就单独成笔说明。
- [ ] 走 Go 那条路（[并行会话](../../../docs/agents/parallel-sessions.md)）：分支、作者自验、非作者评审（空闲通道，或 `/code-review` 的隔离子代理）、推送方在干净检出上带 DSN 跑全量。
- [ ] 票 15 Comments 已记下 Standards (1)(2)(3) 的去向。
