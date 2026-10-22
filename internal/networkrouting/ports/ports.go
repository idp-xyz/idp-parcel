// Package ports 声明 network-routing 自有的语义边界。判断库、计划适用性、交接登记册、
// 计划版本签发、两个发布意图与版本化网络目录都已有 PostgreSQL 适配器；网络证据视图
// （可达性与初始路由）由应用层从目录折出（候选生成与折叠是产品策略，ADR-0146 决定七、
// ADR-0148），目录内容是租户取值。
//
// 不得替租户拟一份网络定义：证据视图不带任何默认网络，目录为空或没有适用的路由策略
// 版本时如实答`未配置`（ADR-0052、ADR-0148 决定六）——那不是默认实现，恰恰是这条禁令
// 想保护的东西。
package ports

import (
	"context"
	"errors"
	"time"

	"go.idp.xyz/idp-parcel/internal/networkrouting/domain"
)

// NetworkEvidence 是一次判断所需的版本化网络事实与它们共同来自的那个视图修订。
//
// 端口只取事实不做评估（ADR-0046）：区域明确排除折成淘汰、地址缺信息折成资料不足，这些
// 是领域拥有的评估规则，落在端口后面就落到了适配器手里，而适配器只翻译不判断。事实与
// 修订一次取回而不分多次调用：两次取回之间视图一变，事实与它标的修订就不再来自同一版。
type NetworkEvidence struct {
	ServiceAreas      []domain.ServiceAreaResolution
	RouteRequirements []domain.RouteRequirement
	PathExecutability []domain.PathExecutability
	HardConstraints   []domain.HardConstraintFinding
	// CustomsCitations 是关务事实逐候选的出处（ADR-0148 决定一），随判断记录一并留痕。
	CustomsCitations []domain.CustomsApplicabilityCitation
	ViewRevision     domain.NetworkViewRevision
}

// RequestCarriedContent 是随判断请求携带、由发起方交来的判断对象内容（ADR-0148 决定一「随请求携带」一路，
// ADR-0075 同款，判据是同版性）：它是判断对象那一版的内容，本上下文只用它判断，不回读、不保存本体。现含地理解析
// 投影与承诺上界，ADR-0148 决定一同列的服务要求还不在其中。零值即发起方什么都没带。
type RequestCarriedContent struct {
	Geo        domain.GeoResolutionProjection
	Commitment domain.CommittedTimeBound
}

// NetworkEvidenceView 为一次判断取回版本化网络事实。
//
// 三格，缺一不可（ADR-0052）：事实在场即证据；第二个返回值为 false 即**这个判断范围
// 未配置**——租户的目录修订锚不存在，或判断时点没有适用于该服务目的的路由策略版本
// （ADR-0148 决定六）；error 只表示依赖调不通、超时，或目录登记了而解不出这类故障。
//
// 三格分开是因为恢复动作两两不同：未配置要租户去登记网络定义，依赖不可用要运维去救那个
// 依赖，而事实齐备才轮到领域评估。**空册绝不折成空证据**——领域会照常评估并得出`无当前
// 有效路由`，那是从缺配置里编出一个业务结论，用例明写它与`未决`不能合并。反向同样禁：
// 把技术故障装扮成一个证据缺口，会让它进入`资料不足`统计。
//
// carried 是随请求携带的内容；视图修订只标目录那一路，不随所携内容变。
type NetworkEvidenceView interface {
	LoadNetworkEvidence(
		ctx context.Context,
		key domain.ReachabilityJudgmentKey,
		carried RequestCarriedContent,
	) (NetworkEvidence, bool, error)
}

// CommercialEligibilityView 取商业侧对「这个服务要不要判断网络可达性」的回答，覆盖用例
// 步骤 4 的产品形态、责任法人、合同约束与网络使用资格。
//
// network-routing 只消费这个回答，绝不自行推导一个：产品、合同与责任法人都属
// party-commercial，在这里判一次就成了第二处定义，而两处口径迟早会分叉。
//
// 依赖调不通要作为错误返回，由应用层形成`未形成判断`。把它读成「不要求」会让一次商业侧
// 故障变成`不适用`，而用例明写不得以`不适用`代替其他结果，也不得虚构运营网络。
//
// 解析标识与判断键并列传入（ADR-0156）：它是本轮已采用的商业解析，不是判断维，不得并进 key。
type CommercialEligibilityView interface {
	AssessNetworkEligibility(
		ctx context.Context,
		key domain.ReachabilityJudgmentKey,
		resolution domain.CommercialResolutionReference,
	) (domain.NetworkEligibility, error)
}

