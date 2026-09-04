package domain

import (
	"errors"
	"strings"
	"time"
)

var (
	ErrInvalidEvidence          = errors.New("visibility exception: invalid evidence item")
	ErrInvalidDisclosureVersion = errors.New("visibility exception: invalid evidence disclosure version")
)

// EvidenceItemID 是证据项的标识。同一证据项可以被异常案件、客户索赔项和追偿事项
// 分别引用（CONTEXT 硬句 170）——引用在各对象上，这里是被引的本体。
type EvidenceItemID struct{ requiredValue }

func NewEvidenceItemID(value string) (EvidenceItemID, error) {
	required, err := newRequiredValue("evidence item ID", value)
	return EvidenceItemID{required}, err
}

// EvidenceProviderReference 指名提交证据的客户、合作伙伴或内部来源。
type EvidenceProviderReference struct{ requiredValue }

func NewEvidenceProviderReference(value string) (EvidenceProviderReference, error) {
	required, err := newRequiredValue("evidence provider reference", value)
	return EvidenceProviderReference{required}, err
}

// EvidenceContentDigest 指名证据内容的稳定指纹——对外披露的脱敏版本靠它对回原件，
// 来源不明、内容不一致的附件复制不出这个对应关系。
type EvidenceContentDigest struct{ requiredValue }

func NewEvidenceContentDigest(value string) (EvidenceContentDigest, error) {
	required, err := newRequiredValue("evidence content digest", value)
	return EvidenceContentDigest{required}, err
}

// EvidenceAppraisal 是证据评价的封闭三值：已收到（默认——材料收到不证明陈述成立，
// CONTEXT 硬句 171）、经调查采信、经调查不采信。
type EvidenceAppraisal uint8

const (
	EvidenceAppraisalInvalid EvidenceAppraisal = iota
	EvidenceReceived
	EvidenceCredited
	EvidenceDiscredited
)

func (appraisal EvidenceAppraisal) valid() bool {
	return appraisal >= EvidenceReceived && appraisal <= EvidenceDiscredited
}

func (appraisal EvidenceAppraisal) String() string {
	switch appraisal {
	case EvidenceReceived:
		return "RECEIVED"
	case EvidenceCredited:
		return "CREDITED"
	case EvidenceDiscredited:
		return "DISCREDITED"
	default:
		return ""
	}
}

// EvidenceItem 是一项证据。提交只表示材料已经收到——评价起点恒为`已收到`，采信是
// 之后的显式判断（171）；证据冲突保留双方进入调查，这里没有删除入口。
type EvidenceItem struct {
	id             EvidenceItemID
	provider       EvidenceProviderReference
	digest         EvidenceContentDigest
	submittedAt    time.Time
	appraisal      EvidenceAppraisal
	appraisalBasis string
}

// SubmitEvidence 受理一项证据：评价起点是`已收到`，不由调用方指定——「提交即采信」
// 在构造上就不可能。
func SubmitEvidence(
	id EvidenceItemID,
	provider EvidenceProviderReference,
	digest EvidenceContentDigest,
	submittedAt time.Time,
) (EvidenceItem, error) {
	if !id.valid() || !provider.valid() || !digest.valid() || submittedAt.IsZero() {
		return EvidenceItem{}, ErrInvalidEvidence
	}
	return EvidenceItem{
		id:          id,
		provider:    provider,
		digest:      digest,
		submittedAt: submittedAt.UTC(),
		appraisal:   EvidenceReceived,
	}, nil
}

func (item EvidenceItem) ID() EvidenceItemID {
	return item.id
}

func (item EvidenceItem) Provider() EvidenceProviderReference {
	return item.provider
}

// SubmittedAt 是材料被收到的时刻——「证据项保存……取得时间」（CONTEXT）；它不是材料
// 所陈述事实的发生时间。
func (item EvidenceItem) SubmittedAt() time.Time {
	return item.submittedAt
}

// EvidenceItemSnapshot 是持久化层落与重建证据项所需的全量状态。评价与依据是已作出
// 的判断，随快照携带，不由持久化层重演 Appraise。
type EvidenceItemSnapshot struct {
	ID             EvidenceItemID
	Provider       EvidenceProviderReference
	Digest         EvidenceContentDigest
	SubmittedAt    time.Time
	Appraisal      EvidenceAppraisal
	AppraisalBasis string
}

// Snapshot 折出证据项的全量状态供持久化。
func (item EvidenceItem) Snapshot() EvidenceItemSnapshot {
	return EvidenceItemSnapshot{
		ID:             item.id,
		Provider:       item.provider,
		Digest:         item.digest,
		SubmittedAt:    item.submittedAt,
		Appraisal:      item.appraisal,
		AppraisalBasis: item.appraisalBasis,
	}
}

