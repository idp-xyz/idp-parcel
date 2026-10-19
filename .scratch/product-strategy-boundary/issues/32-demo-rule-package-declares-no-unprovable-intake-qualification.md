# 32 演示规则包不再声明没有证明方的收寄硬资格：有效网络收寄的采用在演示上越过资格一格

Category: enhancement
Status: resolved——2026-10-18 进 main（至 `23227f44`）。2026-10-18 Claude Code 会话立并实施（用户「你作为业务和系统专家自决……解决所有的问题」授权，代决记在「裁定」），分支 `claude-psb32`。出自 2026-10-16 在 `c68e4342` 上重走演示动线（[05](./05-demo-journey-criterion-evidence.md)）。
Blocked by: 无
地盘：`scripts/demo-seeds/data/commercial/publish-batch.json` 的 `SYN-RULEPKG-01` 一项；`.scratch/admin-web-page-wiring-frontier/dom-check.sh` 里期望这条资格的那一格。
出处：[ADR-0063](../../../docs/adr/0063-intake-qualification-proof-is-a-consumer-side-evidence-port.md)；[ADR-0150](../../../docs/adr/0150-synthetic-tenant-is-treated-as-a-real-tenant-and-isolated-form-retires-per-face.md)；先例 [26](./26-demo-tenant-adopts-the-pre-acceptance-estimate-form.md)（规则包拿掉没有生产者的 `REQUIRED_DOCUMENT`）。

## 现状（实测，钉 `c68e4342`，证据 `S`）

- 经受控 CLI 灌一行收寄能落库，`node-operations.node-intake.formed` 交 PS 采用。采用答 `network intake adoption is undecided: ELIGIBILITY_NOT_ESTABLISHED`，三次后信封落 `ABANDONED`，`intake_adoption` 零行。
- 成因：演示规则包 `SYN-RULEPKG-01` 的收寄资格声明了硬资格 `SYN-QUAL-REALNAME-01`。`cmd/parcel-dispatch` 交给采用的资格证据口是 `intakeQualificationEvidence(db, nodeQualificationAuthority{})`：本部署没认领任何权威段，恒答未证明（ADR-0063）。
- 产品里没有任何一口能为这条引用作证：
  - 唯一的证明方是节点执行事实，它只为节点认领的那一段作证。
  - 认领哪一段、哪条引用由哪件执行事实证，今天写在装配代码里，没有登记口，见 [33](./33-intake-qualification-node-authority-has-no-registration-path.md)。
  - 「实名」在领域文档里没有对应概念。

## 三类归法

- **机制**：资格判断、证据口、节点作证适配器都已在 main，本票不改代码。
- **租户取值（演示租户）**：规则包声明哪些收寄硬资格。`SYN-QUAL-REALNAME-01` 是开发方编的合成值，背后没有任何业务要求。真实租户若真要这条资格，停在未证明正是诚实的产品答案；只因为这一条是虚构的，演示租户才可以如实声明「无硬资格」。

## 裁定（代决）

演示规则包的收寄资格只保留两种来源（节点收寄、场外揽收），不声明硬资格；摘要按 PCC-1 重算。

不采的两条：

- 不造恒答已证明的证据口：ADR-0063 决定四要求清单非空时由显式未配置实现答未证明。
- 不在装配里替演示认领一个前缀：`nodeQualificationAuthority` 整个部署共用一份、不分租户，替演示填它等于替所有租户填（红线「租户取值留给租户」，ADR-0150 决定一）。
- 不把这条引用改写成节点认领段下的引用：证明它要一件节点协作执行事实，那只出在关务协作链上（UC-NO-001），而那条链今天没有生产调用方；认领本身也没有登记口（33）。

与 ADR-0063 的关系：它的 Context、决定四与 Alternatives 都禁「清空清单来越过未证明」。那一禁管的是两件事：机制不得把非空清单默认成立；SYN-PC-SEED 这份测试夹具上两条真库链钉在 `NOT_ESTABLISHED`。本票两件都没碰。决定四首句同时写明，显式空清单就是合法的「无硬资格」声明。演示租户按 ADR-0150 当真实租户对待，它声明的是自己的业务里没有这条要求，不是为了越过一条真要求而清空。

