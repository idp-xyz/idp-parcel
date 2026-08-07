# 国际小包计费平台 MVP 研发实施设计与任务拆解

**版本：** V1.0.1 终审版  
**状态：** 可进入 Sprint 0  
**计划形式：** 16 个 Epic、82 个可验收 Task、12 个 Sprint 退出门禁  
**注意：** Story Point 为相对复杂度，不等于固定人天；实际日历需按团队人数、熟练度和并行能力校准。

## 0. 终审结论

前述架构、领域、计算、API、Runtime 与 PostgreSQL 基线已经足以进入研发。MVP 的正确顺序不是“先做后台页面”，而是：确定性内核 → 版本与 Artifact → Rating 热路径 → API → Quote/Async/Governance → 安全性能上线。

首期采用模块化单体，Go 参考实现，PostgreSQL 权威存储，对象存储保存不可变 Artifact；Redis 和外部 Broker 均为可选增强。所有功能必须通过上游 Golden Cases 和 Contract Test，而不能以“人工点一下看起来正确”验收。

## 1. MVP 范围

### MVP-A：可可靠算出一票价格

必须包括：

- Tenant、Organization、Carrier、ServiceProduct；
- CustomerContract；
- BUY/SELL PricingPlan；
- PricingRelease 与 CompiledPlan；
- Snapshot、MeasurementFact、CarrierAssessmentFact；
- Geometry、Weight、PER_PACKAGE；
- WEIGHT_ZONE、COUNTRY_WEIGHT、FIRST_CONTINUE，以及基础 11 类价表实现；
- 固定、单位、百分比、lookup、minimum、cap、formula 等 Charge；
- Fuel、FX、BUY/SELL 转换；
- RatingEvaluation 与解释；
- Eligibility、Rating、Comparison API；
- PostgreSQL Migration、RLS、幂等、Outbox；
- UPS Ground 基线导入；
- 136 Golden Cases。

### MVP-B 基础

纳入同一实施计划后段：Quote、Worker、Batch、Replay、价卡治理和发布。它们可在 MVP-A 热路径稳定后启用。

### 明确排除

- ChargeAssessment；
- FinancialCharging；
- Settlement；
- CarrierReconciliation；
- 任意脚本规则；
- 通用 BPMN；
- 自动识别歧义价卡并直接发布；
- 复杂返利和周期累积。

## 2. 开工门禁

Sprint 0 结束必须全部满足：

1. Go 工程结构与模块依赖检查；
2. 锁定 PostgreSQL 版本并执行全部 Migration；
3. OpenAPI 生成代码和契约测试骨架；
4. 136 Golden Cases 可加载；
5. Decimal 库通过精度试验；
6. CI 包含 test、race、lint、migration、contract；
7. Artifact Schema 和示例通过；
8. 所有依赖工件版本可追踪。

任何一项未通过，不进入业务开发。

## 3. 工程组织

```text
/cmd/rating-service
/internal/shared
/internal/domain
/internal/engine
/internal/application
/internal/adapters
/internal/generated
/migrations
/test/golden
/test/contract
/test/integration
/test/performance
/docs
```

Domain 和 Engine 不依赖 Adapter/Infrastructure；Engine 不访问数据库、网络和当前时间。API、Worker、Replay 共用同一 Runtime。

## 4. Epic 总览

| Epic | 名称 | 目标 Sprint | Task 数 |
|---|---|---|---:|
| E01 | 工程与质量基线 | S0 | 7 |
| E02 | Shared Kernel 与确定性 | S1 | 6 |
| E03 | 数据库与 Repository | S1, S2 | 7 |
| E04 | Product、Pricing 与 Contract Domain | S2 | 5 |
| E05 | Snapshot 与 Fact Selection | S3 | 4 |
| E06 | Compiled Plan 与发布 | S3, S4 | 6 |
| E07 | Geometry Engine | S4 | 4 |
| E08 | Weight Engine | S4, S5 | 5 |
| E09 | Aggregation 与 Rate Lookup | S5 | 5 |
| E10 | Charge、Fuel、Currency 与商业转换 | S6, S7 | 6 |
| E11 | Stage Executor 与 RatingApplication | S7 | 4 |
| E12 | API、Eligibility 与 Comparison | S8 | 5 |
| E13 | Quote Management | S9 | 4 |
| E14 | Async、Batch 与 Replay | S9, S10 | 4 |
| E15 | 治理、导入与运营能力 | S10 | 4 |
| E16 | 可观测性、安全、性能与上线 | S11 | 6 |

