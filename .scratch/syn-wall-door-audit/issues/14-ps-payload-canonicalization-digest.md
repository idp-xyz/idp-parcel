# PS 载荷规范化摘要未实现，`PayloadDigest` 仍只是非空字符串包装

Category: enhancement
Status: ready-for-agent

来源：[票 01](./01-access-channel-registry-and-first-real-intake.md) 的前置一，按
[ADR-0072](../../../docs/adr/0072-access-channel-capability-is-shared-and-registry-shape-awaits-channel-evidence.md)
Decision 第 3 条与 MCP-6 建议（票 01 Comments，取证于 `0ec62ea`）拆出独立成票；
2026-08-21 MCP-3 受用户委托裁断后立票。

## 缺什么

`internal/parcelshipment/domain` 的 `PayloadDigest` 只是 `requiredValue` 的非空字符串包装，
全仓没有任何函数按规范化业务内容产出它；`SubmissionIntake` 注释自证「`parcel-shipment`
侧的规范化形状尚未实现」。ADR-0055 第五条把它列为真渠道 Intake 的两前置之一。

## 已定死的边界（PS CONTEXT，机制半边）

- `occurredAt` / `receivedAt` 属来源信封元数据，**不进**摘要；
- `requestEffectiveAt` 及其缺失/显式存在状态**进**摘要。

## 落地约束

- 按 [ADR-0014](../../../docs/adr/0014-versioned-canonicalization-shape-for-content-digest.md)：
  规范化形状必须带版本号，版本号进摘要输入；形状变更走新版本号，不改旧形状的产出。
- 不碰 `PAR-INT-01` 的任何一半：渠道、凭据一概不涉。产出摘要的函数是纯领域机制件，
  由测试与未来的真渠道 Intake 共用；本票不接线、不动 `assembleBusinessEndpoints`，
  不撞 ADR-0055「未配置即拒」的语义。
- 主责上下文 `parcel-shipment`；领域包不依赖 HTTP/`pgx`；验测按包路径全量跑。

## 参照

ADR-0072、ADR-0014、ADR-0055（Decision 第五条两前置）；票 01 Comments（MCP-6 取证）。

## Comments

- 2026-08-21 MCP-3：随 ADR-0072 立票。另一个前置「准入范围装配」不在本票——它属
  `PAR-GOV-03..07` 与审计票 02 衍生族的地盘（见[票 11](./11-scope-version-coverage-relation-is-unregistrable.md)、[票 13](./13-production-ownership-bridge-has-no-assembly-point.md)）。
