# Rating API Contract V1.0.1 终审报告

**审查对象：**《Rating API Contract V1.0.1 终审版》  
**机器合同：** `rating-api-openapi-v1.0.1.yaml`  
**示例：** `rating-api-examples-v1.0.1.json`

## 1. 基线

总体方案 V1.2、领域模型 V1.0.1、Runtime 计算语义 V1.0.1、Golden Cases V1.0.1。

## 2. 审查流程

```text
提取聚合、命令、状态机和枚举
→ 区分 Rating/Quote/Assessment/Financial
→ 设计 HTTP 与错误合同
→ 生成 OpenAPI 3.1
→ 生成示例
→ JSON Schema 校验
→ 幂等/ETag 检查
→ 枚举与语义交叉审查
→ 回写终审版
```

## 3. 已解决的关键问题

1. 一次 evaluate 只创建一个 Purpose、Role 和 Evaluation；
2. 不可承运：Eligibility 返回 eligible=false；Rating 返回 422；
3. 201/202 不改变算法；
4. Comparison/Batch 的局部失败不污染单 Evaluation 语义；
5. Quote Accept 不创建 Assessment；
6. BUY/INTERNAL 和成本证据按 scope 隔离；
7. Replay 独立 endpoint 且禁止覆盖；
8. 数值全部为 Decimal 字符串；
9. 所有 POST 强制幂等；
10. Quote 状态命令强制 If-Match。

## 4. 机器审查目标

| 项目 | 目标 |
|---|---:|
| OpenAPI | 3.1.0 |
| Version | 1.0.1 |
| Paths | 14 |
| Operations | 14 |
| Schemas | 72 |
| Examples | 11 |
| operationId 重复 | 0 |
| POST 缺幂等 | 0 |
| Quote 命令缺 If-Match | 0 |
| 核心枚举差异 | 0 |
| 示例 Schema 错误 | 0 |

## 5. 范围

已覆盖 Eligibility、Rating、Replay、Comparison、Batch、Quote。Pricing Governance、Assessment、Financial、Reconciliation 和 Settlement 有意排除，应分别编写 Contract。

## 6. 后续验证

- 实现运行 136 个 Golden Cases；
- 多语言 Decimal；
- 202/幂等故障恢复；
- 成本字段权限；
- ETag 并发；
- SDK 生成；
- Gateway Body/timeout；
- AsyncAPI 事件合同。

## 7. 结论

达到 API Contract 冻结条件。下一阶段进入 Rating Runtime 技术设计 V1.0，随后进行 PostgreSQL 数据模型与迁移设计。
