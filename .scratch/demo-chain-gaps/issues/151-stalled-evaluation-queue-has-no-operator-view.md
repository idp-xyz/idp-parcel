# 151 停着的评价请求没有查阅面，运营续办只有受控进程入口

Category: enhancement
Status: ready-for-human——2026-10-27 冻结到首个租户上线之后（[上线线与冻结清单](../../launch-line-freeze-2026-10-27.md)，分级 R3）：不挡上线，代理不认领；解冻看首个租户的真实使用或用户点名。此前 needs-triage——2026-10-21 Claude Code 会话立（用户「你作为业务和系统专家自决……解决所有的问题」授权），出自票 [14](./14-buy-request-stuck-after-an-unadoptable-evaluation.md) 实现时的取舍。
Blocked by: 无
出处：[ADR-0229](../../../docs/adr/0229-a-stalled-evaluation-request-resumes-as-a-new-generation-on-parameter-registration.md) 越权风险点 8。

## 现象

票 14 之后，停下的评价请求与停在发起之前的那一拍都在册上带着停因，补登参数时自动续办。可是「这个租户眼下停着哪些、各停在哪一格」没有读口，管理台也看不到；结算政策没采用、包裹没有商业解析、计价输入不可得这几格只能运营续办，而续办口是 `parcel-settlement-register evaluation-resume`，要人在数据库网络里手跑、手写 JSON。运营要知道该续办哪一件，今天只能查库。

## 三类

机制（查阅读口、运营决定入口）。规模 M。

## 最小的诚实修法

结算开一只按租户（可按停因、方向、计价范围筛）的续办队列查阅读口，复用 `StalledEvaluationQueue` 的口径，挂结算运营查阅页；运营续办做成一种运营决定（新决定种类，按 ADR-0151），端点转调同一只 `OperatorResumeEvaluationHandler`，续办引用取决定标识。CLI 口保留作受控批量口。

## 要裁的

续办要不要四眼；查阅页要不要连同停下那一份的评价问题项一起读（跨上下文读计价）。归 SA owner。

## 既有票

none
