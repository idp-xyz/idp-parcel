# 11 演示网络作为参考配置，经 psb/03 的采用路径进入演示租户

Category: enhancement
Status: draft
Blocked by: [psb/03](../../product-strategy-boundary/issues/03-reference-configuration-adoption-pattern.md)、08、10；另加 02 若登出「关务资格缺执行器」而另立的那张票
父票：[psb/04](../../product-strategy-boundary/issues/04-routing-product-strategy-first-cut.md)「演示网络作为参考配置」那一步
地盘：参考配置的存放处（psb/03 定）与演示种子；[合成演示动线](../../../docs/design/synthetic-demo-journey-script.md)对应一步。
出处：[ADR-0146](../../../docs/adr/0146-product-strategy-is-a-third-class-between-mechanism-and-tenant-values.md) 决定三、四、五；psb/04 完成判据。

## 做什么

1. 一份演示网络（节点、连接、线路、服务区域、日历与截单、选首个内置排序形态的路由策略版本、线路的 BUY 价卡引用）作为参考配置随产品发布；演示租户经 psb/03 的采用路径显式采用，未采用的租户照旧`未配置`。
2. 演示租户上一票已接受的委托形成初始路由（`S`）。

## 不做

- 不进参数登记册；不代任何真实租户采用。

## 完成判据

- [ ] 演示租户上一票已接受的委托形成初始路由，不再停在路由证据未配置。
- [ ] 采用记录的依据指向参考配置版本；演示数据全为 `SYN-` 合成值，证据只记 `S`。
