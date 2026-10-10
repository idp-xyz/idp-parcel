# 15 候选标识「线路@版本」与成本适配器按「/」切不一致：计价输入一接上，初始路由就落成本来源不可用

Category: bug
Status: in-progress——2026-10-10 21:0x 通道 1 认领（用户令通道 1 自己完成：通道 3 在派单前已 crash，`task-33e8ef5a` 未执行）；分支 `mcp1-rfc15`，基 `1d67e27c`，隔离工作树 `/home/tops/workspace/idp-parcel-mcp1-rfc15`。此前：ready-for-agent——2026-10-10 通道 1 立（用户授权自决），出自 [11](11-demo-network-adopted-as-reference-configuration.md) 完工报里通道 2 的探针实测（探针未入库）
Blocked by: 无逻辑依赖——铸标识的一侧随 [09](09-initial-route-evidence-folded-from-catalog.md)、切标识的一侧随 [10](10-candidate-cost-from-leg-buy-evaluations.md)，两侧在 main 上都已存在
归档：不属 [psb/04](../../product-strategy-boundary/issues/04-routing-product-strategy-first-cut.md) 的子票集；放在本目录是因为它是 09 与 10 之间的缝。
地盘：`internal/networkrouting` 里候选标识的铸造与解析两侧及其用例。

## 现象

- 目录折叠给候选铸的标识形如 `线路@版本`（`catalog_network_evidence.go` 的 `versionReference`）。
- `RouteCandidateCostAdapter.resolveLegs` 按 `/` 切这个标识找线路。
- 通道 2 用探针把计价输入接上后实测：`untranslatable pricing evaluation: candidate "SYN-LINE-CN-SG-01@1" carries no line reference`，初始路由落 `COST_SOURCE_UNAVAILABLE`。
- 今天碰不到，是因为 dispatch 的计价输入仍是 `unconfiguredRoutePricingInput{}`，初始路由先停在 `COST_SOURCE_NOT_CONFIGURED`；10 的用例手造 `线路/1`，没经过真正的铸造路径。能编过、语义不对，测试全绿。

## 做什么

1. 候选标识的格式只在一处定义：铸与解共用同一个类型或同一对函数，不再各自约定分隔符。改哪一侧、用字符串还是结构化引用，按 NR 现有约定定，写进判断项。
2. 补一条经真实铸造路径的用例：目录折叠铸出的候选直接交成本适配器，能解出线路。放回旧写法（一侧 `@`、一侧 `/`）时它必须红。

## 不做

- 不接计价输入本身，那是 [17](17-initial-route-pricing-input-from-customer-declaration.md)。

## 完成判据

- [ ] 格式一处定义，两侧共用。
- [ ] 经真实铸造路径的用例绿；放回旧写法红，写明怎么证的。
