# 38 PS 代码注释随文档自洽 06 改口

Category: chore
Status: resolved——2026-10-27 进 main（见文末「进 main 记录」）。此前 in-progress——完工待评审与进 main，分支 claude-cmtB
Blocked by: 无
出处：[audit/ps.md](../audit/ps.md) PS-04、PS-09、PS-16、PS-17、PS-49 的「代码注释随票改」半边，与 [ADR-0255](../../../docs/adr/0255-a-submitted-request-is-regrouped-only-by-withdrawing-it-and-submitting-linked-new-requests.md)、[ADR-0257](../../../docs/adr/0257-a-lapsed-label-success-only-feeds-the-label-service-final-and-leaves-the-channel-parcel-number-relation-applicable.md) 后果节点名的两处。

## 逐处

| 位置（符号） | 今天写的 | 改成什么 | 依据 |
|---|---|---|---|
| `internal/parcelshipment/application/submit_shipment_request.go` 包头注 | 「它不含任何持久化、事务或事件机制，那些仍阻断在 Bento 闸门之后（ADR-0017）」 | 持久化与事件机制在 `adapters`，不进本包 | ADR-0026；UC-PS-001「开发拆分建议」末段 |
| 同文件 `OutcomeLinkIneligible` 头注 | 「客户该走的是资料修订（待决）或等决定」 | 普通纠错走同一委托的新提交版本；重组先撤回原委托，再以撤回重提方向关联 | ADR-0255 |
| `application/withdraw_shipment_request.go` 的 `releaseOccupation` 头注 | 「从未形成过冻结时」 | 没有任何控制项成立时 | PS CONTEXT「委托撤回不能伪装成运营企业拒绝」一条；ADR-0125 决定四 |
| `domain/customer_source_data.go` 的 `SourceDataAdoptionOutcome` 头注 | 「CONTEXT 声明的封闭集合有五项……`待复核`是 UC-PS-002 对 CONTEXT`待补充`那一格的点名叫法」 | CONTEXT 词条「当前客户资料版本采用判断」的取值，`待复核`是与`待补充`分开的一格，续办方是授权复核角色 | PS CONTEXT 词条 |
| `application/cancel_parcel.go` 的 `ErrIntakeBoundaryCrossed` 分支注释 | 「没有授权处置决定时保持待处置（AT-PS-082）」 | 没有收寄后服务处置决定时保持待处置 | UC-PS-006 `AT-PS-082` |
| `domain/external_identifier.go` 的 `ExternalIdentifierChangeKind` 头注 | 「已失效不在其中：有效期规则今天没有登记处，没有执行器会形成它（ADR-0184 决定七）」 | 外部标识关联只有作废与替代两种终态；面单有效期届满只进面单服务终局判断，不改号码关联（ADR-0257） | ADR-0257 |
| `application/judgment_continuation.go` 的 `SourceDataAmendmentStageUndetermined` 一段注释 | 「今天是关务与装袋的读面未接」 | 两只读面已接提供方真读面（ps-port-remainder/05）；判不出是六格事实里有答「不知道」的 | ADR-0118 Status 行注记 |
| `domain/declared_measurement*.go` 等处注释 | 「申报测量」 | 声明测量 | PS CONTEXT 词条「声明测量」 |
| `adapters/http/withdraw_shipment_request.go`、`amend_customer_source_data.go`、`form_new_submission_version.go` 头注 | 把 `!found` 读成「统一不可见结果不拆开」而答 5xx | 随 [code/22](./22-customer-face-http-contract-misses-accepted-adrs.md) 一起改，本票不动 | ADR-0022 |

## 三类

不涉。只改注释。

## 规模

S。

## 完成记录（2026-10-27，作者会话，分支 claude-cmtB，基 origin/main `f6a4a5298`）

只改注释，`git diff -U0` 去掉注释行后无变动；测试字符串（`t.Fatalf` 里的「申报测量」）属代码，没动。

### 逐处结果

| 表中位置 | 结果 |
|---|---|
| `submit_shipment_request.go` 包头注 | 已改：持久化、事务与事件机制在 `adapters`，不进本包（ADR-0026） |
| `OutcomeLinkIneligible` 头注 | 已改：关联方向锚在原委托终态；普通纠错走同一委托的新提交版本，重组先撤回再以撤回重提方向关联（ADR-0255） |
| `releaseOccupation` 头注 | 已改：「没有任何控制项成立时」；同函数头注里「步骤 6」改引 UC-PS-005 释放一步的原文 |
| `SourceDataAdoptionOutcome` 头注 | 已改：取值集合指向 CONTEXT 词条「当前客户资料版本采用判断」，`待复核`与`待补充`分开写（续办方不同）；原文的「待下游判断」已不是取值，删去；「本仓『枚举取值……』」不是任何文件的原文，去掉引号改成转述 |
| `cancel_parcel.go` `ErrIntakeBoundaryCrossed` 分支 | 已改：「没有收寄后服务处置决定时保持待处置」 |
| `ExternalIdentifierChangeKind` 头注 | 已改：只有作废与替代两种，面单有效期届满只进面单服务终局判断（ADR-0257） |
| `SourceDataAmendmentStageUndetermined` 一段 | 已改：两只读面接提供方真读面，判不出是六格事实里有答`不知道`的（ADR-0118）；同一过时说法还在 `ports/amendment_stage.go` 的 `CustomsStageFacts`、`CustomsStageView`、`ConsolidationStageView` 注释，两个适配器注释里的票号与已退场的 `Unconnected…View`，以及 `amend_customer_source_data_test.go` 的 `unconnectedCustoms` 注释（指向已不存在的符号），一并改 |
| 「申报测量」 | 已改：PS 包内 32 处注释行改「声明测量」（CONTEXT 词条「声明测量」的 Avoid 栏列了它）；字符串字面量不动 |
| `adapters/http/*` 三处头注 | 按表不动（随 code/22） |

