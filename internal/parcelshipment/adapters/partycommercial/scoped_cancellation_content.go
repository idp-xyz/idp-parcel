package partycommercial

import (
	"context"
	"errors"
	"fmt"

	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	pcdomain "go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	pcports "go.idp.xyz/idp-parcel/internal/partycommercial/ports"
)

// ErrCancellationAuthorityConflict 表示同一范围、接受时点上不止一版授权规则登记了取消授权目录：取消授权说不出该听哪一版的。
// 它是登记册的问题（租户要把多余的一版收尾），不是授权服务的问题，也不折成「未配置」——那会让运维去等一份其实已经登了两份的配置。
var ErrCancellationAuthorityConflict = errors.New(
	"parcel shipment partycommercial adapter: several authorization rules in force declare a cancellation authority")

// ScopedCancellationContent 实现 CancellationContentSource：按委托所属账户的商业范围、委托接受那一刻，找出在效的、登记了取消
// 授权目录的那一版授权规则，读它的目录（ADR-0313）。
//
// 为什么不走接受时固定的闭包（ResolvedAdoptedStageOwner）：闭包按「范围内某一类对象恰有一版」选，而授权规则在同一范围里常有
// 不止一版——价格 / 成本方向授权、资料修订的授予载体（ADR-0312）与取消授权目录各自是一版授权规则。把授权规则列进必需依据，
// 这些并存的版本会让解析答适用冲突、接受跟着停下。取消授权要的不是「那一个授权规则」，而是「登记了取消目录的那一个」，所以这里按
// 目录的在场收窄。
//
// 「接受时点」取委托接受决定的时刻，不取当下：取消授权跟着接受时生效的那一版走，与终局规则、收寄资格同一条纪律（ADR-0058、ADR-0062）。
// 没有账户解析键、委托尚未接受、该时点没有登记取消目录的授权规则，都交回 found=false，授权停在`目录未配置`。
type ScopedCancellationContent struct {
	requests  acceptedShipmentFinder
	keys      ResolutionKeyStore
	authority pcports.CommercialAuthorityView
	catalog   pcports.CancellationAuthorityContentView
}

func NewScopedCancellationContent(
	requests acceptedShipmentFinder,
	keys ResolutionKeyStore,
	authority pcports.CommercialAuthorityView,
	catalog pcports.CancellationAuthorityContentView,
) (*ScopedCancellationContent, error) {
	if requests == nil || keys == nil || authority == nil || catalog == nil {
		return nil, fmt.Errorf("parcel shipment partycommercial adapter: scoped cancellation content needs requests, resolution keys, an authority view and a catalog view")
	}
	return &ScopedCancellationContent{requests: requests, keys: keys, authority: authority, catalog: catalog}, nil
}

var _ CancellationContentSource = (*ScopedCancellationContent)(nil)

func (source *ScopedCancellationContent) CancellationContentFor(
	ctx context.Context,
	identity psdomain.SourceIdentity,
) (pcdomain.CancellationAuthorityContent, bool, error) {
	none := pcdomain.CancellationAuthorityContent{}
	request, found, err := source.requests.FindBySourceIdentity(ctx, identity)
	if err != nil {
		return none, false, fmt.Errorf("cancellation content: load shipment request: %w", err)
	}
	if !found || request.State() != psdomain.ShipmentRequestAccepted {
		return none, false, nil
	}
	decision, present := request.AcceptanceDecision()
	if !present {
		return none, false, nil
	}
	row, found, err := source.keys.FindResolutionKey(ctx, identity.TenantID().String(), identity.CustomerAccountID().String())
	if err != nil {
		return none, false, fmt.Errorf("cancellation content: resolution key: %w", err)
	}
	if !found {
		return none, false, nil
	}
	tenant, err := pcTenantOf(identity)
	if err != nil {
		return none, false, err
	}
	scope, err := pcdomain.NewCommercialScopeReference(row.Scope)
	if err != nil {
		return none, false, fmt.Errorf("%w: scope: %v", ErrUntranslatableAnswer, err)
	}
	registry, err := source.authority.LoadScope(ctx, tenant, scope)
	if err != nil {
		return none, false, fmt.Errorf("cancellation content: load scope: %w", err)
	}

	var declared []pcdomain.CancellationAuthorityContent
	for _, rule := range registry.EffectiveVersionsAt(tenant, scope, pcdomain.AuthorizationRuleObject, decision.DecidedAt()) {
		content, configured, err := source.catalog.LoadCancellationAuthority(ctx, tenant, rule)
		if err != nil {
			return none, false, fmt.Errorf("cancellation content: load catalog: %w", err)
		}
		if configured {
			declared = append(declared, content)
		}
	}
	switch len(declared) {
	case 0:
		return none, false, nil
	case 1:
		return declared[0], true, nil
	default:
		return none, false, fmt.Errorf("%w: %d versions in scope %s", ErrCancellationAuthorityConflict, len(declared), scope)
	}
}
