package domain

import (
	"errors"
	"fmt"
	"time"
)

var ErrInvalidLegalEntityProfileResolution = errors.New("party commercial: invalid legal entity profile resolution")

// LegalEntityProfileResolutionOutcome 是「按时点解析法人资料」的答案代数（ADR-0145 决定五、六）。只有`已解析`
// 是成功；其余各格都明确非成功，开立方据此拒绝开立——不以默认值补齐，也不退回去用别的修订。分格交回是因为
// 续办不同：`资料不全`要补登资料修订，法人未登记、未生效或已停用要换一个法人或改开立时点。
type LegalEntityProfileResolutionOutcome uint8

const (
	LegalEntityProfileResolutionOutcomeInvalid LegalEntityProfileResolutionOutcome = iota
	LegalEntityProfileResolved
	LegalEntityProfileIncomplete
	LegalEntityProfileEntityNotRegistered
	LegalEntityProfileEntityNotEffective
	LegalEntityProfileEntityDeactivated
)

func (outcome LegalEntityProfileResolutionOutcome) String() string {
	switch outcome {
	case LegalEntityProfileResolved:
		return "RESOLVED"
	case LegalEntityProfileIncomplete:
		return "PROFILE_INCOMPLETE"
	case LegalEntityProfileEntityNotRegistered:
		return "LEGAL_ENTITY_NOT_REGISTERED"
	case LegalEntityProfileEntityNotEffective:
		return "LEGAL_ENTITY_NOT_EFFECTIVE"
	case LegalEntityProfileEntityDeactivated:
		return "LEGAL_ENTITY_DEACTIVATED"
	default:
		return ""
	}
}

// LegalEntityProfileIncompleteCause 是`资料不全`的成因：解析时点没有有效的修订，或有效的那笔没带开票资料。
type LegalEntityProfileIncompleteCause uint8

const (
	LegalEntityProfileIncompleteCauseInvalid LegalEntityProfileIncompleteCause = iota
	LegalEntityProfileNoEffectiveRevision
	LegalEntityProfileNoInvoicingDetails
)

func (cause LegalEntityProfileIncompleteCause) String() string {
	switch cause {
	case LegalEntityProfileNoEffectiveRevision:
		return "NO_EFFECTIVE_REVISION"
	case LegalEntityProfileNoInvoicingDetails:
		return "NO_INVOICING_DETAILS"
	default:
		return ""
	}
}

// LegalEntityProfileResolution 是一次解析的答案。
type LegalEntityProfileResolution struct {
	outcome     LegalEntityProfileResolutionOutcome
	cause       LegalEntityProfileIncompleteCause
	revision    LegalEntityProfileRevision
	hasRevision bool
}

// NotRegisteredLegalEntityProfileResolution 是法人从未登记时的答案：没有登记可交给 ResolveLegalEntityProfile，
// 由调用方直接给出这一格。
func NotRegisteredLegalEntityProfileResolution() LegalEntityProfileResolution {
	return LegalEntityProfileResolution{outcome: LegalEntityProfileEntityNotRegistered}
}

func (resolution LegalEntityProfileResolution) Outcome() LegalEntityProfileResolutionOutcome {
	return resolution.outcome
}

// IncompleteCause 只在`资料不全`时有意义。
func (resolution LegalEntityProfileResolution) IncompleteCause() LegalEntityProfileIncompleteCause {
	return resolution.cause
}

// Resolved 只在`已解析`时交回那一笔修订；开立方固定的是它的 Reference()。
func (resolution LegalEntityProfileResolution) Resolved() (LegalEntityProfileRevision, bool) {
	if resolution.outcome != LegalEntityProfileResolved {
		return LegalEntityProfileRevision{}, false
	}
	return resolution.revision, true
}

