package parcelshipment

import (
	"context"
	"errors"
	"fmt"

	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	psports "go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	sainbox "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/inbox"
	saapplication "go.idp.xyz/idp-parcel/internal/settlementaccounting/application"
	sadomain "go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	saports "go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// ErrConfirmOnBasisUndecided 表示「记依据并按时点确认」停在自己的未决上（费用册、依据册、时点册或确认编排的依赖读不动）。
// 重投会改变结果，生产装配把它登进 WithUndecidedSentinels；原因留在错误正文里。
var ErrConfirmOnBasisUndecided = errors.New(
	"settlement accounting parcelshipment adapter: recording the confirmation basis is undecided")

// CurrentFinalOutcomeSource 按包裹取回当前有效终局，由 PS 的 *pspostgres.FinalOutcomes 满足；只声明读的这一口。
type CurrentFinalOutcomeSource interface {
	FindCurrentFinal(
		ctx context.Context,
		tenant psdomain.TenantID,
		parcel psdomain.DeclaredParcelID,
	) (psports.FinalOutcomeRecord, bool, error)
}

// EffectiveDeliveryFinalAdapter 实现 saports.EffectiveDeliveryFinalView：费用在终局之后才形成时，形成那一笔经它回读 PS
// 当前终局（ADR-0336 决定二）。只认已终局的「有效交付」，理由同 RequestOnFinalOutcomeAdapter。
type EffectiveDeliveryFinalAdapter struct {
	finals CurrentFinalOutcomeSource
}

var _ saports.EffectiveDeliveryFinalView = (*EffectiveDeliveryFinalAdapter)(nil)

func NewEffectiveDeliveryFinalAdapter(finals CurrentFinalOutcomeSource) (*EffectiveDeliveryFinalAdapter, error) {
	if finals == nil {
		return nil, fmt.Errorf("settlement accounting parcelshipment adapter: current final outcome source is nil")
	}
	return &EffectiveDeliveryFinalAdapter{finals: finals}, nil
}

func (adapter *EffectiveDeliveryFinalAdapter) FindEffectiveDeliveryFinal(
	ctx context.Context,
	tenant sadomain.TenantID,
	parcel sadomain.ParcelReference,
) (sadomain.ConfirmationBasisReference, bool, error) {
	none := sadomain.ConfirmationBasisReference{}
	psTenant, err := psdomain.NewTenantID(tenant.String())
	if err != nil {
		return none, false, fmt.Errorf("%w: tenant: %v", ErrUntranslatableAnswer, err)
	}
	psParcel, err := psdomain.NewDeclaredParcelID(parcel.String())
	if err != nil {
		return none, false, fmt.Errorf("%w: parcel: %v", ErrUntranslatableAnswer, err)
	}
	record, found, err := adapter.finals.FindCurrentFinal(ctx, psTenant, psParcel)
	if err != nil {
		return none, false, err
	}
	if !found || !record.Finalized || record.Key.Kind != psdomain.EffectiveDeliveryOutcome {
		return none, false, nil
	}
	basis, err := effectiveDeliveryBasisOf(record.Key)
	if err != nil {
		return none, false, err
	}
	return basis, true, nil
}

// ChargeConfirmer 是「记依据并按时点确认」那一口。真实装配接 saapplication.ConfirmChargeOnBasisHandler。
type ChargeConfirmer interface {
	OnParcel(
		ctx context.Context,
		tenant sadomain.TenantID,
		parcel sadomain.ParcelReference,
		basis saapplication.ConfirmationBasis,
	) (saapplication.ConfirmOnBasisResult, error)
}

// ConfirmOnFinalOutcomeAdapter 是终局信封那一路的真实处理方（sainbox.NewConfirmationBasisOnFinalOutcomeConsumer 装出的消费门）：按信封四维
// 回读终局记录，已终局的有效交付给这件包裹已形成的客户费用记确认依据并按时点确认。费用还没形成的什么都不记，那一拍由
// 形成那一笔补上。
type ConfirmOnFinalOutcomeAdapter struct {
	finals    FinalOutcomeSource
	confirmer ChargeConfirmer
}

var _ sainbox.FormedFinalOutcomeHandler = (*ConfirmOnFinalOutcomeAdapter)(nil)

func NewConfirmOnFinalOutcomeAdapter(finals FinalOutcomeSource, confirmer ChargeConfirmer) (*ConfirmOnFinalOutcomeAdapter, error) {
	if finals == nil {
		return nil, fmt.Errorf("settlement accounting parcelshipment adapter: final outcome source is nil")
	}
	if confirmer == nil {
		return nil, fmt.Errorf("settlement accounting parcelshipment adapter: charge confirmer is nil")
	}
	return &ConfirmOnFinalOutcomeAdapter{finals: finals, confirmer: confirmer}, nil
}

func (adapter *ConfirmOnFinalOutcomeAdapter) HandleFormedFinalOutcome(
	ctx context.Context,
	formed sainbox.FormedFinalOutcome,
) error {
	key, err := finalAdoptionKeyOf(formed)
	if err != nil {
		return err
	}
	record, found, err := adapter.finals.FindByKey(ctx, key)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrFinalOutcomeNotVisible, err)
	}
	if !found {
		return fmt.Errorf("%w: parcel %q kind %q version %q",
			ErrFinalOutcomeNotVisible, formed.Parcel, formed.Kind, formed.Version)
	}
	if record.Key != key {
		return fmt.Errorf("%w: final outcome parcel %q kind %q version %q",
			ErrAdoptionRecordInconsistent, formed.Parcel, formed.Kind, formed.Version)
	}
	if !record.Finalized || record.Key.Kind != psdomain.EffectiveDeliveryOutcome {
		return nil
	}
	basis, err := effectiveDeliveryBasisOf(key)
	if err != nil {
		return err
	}
	tenant, err := sadomain.NewTenantID(key.TenantID.String())
	if err != nil {
		return fmt.Errorf("%w: tenant: %v", ErrUntranslatableAnswer, err)
	}
	parcel, err := sadomain.NewParcelReference(key.Parcel.String())
	if err != nil {
		return fmt.Errorf("%w: parcel: %v", ErrUntranslatableAnswer, err)
	}
	result, err := adapter.confirmer.OnParcel(ctx, tenant, parcel, saapplication.ConfirmationBasis{
		Kind: sadomain.EffectiveDeliveryFinalizedBasis(), Reference: basis,
	})
	if err != nil {
		return err
	}
	if result.Retryable() {
		cause := result.UndecidedCause()
		if cause == nil {
			cause = fmt.Errorf("charge confirmation %s", result.Confirmation().UndecidedReason())
		}
		return fmt.Errorf("%w: %v", ErrConfirmOnBasisUndecided, cause)
	}
	return nil
}

// effectiveDeliveryBasisOf 把终局采用键译成确认依据引用。写法与 SELL 请求的来源事实引用同形（parcelMomentCommand），
// 两处指的都是同一份终局采用。
func effectiveDeliveryBasisOf(key psports.FinalAdoptionKey) (sadomain.ConfirmationBasisReference, error) {
	basis, err := sadomain.NewConfirmationBasisReference(
		"final-outcome/" + key.Parcel.String() + "/" + key.Kind.String() + "/" + key.Version.String())
	if err != nil {
		return sadomain.ConfirmationBasisReference{}, fmt.Errorf("%w: confirmation basis: %v", ErrUntranslatableAnswer, err)
	}
	return basis, nil
}
