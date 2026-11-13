# 上下文地图

对照 main `1bdbc1b7b`（2026-11-12）写成；「谁读谁」读自代码的 import。

本页只回答一件事：上下文之间，谁拥有什么，谁消费谁。每个上下文自己的规则在它的文件里，见各文件的「管什么，不管什么」。

## 上下文一览

代码包在 `internal/` 下，包名是上下文名去掉连字符。

| 上下文 | 拥有什么 | 文件 |
|---|---|---|
| 参与方与商业（`party-commercial`） | 参与方身份与关系，服务产品、客户合同、供应商商业协议等商业版本及其发布，商业依据解析与商业授权裁定 | [party-commercial.md](./contexts/party-commercial.md) |
| 小包托运（`parcel-shipment`） | 委托及其接受或拒绝决定、委托接受基线、包裹身份、面单交易、包裹取消与终局服务结果、预计承诺与正式承诺 | [parcel-shipment.md](./contexts/parcel-shipment.md) |
| 网络与路由（`network-routing`） | 网络目录及其版本、可达性判断、包裹的路由计划、改路建议与改路决定 | [network-routing.md](./contexts/network-routing.md) |
| 节点作业（`node-operations`） | 节点收寄与身份核对、节点控制的成立与转移、集运单元实例及其成员 | [node-operations.md](./contexts/node-operations.md) |
| 运输履约（`transport-fulfillment`） | 场外揽收、节点间运输、末端派送里的控制事实，权威运输交接与有效交付，运输委托、班次与装载分配，运输收费发生项 | [transport-fulfillment.md](./contexts/transport-fulfillment.md) |
| 关务与贸易合规（`customs-compliance`） | 关务案件、申报单元与提交版本、外部监管结果、监管限制与内部合规限制、放行门禁核对、动作准入判断、关务适用性判断 | [customs-compliance.md](./contexts/customs-compliance.md) |
| 全程追踪与异常（`visibility-exception`） | 全程追踪投影与标准追踪里程碑、ETA、异常信号与异常案件、客户可见异常与通知、客户索赔项、追偿事项 | [visibility-exception.md](./contexts/visibility-exception.md) |
| 小包计价（`parcel-pricing`） | 价卡及其版本发布、计价参考序列、纯价格评价及其解释、回放 | [parcel-pricing.md](./contexts/parcel-pricing.md) |
| 结算与经营核算（`settlement-accounting`） | 评价请求，把评价采用成费用与预期成本，对账单与费用争议，供应商账单与审核应付，外部资金事实的采用与核销，运营结算余额，接受前财务控制 | [settlement-accounting.md](./contexts/settlement-accounting.md) |
| 代收与清分（`collection-remittance`） | 代收货款本金：代收指令、代收事实、代收分户账、回汇批次、汇付主张、差异事项 | [collection-remittance.md](./contexts/collection-remittance.md) |
| 仓配履约（`warehouse-fulfillment`） | 可选能力，未实现，没有代码包；设计上拥有商品库存与库内任务 | [warehouse-fulfillment.md](./contexts/warehouse-fulfillment.md) |
| 接入与身份（`access-identity`） | 接入渠道登记、凭据验证、来源信封铸造。是共享技术能力，不是限界上下文，不拥有业务语言 | [access-identity.md](./contexts/access-identity.md) |
| 试点治理（`pilot-governance`） | 生产归属与准入控制的判断和治理记录。是治理能力，不是限界上下文，不拥有物流业务事实 | [pilot-governance.md](./contexts/pilot-governance.md) |

## 谁读谁

一行是一个消费方的非测试 Go 文件 import 了哪些别的上下文包。`platform` 与 `architecture` 不是上下文，不列。仓配履约没有代码，没有行。

| 消费方 | 读的提供方 |
|---|---|
| `accessidentity` | `pilotgovernance` |
| `pilotgovernance` | `accessidentity` |
| `partycommercial` | `accessidentity` |
| `parcelshipment` | `accessidentity`、`customscompliance`、`networkrouting`、`nodeoperations`、`parcelpricing`、`partycommercial`、`pilotgovernance`、`settlementaccounting`、`transportfulfillment` |
| `networkrouting` | `accessidentity`、`customscompliance`、`parcelpricing`、`parcelshipment`、`partycommercial`、`transportfulfillment` |
| `nodeoperations` | `accessidentity`、`networkrouting`、`parcelshipment`、`partycommercial`、`transportfulfillment` |
| `transportfulfillment` | `accessidentity`、`customscompliance`、`networkrouting`、`nodeoperations`、`parcelshipment`、`partycommercial` |
| `customscompliance` | `accessidentity`、`parcelshipment`、`settlementaccounting` |
| `visibilityexception` | `accessidentity`、`customscompliance`、`networkrouting`、`nodeoperations`、`parcelshipment`、`partycommercial`、`transportfulfillment` |
| `parcelpricing` | `accessidentity`、`parcelshipment`、`settlementaccounting`、`transportfulfillment` |
| `settlementaccounting` | `accessidentity`、`customscompliance`、`parcelpricing`、`parcelshipment`、`partycommercial`、`transportfulfillment` |
| `collectionremittance` | `accessidentity`、`settlementaccounting` |

读法：

- 每个业务上下文都读 `accessidentity`，因为来源信封只在那里铸造。
- 没有任何上下文读 `visibilityexception` 与 `collectionremittance`；`partycommercial` 不读任何业务上下文。
- 有几对是互相读的（如 `parcelshipment` 与 `networkrouting`、`parcelpricing` 与 `settlementaccounting`）。import 只说明消费方的代码引用了提供方的包，不说明事实朝哪边流。

## 跨上下文的几条规矩

- 限界上下文表达语言、规则、生命周期和数据所有权边界。它不等于微服务、独立进程、代码模块或数据库实例，也不是共享库表的许可。
- 任何实现都不得直接修改其他上下文拥有的数据。
- 提供方给出自己拥有的事实、请求或答复；消费方据此形成自己的判断，不取得对方对象的所有权，也不回写或修饰来源事实。
- 计划、承诺、已经发生的事实及其派生状态分别管理，任何一层不得静默覆盖另一层。
- 评价不是费用。小包计价只形成纯评价；结算消费评价，按自己的确认条件形成费用，不得回写评价。
- 客户委托、包裹、集运单元、申报单元和运输履约对象具有不同身份，不能用一个「大运单」同时表示。
- 代收货款本金属于代收与清分，不是物流收入、客户预付余额或普通结算账户资金。
- 技术组件（身份认证、消息等）不得取得领域对象的所有权，也不得成为绕过领域边界共享业务数据的通道。
