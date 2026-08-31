package ports

import (
	"context"
	"time"

	"go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
)

// 本文件是追踪异常**案件侧**三张查阅页的伴生列表读端口（ADR-0077，票
// admin-skeleton-closure-batch/06）：exception-triage、exception-cases、
// claims-recovery 三页的供数面。目录侧的列表读端口在 catalogue_read.go，已接线在服
// 务，本文件一个符号都不动它——案件与目录分野是那张导航裁决的机制半边（案件页空着
// 是三堵墙拦的，混进目录会让人以为墙降了）。
//
// 读口不与信号/建案/索赔/追偿写入方共接口——扩写侧接口会拆全部写侧测试替身，伴生
// 读端口另立（判据同 collectionremittance CodSubledgerCatalogueRow 那句）。上列的是
// **检索列面**的照实转写：发作期连同它的分诊结论（同键两表，0002 同笔提交）、处置
// 请求、异常案件、客户通知、索赔项与追偿事项各照登记行转写，不重建领域对象、不形成
// 判断。
//
// 三组接口按页分立：一页一读口，某页读不回不该把另两页也拖进故障面。租户在方法签名
// 上（ADR-0077 Decision 五）；Limit 必须为正，每页多大由接入面按渠道契约裁决；空登
// 记册如实交回空列表（Decision 四：空册本身就是内容——案件侧的写入方是信号→分诊→
// 建案的编排，事实在接入渠道墙后面，三页接完仍是空册是预期不是缺陷）。

// SignalEpisodeCatalogueRow 是信号发作期册上列的一行：发作期连同它的分诊结论。
//
// 结论三件（Outcome/Rule/TriagedAt）成对在场：0002 把发作期与结论钉成同笔提交，
// 缺席只该出现在坏历史上，读口照实透出不代补。Outcome 是分诊四结果的封闭词
// （ATTACH_TO_EXISTING / AUTO_ESTABLISH / MANUAL_REVIEW / NO_CASE）。Ended 两件
// 成对缺席表示发作期仍活跃——结束依据与结束时间同在场是 0002 的在场规则。
//
// 页面详情区想要的「事实依据」在发作期行上没有登记格：已接受事实按（租户+包裹）
// 登在 accepted_fact，行上不指回发作期，本行不代连（票 06 Comments 记明）。
type SignalEpisodeCatalogueRow struct {
	EpisodeID    string
	Parcel       string
	Kind         string
	Rule         string
	Confidence   string
	Hits         int64
	StartedAt    time.Time
	LastHitAt    time.Time
	ReleaseBasis string
	EndedAt      *time.Time
	PriorEpisode string
	Outcome      string
	OutcomeRule  string
	TriagedAt    *time.Time
}

// DispositionRequestCatalogueRow 是处置请求册上列的一行：请求六要件、发送与接受
// 窗口、源上下文答复与取消结果照行转写。
//
// Judgment 两件成对缺席表示尚无答复（封闭四走向：ACCEPTED / PARTIALLY_ACCEPTED /
// REFUSED / SUPPLEMENT_REQUIRED）；Cancellation 只在已判断后可能在场（封闭四值）。
// SupersededBy 在场即这行已被替代——替代不是删除（0005），原请求与其判断原样在册，
// 版本链因此完整可见。实际执行结果不在本册：那是目标上下文按事实返回的东西，行内
// 没有它的字段。
type DispositionRequestCatalogueRow struct {
	RequestID        string
	CaseID           string
	TargetContext    string
	Action           string
	Scope            string
	Reason           string
	Evidence         string
	IntentVersion    int64
	SentAt           time.Time
	AcceptanceWindow *time.Time
	Judgment         string
	JudgedAt         *time.Time
	Cancellation     string
	SupersededBy     string
}

