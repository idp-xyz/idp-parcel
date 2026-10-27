package domain

import (
	"errors"
	"fmt"
	"time"
)

// ErrPublicationDraftNotReadable 表示库里这一行载体的快照文档本构建折不回正文：规范化版本本构建不认得
// （ErrCanonicalizationUnsupported，滚动发布或回滚时旧构建读新构建写的行），或文档里的正文过不了本构建的正文构造门
// （如 ErrVolumetricFactorOnPricePolicy，政策收窄之前录下的行）。
//
// 它与 ErrInvalidRehydratedPublicationDraft 是两格：后者是行本身坏了，处置是去查库；这一格是行没坏、只是本构建读不了它，
// 恢复动作落在操作者手里——`待批准`的按同一版本重新录入（登记册的修订只在待批准期间替换行），`已批准`的内容已固定、换版本号再录。
// 重发同一个请求不会好，所以上层答一格业务结论，不答技术失败（ADR-0022、ADR-0029）。
var ErrPublicationDraftNotReadable = errors.New("party commercial: publication draft cannot be read back by this build")

// UnreadablePublicationDraft 是本构建折不回正文的那一行在库上剩下的事实：版本壳、状态与各痕迹、列上的规范化版本与摘要，
// 加折不回来的成因。它不是载体——没有正文，交不出发布规格，也推进不了状态；上层只拿它认出是哪一版、在哪一格、
// 调用方看到的摘要是不是它，再答业务格。它同时是 error，错误链既认 ErrPublicationDraftNotReadable 也认成因。
type UnreadablePublicationDraft struct {
	shell            CommercialVersion
	canonicalization string
	submitter        OperatorSubjectReference
	submittedAt      time.Time
	status           PublicationDraftStatus
	approver         OperatorSubjectReference
	approvedAt       time.Time
	publishedAt      time.Time
	cause            error
}

func (unreadable *UnreadablePublicationDraft) Error() string {
	return fmt.Sprintf("%v: %v", ErrPublicationDraftNotReadable, unreadable.cause)
}

func (unreadable *UnreadablePublicationDraft) Unwrap() error { return unreadable.cause }

func (unreadable *UnreadablePublicationDraft) Is(target error) bool {
	return target == ErrPublicationDraftNotReadable
}

// Cause 交回折不回来的成因：错误链里认得 ErrCanonicalizationUnsupported 或正文构造门的拒因。
func (unreadable *UnreadablePublicationDraft) Cause() error { return unreadable.cause }

func (unreadable *UnreadablePublicationDraft) Tenant() TenantID { return unreadable.shell.tenant }
func (unreadable *UnreadablePublicationDraft) Kind() CommercialObjectKind {
	return unreadable.shell.kind
}
func (unreadable *UnreadablePublicationDraft) ObjectID() CommercialObjectID {
	return unreadable.shell.objectID
}
func (unreadable *UnreadablePublicationDraft) Version() CommercialVersionLabel {
	return unreadable.shell.version
}
func (unreadable *UnreadablePublicationDraft) Scope() CommercialScopeReference {
	return unreadable.shell.scope
}
func (unreadable *UnreadablePublicationDraft) Effective() EffectiveInterval {
	return unreadable.shell.effective
}
func (unreadable *UnreadablePublicationDraft) DeclaredReferences() []DeclaredReference {
	return unreadable.shell.DeclaredReferences()
}

// ShellDigest 与读得回的载体同一算法同一串：调用方看到的壳摘要与它比，不因行读不回而失去绑定。
func (unreadable *UnreadablePublicationDraft) ShellDigest() string {
	return unreadable.shell.shellDigest()
}

// ContentDigest 是列上的内容摘要原值：正文折不回来，无从重算，列上这一串就是这一行的内容身份。
func (unreadable *UnreadablePublicationDraft) ContentDigest() CommercialContentDigest {
	return unreadable.shell.contentDigest
}

// Canonicalization 是列上的规范化版本原值，可以是本构建不认得的那一个。
func (unreadable *UnreadablePublicationDraft) Canonicalization() string {
	return unreadable.canonicalization
}

func (unreadable *UnreadablePublicationDraft) Submitter() OperatorSubjectReference {
	return unreadable.submitter
}
func (unreadable *UnreadablePublicationDraft) SubmittedAt() time.Time { return unreadable.submittedAt }
func (unreadable *UnreadablePublicationDraft) Status() PublicationDraftStatus {
	return unreadable.status
}

func (unreadable *UnreadablePublicationDraft) Approver() (OperatorSubjectReference, bool) {
	return unreadable.approver, unreadable.approver.valid()
}

func (unreadable *UnreadablePublicationDraft) ApprovedAt() (time.Time, bool) {
	if unreadable.approvedAt.IsZero() {
		return time.Time{}, false
	}
	return unreadable.approvedAt, true
}

func (unreadable *UnreadablePublicationDraft) PublishedAt() (time.Time, bool) {
	if unreadable.publishedAt.IsZero() {
		return time.Time{}, false
	}
	return unreadable.publishedAt, true
}
