# 网络解析层——把目录折成逐候选事实

Category: enhancement
Status: ready-for-human

演示动线三堵墙的**墙三**，也是本批唯一一件真正在长的工程。取证基线 `c9835bf`。

## 墙

初始路由停在 `RouteEvidenceNotConfigured`（`create_initial_route.go`），可达性停在 `NetworkEvidenceNotConfigured`（`assess_parcel_reachability.go`），复核停在 `ReassessEvidenceNotConfigured`（`reassess_route.go`）。

成因是两张表之间没有桥：三个证据视图只读 `network_routing.network_definition`（迁移 `0007`），而那张表**零生产写入方**；演示种子灌的是迁移 `0008` 的七族目录版本表，`bumpRevision` 推的是目录修订锚，长不出 `0007` 的行。

目录那半边已经通了——`.scratch/syn-wall-door-audit/issues/04-network-definition-register-no-writer-no-resolver.md` 交付了登记用例 `register_network_catalog.go` 与进程口 `cmd/parcel-network-register`，并在票面预告「本票不降墙」。墙面状态因此是「无门（解析层缺）」。

## 做什么

把目录折成逐候选事实：候选生成、资格过滤、事实装配，交给三个证据视图能用的形状。

**这是全批最需要先设计再动手的一件，开工前必须先答三个问题：**

1. **`0007` 与 `0008` 的合流口径。**`network_definition` 那一行应随目录登记自动形成，还是另有登记路径？票 04 明确把这一问留给了解析层设计，没有替它定。
2. **`PAR-NET-14` 到底挡住了什么。**票 04 与 [ADR-0068](../../../docs/adr/0068-versioned-network-catalog-structure-precedes-rule-content.md) 都把解析层记为被 `PAR-NET-14` 阻断。但首发的准则集合其实已由 `PAR-NET-16` 定死为**成本单维**，而 `RankingCriterion` 本就是开放引用、准则由策略版本声明。**要分清被挡的是「候选怎么生成」还是「候选怎么排序」**——如果只是后者，解析层的前半段今天就能做。这一问答错会让整票要么白等、要么撞穿护栏。
3. **要不要改 ADR-0068。** 它的 Decision 六明写「三个证据视图仍不读本目录」，护栏钉在 `0008` 头注与 `network_catalog.go` 的类型注释两处。解析层落地那一刻这条护栏按定义要动。**同笔改 ADR（新 ADR 或 supersede），不留「代码已读、ADR 仍说不读」的中间态**——它的 Context 会在决定落地那一刻失效，这正是 AGENTS.md 点名 ADR 尤其要守的那一类。

三问答完再拆实现票。答案落在本票 `## Answer` 段。

## 必须守住的一格

**服务区域的地理覆盖、服务日历的营业日/服务窗口/截单、路由策略的规则正文在 `0008` 里刻意一列未建**（头注：属 `PAR-NET-14`，形态定了再以新迁移扩列）。解析层不得为了凑出候选而给这些列填形状——缺哪一格就让候选在那一格上如实不可用，按 ADR-0029 的取数失败代数分格答；`ErrNetworkDefinitionUnresolvable` 不得退成`未配置`或空事实（ADR-0053 第四条第三格）。

## 完成判据

三个证据视图之一能从合成目录产出逐候选事实并让初始路由形成计划；`ErrNetworkDefinitionUnresolvable` 那条响亮上抛的分支保持原语义；ADR 同笔更新；`gofmt -l` 空、`go build`/`go vet` 退 0、`go test -count=1 ./...` 绿并注明含不含真库。

测试值全 `SYN-` 合成，`S` 只记 `S`。

## 参照

`internal/networkrouting/adapters/postgres/network_catalog.go` 与 `network_definition.go`；`internal/networkrouting/application/` 的三个证据消费点；迁移 `0007_network_definition`、`0008_network_catalog`；ADR-0068、[ADR-0053](../../../docs/adr/0053-network-fact-families-are-derived-not-registrable.md)、[ADR-0052](../../../docs/adr/0052-network-evidence-catalogue-has-an-unconfigured-grade.md)、[ADR-0029](../../../docs/adr/0029-retrieval-failure-algebra-is-partitioned-by-recovery-action.md)；`PAR-NET-14`、`PAR-NET-16`；`.scratch/syn-wall-door-audit/issues/04` 与票 05。

**跨批依赖**：`label-channel-service-first-release` 的票 `13`（`BUY` 评价到成本准则分值的桥）与票 `01`（平局裁决）。本票产出候选、那两票给候选算分与定序，端到端出计划三件缺一不可。开工时按当时状态与那一批对齐，不重复立票。
