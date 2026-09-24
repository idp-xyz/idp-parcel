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
	ViewRevision      domain.NetworkViewRevision
}

// RequestCarriedContent 是随判断请求携带、由发起方交来的判断对象内容（ADR-0148 决定一「随请求携带」一路，
// ADR-0075 同款，判据是同版性）：它是判断对象那一版的内容，本上下文只用它判断，不回读、不保存本体。首版只含
// 地理解析投影；服务要求与承诺上界随 routing-first-cut/08、09 加入。零值即发起方什么都没带。
type RequestCarriedContent struct {
	Geo domain.GeoResolutionProjection
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
type CommercialEligibilityView interface {
	AssessNetworkEligibility(
		ctx context.Context,
		key domain.ReachabilityJudgmentKey,
	) (domain.NetworkEligibility, error)
}

// ReachabilityJudgmentRecord 是一次判断越过提交边界后留下的东西。判断时间不在
// `ReachabilityFinding` 里，因为它不是领域结论的一部分——`asOf` 决定按哪一刻的网络证据
// 评估，判断时间只说明这次判断何时作出，压成一个会让重放看起来像新判断。
//
// ViewRevision 同在记录而不在结论里：它是证据出处不是三值判断的一部分，留在记录上供
// 消费方比对「判断形成后视图有没有换代」（CONTEXT「保留……当前修订标识」）。
type ReachabilityJudgmentRecord struct {
	Key          domain.ReachabilityJudgmentKey
	Finding      domain.ReachabilityFinding
	JudgedAt     time.Time
	ViewRevision domain.NetworkViewRevision
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
type CandidatePath struct {
	Candidate domain.CandidateID
	Legs      []domain.PlannedLeg
}

// InitialRouteEvidence 是一次初始路由判断所需的版本化事实（UC-NR-001 层次 1–4 的输入
// 清单）。与可达性证据同一条 ADR-0046 纪律：端口只取事实不做评估，事实与修订一次取回。
// 日历与缓冲取值属租户取值。
//
// RankingForm 是 Strategy 所指那一版声明的排序形态，零值即该版未声明；CandidateCosts 按
// 形态要的维度给出逐候选事实，每个合格候选恰一条——待判断与不可计价各有一格，不以缺席表达。
type InitialRouteEvidence struct {
	ServiceAreas      []domain.ServiceAreaResolution
	RouteRequirements []domain.RouteRequirement
	PathExecutability []domain.PathExecutability
	HardConstraints   []domain.HardConstraintFinding
	Projections       []domain.CandidateTimeProjection
	CommittedBound    domain.CommittedTimeBound
	RankingForm       domain.RankingForm
	CandidateCosts    []domain.CandidateCostFact
	Paths             []CandidatePath
	Strategy          domain.RouteStrategyReference
	ViewRevision      domain.NetworkViewRevision
}

// InitialRouteEvidenceView 为一次初始路由判断取回版本化事实。三格语义同
// NetworkEvidenceView（ADR-0052）：事实、`未配置`、依赖不可用，各自对应不同的恢复动作，
// 应用层为前两者分设未决原因。空册走第二格，绝不折成一份空证据。
type InitialRouteEvidenceView interface {
	LoadInitialRouteEvidence(
		ctx context.Context,
		key domain.InitialRouteJudgmentKey,
	) (InitialRouteEvidence, bool, error)
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
type InitialRouteStore interface {
	FindByKey(
		ctx context.Context,
		key domain.InitialRouteJudgmentKey,
	) (InitialRouteRecord, bool, error)
	Save(ctx context.Context, record InitialRouteRecord) (InitialRouteSaveOutcome, error)
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
// 两者分开存正是「改后者不动前者」的存储面。
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

// AutoRerouteFactsView 按判断键取自动改路四条件的事实（政策允许、在受控节点、仅未
// 执行受影响、限制与责任清单）。第二个返回值为 false 即「事实目录未配置」——不猜：
// 只失效不改路，连改路建议都形不成（说不出「为什么没自动」）。依赖调不通作为错误返回。
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
// 改路三件只在失效路上有意义：Authority 记录三态判定（invalid=没评估——事实未配置或
// 候选未收敛）；建议与决定互斥——建议是「没自动成，等授权角色」，决定是「自动成了」。
type ReassessmentRecord struct {
	Correlation     domain.RequestCorrelationID
	Key             domain.InitialRouteJudgmentKey
	Conclusion      ReassessmentConclusionKind
	ReviewedPlan    domain.RoutePlanVersionID
	LapseBasis      domain.ApplicabilityBasisReference
	CandidateState  CandidateReviewState
	NewPlan         domain.InitialRoutePlan
	HasNewPlan      bool
	RerouteState    domain.RerouteAuthority
	RerouteBlockers []string
	Suggestion      domain.RerouteSuggestion
	HasSuggestion   bool
	Decision        domain.RerouteDecision
	HasDecision     bool
	ReassessedAt    time.Time
}

// ReassessmentSaveOutcome 与其余判断库同一套写入代数（ADR-0031）。
type ReassessmentSaveOutcome uint8

const (
	ReassessmentSaveOutcomeInvalid ReassessmentSaveOutcome = iota
	ReassessmentSaved
	ReassessmentAlreadyRecorded
)

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
}
