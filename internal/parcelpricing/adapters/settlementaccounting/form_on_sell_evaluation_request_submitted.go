package settlementaccounting

import (
	"context"
	"fmt"

	ppinbox "go.idp.xyz/idp-parcel/internal/parcelpricing/adapters/inbox"
	ppdomain "go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	sadomain "go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	saports "go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// FormOnSellEvaluationRequestSubmittedAdapter 是 ppinbox.SellEvaluationRequestSubmittedConsumer 的真实处理方（ADR-0190
// 决定四）：按信封引用向 SA 的 SELL 请求册取那一份，译成命令交「按评价请求形成评价」入口——与 BUY 那一路同一只入口，
// 选卡、按回指幂等、入册与交付都在那里——再把入口的结果照 BUY 那一路同一套折法落成消费两格。
//
// 哨兵与 BUY 那一路共用：每一枚说的是入口停在哪一格、等谁，与请求出自哪本册无关；装配为两路各登一份未决名单。
type FormOnSellEvaluationRequestSubmittedAdapter struct {
	requests SellEvaluationRequestSource
	former   EvaluationFormer
	evidence ppdomain.EvidenceKind
}

// NewFormOnSellEvaluationRequestSubmittedAdapter 构造期拒 nil 与未声明的证据层级，理由同 BUY 那一路。
func NewFormOnSellEvaluationRequestSubmittedAdapter(
	requests SellEvaluationRequestSource,
	former EvaluationFormer,
	evidence ppdomain.EvidenceKind,
) (*FormOnSellEvaluationRequestSubmittedAdapter, error) {
	if requests == nil {
		return nil, fmt.Errorf("parcel pricing settlementaccounting adapter: sell evaluation request source is nil")
	}
	if former == nil {
		return nil, fmt.Errorf("parcel pricing settlementaccounting adapter: evaluation former is nil")
	}
	if !evidence.Declared() {
		return nil, fmt.Errorf("parcel pricing settlementaccounting adapter: evidence kind %q is not one of S / R / P", evidence)
	}
	return &FormOnSellEvaluationRequestSubmittedAdapter{requests: requests, former: former, evidence: evidence}, nil
}

var _ ppinbox.SubmittedSellEvaluationRequestHandler = (*FormOnSellEvaluationRequestSubmittedAdapter)(nil)

// HandleSubmittedSellEvaluationRequest 译引用 → 取请求 → 译命令 → 交入口 → 折结果。请求册上的状态格不看：同一请求
// 再来一封，入口按回指答已存在，状态由 SA 自己的消费门改记（ADR-0190 决定五）。
func (adapter *FormOnSellEvaluationRequestSubmittedAdapter) HandleSubmittedSellEvaluationRequest(
	ctx context.Context,
	submitted ppinbox.SubmittedSellEvaluationRequest,
) error {
	tenant, err := sadomain.NewTenantID(submitted.TenantID)
	if err != nil {
		return fmt.Errorf("%w: tenant: %v", ErrUntranslatableReference, err)
	}
	requestID, err := sadomain.NewSellEvaluationRequestID(submitted.SellEvaluationRequestID)
	if err != nil {
		return fmt.Errorf("%w: sell evaluation request: %v", ErrUntranslatableReference, err)
	}

	record, found, err := adapter.requests.FindByID(ctx, saports.SellEvaluationRequestKey{TenantID: tenant, Request: requestID})
	if err != nil {
		return fmt.Errorf("%w: %v", ErrEvaluationRequestNotVisible, err)
	}
	if !found {
		return fmt.Errorf("%w: sell evaluation request %s", ErrEvaluationRequestNotVisible, requestID)
	}

	command, err := TranslateSellEvaluationRequest(record, adapter.evidence)
	if err != nil {
		return err
	}
	result, err := adapter.former.Handle(ctx, command)
	if err != nil {
		return fmt.Errorf("form evaluation from sell request %s: %w", requestID, err)
	}
	return foldFormation(command, requestID.String(), result)
}