总相对复杂度：**791 Story Points**。该数字只用于范围和团队容量测算，不构成完成时间承诺。

## 5. Sprint 路线

### S0：工程与 CI 可重复

关联 Epic：E01

退出条件：
- Go build/test/lint
- PostgreSQL CI
- Golden harness 可加载136案例
- OpenAPI generated

### S1：确定性内核和数据库基础

关联 Epic：E02, E03

退出条件：
- Decimal/Unit/Hash
- Migration/RLS smoke

### S2：产品、定价、合同领域骨架

关联 Epic：E03, E04

退出条件：
- 版本/Release Repository
- BUY/SELL 领域边界

### S3：Snapshot、Fact、Artifact 编译骨架

关联 Epic：E05, E06

退出条件：
- Fact selector
- Artifact schema/hash

### S4：Geometry、Weight 与 Release Gate

关联 Epic：E06, E07, E08

退出条件：
- 几何/重量案例通过
- 首个可激活测试 Release

### S5：Aggregation 和 Rate Lookup

关联 Epic：E08, E09

退出条件：
- 五种聚合
- 11价表family

### S6：Charge、Fuel 与 Currency

关联 Epic：E10

退出条件：
- Scope/Basis/Method/Effect
- Fuel/FX

### S7：商业转换和完整 Runtime

关联 Epic：E10, E11

退出条件：
- 26阶段
- BUY/SELL
- Evaluation draft

### S8：Rating API 和 Comparison

关联 Epic：E12

退出条件：
- 核心 API contract
- 成本投影

### S9：Quote、Worker 和 Replay

关联 Epic：E13, E14

退出条件：
- Quote状态机
- 异步恢复
- Replay

### S10：Batch、价卡导入和发布

关联 Epic：E14, E15

退出条件：
- 1000批量
- UPS导入
- 发布治理

### S11：安全、性能、恢复和上线

关联 Epic：E16

退出条件：
- 全部生产门禁
- 试运行批准

## 6. 关键研发依赖链

```text
E01 工程基线
  → E02 Shared Kernel
  → E04 领域骨架
  → E05 Snapshot/Fact
  → E06 Compiled Plan
  → E07/E08/E09 计算内核
  → E10 Charge/Fuel/FX/BUY-SELL
  → E11 26阶段 Runtime
  → E12 API
  → E13/E14 Quote/Async/Replay
  → E15 Governance
  → E16 生产验收
```

数据库 E03 从 S1 开始并行，但不能先于领域语义自行增加业务字段。

## 7. Definition of Ready

Task 开始前必须具备：

- 明确上游章节；
- 输入/输出类型；
- 错误码；
- 前置依赖；
- 对应 Golden Case 或说明为何不适用；
- API/表影响；
- 可自动化验收条件；
- 安全与租户影响。

## 8. Definition of Done

每个 Task 必须：

1. 代码、测试和文档同一变更；
2. 无 binary float 进入业务；
3. `go test -race ./...`；
4. 相关 Golden Cases；
5. Contract/Integration Test；
6. 无 tenant 或 BUY 泄漏；
7. 有错误码和可观测性；
8. Migration 向后兼容；
9. Reviewer 能从结果追溯到规则、版本和证据。

## 9. 分支与发布策略