// RehydrateEvidenceItem 从快照重建证据项。读回的东西同样要过一遍不变量——评价在封闭
// 三值内、经调查的评价必带依据而`已收到`必不带——一次坏写入不得变成一个看起来合法的
// 证据项。这扇门只对持久化适配器开放：它相信快照里的评价是当初经调查作出的，从别处
// 灌一份进来就等于绕过「提交即采信在构造上不可能」那条。
func RehydrateEvidenceItem(snapshot EvidenceItemSnapshot) (EvidenceItem, error) {
	if !snapshot.ID.valid() || !snapshot.Provider.valid() || !snapshot.Digest.valid() ||
		snapshot.SubmittedAt.IsZero() || !snapshot.Appraisal.valid() {
		return EvidenceItem{}, ErrInvalidEvidence
	}
	if (snapshot.Appraisal == EvidenceReceived) != (snapshot.AppraisalBasis == "") {
		return EvidenceItem{}, ErrInvalidEvidence
	}
	return EvidenceItem{
		id:             snapshot.ID,
		provider:       snapshot.Provider,
		digest:         snapshot.Digest,
		submittedAt:    snapshot.SubmittedAt.UTC(),
		appraisal:      snapshot.Appraisal,
		appraisalBasis: snapshot.AppraisalBasis,
	}, nil
}

func (item EvidenceItem) Digest() EvidenceContentDigest {
	return item.digest
}

func (item EvidenceItem) Appraisal() EvidenceAppraisal {
	return item.appraisal
}

// AppraisalBasis 只在经调查的评价上给出。
func (item EvidenceItem) AppraisalBasis() (string, bool) {
	return item.appraisalBasis, item.appraisal != EvidenceReceived
}

// Appraise 经调查形成采信或不采信：依据必备（没有依据的采信与提交即采信分不开）；
// 已评价不再评价——异议走调查与裁决，不走改写。
func (item EvidenceItem) Appraise(appraisal EvidenceAppraisal, basis string, at time.Time) (EvidenceItem, error) {
	if item.appraisal != EvidenceReceived {
		return EvidenceItem{}, ErrInvalidEvidence
	}
	if appraisal != EvidenceCredited && appraisal != EvidenceDiscredited {
		return EvidenceItem{}, ErrInvalidEvidence
	}
	if basis == "" || at.IsZero() || at.Before(item.submittedAt) {
		return EvidenceItem{}, ErrInvalidEvidence
	}
	appraised := item
	appraised.appraisal = appraisal
	appraised.appraisalBasis = basis
	return appraised, nil
}

// EvidenceDisclosureVersion 是证据的对外披露版本：明确披露范围或脱敏版本（CONTEXT
// 硬句 170 后半），锚定原件指纹——「来源不明、内容不一致的附件」造不出与原件的对应。
type EvidenceDisclosureVersion struct {
	item       EvidenceItemID
	original   EvidenceContentDigest
	redacted   EvidenceContentDigest
	scope      string
	preparedAt time.Time
}

// PrepareDisclosure 形成一个披露版本：披露范围必备（空白同缺席，与 requiredValue 一个
// 口径）、脱敏指纹不得与原件指纹相同（相同即原件外流，范围声明成了空话）。
func PrepareDisclosure(
	item EvidenceItem,
	redacted EvidenceContentDigest,
	scope string,
	preparedAt time.Time,
) (EvidenceDisclosureVersion, error) {
	if !item.id.valid() || !redacted.valid() || strings.TrimSpace(scope) == "" || preparedAt.IsZero() {
		return EvidenceDisclosureVersion{}, ErrInvalidDisclosureVersion
	}
	if redacted == item.digest {
		return EvidenceDisclosureVersion{}, ErrInvalidDisclosureVersion
	}
	return EvidenceDisclosureVersion{
		item:       item.id,
		original:   item.digest,
		redacted:   redacted,
		scope:      scope,
		preparedAt: preparedAt.UTC(),
	}, nil
}

func (version EvidenceDisclosureVersion) Item() EvidenceItemID {
	return version.item
}

// Original 是原件指纹——披露版本永远对得回它锚定的原件。
func (version EvidenceDisclosureVersion) Original() EvidenceContentDigest {
	return version.original
}

func (version EvidenceDisclosureVersion) Redacted() EvidenceContentDigest {
	return version.redacted
}

func (version EvidenceDisclosureVersion) Scope() string {
	return version.scope
}

// PreparedAt 是披露版本准备完成的时刻——准备完成不等于已对外提交（`AT-VE-132`），
// 对外提交、送达与确认是追偿动作或通知那一侧分别记录的节点，不在证据版本上。
func (version EvidenceDisclosureVersion) PreparedAt() time.Time {
	return version.preparedAt
}
