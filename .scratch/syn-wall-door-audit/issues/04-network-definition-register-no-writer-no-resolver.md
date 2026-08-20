# 网络定义登记册无写入方也无解析层,路由证据墙双缺件

Category: enhancement
Status: needs-triage

来源:SYN-WALL-DOOR-AUDIT 走通审计(基线 `49a2ab0`),对应清单 W09。票面点名的三疑似无门之一。

## 墙

`NETWORK_EVIDENCE_NOT_CONFIGURED`(`assess_parcel_reachability.go`)、`ROUTE_EVIDENCE_NOT_CONFIGURED`(`create_initial_route.go`)、`REASSESS_EVIDENCE_NOT_CONFIGURED`(`reassess_route.go`);另有 `ErrNetworkDefinitionUnresolvable`(`networkrouting/adapters/postgres/network_definition.go`)。

## 现状:门框在,门与楼板都不在

- 表 `network_routing.network_definition`(迁移 0007)与读口 `NewNetworkDefinitions` 就位,已接调度器两条链。
- 写入方:零——读口注释自证「今天本表没有写入方,因此生产上恒答`未配置`」。
- 解析层:零——即便登记了定义,本构建也产不出九族事实(候选生成、过滤与排序,PAR-NET-14 机制半边),读口按 ADR-0053 响亮上抛 `ErrNetworkDefinitionUnresolvable`,不退成未配置也不退成空事实。

## 缺的最小机制件

1. 网络定义登记口 + 写入方:定义原语的模式设计(节点/连接/线路/服务区域/日历/截单,按 CONTEXT 的所有权词表)与版本化登记用例。读口注释已言明「模式留待真有定义可登时再设计」——本票就是那一步。
2. 解析层:从已登记定义推导逐判断事实族(可达性三格、初始路由候选、复核证据)。

两件可分票执行,先 1 后 2;只有 1 没有 2 时读口仍响亮报 unresolvable,不许静默。

## 红线

- PAR-NET-01..15 的实例值(真实线路/节点)待提供是常态;本票只建门与解析机制,验证用 SYN-* 合成定义,S 级只记 S。
- 不得为纵向变绿在生产装配里种网络定义(装配点注释已禁)。

## 参照

ADR-0052、ADR-0053;PAR-NET-14;`docs/domain/network-routing/CONTEXT.md`。