// ReachabilityJudgmentRecord 是一次判断越过提交边界后留下的东西。判断时间不在
// `ReachabilityFinding` 里，因为它不是领域结论的一部分——`asOf` 决定按哪一刻的网络证据
// 评估，判断时间只说明这次判断何时作出，压成一个会让重放看起来像新判断。
//
// ViewRevision 同在记录而不在结论里：它是证据出处不是三值判断的一部分，留在记录上供
// 消费方比对「判断形成后视图有没有换代」（CONTEXT「保留……当前修订标识」）。
//
// GeoProjectionDigest 与 ServiceAreaVersions 是当次解析依据里除判断键以外的两件
// （ADR-0075 决定三）：所携投影的版本化内容摘要，以及这次用过的服务区域版本。地址本体不在记录上。
// CustomsCitations 是关务事实逐候选的出处（ADR-0148 决定一）：已接的关务来源逐候选带
// 出处；来源未接时缺席是如实形态，不是判断缺件。
type ReachabilityJudgmentRecord struct {
	Key                 domain.ReachabilityJudgmentKey
	Finding             domain.ReachabilityFinding
	JudgedAt            time.Time
	ViewRevision        domain.NetworkViewRevision
	GeoProjectionDigest string
	ServiceAreaVersions []string
	CustomsCitations    []domain.CustomsApplicabilityCitation
}

// ReachabilityJudgmentSaveOutcome 是保存一次判断的封闭写入结果。error 只留给「答不出」，
// 「已有记录」是一个业务答案（ADR-0031 的同一裁决）：`AT-NR-028` 只允许一个结果版本越过
// 提交边界，第二个写入方要按它读回赢家——而一个 error 分不出「库坏了」与「有人先到」，
// 前者该重试，后者重试一万次也还是有人先到。
type ReachabilityJudgmentSaveOutcome uint8

const (
	ReachabilityJudgmentSaveOutcomeInvalid ReachabilityJudgmentSaveOutcome = iota
	ReachabilityJudgmentSaved
	ReachabilityJudgmentAlreadyRecorded
)

func (outcome ReachabilityJudgmentSaveOutcome) String() string {
	switch outcome {
	case ReachabilityJudgmentSaved:
		return "SAVED"
	case ReachabilityJudgmentAlreadyRecorded:
		return "ALREADY_RECORDED"
	default:
		return ""
	}
}

// ReachabilityJudgmentStore 按请求关联找回并保存判断。租户是显式入参、不从 context 里
// 补，因为按 ADR-0003 运营集团租户是最高数据隔离边界，跨越它必须在签名上看得见。
//
// Save 对同一关联只接纳第一份记录；再来的写入答`已有记录`而不覆盖——迟到结果不按到达
// 顺序覆盖原判断（`AT-NR-028`）。
type ReachabilityJudgmentStore interface {
	FindByCorrelation(
		ctx context.Context,
		tenant domain.TenantID,
		correlation domain.RequestCorrelationID,
	) (ReachabilityJudgmentRecord, bool, error)
	Save(
		ctx context.Context,
		correlation domain.RequestCorrelationID,
		record ReachabilityJudgmentRecord,
	) (ReachabilityJudgmentSaveOutcome, error)
}

// ReachabilityJudgmentHandoffIntent 是一次已提交判断交给发起方一侧适用下游的那份引用。
// 意图由请求关联认领：同一判断无论交几次都是同一份，不是第二份。
type ReachabilityJudgmentHandoffIntent struct {
	Correlation  domain.RequestCorrelationID
	Key          domain.ReachabilityJudgmentKey
	Finding      domain.ReachabilityFinding
	JudgedAt     time.Time
	ViewRevision domain.NetworkViewRevision
}

// ReachabilityJudgmentHandoff 把一份已提交的三值判断交给适用下游——`AT-NR-030` 的意图
// 半边，发布意图这条缝的第三个样本（ADR-0043）。
//
// 本上下文不记意图完没完成：那份状态要与判断同一事务落库才算数。入队这一步已由
// OutboxReachabilityHandoff 承担（意图与判断同一事务），但「完没完成」仍然不记——重放
// 一律重发同一意图，由下游按请求关联认领。
type ReachabilityJudgmentHandoff interface {
	HandOffReachabilityJudgment(ctx context.Context, intent ReachabilityJudgmentHandoffIntent) error
}

type Clock interface {
	Now() time.Time
}

// CandidatePath 是一个候选的路径内容：被选中后成为计划的段链。段链属候选事实——网络
// 定义拥有节点与连接，领域只校验链形不发明节点。
//
// Legs 只放节点间段，与线路的连接逐一对应；交付段另放 Delivery（ADR-0217）。分开放是因为候选
// 成本按线路段号取成本依据：交付段没有成本依据，混进 Legs 会让每个候选都停在待判断。
type CandidatePath struct {
	Candidate   domain.CandidateID
	Legs        []domain.PlannedLeg
	Delivery    domain.PlannedLeg
	HasDelivery bool
}

// PlanLegs 交回被选中后成为计划段链的那一串：节点间段，之后有交付段就接上它。
func (path CandidatePath) PlanLegs() []domain.PlannedLeg {
	legs := append([]domain.PlannedLeg(nil), path.Legs...)
	if path.HasDelivery {
		legs = append(legs, path.Delivery)
	}
	return legs
}

