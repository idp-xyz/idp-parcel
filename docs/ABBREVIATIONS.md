# 缩写与标识索引

**这一页只指路，不定义任何东西。** 每一行给出缩写的全称、一句话定位和权威文档链接；真正的定义永远在被链接的那份文档里。领域术语的权威是[统一领域语言](./domain/GLOSSARY.md)，边界的权威是[领域上下文地图](./domain/CONTEXT-MAP.md)——本页不重复它们的内容，也不得与它们冲突。

会这么约束是因为[维护原则](./README.md)那一条：一个决策只保留一个权威定义。缩写页一旦自己解释起「投影是什么」，就会长出第二套口径，而两套口径一定先后漂移。

## 限界上下文缩写

十个限界上下文的所有权边界见[领域上下文地图](./domain/CONTEXT-MAP.md)。这里只给代码、目录名与中文名的对照——在此之前，这份对照只存在于上下文地图那张 mermaid 图的节点标签里，搜不到也不成表。

| 缩写 | 目录名 | 中文名 | 权威文档 |
|---|---|---|---|
| `PC` | `party-commercial` | 参与方与商业 | [CONTEXT.md](./domain/party-commercial/CONTEXT.md) |
| `PP` | `parcel-pricing` | 小包计价 | [CONTEXT.md](./domain/parcel-pricing/CONTEXT.md) |
| `PS` | `parcel-shipment` | 小包托运 | [CONTEXT.md](./domain/parcel-shipment/CONTEXT.md) |
| `NR` | `network-routing` | 网络与路由 | [CONTEXT.md](./domain/network-routing/CONTEXT.md) |
| `NO` | `node-operations` | 节点作业 | [CONTEXT.md](./domain/node-operations/CONTEXT.md) |
| `TF` | `transport-fulfillment` | 运输履约 | [CONTEXT.md](./domain/transport-fulfillment/CONTEXT.md) |
| `CC` | `customs-compliance` | 关务与贸易合规 | [CONTEXT.md](./domain/customs-compliance/CONTEXT.md) |
| `VE` | `visibility-exception` | 全程追踪与异常 | [CONTEXT.md](./domain/visibility-exception/CONTEXT.md) |
| `SA` | `settlement-accounting` | 结算与经营核算 | [CONTEXT.md](./domain/settlement-accounting/CONTEXT.md) |
| `CR` | `collection-remittance` | 代收与清分 | 仅由[上下文地图](./domain/CONTEXT-MAP.md)定义 |

`CR` 没有自己的 `CONTEXT.md`：首发已确认不包含代收货款，按「不建立只有标题的空文档」这条不创建。

## Go 包前缀命名法

代码里的包别名由上表的缩写拼成，规律与目录结构一一对应。知道这条之后，`pstf.ErrPickupNotVisible` 这种符号就能直接读出归属，不必去翻 import 段。

跨上下文的适配器一律放在**消费方**目录下（`internal/<拥有方>/adapters/<提供方>/`），因此别名的第一段是拥有方、第二段是提供方或技术层：

| 别名 | 实际包路径 | 读法 |
|---|---|---|
| `pstf` | `internal/parcelshipment/adapters/transportfulfillment` | 小包托运里的运输履约适配器 |
| `psnodeops` | `internal/parcelshipment/adapters/nodeoperations` | 小包托运里的节点作业适配器 |
| `venodeops` | `internal/visibilityexception/adapters/nodeoperations` | 全程追踪里的节点作业适配器 |
| `nrpartycommercial` | `internal/networkrouting/adapters/partycommercial` | 网络与路由里的参与方与商业适配器 |
| `vepostgres` | `internal/visibilityexception/adapters/postgres` | 全程追踪的 PostgreSQL 适配器 |
| `veinbox` | `internal/visibilityexception/adapters/inbox` | 全程追踪的消费门（收信封那一层） |
| `veapplication` | `internal/visibilityexception/application` | 全程追踪的应用编排层 |
| `nrdomain` | `internal/networkrouting/domain` | 网络与路由的领域层 |

「适配器在消费方」这条取舍本身由 ADR 裁定，见[架构决策记录](./adr/README.md)；本表只记怎么读，不复述它的理由。

**第二段的写法不统一，这是现状不是规则**：提供方有时缩成两字母（`pstf`、`vetf`），有时写全（`nrpartycommercial`、`pspartycommercial`）。按上面两段拆开读即可，不要据此推断某种命名规范。

## 标识前缀家族

看到一个陌生标识时，先按前缀定位它属于哪个登记册，再去对应权威文档里查具体条目。