// ExceptionCaseCatalogueRow 是异常案件册上列的一行：案件身份、根对象、影响范围、
// 责任团队、主状态三相（AWAITING_RESPONSE / IN_PROGRESS / CLOSED）与建立/首次响应/
// 关闭三时刻照行转写。
//
// Conclusion 只随已关闭在场（0008 的在场规则：没有结论的已关闭案件业务上不成立）；
// MergedInto 在场即受控归并（归并是关闭的一种，原编号不删除）。页面栏目里当前工作
// 条件、严重度、处置优先级与响应周期（首次响应之外的时限版本）在存储上没有登记格
// ——0008 刻意只落精简主生命周期，工作条件多一格就等于默许扩成互斥主状态；本行
// 不带那几键，也不代填（票 06 Comments 记明）。
type ExceptionCaseCatalogueRow struct {
	CaseID          string
	RootParcel      string
	ImpactScope     string
	ResponsibleTeam string
	Phase           string
	EstablishedAt   time.Time
	FirstResponse   *time.Time
	ClosedAt        *time.Time
	Conclusion      string
	MergedInto      string
}

// NotificationMilestoneNode 是通知过程的一个节点：封闭六值（GENERATED /
// SUBMITTED_TO_CHANNEL / CHANNEL_ACCEPTED / DELIVERED / FAILED / CUSTOMER_CONFIRMED，
// domain.NotificationMilestone），分别记录不覆盖。
type NotificationMilestoneNode struct {
	Milestone  string
	RecordedAt time.Time
}

// CustomerNotificationCatalogueRow 是客户异常通知册上列的一行：披露身份三维
// （客户+发作期+决定时刻）、披露依据、内容引用、要求时限、适用渠道与过程节点照行
// 转写。
//
// Content 是披露内容快照的**引用**不是快照正文——快照本体版本化在披露内容侧，列面
// 只指名。Milestones 整列照登记转写：渠道接受、送达与客户确认是数组里各自的节点，
// 读口不把它们折成一个「已通知」，哪个结果满足通知义务由客户合同与通知策略判断
// （CONTEXT 硬句），本行只带义务判据引用（Obligation）。
type CustomerNotificationCatalogueRow struct {
	NotificationID string
	Customer       string
	Episode        string
	DecidedAt      time.Time
	Policy         string
	Content        string
	Deadline       time.Time
	Channel        string
	Obligation     string
	Milestones     []NotificationMilestoneNode
}

// ClaimItemCatalogueRow 是客户索赔项册上列的一行：收到、通过资格审核、确认赔偿
// 责任是三个不同判断（CONTEXT 硬句），三判各自的格照行转写不压并。
//
// Screen 封闭三值（ELIGIBLE / INELIGIBLE / AWAITING_SUPPLEMENT，ADR-0051 的第三
// 态）；等待补充的四件（缺少材料、补充范围、通知依据、当前截止）只随该态在场。
// Conclusion 封闭四值，随行带结论时刻与复核截止；PriorConclusion 在场即复核换过版
// ——原结论保留在前版列（0004），不翻旧插新。DeadlineVersions 计补充期限的版本数
// （获批延期形成新版本，原期限保留在 claim_supplement_deadline）：期限历史逐版属
// 详情读口，列面只计数，把「从未设过期限」与「延过几次」分开。
//
// Applicant 是代提申请人引用（0018）：存量行缺席即空串，不代填客户自己。
//
// 页面「首次索赔期限」栏在存储上没有登记格（行上只有提交时刻与结论复核截止，首次
// 索赔期限是合同侧口径），本行不带那一键（票 06 Comments 记明）。**没有金额键**是
// 边界不是遗漏：赔付与追偿金额由 settlement-accounting 形成，本上下文只有索赔项本体。
type ClaimItemCatalogueRow struct {
	Batch              string
	ItemID             string
	Customer           string
	Applicant          string
	Contract           string
	Target             string
	Kind               string
	SubmittedAt        time.Time
	Revision           int64
	Screen             string
	ScreenBasis        string
	MissingMaterials   string
	SupplementScope    string
	SupplementNotice   string
	SupplementDeadline *time.Time
	DeadlineVersions   int64
	Conclusion         string
	ConcludedAt        *time.Time
	ReviewBy           *time.Time
	PriorConclusion    string
	Withdrawn          bool
	WithdrawnAt        *time.Time
}

