# 40 容量池的容量更正没有入口：登记口只答池冲突，缩容形不成超配缺口

Category: enhancement
Status: needs-triage——2026-10-19 Claude Code 会话立（用户「你作为业务和系统专家自决……解决所有的问题」授权）。出自 [17](./17-capacity-pools-are-registered.md) 的隔离评审。
Blocked by: [17](./17-capacity-pools-are-registered.md)、[20](./20-capacity-reservation-and-release-decision-faces.md)
出处：[ADR-0201](../../../docs/adr/0201-schedule-templates-materialize-concrete-schedules-by-date-and-preparation-answers-not-configured-on-an-empty-register.md) 文末补记「登记口不改池」与越权风险点 19。

## 现象

票 17 落了容量池的登记口：同一（班次、维度）同内容答`已存在`，异内容答`池冲突`，库里那一池不动。登记口立池，不更正池，这是有意的。但容量事实变了（车换小了、舱位被收回、上限登错了要改）目前没有任何入口：

- UC-TF-003「一致性、权限与恢复」要资源或容量事实更正形成新版本，原判断与原准备历史保留；池表没有版本，做不到。
- TF CONTEXT Rules 要容量后来缩减时不改写既有预占，形成明确的超配缺口，再对受影响范围发起释放、换班、重新分配或升级处置；Lifecycles 也有「容量缩减……形成超配缺口并进入授权处置」一条。现在没有缩容这个动作，缺口无从形成。
- 眼下的补救只有不登那一维或换一个班次，都不是更正。

## 证据（读码）

`internal/transportfulfillment/application/prepare_transport_opportunity.go` 的 `poolReplayOrConflict` 同身份异内容只答 `PoolConflict`；池表（TF 迁移 `0027_capacity_pool_dimension_and_template_capacity.sql` 之后）一池一行，没有版本列；预占挂在池身份上（`capacity_reservation` 表）。

## 三类

更正入口与版本化是机制；缩容后超配缺口怎样处置（释放谁、换班还是升级）是产品策略，已在 [psb/09](../../product-strategy-boundary/issues/09-tf-fulfillment-judgment-methods-and-connectors.md) 第 6 项待核；硬软阈值与授权取值是租户取值（`PAR-NET-12`）。规模 M 到 L。

## 最小的诚实修法

池加版本：更正是一种运营决定或登记更正（定族照 ADR-0151 判），指名前版，形成下一版，前版与据前版做过的准备判断留存。缩容时既有预占不改写；新上限低于已预占量就形成一条超配缺口记录（缺口量、受影响预占），交给处置。扩容只成新版。处置方法先内置一种（例如只形成缺口、等运营决定释放），等 psb/09 第 6 项核完再扩。

## 要裁的

- 更正走登记册配置写还是运营决定（ADR-0151 的判据：缩容会形成生产事实，即超配缺口，倾向运营决定）。
- 超配缺口是否发信封、谁订阅（NR 容量风险信号那条边与 D03-7 同一处等 NR owner）。
- 周期安排换版改了某维容量时，已物化的池要不要跟（票 17 定的是登记口不跟）。

## 既有票

none
