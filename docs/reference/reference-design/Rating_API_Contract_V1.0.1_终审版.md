# Rating API Contract

**文档版本：** V1.0.1  
**文档状态：** 终审 API 基线 / 可进入 Runtime 技术设计与数据库设计阶段  
**OpenAPI：** `rating-api-openapi-v1.0.1.yaml`  
**示例集：** `rating-api-examples-v1.0.1.json`  
**验证程序：** `validate_rating_api_contract_v1_0_1.py`

**上游规范：**

- 《国际小包计费与结算平台最终解决方案 V1.2 审定版》
- 《国际小包计费领域模型 V1.0.1 终审版》
- 《Rating Runtime 计算语义规范 V1.0.1 终审版》
- 《国际小包计费 Golden Cases V1.0.1 终审版》

---

# 0. 文档目的与权威性

本文冻结 Rating、Eligibility、Comparison、Batch 和 Quote 接口的 HTTP 资源、Schema、幂等、并发、同步/异步、错误、安全投影和版本语义。

本文不重新定义计费算法；算法权威仍是《Rating Runtime 计算语义规范 V1.0.1》。实现代码、DTO、数据库和 API Gateway 不得通过默认值或转换改变其语义。

冲突时权威顺序：

```text
正式业务合同与价卡
→ Rating Runtime 计算语义 V1.0.1
→ 本 API Contract
→ 领域模型 V1.0.1
→ 总体方案 V1.2
→ 实现代码
```

# 1. 与上游基线的一致性审查

1. RatingEvaluation 是纯计算结果；
2. 一次核心计费请求只创建一个 CalculationPurpose、一个 PriceRole、一个 Evaluation；
3. BUY 与 SELL 独立；SELL 只能显式引用冻结 BUY Evaluation；
4. Quote 只引用 COMPLETED 的 SELL/PUBLIC Evaluation；
5. Quote Accepted 不自动产生 ChargeAssessment；
6. Rating API 不直接形成应收、应付或财务凭证；
7. MeasurementFact 与 CarrierAssessmentFact 分开传输；
8. 生产计算使用一个冻结 PricingRelease；
9. REPLAY 只能使用原快照、原事实和原版本；
10. 失败不返回部分正式金额；
11. 不可承运不是金额为零的成功 Evaluation；
12. 正式金额非负，方向由 ADD/DEDUCT 表达。

## 1.1 对 V1.2 暂定路由的精化

| V1.2 暂定表达 | 正式路由 | 理由 |
|---|---|---|
| `POST /v1/services/eligible` | `POST /v1/eligibility-evaluations` | 明确可审计的评估资源 |
| `POST /v1/ratings/evaluate` | 保留 | 保持主集成入口 |
| `POST /v1/ratings/compare` | `POST /v1/rating-comparisons` | Comparison 为独立应用资源 |
| `POST /v1/quotes/{id}/accept` | 保留并要求 `If-Match` | 防止并发状态覆盖 |

## 1.2 有意排除的相邻 API

本 Contract 不覆盖 Pricing Governance、Charge Assessment、Financial Charging、Carrier Reconciliation、Settlement 和 Security Administration。它们是独立限界上下文，应形成专项 Contract。

# 2. 协议与表示

```text
HTTPS
JSON UTF-8
OpenAPI 3.1
JSON Schema 2020-12
application/json
application/problem+json
```

- URL：小写 kebab-case；
- JSON：snake_case；
- 枚举：UPPER_SNAKE_CASE；
- ID：不透明稳定字符串，不暴露数据库自增键；
- 请求严格拒绝未知字段；
- 客户端必须忽略未知响应字段；
- Decimal 一律作为字符串；
- 科学计数法和 binary float 禁止；
- Money.amount 非负；折扣和封顶使用 DEDUCT；
- 邮编按字符串处理，保留前导零。

# 3. 安全与租户

- Bearer Token 来自 OIDC/OAuth2；
- tenant_id 从验证后的 Token Claim 取得，不在请求体中覆盖；
- organization_id 必须属于调用方范围；
- 无权限资源可以返回 404 防止枚举；
- requested_projection 只是偏好，不能提升权限。

## 3.1 授权 Scope

