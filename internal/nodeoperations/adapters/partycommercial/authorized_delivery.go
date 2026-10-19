// Package partycommercial 是 node-operations 消费 party-commercial 的适配器（ADR-0025 消费方侧）：问授权交付判断
// （ADR-0212 决定三，见 AuthorizedDeliveryView）。它只翻译不判断：交付方可不可以替这个客户账户送站，由 PC 自己的读口答；
// 参与方关系的形状（角色、范围、生命周期）不越过这一层进本上下文。
package partycommercial

import (
	"context"
	"errors"
	"fmt"

	noports "go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
	pcdomain "go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	pcports "go.idp.xyz/idp-parcel/internal/partycommercial/ports"
)

// ErrUntranslatableAnswer 语义与其他消费方适配器的同名哨兵一致：输入在提供方那边立不起来，或提供方的答复落在本上下文
// 认得的几格之外。
var ErrUntranslatableAnswer = errors.New("node operations partycommercial adapter: untranslatable answer")

// AuthorizedDeliveryView 把 PC 的授权交付判断译成 noports.AuthorizedDeliveryView。
type AuthorizedDeliveryView struct {
	judgments pcports.AuthorizedDeliveryJudgmentView
}

var _ noports.AuthorizedDeliveryView = (*AuthorizedDeliveryView)(nil)

// NewAuthorizedDeliveryView 构造期拒缺件：漏装的读口到第一次运营决定确认才暴露，会被当成一次依赖故障。
func NewAuthorizedDeliveryView(judgments pcports.AuthorizedDeliveryJudgmentView) (*AuthorizedDeliveryView, error) {
	if judgments == nil {
		return nil, errors.New("node operations partycommercial adapter: authorized delivery view needs a judgment view")
	}
	return &AuthorizedDeliveryView{judgments: judgments}, nil
}

// JudgeAuthorizedDelivery 逐格译问法、问 PC、译答复。PC 读不动原样包住交回，答复留空：「没问到」不能译成否，更不能译成是。
func (view *AuthorizedDeliveryView) JudgeAuthorizedDelivery(
	ctx context.Context,
	query noports.AuthorizedDeliveryQuery,
) (noports.AuthorizedDeliveryAnswer, error) {
	none := noports.AuthorizedDeliveryAnswerInvalid
	pcQuery, err := pcQueryOf(query)
	if err != nil {
		return none, err
	}
	answer, err := view.judgments.JudgeAuthorizedDelivery(ctx, pcQuery)
	if err != nil {
		return none, fmt.Errorf("node operations partycommercial adapter: judge authorized delivery: %w", err)
	}
	switch answer {
	case pcdomain.DeliveryAuthorized:
		return noports.DeliveryAuthorized, nil
	case pcdomain.DeliveryNotAuthorized:
		return noports.DeliveryNotAuthorized, nil
	case pcdomain.DeliveryAuthorizationNotConfigured:
		return noports.DeliveryAuthorizationNotConfigured, nil
	default:
		return none, fmt.Errorf("%w: authorized delivery answer %q", ErrUntranslatableAnswer, answer)
	}
}

func pcQueryOf(query noports.AuthorizedDeliveryQuery) (pcdomain.AuthorizedDeliveryQuery, error) {
	none := pcdomain.AuthorizedDeliveryQuery{}
	tenant, err := pcdomain.NewTenantID(query.Tenant.String())
	if err != nil {
		return none, fmt.Errorf("%w: tenant: %v", ErrUntranslatableAnswer, err)
	}
	party, err := pcdomain.NewPartyID(query.DeliveringParty.String())
	if err != nil {
		return none, fmt.Errorf("%w: delivering party: %v", ErrUntranslatableAnswer, err)
	}
	account, err := pcdomain.NewCustomerAccountID(query.CustomerAccount)
	if err != nil {
		return none, fmt.Errorf("%w: customer account: %v", ErrUntranslatableAnswer, err)
	}
	node, err := pcdomain.NewLogisticsNodeReference(query.Node.String())
	if err != nil {
		return none, fmt.Errorf("%w: node: %v", ErrUntranslatableAnswer, err)
	}
	if query.OpenedAt.IsZero() {
		return none, fmt.Errorf("%w: opened at is required", ErrUntranslatableAnswer)
	}
	return pcdomain.AuthorizedDeliveryQuery{
		Tenant:          tenant,
		DeliveringParty: party,
		CustomerAccount: account,
		Node:            node,
		At:              query.OpenedAt,
	}, nil
}
