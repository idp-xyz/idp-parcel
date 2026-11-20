package accessidentity

import (
	"context"
	"errors"
	"fmt"

	identity "go.idp.xyz/idp-parcel/internal/accessidentity"
	warehousehttp "go.idp.xyz/idp-parcel/internal/warehousefulfillment/adapters/http"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
)

// OperatorDiscrepancyConclusionAuthenticator 实现收货差异结论代录的认证：核这一种运营决定的授予，并判准入范围。
// 准入的能力词与事实类型词都用这一种决定的字面量（ADR-0384 决定八）。不带决定标识。
type OperatorDiscrepancyConclusionAuthenticator struct {
	minter Minter
}

var _ warehousehttp.DiscrepancyConclusionAuthenticator = (*OperatorDiscrepancyConclusionAuthenticator)(nil)

func NewOperatorDiscrepancyConclusionAuthenticator(minter Minter) (*OperatorDiscrepancyConclusionAuthenticator, error) {
	if minter == nil {
		return nil, errors.New("warehouse fulfillment accessidentity: receipt discrepancy conclusion authenticator needs a minter")
	}
	return &OperatorDiscrepancyConclusionAuthenticator{minter: minter}, nil
}

func (authenticator *OperatorDiscrepancyConclusionAuthenticator) AuthenticateDiscrepancyConclusion(
	ctx context.Context,
	bearerToken string,
) (warehousehttp.OperatorIdentity, error) {
	kind := identity.DecisionReceiptDiscrepancyRecordOnBehalf
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