// InitialRouteEvidence 是一次初始路由判断所需的版本化事实（UC-NR-001 层次 1–4 的输入
// 清单）。与可达性证据同一条 ADR-0046 纪律：端口只取事实不做评估，事实与修订一次取回。
// 日历与缓冲取值属租户取值。
//
// RankingForm 是 Strategy 所指那一版声明的排序形态，零值即该版未声明；CandidateCosts 按
// 形态要的维度给出逐候选事实，每个合格候选恰一条——待判断与不可计价各有一格，不以缺席表达。
// 证据视图不折 CandidateCosts：它经端口取（ADR-0148 决定一），由编排向 RouteCandidateCostSource 要来再填上，初始路由
// 与复核同一口。
type InitialRouteEvidence struct {
	ServiceAreas      []domain.ServiceAreaResolution
	RouteRequirements []domain.RouteRequirement
	PathExecutability []domain.PathExecutability
	HardConstraints   []domain.HardConstraintFinding
	// CustomsCitations 与可达性证据同一形状：关务事实逐候选的出处，随初始路由记录留痕。
	CustomsCitations []domain.CustomsApplicabilityCitation
	Projections      []domain.CandidateTimeProjection
	CommittedBound   domain.CommittedTimeBound
	RankingForm      domain.RankingForm
	// FreezeForm 零值且 FreezeRemainingSegmentLimit 为 nil 即这一版没有声明冻结形态。
	// 未声明不是未冻结。
	FreezeForm                  domain.FreezeForm
	FreezeRemainingSegmentLimit *int
	// AutoRerouteForm 零值或阈值为 nil 即这一版没有声明自动改路。未声明不是允许，也不是不允许。
	AutoRerouteForm                      domain.AutoRerouteForm
	AutoRerouteImprovementThresholdMinor *int
	// 比较币种与所引价格政策（ADR-0148 决定四）是路由策略版本上的租户取值：缺格如实交回，
	// 取数侧只在各段同币种时合成，异币种进未决格——不写死任何默认币种。
	HasComparisonCurrency       bool
	ComparisonCurrency          string
	HasComparisonPricePolicy    bool
	ComparisonPricePolicy       string
	UnresolvedRestrictions      []domain.RestrictionReference
	OutstandingResponsibilities []domain.ResponsibilityReference
	CandidateCosts              []domain.CandidateCostFact
	Paths                       []CandidatePath
	Strategy                    domain.RouteStrategyReference
	ViewRevision                domain.NetworkViewRevision
}

// InitialRouteEvidenceView 为一次初始路由判断取回版本化事实。三格语义同
// NetworkEvidenceView（ADR-0052）：事实、`未配置`、依赖不可用，各自对应不同的恢复动作，
// 应用层为前两者分设未决原因。空册走第二格，绝不折成一份空证据。
type InitialRouteEvidenceView interface {
	LoadInitialRouteEvidence(
		ctx context.Context,
		key domain.InitialRouteJudgmentKey,
		carried RequestCarriedContent,
	) (InitialRouteEvidence, bool, error)
}

// ErrRouteCostSourceNotConfigured 是候选成本取数侧整条取数路径没接时的如实答复，编排据它形成
// `COST_SOURCE_NOT_CONFIGURED`：没接就没有依据，不得编一份空输入让每个候选都算出一个看着合法的价格。
// 缺输入不走这一格：只缺某一段的区域，那一段待判断；缺包裹事实，各段都待判断。候选随之缺成本依据。
var ErrRouteCostSourceNotConfigured = errors.New("network routing: route cost source not configured")

// ErrRouteCostAnswerUnusable 说取数侧交回或读到的东西本桥用不了，而且重投多少次都一样：提供方交回认不出的形状、比较
// 币种或方案引用译不出、方案的方向或用途答的不是这一段的成本、候选没有线路引用、包裹事实立不成。它与依赖读不回分格
// （ADR-0029：恢复动作不同必须分）：读不回等依赖恢复、重投推得动；这一格要有人改登记或改代码，重投只会把同一个错再答
// 一遍、空占着分区。编排见它响亮报错，不折成任何一格未决。
var ErrRouteCostAnswerUnusable = errors.New("network routing: route cost answer is unusable")

// RouteCandidateCosts 是取数侧为一次判断交回的成本族：逐候选三态事实 + 逐候选逐段评价出处。
type RouteCandidateCosts struct {
	Facts     []domain.CandidateCostFact
	Citations []CandidateLegCostCitation
}

// CandidateLegCostCitation 是一条候选某一段的成本出处。出处本身只带段号，候选要在这里另带：取数侧交回时还不知道哪一条
// 会被选中（排序在编排里），而计划只留选中候选那几段、段号按那条候选的段链计。不带候选，几条都出价的候选各自的第 1 段
// 是同一个段号，计划形成必拒（票 demo-chain-gaps/105）；评价标识由 parcel-pricing 派生，从它也认不出候选。
type CandidateLegCostCitation struct {
	Candidate domain.CandidateID
	domain.PlannedLegCostCitation
}

// CitationsOf 交回一条候选的逐段出处，段序照取数侧交回的。没出价的候选没有出处，答空。
func (costs RouteCandidateCosts) CitationsOf(candidate domain.CandidateID) []domain.PlannedLegCostCitation {
	var citations []domain.PlannedLegCostCitation
	for _, citation := range costs.Citations {
		if citation.Candidate == candidate {
			citations = append(citations, citation.PlannedLegCostCitation)
		}
	}
	return citations
}

