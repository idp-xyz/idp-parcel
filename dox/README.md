# idp-parcel 文档入口

取证于 main `1bdbc1b7b`（2026-11-12）。

`dox/` 是现行文档，规则以这里为准。`docs/` 与 `.scratch/` 自 2026-11-12 起冻结，是历史记录，只在追一条规则的来历时翻。

## 这个目录的规矩

1. **整份改写，不打补丁。** 规则变了就改那一句；来历看 `git log`。不写「补记」「更正」「此前」。
2. **一条规则只有一个家。** 在它所属上下文的那一个文件里。别处要用就链过去，不抄。
3. **不建子目录装决定和票。** 决定只在 [decisions.md](./decisions.md)，活只在 [work.md](./work.md)；做完的条目删掉。
4. **有行数上限。** `contexts/` 下每个文件不过 200 行，其余每个不过 100 行。超了是写多了，删，不拆文件。
5. **对着代码写。** 写进来的规则要在代码里找得到对应；文档走在代码前面的，单列在该文件末节。

## 文件

| 文件 | 回答什么 |
|---|---|
| 本页 | 现状，以及每一块看哪 |
| [product.md](./product.md) | 产品卖给谁、首发范围、主链与切片、机制／产品策略／租户取值三类 |
| [context-map.md](./context-map.md) | 上下文之间谁拥有什么、谁读谁 |
| [contexts/](./contexts/) | 每个上下文一个文件：管什么、词、取值、规则、生命周期、入口，以及文档走在代码前面的 |
| [decisions.md](./decisions.md) | 难逆转的取舍，每条一段（目前只有小包计价的几条） |
| [work.md](./work.md) | 在做、待做、上线前要核的、搁置 |
| [tenant.md](./tenant.md) | 一个租户上线要提供什么、登在哪个口、怎样算可以上生产 |

各文件的规则句提炼自旧文档并对过代码里有对应的机制，没有逐条跑测试验证行为；每个文件开头写明了它核到什么程度。

## 一句话

上线线已于 2026-10-28 达成；上线前与限量生产前必须做的票都已在 main。离第一家租户上线，产品这边没有欠着的代码，差的是租户取值与 PN-08 `Go`。现在唯一在做的是客户要求清单那一批（合同、价卡、计费、报价）。

## 现在按什么规矩干活

[AGENTS.md「交付模式」](../AGENTS.md#交付模式)：只做三种——冻结清单里必须做的、真实租户撞上的、用户点名的。

## 各切片做到哪

「机制半边」一栏照抄[开发主线「机制半边现状」](../docs/product/PARCEL-NETWORK-FIRST-RELEASE-DEVELOPMENT-BASELINE.md#机制半边现状)的定级；那张表之后的更正写在它下面的各条补记里，本页不复述。实例半边（某个租户能不能上生产）八个切片都还没有，只来自[参数登记册](../docs/product/PILOT-PARAMETER-REGISTER.md)的证据与 PN-08 `Go`。

| 切片 | 是什么 | 主责上下文 | 机制半边 |
|---|---|---|---|
| PN-01 | 范围、责任与治理基础 | `pilotgovernance` | 达标 |
| PN-02 | 商业权威、生产归属、委托与包裹身份 | `party-commercial`、`parcel-shipment`、`network-routing` | 达标（留待项已认可） |
| PN-03 | 接受后路由、收寄与节点作业 | `network-routing`、`node-operations`、`transport-fulfillment` | 达标（留待项已认可） |
| PN-04 | 取消与处置、运输履约、交付与终局 | `transport-fulfillment`、`parcel-shipment` | 达标 |
| PN-05 | 关务合规控制 | `customs-compliance` | 达标 |
| PN-06 | 追踪、异常与客户披露 | `visibility-exception` | 达标（留待项已认可） |
| PN-07 | 计价、账单与运营结算 | `parcel-pricing`、`settlement-accounting` | 达标（留待项已认可） |
| PN-08 | 端到端试点与阶段准入 | `pilotgovernance` | 达标；真实候选与证据包等租户 |
| PN-09 | 仓配履约（可选，首发不含） | `warehouse-fulfillment` | 用例已写，main 上无生产代码；已搁置 |

## 一条规则现在是什么，看哪

每个上下文一个文件，只看它。每个文件开头写明核到什么程度，末节列出旧文档写了而代码里还没有的。要追一条规则当初为什么这样定，才去翻 `docs/` 下的旧 `CONTEXT.md`、`UC-*` 与 ADR。

| 上下文 | 文件 |
|---|---|
| 参与方与商业 | [contexts/party-commercial.md](./contexts/party-commercial.md) |
| 小包托运 | [contexts/parcel-shipment.md](./contexts/parcel-shipment.md) |
| 网络与路由 | [contexts/network-routing.md](./contexts/network-routing.md) |
| 节点作业 | [contexts/node-operations.md](./contexts/node-operations.md) |
| 运输履约 | [contexts/transport-fulfillment.md](./contexts/transport-fulfillment.md) |
| 关务与贸易合规 | [contexts/customs-compliance.md](./contexts/customs-compliance.md) |
| 全程追踪与异常 | [contexts/visibility-exception.md](./contexts/visibility-exception.md) |
| 小包计价 | [contexts/parcel-pricing.md](./contexts/parcel-pricing.md) |
| 结算与经营核算 | [contexts/settlement-accounting.md](./contexts/settlement-accounting.md) |
| 代收与清分 | [contexts/collection-remittance.md](./contexts/collection-remittance.md) |
| 仓配履约（可选，未实现） | [contexts/warehouse-fulfillment.md](./contexts/warehouse-fulfillment.md) |
| 接入与身份（共享技术能力） | [contexts/access-identity.md](./contexts/access-identity.md) |
| 试点治理 | [contexts/pilot-governance.md](./contexts/pilot-governance.md) |

上下文之间谁拥有什么、谁读谁，看 [context-map.md](./context-map.md)。

## 在做、待做、搁置

只在 [work.md](./work.md)。

## 离第一家租户上线还差

1. 一个真实租户的取值（服务商通道、账期等），登进[参数登记册](../docs/product/PILOT-PARAMETER-REGISTER.md)；产品在这些格上按设计答`未配置`。
2. PN-08 阶段评审给出 `Go`（[交接](../docs/design/pn-08-end-to-end-pilot-and-stage-admission-development-handoff.md)）。
3. [work.md](./work.md)「上线前要核的」那几条。

## 怎么维护

一笔进 main 的改动若让本页某一格变了，同一笔改这一格，并把顶上的 SHA 与日期换成当时的。
