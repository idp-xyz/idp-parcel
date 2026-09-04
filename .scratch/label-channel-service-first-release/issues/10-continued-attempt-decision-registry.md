# 10 `面单继续尝试决定`登记册未建，读面那一格派生自空历史

Category: enhancement
Status: resolved——四层齐：领域 `7c79b3d`、端口 `3b37845`、持久化 `0d492b8`、读面派生 `08e62ec`；完成记录见文末
Blocked by: 无

## 缺口

[ADR-0084](../../../docs/adr/0084-label-transaction-is-an-independent-aggregate-with-two-level-results.md)
决定六点名本切片不建`面单继续尝试决定`登记册，`internal/parcelshipment/domain/label_transaction.go`
的聚合注释也显式点名。后果是读面 `ports.LabelTransactionParcelRow` 的 `ContinuedAttemptOpen`
**派生自一段真实为空的决定历史**——它今天恒答同一个值，而页面上看不出这一点。

这与本仓治过多次的病同形：「尚未接线」与「已接线但登记册为空」长同一张脸。区别是这一格
连登记册都还没有，所以连「为空」都算不上。

## 做什么

建`面单继续尝试决定`登记册：决定的身份、它挂在哪一笔交易与哪一个包裹上、决定内容的封闭集
（继续尝试 / 不再尝试 / 待定，具体格数按 PS CONTEXT 与 ADR-0084 的原词，**不新造**），以及
`ContinuedAttemptOpen` 据以派生的规则。

## 红线

