# SA 采用信封的 ID 是「分区键 / 版本」串接、三个引用无长度上界，超框架 128 字节上限时 `Validate` 确定性拒；`handOffFact` 把它折成续办引用——版本行已提交、信封永不出，而 CLI 打出的「重跑同一命令补发同一封」在此因下永远为假

Category: bug
Status: draft——**2026-09-15 14:4x 通道 1 立票**（sa-cc/27 评审 ← 通道 2 Spec ① 转记；推送方处置时点名「另立 SA 票」）。与 [29](29-cc-handoff-envelope-id-exceeds-framework-limit-and-rederive-path-swallows-handoff-failure.md) 同一张脸、另一只手：29 只改 CC 三只 `*EventID`，SA 这一只不在其内。只写票面未动代码；取证锚第六批 tip `de822820`
Blocked by: 无（[27](27-sa-external-funds-fact-adoption-and-correction-registration-face.md) 已进 main——本票缺口是它让「从生产到达」第一次成立的；[20](20-sa-external-funds-fact-holds-one-row-per-fact-and-cannot-store-a-correction.md) 已进 main）

## 缺口（评审 ← 通道 2 钉 `aa48912e`，逐符号名；推送方未复量，作者开工先复）

- **ID 形**：`internal/settlementaccounting/adapters/postgres/external_funds_fact_handoff.go` 信封 ID = `externalFundsFactPartitionKey(...) + "/" + version`，分区键由租户与事实引用拼成；租户 / 事实 / 版本三个引用只经 `newRequiredValue` 的非空门，**没有长度上界**——它们全是实例半边，本仓给不出「够短」的保证（29 裁决 1 的同一句理由）。
- **上限**：`go.idp.xyz/idp-bento-go/eventing` `MaxEventIDLength = 128`，`Envelope.Validate` 对 `id` 确定性拒（29 裁决 0 量实）；`Subject` / `PartitionKey` 上限 512 同样只靠引用长度。
- **失败被折成续办**：`internal/settlementaccounting/application/map_external_funds.go` `handOffFact` 在 `Facts.Save` 落行后调 `FactHandoff.HandOffExternalFundsFact`，失败**不翻采用**、返回 `fundsContinuation("EXTERNAL_FUNDS_FACT_HANDOFF", …)` 续办引用，结果仍 `FundsFactAdopted` / `FundsFactExisting`。这是 27 之前就有的形（sa-cc/02），27 让它第一次能从生产入口（`cmd/parcel-settlement-register`）到达。
- **CLI 的话在此因下为假**：`cmd/parcel-settlement-register` `fundsAnswer` 对续办引用非空一格打「重跑同一命令补发同一封」并退 3——依赖故障下这句成立（重跑走 `已存在` 路再交一次），**信封不合法**下重跑永远同一结果，人照做只会一直退 3。
- **CC 那侧已收口的对照**：29 把重派路的确定性拒改成整笔硬失败、依赖不可用改成整笔未决重投，人重核路（有读者）兜底不动。SA 采用口今天只有 CLI 一个生产调用方（27），第二步端点在 [31](31-sa-external-funds-fact-registration-endpoints-and-admin-write-face.md)。

## 语言从哪里来

SA `CONTEXT.md` Boundaries「外部资金事实进入本产品只有本上下文的采用这一口」——这一口若把信封没发折成幂等成功，CC 永远收不到那一版，而 SA 侧看是`已采用`；`docs/agents/parallel-sessions.md`「不同的『绿』在输出上长着同一张脸」；ADR-0137 决定四。

## 做法候选（只列不选，归 SA owner）

1. **ID 指纹化（照 29 裁决 1）**：`externalFundsFactEventID` = 口名前缀 + `sha256(租户 / 事实 / 版本)` 十六进制，定长必在 128 内；载荷照旧全量。要量：CC 消费门（`ccinbox` 那只消费者）按信封 ID 恰一次去重，换形前后同一版本若各发一封会被当两封——CC 侧 `ReceiveFundsFact` 对同一（租户 / 事实 / 版本）是否幂等（29 裁决 4 的 SA 半边在这里反过来问 CC）。
2. **不合法信封不折续办**：`handOffFact` 对 `errors.Is(err, eventing.ErrInvalidEnvelope)`（或 handoff 包成的哨兵）**整笔回滚**、答一格新的未决 / 硬失败；依赖不可用照旧折续办（有 CLI 读者、重跑能补）。CLI `fundsAnswer` 随之多一格、那句「重跑补发」只对依赖故障说。
3. **只改 CLI 的话**：不改形，只让「重跑补发」按错误种类分说。治的是话不是病，列出防重开。

## 红线

- 不改 `FundsOutcome` 既有五格语义；不改 CC；迁移零 diff。
- 实例半边不写死：任何「够短」都写成对上限的量法。
- 27 落的六口装配、事务壳不动；第二步端点（31）若先落，本票在两个入口上同一形。

## 完成判据（待裁后写实）

1. 超长合成引用（串接形下 > 128 字节）下 `AdoptFact` / `CorrectFact` 交接**不再静默为成功**：要么信封发出、要么结果可观测且事务回滚。
2. `handOffFact` 对确定性拒与依赖不可用两格分开，CLI 退出码与提示随格。
3. 带 DSN SA adapters/postgres + `cmd/parcel-settlement-register` 全 PASS 非 SKIP。

## 地盘

`internal/settlementaccounting/adapters/postgres/external_funds_fact_handoff.go`（+ 测试）、`internal/settlementaccounting/application/map_external_funds.go`（`handOffFact`，+ 测试）、`cmd/parcel-settlement-register/{main.go,translate.go}`（`fundsAnswer` 一格）。撞点：[31](31-sa-external-funds-fact-registration-endpoints-and-admin-write-face.md) 碰 SA `adapters/http` 与 `cmd/parcel-api`，与本票不同文件；[33](33-sixth-batch-review-standards-tails-test-currency-code-count-words-sentinel-count-and-unanchored-byte-claim.md) 碰 `cmd/parcel-settlement-register` 的测试夹具（币种字面），先后进即可。

## 要裁的（归 SA owner）

1. ID 形取指纹化还是别的；换形对 CC 消费门去重的后果谁量。
2. 不合法信封在 SA 采用口响成什么（未决 / 硬失败），与 29 在 CC 侧的两格是否同名同族。
3. 27 的续办引用一格（依赖故障）是否保留。

## 参照

[27](27-sa-external-funds-fact-adoption-and-correction-registration-face.md) 完成记录判断项 ⑦ 与 Comments「评审 ← 通道 2」Spec ①；[29](29-cc-handoff-envelope-id-exceeds-framework-limit-and-rederive-path-swallows-handoff-failure.md) 裁决 0–4；[02](02-sa-external-funds-fact-adoption-hands-off-an-envelope.md)；ADR-0137 决定四；`internal/platform/dispatch/dispatcher.go` 失败码头注。

## Comments

- 2026-09-15 14:4x · 通道 1：立票（27 评审 ← 通道 2 Spec ① 转记）。只写票面，未动代码。**能力边界**：缺口节全部取自评审原文与 27 完成记录，推送方**未读** `external_funds_fact_handoff.go` 正文、`handOffFact` 正文、CC `ReceiveFundsFact` 的幂等实现；作者开工第一步先在 `de822820` 复量四条缺口。