越权风险点：
- 数据上分不出「租户声明无硬资格」和「为越过未证明而清空清单」。owner 也可以宁可让演示停在资格未证明，等 33 与协作链的生产面落地；回退就是发一版重新声明这条资格的规则包。
- 演示动线因此不再演示「收寄硬资格」这一格。资格判断仍由 `cmd/parcel-dispatch/intake_qualification_evidence_wiring_test.go` 的装配用例与 PS 的领域用例钉着。33 落地、关务协作链在演示上走得通之后，演示租户可以按普通登记重新声明一条可证的资格。

## 做什么

1. `SYN-RULEPKG-01` 的 `intakeQualification` 去掉 `qualifications`，`contentDigest` 换成登记口算出的那一串。
2. `dom-check.sh` 的规则包页签一格改为期望页面渲染的「已声明,无硬资格」。

## 完成判据

- 55432 上自己命名的一次性库干净灌种子退 0，规则包 `PUBLISHED_EFFECTIVE`。
- 照动线「前置」实走，经作业事实口收寄一件已接受委托的包裹：采用越过资格一格，`intake_adoption` 落行。证据只记 `S`，走完删库。

## 完成记录（2026-10-18，Claude Code 会话，分支 `claude-psb32`）

- `SYN-RULEPKG-01` 的收寄资格只留两种来源；`contentDigest` 换成登记口算出的 `PCC-1:d6b23fa1…`。没有别处钉着旧摘要。
- 实走（`S`）：一次性库 `c8_walk3`，代码取 main 加票 30、本票两条分支合起来的树。
  - 种子灌入退 0，规则包 `PUBLISHED_EFFECTIVE`。
  - 提交委托，初始路由 `ROUTE_FORMED`。
  - 带上海枢纽设备、用客户包裹号作标识经收寄口收寄，答 `201 INTAKE_FORMED`；PS 采用落 `intake_adoption` 一行，已采用。
  - 下一停点是路由复核：`parcel-shipment.network-intake.recorded` 消费答未决 `PLAN_REVIEW_INCONCLUSIVE`，见 [routing-first-cut/21](../../routing-first-cut/issues/21-reassessment-after-network-intake-carries-no-geo-projection.md)。
  - 走完已停进程并删库。
- 种子摘要自校与读 `publish-batch.json` 的用例不受影响。

## 评审记录（2026-10-18，隔离子代理一次过两轴）

- 阻断 1 条，已修：裁定原先没回应 ADR-0063 禁「清空清单来越过未证明」的三处原文。现在「裁定」写明那一禁管的是机制与 SYN-PC-SEED 夹具；越权风险点补上数据上分不出两种意图、owner 可回退的那一条；「三类归法」收窄到「只因这条是虚构的」。
- 非阻断已修：
  - 33 补上第二个缺口：协作链今天没有生产调用方；「不带租户维」改成「认领不带租户维」。
  - 本票补完成记录。分支已 rebase 到含票 30 的 main。
  - 演示动线补记改了下一停点。
  - 否掉「装配里认领前缀」的依据改引红线与 ADR-0150。原先那句「拆出一个假身份」在 ADR-0063 里搜不到。
  - `dom-check.sh` 改成期望「已声明,无硬资格」。Category 对齐词表。

## 进 main 记录（2026-10-18）

- 全量 `go test -p 1 -count=1 ./...`（带 55432 DSN）在干净游离检出上钉 `f0996b8a`：143 包 ok，0 FAIL，0 SKIP。
- 之后 rebase 跨过 main 上关务一侧的几笔（领域与用例）。本票没有 Go 改动，读 `publish-batch.json` 的用例不碰关务，所以没重跑全量。
- 快进推到 main，至 `23227f44`。机制清点不变。