// RouteCandidateCostSource 为一次初始路由判断取回候选成本（routing-first-cut/10，ADR-0148
// 决定四）。事实族在取数侧兑现：逐段向 parcel-pricing 取评价、按比较币种不舍入求和取整一次，
// 合成每候逐一格三态事实；出处（评价标识、方案引用、内部政策引用与全精度比较金额）逐条标明
// 候选、随事实交回，编排选出候选后只把选中那一条的出处并入计划留痕。整条取数路径没接时以
// ErrRouteCostSourceNotConfigured 如实作答；缺某一段的输入只让那一段待判断，不整判断停下。
//
// carried 是随这次判断请求携带的内容，与证据视图收到的是同一份：绑了分区目录的卡要包裹的邮编
// 路线才解得出区域（ADR-0189 决定一），路线只在请求里，本上下文不另开读地址的口（ADR-0075）。
//
// 读失败不得连累调用方的事务：编排在判断那一笔事务里调它，读失败之后还要照常落别的册——复核失效线上的失效与复核记录、
// 仍适用线上的仍适用，初始路由那一件的提交前重校也还要读证据。取数侧的仓储读口在事务里走同一笔事务，所以生产装配给它
// 包一层 savepoint（postgres.SavepointedRouteCandidateCosts）；编排只据错误落格，不碰事务。
type RouteCandidateCostSource interface {
	LoadCandidateCosts(
		ctx context.Context,
		key domain.InitialRouteJudgmentKey,
		evidence InitialRouteEvidence,
		carried RequestCarriedContent,
	) (RouteCandidateCosts, error)
}

// RoutingApplicabilityView 取商业侧对「这个服务要不要形成网络路由」的回答（UC-NR-001
// 步骤 3）。复用 NetworkEligibility 语义：要求/不要求加依据，读不回是错误不是`不适用`。
// 解析标识与判断键并列传入（ADR-0064）：它不是判断维，不得并进 key。
type RoutingApplicabilityView interface {
	AssessRoutingApplicability(
		ctx context.Context,
		key domain.InitialRouteJudgmentKey,
		resolution domain.CommercialResolutionReference,
	) (domain.NetworkEligibility, error)
}

// InitialRouteRecord 是一次包裹级初始路由判断越过提交边界后留下的东西：计划或无路由
// 二居其一。两个都带或都缺的记录是坏数据——那正是「无路由不得用空计划表达」的存储面。
//
// RecordedAt 是判断落库的时刻，只由读回填充（Save 由库面 DEFAULT 承担）。消费方要用
// 它当接收时间——业务发生时间、有效时间与接收时间分别保存，禁止拿信封时间顶替。
type InitialRouteRecord struct {
	Key        domain.InitialRouteJudgmentKey
	Plan       domain.InitialRoutePlan
	HasPlan    bool
	NoRoute    domain.NoCurrentRouteJudgment
	HasNoRoute bool
	RecordedAt time.Time
}

// InitialRouteSaveOutcome 与可达性判断库同一套写入代数（ADR-0031）：`已有记录`是业务
// 答案不是错误，第二个写入方按它读回赢家（`AT-NR-004` 并发裁决）。
type InitialRouteSaveOutcome uint8

const (
	InitialRouteSaveOutcomeInvalid InitialRouteSaveOutcome = iota
	InitialRouteSaved
	InitialRouteAlreadyRecorded
)

// InitialRouteStore 按判断键找回并保存包裹级结果。键含接受基线与服务目的——「同一接受
// 基线、同一包裹和同一初始路由目的只能形成一个当前有效初始路由结果」由键的选维承担。
//
// FindByKey 读回的计划或无路由判断重建不出来时交 ErrStoredRouteRecordUnreadable。
type InitialRouteStore interface {
	FindByKey(
		ctx context.Context,
		key domain.InitialRouteJudgmentKey,
	) (InitialRouteRecord, bool, error)
	Save(ctx context.Context, record InitialRouteRecord) (InitialRouteSaveOutcome, error)
}

// InitialRouteNotFormedReason 指名一件包裹的初始路由判断为什么停在重投推不动的一格（ADR-0273 决定二）。入格判据是恢复
// 动作，与复核的 NotJudgedReason 同一条：重投改变不了结果，只有一次登记、一次部署或授权角色的裁决推得动；依赖读不回之类
// 重投推得动的不在这里，照旧整封回滚重投。码与这几格进本集合之前作未决原因时的码相同，运维按码查的口径不变。
type InitialRouteNotFormedReason uint8