- 主干开发，短生命周期分支；
- 所有 PR 必须通过必需门禁；
- API 和 DB 使用兼容扩展；
- Pricing 通过新 Release 激活，不修改旧版本；
- Feature Flag 只能控制实现路由、缓存和采样，不能无版本改变公式；
- 生产镜像带 commit、SBOM 和 provenance。

## 10. 测试金字塔

- Unit：Decimal、Unit、Geometry、Weight、Lookup、Charge、FX；
- Property/Fuzz：区间唯一、分摊守恒、排序确定性、Artifact decoder；
- Golden：136 个中间断言；
- Contract：14 API operation；
- Integration：PostgreSQL、RLS、Outbox、lease、Quote ETag；
- Fault：两段事务崩溃、Artifact timeout、重复完成；
- Performance：单包、200 包、1000 Batch、50 Comparison；
- Security：tenant、BUY、PII、Artifact。

## 11. 研发环境

最低环境：Go 项目锁定版本、PostgreSQL 项目锁定版本、对象存储兼容服务。Redis/Broker 在其功能 Task 前才引入，不能成为本地运行前置。

本地一条命令应完成：

```bash
make dev-up
make migrate
make test
make golden
make contract
```

## 12. 数据初始化

顺序：Tenant/Organization → Carrier/Product → Typed Policy → RateTable → PricingPlan → Contract → Golden Suite → Compile Artifact → Activate Release。

任何直接 UPDATE 正式价卡的运维脚本都禁止进入生产。

## 13. UPS Ground 首个验证切片

首个纵向切片选择：单包裹、SELL Quote、UPS Ground、Zone 2–8、体积除数 250、住宅、AHS Dimension、Fuel。必须从 API 输入贯穿到 Snapshot、Fact、Artifact、26 阶段、ChargeLine、数据库和 Explanation。

源价卡中的 Unauthorized 金额冲突、FedEx 标签污染、“1.5折起”和燃油表述歧义保持隔离，不得为了演示而猜测。

## 14. 生产门禁

MVP-A 试运行前：

- 136 Golden Cases 100%；
- Replay hash 100%；
- API Contract 100%；
- PostgreSQL 真实 Migration/RLS/Trigger 测试；
- BUY/tenant 泄漏 0；
- 热 Artifact 性能达目标或有批准偏差；
- PITR 恢复演练；
- Pricing、应用、数据库分别有回滚方案；
- 关键 Runbook 演练；
- 运营只可通过 Governance 发布价卡。

## 15. 风险与控制

| 风险 | 控制 |
|---|---|
| 过早微服务化 | 模块化单体，Worker 仅进程角色分离 |
| 页面先行导致内核空心 | 纵向切片和 Golden Gate 优先 |
| PostgreSQL 成为规则引擎 | Runtime 只消费 Artifact |
| Story Point 被误当工期 | 仅作为相对容量，团队校准 |
|价卡歧义被静默处理 | Import quarantine + 发布阻断 |
| 并行导致结果漂移 | 稳定排序、串行合并、Replay hash |
| BUY 泄漏 | 独立 Evaluation + Projection + Security test |
| Migration 锁表 | Expand/Migrate/Contract、Concurrent index |
| Worker 重复计算 | Idempotency、lease、条件完成 |
| 外部缓存/消息故障 | 不进入计费正确性热路径 |

## 16. 任务工件

配套文件：

- `mvp-work-breakdown-v1.0.1.yaml`：完整 Epic/Task；
- `mvp-sprint-plan-v1.0.1.yaml`：Sprint 目标和退出条件；
- `mvp-traceability-matrix-v1.0.1.json`：Task 对上游、Golden、API、表的追踪；
- `mvp-task-import-v1.0.1.csv`：可导入任务管理工具；
- `validate_mvp_implementation_plan_v1_0_1.py`：结构审查。

## 17. 冻结结论

本实施设计足以启动 Sprint 0。首个开发动作不是写计费公式，而是建立可验证工程基线；首个业务里程碑不是“有页面”，而是 UPS Ground 纵向切片通过 Golden Cases、API Contract、PostgreSQL 持久化和 Replay。
