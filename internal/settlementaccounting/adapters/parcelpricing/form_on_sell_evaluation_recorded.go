package parcelpricing

import (
	"context"
	"errors"
	"fmt"

	sainbox "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/inbox"
	sadomain "go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	saports "go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// ErrSellChargeReferencesUnrecorded 表示这份 SELL 评价还凑不齐客户费用的费用项目。
// 信封只有评价引用。不为缺的费用项目发明一个。
var ErrSellChargeReferencesUnrecorded = errors.New(
	"settlement accounting parcelpricing adapter: fee item for the sell charge is not recorded yet")

// FormOnSellEvaluationRecordedAdapter 分辨评价是不是 SELL·CUSTOMER_CHARGE。
// BUY 评价答 nil，让另一条消费门去处理。不调用 BUY 评价请求。
type FormOnSellEvaluationRecordedAdapter struct {
	evaluations saports.SellEvaluationView
}

func NewFormOnSellEvaluationRecordedAdapter(
	evaluations saports.SellEvaluationView,
) (*FormOnSellEvaluationRecordedAdapter, error) {
	if evaluations == nil {
		return nil, fmt.Errorf("settlement accounting parcelpricing adapter: sell evaluation view is nil")
	}
	return &FormOnSellEvaluationRecordedAdapter{evaluations: evaluations}, nil
}

func (adapter *FormOnSellEvaluationRecordedAdapter) HandleRecordedSellEvaluation(
	ctx context.Context,
	recorded sainbox.RecordedBuyEvaluation,
) error {
	tenant, err := sadomain.NewTenantID(recorded.TenantID)
	if err != nil {
		return fmt.Errorf("%w: tenant: %v", ErrUntranslatableReference, err)
	}
	reference, err := sadomain.NewSellEvaluationReference(recorded.EvaluationID)
	if err != nil {
		return fmt.Errorf("%w: evaluation: %v", ErrUntranslatableReference, err)
	}
	_, found, err := adapter.evaluations.LoadSellEvaluation(ctx, tenant, reference)
	switch {
	case errors.Is(err, ErrNotASellEvaluation):
		return nil
	case errors.Is(err, ErrUntranslatableAnswer), errors.Is(err, ErrAmountPrecisionUndeclared):
		return err
	case err != nil:
		return fmt.Errorf("%w: %v", ErrEvaluationNotVisible, err)
	case !found:
		return fmt.Errorf("%w: evaluation %s", ErrEvaluationNotVisible, reference)
	}
	return fmt.Errorf("%w: evaluation %s", ErrSellChargeReferencesUnrecorded, reference)
}
