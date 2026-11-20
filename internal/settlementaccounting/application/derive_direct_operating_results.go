package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// DirectOperatingDisposition 是答不出毛利时写下的当前答案。它不是另一条派生公式。
type DirectOperatingDisposition uint8

const (
	DirectOperatingDispositionInvalid DirectOperatingDisposition = iota
	DirectOperatingIndeterminate
	DirectOperatingIncompleteCost
	DirectOperatingPendingJudgment
)

func (disposition DirectOperatingDisposition) String() string {
	switch disposition {
	case DirectOperatingIndeterminate:
		return "INDETERMINATE"
	case DirectOperatingIncompleteCost:
		return "INCOMPLETE_COST"
	case DirectOperatingPendingJudgment:
		return "PENDING_JUDGMENT"
	default:
		return ""
	}
}

// DirectOperatingNotifier 是四个事实编排在成功之后通知的那一步。没装上就不通知：
// 单测里的编排保持原来的答案；生产装配必须装上。
type DirectOperatingNotifier interface {
	AfterParcel(ctx context.Context, tenant domain.TenantID, parcel domain.ParcelReference) error
	AfterOccurrence(
		ctx context.Context,
		tenant domain.TenantID,
		occurrence domain.ChargeOccurrenceID,
		version domain.OccurrenceVersion,
	) error
	AfterCharge(ctx context.Context, tenant domain.TenantID, charge domain.CustomerChargeID) error
	AfterPayable(ctx context.Context, tenant domain.TenantID, payable domain.PayableID) error
}

// DeriveDirectOperatingResultsHandler 在已采用的口径上选料，然后调既有的 Derive 或 Rederive。
// 没采用就不调。读口不走这里。
type DeriveDirectOperatingResultsHandler struct {
	adoptions    ports.OperatingBasisAdoptionStore
	facts        ports.ParcelDirectFactsView
	dispositions ports.DirectOperatingDispositionStore
	results      ports.OperatingResultStore
	allocate     *AllocateCostsHandler
	clock        ports.Clock
}

func NewDeriveDirectOperatingResultsHandler(deps DeriveDirectOperatingResultsDeps) (*DeriveDirectOperatingResultsHandler, error) {
	if deps.Adoptions == nil || deps.Facts == nil || deps.Dispositions == nil || deps.Results == nil || deps.Allocate == nil || deps.Clock == nil {
		return nil, fmt.Errorf("%w: direct operating results", ErrNilDependency)
	}
	return &DeriveDirectOperatingResultsHandler{
		adoptions: deps.Adoptions, facts: deps.Facts, dispositions: deps.Dispositions,
		results: deps.Results, allocate: deps.Allocate, clock: deps.Clock,
	}, nil
}

// DeriveDirectOperatingResultsDeps 六件都必备。Allocate 是既有的派生编排，本类型不另写净额。
type DeriveDirectOperatingResultsDeps struct {
	Adoptions    ports.OperatingBasisAdoptionStore
	Facts        ports.ParcelDirectFactsView
	Dispositions ports.DirectOperatingDispositionStore
	Results      ports.OperatingResultStore
	Allocate     *AllocateCostsHandler
	Clock        ports.Clock
}

var _ DirectOperatingNotifier = (*DeriveDirectOperatingResultsHandler)(nil)

func (handler *DeriveDirectOperatingResultsHandler) AfterParcel(
	ctx context.Context,
	tenant domain.TenantID,
	parcel domain.ParcelReference,
) error {
	adoption, found, err := handler.adoptions.FindOperatingBasisAdoption(ctx, tenant)
	if err != nil {
		return fmt.Errorf("direct operating results: adoption: %w", err)
	}
	if !found {
		return nil
	}
	facts, err := handler.facts.LoadParcelDirectFacts(ctx, tenant, parcel)
	if err != nil {
		return fmt.Errorf("direct operating results: facts: %w", err)
	}
	for _, basis := range []domain.OperatingBasis{domain.EstimatedBasis, domain.ConfirmedBasis} {
		if !adoption.Adopts(basis) {
			continue
		}
		if err := handler.one(ctx, tenant, parcel, basis, facts); err != nil {
			return err
		}
	}
	return nil
}

