# 06 面单交易写侧执行器全缺：聚合方法齐备而编排一环没有

Category: enhancement
Status: resolved——五步编排链与测试落地，棘轮基线两条随之剪掉，见文末「完成记录」
Blocked by: 无

## 缺口

`internal/parcelshipment/application/` 下**一个面单交易用例都没有**（实测于 `957e768`，
该目录的执行器全是委托/受理/取消/终局侧）。`internal/architecture/production_wiring_baseline.txt`
把 `EstablishLabelTransaction` 与 `EstablishPriorLabelTransactionLink` 显式登为「无生产调用方」，
并注明这是设计而非遗漏——[ADR-0084](../../../docs/adr/0084-label-transaction-is-an-independent-aggregate-with-two-level-results.md)
立聚合时就明写「写入方缺席是设计」。

[ADR-0088](../../../docs/adr/0088-label-channel-service-enters-the-first-release-service-forms.md)
把这条缝从墙后之物变为首发主链路，**「设计如此」的理由随之失效**：它 Consequences 原话是
「它立下的面单交易聚合正是本决定落地时要接上写入方的那一个」。

## 做什么

补「委托提交 → 建立交易 → 提交渠道 → 记录结果 → 追加后续动作」这条编排链，落
`internal/parcelshipment/application/`。聚合侧已齐，本票不改领域：

- 建立：`EstablishLabelTransaction`（`EstablishLabelTransactionSpec` 七类引用逐项过构造门）。
- 提交：`SubmitToChannel`；结果不确定：`MarkResultUncertain`。
- 记录结果：`RecordChannelResult`（两层结果一次写下，逐包裹结果经 `matchResultsToCoverage` 对齐覆盖范围）。
- 后续动作：`AppendFollowUpAction`（`ChannelVoidAction` / `ChannelRefundAction` / `ChannelReplacementAction`）。
- 重试/替代：`EstablishPriorLabelTransactionLink`，在**新**交易出生时建立且要求原交易已定案。

写入代数已在 `ports`：`LabelTransactionInsertOutcome` 与 `LabelTransactionSaveOutcome` 分立，
编排要把两者分别答完，不压成一格。

## 与相邻票的边界

- **不接真渠道**：提交渠道那一步的出向调用形状归 `07`，本票对它只留缝，用替身走通。
- **不定载荷落点**：渠道返回的 PDF/ZPL 往哪放归 `09`。
- 本票只做编排，`RecordChannelResult` 收到什么由替身给。

## 红线

- 不可覆盖、更正走版本链、停用走状态推进；定案 `Finalized()` 是派生谓词，**不要为它加存储字段**。
- 不填任何渠道账号、字段名、金额（实例半边）。

## 完成判据

编排链五步各有用例与测试；`internal/architecture/production_wiring_baseline.txt` 里那两条
「无生产调用方」的登记随之更正（**改它是本票的一部分，不是顺手**）；`gofmt -l` 空、
`go build`/`go vet` 退 0、`go test -count=1 ./...` 绿并注明含不含真库。

## 参照

[能力形状盘点](../capability-shape-inventory.md)第一段；ADR-0084；
`internal/parcelshipment/domain/label_transaction.go`。

## 完成记录

2026-09-02 由 MCP-4 落地，落 `internal/parcelshipment/application/operate_label_transaction.go`。
聚合一行未改——本票只补编排，那是立票时就写明的边界。

**五步一个 handler 而不是五个**：它们操作同一个聚合、依赖同一对端口，拆开只会让装配点多四次
接线而缝一条都不少。后四步共用一个 `advance` 骨架，因为它们在**恢复动作**上完全同形（读不回、
状态不允许、版本冲突三处一字不差），各写一遍就有四份会各自漂移的口径，而漂移在编译期不报。

**结果代数按恢复动作分格**（ADR-0029），五步共用一套六格：`已落下` / `重放` / `状态不允许` /
`原交易未定案` / `输入未受理` / `版本冲突`。领域已经把这条分界守在错误上
（`ErrInvalidLabelTransaction` 与 `ErrLabelTransactionStateNotAdmitted` 分立，
`ErrPriorLabelTransactionNotFinalized` 再单列），本层照搬不重新发明。

**时间分两种来源，与 `03` 的裁决同一条分界**：建立与提交渠道是我方的动作，时间取编排时钟；
渠道结果时间与后续动作时间随命令进来，不代铸——那是渠道那边的业务事实。

**不发起任何渠道调用。** 出向形状归 [ADR-0090](../../../docs/adr/0090-outbound-integration-result-algebra-partitioned-by-recovery-action.md)
与票 `07`，本编排只在提交之后留缝。测试里因此**没有任何渠道替身**——有的话就说明编排替调用方
发了请求。ADR-0090 那条「`答案未确定` 不得重发」在本上下文的落点是 `MarkResultUncertain` 落库：
之后重发路径读到的不再是`已提交渠道`，`SubmitToChannel` 的状态门自动挡住第二次提交，纪律由
状态机交付而不靠调用方自觉。用例 `TestOnceTheResultIsUncertainTheTransactionCannotBeSubmittedAgain`
钉住这一条。

### 棘轮基线：两条剪掉，顺带改正一处已经错了的计数

`production_wiring_baseline.txt` 那两条按它自己写的「写编排落地那天，两条一起出名单」剪掉。
剪之前按门禁提示逐条分过成因：两个名字全仓各只有一处声明，不存在「别处同名声明造成误判」
那一种。**同时留了一句边界**：该门禁量的是 domain 包外的非测试引用，而这条编排尚未进
`cmd/parcel-api` 的装配，所以「出名单」不等于「生产可达」，装配随 `07` 落。

**顺带改正**：剪之前实测条目数为 **34**，而表头流水账写着「现为 35」——某次增减只改了名单
没改那句。剪后的 32 是**实测**得来的，不是拿 35 减 2 算的（那样会得 33，继续错下去）。
这处不一致连同判据一并记在表头，没有静默改数。

### 验证

`gofmt -l` 空，`go build ./...`／`go vet ./...` 退 0，`go test -count=1 ./...` 退 0 **含真库**
（DSN 已设，同刻抽验真库用例为 `PASS` 不是 `SKIP`）。

**「提交后另在临时 worktree 复跑」这一句原写成了已完成，实际没跑。** 会话崩在 `git commit`
那一刻——存档末条即该命令且无结果，而这段验证文字在它之前就已写好并随同一笔暂存，于是一句
预期被当成事实留在了票面上。提交本身完整落地（五个文件全在 `97d7407`）。

补跑于 2026-09-02 由 MCP-1 在临时 worktree 检出 `97d7407`：`gofmt -l` 空、`go build ./...`
与 `go vet ./...` 退 0、`go test -count=1 ./...` 退 0；同刻抽验
`TestFreezeScopesAreInvisibleToEachOther` 为 `PASS` 不是 `SKIP`，故此绿含真库，
`TestOnceTheResultIsUncertainTheTransactionCannotBeSubmittedAgain` 亦 `PASS`。验证树按纪律
不加 `--force` 移除。