const (
	InitialRouteNotFormedReasonInvalid InitialRouteNotFormedReason = iota
	// NotFormedEvidenceNotConfigured 是网络目录为空，或判断时点没有适用于该服务目的的路由策略版本（ADR-0052、ADR-0148
	// 决定六）：等租户登记网络定义。绝不译成`无当前有效路由`——还没人说过网络长什么样。
	NotFormedEvidenceNotConfigured
	// NotFormedCandidateSpaceNotEstablished 是证据里一条服务区域解析都没有，候选空间立不起来：等登记服务区域。
	NotFormedCandidateSpaceNotEstablished
	// NotFormedCandidateEvidenceIncomplete 是还有证据未知的候选，既选不出也立不成`无当前有效路由`：地址信息不足（受理基线
	// 一经固定不再改写）或限制状态未知（关务目录为空、来源未接）。限制状态未知的来源只交「状态未知」时一律是重投推不动的
	// 缺口，真依赖故障在取数侧交错误（ADR-0250 决定一同一条）。
	NotFormedCandidateEvidenceIncomplete
	// NotFormedRankingFormNotConfigured 是路由策略版本没有声明排序形态：等租户选形态（ADR-0146）。
	NotFormedRankingFormNotConfigured
	// NotFormedCandidatesTied 是最低成本并列、选不出唯一一条（`PAR-NET-16`）：候选可行、等授权角色裁。同一份依据再判仍
	// 是同一个并列，重投推不动。
	NotFormedCandidatesTied
	// NotFormedCandidateCostsPending 是合格候选无一已计价、至少一条待判断：等段依据、段计价区域、价卡或参考序列登记。
	NotFormedCandidateCostsPending
	// NotFormedCandidateCostsUnpriceable 是合格候选无一已计价、都不可计价：要有人改价卡或候选集合（ADR-0029）。
	NotFormedCandidateCostsUnpriceable
	// NotFormedCandidateCostCurrenciesDiffer 是已计价候选币种不齐、策略版本又没登比较币种：等租户登比较币种。
	NotFormedCandidateCostCurrenciesDiffer
	// NotFormedCostSourceNotConfigured 是候选成本取数侧没接计价输入路径：那半是消费方的实例半边，等部署接入。
	NotFormedCostSourceNotConfigured
)

func (reason InitialRouteNotFormedReason) String() string {
	switch reason {
	case NotFormedEvidenceNotConfigured:
		return "ROUTE_EVIDENCE_NOT_CONFIGURED"
	case NotFormedCandidateSpaceNotEstablished:
		return "CANDIDATE_SPACE_NOT_ESTABLISHED"
	case NotFormedCandidateEvidenceIncomplete:
		return "CANDIDATE_EVIDENCE_INCOMPLETE"
	case NotFormedRankingFormNotConfigured:
		return "RANKING_FORM_NOT_CONFIGURED"
	case NotFormedCandidatesTied:
		return "CANDIDATES_TIED"
	case NotFormedCandidateCostsPending:
		return "CANDIDATE_COSTS_PENDING"
	case NotFormedCandidateCostsUnpriceable:
		return "CANDIDATE_COSTS_UNPRICEABLE"
	case NotFormedCandidateCostCurrenciesDiffer:
		return "CANDIDATE_COST_CURRENCIES_DIFFER"
	case NotFormedCostSourceNotConfigured:
		return "COST_SOURCE_NOT_CONFIGURED"
	default:
		return ""
	}
}

// InitialRouteNotFormed 是一件包裹的初始路由停在重投推不动一格时留下的原因与对象：原因没落库，编排不得答`路由未形成`
// （ADR-0273 决定二、ADR-0094 决定五同一条护栏）。它不是判断结果：不进 initial_route，不挡同一判断键以后再判。
//
// Candidates 在并列与候选证据不全时给出：本轮评估过的全部候选及各自的选择或淘汰依据。Tied 与 TiedCosts 只在并列时给出：
// 并列的那几家与各自的候选成本事实，是并列的理由——授权角色裁的时候要看得见在哪几家之间、按什么金额打平。Gaps 只在候选
// 证据不全时给出：缺什么、什么条件再判。
//
// AcceptedAt 是委托接受的业务时间（计划日后若形成，从它起生效）；SettledAt 是入账那一刻的处理时钟。
type InitialRouteNotFormed struct {
	Key         domain.InitialRouteJudgmentKey
	Correlation domain.RequestCorrelationID
	Reason      InitialRouteNotFormedReason
	Candidates  []domain.RouteCandidate
	Tied        []domain.CandidateID
	TiedCosts   []domain.CandidateCostFact
	Gaps        []domain.EvidenceGap
	AcceptedAt  time.Time
	SettledAt   time.Time
}

// InitialRouteNotFormedRegister 留下初始路由未形成的原因与对象。同一判断键再来一次（携带同一基线的另一封交接）停在
// 未形成时按最新这一次改写：这一行答的是这件包裹「最近一次停住的原因」，不是某一次投递为什么没判。写口要事务：
// 这一行与同一交接里已判成的包裹、交接登记与 inbox 的入账同笔提交。
type InitialRouteNotFormedRegister interface {
	RecordNotFormed(ctx context.Context, record InitialRouteNotFormed) error
}

// RouteHandoffLog 按交接关联登记内容指纹，供重放与冲突分界（UC-NR-001 步骤 2）：同关联
// 同指纹是重放、异指纹是冲突。Append 对同一关联只接纳第一份，再来的交回已有指纹。
type RouteHandoffLog interface {
	FindDigest(
		ctx context.Context,
		tenant domain.TenantID,
		correlation domain.RequestCorrelationID,
	) (string, bool, error)
	Append(
		ctx context.Context,
		tenant domain.TenantID,
		correlation domain.RequestCorrelationID,
		digest string,
	) error
}