| 前缀 | 例 | 是什么 | 去哪查 |
|---|---|---|---|
| `PN-` | `PN-06` | 首发开发切片编号 | [开发主线](./product/PARCEL-NETWORK-FIRST-RELEASE-DEVELOPMENT-BASELINE.md) |
| `PAR-` | `PAR-COM-17` | 首发试点参数与证据登记条目 | [参数登记册](./product/PILOT-PARAMETER-REGISTER.md) |
| `UC-` | `UC-VE-002` | 应用用例 | [应用用例索引](./application/README.md) |
| `AT-` | `AT-VE-044` | 用例内的验收场景 | 对应 `UC-*` 的「首发约束与验收」表 |
| `ADR-` | `ADR-0049` | 架构决策记录 | [ADR 索引](./adr/README.md) |
| `BD-` | `BD-PP-001` | 尚未确认的业务决定 | 对应设计简报，见 [docs/README.md](./README.md) |
| `W` | `PN02-W03`、`CC-S0-W01` | 交接文档里的工作包 | 对应 PN 切片或关务切片的 handoff |
| `SYN-` | `SYN-COM-01` | 隔离合成实例 | 对应 `PN02-SYN` 等合成任务包 |

`PAR-` 与 `UC-`/`AT-` 的第二段都是上下文缩写，但取值范围不同：`PAR-` 用的是参数分类（`COM`、`NET`、`INT`、`CUS`、`NFR`、`VIS`、`GOV`、`SET`），不是限界上下文。

`UC-` 目前覆盖八个上下文（`PC`、`PS`、`NR`、`NO`、`TF`、`CC`、`VE`、`SA`）。`parcel-pricing` 与 `collection-remittance` 没有 `UC-*`。

### 两个容易混的登记册

`VIS-01` 和 `PAR-VIS-01` 看起来像一对，其实分属两个登记册，不能互相替代：

- `VIS-01` 是[验收场景矩阵](./product/PILOT-ACCEPTANCE-MATRIX.md)里的**验收场景 ID**，回答「怎么证明这条能力成立」。同族还有 `E2E-`、`GOV-`、`CPS-`、`OPS-`、`CUS-`、`SET-`、`DAT-`、`NFR-`、`INT-`。
- `PAR-VIS-01` 是[参数登记册](./product/PILOT-PARAMETER-REGISTER.md)里的**参数条目**，回答「这条能力还缺哪个真实参数」。

参数登记册的「关联场景」列把两者连起来。引用时务必带全前缀。

## 证据层级

权威定义在[验收场景矩阵](./product/PILOT-ACCEPTANCE-MATRIX.md)的「证据层级」一节，此处只作速查。

| 代码 | 含义 |
|---|---|
| `P` | 真实生产 |
| `R` | 历史数据回放 |
| `S` | 受控模拟 |
| `N/A` | 本期不适用 |

计划证据里的 `+` 表示几个层级都必须覆盖，`/` 表示按已批准条件选其一；`P+R/S` 因此读作「必须有 `P`，另按条件补 `R` 或 `S`」。

层级只降不升：隔离合成只能记 `S`，重放结果再一致也不升为 `R`。这条与 [AGENTS.md](../AGENTS.md) 的「证据层级诚实」是同一条红线。

## 其他常见缩写

| 缩写 | 全称 | 在本仓的位置 |
|---|---|---|
| `COD` | Cash On Delivery，代收货款 | 首发已排除；边界见[上下文地图](./domain/CONTEXT-MAP.md)的 `collection-remittance` |
| `POD` | Proof of Delivery，交付证明 | 由 `transport-fulfillment` 拥有 |
| `WMS` | 仓储管理系统 | 用于划界——节点作业**不**提供完整 WMS 能力 |
| `TMS` | 运输管理系统 | 指公司另一产品 `idp-tms`；本产品不依赖它的领域模型 |
| `DSN` | 数据库连接串 | 环境变量 `IDP_PARCEL_POSTGRES_DSN`，见 [workflow.md](./agents/workflow.md) |

## 本页不收录什么

- **票名与实现期用语**（`CONS-PROJ-A`、`MAP-KIND`、`FanOut` 这类）。它们以周为单位过期，写进权威文档等于预约一批死链；需要留存时放 [docs/agents/](./agents/)。
- **领域术语**（`委托`、`投影`、`采认`、`预计承诺`）。权威在[统一领域语言](./domain/GLOSSARY.md)，本页不给第二个定义。
- **尚未确认的标识**。未确认参数保持显式未决，不在本页写成看起来已定的样子。
