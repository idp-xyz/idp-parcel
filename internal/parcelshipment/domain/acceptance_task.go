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
	if request.acceptanceTask.complete {
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

// ResumePath 区分这一轮该由谁来续，取值与 CONTEXT 接受判断任务的三个等待态一一对应：
// 等待受控补充、等待内部续办、等待人工复核。CONTEXT 要求三者「使用不同原因和续办路径」，
// 因为续办方分别是客户、系统和授权复核角色，后续动作互不替代——通知客户并等新提交版本、
// 重试依赖且绝不惊动客户、把复核派给够格的角色。合并任意两个都会让等待对象弄错：压成一个
// 字段，依赖抖动就会变成催客户补件；把复核算作内部重试，则会永远重试一件重试推不动的事。
type ResumePath uint8

const (
	ResumePathInvalid ResumePath = iota
	ResumeByCustomerSupplement
	ResumeByInternalRetry
	ResumeByManualReview
)

func (path ResumePath) valid() bool {
	return path >= ResumeByCustomerSupplement && path <= ResumeByManualReview
}

func (path ResumePath) String() string {
	switch path {
	case ResumeByCustomerSupplement:
		return "CUSTOMER_SUPPLEMENT"
	case ResumeByInternalRetry:
		return "INTERNAL_RETRY"
	case ResumeByManualReview:
		return "MANUAL_REVIEW"
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
	if request.acceptanceTask.complete {
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
