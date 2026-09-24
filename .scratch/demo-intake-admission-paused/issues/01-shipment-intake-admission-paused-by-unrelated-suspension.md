# 01 演示库上隔离写路径的委托提交恒答 `ADMISSION_PAUSED`：种子注释「准入控制为 OPEN」在保守规则下已不成立

Category: bug
Status: in-progress——2026-09-24 通道 4 认领（派单 `task-e7f42ddf` ← 通道 3），隔离 worktree 分支 `mcp4-dia01`，基本笔
Blocked by: 无
地盘：`cmd/parcel-governance-register`（新增阶段评审登记子命令）、pilot-governance 应用层里阶段评审的入口（若 CLI 需要新接）、`scripts/demo-seeds`（`seed.sh`
与 `data/governance/`）。碰 Go，走[并行会话](../../../docs/agents/parallel-sessions.md)那条路。
出处：2026-09-24 通道 3 备真浏览器验收环境时实测（独立库 `idp_verify`，代码钉 `04898af1`）。

## 现象

隔离读、写两个开关都开（`SYN-TENANT-01`），演示种子全量灌入之后，`POST /shipment-requests` 答 `ADMISSION_PAUSED`，
`productionOwnership.suspensionReference` 为 `SYN-GOV-SUS-0002`——种子 `06-suspension-pricing-scope.json`，范围 `SYN-PILOT-SCOPE/pricing@v1`。
委托受理维的范围却是 `SYN-PILOT-SCOPE/shipment-intake@v1`（种子 `07-authority-interval-shipment-intake.json`）。演示库上因此造不出任何委托，
委托查阅、检查器、列表 → 详情往返在演示环境里都没有数据可验。

## 为什么

pilot-governance postgres 适配器 `Suspensions.FindUnresumedSuspension` 头注：答不出覆盖关系时**保守答暂停**
（`domain.AdmissionSuspendedByUnreadableScopeRelation`）；只有 `scope_version_relation` 里登了「互不相干」的边，别处的暂停才不及于所问版本。
种子从没登过这条边，所以任何一笔未恢复的暂停都会拦住委托受理。而 `seed.sh` 在委托受理那一行上方的注释仍写「本行的试点范围版本与上面两笔暂停
各不相同，因此不受它们影响，准入控制为 OPEN」——那是保守规则落地之前的口径。

种子登不上那条边，是因为治理 CLI 今天只开了 `authority-interval` / `suspend` / `resume` 三类；范围版本关系随阶段评审登记
（`application/record_stage_review.go`），CLI 没有入口。

## 裁决（用户 2026-09-24 授权通道 3 自决）

按保守规则的本意修：给治理 CLI 开阶段评审登记口（种子注释里「阶段评审与接管第二批」本就要开的那一类），种子为 `shipment-intake@v1` 与两笔暂停
各自的范围各登一条「互不相干」关系；`seed.sh` 那句注释改成现状。

不取「种子把 0002 也恢复掉」：那会让治理页失去「暂停中」的演示格，而且只是把这次撞上的那一笔挪开——下一笔没恢复的暂停照样拦。

## 完成判据

- 干净库上 `seed.sh` 之后，隔离读写都开，`POST /shipment-requests` 答 `SUBMITTED`；治理页上仍有一笔暂停中。
- CLI 新子命令有用例；`seed.sh --reset` 重灌全程零报错。

## 旁证

验收环境里临时在 `idp_verify` 为 0002 登了一笔恢复，才造出三条委托；那笔恢复只在隔离库里，不进种子。