// InitialRouteHandoffIntent 把一份已提交的包裹级结果交给适用下游（ADR-0043 缝形，
// 第四个样本）。意图由判断键认领：同一结果无论交几次都是同一份。
type InitialRouteHandoffIntent struct {
	Correlation domain.RequestCorrelationID
	Record      InitialRouteRecord
}

// InitialRouteHandoff 把已提交的包裹级初始路由判断写入 Outbox
// （`OutboxInitialRouteHandoff`）。信封 ID 由判断键（含租户）再加类型段认领，入队由
// outboxintent.EnqueueOnce 承担；重放重发同一份（ADR-0043）。
type InitialRouteHandoff interface {
	HandOffInitialRoute(ctx context.Context, intent InitialRouteHandoffIntent) error
}

// RouteIdentityFactory 签发本上下文自己拥有的计划版本标识。刻意不从调用方接收：交接
// 关联不得变成计划版本号。
type RouteIdentityFactory interface {
	NextRoutePlanVersionID(ctx context.Context) (domain.RoutePlanVersionID, error)
}

// PlanApplicabilityStore 按计划版本存取适用性记录。计划本体不可变、适用性另立记录，
// 两者分开存正是「改后者不动前者」的存储面。FindByPlan 读回的行重建不出来时交 ErrStoredRouteRecordUnreadable。
type PlanApplicabilityStore interface {
	FindByPlan(
		ctx context.Context,
		plan domain.RoutePlanVersionID,
	) (domain.PlanApplicability, bool, error)
	Save(ctx context.Context, applicability domain.PlanApplicability) error
}

// CandidateReviewState 是复核时替代候选评估线（6B）的独立状态。它与计划适用性分开保存
// ——硬句要求「原计划已失效 + 无当前有效路由 + 替代候选评估未决」三件并存，压在一起
// 就表达不出「失效已定、候选还看不清」。
type CandidateReviewState uint8

const (
	CandidateReviewStateInvalid CandidateReviewState = iota
	CandidatesAvailable
	NoQualifiedCandidates
	CandidateReviewUndecided
)

func (state CandidateReviewState) String() string {
	switch state {
	case CandidatesAvailable:
		return "CANDIDATES_AVAILABLE"
	case NoQualifiedCandidates:
		return "NO_QUALIFIED_CANDIDATES"
	case CandidateReviewUndecided:
		return "CANDIDATE_REVIEW_UNDECIDED"
	default:
		return ""
	}
}

// ReassessmentConclusionKind 是一次复核越过提交边界的四种领域走向。`已改路`独立一格：
// 它同时携带失效与新版本的替代关系，与「失效后无路可走」不是一种结论。
type ReassessmentConclusionKind uint8

const (
	ReassessmentConclusionKindInvalid ReassessmentConclusionKind = iota
	ReassessmentStillApplicable
	ReassessmentPlanLapsed
	ReassessmentFirstPlanFormed
	ReassessmentRerouted
)

func (kind ReassessmentConclusionKind) String() string {
	switch kind {
	case ReassessmentStillApplicable:
		return "STILL_APPLICABLE"
	case ReassessmentPlanLapsed:
		return "PLAN_LAPSED"
	case ReassessmentFirstPlanFormed:
		return "FIRST_PLAN_FORMED"
	case ReassessmentRerouted:
		return "REROUTED"
	default:
		return ""
	}
}

// AutoRerouteFactsView 是事实目录的读口。复核不再读它：自动改路由策略版本当场折出
// （ADR-0173）。目录只留历史陈述，不是判断权威。第二个返回值为 false 即这本册没有该键。
type AutoRerouteFactsView interface {
	LoadAutoRerouteFacts(
		ctx context.Context,
		key domain.InitialRouteJudgmentKey,
	) (domain.AutoRerouteFacts, bool, error)
}

// AutoRerouteFactsRecord 是事实目录里的一版陈述：判断键 + 版本号 + 五件事实 + 折算
// 依据。StrategyBasis 指名这份陈述按哪个策略版本折出来——阈值与冻结边界的取值属
// PAR-NET-14 实例半边，目录只存折算完的结论与出处，不存阈值本身。
type AutoRerouteFactsRecord struct {
	Key           domain.InitialRouteJudgmentKey
	Version       int
	Facts         domain.AutoRerouteFacts
	StrategyBasis string
	RegisteredAt  time.Time
}

// AutoRerouteFactsSaveOutcome 是事实目录的写入代数。只有两格（ADR-0031 同款）：
// `已登记`是业务答案不是错误，**没有覆盖格是有意的**——同键同版本已在册就交回
// `已登记`，内容是否一致由编排读回既有版本自己比（先例：CC 案件配置登记册），
// 「重放同一份」与「换了内容」这两件事在用例结果上必须分得开。
type AutoRerouteFactsSaveOutcome uint8

const (
	AutoRerouteFactsSaveOutcomeInvalid AutoRerouteFactsSaveOutcome = iota
	AutoRerouteFactsRegistered
	AutoRerouteFactsAlreadyRegistered
)

