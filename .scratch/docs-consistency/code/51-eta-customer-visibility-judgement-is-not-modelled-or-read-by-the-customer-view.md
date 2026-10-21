# 51 VE 的 ETA 客户可见性判断（AT-VE-187）没有建模，客户视图的 ETA 一维只能答待确认

Category: enhancement
Status: needs-triage——建议 R2（上线后尽快）：不答错，但缺一整块机制。code/06 之后，客户视图的 ETA 一维在策略准许展示时也一律答待确认：本上下文没有判断对象可读，租户把 ETA 登成展示也看不到任何 ETA。2026-10-21 文档自洽 code/05、06 隔离评审立（用户「你作为业务和系统专家自决」授权），分支 claude-code05。分级由协调方定稿。
Blocked by: [docs-consistency/code/07](./07-eta-store-overwrites-history-and-mixes-sources.md)（ETA 存储先改成版本表、键加来源维；判断要引用候选 ETA 的版本，读法在那之后才稳）
出处：code/06 完成记录越权风险点 1；code/06 隔离评审。

## 现象

UC-VE-003 要求「ETA 客户可见性判断」：针对客户账户、服务产品、合同、预测对象与目标里程碑范围，引用候选内部 ETA 版本，保存质量评价、门槛版本、规则版本、结论（客户可见 ETA / ETA 不可见 / ETA 可见性待判断）、理由、判断时点、版本与替代关系。代码里只有内部预测 `ETAPrediction`，没有这个判断对象，也没有形成它的编排。客户视图因此无从读，`domain.ETADimensionWithoutVisibilityJudgement` 在策略准许展示时只能答待确认。

## 证据

- 文档：UC-VE-003 结果语义「ETA 客户可见性判断」一行与主流程第 6、8 步；`AT-VE-187`、`AT-VE-170`。UC-VE-008 主流程第 4 步「读取 `UC-VE-003` 的客户可见性判断」；`AT-VE-158`「只有内部 ETA 或质量未达对客门槛 → 不展示 ETA，也不使用计划或承诺时间填充」。
- 代码：`internal/visibilityexception/domain/eta_visibility_gap.go` 只有 `ETAPrediction` 与 `VisibilityGap`；`application.FormETAHandler` 只形成预测与缺口；`domain.ETADimensionWithoutVisibilityJudgement` 头注写明判断落地时改为收那份判断。
- AT 盘点：`AT-VE-158`、`AT-VE-170`、`AT-VE-187` 都在 ④。

## 三类

- 机制：判断对象、它的版本与替代关系、按（客户账户，包裹，目标里程碑）取当前判断的读口，以及客户视图 ETA 一维按判断结论作答。
- 产品策略：无判断时答待确认还是不展示（code/06 代决答待确认，待 VE owner 裁，见 code/06 完成记录）。
- 租户取值：质量门槛（`PAR-VIS-03`）、合同与客户服务规则（`PAR-COM-07`）照参数登记册，未登记答未配置，结论落「ETA 可见性待判断」。

## 规模

L：领域对象与版本表迁移、形成编排（由 ETA 新版本、门槛或规则版本变化触发）、读口、`ETADimensionWithoutVisibilityJudgement` 改为收判断、用例钉住 `AT-VE-187`、`AT-VE-170`、`AT-VE-158`。

## 最小的诚实修法

1. 建判断对象，结论封闭三格；形成编排引用候选 ETA 版本（code/07 之后的版本表）。
2. 客户视图派生读当前判断：客户可见 ETA → 照策略；ETA 不可见 → 不展示（钉住 `AT-VE-158`）；可见性待判断或读不到 → 待确认。
3. ETA 判断换版本时触发客户视图重派生（CONTEXT「ETA 新版本……必须重新派生客户视图」）。

## 要裁的

- 判断按哪几维取当前版：客户视图只有一维 ETA，而判断按目标里程碑范围形成，要裁视图取哪一个里程碑的判断（例如下一个未达里程碑）。
- 无判断时答待确认还是不展示，随 code/06 的代决由 VE owner 一并裁。
