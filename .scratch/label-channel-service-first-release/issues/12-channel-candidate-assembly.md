# 12 候选装配：产品—渠道映射到候选集合之间没有适配器

Category: enhancement
Status: ready-for-agent
Blocked by: 01（已 resolved：择优取乙落 `parcel-shipment`，装配随之落该上下文的 `adapters/partycommercial/`）

## 缺口

按 [ADR-0088](../../../docs/adr/0088-label-channel-service-enters-the-first-release-service-forms.md)
Consequences，渠道候选来自**产品—渠道映射与渠道约束**，那两样在 `internal/partycommercial`
（`ports.ProductChannelMappingRegistry`、`ports.ProductChannelMappingCatalogueRead`）。

而比较器 `internal/networkrouting/domain/route_ranking.go` 的 `SelectRouteCandidate` 收的是
**已经装好的** `[]RouteCandidate` 与 `[]CandidateScores`。**中间那一段没有任何适配器**：
没有东西从产品—渠道映射生成候选集合。

## 做什么

补候选装配：从产品—渠道映射与渠道约束读出可用渠道，装成择优所需的候选集合。落点由 `01`
裁定的归属决定（择优住哪个上下文，装配就落在那个上下文的 `adapters/partycommercial/`）。

一件要正面处理的事：**渠道约束**今天在哪里、是不是已有形状。若发现约束根本没有登记处，
如实记「无登记册可读」并回票面，不为了让装配跑通而现造一个约束册。

## 红线

- 不填任何候选内容、渠道账号、映射取值（实例半边）。
- 只引用 `partycommercial` 的读口，不在本上下文复制第二套映射。
- 领域包不依赖 HTTP/`pgx`。

## 完成判据

装配有适配器与测试，能从产品—渠道映射产出候选集合；`gofmt -l` 空、`go build`/`go vet` 退 0、
`go test -count=1 ./...` 绿并注明含不含真库。

## 参照

[能力形状盘点](../capability-shape-inventory.md)第三段；票 `01`；ADR-0088 Consequences。