| Scope | 权限 |
|---|---|
| `eligibility:evaluate` | 执行单渠道准入判断 |
| `rating:evaluate` | 创建 RatingEvaluation |
| `rating:read` | 读取 RatingEvaluation |
| `rating:replay` | 历史重放 |
| `rating:compare` | 多渠道比较 |
| `rating:batch` | 提交批量计费 |
| `rating:batch:read` | 读取批量状态与结果 |
| `cost:read` | 读取 BUY/INTERNAL 总价与费用行 |
| `trace:read` | 读取执行轨迹 |
| `evidence:read` | 读取规则和事实证据 |
| `quote:create` | 创建 DRAFT Quote |
| `quote:offer` | 发布 Quote |
| `quote:accept` | 接受 Quote |
| `quote:cancel` | 取消 Quote |
| `quote:reprice` | 生成新 QuoteVersion |
| `quote:read` | 读取 Quote |

没有 `cost:read` 时，不返回 BUY Evaluation、BUY ChargeLine、buy_total 或可反推采购价的版本证据。

# 4. 通用 Header

## 4.1 Authorization

```http
Authorization: Bearer <token>
```

## 4.2 Idempotency-Key

所有 POST 必须提供：

```http
Idempotency-Key: <8-128 chars>
```

作用域：

```text
tenant + principal + operationId + idempotency_key
```

相同 key 和相同规范化请求返回原业务结果；相同 key 不同请求返回 `409 IDEMPOTENCY_CONFLICT`。

## 4.3 X-Correlation-Id 与 traceparent

用于关联和分布式追踪，但不参与业务请求哈希。

## 4.4 Prefer

```http
Prefer: respond-sync
Prefer: respond-async
```

同步和异步必须使用同一算法和版本。

## 4.5 If-Match

Quote offer、accept、cancel、reprice 必须带当前 ETag。失败返回 412。

# 5. HTTP 状态

| 状态 | 含义 |
|---:|---|
| 200 | 查询或状态命令成功 |
| 201 | Evaluation、Eligibility、Comparison、Quote/Version 创建成功 |
| 202 | 已接受异步执行 |
| 304 | 条件 GET 未变化 |
| 400 | JSON、字段、类型、格式或未知字段错误 |
| 401 | 未认证 |
| 403 | 无权限 |
| 404 | 不存在或不可见 |
| 409 | 幂等、状态、版本冲突 |
| 412 | ETag 前置条件失败 |
| 422 | 准入、事实、价表、策略或计算错误 |
| 429 | 限流 |
| 500 | 未预期错误 |
| 503 | 依赖不可用 |
| 504 | 超时；无部分正式金额 |

# 6. Purpose、Role 与事实

| Purpose | PriceRole | calculation_basis | 默认事实 | 语义 |
|---|---|---|---|---|
| `QUOTE` | SELL、PUBLIC | 禁止 | 客户申报/合同策略 | 只生成计算结果，可创建 Quote |
| `ESTIMATED_COST` | BUY、INTERNAL | 禁止 | 仓库测量优先 | 预估采购成本 |
| `ACTUAL_COST` | BUY、INTERNAL | 必需 | Measurement 或 CarrierAssessment | 必须声明 calculation_basis |
| `CUSTOMER_BILLING` | SELL、PUBLIC | 禁止 | 客户合同指定 | 只是候选客户费用，不直接形成应收 |
| `SIMULATION` | 全部 | 按模拟目标 | 显式 | 可使用 Draft/Test 版本，不得进入正式业务 |
| `DISPUTE_REVIEW` | BUY、INTERNAL、SELL、PUBLIC | 按分析目标 | 显式事实集合 | 每种事实生成独立 Evaluation |
| `REPLAY` | 沿原 Evaluation | 沿原 Evaluation | 原冻结事实 | 只能使用 replay endpoint |

强制规则：

- REPLAY 不得调用普通 evaluate；
- ACTUAL_COST 必须传 calculation_basis；
- EXPLICIT_COMPONENTS 只允许 SIMULATION；
- CUSTOMER 投影不得请求成本角色；
- CarrierAssessmentFact 不能解释为物理 Measurement；
- QUOTE 和 CUSTOMER_BILLING 不形成应收。

# 7. 端点总览

