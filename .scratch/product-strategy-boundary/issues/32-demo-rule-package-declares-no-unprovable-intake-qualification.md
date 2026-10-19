# 32 演示规则包不再声明没有证明方的收寄硬资格：有效网络收寄的采用在演示上越过资格一格

Category: task
Status: 完工，待评审与进 main——2026-10-18 Claude Code 会话立并实施（用户「你作为业务和系统专家自决……解决所有的问题」授权，代决记在「裁定」），分支 `claude-psb32`。出自 2026-10-16 在 `c68e4342` 上重走演示动线（[05](./05-demo-journey-criterion-evidence.md)）。
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
- **租户取值（演示租户）**：规则包声明哪些收寄硬资格。演示租户声明一条产品给不出证明方的资格，等于把自己的收寄采用永远挡在未证明——真实租户这样配也是同一个结果，那不是演示该展示的路。

## 裁定（代决）

演示规则包的收寄资格只保留两种来源（节点收寄、场外揽收），不声明硬资格；摘要按 PCC-1 重算。

不采的两条：

- 不造恒答已证明的证据口，也不在装配里替演示认领一个前缀：那正是 ADR-0063 Consequences 禁的「为了变绿拆出一个假身份」。
- 不把这条引用改写成节点认领段下的引用：证明它要一件节点协作执行事实，那只出在关务协作链上（UC-NO-001）；认领本身也没有登记口（33）。

越权风险点：演示动线因此不再演示「收寄硬资格」这一格。资格判断仍由 `cmd/parcel-dispatch/intake_qualification_evidence_wiring_test.go` 的装配用例与 PS 的领域用例钉着。33 落地、关务协作链在演示上走得通之后，演示租户可以按普通登记重新声明一条可证的资格。

## 做什么

1. `SYN-RULEPKG-01` 的 `intakeQualification` 去掉 `qualifications`，`contentDigest` 换成登记口算出的那一串。
2. `dom-check.sh` 的规则包页签一格不再期望这条资格。

## 完成判据

- 55432 上自己命名的一次性库干净灌种子退 0，规则包 `PUBLISHED_EFFECTIVE`。
- 照动线「前置」实走，经作业事实口收寄一件已接受委托的包裹：采用越过资格一格，`intake_adoption` 落行。证据只记 `S`，走完删库。