func (outcome AutoRerouteFactsSaveOutcome) String() string {
	switch outcome {
	case AutoRerouteFactsRegistered:
		return "REGISTERED"
	case AutoRerouteFactsAlreadyRegistered:
		return "ALREADY_REGISTERED"
	default:
		return ""
	}
}

// AutoRerouteFactsRegistry 是 AutoRerouteFactsView 的写口半边。事实的取值随包裹移动
// 与限制解除而变化，新陈述登记为同键的下一版本，历史版本原样保留——读口取最大版本，
// 「谁在什么时候按什么依据陈述过什么」审计要答得出。FindAutoRerouteFacts 按（键+版本）
// 精确取一版，供登记编排在`已登记`后读回比对幂等与冲突。
type AutoRerouteFactsRegistry interface {
	RegisterAutoRerouteFacts(
		ctx context.Context,
		record AutoRerouteFactsRecord,
	) (AutoRerouteFactsSaveOutcome, error)
	FindAutoRerouteFacts(
		ctx context.Context,
		key domain.InitialRouteJudgmentKey,
		version int,
	) (AutoRerouteFactsRecord, bool, error)
}

// ReassessmentRecord 是一次复核越过提交边界后留下的东西。`已失效`的记录同时携带失效
// 依据与候选评估状态（三件并存的硬句）；「无当前有效路由」由「已失效且无新计划」这个
// 记录状态表达，不复用初始判断的全淘汰对象——复核失效时候选可以仍在评估。
//
// 改路三件出自失效线，也出自仍适用线上的改善改路（ADR-0173 决定三）：Authority 记录三态判定（invalid=没评估——事实
// 未配置或候选未收敛）；建议与决定互斥——建议是「没自动成，等授权角色」，决定是「自动成了」。
//
// `已改路`的依据恰有其一：失效线上是 LapseBasis，仍适用线上是 ImprovementBasis。只看依据就分得出原计划是走不通了还是
// 被更便宜的路替下，复核册的逐结论 CHECK 同此。
type ReassessmentRecord struct {
	Correlation         domain.RequestCorrelationID
	Key                 domain.InitialRouteJudgmentKey
	Conclusion          ReassessmentConclusionKind
	ReviewedPlan        domain.RoutePlanVersionID
	LapseBasis          domain.ApplicabilityBasisReference
	ImprovementBasis    domain.CostImprovementBasis
	HasImprovementBasis bool
	CandidateState      CandidateReviewState
	NewPlan             domain.InitialRoutePlan
	HasNewPlan          bool
	RerouteState        domain.RerouteAuthority
	RerouteBlockers     []string
	Suggestion          domain.RerouteSuggestion
	HasSuggestion       bool
	Decision            domain.RerouteDecision
	HasDecision         bool
	// TriggerOccurredAt 是这次复核所判触发的业务时间（ADR-0235）：它是路由历史里的「当前位置」时刻，较早的触发
	// 据它不判。ReassessedAt 是处理时钟，两者不可互代——按处理时刻比，就又成了按到达顺序裁决。
	TriggerOccurredAt time.Time
	ReassessedAt      time.Time
}

// ReassessmentSaveOutcome 与其余判断库同一套写入代数（ADR-0031）。
type ReassessmentSaveOutcome uint8

const (
	ReassessmentSaveOutcomeInvalid ReassessmentSaveOutcome = iota
	ReassessmentSaved
	ReassessmentAlreadyRecorded
)

// NotJudgedReason 指名一次复核触发为什么入账却不形成判断（ADR-0235）。入格判据是恢复动作：
// 重投推不动、只等登记或等下一次控制触发的才在这里；依赖读不回之类重投推得动的，仍是应用层
// 的未决原因、整笔回滚。
type NotJudgedReason uint8

const (
	NotJudgedReasonInvalid NotJudgedReason = iota
	// NotJudgedNoRoutingHistory 是这件包裹没有初始路由判断可复核：复核复的是一份已有判断，
	// 不凭空假定初始计划。
	NotJudgedNoRoutingHistory
	// NotJudgedEvidenceNotConfigured 是判断时点没有适用的网络证据，等租户登记（ADR-0052）。
	NotJudgedEvidenceNotConfigured
	// NotJudgedFreezeFormUnconfigured 是原计划仍可执行、策略版本却没声明冻结形态：不当成未冻结。
	NotJudgedFreezeFormUnconfigured
	// NotJudgedTriggerPrecedesCurrentPosition 是触发的业务时间早于路由历史里最近一次已判控制触发：包裹已经在更晚
	// 的位置上判过，这次不判（ADR-0235）。它不等登记也不等续办，是终局的入账。
	NotJudgedTriggerPrecedesCurrentPosition
	// NotJudgedTriggerPrecedesShipmentAcceptance 是触发的业务时间早于这一判断键的委托接受时刻（ADR-0279）：初始路由从接受
	// 起生效，据这次触发离场会早于生效。与早于当前位置分开：那一格比的是路由历史里的已判复核，这一格比的是委托本身，
	// 审计要答得出早于的是哪一个。同样是终局的入账。
	NotJudgedTriggerPrecedesShipmentAcceptance
	// NotJudgedPlanApplicabilityUnclear 是原计划选中候选的限制状态未知，适用性看不清（ADR-0250）。限制状态未知的来处都在
	// 取数侧、都是重投推不动的缺口——等目录登记、等部署接上关务判断口，或受理基线缺的端点国家永远补不上；依赖读不动在
	// 取数侧交错误，到不了这一格。补法各不相同，所以这一格必带取数侧交回的缺口与再判条件。
	NotJudgedPlanApplicabilityUnclear
)

