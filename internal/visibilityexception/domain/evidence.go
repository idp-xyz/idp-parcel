package domain

import (
	"errors"
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

// PrepareDisclosure 形成一个披露版本：披露范围必备、脱敏指纹不得与原件指纹相同
// （相同即原件外流，范围声明成了空话）。
func PrepareDisclosure(
	item EvidenceItem,
	redacted EvidenceContentDigest,
	scope string,
	preparedAt time.Time,
) (EvidenceDisclosureVersion, error) {
	if !item.id.valid() || !redacted.valid() || scope == "" || preparedAt.IsZero() {
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