func (handler *DeriveDirectOperatingResultsHandler) AfterOccurrence(
	ctx context.Context,
	tenant domain.TenantID,
	occurrence domain.ChargeOccurrenceID,
	version domain.OccurrenceVersion,
) error {
	parcels, err := handler.facts.ParcelsOfOccurrence(ctx, tenant, occurrence, version)
	if err != nil {
		return fmt.Errorf("direct operating results: occurrence parcels: %w", err)
	}
	for _, parcel := range parcels {
		if err := handler.AfterParcel(ctx, tenant, parcel); err != nil {
			return err
		}
	}
	return nil
}

func (handler *DeriveDirectOperatingResultsHandler) AfterCharge(
	ctx context.Context,
	tenant domain.TenantID,
	charge domain.CustomerChargeID,
) error {
	parcels, err := handler.facts.ParcelsOfCharge(ctx, tenant, charge)
	if err != nil {
		return fmt.Errorf("direct operating results: charge parcels: %w", err)
	}
	for _, parcel := range parcels {
		if err := handler.AfterParcel(ctx, tenant, parcel); err != nil {
			return err
		}
	}
	return nil
}

func (handler *DeriveDirectOperatingResultsHandler) AfterPayable(
	ctx context.Context,
	tenant domain.TenantID,
	payable domain.PayableID,
) error {
	parcels, err := handler.facts.ParcelsOfPayable(ctx, tenant, payable)
	if err != nil {
		return fmt.Errorf("direct operating results: payable parcels: %w", err)
	}
	for _, parcel := range parcels {
		if err := handler.AfterParcel(ctx, tenant, parcel); err != nil {
			return err
		}
	}
	return nil
}

func (handler *DeriveDirectOperatingResultsHandler) one(
	ctx context.Context,
	tenant domain.TenantID,
	parcel domain.ParcelReference,
	basis domain.OperatingBasis,
	facts ports.ParcelDirectFacts,
) error {
	// 形态不是面单渠道（含不可观察），或还挂着要分摊才归得到的共享成本，或供应商金额不能唯一指到这一件：
	// 都不出毛利。先于「缺一侧」判断，免得网络服务产品在供应商成本还没到时被答成判不出。
	if facts.ServiceForm != ports.ServiceFormLabelChannel || facts.SharedCost || facts.NonUniqueSupplier {
		return handler.disposition(ctx, tenant, parcel, basis, DirectOperatingIncompleteCost)
	}
	customer, supplier, credits := sidesOf(basis, facts)
	if len(customer) == 0 && len(supplier) == 0 && len(credits) == 0 {
		return handler.dispositions.ForgetDirectOperatingAnswer(ctx, tenant, parcel.String(), basis)
	}
	if len(customer) == 0 || len(supplier) == 0 {
		return handler.disposition(ctx, tenant, parcel, basis, DirectOperatingIndeterminate)
	}
	// 领域把同一口径的审核应付与贷项各限至多一项。多笔不能并进同一份结果，也不挑一笔或加总。
	if basis == domain.ConfirmedBasis && (len(supplier) > 1 || len(credits) > 1) {
		return handler.disposition(ctx, tenant, parcel, basis, DirectOperatingPendingJudgment)
	}
	// 费用归属日和主张账期不是同一种标识，不对着比；归属日没写下也不挡派生。
	// 按包裹的直接成本不占对账单账期，键上固定用产品词，同一包裹同一口径一行。预估与已确认都用它。
	if basis == domain.ConfirmedBasis && !confirmedClaimPeriodsHold(supplier, credits) {
		return handler.disposition(ctx, tenant, parcel, basis, DirectOperatingPendingJudgment)
	}
	currency, ok := oneCurrency(customer, supplier, credits)
	if !ok {
		return handler.disposition(ctx, tenant, parcel, basis, DirectOperatingPendingJudgment)
	}
	directives := componentsOf(basis, customer, supplier, credits)
	return handler.persist(ctx, tenant, parcel.String(), directOperatingPeriod, basis, currency, directives)
}

