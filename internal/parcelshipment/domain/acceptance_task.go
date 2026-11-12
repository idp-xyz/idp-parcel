package domain

import (
	"errors"
	"strings"
	"time"
)

var (
	ErrInvalidProcessingAttempt      = errors.New("parcel shipment: invalid processing attempt")
	ErrAcceptanceTaskComplete        = errors.New("parcel shipment: the acceptance judgment task is already complete")
	ErrInvalidManualReviewCompletion = errors.New("parcel shipment: invalid manual review completion")
	ErrManualReviewAlreadyCompleted  = errors.New("parcel shipment: the manual review is already completed")
)

// 复核完成的三项引用都属别处：授权规则属 party-commercial，复核方与证据属运营留痕。本上下文
// 只记引用，不拥有角色目录，也不校验某个角色够不够格——那是 BD-PS-002 的未确认参数。

type ReviewAuthorityReference struct{ requiredValue }

func NewReviewAuthorityReference(value string) (ReviewAuthorityReference, error) {
	required, err := newRequiredValue("review authority reference", value)
	return ReviewAuthorityReference{required}, err
}

type ReviewerReference struct{ requiredValue }

func NewReviewerReference(value string) (ReviewerReference, error) {
	required, err := newRequiredValue("reviewer reference", value)
	return ReviewerReference{required}, err
}

type ReviewEvidenceReference struct{ requiredValue }

func NewReviewEvidenceReference(value string) (ReviewEvidenceReference, error) {
	required, err := newRequiredValue("review evidence reference", value)
	return ReviewEvidenceReference{required}, err
}

type ManualReviewCompletionSpec struct {
	Authority   ReviewAuthorityReference
	Reviewer    ReviewerReference
	Evidence    ReviewEvidenceReference
	CompletedAt time.Time
}

// ManualReviewCompletion 是一次已完成的人工复核。三项引用全部必填：少了授权就说不出这次
// 复核凭什么算数，少了实际复核方就无从追责，少了证据就与「有人点了一下」分不开——用例要求
// 复核「按证据完成」。
type ManualReviewCompletion struct {
	authority   ReviewAuthorityReference
	reviewer    ReviewerReference
	evidence    ReviewEvidenceReference
	completedAt time.Time
}

func NewManualReviewCompletion(spec ManualReviewCompletionSpec) (ManualReviewCompletion, error) {
	if !spec.Authority.valid() || !spec.Reviewer.valid() ||
		!spec.Evidence.valid() || spec.CompletedAt.IsZero() {
		return ManualReviewCompletion{}, ErrInvalidManualReviewCompletion
	}
	return ManualReviewCompletion{
		authority:   spec.Authority,
		reviewer:    spec.Reviewer,
		evidence:    spec.Evidence,
		completedAt: spec.CompletedAt.UTC(),
	}, nil
}

func (completion ManualReviewCompletion) Authority() ReviewAuthorityReference {
	return completion.authority
}

func (completion ManualReviewCompletion) Reviewer() ReviewerReference {
	return completion.reviewer
}

func (completion ManualReviewCompletion) Evidence() ReviewEvidenceReference {
	return completion.evidence
}

func (completion ManualReviewCompletion) CompletedAt() time.Time {
	return completion.completedAt
}

func (completion ManualReviewCompletion) done() bool {
	return completion.authority.valid()
}

// ManualReviewCompletion 交回本任务上已完成的复核。未完成时报告缺席而不是给零值——零值与
// 「完成了但没带授权」在读的人眼里一样。
func (task AcceptanceDecisionTask) ManualReviewCompletion() (ManualReviewCompletion, bool) {
	return task.reviewCompletion, task.reviewCompletion.done()
}

