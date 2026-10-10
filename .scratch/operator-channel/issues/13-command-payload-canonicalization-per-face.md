# 13 各命令口的载荷规范化形状：ADR-0055 决定五第一项逐口解

Category: enhancement
Status: 完工，待评审与重放——2026-10-10 11:43 通道 3（分支 `mcp3-oc13`，代码 tip `1176721c`，基 `1121ba61`，工作树 `/home/tops/workspace/idp-parcel-mcp3-oc13`）。此前：in-progress——2026-10-10 11:21 通道 3 认领；ready-for-agent——2026-09-24 随 ADR-0149 立（用户授权通道 4 自决）；按上下文拆笔
Blocked by: 无
父票：[psb/15](../../product-strategy-boundary/issues/15-operator-channel-per-adr-0100.md) 丙轨实施
地盘：各上下文 `domain` 里命令载荷的规范化形状与摘要（NO、TF、CC、SA 各一笔），`adapters/http` 的译装不改答复。
出处：[ADR-0149](../../../docs/adr/0149-business-command-faces-split-into-frontline-operator-and-integration-client-families.md) 决定四第一条；先例 `PSC-1`（parcel-shipment）、`PCC-1`（party-commercial）。

## 做什么

1. 逐口列出作业事实与外部结果命令口，各自定一版规范化形状、带形状版本前缀，摘要进信封。
2. 同身份同摘要答重放、同身份异摘要答内容冲突；已有形状的口（如提交口）只核对不重做。

## 完成判据

- 每个口有形状版本与往返用例；未定形状的口在清单上标明，真渠道不对它开。

## 完成记录（通道 3，分支 `mcp3-oc13`；待非作者评审与重放）

形状都在各上下文 `domain`。文档是 JSON，版本写在文档里，摘要是 `版本:sha256`。字段集沿用各口原先的内容判据；集合排序，使提交顺序不构成另一份内容。已保存的旧摘要不回算（ADR-0014）。`adapters/http` 未改，答复未改。续办哈希（`CONT-`）与回执短指纹未改。`PSC-1`、`PCC-1` 只核对、未重做。

| 笔 | 提交 | 形状 |
|---|---|---|
| 认领 | `5fbf3340` | — |
| NO | `0a1c8a08` | `NOC-1` |
| TF | `426ec2ed` | `TFC-1` |
| CC | `9465ff5b` | `CCC-1` |
| SA | `1176721c` | `SAC-1` |

### 已定形

- **NOC-1**（九个命令口，各有往返用例）：承接 `ACCEPT_COLLABORATION`、执行 `RECORD_EXECUTION`、集运 `OPEN_UNIT` / `ADD_MEMBER` / `REMOVE_MEMBER` / `SEAL_UNIT` / `UNSEAL_UNIT` / `CLOSE_UNIT`、收寄 `RECEIVE_DELIVERED_UNIT`。
- **TFC-1**：交接首登与更正、有效交付首登、场外揽收首登与更正（同一内容判据）、场外揽收执行、委托、订舱、订舱应答、替代旅程、监管承接、建立班期、建立运力池。
- **CCC-1**：外部结果（放行三件可空）、申报提交与原案内更正（同一内容判据）、税费付款核对、处置执行核对的事实集。
- **SAC-1**：供应商预计成本、发布对账单、纳入迟到费用、纳入调整（与迟到费用分面）、开立争议、分摊与重分摊、派生与重派生、审核账单行、供应商贷项、索赔金额、应收、认可应收、调整索赔、接收供应商账单、评估垫付、形成追偿、调整追偿、采用资金事实、更正资金事实、映射资金、核销。核销的分配行保留提交顺序，与原先的内容判据一致。

### 未定形状

本票没有为下列命令发明字段，也没有新开 HTTP 路由。端点表里已有的行是前票装配，本票不改译装，所以没有把它们从真渠道上拆下来。

