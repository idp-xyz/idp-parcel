package domain

import "time"

// RehydrateCustomerClaimAmountSpec 是索赔金额行在库里的样子。规则目录是否配置是编
// 排视图的事，读回只复验行自身形状（两族不混、正金额、责任结论在场）。
type RehydrateCustomerClaimAmountSpec struct {
	ID             CustomerClaimAmountID
	Kind           CustomerClaimAmountKind
	ClaimItem      ClaimItemReference
	Responsibility ResponsibilityConclusionReference
	RuleVersion    AmountRuleVersionReference
	LegalEntity    LegalEntityReference
	OriginalCharge CustomerChargeID
	Currency       CurrencyCode
	AmountMinor    int64
	Period         BillingPeriodReference
	FormedAt       time.Time
}

func RehydrateCustomerClaimAmount(spec RehydrateCustomerClaimAmountSpec) (CustomerClaimAmount, error) {
	return FormCustomerClaimAmount(CustomerClaimAmountSpec{
		ID:             spec.ID,
		Kind:           spec.Kind,
		ClaimItem:      spec.ClaimItem,
		Responsibility: spec.Responsibility,
		RuleVersion:    spec.RuleVersion,
		LegalEntity:    spec.LegalEntity,
		OriginalCharge: spec.OriginalCharge,
		Currency:       spec.Currency,
		AmountMinor:    spec.AmountMinor,
		Period:         spec.Period,
		FormedAt:       spec.FormedAt,
	})
}

// RehydrateRecoveryReceivableSpec 是应追偿行在库里的样子。责任结论是否仍成立是写入
// 时已经判过的，读回不重审。
type RehydrateRecoveryReceivableSpec struct {
	ID             RecoveryReceivableID
	Matter         RecoveryMatterReference
	Responsibility ResponsibilityConclusionReference
	Counterparty   RecoveryCounterpartyReference
	RuleVersion    AmountRuleVersionReference
	LegalEntity    LegalEntityReference
	Currency       CurrencyCode
	AmountMinor    int64
	FormedAt       time.Time
}

func RehydrateRecoveryReceivable(spec RehydrateRecoveryReceivableSpec) (RecoveryReceivable, error) {
	return FormRecoveryReceivable(RecoveryReceivableSpec{
		ID:             spec.ID,
		Matter:         spec.Matter,
		Responsibility: spec.Responsibility,
		Counterparty:   spec.Counterparty,
		RuleVersion:    spec.RuleVersion,
		LegalEntity:    spec.LegalEntity,
		Currency:       spec.Currency,
		AmountMinor:    spec.AmountMinor,
		FormedAt:       spec.FormedAt,
	})
}

// RehydrateRecoveryAcknowledgementSpec 是认可行在库里的样子。AcknowledgeRecovery
// 要一份应追偿才能限量并对形成时刻，而行里只有认可本身——应追偿在场与上限是写入时
// 已经判过的。
type RehydrateRecoveryAcknowledgementSpec struct {
	ID                AcknowledgementID
	Receivable        RecoveryReceivableID
	Response          CounterpartyResponseReference
	Standing          ResponseStanding
	Currency          CurrencyCode
	AcknowledgedMinor int64
	ReceivableMinor   int64
	AcknowledgedAt    time.Time
}

func RehydrateRecoveryAcknowledgement(spec RehydrateRecoveryAcknowledgementSpec) (RecoveryAcknowledgement, error) {
	if !spec.ID.valid() || !spec.Receivable.valid() || !spec.Response.valid() ||
		!spec.Standing.valid() || !spec.Currency.valid() || spec.AcknowledgedAt.IsZero() {
		return RecoveryAcknowledgement{}, ErrInvalidAcknowledgement
	}
	if spec.AcknowledgedMinor <= 0 || spec.ReceivableMinor <= 0 ||
		spec.AcknowledgedMinor > spec.ReceivableMinor {
		return RecoveryAcknowledgement{}, ErrInvalidAcknowledgement
	}
	if spec.Standing == ResponseAccepted && spec.AcknowledgedMinor != spec.ReceivableMinor {
		return RecoveryAcknowledgement{}, ErrInvalidAcknowledgement
	}
	if spec.Standing == ResponsePartiallyAccepted && spec.AcknowledgedMinor == spec.ReceivableMinor {
		return RecoveryAcknowledgement{}, ErrInvalidAcknowledgement
	}
	return RecoveryAcknowledgement{
		id:                spec.ID,
		receivable:        spec.Receivable,
		response:          spec.Response,
		standing:          spec.Standing,
		currency:          spec.Currency,
		acknowledgedMinor: spec.AcknowledgedMinor,
		receivableMinor:   spec.ReceivableMinor,
		acknowledgedAt:    spec.AcknowledgedAt.UTC(),
	}, nil
}

// RehydrateClaimAmountAdjustmentSpec 是索赔金额调整行在库里的样子。形成门要的目标
// 金额是否仍在场是写入时已经判过的，读回只复验行自身形状。
type RehydrateClaimAmountAdjustmentSpec struct {
	ID          ClaimAmountAdjustmentID
	TargetKind  AdjustedAmountKind
	Target      AdjustedAmountReference
	Reason      ClaimAdjustmentReason
	Basis       ResponsibilityConclusionReference
	Direction   AdjustmentDirection
	Currency    CurrencyCode
	AmountMinor int64
	Period      BillingPeriodReference
	FormedAt    time.Time
}

func RehydrateClaimAmountAdjustment(spec RehydrateClaimAmountAdjustmentSpec) (ClaimAmountAdjustment, error) {
	return FormClaimAmountAdjustment(ClaimAmountAdjustmentSpec{
		ID:          spec.ID,
		TargetKind:  spec.TargetKind,
		Target:      spec.Target,
		Reason:      spec.Reason,
		Basis:       spec.Basis,
		Direction:   spec.Direction,
		Currency:    spec.Currency,
		AmountMinor: spec.AmountMinor,
		Period:      spec.Period,
		FormedAt:    spec.FormedAt,
	})
}