// CompleteManualReview 在接受判断任务上记下一次已完成的人工复核。
//
// 它不判断复核该不该做——要不要复核由所采用规则包声明，记在商业依据快照里；这里只记录它做
// 完了。也不判断这个角色够不够格：授权规则属 party-commercial，本上下文保存所采用的授权
// 引用，实际角色目录仍是 BD-PS-002 的未确认参数。
//
// 决定已经越过提交边界后拒绝补录：那会让复核去追认一个已经形成的结果。同一提交版本也只
// 接受一次完成，第二次要么是重复提交要么是换人补签，两者都不该静默覆盖第一次的留痕。
func (request ShipmentRequest) CompleteManualReview(completion ManualReviewCompletion) (ShipmentRequest, error) {
	if !completion.done() || completion.completedAt.IsZero() {
		return ShipmentRequest{}, ErrInvalidManualReviewCompletion
	}
	if !request.acceptanceTask.running() {
		return ShipmentRequest{}, ErrAcceptanceTaskComplete
	}
	if request.acceptanceTask.reviewCompletion.done() {
		return ShipmentRequest{}, ErrManualReviewAlreadyCompleted
	}

	request.acceptanceTask.reviewCompletion = completion
	return request, nil
}

// ProcessingAttemptReason 指名一轮处理停在哪里。它是引用而非自由文本，取值由发起该轮的
// 编排给出——本上下文不在领域层复制一份应用侧的未决原因集合，那会是同一决定的第二处定义。
type ProcessingAttemptReason struct{ requiredValue }

func NewProcessingAttemptReason(value string) (ProcessingAttemptReason, error) {
	required, err := newRequiredValue("processing attempt reason", value)
	return ProcessingAttemptReason{required}, err
}

// ResumePath 区分这一轮该由谁来续，取值与 CONTEXT 接受判断任务的各等待态一一对应：
// 等待受控补充、等待内部续办、等待人工复核、等待运营登记、等待授权处置。CONTEXT 要求它们
// 「使用不同原因和续办路径」，因为续办方分别是客户、系统、授权复核角色、运营企业的登记动作
// 和授权处置角色，后续动作互不替代——通知客户并等新提交版本、重试依赖且绝不惊动客户、把复核
// 派给够格的角色、等运营企业按那份参数自己的登记路径补齐、把去向交给有处置权的角色选。合并
// 任意两个都会让等待对象弄错：压成一个字段，依赖抖动就会变成催客户补件；把复核算作内部重试，
// 则会永远重试一件重试推不动的事。
//
// `等待运营登记`（ADR-0094）与`等待内部续办`最容易压在一起，而两者的区别不在谁失败了而在
// **有没有可答的东西**：权威一时答不出会自行恢复，重试是对的；某个范围一条现行规则或参数都
// 没有登记时，权威并没有答不出，是根本没有可答的东西，重试一万次也长不出一条登记。压成一格
// 的代价是失败预算被一件重试永远推不动的事烧尽。
//
// `等待授权处置`（ADR-0132）与`等待人工复核`最容易压在一起，而两者的前置相反：复核的前置是
// 全部所需权威结果到齐**通过**、只差规则要求的复核；处置的前置是接受前财务控制按共同通过条件
// **不成立**、策略正文把去向交给授权角色。复核完成之后决定由任务按规则形成、可以是接受；处置
// 只选去向（`拒绝`或`交客户补充`），放行不在集内。把不通过的控制放进复核那一格，复核完成那条
// 路就会把一项`业务限制`推到接受——那正是 CONTEXT「人工处理不得绕过硬规则」禁的。
type ResumePath uint8

const (
	ResumePathInvalid ResumePath = iota
	ResumeByCustomerSupplement
	ResumeByInternalRetry
	ResumeByManualReview
	ResumeByOperatorRegistration
	ResumeByAuthorizedDisposition
)

func (path ResumePath) valid() bool {
	return path >= ResumeByCustomerSupplement && path <= ResumeByAuthorizedDisposition
}

func (path ResumePath) String() string {
	switch path {
	case ResumeByCustomerSupplement:
		return "CUSTOMER_SUPPLEMENT"
	case ResumeByInternalRetry:
		return "INTERNAL_RETRY"
	case ResumeByManualReview:
		return "MANUAL_REVIEW"
	case ResumeByOperatorRegistration:
		return "OPERATOR_REGISTRATION"
	case ResumeByAuthorizedDisposition:
		return "AUTHORIZED_DISPOSITION"
	default:
		return ""
	}
}

// SettlementNotFormedReason 是 settlement-accounting 答接受前财务控制`未形成`时交回的原因码原样（如
// ESTIMATE_PRICE_CARD_NOT_CONFIGURED）。它与 ProcessingAttemptReason 分开记：后者是本上下文的待判原因，同一个
// `财务控制未配置`下压着作用域、策略、金额、价卡几种缺口，运营要知道该去登哪一样，只能看提供方那一格（ADR-0383
// 决定三）。本上下文不解释、不复制结算的原因集合，只原样留下；零值即结算没答未形成。
type SettlementNotFormedReason struct{ requiredValue }