// EffectiveRevision 交回解析时点有效的那一笔，不论它能不能用——`资料不全`因缺开票资料时，读的人要知道该给
// 哪一笔之后补登。开立方不得拿它开立，开立只认 Resolved。
func (resolution LegalEntityProfileResolution) EffectiveRevision() (LegalEntityProfileRevision, bool) {
	return resolution.revision, resolution.hasRevision
}

// ResolveLegalEntityProfile 回答某个责任法人在明确时点的法人资料（ADR-0145 决定五、六；CONTEXT Lifecycles
// 「法人资料」）。entity 是该法人在登记册上的最新修订，chain 是它的全部资料修订，次序不限。
//
// 有效修订的判据取自 CONTEXT「后一修订生效时，前一修订自该时点起不再参与新的解析」：在生效时点不晚于解析
// 时点的修订里取修订号最大的那一笔。所以追溯生效的修订一经登记，就取代它生效时点之后的全部前序修订——
// 包括生效时点比它晚、修订号比它小的那些；未来生效的修订在生效前不参与。按生效时点取最晚的那一笔是另一种
// 读法，它会让被取代的修订在追溯修订之后又冒出来，与那一句相悖。
//
// 法人状态按解析时点读：停用自停用时点起不再参与新的解析；生效时点未到的法人还没有资格对外开立。
func ResolveLegalEntityProfile(
	entity LegalEntityRegistration,
	chain []LegalEntityProfileRevision,
	at time.Time,
) (LegalEntityProfileResolution, error) {
	if at.IsZero() {
		return LegalEntityProfileResolution{}, fmt.Errorf("%w: resolution time is required", ErrInvalidLegalEntityProfileResolution)
	}
	tenant, id := entity.Entity().Tenant(), entity.Entity().ID()
	if !tenant.valid() || !id.valid() {
		return LegalEntityProfileResolution{}, fmt.Errorf("%w: legal entity registration is required", ErrInvalidLegalEntityProfileResolution)
	}
	switch status := entity.Lifecycle().StatusAt(at); status {
	case IdentityEffective:
	case IdentityDeactivated:
		return LegalEntityProfileResolution{outcome: LegalEntityProfileEntityDeactivated}, nil
	case IdentityRegistered:
		return LegalEntityProfileResolution{outcome: LegalEntityProfileEntityNotEffective}, nil
	default:
		return LegalEntityProfileResolution{}, fmt.Errorf("%w: legal entity lifecycle is unreadable", ErrInvalidLegalEntityProfileResolution)
	}

	seen := make(map[int]bool, len(chain))
	var effective LegalEntityProfileRevision
	found := false
	for _, revision := range chain {
		if revision.tenant != tenant || revision.entity != id || revision.revision < 1 {
			return LegalEntityProfileResolution{}, fmt.Errorf(
				"%w: revision %d does not belong to legal entity %s", ErrInvalidLegalEntityProfileResolution, revision.revision, id)
		}
		// 同一修订号出现两次说明读侧没有按主键取数，照收下会让答案随取数次序变。
		if seen[revision.revision] {
			return LegalEntityProfileResolution{}, fmt.Errorf(
				"%w: revision %d appears twice", ErrInvalidLegalEntityProfileResolution, revision.revision)
		}
		seen[revision.revision] = true
		if revision.effectiveFrom.After(at) {
			continue
		}
		if !found || revision.revision > effective.revision {
			effective, found = revision, true
		}
	}
	if !found {
		return LegalEntityProfileResolution{outcome: LegalEntityProfileIncomplete, cause: LegalEntityProfileNoEffectiveRevision}, nil
	}
	if !effective.content.hasInvoicing {
		return LegalEntityProfileResolution{
			outcome:     LegalEntityProfileIncomplete,
			cause:       LegalEntityProfileNoInvoicingDetails,
			revision:    effective,
			hasRevision: true,
		}, nil
	}
	return LegalEntityProfileResolution{outcome: LegalEntityProfileResolved, revision: effective, hasRevision: true}, nil
}
