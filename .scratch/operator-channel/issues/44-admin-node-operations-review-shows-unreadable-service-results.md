# 44 管理台节点作业复核页把收寄的「服务结果读不动」显示成单独一格，不再与「无标记」同形

Category: enhancement
Status: ready-for-agent——2026-10-18 会话 `idp-parcel-9b` 立（票 26 序 2 评审遗留；用户授权代决）
Blocked by: operator-channel/43 进 main
父票：operator-channel/43。
出处：簇 C1 终裁 C1-Q2，可见性那一条。
地盘：
- apps/admin-web/src/pages/operations/NodeOperationsReviewPage.tsx
- apps/admin-web/src/pages/operations/records-api.ts
- apps/admin-web/src/pages/operations/（新的纯逻辑映射模块与它的 node:test 测试）

代决依据：用户 2026-10-16 授权会话 `idp-parcel-9b`「你作为业务和系统专家你来自决」；裁定经隔离子代理研判、对抗核验与合成（研判轮 review26-followups-decide）。

前端票：照 docs/agents/workflow.md「前端切片」在 main 上直接做。

## 为什么

operator-channel/29、C1-T2 落地后，节点作业查阅答复里，INTAKE_FORMED 行会带上服务结果核对态。

但复核页收寄区的「服务标记」一列，今天只做一件事：把 serviceMarkers 用顿号拼起来。结果是「读不动」「已查、无标记」「待识别，不适用」三种情况都显示成空白。今天没有方向性门禁，运营在这页上看到空白，就会当成「没有取消、没有终局」。

## 裁定

- 「服务标记」一列按三态出文字，文字各不相同：
  - 已查、有标记：照旧用顿号拼接；
  - 已查、无标记：写明「无取消与终局」；
  - 读不动：写明服务结果读不动，标记缺席不代表没有取消或终局。
- 待识别行写「不适用」。
- 「读不动」的文字不得与空白或「无」同形。具体措辞由作者定。

## 做什么

- records-api.ts：ReceptionRecord 类型加核对态字段，可选。
- 把 NodeOperationsReviewPage 里 rowsOf 收寄区的这一段映射，抽成纯函数，并加 node:test 测试。

## 不做

- 不改 Go 代码，不改其他区。

## 完成判据

- [ ] 三道门退 0，用 Node 22：tsc -b --noEmit、node scripts/run-tests.mjs、vite build。
- [ ] node:test 覆盖四格：已查有标记、已查无标记、读不动、待识别。
- [ ] 票面的完成记录里，判据逐条标 ✅ / ◑ / ❌。
