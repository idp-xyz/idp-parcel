package application_test

import (
	"context"
	"testing"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
)

// pricingParameterDouble 记下交出的「参数已登记」，可令它失败。
type pricingParameterDouble struct {
	intents []ports.PricingParameterRegisteredIntent
	err     error
}

func (double *pricingParameterDouble) HandOffPricingParameterRegistered(
	_ context.Context, intent ports.PricingParameterRegisteredIntent,
) error {
	double.intents = append(double.intents, intent)
	return double.err
}

// pricedScopesDouble 答绑了某条序列或某份目录的卡在哪些范围上，记下问过什么，可令它失败。
type pricedScopesDouble struct {
	scopes []ports.PricedScope
	asked  []string
	err    error
}

func (double *pricedScopesDouble) FindPricedScopesBinding(
	_ context.Context, _ domain.TenantID, kind ports.PricingParameterKind, id string,
) ([]ports.PricedScope, error) {
	double.asked = append(double.asked, kind.String()+"/"+id)
	return double.scopes, double.err
}

// boundScopes 是绑定读口的常用答复：一张 BUY 卡、一张 SELL 卡、一张内部卡各在自己的范围上。
func boundScopes(t *testing.T) []ports.PricedScope {
	t.Helper()
	return []ports.PricedScope{
		{Scope: mustValue(t, domain.NewPricingScopeID, "SYN-SCOPE-BUY"), Direction: domain.PricingDirectionBuy},
		{Scope: mustValue(t, domain.NewPricingScopeID, "SYN-SCOPE-SELL"), Direction: domain.PricingDirectionSell},
		{Scope: mustValue(t, domain.NewPricingScopeID, "SYN-SCOPE-INTERNAL"), Direction: domain.PricingDirectionInternal},
	}
}