func NewSettlementNotFormedReason(value string) (SettlementNotFormedReason, error) {
	required, err := newRequiredValue("settlement not-formed reason", value)
	return SettlementNotFormedReason{required}, err
}

// ProcessingAttemptSpec 的 SettlementReason 与 ServiceProduct 可缺（ADR-0383 决定三）：结算没答未形成、这一版没申报服务产品时都不记，不代拟。
type ProcessingAttemptSpec struct {
	Reason       ProcessingAttemptReason
	ResumePath   ResumePath
	Continuation OwnershipContinuationReference
	AttemptedAt  time.Time
	// SettlementReason 只在结算答未形成时在场。
	SettlementReason SettlementNotFormedReason
	// ServiceProduct 是这一版申报的服务产品，取本上下文自己的提交版本，不经结算与计价转交。
	ServiceProduct DeclaredServiceProduct
	// EstimateGapCodes 是申报不全那一轮计价点名的缺项码原样、去重（ADR-0381 决定四），只在申报不全分出的
	// 两路等待上有；其余停法缺席。照来时顺序，不排序——码面不承诺序。
	EstimateGapCodes []string
}

// ProcessingAttempt 是接受判断任务上的一条处理记录。它记录本轮停在哪里、由谁来续、按哪个
// 引用续办，以及发生时间；它不是业务决定，因此累积多少条都不改变委托状态。
//
// 它不挂在聚合上：处理尝试经 AcceptanceJudgmentRecorder 落进自己的表，委托详情的「上次处理」从那里读
// （ADR-0383 决定四）。聚合上曾有一份处理尝试列表，生产从没写过它，读面却拿它当读源，于是「上次处理」恒空。
type ProcessingAttempt struct {
	reason           ProcessingAttemptReason
	resumePath       ResumePath
	continuation     OwnershipContinuationReference
	attemptedAt      time.Time
	settlementReason SettlementNotFormedReason
	serviceProduct   DeclaredServiceProduct
	estimateGapCodes []string
}

func NewProcessingAttempt(spec ProcessingAttemptSpec) (ProcessingAttempt, error) {
	if !spec.Reason.valid() || !spec.ResumePath.valid() ||
		!spec.Continuation.valid() || spec.AttemptedAt.IsZero() {
		return ProcessingAttempt{}, ErrInvalidProcessingAttempt
	}
	for _, code := range spec.EstimateGapCodes {
		// 空白码拒在构造门：缺项码出自计价的封闭集，空白只能是翻译坏了，落库会被列上的非空白约束拦下，
		// 把整笔事务拖死（迁移 0038 头注量过同一幕）。
		if strings.TrimSpace(code) == "" {
			return ProcessingAttempt{}, ErrInvalidProcessingAttempt
		}
	}
	return ProcessingAttempt{
		reason:           spec.Reason,
		resumePath:       spec.ResumePath,
		continuation:     spec.Continuation,
		attemptedAt:      spec.AttemptedAt.UTC(),
		settlementReason: spec.SettlementReason,
		serviceProduct:   spec.ServiceProduct,
		estimateGapCodes: append([]string(nil), spec.EstimateGapCodes...),
	}, nil
}

func (attempt ProcessingAttempt) Reason() ProcessingAttemptReason {
	return attempt.reason
}

func (attempt ProcessingAttempt) ResumePath() ResumePath {
	return attempt.resumePath
}

func (attempt ProcessingAttempt) ContinuationReference() OwnershipContinuationReference {
	return attempt.continuation
}

func (attempt ProcessingAttempt) AttemptedAt() time.Time {
	return attempt.attemptedAt
}

// SettlementReason 交回结算答未形成时的原因码；结算没答未形成时报告缺席。
func (attempt ProcessingAttempt) SettlementReason() (SettlementNotFormedReason, bool) {
	return attempt.settlementReason, attempt.settlementReason.valid()
}

// ServiceProduct 交回这一版申报的服务产品；零值即没申报。
func (attempt ProcessingAttempt) ServiceProduct() DeclaredServiceProduct {
	return attempt.serviceProduct
}

