# 154 NR 复核入账未判之后，登记补齐或初始路由形成也没有续办

Category: enhancement
Status: needs-triage——2026-10-21 Claude Code 会话立（票 131 实施时按 ADR-0235 决定六另立；用户「要我裁定的部分，你作为业务和系统专家，请你自决吧」授权）
Blocked by: [demo-chain-gaps/14](./14-buy-request-stuck-after-an-unadoptable-evaluation.md)（同族续办形状先在 SA、PP 落定）
出处：[ADR-0235](../../../docs/adr/0235-route-reassessment-settles-what-a-retry-cannot-change-and-does-not-judge-triggers-older-than-the-current-position.md) 决定六与越权风险点 1；票 [131](./131-network-reassessment-undecided-is-split-by-reason.md)

## 现象

复核停在没有路由历史、证据未配置、冻结形态未配置三格时，票 131 之后入账未判，原因落 `network_routing.route_reassessment_not_judged`。之后租户登记了网络证据或策略版本的冻结形态，或者初始路由事后形成，这几次复核都不会自动重判。只有下一次控制触发会从它自己的位置判；同一封信封手工再投一次才会重判这一次。未判登记也没有读面，只能在库里查。

## 三类

机制（续办路径）。规模 M。租户取值：无。

## 最小的诚实修法

照票 14 落定的同族续办形状办，即 ADR-0094 决定四：登记动作发一类「参数已登记」信封，消费方按等待队列逐份重驱，不用定时扫描。

- 续办信封的来处：NR 网络证据与策略版本的登记口，以及初始路由形成。
- 复核侧按（租户，判断键）列出未判登记里原因属这三格、复核册里还没有结论的触发，逐份重驱。
- 重驱要拿回原触发的全部原料。交接一路按关联回到交接册；收寄采用一路回到 PS 采用记录与它的信封。地理投影不在 NR 落库（ADR-0075 决定三）。
- 「触发早于当前位置」是终局入账，不进续办。
- 重驱时照样先比当前位置：等待期间包裹已在更晚的位置判过的，这一次入账为早于当前位置。

## 要裁的

- 续办信封由哪几个登记口发，初始路由形成算不算一个。
- 收寄采用一路重驱时地理投影从哪里来：重读 PS 采用信封，还是经 ADR-0215 的消费侧适配器取受理基线那一版。
- 未判登记的读面（管理台列表、HTTP 读口）是否同票。

归 NR owner。

## 既有票

[14](./14-buy-request-stuck-after-an-unadoptable-evaluation.md)（同族，SA、PP）；[155](./155-initial-route-unconfigured-undecided-burns-the-acceptance-partition.md)（初始路由一路同病）。