- **不新造领域语言**：决定的名字与格数取 CONTEXT/ADR-0084 原词，要改先回 `CONTEXT.md`
  （[AGENTS 改文档](../../../AGENTS.md#改文档)）。
- 登记不可覆盖，更正走版本链。
- 读面那一格改成据实派生之后，**如果登记册为空要说得出「没有人作过决定」**，不得折成
  「不继续」——两者续办动作相反。

## 完成判据

登记册的领域、端口、持久化与读面派生四处齐；`ContinuedAttemptOpen` 不再派生自空历史；
`gofmt -l` 空、`go build`/`go vet` 退 0、`go test -count=1 ./...` 绿并注明含不含真库
（**本票大概率动迁移，真库必须实跑**）。

## 参照

[能力形状盘点](../capability-shape-inventory.md)第一段；ADR-0084 决定六。

## 取词：票面对这套语言的猜测是错的

2026-09-02 由 MCP-1 取自 [PS CONTEXT](../../../docs/domain/parcel-shipment/CONTEXT.md)，取证于 `98c2e5c`。
本节只记原词与它们之间的关系，不作设计。

**票面「做什么」一节猜的「继续尝试 / 不再尝试 / 待定」三格不存在。** CONTEXT 里是**两族东西**，
混成一族正是这一票最容易一开始就走歪的地方：

- **`面单继续尝试决定`是被追加的决定**，两种：`受控关闭决定`与`重开决定`。
- **`包裹级继续尝试判断`是从决定派生出来的值**，封闭两格：`开放`与`受控关闭`
  （见 CONTEXT 生命周期「面单继续尝试」一节）。读面那个 `ContinuedAttemptOpen` 是它，不是决定。

**两种决定各自的必备项，逐字取自 CONTEXT：**

- `受控关闭决定`：生效时间、权威业务截断边界、原因、请求方（如有）、实际决定方、关闭责任来源、
  授权角色、授权依据快照。
- `重开决定`：原因、关联此前关闭、只面向未来生效；另按「关闭和重开决定必须由运营企业责任法人
  授权的业务角色明确形成」一条，实际决定方与授权角色同样必备。

**`权威业务截断边界`不是时间戳。** CONTEXT 单列了它的定义并明写「客户端时间、消息到达时间、
数据库写入时间或外部墙钟均不能单独替代该边界」；它与`生效时间`分工不同——生效时间说从何时适用，
截断边界裁决并发的新尝试是否合法。做成一个时间字段就把这条定义删掉了。

**派生规则**（CONTEXT「面单继续尝试」一节与「当前无有效终局且没有生效关闭时派生为开放，仍有
生效关闭时保持受控关闭」一句）：`开放` = 当前无有效终局 **且** 无生效且未被重开的关闭；否则
`受控关闭`。现行读面那个 `deriveContinuedAttemptOpen` 的形状与此一致，缺的只是两个真实输入。

**票面红线「登记册为空要说得出没有人作过决定」怎么落。** 按 CONTEXT，空登记册加无终局派生的就是
`开放`——与「最近适用决定为重开」派生出的是同一格。所以**不能加第三格**（那就是新造领域语言），
要分辨得开只能在读面**另外交代决定历史在不在**：判断值仍两格，页面据此说得出「没有人作过决定」
与「有决定历史、最近适用为重开」的区别。

**可复用的既有值类型**：`RequesterReference`、`DeciderReference`（两者分立的理由 CONTEXT 已写死：
「登录操作人可以作为操作证据，但不能替代实际决定方和授权角色」）。授权依据快照沿本仓惯例按用途
另立类型，不与 `AmendmentAuthoritySnapshot` 共用。

## 进度：领域层已落，余三层

2026-09-02 MCP-1。**已落**（`7c79b3d`）：`domain/continued_attempt.go` 与
`domain/continued_attempt_register.go`，十条用例。要点都在那两个文件的注释里，此处只记接手点。

- `ContinuedAttemptRegister` 按（租户 + 包裹）成册，决定追加不可覆盖；`Judge(currentFinalPresent)`
  现算判断、不存列；`HasAnyDecision()` 承载「没有人作过决定」那一句；`RehydrateContinuedAttemptRegister`
  已备（重建时终局一律按不在场传入，理由见其注释）。

**余下三层，按此顺序做：**

1. **端口**：`ports` 加登记册仓储（`FindByParcel`／`Insert`／`Save`，写入代数照
   `LabelTransactionInsertOutcome`／`SaveOutcome` 那一对分立），以及读面取数口。
2. **持久化**：**本层要新迁移**（`parcel_shipment/0011`），与票 `09` 不同——那一票落的是聚合内的
   小值走既有快照，这一册是**另一个聚合**（键为租户+包裹），没有现成的行可挂。行模型可照
   `0010`：键 + `revision` + `snapshot jsonb`。迁移用目录级 `go:embed`，**不必碰
   `migrations/migrations.go`**（已核，`all:parcel_shipment` 是整目录嵌入），因此并行会话那条
   「共享接线文件占号」不适用于本票。真库必须实跑。
3. **读面派生**：把 `adapters/postgres/label_transaction_views.go` 的 `deriveContinuedAttemptOpen`
   换成真输入。**注意它今天恒答开放且注释写明「不是默认值」**——换真之后那句注释要一并改，
   否则它会从一句诚实的说明变成旧话。同时按上文红线，读面要另外交代决定历史在不在。

**一处接手时要当心的**：读面那一格还需要「当前有效终局在不在」。终局属本上下文（`ParcelFinalOutcome`）
但不在本册，也不在面单交易快照里——读面怎么拿到它是这一层第一个要答的问题，别顺手在本册里存一份。

## 完成记录

2026-09-03／04，MCP-1。四层各一笔：

| 层 | 提交 | 落点 |
|---|---|---|
| 领域 | `7c79b3d` | `domain/continued_attempt.go`、`domain/continued_attempt_register.go`：两种决定、两格判断、`Judge` 现算、`HasAnyDecision`、重建门 |
| 端口 | `3b37845` | `ports.ContinuedAttemptRegisterRepository`（`FindByParcel`／`Insert`／`Save`），写入代数照面单交易那一对分立 |
| 持久化 | `0d492b8` | 迁移 `parcel_shipment/0012`、`adapters/postgres/continued_attempt_register.go`；键＋`revision`＋`snapshot jsonb`，无判断列 |
| 读面派生 | `08e62ec` | `adapters/postgres/label_transaction_views.go` 的 `continuedAttemptJudgments`：一次取回覆盖包裹的登记册与 `final_outcome` 的 `is_current` 行，逐件由 `Judge` 现算；`ports.LabelTransactionParcelRow` 加 `ContinuedAttemptDecided`；传输层加 `continuedAttemptDecided`、依据代码换 `DERIVED_FROM_DECISION_REGISTER_AND_CURRENT_FINAL`；admin-web 页头改述规则、「开放」下另注来源 |

**逐条完成判据：**

- 登记册四处齐——见上表。
- `ContinuedAttemptOpen` 不再派生自空历史——`deriveContinuedAttemptOpen` 那个恒答开放的函数已删，换成 `continuedAttemptJudgments.judge`，输入是真登记册（没开过册的按空册）与真终局。原来那句「不是默认值」的注释随之一并换掉，没留成旧话。
- 红线「登记册为空要说得出没有人作过决定」——按「取词」一节的结论**没有加第三格**：判断仍两格，读面另交出 `ContinuedAttemptDecided`；页面「开放」下注「没有人作过决定」或「最近适用决定为重开」（后者取 CONTEXT 生命周期原词）。
- 「当前有效终局在不在」怎么拿——读 `final_outcome` 的 `is_current` 行，只问在不在，不抄进本册。
- 「开不出空册」这条本不可达的失败路报错而不答零值：零值读出来是「受控关闭、无人决定」，一格看着合法的答案会把坏行藏起来。

**真库用例**（`label_transaction_test.go`）：一笔交易四件包裹各占一格——没开过册／一条生效关闭／关过又重开／仅当前终局在场——加他租户同号包裹上的关闭渗不进来。原 `TestTheLabelTransactionViewExpandsOneRowPerCoveredParcel` 里「恒为开放」那条断言改成同时断无决定历史。

**验证**：`08e62ec` 在共享树上——`gofmt -l` 对七个改动文件为空；`go build ./...`、`go vet ./internal/parcelshipment/...` 退 0；`go test -count=1 ./internal/parcelshipment/adapters/postgres/ -run 'LabelTransaction|ContinuedAttempt' -v` 十二条 `PASS`（DSN 55432，非 `SKIP`）；`adapters/http` 三条 `PASS`；admin-web `tsc --noEmit` 退 0、`node scripts/run-tests.mjs` 38 通过。全仓按 `08e62ec` 在临时 worktree 上另验（干净检出，DSN 55432）：`gofmt -l .` 空；`go build ./...` 与 `go vet ./...` 退 0；`go test -p 1 -count=1 ./...` 退 0、零 `FAIL`（含真库）；`tools/mechanism-inventory` 在该检出上重生成，与提交版逐字节一致，本票不需重生成清点。

**刻意没做**：写面（形成关闭／重开决定的命令口）——它要先过 party-commercial 的授权校验，属另一张票（持久化那笔的提交信已写明「本适配器今天没有生产写入方」）；`label_transaction_views.go` 之外没有第二处消费 `Judge`。