票里列的位置没有一处已漂移或不存在。

### 同类残留（PS 包内）

- 不是原文的引文，改成转述或换成原文：`withdraw_shipment_request.go` 的「按原业务关联幂等形成适用冻结释放」「读取已提交决定并返回既有结果」；`amend_customer_source_data.go` 的「未登记时只能形成未决或业务拒绝，不能以系统便利推断允许」（UC-PS-002 实际写的是「未声明答等复核……不能以系统便利推断允许」）及测试里转引它的注释；`ports.go` `CommercialBasisResolver` 注释里 UC-PC-002 的两句引文（「必须在类型和测试中可见」「两阶段端口」在原文里不存在）。
- 行位置词：`amend_customer_source_data.go`、`_test.go` 的「往下二十行」。
- 跨文件计数：`CancelParcelOutcome`（UC-PS-006 的「四种结束」「三格」）、`query_label_transactions.go`（CONTEXT「只有两格」）、`form_acceptance_decision.go`（ADR-0282 的「三格」）、`CommercialBasisResolver`（UC-PC-002 的「三个阶段」，原文其实写两阶段）、`labelfinal` 测试（CONTEXT「分四格」）。
- 变更叙述与「今天」：`AmendmentIntent`、`ExternalIdentifierKind`、`ExternalIdentifierRelation`、`judgment_continuation.go` 三处（「此前」「不再」「上一条」）、`declared_measurement_resolution.go`（「这是今天唯一带测量的一格」已经不实：已采用版本格也带测量）、`declared_measurement_view_test.go`、`shipment_request.go` 一处。
- 核了 PS、PC、PP、SA、CR 五个包注释里所有 `ADR-NNNN`、`AT-XX-NNN` 引用，编号都存在；「指错了号」这一类无法机器核，没有再发现。

### 代决

1. 只扫了五个包里票点名的几类：行位置词、跨文件计数、非原文引文、过时的「今天」叙述与已退场符号。`今天`、`不再`、`照旧`在 PS 包注释里各有一百来处，大半是在讲代码逻辑本身，逐一改写不是这张票的范围，没有动。
2. 票号（如 `pp-seams/05`、`ps-port-remainder/05`）：被我改写的几处去掉了，其余没碰。
3. UC 的「步骤 N」标号当作 UC 自己的名字，只在顺手改的两三处换成了原文；没有全包清理。

### 越权风险点

- `SourceDataAdoptionOutcome` 删去了「待下游判断」：依据是 CONTEXT 词条明说「下游要不要据新版本重新判断不是一个取值」。PS owner 若认为那一格仍要保留，需要先改 CONTEXT。
- `待补充`写成「本层没有判据」：代码里确实没有产生它的规则，但它是否该由本层产生，没有裁。

## 进 main 记录（2026-10-27，协调方）

- 分支 `claude-cmtB` 与 code/38、40、45、55 同批，由协调方快进，tip `c6f983ea2`；隔离评审（Sonnet）可合入，无阻断：每个改动的 `.go` 文件在去掉注释后语法树逐个比对（程序放评审私有目录，对有代码变动的老提交试过会报差异），38、40、45 三个提交 0 差异；新增的 38 条带引号或「」的引文机械核了 36 条逐字相同，另 2 条是带省略号的前缀引文与一句假想读者的话；新增的 ADR、UC、AT 指针逐条核过。协调方单独提交重算的 AT 计数、重生成的 ADR 索引与机制清点（本批无变动）；全量 `scripts/test-full.sh` 退 0、159 个包 ok；`go build ./...`、`go vet ./...` 干净；管理台 `tsc -b --noEmit`、`node scripts/run-tests.mjs`（803 过）、`vite build` 与 `./internal/architecture/...` 全过。无迁移、无新 ADR、无 AT 号变化。
- 本票：PS 注释表里七处按期望改了，`adapters/http` 三处按表不动（随 code/22）；「申报测量」改「声明测量」；`SourceDataAdoptionOutcome` 里不是 CONTEXT 词条取值的「待下游判断」删掉；`ports/amendment_stage.go` 与两个适配器注释里过期的「今天是关务与装袋的读面未接」一并改了。评审核实 `ports/operator_registration_handoff.go` 原先引的「不在消费门认原因名字」在 ADR-0094 里并不存在，改成转述是对的。
- 去向：无新票。
