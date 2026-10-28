// confirm_charge_on_basis.go 编排「确认依据到了就确认」（ADR-0336 决定二）：给一笔已形成的客户费用记下一份确认依据，
// 这一户登了确认时点就当场调确认编排。依据从两头来——PS 终局信封到达时补给这件包裹已形成的费用，费用在终局之后才形成
// 的由形成那一笔回头补记——两头都落到 OnCharge 这一处。
package application

import (
	"context"
	"fmt"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// ConfirmOnBasisOutcome 是一次「记依据并按时点确认」的答案。
type ConfirmOnBasisOutcome uint8

const (
	ConfirmOnBasisOutcomeInvalid ConfirmOnBasisOutcome = iota
	// ConfirmOnBasisNoFormedCharge：这件包裹当前没有已形成的客户费用（还没形成、或停在待判断），依据无处可记。费用之后
	// 形成时由形成那一笔回读终局补记，这里不留占位。
	ConfirmOnBasisNoFormedCharge
	// ConfirmOnBasisRecorded：依据记下了，这一户没登确认时点，不调确认（ADR-0170 决定二）。补登时点之后经运营确认口
	// 补拍。
	ConfirmOnBasisRecorded
	// ConfirmOnBasisConfirmAttempted：依据记下了并调了确认，确认自己的答案见 Confirmation。
	ConfirmOnBasisConfirmAttempted
	// ConfirmOnBasisUndecided：费用册、依据册或时点册读写不动，记没记上不知道，要重投。
	ConfirmOnBasisUndecided
)

func (outcome ConfirmOnBasisOutcome) String() string {
	switch outcome {
	case ConfirmOnBasisNoFormedCharge:
		return "NO_FORMED_CHARGE"
	case ConfirmOnBasisRecorded:
		return "BASIS_RECORDED"
	case ConfirmOnBasisConfirmAttempted:
		return "CONFIRM_ATTEMPTED"
	case ConfirmOnBasisUndecided:
		return "CONFIRM_ON_BASIS_UNDECIDED"
	default:
		return ""
	}
}

// ConfirmationBasis 是到达的一份确认依据：种类与指得回它的引用。
type ConfirmationBasis struct {
	Kind      domain.ConfirmationBasisKind
	Reference domain.ConfirmationBasisReference
}

type ConfirmOnBasisResult struct {
	outcome      ConfirmOnBasisOutcome
	charge       domain.CustomerChargeID
	moment       SettlementMomentOutcome
	confirmation ConfirmChargeResult
	cause        error
}

func (result ConfirmOnBasisResult) Outcome() ConfirmOnBasisOutcome { return result.outcome }

func (result ConfirmOnBasisResult) Charge() domain.CustomerChargeID { return result.charge }

// Moment 是确认时点门的答案；没走到那一道是零值。
func (result ConfirmOnBasisResult) Moment() SettlementMomentOutcome { return result.moment }

// Confirmation 只在 ConfirmOnBasisConfirmAttempted 时有内容。
func (result ConfirmOnBasisResult) Confirmation() ConfirmChargeResult { return result.confirmation }

// UndecidedCause 只在未决时非空，给消费门写进重投的错误。
func (result ConfirmOnBasisResult) UndecidedCause() error { return result.cause }

// Retryable 说这一拍要不要重投：本编排自己读写不动，确认编排停在依赖读不动的那几格，或费用已确认而确认信封没交出去。
// 确认条件目录与确认事实册没登是租户取值缺席，补登之前重投只会再答一次同一格，不算（ADR-0029 按恢复动作分格）。
func (result ConfirmOnBasisResult) Retryable() bool {
	if result.outcome == ConfirmOnBasisUndecided {
		return true
	}
	if result.outcome != ConfirmOnBasisConfirmAttempted {
		return false
	}
	// 确认编排把交接失败折成一个续办引用、照答已确认：经运营口进来的有人能重放，消费门这一路入账之后就再没人推它。
	if result.confirmation.ConfirmationHandoffReference() != "" {
		return true
	}
	if result.confirmation.Outcome() != ConfirmUndecided {
		return false
	}
	switch result.confirmation.UndecidedReason() {
	case ConditionUnconfigured, ConfirmationFactsUnconfigured:
		return false
	default:
		return true
	}
}

type ConfirmChargeOnBasisDeps struct {
	// Serializer 让 OnParcel 与费用形成那一头的回读排队，见 ports.ParcelConfirmationSerializer。
	Serializer ports.ParcelConfirmationSerializer
	Requests   ports.SellEvaluationRequestView
	Charges    ports.CustomerChargeStore
	Bases      ports.ConfirmationBasisRecorder
	Moments    *AdmitSettlementMomentHandler
	Confirm    *ConfirmChargeHandler
	Clock      ports.Clock
}

// ConfirmChargeOnBasisHandler 见文件头注。调用方给环境事务：依据行、确认后的费用行与确认信封同笔落下。
type ConfirmChargeOnBasisHandler struct {
	deps ConfirmChargeOnBasisDeps
}

func NewConfirmChargeOnBasisHandler(deps ConfirmChargeOnBasisDeps) (*ConfirmChargeOnBasisHandler, error) {
	if deps.Serializer == nil || deps.Requests == nil || deps.Charges == nil || deps.Bases == nil || deps.Moments == nil || deps.Confirm == nil ||
		deps.Clock == nil {
		return nil, fmt.Errorf("%w: confirm charge on basis", ErrNilDependency)
	}
	return &ConfirmChargeOnBasisHandler{deps: deps}, nil
}

// OnParcel 按包裹找当前那份 SELL 请求形成的费用，再走 OnCharge。只认当前一代：前几代停在待判断、没有费用。先取包裹确认
// 锁再读请求：形成那一头若正持锁，这里等它提交，读到的就是形成之后的请求。
func (handler *ConfirmChargeOnBasisHandler) OnParcel(
	ctx context.Context,
	tenant domain.TenantID,
	parcel domain.ParcelReference,
	basis ConfirmationBasis,
) (ConfirmOnBasisResult, error) {
	if err := handler.deps.Serializer.SerializeParcelConfirmation(ctx, tenant, parcel); err != nil {
		return ConfirmOnBasisResult{outcome: ConfirmOnBasisUndecided, cause: err}, nil
	}
	record, found, err := handler.deps.Requests.FindCurrentByParcel(ctx, tenant, parcel)
	if err != nil {
		return ConfirmOnBasisResult{outcome: ConfirmOnBasisUndecided, cause: fmt.Errorf("sell request register: %w", err)}, nil
	}
	if !found || record.FormedCharge.String() == "" {
		return ConfirmOnBasisResult{outcome: ConfirmOnBasisNoFormedCharge}, nil
	}
	return handler.OnCharge(ctx, tenant, record.FormedCharge, basis)
}

// OnCharge 记依据 → 问确认时点 → 调确认。费用已确认的照样记依据（重放无害），确认编排自己答已确认。
func (handler *ConfirmChargeOnBasisHandler) OnCharge(
	ctx context.Context,
	tenant domain.TenantID,
	charge domain.CustomerChargeID,
	basis ConfirmationBasis,
) (ConfirmOnBasisResult, error) {
	stored, found, err := handler.deps.Charges.FindByID(ctx, tenant, charge)
	if err != nil {
		return ConfirmOnBasisResult{outcome: ConfirmOnBasisUndecided, charge: charge,
			cause: fmt.Errorf("customer charge store: %w", err)}, nil
	}
	if !found {
		return ConfirmOnBasisResult{outcome: ConfirmOnBasisNoFormedCharge, charge: charge}, nil
	}
	if _, err := handler.deps.Bases.RecordConfirmationBasis(
		ctx, tenant, charge, stored.FeeItem(), basis.Kind, basis.Reference, handler.deps.Clock.Now()); err != nil {
		return ConfirmOnBasisResult{outcome: ConfirmOnBasisUndecided, charge: charge,
			cause: fmt.Errorf("confirmation basis register: %w", err)}, nil
	}
	moment, err := handler.deps.Moments.Admit(ctx, tenant, domain.SettlementMomentConfirm)
	if err != nil {
		return ConfirmOnBasisResult{}, err
	}
	switch moment {
	case SettlementMomentAdmitted:
	case SettlementMomentViewUnavailable:
		return ConfirmOnBasisResult{outcome: ConfirmOnBasisUndecided, charge: charge, moment: moment,
			cause: fmt.Errorf("settlement moment register unavailable")}, nil
	default:
		return ConfirmOnBasisResult{outcome: ConfirmOnBasisRecorded, charge: charge, moment: moment}, nil
	}
	confirmation, err := handler.deps.Confirm.Handle(ctx, ConfirmChargeCommand{TenantID: tenant, ChargeID: charge.String()})
	if err != nil {
		return ConfirmOnBasisResult{}, err
	}
	return ConfirmOnBasisResult{outcome: ConfirmOnBasisConfirmAttempted, charge: charge, moment: moment,
		confirmation: confirmation}, nil
}