// directOperatingPeriod 是按包裹直接成本经营结果的账期词。它不是对账单账期，也不是费用归属日。
const directOperatingPeriod = "DIRECT"

func sidesOf(basis domain.OperatingBasis, facts ports.ParcelDirectFacts) (customer, supplier, credits []ports.DirectAmount) {
	switch basis {
	case domain.EstimatedBasis:
		return facts.Estimates, facts.ExpectedCosts, nil
	case domain.ConfirmedBasis:
		return facts.Receivables, facts.Payables, facts.Credits
	default:
		return nil, nil, nil
	}
}

// confirmedClaimPeriodsHold 只看已确认口径里应付和贷项自己的主张账期。预期成本没有主张账期
// （PeriodStated 为假），不参与。PeriodStated 为真而 Period 空，是同一主张上不止一个账期。
// 应付与贷项都写下了主张账期但彼此不同，也不挑一侧。
func confirmedClaimPeriodsHold(groups ...[]ports.DirectAmount) bool {
	var stated string
	have := false
	for _, group := range groups {
		for _, amount := range group {
			if !amount.PeriodStated {
				continue
			}
			if amount.Period == "" {
				return false
			}
			if !have {
				stated = amount.Period
				have = true
				continue
			}
			if amount.Period != stated {
				return false
			}
		}
	}
	return true
}

func oneCurrency(groups ...[]ports.DirectAmount) (string, bool) {
	currency := ""
	for _, group := range groups {
		for _, amount := range group {
			if amount.Currency == "" {
				return "", false
			}
			if currency == "" {
				currency = amount.Currency
				continue
			}
			if amount.Currency != currency {
				return "", false
			}
		}
	}
	return currency, currency != ""
}

func componentsOf(basis domain.OperatingBasis, customer, supplier, credits []ports.DirectAmount) []ComponentDirective {
	directives := make([]ComponentDirective, 0, len(customer)+len(supplier)+len(credits))
	customerRole, supplierRole := domain.CustomerEstimateRole, domain.SupplierExpectedCostRole
	if basis == domain.ConfirmedBasis {
		customerRole, supplierRole = domain.CustomerOperatingReceivableRole, domain.AuditedPayableRole
	}
	for _, amount := range customer {
		directives = append(directives, ComponentDirective{
			Source: amount.Source, Role: customerRole, Effect: domain.IncreasesResult, AmountMinor: amount.AmountMinor,
		})
	}
	for _, amount := range supplier {
		directives = append(directives, ComponentDirective{
			Source: amount.Source, Role: supplierRole, Effect: domain.DecreasesResult, AmountMinor: amount.AmountMinor,
		})
	}
	for _, amount := range credits {
		directives = append(directives, ComponentDirective{
			Source: amount.Source, Role: domain.SupplierCreditNoteRole, Effect: domain.IncreasesResult, AmountMinor: amount.AmountMinor,
		})
	}
	sort.Slice(directives, func(i, j int) bool {
		if directives[i].Role != directives[j].Role {
			return directives[i].Role < directives[j].Role
		}
		return directives[i].Source < directives[j].Source
	})
	return directives
}

