# 试点治理读面的准入形状要先裁一道键形——治理登记册没有租户维

Category: chore
Status: resolved——交付 [ADR-0083](../../../docs/adr/0083-pilot-governance-read-face-carries-registry-dimensions-only.md)（MCP-3，2026-08-31）
Blocked by: 无

本票**只出一份 ADR，不写实现代码**。裁完之前票 02 不许动读口。

## 为什么这一问不答就动不了手

`internal/pilotgovernance` 今天没有 `adapters/http` 包，`adapters/postgres` 五个文件全是写口。
要给 `stage-admission` 开读面，照现有形状抄就是照
[ADR-0077](../../../docs/adr/0077-master-data-catalogue-read-follows-the-operations-read-pattern.md)
抄——它把**租户**放在读口方法签名上；而
[ADR-0078](../../../docs/adr/0078-isolated-environment-operations-reads-admit-by-assembly-injection.md)
的隔离读准入注入的也是一个**租户**范围。

问题在于**治理登记册压根没有这一维**。取证于 `65b6cf2`：`migrations/pilot_governance/` 下八张
表，`tenant_id` 出现 **0** 次；主键分别是 `authority_interval(interval_id)`、
`suspension_decision(suspension_id)`、`resumption_decision(suspension_id)`、
`stage_review(objective, candidate_set_id)`、`candidate_version_set(set_id)`、
`channel_execution(execution_id)`、`scope_version_relation(successor_scope, predecessor_scope)`、
`takeover_record`。**这是全仓唯一零 `tenant_id` 的上下文**——其余十个上下文全都有（结算 25 表
78 处、履约 16 表 59 处、追踪异常 30 表 109 处，同 SHA）。

照抄的后果是一个**按租户过滤的读口去查一张没有租户的表**：它编得过、测得过、页面也出得来，
只是答的东西不对。这正是本仓反复点名的那一类——**接错看着像接对**，两种状态的可观察签名完全
相同，没有任何东西会红。所以这一问必须在写读口之前答。

## 要答的三问

1. **治理登记册没有租户维，是设计对，还是漏了？** 倾向（非裁决）：是对的。试点治理治的是
   **试点本身**，不是租户的数据；而本仓开发的是卖给物流企业的产品、开发方不是运营企业，治理
   的主体是产品运营方而非租户。若采纳此说，「加 `tenant_id`」这条路应被明确否决并记明理由，
   免得下一个人重走。
2. **既然不按租户隔离，隔离读准入按什么放行？** ADR-0078 的三条判据（消费所属上下文存储读面、
   零持久化、作用域为运营侧授权结果）里，第三条对治理该读作什么？要给出一个可落地的替代
   作用域，不能留白——留白等于下一个实现者自己发明一个。
3. **页面按什么隔离？** `stage-admission` 挂在租户管理台的「试点治理」分区里。若该页的内容不
   按租户隔离，它出现在租户管理台是否正当？三条可选：仍留在管理台但明示其作用域不是本租户；
   移出租户管理台另立产品运营台；或按试点范围（`scope`）隔离而非租户。各自的连带要写清。

## 交付

一份新 ADR（编号取 `docs/adr/` 现有最大号加一），Status 按本仓惯例；`docs/adr/README.md` 补
目录项——**只改自己那一行，不动邻行**（该文件是索引类文件，每新增一份 ADR 必然要碰，防的是
盲覆盖）。若裁决结论是「移出租户管理台」，则票 02 范围随之改写，在本票 Comments 里写明。

## 不在本票内

不写任何 Go 代码；不动 `apps/admin-web`；不碰 `seed.sh`。阶段评审与接管两类属第二批（票
`syn-wall-door-audit/12` 裁定，`cmd/parcel-governance-register/main.go` 包注释自证「阶段评审与
接管第二批，不在本入口」），本票不重开那个决定。

## 参照

ADR-0077、ADR-0078、ADR-0017；`docs/design/pn-08-end-to-end-pilot-and-stage-admission-development-handoff.md`；
`.scratch/syn-wall-door-audit/issues/12-governance-registration-has-no-process-entry.md`。

## Comments

- 2026-08-31 MCP-3：三问出裁，交付 ADR-0083。①无租户维是设计，「加 `tenant_id`」明文否决；②隔离读放行沿用同一开关同一装配点，治理行注入（作用域引用，页大小）不带租户，钉窄只覆盖 `pilot_governance`，ADR-0078 Decision 四原文不动；③页面留在管理台「试点治理」分区，明示产品实例级作用域——受众按 syn-wall-door-audit 票 12 的登记主体裁定作答，「移出管理台」否决（复活 ADR-0019 排除的跨租户运维后台），票 02 范围不改写。08-26 旧裁（admin-web-page-wiring-frontier/03）的三条重开判据在 ADR Context 逐条对表，该票已补取代注记。