func (reason NotJudgedReason) String() string {
	switch reason {
	case NotJudgedNoRoutingHistory:
		return "NO_ROUTING_HISTORY"
	case NotJudgedEvidenceNotConfigured:
		return "REASSESS_EVIDENCE_NOT_CONFIGURED"
	case NotJudgedFreezeFormUnconfigured:
		return "FREEZE_FORM_UNCONFIGURED"
	case NotJudgedTriggerPrecedesCurrentPosition:
		return "TRIGGER_PRECEDES_CURRENT_POSITION"
	case NotJudgedTriggerPrecedesShipmentAcceptance:
		return "TRIGGER_PRECEDES_SHIPMENT_ACCEPTANCE"
	case NotJudgedPlanApplicabilityUnclear:
		return "PLAN_APPLICABILITY_UNCLEAR"
	default:
		return ""
	}
}

// NotJudgedTrigger 是一次入账未判的复核触发留下的原因。触发本身已由 RouteHandoffLog 保全，
// 这一行只答「为什么这一次没判」；它不是复核结论，不进路由历史，也不挡同一触发以后再判。
type NotJudgedTrigger struct {
	Correlation domain.RequestCorrelationID
	Key         domain.InitialRouteJudgmentKey
	Reason      NotJudgedReason
	// TriggerOccurredAt 是触发的业务时间，不是处理时刻。
	TriggerOccurredAt time.Time
	// CurrentPositionAt 只在触发早于当前位置时有值：比较所用的那一次已判控制触发的业务时间，审计要答得出「早于谁」。
	CurrentPositionAt time.Time
	// ShipmentAcceptedAt 只在触发早于委托接受时有值：比较所用的委托接受时刻。
	ShipmentAcceptedAt time.Time
	// Missing 与 Reassess 是取数侧交回的缺口与再判条件，适用性看不清时必带：运维按缺口查哪几件包裹在等哪一项登记，
	// 续办据此认该重驱哪几行。两件成对，缺口没有出口就是一句续不上的否定（AT-NR-018）。
	Missing   domain.EvidenceGapReference
	Reassess  domain.ReassessmentCondition
	SettledAt time.Time
}

// ReassessmentStore 按触发关联找回并保存复核结果：同一触发和输入版本已经处理即返回
// 原结果，不重复决定。
type ReassessmentStore interface {
	FindByCorrelation(
		ctx context.Context,
		tenant domain.TenantID,
		correlation domain.RequestCorrelationID,
	) (ReassessmentRecord, bool, error)
	Save(
		ctx context.Context,
		correlation domain.RequestCorrelationID,
		record ReassessmentRecord,
	) (ReassessmentSaveOutcome, error)
	// RecordNotJudged 留下一次入账未判的原因。同（租户，触发关联）已有一行时保留先到那一行、
	// 答成功：携带同一触发的另一封信封再来时编排会重新判一遍，仍判不成就再来这里。
	RecordNotJudged(ctx context.Context, record NotJudgedTrigger) error
	// LatestTriggerAt 交回这一判断键在册复核里最近一次控制触发的业务时间；没有带业务时间的复核时第二个返回值为假。
	// 实现须把「读最近一次 → 判 → 落复核」按判断键串行到事务结束（ADR-0235）：两路触发在两笔并发事务里各自读不到
	// 对方尚未提交的复核，会双双判过，较早的那次照样落在较新的之后。
	LatestTriggerAt(ctx context.Context, key domain.InitialRouteJudgmentKey) (time.Time, bool, error)
	// FormedPlans 交回这一判断键在册复核里形成过的全部新计划（`已改路`或`首个计划`那一行），按触发业务时间排，只为读来
	// 可复现，先后不作判断依据。复核据它与初始路由定判断起点，所以要在 LatestTriggerAt 取的判断键锁里读：锁外读到的起点，
	// 落册之前可能已被另一次复核换掉。在册计划重建不出来时交 ErrStoredRouteRecordUnreadable。
	FormedPlans(ctx context.Context, key domain.InitialRouteJudgmentKey) ([]domain.InitialRoutePlan, error)
}

// ErrStoredRouteRecordUnreadable 说明路由历史里落了册的一行经领域构造门重建不出来：初始路由的计划或无路由判断、复核
// 形成的新计划、计划适用性。那是坏数据，重投推不动；与「读不回」（依赖故障，重投推得动）分开交，调用方才分得出该等依赖
// 恢复还是该修数据。
var ErrStoredRouteRecordUnreadable = errors.New("network routing: a stored routing record does not rebuild")
