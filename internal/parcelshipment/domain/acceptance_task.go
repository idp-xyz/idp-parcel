package domain

import (
	"errors"
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

// ResumePath 区分这一轮该由谁来续，取值与 CONTEXT 接受判断任务的四个等待态一一对应：
// 等待受控补充、等待内部续办、等待人工复核、等待运营登记。CONTEXT 要求四者「使用不同原因
// 和续办路径」，因为续办方分别是客户、系统、授权复核角色和运营企业的登记动作，后续动作互不
// 替代——通知客户并等新提交版本、重试依赖且绝不惊动客户、把复核派给够格的角色、等运营企业
// 按那份参数自己的登记路径补齐。合并任意两个都会让等待对象弄错：压成一个字段，依赖抖动就会
// 变成催客户补件；把复核算作内部重试，则会永远重试一件重试推不动的事。
//
// `等待运营登记`是第四格（ADR-0094）。它与`等待内部续办`最容易压在一起，而两者的区别不在
// 谁失败了而在**有没有可答的东西**：权威一时答不出会自行恢复，重试是对的；某个范围一条现行
// 规则或参数都没有登记时，权威并没有答不出，是根本没有可答的东西，重试一万次也长不出一条
// 登记。压成一格的代价是失败预算被一件重试永远推不动的事烧尽。
type ResumePath uint8

const (
	ResumePathInvalid ResumePath = iota
	ResumeByCustomerSupplement
	ResumeByInternalRetry
	ResumeByManualReview
	ResumeByOperatorRegistration
)

func (path ResumePath) valid() bool {
	return path >= ResumeByCustomerSupplement && path <= ResumeByOperatorRegistration
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
	default:
		return ""
	}
}

type ProcessingAttemptSpec struct {
	Reason       ProcessingAttemptReason
	ResumePath   ResumePath
	Continuation OwnershipContinuationReference
	AttemptedAt  time.Time
}

// ProcessingAttempt 是接受判断任务上的一条处理记录。它记录本轮停在哪里、由谁来续、按哪个
// 引用续办，以及发生时间；它不是业务决定，因此累积多少条都不改变委托状态。
type ProcessingAttempt struct {
	reason       ProcessingAttemptReason
	resumePath   ResumePath
	continuation OwnershipContinuationReference
	attemptedAt  time.Time
}

func NewProcessingAttempt(spec ProcessingAttemptSpec) (ProcessingAttempt, error) {
	if !spec.Reason.valid() || !spec.ResumePath.valid() ||
		!spec.Continuation.valid() || spec.AttemptedAt.IsZero() {
		return ProcessingAttempt{}, ErrInvalidProcessingAttempt
	}
	return ProcessingAttempt{
		reason:       spec.Reason,
		resumePath:   spec.ResumePath,
		continuation: spec.Continuation,
		attemptedAt:  spec.AttemptedAt.UTC(),
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

// WaitingOn 交回本任务当前停在哪个等待态上；本轮形成了决定或任务已完成时报告缺席。
//
// 它由 Decide 在看过全部校验之后写下，而不是由编排回头推断：哪一类缺口该由谁来续是接受语言
// 的一部分，编排照它选原因与续办路径就行。编排自己推，就等于在应用层重做一遍这条判断。
func (task AcceptanceDecisionTask) WaitingOn() (ResumePath, bool) {
	return task.waitingOn, task.waitingOn.valid()
}

// ProcessingAttempts 交回本任务累积的全部处理记录，按发生顺序。返回副本：追加是聚合的
// 事，调用方拿到的切片改不动历史。
func (task AcceptanceDecisionTask) ProcessingAttempts() []ProcessingAttempt {
	return append([]ProcessingAttempt(nil), task.processingAttempts...)
}

// RecordProcessingAttempt 在接受判断任务上追加一条处理记录。
//
// 追加而不替换：用例要求未决按原因分类统计，只留最近一条就说不出这份委托卡过几轮、各卡在
// 哪里。已完成的任务拒绝追加——决定已经越过提交边界，再累积记录会让一份已决委托看起来还
// 在处理中。
func (request ShipmentRequest) RecordProcessingAttempt(attempt ProcessingAttempt) (ShipmentRequest, error) {
	if !attempt.valid() {
		return ShipmentRequest{}, ErrInvalidProcessingAttempt
	}
	// 已完成与已停止都不再接受追加：前者决定已经越过提交边界，后者撤回已经成立，两种情况
	// 下继续累积记录都会让一份不再判断的委托看起来还在处理中。
	if !request.acceptanceTask.running() {
		return ShipmentRequest{}, ErrAcceptanceTaskComplete
	}

	appended := append([]ProcessingAttempt(nil), request.acceptanceTask.processingAttempts...)
	request.acceptanceTask.processingAttempts = append(appended, attempt)
	return request, nil
}

func (attempt ProcessingAttempt) valid() bool {
	return attempt.reason.valid() && attempt.resumePath.valid() &&
		attempt.continuation.valid() && !attempt.attemptedAt.IsZero()
}

// AwaitOperatorRegistration 在**决定形成之前**把接受判断任务停在`等待运营登记`上。
//
// Decide 写等待态的前提是校验已经到场，而`判断时点未配置`那一族停在校验之前——时点都还没
// 形成，没有任何一条校验可以进 Decide。ADR-0094 Decision 五要求落此格前先把带等待态的聚合
// Save 落库（暂停没落库就不得交回该原因，否则等待态随本轮回滚蒸发而投递已被记为完毕，队列
// 从此列不出这份委托），于是这一格需要一条不经校验的转移。它只写等待态：不动状态、不形成
// 决定、不动版本，也不追加处理记录——那是 RecordProcessingAttempt 的事，两者由编排各调一次。
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