func (handler *DeriveDirectOperatingResultsHandler) persist(
	ctx context.Context,
	tenant domain.TenantID,
	scope, period string,
	basis domain.OperatingBasis,
	currency string,
	directives []ComponentDirective,
) error {
	version := directVersion(directives)
	key, bad := handler.allocate.resultKey(tenant, scope, period, basis)
	if bad != nil {
		return fmt.Errorf("direct operating results: scope %s is not a result key", scope)
	}
	existing, found, err := handler.results.FindByKey(ctx, key)
	if err != nil {
		return fmt.Errorf("direct operating results: find: %w", err)
	}
	if found {
		got, _ := existing.Result.Margin()
		// 重派生沿用原快照的币种。整对换成另一种币不能借它换币，也不在这里换算。
		if got.String() != currency {
			return handler.disposition(ctx, tenant, mustParcel(scope), basis, DirectOperatingPendingJudgment)
		}
		if componentsMatch(existing.Result.Components(), directives) {
			return handler.dispositions.ClearDirectOperatingDisposition(ctx, tenant, scope, basis)
		}
		rederived, err := handler.allocate.Rederive(ctx, RederiveResultCommand{
			TenantID: tenant, Scope: scope, Period: period, Basis: basis,
			Components: directives, NewVersion: version, AsOf: existing.Result.AsOf(),
		})
		if err != nil {
			return err
		}
		if rederived.Outcome() != ResultRederived {
			return fmt.Errorf("direct operating results: rederive %s", rederived.Outcome())
		}
		return handler.dispositions.ClearDirectOperatingDisposition(ctx, tenant, scope, basis)
	}
	derived, err := handler.allocate.Derive(ctx, DeriveResultCommand{
		TenantID: tenant, Scope: scope, Period: period, Basis: basis, Currency: currency,
		Components: directives, Version: version, AsOf: handler.clock.Now(),
	})
	if err != nil {
		return err
	}
	switch derived.Outcome() {
	case ResultDerived, ResultExistingResult:
		return handler.dispositions.ClearDirectOperatingDisposition(ctx, tenant, scope, basis)
	default:
		return fmt.Errorf("direct operating results: derive %s", derived.Outcome())
	}
}

func mustParcel(scope string) domain.ParcelReference {
	parcel, err := domain.NewParcelReference(scope)
	if err != nil {
		return domain.ParcelReference{}
	}
	return parcel
}

func (handler *DeriveDirectOperatingResultsHandler) disposition(
	ctx context.Context,
	tenant domain.TenantID,
	parcel domain.ParcelReference,
	basis domain.OperatingBasis,
	disposition DirectOperatingDisposition,
) error {
	if parcel.String() == "" {
		return fmt.Errorf("direct operating results: blank scope")
	}
	return handler.dispositions.SaveDirectOperatingDisposition(ctx, tenant, parcel.String(), basis, disposition.String(), handler.clock.Now())
}

func componentsMatch(stored []domain.ResultComponent, directives []ComponentDirective) bool {
	if len(stored) != len(directives) {
		return false
	}
	left := append([]domain.ResultComponent(nil), stored...)
	sort.Slice(left, func(i, j int) bool {
		if left[i].Role != left[j].Role {
			return left[i].Role < left[j].Role
		}
		return left[i].Source.String() < left[j].Source.String()
	})
	for i := range left {
		if left[i].Source.String() != directives[i].Source || left[i].Role != directives[i].Role ||
			left[i].Effect != directives[i].Effect || left[i].AmountMinor != directives[i].AmountMinor {
			return false
		}
	}
	return true
}

func directVersion(directives []ComponentDirective) string {
	var raw string
	for _, directive := range directives {
		raw += directive.Source + "\x1f" + directive.Role.String() + "\x1f" + directive.Effect.String() + "\x1f" + fmt.Sprint(directive.AmountMinor) + "\n"
	}
	sum := sha256.Sum256([]byte(raw))
	return "direct-" + hex.EncodeToString(sum[:8])
}

func (handler *FormEstimatedCustomerChargeHandler) NoticeDirectOperating(notifier DirectOperatingNotifier) {
	if handler != nil {
		handler.direct = notifier
	}
}

func (handler *FormSupplierExpectedCostHandler) NoticeDirectOperating(notifier DirectOperatingNotifier) {
	if handler != nil {
		handler.direct = notifier
	}
}

func (handler *ConfirmChargeHandler) NoticeDirectOperating(notifier DirectOperatingNotifier) {
	if handler != nil {
		handler.direct = notifier
	}
}

func (handler *ReceiveSupplierBillHandler) NoticeDirectOperating(notifier DirectOperatingNotifier) {
	if handler != nil {
		handler.direct = notifier
	}
}

func (handler *WithdrawSupplierExpectedCostHandler) NoticeDirectOperating(notifier DirectOperatingNotifier) {
	if handler != nil {
		handler.direct = notifier
	}
}
