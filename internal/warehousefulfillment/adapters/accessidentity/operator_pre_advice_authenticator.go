package accessidentity

import (
	"context"
	"errors"
	"fmt"

	identity "go.idp.xyz/idp-parcel/internal/accessidentity"
	warehousehttp "go.idp.xyz/idp-parcel/internal/warehousefulfillment/adapters/http"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
)

// OperatorPreAdviceAuthenticator 实现入库预报代录的认证：核「入库预报代录」这一种运营决定的授予，并判准入范围。
// 面仍是 CapabilityOperationDecision。准入的能力词与事实类型词都用这一种决定的字面量，不用 CapabilityOperationDecision
// （ADR-0384 决定八）：它没有作业事实，事实类型词就用能力词。
type OperatorPreAdviceAuthenticator struct {
	minter Minter
}

var _ warehousehttp.PreAdviceOnBehalfAuthenticator = (*OperatorPreAdviceAuthenticator)(nil)

func NewOperatorPreAdviceAuthenticator(minter Minter) (*OperatorPreAdviceAuthenticator, error) {
	if minter == nil {
		return nil, errors.New("warehouse fulfillment accessidentity: inbound pre-advice on-behalf authenticator needs a minter")
	}
	return &OperatorPreAdviceAuthenticator{minter: minter}, nil
}

func (authenticator *OperatorPreAdviceAuthenticator) AuthenticatePreAdviceOnBehalf(
	ctx context.Context,
	bearerToken string,
) (warehousehttp.OperatorIdentity, error) {
	kind := identity.DecisionInboundPreAdviceRecordOnBehalf
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
