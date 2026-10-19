# 31 关务动作准入判断与节点关务区域

Category: enhancement
Status: ready-for-agent——2026-10-19 Claude Code 会话立（用户「你作为业务和系统专家自决……解决所有的问题」授权）。代决见 [tf-chain-plan](../tf-chain-plan.md)。
Blocked by: 无
地盘：CC 新 `application/judge_action_admission.go`；ports 新文件（对象→范围/程序、节点区域）；`domain` 新 `node_customs_zone.go`；postgres 新册 + 迁移 CC 新号；CC http 登记口；`cmd/parcel-customs-register` 新 kind；`cmd/parcel-api` 装配 + `endpoints.go` 行；种子 customs 四个节点区域；ADR-0206、CC CONTEXT、UC-CC、CONTEXT-MAP。
出处：[05](./05-movement-facts-are-unlinked-and-departure-gate-is-self-reported.md)；方案「05 移动事实与出发门禁」D05-2、D05-3、D05-4、D05-5，「跨票通则」X3，「需要的 ADR」ADR-0206，「已不成立或已修」05。

## 做什么

- CC 自有「动作准入判断」查询，请求驱动；输入与三源折叠照方案 D05-3 与「需要的 ADR」ADR-0206。
- 适用动作由 CC 按节点关务区域定，未登记节点的答法照方案 D05-5；不采纳「每次出发都问两种」。
- 新登记册「节点关务区域」，值属租户取值，`parcel-customs-register` 加 kind；演示种子登 SYN 节点区域。
- 对象到范围与程序由 CC 解析，解析不到答未决；演示停在 scope 一级（方案 D05-4）。
- 判断记录可重放，不入队（方案 X3）；现有 `VerifyReleaseGate` 与 `gate-verification.recorded` 不变。
- 起草 ADR-0206；CC CONTEXT、UC-CC 准入判断、CONTEXT-MAP 的 TF→CC 拉取边照方案「随票同步的文档」。
- 进 main 的令牌次序与波次见方案「并行车道」。

## 完成判据

- 按区域定适用动作。
- 三源折叠成封闭答复，门禁满足不答放行。
- 节点未登记答 `未配置`。
- 判断记录可重放、不入队。
- 真库测试覆盖放行 / 阻断 / 不受门禁 / 未决 / 未配置。
- 演示：SHA→SZX 只适用 `LOADING_DEPARTURE`；SZX→SIN 另加 `CROSS_CUSTOMS_MOVEMENT`。

## 规模

L