// RecoveryActionCell 是追偿事项行上某一动作种类的最近过程节点：节点封闭七值
// （PREPARED / SUBMITTED / CHANNEL_ACCEPTED / DELIVERED / ACKNOWLEDGED /
// SUBMISSION_FAILED / DELIVERY_FAILED），Attempt 是该种类当前尝试序。
type RecoveryActionCell struct {
	Milestone  string
	Attempt    int64
	OccurredAt time.Time
}

// RecoveryMatterCatalogueRow 是追偿事项册上列的一行：事项要件（相对方、依据、责任
// 法人、证据、范围、关联案件）与适用期限照行转写——要件成立即固定，行内无可回写列。
//
// PreliminaryNotice 与 FormalAssertion 各取该种类**最近一个**过程节点（写侧按到达
// 序追加，末行即最近；判据同 nodeoperations 集运单元最近封签）：预先通知与正式主张
// 不能合并为一个模糊的「已追偿」（CONTEXT 硬句），两格成对缺席表示该种类尚无动作。
// 过程节点逐行历史属详情读口，列面只取最近。页面「对方响应」与「外部责任结论」两栏
// 在存储上没有登记格（0004 的事项行与动作行都没有响应/结论列），本行不带那两键，
// 也不把「无响应」代判成拒绝（票 06 Comments 记明）。
type RecoveryMatterCatalogueRow struct {
	MatterID          string
	CaseID            string
	Counterparty      string
	Scope             string
	Basis             string
	LegalEntity       string
	Evidence          string
	Deadline          time.Time
	OpenedAt          time.Time
	PreliminaryNotice *RecoveryActionCell
	FormalAssertion   *RecoveryActionCell
}

// TriageReviewRead 是异常分诊与处置协调页（exception-triage）的伴生列表读端口。
type TriageReviewRead interface {
	ListSignalEpisodes(
		ctx context.Context,
		tenant domain.TenantID,
		limit int,
	) ([]SignalEpisodeCatalogueRow, error)
	ListDispositionRequests(
		ctx context.Context,
		tenant domain.TenantID,
		limit int,
	) ([]DispositionRequestCatalogueRow, error)
}

// CaseReviewRead 是异常案件页（exception-cases）的伴生列表读端口。
//
// 刻意没有可见性缺口的方法：visibility_gap 表在场，但案件页没有缺口栏——缺口是
// 「预期观察届满未得」的证明，行内没有延误/遗失判断，页面栏目也没给它位置；无栏
// 可供就不设读法，造一个没人消费的册子只会引人把缺口读成案件（票 06 Comments 记明）。
type CaseReviewRead interface {
	ListExceptionCases(
		ctx context.Context,
		tenant domain.TenantID,
		limit int,
	) ([]ExceptionCaseCatalogueRow, error)
}

// ClaimsRecoveryReviewRead 是索赔与追偿页（claims-recovery）三个页签的伴生列表读
// 端口。
//
// 材料收讫册（claim_material_receipt）刻意没有方法：页签栏目里没有收讫列——收讫
// 减撤销的在手口径属资格审核的证据视图，不是本页列面（票 06 Comments 记明）。
type ClaimsRecoveryReviewRead interface {
	ListCustomerNotifications(
		ctx context.Context,
		tenant domain.TenantID,
		limit int,
	) ([]CustomerNotificationCatalogueRow, error)
	ListClaimItems(
		ctx context.Context,
		tenant domain.TenantID,
		limit int,
	) ([]ClaimItemCatalogueRow, error)
	ListRecoveryMatters(
		ctx context.Context,
		tenant domain.TenantID,
		limit int,
	) ([]RecoveryMatterCatalogueRow, error)
}