| Method | Path | operationId | 作用 |
|---|---|---|---|
| `POST` | `/v1/eligibility-evaluations` | `createEligibilityEvaluation` | Evaluate service eligibility |
| `POST` | `/v1/ratings/evaluate` | `evaluateRating` | Create one immutable RatingEvaluation |
| `GET` | `/v1/rating-evaluations/{evaluation_id}` | `getRatingEvaluation` | Read an immutable RatingEvaluation |
| `POST` | `/v1/rating-evaluations/{evaluation_id}/replays` | `replayRatingEvaluation` | Replay using original immutable artifacts |
| `POST` | `/v1/rating-comparisons` | `createRatingComparison` | Compare candidate service products |
| `POST` | `/v1/rating-batches` | `createRatingBatch` | Submit asynchronous rating batch |
| `GET` | `/v1/rating-batches/{batch_id}` | `getRatingBatch` | Read batch status |
| `GET` | `/v1/rating-batches/{batch_id}/results` | `getRatingBatchResults` | Read paged batch results |
| `POST` | `/v1/quotes` | `createQuote` | Create DRAFT Quote from completed SELL/PUBLIC evaluation |
| `GET` | `/v1/quotes/{quote_id}` | `getQuote` | Read Quote |
| `POST` | `/v1/quotes/{quote_id}/offer` | `offerQuote` | Offer a DRAFT Quote |
| `POST` | `/v1/quotes/{quote_id}/accept` | `acceptQuote` | Accept an OFFERED Quote |
| `POST` | `/v1/quotes/{quote_id}/cancel` | `cancelQuote` | Cancel a DRAFT or OFFERED Quote |
| `POST` | `/v1/quotes/{quote_id}/repricings` | `repriceQuote` | Create a new QuoteVersion from a new evaluation |

# 8. Eligibility API

`POST /v1/eligibility-evaluations`

- 只判断指定渠道可承运性，不计算金额；
- eligible=false 仍返回 201，因为判断本身成功；
- 返回全部拒绝原因和策略版本；
- Package ID 必须唯一；
- 规则需要重量或尺寸时必须提供对应物理事实。

Rating API 中准入失败则返回 422，不得生成零金额 COMPLETED Evaluation。

# 9. Rating API

`POST /v1/ratings/evaluate`

一次请求等于一个 Purpose、一个 PriceRole 和一个不可变 Evaluation。SELL 成本加价可以内部显式引用 BUY Evaluation，但对外主资源仍为一个 SELL Evaluation。

- 201：已完成并持久化；
- 202：异步接受，必须提供 status_url；
- 422：业务失败；可携带 FAILED evaluation_id；
- 不允许返回部分费用或部分总价。

`GET /v1/rating-evaluations/{{evaluation_id}}`

Evaluation 不可更新和删除。`include=lines,trace,evidence,commercial_summary` 仍受权限约束。COMPLETED Evaluation 的 ETag 应来自内容哈希。

# 10. Replay API

`POST /v1/rating-evaluations/{{evaluation_id}}/replays`

不得携带新输入、新事实、新 Release、新汇率或字段覆盖。Replay 创建 purpose=REPLAY 的新 Evaluation，并比较 content hash。缺少工件返回 `REPLAY_ARTIFACT_MISSING`，不得使用 latest 替代。

# 11. Comparison API

`POST /v1/rating-comparisons`

- 2–50 个唯一候选渠道；
- 共用 ShipmentSnapshot；
- 每个候选独立准入和计费；
- 候选状态：PRICED、INELIGIBLE、FAILED；
- 局部失败只存在于 Comparison 组合层；
- 无 cost:read 时不能按 BUY_TOTAL 排序；
- 同值按请求顺序和 ID 稳定排序。

# 12. Batch API

`POST /v1/rating-batches`

- 1–1000 项；
- 总是异步；
- 每项独立成功/失败；
- client_item_id 在批次内唯一；
- 批次幂等键与 item key 是不同概念。

`GET /v1/rating-batches/{{batch_id}}` 返回状态；`GET /results` 使用 cursor 分页。失败项返回 ProblemDetails，不返回部分金额。

# 13. Quote API

```text
COMPLETED SELL/PUBLIC Evaluation
→ Quote DRAFT
→ OFFERED
→ ACCEPTED
```

- 创建 Quote 不产生费用认定；
- Offer 需要 DRAFT、ETag 匹配和有效 valid_until；
- Accept 需要 OFFERED、未过期、ETag 匹配；
- Accept 只表示商业承诺，不表示运输发生；
- DRAFT/OFFERED 可 cancel；ACCEPTED 默认不得直接取消；
- Reprice 必须引用新的 COMPLETED Evaluation，创建新 QuoteVersion，保留历史。

# 14. 输入快照与事实

RatingContext 包含 customer、contract、service product、warehouse、carrier account、organization。tenant 不在 Body 中。

ShipmentSnapshot 创建后不可变。Package 数组输入顺序不是计算语义，Runtime 使用稳定排序。

MeasurementFactInput 只表达物理重量和尺寸；CarrierAssessmentFactInput 表达 assessed/billed weight、zone 和分类。两者不得覆盖。

