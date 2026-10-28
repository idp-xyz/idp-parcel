// convert_occupation_on_charge_confirmation.go 编排费用确认之后的占用收口：把这件包裹在接受前控制里占下的冻结与信用暴露
// 转为正式扣费，并按账重算该作用域的入账余额与已确认未结（ADR-0343）。
package application

import (
	"context"
	"errors"
	"fmt"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// ConvertOccupationResult 交回这一次确认转出了多少，给测试与运营留痕看；编排本身不据它分支。
type ConvertOccupationResult struct {
	freezeConvertedMinor   int64
	exposureConvertedMinor int64
	recomputed             bool
}

// FreezeConvertedMinor 是这一次从冻结账本转为正式扣费的金额。
func (result ConvertOccupationResult) FreezeConvertedMinor() int64 {
	return result.freezeConvertedMinor
}

// ExposureConvertedMinor 是这一次从暴露账本转入已确认未结的金额。
func (result ConvertOccupationResult) ExposureConvertedMinor() int64 {
	return result.exposureConvertedMinor
}

// Recomputed 说明这一次重算并推进了该作用域的入账余额与已确认未结。
func (result ConvertOccupationResult) Recomputed() bool { return result.recomputed }

// ConvertOccupationDeps 是占用收口的依赖。ScopeLock 与接受前控制的施加、释放共用同一把作用域锁：同一作用域上转出与
// 新占用、释放依次判，后一笔按前一笔已落的账判。
type ConvertOccupationDeps struct {
	Book      ports.PreAcceptanceConversionBook
	Freezes   ports.FreezeLedgerRepository
	Exposures ports.CreditExposureLedgerRepository
	ScopeLock ports.PreAcceptanceControlScopeLock
	Balances  ports.OperationalBalanceAdvance
	Books     ports.CustomerReceiptBooks
	Clock     ports.Clock
}

type ConvertOccupationOnChargeConfirmationHandler struct {
	deps ConvertOccupationDeps
}

func NewConvertOccupationOnChargeConfirmationHandler(
	deps ConvertOccupationDeps,
) (*ConvertOccupationOnChargeConfirmationHandler, error) {
	if deps.Book == nil || deps.Freezes == nil || deps.Exposures == nil || deps.ScopeLock == nil ||
		deps.Balances == nil || deps.Books == nil || deps.Clock == nil {
		return nil, fmt.Errorf("%w: convert occupation on charge confirmation", ErrNilDependency)
	}
	return &ConvertOccupationOnChargeConfirmationHandler{deps: deps}, nil
}

// Handle 在确认费用的那一笔事务里收口占用。
//
// 认领口径：费用 → 形成它的 SELL 评价请求 → 包裹 → 受理前估价留痕里这件包裹挂着的控制请求 → 该请求在两本账上的占用。
// 控制请求身份就是接受前控制的原业务关联（PS 适配器 controlRequestIdentity 用的那一个），作用域取费用确认时固定的
// 责任法人、结算账户与结算币种——与占用当初随控制结果保存的作用域是同一个，不在这里重新推导。
//
// 转多少：这件包裹在估价里的份额，整份一次转出（以包裹为单位幂等，转出册认得出重放）。同一件包裹的后续费用再确认时
// 份额已转出，只重算。份额与费用实际金额的差不另立科目：占用离开、实际金额按 ADR-0181 决定三进已确认未结，差额就是
// 两者的自然落差——估价高了可用余额回升，估价低了（CONTEXT「最终确认费用高于冻结金额时必须完整入账」）差额同样完整
// 入账、可用余额随之为负，不截断到占用金额。
//
// 不是接受前控制那一路的费用（没有包裹、包裹不在估价留痕里、占用已释放）没有可转，仍重算：已确认未结随确认变化，
// 与占用有无无关（ADR-0181 决定三，确认费用调用同一个函数）。
//
// 任何依赖失败都作错误交回：调用方的事务整笔回滚，费用确认与占用收口同生同灭，不留「确认了、占用还挂着」的中间态。
func (handler *ConvertOccupationOnChargeConfirmationHandler) Handle(
	ctx context.Context,
	tenant domain.TenantID,
	charge domain.CustomerCharge,
) (ConvertOccupationResult, error) {
	facts, confirmed := charge.ConfirmedFacts()
	if !confirmed {
		return ConvertOccupationResult{}, errors.New("convert occupation: charge is not confirmed")
	}
	if facts.Direction != domain.ChargeReceivable {
		// 应付方向的费用不进客户应收作用域，也没有接受前占用。
		return ConvertOccupationResult{}, nil
	}
	currency, _ := charge.SettlementAmount()
	scope, err := domain.NewSettlementScope(facts.ResponsibleEntity, facts.SettlementAccount, currency)
	if err != nil {
		return ConvertOccupationResult{}, fmt.Errorf("convert occupation: settlement scope: %w", err)
	}

	// 控制作用域锁先于账本读，后于它的是余额作用域锁（recomputeCustomerScope 内取）：与施加、释放同序，不成环。
	if err := handler.deps.ScopeLock.LockControlScope(ctx, tenant, scope); err != nil {
		return ConvertOccupationResult{}, fmt.Errorf("convert occupation: %w", err)
	}

	var result ConvertOccupationResult
	parcel, found, err := handler.deps.Book.ParcelOfCharge(ctx, tenant, charge.ID())
	if err != nil {
		return ConvertOccupationResult{}, fmt.Errorf("convert occupation: parcel of charge: %w", err)
	}
	if found {
		if err := handler.convertParcelShares(ctx, tenant, scope, parcel, charge.ID(), &result); err != nil {
			return ConvertOccupationResult{}, err
		}
	}

	if _, err := recomputeCustomerScope(ctx, tenant, scope, handler.deps.Balances, handler.deps.Books, handler.deps.Clock); err != nil {
		return ConvertOccupationResult{}, fmt.Errorf("convert occupation: recompute: %w", err)
	}
	result.recomputed = true
	return result, nil
}

func (handler *ConvertOccupationOnChargeConfirmationHandler) convertParcelShares(
	ctx context.Context,
	tenant domain.TenantID,
	scope domain.SettlementScope,
	parcel string,
	charge domain.CustomerChargeID,
	result *ConvertOccupationResult,
) error {
	shares, err := handler.deps.Book.ParcelShares(ctx, tenant, scope, parcel)
	if err != nil {
		return fmt.Errorf("convert occupation: parcel shares: %w", err)
	}
	if len(shares) == 0 {
		return nil
	}

	freezes, err := handler.deps.Freezes.LoadForScope(ctx, tenant, scope)
	if err != nil || freezes == nil {
		return fmt.Errorf("convert occupation: freeze ledger: %w", errOrNil(err))
	}
	exposures, err := handler.deps.Exposures.LoadForScope(ctx, tenant, scope)
	if err != nil || exposures == nil {
		return fmt.Errorf("convert occupation: exposure ledger: %w", errOrNil(err))
	}

	now := handler.deps.Clock.Now()
	freezeChanged, exposureChanged := false, false
	for _, share := range shares {
		if share.ShareMinor <= 0 {
			// 估价里这一件合计为零：没有占下东西，也就没有可转的。
			continue
		}
		if freeze, held := freezes.FindByRequest(share.RequestID); held && freeze.Status() == domain.FreezeHeld {
			minor := min(share.ShareMinor, freeze.RemainingMinor())
			recorded, err := handler.deps.Book.RecordConversion(
				ctx, tenant, scope, share.RequestID, parcel, ports.OccupationFreeze, minor, charge, now)
			if err != nil {
				return fmt.Errorf("convert occupation: record freeze conversion: %w", err)
			}
			if recorded {
				if _, err := freezes.Convert(share.RequestID, minor, now); err != nil {
					return fmt.Errorf("convert occupation: convert freeze: %w", err)
				}
				freezeChanged = true
				result.freezeConvertedMinor += minor
			}
		}
		if exposure, held := exposures.FindByRequest(share.RequestID); held && exposure.Status() == domain.ExposureRecorded {
			minor := min(share.ShareMinor, exposure.RemainingMinor())
			recorded, err := handler.deps.Book.RecordConversion(
				ctx, tenant, scope, share.RequestID, parcel, ports.OccupationExposure, minor, charge, now)
			if err != nil {
				return fmt.Errorf("convert occupation: record exposure conversion: %w", err)
			}
			if recorded {
				if _, err := exposures.Convert(share.RequestID, minor, now); err != nil {
					return fmt.Errorf("convert occupation: convert exposure: %w", err)
				}
				exposureChanged = true
				result.exposureConvertedMinor += minor
			}
		}
	}
	if freezeChanged {
		if err := handler.deps.Freezes.Save(ctx, tenant, scope, freezes); err != nil {
			return fmt.Errorf("convert occupation: save freeze ledger: %w", err)
		}
	}
	if exposureChanged {
		if err := handler.deps.Exposures.Save(ctx, tenant, scope, exposures); err != nil {
			return fmt.Errorf("convert occupation: save exposure ledger: %w", err)
		}
	}
	return nil
}

// errOrNil 把「读回 nil 账本却无错」折成一个可包的错误，免得 %w 包 nil。
func errOrNil(err error) error {
	if err != nil {
		return err
	}
	return errors.New("ledger is nil")
}
