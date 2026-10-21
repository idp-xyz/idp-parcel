# 69 业务时间早于委托接受的复核触发撞离场门，整封信封响亮失败

Category: bug
Status: needs-info——R2（上线后尽快；若某租户首发动线里「先收后受理」加触发晚处理是常态则升 R1）。需要一份新 ADR 定判法（见「要裁的」），协调方已给裁向。2026-10-21 协调方据 code/60 越权风险点 1 与其隔离评审建议立。
Blocked by: 无（裁向已给；实现与 ADR 由同一张票落）
出处：code/60 完成记录越权风险点 1；code/60 隔离评审 risk1_recommendation。

## 现象

code/60 之后初始路由从委托接受时刻起生效。触发的业务时间早于接受、信封却在初始路由形成之后才处理（积压、离线补登、来源时间写早了），而复核又判出失效或替代时，离场门按「离场不早于生效」拒，整封信封响亮失败，重投到 ABANDONED，无绕法。

接受之前就处理到的收寄（PS 答 `REQUEST_NOT_ACCEPTED`）与交接（NR 按 ADR-0215 入账）不撞；只有「早于接受、晚于初始路由形成才处理」这一格撞。

## 证据

- `internal/networkrouting/application/create_initial_route.go`：`planRecord` 的生效取 `RouteHandoff.AcceptedAt()`（code/60）。
- `internal/networkrouting/application/reassess_route.go`：失效线 `Lapse`、改善替代 `Supersede` 都以触发业务时间离场（ADR-0272）；领域 `leaveCurrent` 头注「离场必须带依据与不早于生效的时刻」。

## 三类

机制。

## 要裁的（协调方裁向，2026-10-21）

取「入账为`未判`、不判」，否决「离场夹到生效时刻」（ADR-0272 已否决同形候选，`AT-NR-064` 钉住失效时刻取触发业务时间）与「PS 采用时拒」（所有权错、且盖不到 TF 交接那一路）。依据：ADR-0235 决定四（早于当前位置的触发入账不判，「业务时间不会变，重投永远推不动」）、ADR-0174、ADR-0250 先例。

落法：新立未判原因（例如「触发早于委托接受」，与 `TRIGGER_PRECEDES_CURRENT_POSITION` 分开）；下限取这一判断键的接受时刻（不取 `plan.EffectiveFrom()`，免得「原先无当前有效路由」那条线形成一份早于接受就生效的首个计划）；接受时刻由收寄与交接两路适配器从 PS 委托聚合的 `AcceptanceBaseline().FixedAt()` 带进来。新 ADR 写明对 ADR-0235 决定一（未判原因封闭集）与决定四的作用，扩成也算部分停用，照 `docs/adr/README.md` 规则挂指针；越权风险点记「那次触发的硬约束检查被跳过，要等下一次触发再判」。

## 规模

M：一份 ADR、一个未判原因、两路适配器带接受时刻、编排在比较当前位置前先比接受时刻、编排与派发真库用例。