// EstimateGapCodes 交回申报不全那一轮计价点名的缺项码；空即这一轮没有（不是申报不全，或缺项没点出名字）。
func (attempt ProcessingAttempt) EstimateGapCodes() []string {
	return append([]string(nil), attempt.estimateGapCodes...)
}

// WaitingOn 交回本任务当前停在哪个等待态上；本轮形成了决定或任务已完成时报告缺席。
//
// 它由 Decide 在看过全部校验之后写下，而不是由编排回头推断：哪一类缺口该由谁来续是接受语言
// 的一部分，编排照它选原因与续办路径就行。编排自己推，就等于在应用层重做一遍这条判断。
func (task AcceptanceDecisionTask) WaitingOn() (ResumePath, bool) {
	return task.waitingOn, task.waitingOn.valid()
}

// AwaitOperatorRegistration 在**决定形成之前**把接受判断任务停在`等待运营登记`上。
//
// Decide 写等待态的前提是校验已经到场，而`判断时点未配置`那一族停在校验之前——时点都还没
// 形成，没有任何一条校验可以进 Decide。ADR-0094 Decision 五要求落此格前先把带等待态的聚合
// Save 落库（暂停没落库就不得交回该原因，否则等待态随本轮回滚蒸发而投递已被记为完毕，队列
// 从此列不出这份委托），于是这一格需要一条不经校验的转移。它只写等待态：不动状态、不形成
// 决定、不动版本，也不记处理尝试——那由编排经 AcceptanceJudgmentRecorder 另记，两者由编排各调一次。
//
// 与 Decide 的两道门一致：已越过决定边界的委托说出真实原因，不再进入任何等待态。
func (request ShipmentRequest) AwaitOperatorRegistration() (ShipmentRequest, error) {
	if request.decisionFormed {
		return ShipmentRequest{}, ErrDecisionAlreadyFormed
	}
	if request.state != ShipmentRequestSubmitted {
		return ShipmentRequest{}, ErrInvalidShipmentRequest
	}
	if !request.acceptanceTask.running() {
		return ShipmentRequest{}, ErrAcceptanceTaskComplete
	}
	request.acceptanceTask.waitingOn = ResumeByOperatorRegistration
	return request, nil
}

// AwaitCustomerSupplement 在**决定形成之前**把接受判断任务停在`等待受控补充`并记下缺口（ADR-0381 决定四）。
//
// Decide 写这一等待态的前提是校验到场，而申报不全停在接受前财务控制那一步——控制结果都没形成，没有校验可进
// Decide，于是与 AwaitOperatorRegistration 同理需要一条不经校验的转移（ADR-0094 Decision 五那道「先落库再交回」
// 的护栏由编排照 awaitOperatorRegistration 守）。缺口随等待态一起写：读面与受控补充入口都按「等待态 + 缺口」
// 说话，只停不记缺口，查阅的人只看得到「在等补充」，看不到缺的是哪一条、哪几箱。
//
// 缺口不得为空也不得有坏项：计价答申报不全必点名缺项，一格都点不出的那一路照旧内部重试（ADR-0381 决定一），
// 空缺口走到这里只能是调用方的翻译坏了——编程错误，不是一种可停的等待。
//
// 与 Decide 的两道门一致：已越过决定边界的委托说出真实原因，不再进入任何等待态。
func (request ShipmentRequest) AwaitCustomerSupplement(gaps []SupplementGap) (ShipmentRequest, error) {
	if len(gaps) == 0 {
		return ShipmentRequest{}, ErrInvalidAcceptanceCheck
	}
	for _, gap := range gaps {
		if !gap.group.valid() || !gap.reason.valid() {
			return ShipmentRequest{}, ErrInvalidAcceptanceCheck
		}
	}
	if request.decisionFormed {
		return ShipmentRequest{}, ErrDecisionAlreadyFormed
	}
	if request.state != ShipmentRequestSubmitted {
		return ShipmentRequest{}, ErrInvalidShipmentRequest
	}
	if !request.acceptanceTask.running() {
		return ShipmentRequest{}, ErrAcceptanceTaskComplete
	}
	request.acceptanceTask.waitingOn = ResumeByCustomerSupplement
	request.acceptanceTask.supplementGaps = append([]SupplementGap(nil), gaps...)
	return request, nil
}
