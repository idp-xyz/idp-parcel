# Rating Runtime 技术设计 V1.0.1 终审报告

**审查对象：**《Rating Runtime 技术设计 V1.0.1 终审版》  
**机器架构清单：** `rating-runtime-architecture-manifest-v1.0.1.yaml`  
**Artifact Schema：** `compiled-pricing-plan-manifest-v1.0.1.schema.json`  
**Artifact 示例：** `compiled-pricing-plan-manifest-example-v1.0.1.json`

---

## 1. 审查基线

本次终审逐项对照：

1. 总体方案 V1.2；
2. 领域模型 V1.0.1；
3. 计算语义 V1.0.1；
4. Golden Cases V1.0.1；
5. API Contract V1.0.1。

---

## 2. 审查方式

```text
建立上游术语与不变量基线
        ↓
设计运行时组件和依赖方向
        ↓
映射 26 个执行阶段
        ↓
设计 Artifact、缓存和发布
        ↓
设计事务、幂等和恢复
        ↓
设计同步、异步、批量、比较和 Replay
        ↓
生成机器架构 Manifest
        ↓
生成 Artifact JSON Schema 与示例
        ↓
校验模块 DAG 和 Charge DAG
        ↓
校验 OpenAPI operation 映射
        ↓
审查 Markdown 和术语残留
```

---

## 3. 终审中解决的关键问题

### 3.1 是否立即采用微服务

最终选择：

```text
模块化单体优先
+
API/Worker 进程角色可独立扩展
```

避免计费内核过早产生网络调用和分布式一致性问题。

### 3.2 计算热路径是否依赖 Redis 或消息中间件

最终冻结：

- Redis 可选；
- 外部 Broker 可选；
- 缓存和事件基础设施故障不得改变金额；
- PostgreSQL、不可变 Artifact 和对象存储构成权威路径。

### 3.3 如何避免运行时动态解释配置

采用发布时 `CompiledPricingPlan`：

- 类型检查；
- 单位检查；
- Rate Index；
- Charge DAG；
- 稳定拓扑序；
- VersionManifest；
- Golden Case Gate。

运行时不再解析任意规则。

### 3.4 如何避免长事务

采用：

```text
事务 A：幂等 + Snapshot + REQUESTED
纯计算
事务 B：COMPLETED/FAILED + Outbox + 幂等结果
```

并增加 lease 和 Reaper。

### 3.5 如何保证并行仍确定

- 阶段顺序固定；
- 阶段内并行；
- 每任务独立 slot；
- 稳定排序后合并；
- 禁止依赖 goroutine 完成顺序和 map 遍历。

### 3.6 如何保证 Replay 不误用当前配置

Replay 使用不同 Application 路径，只允许：

- 原 Snapshot；
- 原 FactRefs；
- 原 Artifact hash；
- 原 VersionManifest；
- 原 CustomFunction。

### 3.7 如何防止数据库结构反向影响领域

技术设计只定义 Repository Port 和事务语义，不提前冻结物理表。

下一阶段数据库设计必须服从聚合与访问模式。

---

## 4. 机器审查结果

```json
{
  "status": "PASSED",
  "manifest_version": "1.0.1",
  "module_count": 29,
  "service_count": 4,
  "stage_count": 26,
  "api_operation_mappings": 14,
  "plan_component_count": 3,
  "charge_graph_node_count": 5,
  "errors": []
}
```

---

## 5. 结构审查结论

| 检查项 | 结果 |
|---|---:|
| 模块依赖 DAG | 通过 |
| Domain 禁止依赖 | 通过 |
| Engine 禁止依赖 | 通过 |
| 26 阶段顺序 | 完全一致 |
| API operation 映射 | 14/14 |
| 核心枚举差异 | 0 |
| Artifact Schema | 通过 |
| Artifact Charge DAG | 无环 |
| Artifact 拓扑序 | 通过 |
| Markdown 代码块 | 闭合 |
| 重复标题 | 0 |
| 外部执行平台依赖 | 0 |

---

## 6. 冻结结论

本技术设计可以冻结以下内容：

- 模块化单体与进程角色；
- Engine 纯计算边界；
- Port/Adapter；
- 两段事务；
- 26 阶段 Stage Executor；
- Artifact 内容寻址；
- L1/L2 缓存语义；
- 运行时 DAG；
- 幂等、lease 和恢复；
- 同步、异步、批量和比较；
- Replay；
- 性能和高可用原则。

---

## 7. 下一阶段

下一阶段正式进入：

```text
PostgreSQL 数据模型与迁移设计 V1.0
```

该文档必须同时引用：

- 领域聚合边界；
- API 访问模式；
- 本技术设计的 Repository Port；
- 双时间和不可变版本；
- Evaluation append-only；
- Outbox、Idempotency、Operation lease；
- 大型 RateTable 和 Artifact 的存储边界。