- **TF**：`CancelCommission`、`ReserveCapacity`、`ReleaseCapacity`、`ConsumeCapacity`、`CorrectDeliveryProof`、`RegisterEffectiveTimeRule`、`RegisterExternalCarrierCredential`、`ChangeCredentialApplicability`、`FormActualCarrierJudgment`、`RegisterMasterDocument`、`ReviseMasterDocument`、`RegisterFailedAttemptCharge`、`FormLoadAssignment`、`TriggerDeliveryDispatch`、`JudgeCarrierFirstEffectivePickup`、`RecordMovementFact`、`JudgeEffectiveTime`、`RecordDeliveryAttempt`、`OpenDispatchTask`、`CloseFulfillmentSegment`、`EndFulfillmentParticipation`。
- **CC**：`RegisterCandidatePort`、`RegisterDeclarationPath`、`ReceiveManifest`、`ReviseManifest`、`RegisterCaseRequirementRule`、`JudgeCredentialApplicability`、`RegisterCredential`、`EstablishCase`、`RegisterReadiness`、`RevokeReadiness`、`GrantSubmissionAuthority`、`RevokeSubmissionAuthority`、`RegisterInterpretationRule`、`RegisterObligationCatalog`、`RegisterObligationItem`、`RegisterGateCatalog`、`RegisterGateFinding`、`RegisterDutyPaymentGateRule`、`RegisterPayerRequirement`、`FormDutyCollaboration`、`ReceiveExternalFundsFact`、`VerifyReleaseGate`、`RecordCredentialGate`、`EstablishRestriction`、`ReleaseRestriction`、`FormFollowUpTarget`、`ProposeReplacement`、`RecordReplacementEffect`、`CloseCustomsCase`、`RederiveDutyVerifications`（重派生写出的核对版本仍走 `VERIFY_DUTY_PAYMENT` 那一形）。
- **SA**：`TriggerSellEvaluation`、`RecordChargeAdjustment`、`VoidStatement`、`ResolveDispute`、`RequestBuyEvaluation`、`ApportionCosts`、`ReleasePreAcceptanceControl`、`ConfirmCharge`、`RegisterSettlementAccount`、`ApplyPreAcceptanceControl`、`ReverseApplication`、`AdoptDutyPaymentVerification`、`FormSellCustomerCharge`、`JudgeChargeAttribution`。

### 已有指纹、本票不改写

- CC `FindingsDigest` / `GateVersionDigest` / `CredentialGateDigest`：注释写明改词形会让已入库门禁版本换指纹，要改另起一票。它们仍是未带形状版本的旧指纹。
- SA 领域 `freezeDigest`、`exposureDigest`：冻结与敞口请求的内部指纹，不是上列命令口。

### 作者自验（钉 `1176721c`）

`go build ./...` 与 `go vet ./...` 退 0。`go test -count=1 -p 1` 带 `IDP_PARCEL_POSTGRES_DSN=postgres://parcel:parcel@127.0.0.1:55432/postgres?sslmode=disable`，跑 NO / TF / CC / SA 全包、`internal/architecture`、反向依赖（`parcel-api`、`parcel-customs-register`、`parcel-dispatch`、`parcel-frontline-import`、`parcel-settlement-register`，以及 parcel-pricing / parcel-shipment / visibility-exception 里指向这四个上下文的适配器）——退 0。不自评。

### 判断项（交评审看）

1. 枚举进摘要用领域 `String()` 词形（NO 的收寄主张没有领域 `String()`，仍用数字）。旧的 `%d` 拼接与新文档不可比，靠版本前缀分开，不回算存量。
2. 未定形状且端点表已有行的口，本票没有关闭。关闭会改已放行口的答复，超出「译装不改答复」。
3. 结算侧应用包装在编码失败时 panic。这些文档只有字符串、整数和它们的切片，编码失败说明形状写坏了；空摘要会把两份不同内容读成重放。
