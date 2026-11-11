# 10 受控补充收平台号：受理前补报或更正平台货件号、平台参考号与平台箱标号

Category: enhancement
Status: in-progress
Blocked by: 无（已提交，未进 main；分支 `feat/goods-details-admin`，与 09、11、12 同一条线）

依据：[ADR-0376](../../../docs/adr/0376-fba-submission-conditions-are-judged-as-required-documents-platform-identifiers-may-be-supplied-before-acceptance-and-rule-packages-follow-the-service-product.md) 决定二（部分停用 ADR-0374 决定四一句）；PS CONTEXT「外部标识」词条。

## 要做的

- 首方受控补充口的载荷照提交口同形，收平台货件段（`platformShipment`）与逐件 `platformBoxLabel`；去掉「箱标号只在首次提交声明」的拒收。
- 补充重报整份内容：带了平台段，即这一版起委托的平台声明（整段替换）；没带，即这一版不报。形状规则与提交口同：平台与货件号同在同缺，箱标号要有平台货件，委托内不重号。
- 规范化 `PSU-1` 与提交共用编码（ADR-0373 决定八），平台段进补充摘要；旧的不带平台段的补充摘要逐字节不变。
- 领域：`FormNewSubmissionVersion` 收平台声明并替换委托上的现行声明，对着新成员集合核。接受后不变（补充本来只在受理前）。
- 被补充撤掉的箱标号不再算歧义信号（读快照即随之更新），并照样响声明撤除门铃，与拒绝、撤回同一条。
- 首方契约文档补充一节写明：平台段随补充整份重报，没带即清空。

## 完成判据

- 补充补报箱标号后，按号核对不再答歧义；补充撤掉的箱标号响撤除门铃；带平台段的补充重放答同一结果、改了平台段答来源冲突。
- 旧补充摘要的常量不变。

## 不做的

接受后改平台号（ADR-0376 决定二：撤回重下或关联新委托）。

## 完成记录（作者一侧，2026-11-11）

- 提交：分支 `feat/goods-details-admin` 上的 `99d149d4c`（在分支 `feat/supplement-platform-ids` 上写成 `5a383c946`，基线 `22a710c40`，拣入时无冲突）与评审修订 `95386e6f4`，未推送、未进 main。
- 落点：
  - 首方补充口照提交口同形收 `platformShipment` 与逐件 `platformBoxLabel`，去掉「箱标号只在首次提交声明」的拒收。补充整份重报：带了平台段即这一版起委托的平台声明，没带即这一版不报、原声明撤下。
  - 形状规则与提交口同一处：接入口对着客户包裹参考译一次进摘要、对着回执成员再译一次进命令（`declaredPlatform`）；聚合 `FormNewSubmissionVersion` 对着新版本成员再核（`platformBoxLabelsFor`），换下委托上的平台货件与全部箱标号。受理前才走得到，接受后平台号随关联固定。
  - `PSU-1` 与提交共用编码，平台段进补充摘要。钉了取证于改动前提交 `22a710c40` 的补充摘要常量 `PSU-1:6597db4a1ee310ca643b57f76861ca6cf143c60ec77ab9f17ce9ff4f5bdf74ca`，不带平台段的补充逐字节同前。
  - 歧义信号读快照，补充换下的箱标号随保存即不再算。保存那一笔的同一条 SQL 读出原箱标号，换下的号各响一封声明撤除门铃，与拒绝、撤回同一种铃；信封 ID 另带新提交版本，一个号先被换下、再报回、后随拒绝撤下，两次都响，不被当成重放。补了门铃的无事务负向证据（架构门要求）。
  - 首方契约文档的补充一节写明平台段整份重报、没带即清空、接受后不再改。
- 判据：
  - 补充补报箱标号后按号核对带歧义信号、换下的号不再带；换下的号响撤除门铃；箱标号没变的补充不响 ✅（真库）。
  - 带平台段的补充重放答已处理、同一版本；同一补充请求标识改了箱标号答来源冲突 ✅（`cmd/parcel-api` 真库端到端）。
  - 旧补充摘要常量不变 ✅。
  - 领域：补充替换平台声明、没带平台段即撤下、没有平台货件的箱标号、两箱同号、指到成员之外都拒 ✅。
- 未做：接受后改平台号（本票「不做的」）。
- 验证（含 PG，`IDP_PARCEL_POSTGRES_DSN` 指 55432 门禁库）：`go build ./...`、`gofmt`、`go vet` 通过；`./internal/parcelshipment/...`、`./internal/architecture/...`、`./cmd/parcel-api/...`、`./internal/nodeoperations/...` 全部 ok。
- 评审（非作者，2026-11-11）：无阻断；四处说法跟不上新来由，已改（`95386e6f4` 与文档分支同日一笔）：
  - NO 消费门的事件类型注释只写了拒绝、撤回，补上受理前补充换下平台箱标号；
  - 信封 ID 口名注释只写了（租户，委托，号码）一种认领，补上带提交版本的那一种；
  - `RingBoxLabelsWithdrawn` 的举例说「三次都要响」，报回来并不响，改为换下与撤下两次；
  - CONTEXT-MAP `parcel-shipment → node-operations` 声明撤除的交接与 PS CONTEXT 同一条规则句补上补充这一来由，以及同一个号被几份补充换下各算一封。
- 全量门禁（拣入 `feat/goods-details-admin` 后、含 09 与 11）：带 PG 的 `go test ./internal/... ./cmd/... ./referenceconfig/... ./migrations/...` 退 0；管理台 `tsc`、`run-tests` 1345 过 0 败、`vite build` 退 0。`scripts/test-full.sh` 未跑。