# 15. PricingSelection

- ACTIVE_RELEASE：服务端按 tenant、合同、渠道和 business_time 解析并冻结唯一生产 Release；
- PINNED_RELEASE：显式指定一个有权使用的 ACTIVE Release；
- EXPLICIT_COMPONENTS：仅 SIMULATION，不得进入 Quote、Assessment 或 Financial。

# 16. RatingEvaluation 响应

必需：evaluation_id、status、purpose、price_role、snapshot、business_time、pricing_release_id、version_manifest。

COMPLETED 还必须有 package_results、charge_lines、totals 和 content_hash。

PackageResult 必须解释几何、实重、体积重、最低重量、计费重、舍入、特征、Zone 和地址分类。

ChargeLine 必须包含 code、effect、scope、subject、leg、basis、method、raw amount、rounded amount、rule、dependencies 和 evidence。

```text
net_total = gross_additions - gross_deductions
```

CommercialSummary 只在授权下返回，不能替代 BUY Evaluation 的正式证据。

# 17. 错误合同

统一使用 `application/problem+json`，字段包括 type、title、status、detail、error_code、category、request_id、trace_id、retryable、violations 和 version_refs。

| error_code | HTTP | category | 含义 |
|---|---:|---|---|
| `INVALID_REQUEST` | 400 | `INPUT_ERROR` | JSON、字段、格式或类型无效 |
| `UNKNOWN_FIELD` | 400 | `INPUT_ERROR` | 请求出现未定义字段 |
| `IDEMPOTENCY_CONFLICT` | 409 | `CONFLICT_ERROR` | 同一幂等键对应不同规范化请求 |
| `PURPOSE_PRICE_ROLE_CONFLICT` | 422 | `POLICY_ERROR` | 计算目的和价格角色不兼容 |
| `ELIGIBILITY_FAILED` | 422 | `ELIGIBILITY_ERROR` | 指定渠道不可承运；不返回零金额成功 |
| `VERSION_NOT_FOUND` | 422 | `VERSION_ERROR` | 业务时间未命中可用版本 |
| `VERSION_RESOLUTION_CONFLICT` | 409/422 | `VERSION_ERROR` | 同一作用域命中多个互斥版本 |
| `FACT_SELECTION_AMBIGUOUS` | 422 | `FACT_ERROR` | 事实无法唯一选择 |
| `ACTUAL_WEIGHT_REQUIRED` | 422 | `FACT_ERROR` | 算法要求实际重量但事实缺失 |
| `INVALID_GEOMETRY` | 422 | `FACT_ERROR` | 尺寸为零、负数或非法数值 |
| `RATE_NOT_FOUND` | 422 | `RATE_ERROR` | 价表未命中 |
| `RATE_TABLE_OVERLAP` | 422 | `RATE_ERROR` | 价表区间重叠 |
| `CHARGE_EXCLUSIVITY_CONFLICT` | 422 | `CALCULATION_ERROR` | 互斥费用同时命中 |
| `CHARGE_DEPENDENCY_CYCLE` | 422 | `CALCULATION_ERROR` | 费用依赖成环 |
| `FX_RATE_NOT_FOUND` | 422 | `CURRENCY_ERROR` | 汇率版本或路径缺失 |
| `CUSTOM_FUNCTION_FAILED` | 422/500 | `CUSTOM_FUNCTION_ERROR` | 受控函数失败 |
| `REPLAY_ARTIFACT_MISSING` | 422 | `VERSION_ERROR` | 历史重放工件不存在 |
| `QUOTE_STATE_CONFLICT` | 409 | `CONFLICT_ERROR` | Quote 当前状态不允许命令 |
| `ETAG_MISMATCH` | 412 | `CONFLICT_ERROR` | If-Match 与当前 Quote 版本不一致 |
| `RATE_LIMITED` | 429 | `SYSTEM_ERROR` | 调用频率超过限制 |
| `DEPENDENCY_UNAVAILABLE` | 503 | `SYSTEM_ERROR` | 版本、事实或存储依赖不可用 |
| `EXECUTION_TIMEOUT` | 504 | `SYSTEM_ERROR` | 执行超时且不返回部分正式金额 |

客户端不得只按 HTTP 状态重试，必须读取 retryable。

# 18. 幂等与规范化哈希

哈希包含 tenant、principal、operation、业务 Body、purpose、role、business_time 和 pricing selection；不包含 Token、correlation ID、traceparent。

JSON 规范化必须固定对象 key、Decimal 表示和数组业务顺序。metadata 参与哈希。

