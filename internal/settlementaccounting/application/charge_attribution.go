package application

import (
	"context"
	"fmt"
	"time"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// ChargeAttributionOutcome 是一次归属日判定的答案。未配置时没有日期。
type ChargeAttributionOutcome uint8

const (
	ChargeAttributionOutcomeInvalid ChargeAttributionOutcome = iota
	ChargeAttributionUnconfigured
	ChargeAttributionViewUnavailable
	ChargeAttributionNotAccepted
	ChargeAttributionJudged
)

func (outcome ChargeAttributionOutcome) String() string {
	switch outcome {
	case ChargeAttributionUnconfigured:
		return "CHARGE_ATTRIBUTION_UNCONFIGURED"
	case ChargeAttributionViewUnavailable:
		return "CHARGE_ATTRIBUTION_VIEW_UNAVAILABLE"
	case ChargeAttributionNotAccepted:
		return "CHARGE_ATTRIBUTION_NOT_ACCEPTED"
	case ChargeAttributionJudged:
		return "CHARGE_ATTRIBUTION_JUDGED"
	default:
		return ""
	}
}

// JudgeChargeAttributionCommand 携带一次判定。只有来源发生与费用确认两个时点，没有包裹创建、收寄或签收。
// 账户是这件费用所属的结算账户：业务时区与截单时刻读它的对账安排（ADR-0269 决定二）。
type JudgeChargeAttributionCommand struct {
	TenantID        domain.TenantID
	FeeItem         string
	Account         string
	SourceOccurred  time.Time
	ChargeConfirmed time.Time
}

// ChargeAttributionResult 在判定成功时交回归属日。未配置时日期是空的，Missing 指名缺哪一册。
type ChargeAttributionResult struct {
	outcome ChargeAttributionOutcome
	date    domain.AttributionDate
	has     bool
	missing []domain.AttributionRegister
}

func (result ChargeAttributionResult) Outcome() ChargeAttributionOutcome { return result.outcome }

func (result ChargeAttributionResult) Date() (domain.AttributionDate, bool) {
	return result.date, result.has
}

// Missing 只在未配置时非空，按对账安排、归属日形态的次序列出没登的册；两本都没登就两本都列。
func (result ChargeAttributionResult) Missing() []domain.AttributionRegister {
	return append([]domain.AttributionRegister(nil), result.missing...)
}

// JudgeChargeAttributionHandler 按费用项目已登记的形态与所属结算账户的对账安排判定归属日。任一册上没有
// 相符行答未配置并指名缺的那一册，不用签收日、系统当天或服务器时区顶上。费用确认与截单都经它判，判法
// 只有这一份。
type JudgeChargeAttributionHandler struct {
	attributions ports.ChargeAttributionView
	arrangements ports.StatementArrangementView
}

func NewJudgeChargeAttributionHandler(
	attributions ports.ChargeAttributionView,
	arrangements ports.StatementArrangementView,
) (*JudgeChargeAttributionHandler, error) {
	if attributions == nil || arrangements == nil {
		return nil, fmt.Errorf("%w: charge attribution", ErrNilDependency)
	}
	return &JudgeChargeAttributionHandler{attributions: attributions, arrangements: arrangements}, nil
}

func (handler *JudgeChargeAttributionHandler) Judge(
	ctx context.Context,
	command JudgeChargeAttributionCommand,
) (ChargeAttributionResult, error) {
	feeItem, err := domain.NewFeeItemReference(command.FeeItem)
	if err != nil || command.TenantID.String() == "" {
		return ChargeAttributionResult{outcome: ChargeAttributionNotAccepted}, nil
	}
	account, err := domain.NewSettlementAccountID(command.Account)
	if err != nil {
		return ChargeAttributionResult{outcome: ChargeAttributionNotAccepted}, nil
	}
	return handler.judge(ctx, command.TenantID, feeItem, account, command.SourceOccurred, command.ChargeConfirmed), nil
}

// JudgeConfirmedCharge 判一件已确认费用的归属日：费用项目、所属结算账户、来源发生与确认两个时点都取自
// 费用本身。未确认的费用没有确认时点也没有所属账户，答未受理。
func (handler *JudgeChargeAttributionHandler) JudgeConfirmedCharge(
	ctx context.Context,
	tenant domain.TenantID,
	charge domain.CustomerCharge,
) ChargeAttributionResult {
	facts, confirmed := charge.ConfirmedFacts()
	confirmedAt, _ := charge.ConfirmedAt()
	if !confirmed || tenant.String() == "" {
		return ChargeAttributionResult{outcome: ChargeAttributionNotAccepted}
	}
	sourceOccurred, _ := charge.SourceOccurredAt()
	return handler.judge(ctx, tenant, charge.FeeItem(), facts.SettlementAccount, sourceOccurred, confirmedAt)
}

func (handler *JudgeChargeAttributionHandler) judge(
	ctx context.Context,
	tenant domain.TenantID,
	feeItem domain.FeeItemReference,
	account domain.SettlementAccountID,
	sourceOccurred, chargeConfirmed time.Time,
) ChargeAttributionResult {
	// 两本都读完再答：只报先查到的那一本缺，补登之后再截又撞上另一本，登记的人要白走一趟。
	arrangement, arranged, err := handler.arrangements.LoadStatementArrangement(ctx, tenant, account)
	if err != nil {
		return ChargeAttributionResult{outcome: ChargeAttributionViewUnavailable}
	}
	registration, registered, err := handler.attributions.LoadChargeAttribution(ctx, tenant, feeItem)
	if err != nil {
		return ChargeAttributionResult{outcome: ChargeAttributionViewUnavailable}
	}
	var missing []domain.AttributionRegister
	if !arranged {
		missing = append(missing, domain.StatementArrangementRegister)
	}
	if !registered {
		missing = append(missing, domain.ChargeAttributionFormRegister)
	}
	if len(missing) > 0 {
		return ChargeAttributionResult{outcome: ChargeAttributionUnconfigured, missing: missing}
	}
	date, err := registration.Judge(arrangement, sourceOccurred, chargeConfirmed)
	if err != nil {
		return ChargeAttributionResult{outcome: ChargeAttributionNotAccepted}
	}
	return ChargeAttributionResult{outcome: ChargeAttributionJudged, date: date, has: true}
}

// RegisterChargeAttributionHandler 登记一条费用项目的归属日形态。落库时刻不是载荷的一格。
type RegisterChargeAttributionHandler struct {
	attributions ports.ChargeAttributionRegister
	clock        ports.Clock
}

func NewRegisterChargeAttributionHandler(
	attributions ports.ChargeAttributionRegister,
	clock ports.Clock,
) (*RegisterChargeAttributionHandler, error) {
	if attributions == nil || clock == nil {
		return nil, fmt.Errorf("%w: charge attribution register", ErrNilDependency)
	}
	return &RegisterChargeAttributionHandler{attributions: attributions, clock: clock}, nil
}

func (handler *RegisterChargeAttributionHandler) RegisterChargeAttribution(
	ctx context.Context,
	tenant domain.TenantID,
	registration domain.ChargeAttributionRegistration,
) (ports.CatalogueRegistrationEffect, error) {
	if tenant.String() == "" {
		return 0, fmt.Errorf("%w: tenant", domain.ErrBlankValue)
	}
	return handler.attributions.SaveChargeAttribution(ctx, tenant, registration, handler.clock.Now())
}

// RegisterStatementArrangementHandler 给一个结算账户的对账安排追加一版。落库时刻不是载荷的一格；新一版只作用于
// 此后形成的归属日，已形成的不回改（ADR-0269 决定一）。
type RegisterStatementArrangementHandler struct {
	arrangements ports.StatementArrangementRegister
	clock        ports.Clock
}

func NewRegisterStatementArrangementHandler(
	arrangements ports.StatementArrangementRegister,
	clock ports.Clock,
) (*RegisterStatementArrangementHandler, error) {
	if arrangements == nil || clock == nil {
		return nil, fmt.Errorf("%w: statement arrangement register", ErrNilDependency)
	}
	return &RegisterStatementArrangementHandler{arrangements: arrangements, clock: clock}, nil
}

func (handler *RegisterStatementArrangementHandler) RegisterStatementArrangement(
	ctx context.Context,
	tenant domain.TenantID,
	arrangement domain.StatementArrangement,
) (ports.CatalogueRegistrationEffect, error) {
	if tenant.String() == "" {
		return 0, fmt.Errorf("%w: tenant", domain.ErrBlankValue)
	}
	return handler.arrangements.SaveStatementArrangement(ctx, tenant, arrangement, handler.clock.Now())
}
