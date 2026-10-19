# 64 路由计划带交付段，运输准备跳过它

Category: enhancement
Status: ready-for-agent——2026-10-19 Claude Code 会话立（用户「你作为业务和系统专家自决……解决所有的问题」授权）。代决见 [handover-dispatch-plan](../handover-dispatch-plan.md)。
Blocked by: 无（种子令牌）
地盘：NR `domain/initial_route_plan.go`（段种类与交付终点）、`application/calendar_projection.go`、`application/catalog_initial_route_evidence.go`（带出所用服务区域版本）、`application/reassess_route.go`（已执行前缀只取节点间段）、`ports/catalog_registration.go`（服务日历节点行派送时长）、`adapters/registrationjson/translate.go`、`adapters/postgres/network_catalog*.go`、`adapters/postgres/initial_route.go`（段 json）、NR 迁移（认领时现取）、`adapters/http/query_route_plans.go`；TF `adapters/networkrouting/planned_leg_source.go`（交付段不形成准备腿）；种子 `scripts/demo-seeds/data/network/` 加 SIN-LM 日历新版本（带派送时长）+ `seed.sh` 一行；文档 ADR-0217、NR CONTEXT「路由形成与选择」一句、GLOSSARY「计划履约段」一行、UC-NR-001、`PAR-NET-14`、演示脚本（末端交接声明交付段）。
出处：[06](./06-delivery-dispatch-has-no-in-process-trigger-and-no-recipient-leg.md)；方案「复核」06「升级」，「代决」C11，「需要的 ADR」ADR-0217。

## 做什么

- NR 计划在交付节点之后带一个交付段；终点、窗口、未登记时的答法照方案 C11。
- 段带种类；已执行前缀与 TF 运输准备只取节点间段，避免准备消费者把交付段判为译不出。
- 演示种子给 SIN-LM 登派送时长（SYN 值，证据 `S`）。
- 文档：起草 ADR-0217。
- 进 main 的令牌次序见方案「波次与令牌」；种子落地后把 SHA 通知 idp-parcel-9b。

## 完成判据

- 登了派送时长：计划多出交付段，段种类为交付，窗口 = 离开交付节点的时刻 + 时长。
- 没登：计划段链止于交付节点，与今天相同。
- TF 窗口缝：交付段引用答 `RESOLVED`；不存在的序位答 `MISSING`。
- TF 运输准备只备节点间段，不成毒丸。
- 复核的已执行前缀不变；旧计划行读回不变。
- 真库测试。
- 演示：提交一单 → `/route-plans?registry=initial-route` 末段为交付段；末端交接声明该段引用 → 派送任务窗口取交付段窗口（配合 [60](./60-delivery-dispatch-is-triggered-in-process-from-the-handover-envelope.md)）。

## 规模

L
