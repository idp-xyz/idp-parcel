# 12 候选装配：产品—渠道映射到候选集合之间没有适配器

Category: enhancement
Status: resolved——装配器接上择优端口（`cb86027`），完成判据满足；生产可达（组合根与调用入口）不在本票范围，另立票承接，见 Comments 末条（2026-09-03，MCP-6）
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

- 2026-09-02 MCP-6：上条列的前两件已补测试，**两条都是回归测试而非红→绿**——行为在第二片
  的实现里就写好了（我又写多了）。照上一条的办法逐条变异坐实它们不空：忽略「未登记」那一
  判，第一条落到另一个错误上；把时点写死成区间内，第二条让到期映射继续供出两个渠道。两次
  都当场变红，随后还原。

  **时点落在有效期外交回零个候选而不是错误**，这一格是有意的：到期正是映射把渠道排除在新
  决定之外的方式，它对这个时点如实作过答。折成错误会让「这个时点没有可用渠道」与「装配没
  能进行」混成一格，而两者续办不同。用例把区间内与区间外两次对照摆在一起——只断言区间外为
  空时，一个恒返回空的实现也能过。

  **映射未登记与「登记了但没有候选」分成两格**同理：都交回零个候选，压成一格就答不出续办
  是去登记映射，还是改约束/换产品版本。票 `14` 的落选留痕要答的正是这一类。

- 2026-09-03 MCP-6：**收尾重核（取证于 `966c4ce`..`c25d145`）。** 上条之后还有两笔在本票名下
  落地而没回写票面：`21b0ef2`（择优编排 `application/select_channel_candidate.go`、
  `ports/channel_selection.go`、`domain/channel_selection.go`，剪掉比较器那条棘轮基线）与
  `4474ece`（成本取数适配器 `adapters/parcelpricing/cost_source.go`，剪掉批量评价口那条）。
  上条「尚缺接上生产调用路径」写在它们之前，已过期——但只过期了一半。

  **接线那笔留了一道缝，且三道门禁都量不到它。** 编排经 `ports.ChannelCandidateAssembly`
  传的是本上下文的 `ChannelSelectionQuery`（PS 侧引用），而装配器只收自己那套 PC 侧
  `ChannelCandidateQuery`——`*ChannelCandidateAssembler` **不实现**该端口，真装配器交不进
  `SelectChannelCandidateDeps.Assembly`。编排测试用替身一路绿，装配器测试用自己的查询一路绿，
  棘轮只量领域工厂的包外引用。成本取数适配器那侧有 `var _ psports.ChannelCandidateCostSource`
  的编译期断言，装配器这侧没有——缝恰好在没有断言的那一半。这是「不同的绿长着同一张脸」
  的又一面：两个都绿的包，合起来接不上。

  `cb86027` 补上：装配器改收 `ports.ChannelSelectionQuery`，`providerKeysOf` 在本适配器内把
  租户 / 商业范围 / 映射引用译成 `pcdomain` 的键——翻译只许发生在这一层（应用层不得导入
  party-commercial）；译不过去的查询在问任何协作方之前即停，单列 `ErrUntranslatableQuery`
  （与 `ErrUntranslatableAnswer` 方向相反）；`ChannelConstraintSource` 随之改收择优侧查询——
  约束是本上下文对自己客户的事实，本就该按本上下文的引用去问。`ChannelCandidateQuery` 删除，
  全仓只有本包与其测试引用过它，不拆别处调用点。

  测试两片。一：编译期断言 + 两个提供方读口**记录收到的键**——只断言实现了接口不够，译错
  一个字段照样满足接口。二：译不过去先拒、三个协作方零调用——这片**没经过红**（green 时
  顺序已带上），照本票前面的办法用一次变异（先问约束再翻译）坐实测试当场红在「约束 1 次」，
  随后还原。

  **验证（detached worktree 检出 `cb86027`）**：`gofmt -l` 空、`go build ./...` 与
  `go vet ./...` 退 0、`go test -count=1 ./...` 退 0；**未设 DSN，PG 用例 SKIP**（同刻
  `TestFreezeScopesAreInvisibleToEachOther -v` 为 SKIP）。本笔不涉 `.sql` 与 postgres 适配器，
  真库对它无可证之物，故不把「未接真库」算作缺口。

  **生产可达仍差两步，且都不在本票「做什么」里。** (1) 组合根：`cmd/parcel-api` 里整条面单
  渠道链——票 06 的编排、票 07 的出向端口、本票的编排与两个适配器——都没有装配点，取证
  于 `c25d145`（`cmd/` 下 `OperateLabel`、`NewChannelCandidate`、`outbound` 零命中）。
  (2) 调用入口：择优是运营端点还是面单交易编排的前置步，是产品流程决定；票 06 的编排「只在
  提交之后留缝」没把择优放进去，票 `14` 的落选留痕又会改择优结果的形状。这两步应另立一票承接
  整条链的组合根与入口，不在本票硬造一个端点。本票完成判据（装配有适配器与测试、能从映射产出
  候选、四项门禁绿）至此满足，转 resolved；差的那两步如实记在这里，不算进本票。

- 2026-09-03 MCP-6：补上一条里两个候选入口各自的代价，供 owner 裁是否另开票（MCP-5 所要）。
  **甲 · 运营端点**（如一个择优查询口）：要新增传输层 + 端点表/探针/放行表三处共享接线 + Intake，
  而三个取数口（渠道约束、计价输入、`BUY` 价卡）今天全未配置，端点上线即恒答「未配置」；好处
  是择优可独立回放与审计，且不动票 06 的编排。
  **乙 · 面单交易编排的前置步**（在 `EstablishLabelTransaction` 之前择优）：要改票 06 编排的输入
  形状（渠道从调用方给改为编排选出），且必须先有票 `14` 的落选留痕，否则并列→冲突→人工裁决
  那条路在编排里没有落点；好处是不新增端点，生产可达一步到位。两条都不在本票内。

- 2026-09-10 · 通道 3（task-b5dba034，取证锚 `c7e3522c`；只写票面，未动代码，本票 Status 不改）：上两条留下的「组合根 + 调用入口」已立为 [`28`](./28-channel-selection-composition-root-and-call-entry.md)（draft，等 owner 裁甲 / 乙）。`28` 以上面两条 Comment 为代价分析的唯一权威，只补此后变了的三件（`14` 已 resolved、`23` 读面已进 `cmd/parcel-api`、三取数口未重核）与一段两路共有、这里没提的缺翻译：择优交回的 `ChannelCandidateID` 到票 `06` `EstablishLabelTransactionCommand` 七类依据引用之间没有适配器。
