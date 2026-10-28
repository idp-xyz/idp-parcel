# 上线前接线（2026-10-28）

用户 2026-10-28 答「现在就接线」：冻结清单「六」那 6 张上线前必须的票进 main 后，有 5 处在生产上还走不到（模型与答法已落、`cmd/` 下无装配或无下游）。本目录一票一缺口，接成生产可用。出处行见 [冻结清单](../launch-line-freeze-2026-10-27.md)「五 → 记录」2026-10-28 那几行。code/43 的「旅程启动不核承接」不在本批（用户只选了接线），仍按冻结清单 R2。

| 票 | 缺口 | 来自 |
|---|---|---|
| [01](./issues/01-internal-compliance-restriction-write-face.md) | 内部合规限制形成与解除没有生产入口 | docs-consistency/code/27、ADR-0334 决定七 |
| [02](./issues/02-customs-case-closure-production-face.md) | CC 关案编排无装配 | code/03 |
| [03](./issues/03-release-gate-verification-production-face.md) | CC 放行门禁核对编排无装配 | code/34 |
| [04](./issues/04-node-collaboration-acceptance-production-face.md) | NO 监管协作承接编排无装配 | code/18、ADR-0333 |
| [05](./issues/05-post-intake-disposition-request-consumers.md) | 收寄后服务处置请求信封无下游消费 | code/37、ADR-0332 |
