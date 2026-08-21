# PS 载荷规范化摘要未实现，`PayloadDigest` 仍只是非空字符串包装

Category: enhancement
Status: resolved

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

- 2026-08-24 MCP-3（死现场抢救合入并收口，转 resolved）：实现出自 `t14-payload-digest`
  孤儿分支上的 `321c841`（2026-08-21 作，工作树 mtime 停在当日 23:07，其后未再响应；
  该笔自己没带票面回写，本条即为收口）。本笔按票 13/11/04 先例单摘该笔到 main
  （cherry-pick 零冲突；其间 main 的提交均不触 `parcelshipment/domain` 与棘轮基线，
  连续性成立），逐行复核后采用：
  1. **机制件**：`CanonicalizeSubmissionPayload` 按 ADR-0014 版本化形状 `PSC-1` 产出
     自带版本前缀（`PSC-1:<sha256>`）的 `PayloadDigest`——版本既进摘要输入又随摘要串
     同行保存，零迁移满足「已保存摘要必须携带产生它的规范化版本」。
  2. **边界照单**：`occurredAt`/`receivedAt` 结构性排除（文档形状里没有它们的位置）；
     `requestEffectiveAt` 值与声明态两格分开进摘要；成员+测量画像沿用提交管线同款裁决
     （空集/重复/指集合外成员即拒）；寄收件范围与服务要求以规范化条目承载（排序、
     去重、UTF-8 编码纪律在此，**词表属 PAR-INT-01 实例半边，一个渠道字段名都不认**）。
  3. **不接线**：纯领域函数，未动 `assembleBusinessEndpoints` 与任何装配点；
     `adapters/http` 仅修订 SubmissionIntake 注释里「规范化形状尚未实现」的过时半句。
     两个新工厂（摘要函数与版本报出口）按门禁纪律入棘轮基线并记理由。
  4. **验证由本笔重做**（原分支验证断言随死会话作废）：隔离树 gofmt 判输出干净、
     build/vet 零信号、全仓 `go test -p 1 -count=1 ./...` 设 DSN 零 FAIL；
     真摘要驱动 `ClassifySourceSubmission` 重放/冲突分类的用例（信封时间变化仍是重放）
     随全量真跑。结果记于本笔提交信。
