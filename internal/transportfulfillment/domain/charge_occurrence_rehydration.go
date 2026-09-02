package domain

import "time"

// 运输收费发生项的重建门（ADR-0028）。
//
// 与构造门 `FormTransportChargeOccurrence` 分属两扇：那扇从零固定全部依据，每条不变量当场
// 算；从库里读回一个**已经被修订过**的版本走不了它——修订四件是那次修订留下的事实，构造门
// 既不接受它们，重放 `ReviseValidity` 又等于拿今天的输入去追认昨天那次修订。
//
// 本门验形状与成对关系，**不重走转换门**：不重算「此刻该不该修订」，只核带回来的东西自身
// 立不立得住。行内比得出来的那两条（沿用原版本号是覆盖、修订时刻不早于发生时间）要核——
// 它们不是重放，是拿同一行上的两个值作比对。

// RehydrateChargeOccurrenceSpec 是一个发生项版本在库面的样子。
//
// 修订四件（前身、走向、依据、时刻）同在或同缺：`Revision()` 按 `RevisedAt` 是否零值给出，
// 半截会重建出一个既非首版又非修订版的东西。
type RehydrateChargeOccurrenceSpec struct {
	TenantID    TenantID
	Occurrence  ChargeOccurrenceReference
	Journey     JourneyReference
	LegalEntity ProcurementLegalEntityReference
	Provider    ServiceProviderReference
	Agreement   AgreementSnapshotReference
	Reason      ChargeOccurrenceReason
	FactBasis   OccurrenceBasisReference
	Scope       OccurrenceScopeReference
	Members     []CarriedObjectReference
	Quantity    int64
	Unit        QuantityUnitReference
	OccurredAt  time.Time
	Validity    OccurrenceValidityVersion

	Corrects      OccurrenceValidityVersion
	RevisionKind  OccurrenceRevisionKind
	RevisionBasis OccurrenceBasisReference
	RevisedAt     time.Time
}

// RehydrateChargeOccurrence 从库面重建一个发生项版本。
func RehydrateChargeOccurrence(spec RehydrateChargeOccurrenceSpec) (TransportChargeOccurrence, error) {
	// 本体那一半的判据与构造门同一套，复用它而不是抄一遍——抄一遍就是为同一形状立第二个
	// 口径，构造门改一次判据这里会悄悄漂移。
	occurrence, err := FormTransportChargeOccurrence(TransportChargeOccurrenceSpec{
		TenantID:    spec.TenantID,
		Occurrence:  spec.Occurrence,
		Journey:     spec.Journey,
		LegalEntity: spec.LegalEntity,
		Provider:    spec.Provider,
		Agreement:   spec.Agreement,
		Reason:      spec.Reason,
		FactBasis:   spec.FactBasis,
		Scope:       spec.Scope,
		Members:     spec.Members,
		Quantity:    spec.Quantity,
		Unit:        spec.Unit,
		OccurredAt:  spec.OccurredAt,
		Validity:    spec.Validity,
	})
	if err != nil {
		return TransportChargeOccurrence{}, err
	}

	revised := !spec.RevisedAt.IsZero()
	if revised != spec.Corrects.valid() ||
		revised != spec.RevisionKind.valid() ||
		revised != spec.RevisionBasis.valid() {
		return TransportChargeOccurrence{}, ErrInvalidChargeOccurrence
	}
	if !revised {
		return occurrence, nil
	}

	// 这两条是行内比对不是重放：同一行上的两个值自相矛盾时，那一行本身就立不住。
	if spec.Corrects == spec.Validity {
		return TransportChargeOccurrence{}, ErrInvalidChargeOccurrence
	}
	if spec.RevisedAt.Before(spec.OccurredAt) {
		return TransportChargeOccurrence{}, ErrInvalidChargeOccurrence
	}

	occurrence.corrects = spec.Corrects
	occurrence.revisionKind = spec.RevisionKind
	occurrence.revisionBasis = spec.RevisionBasis
	occurrence.revisedAt = spec.RevisedAt.UTC()
	return occurrence, nil
}
