package accessidentity

import (
	"context"
	"errors"
	"fmt"

	identity "go.idp.xyz/idp-parcel/internal/accessidentity"
	warehousehttp "go.idp.xyz/idp-parcel/internal/warehousefulfillment/adapters/http"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
)

// OperatorGoodsAuthenticator 实现商品代录的认证：核「商品代录」这一种运营决定的授予，并判准入范围。
// 代录形成生产事实，所以带准入要求，这一点与登记册配置写不同（ADR-0100 决定四，ADR-0151 决定三）。
// 准入的能力词与事实类型词都用这一种决定的字面量：它没有作业事实，事实类型词就用能力词（ADR-0384 决定八）。
type OperatorGoodsAuthenticator struct {
	minter Minter
}

var _ warehousehttp.GoodsOnBehalfAuthenticator = (*OperatorGoodsAuthenticator)(nil)

func NewOperatorGoodsAuthenticator(minter Minter) (*OperatorGoodsAuthenticator, error) {
	if minter == nil {
		return nil, errors.New("warehouse fulfillment accessidentity: goods on-behalf authenticator needs a minter")
	}
	return &OperatorGoodsAuthenticator{minter: minter}, nil
}

func (authenticator *OperatorGoodsAuthenticator) AuthenticateGoodsOnBehalf(
	ctx context.Context,
	bearerToken string,
) (warehousehttp.OperatorIdentity, error) {
	kind := identity.DecisionGoodsRecordOnBehalf
	envelope, err := authenticator.minter.MintOperator(ctx, identity.NewOperatorCredential(bearerToken), identity.OperatorRequest{
		Face:         identity.CapabilityOperationDecision,
		DecisionKind: kind,
		Admission:    &identity.AdmissionRequirement{Capability: kind.String(), FactKind: kind.String()},
	})
	if err != nil {
		return warehousehttp.OperatorIdentity{}, answerGoods(err)
	}
	tenant, err := domain.NewTenantID(envelope.TenantID())
	if err != nil {
		return warehousehttp.OperatorIdentity{}, fmt.Errorf("warehouse fulfillment accessidentity: tenant: %w", err)
	}
	subject := envelope.Subject()
	registrant, err := domain.NewRegistrant(subject.Issuer() + "#" + subject.Subject())
	if err != nil {
		return warehousehttp.OperatorIdentity{}, fmt.Errorf("warehouse fulfillment accessidentity: registrant: %w", err)
	}
	return warehousehttp.OperatorIdentity{Tenant: tenant, Registrant: registrant}, nil
}

// answerGoods 在登记写面的译法之上多译一格：代录带准入要求，所以会遇到「不在准入范围」。准入范围读不动与身份依赖读不动同一格。
func answerGoods(err error) error {
	switch {
	case errors.Is(err, identity.ErrOutsideAdmissionScope):
		return fmt.Errorf("%w: %w", warehousehttp.ErrOutsideAdmissionScope, err)
	case errors.Is(err, identity.ErrAdmissionScopeUnavailable):
		return fmt.Errorf("%w: %w", warehousehttp.ErrIdentityDependencyUnavailable, err)
	}
	return answer(err)
}
