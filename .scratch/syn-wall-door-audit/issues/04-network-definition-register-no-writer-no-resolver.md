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

## Comments

- 2026-08-20 · MCP-3：对 3324ecb 重核四件（只读）。**票面大幅过时——「缺的最小机制件」
  第 1 件的模式设计半边已由 NR-CATALOG-MECH 落地（3b9f212，2026-08-20，ADR-0068）。**
  仓储表：**已有**——`migrations/network_routing/0008_network_catalog.sql` 七表骨架（节点/
  连接/线路/服务区域/服务日历/路由策略六类稳定定义 + 临时可用性调整 + 目录修订锚）；
  服务区域与日历的**内容列刻意未定**（0008 头注：属 PAR-NET-14，形态定了再以新迁移扩列）。
  装载口：**已有**——`NetworkCatalog.LoadDefinitionsAt`（单语句单快照、未配置与空目录
  分格、两版同时适用报 `ErrAmbiguousNetworkCatalog`）。写入方：**适配器级已有**——七个
  `Register*` 方法带修订锚同事务推进，但调用方只有测试；**应用层登记用例与进程级登记口
  仍零**（`NewNetworkCatalog` 未接 cmd）。0007 的 `network_definition` 登记表本身仍无写入
  方（读口注释原话未变，0f266be），三个证据视图仍只读 0007 恒答未配置——「解析层存在前
  三口取数侧不接」的护栏在 0008 头注与 `network_catalog.go` 注释两处钉着。解析层：仍零
  （设计上后置：候选生成/过滤/排序属 PAR-NET-14）。
  建议：票面按上述改写后 ready-for-agent，范围收敛为「登记**用例** + 进程级登记口
  （含 0007 登记行随目录登记如何形成——两表合流口径属解析层设计，先只登目录）」；
  解析层与内容列被 PAR-NET-14 阻断，另票另裁，勿并入。
- 2026-08-20 MCP-1：重核属实，票面先改写再 ready；本轮不派实现。解析层/内容列另票。
