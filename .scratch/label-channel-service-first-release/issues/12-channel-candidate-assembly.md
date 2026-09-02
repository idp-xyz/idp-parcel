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

## Comments

- 2026-09-02 MCP-6：认领。先答票面点名要正面处理的那一问——**渠道约束今天没有登记处，
  但候选生成逻辑有**，两件要分开说。

  **有的那半**：`partycommercial/domain/service_product.go` 的 `ProductChannelMapping`
  带 `CandidatesAt`（某时点可用于新选择的渠道，落在有效期外一个都不列）与
  `CandidatesAllowedBy`（施加客户约束，只收窄不扩张）。这正是本票要的那段逻辑，已经
  写好且有测试。**但它零生产消费者**——适配器、端口与应用编排接的全是近名的另一个类型
  `ProductChannelMappingRegistration`（修订版本化映射册）。这一条不是本票发现的，
  `.scratch/mechanism-executor-triage/issues/04` 已逐条核过；本票据以确认落点。

  **没有的那半**：约束是 `CandidatesAllowedBy` 的一个纯参数
  （`allowed []ChannelProductReference`）。全仓无 `ChannelConstraint` 类型、无端口、
  无迁移。PC CONTEXT 与 CONTEXT-MAP 都写 `party-commercial` 拥有「可复用渠道约束」，
  那是**有语言无形状**的一处。按本票红线，如实记「无登记册可读」，不现造一个约束册。

- 2026-09-02 MCP-6：**上一条不只是缺件，它带着一个必须挡住的默认值。**
  `CandidatesAllowedBy` 在 `len(allowed) == 0` 时交回**全部**可用渠道。这对它自己是对的
  ——「客户没提约束」本就等于不收窄。但对装配是个陷阱：约束读不回来时若照样传空切片，
  得到的是「所有渠道都许可」，而那是**最宽**的答案，不是最保守的。一次读取失败会因此
  静默变成一次范围放大，且下游没有任何东西看得出来——它选出的是一笔真实的供应商采购。

  所以装配侧的约束口必须是三态而不是切片：`限定为这几个` / `明确无约束` / `未配置`。
  前两者才可进 `CandidatesAllowedBy`，第三者停下。这与本上下文既有的
  `CommercialBasisAdapter.keys`（`ResolutionKeySource` 允许为 nil，nil 即「显式未配置」，
  据以交回`解析未决`而不代拟一个范围）是同一手法，不是本票自创。

- 2026-09-02 MCP-6：落点取**适配器自己 join**，不加 PC 侧读口。三条路里它最轻且不碰提供方
  地盘：登记册交回的 `ProductChannelMappingRegistration` 只带产品的标识与版本号，而候选逻辑
  住在 `ProductChannelMapping` 上；适配器分别读登记册与发布册、拼出映射，再**调用**
  `CandidatesAllowedBy`。收窄规则因此仍在 PC 那一侧，本上下文没有第二套映射。

  取证：`CommercialPublicationView.LoadForScope` 交回 `*CommercialRegistry`，其
  `ServiceProducts()` 交得出已登记的 `ServiceProduct`，前提成立。

- 2026-09-02 MCP-6：三片已落，均在 `adapters/partycommercial/channel_candidate_assembly.go`。

  **一** 约束未配置即停（`ErrChannelConstraintNotConfigured`）。夹具刻意让映射真有两个渠道
  可放——映射为空时「停下」与「没有候选」在结果上分不开，那样的绿是假的。

  **二** 交回按约束收窄后的候选，并译成 `ChannelCandidateID`。约束里多点一个映射未提供的
  渠道，把两个方向的错一起钉住。

  **三** 映射指名的产品版本不在册即停（`ErrMappedServiceProductNotEffective`）。这一格来自
  一个动笔时才看清的事实：`CandidatesAt` / `CandidatesAllowedBy` 的实现**根本不读**
  `mapping.product`，所以 `NewProductChannelMapping` 要 `ServiceProduct` 是一条不变量而非
  计算需要——而那条不变量正是「退役产品不再产新候选」的守卫。绕过它本可以省掉读发布册
  那一步，代价是让一份已收尾的商业决定重新参与新的采购。

  **两处如实记**：第三片**没经过红**（第二片的实现已提前满足它，我预判了下一片），已用
  一次变异坐实它有牙——查找时只比对象标识不比版本号，v2 的映射会挂到 v1 的产品上并照常
  产出候选，测试当场变红，随后还原。另：`ConstrainedToChannels` 拒空列表，因为「点名了零个
  渠道」与「没有约束」在取值上分不开而后果相反。

- 2026-09-02 MCP-6：本票**未完**。已落的是装配主路径与两道门；尚缺至少：映射未登记那一格
  （`ErrProductChannelMappingNotRegistered` 已定义但无测试）、时点落在映射有效期外、以及
  接上生产调用路径。验证：本包 `go test` 绿、`gofmt -l` 空。全仓此刻有红，但红在
  `internal/transportfulfillment/`（另一会话的在途未跟踪文件），与本票无关。