# 19. 并发与一致性

- Evaluation 不可变；
- Quote 使用 ETag/If-Match；
- Quote 命令在聚合事务内完成并通过 Outbox 发布；
- 一次 Rating 只消费一个完整 PricingRelease manifest；
- 创建成功后保证读己之写。

# 20. 同步、异步与超时

202 必须提供 status_url，推荐 Retry-After。最终资源 ID 已知时应返回。幂等重试必须回到同一 operation/resource。

若服务端不能确认结果，返回 504；客户端使用同一幂等键重试。服务端不得重复产生业务副作用。

# 21. 分页

Batch results 使用不透明 cursor，limit 1–200。cursor 与 tenant、过滤条件绑定，不允许客户端解析或修改。

# 22. 版本兼容

`/v1` 表示主版本。兼容变更包括新增可选响应字段、端点和默认行为不变的可选请求字段。删除/改名字段、改变 Decimal、舍入、区间、结果层次或 Quote Accept 语义属于破坏性变更，必须发布 `/v2`。

客户端应对响应枚举准备 UNKNOWN fallback；请求未知枚举由服务端 400 拒绝。

# 23. OpenAPI 机器合同

```text
Paths: {len(PATHS)}
Operations: {len(endpoints)}
Schemas: {len(S)}
Validation examples: {len(VALIDATION['examples'])}
```

运行：

```bash
python validate_rating_api_contract_v1_0_1.py \
  rating-api-openapi-v1.0.1.yaml \
  rating-api-examples-v1.0.1.json
```

校验 OpenAPI 版本、operationId、POST 幂等、Quote If-Match、上游枚举、示例 Schema 和关键语义。

# 24. 事件边界

事务成功后可通过 Outbox 发布 RatingEvaluationCompleted/Failed、QuoteCreated/Offered/Accepted/Cancelled/Superseded。事件只引用资源 ID，不复制未授权 BUY 明细。完整格式在后续 AsyncAPI Contract 冻结。

# 25. 可观测性与审计

记录 request_id、trace_id、tenant、operation、purpose、role、service product、release、evaluation、latency、status、error、幂等重放和投影。

不得记录 Token、完整地址、合同正文、未脱敏成本或客户 PII。

审计谁读取成本、执行 Replay、创建/Offer/Accept/Cancel/Reprice Quote、使用哪些版本，以及越权尝试。

# 26. 客户端要求

1. 使用 Decimal 字符串；
2. 每个 POST 使用稳定幂等键；
3. 不把 422 当网络错误重试；
4. 对 202 轮询 status_url；
5. Quote 命令带 If-Match；
6. 不依赖字段顺序；
7. 忽略未知响应字段；
8. 不解析 ID；
9. 不把 Quote Accepted 当应收；
10. 保存 evaluation_id、content_hash、pricing_release_id。

# 27. 服务端要求

1. 进入 Runtime 前冻结 Release；
2. 验证 purpose-role-basis；
3. 拒绝未知字段；
4. 使用 Decimal；
5. 建立不可变快照；
6. 不返回部分总额；
7. 授权投影成本字段；
8. 保存幂等业务结果；
9. Evaluation 不可更新；
10. Quote 使用聚合事务和 ETag；
11. 同步/异步语义相同；
12. 运行全部 Golden Cases。

# 28. Golden Cases 映射

```text
API Request
→ DTO validation
→ immutable RatingInputSnapshot
→ Rating Runtime
→ RatingEvaluation response
```

DTO Mapper 不得预先舍入、删除邮编前导零、合并两类事实、使用 float 或将 latest 价卡替换显式版本。

# 29. 终审自洽检查

- RatingEvaluation=计算；Quote=商业承诺；Assessment/Financial 不在本文；
- Measurement 与 CarrierAssessment 分离；
- 一个 Evaluation 一个 PriceRole；
- BUY/SELL 独立；
- business_time 显式，Release 原子；
- Replay 使用原版本；
- 单 Evaluation 失败无部分金额；Comparison/Batch 只在组合层局部失败；
- 所有 POST 幂等；Quote 状态命令并发受控。

# 30. 冻结结论

本 Contract 可作为 Rating Runtime 技术设计、PostgreSQL 数据模型、API Gateway、SDK、契约测试和 MVP 接口实现的正式输入。

下一阶段建议先完成《Rating Runtime 技术设计 V1.0》，再用访问模式和聚合事务审查 PostgreSQL 数据模型，避免数据库反向扭曲计算语义。
