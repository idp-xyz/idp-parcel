package accessidentity

import (
	"context"
	"errors"
	"fmt"

	identity "go.idp.xyz/idp-parcel/internal/accessidentity"
	customshttp "go.idp.xyz/idp-parcel/internal/customscompliance/adapters/http"
	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
)

// OperatorDecisionAuthenticator 实现 customshttp.OperatorDecisionAuthenticator：按本口的决定种类铸信封，并判准入范围——
// 建案、授权确认与提交执行都形成生产事实，准入照 ADR-0151 决定三、ADR-0149 决定四判（ADR-0227 决定一）。
type OperatorDecisionAuthenticator struct {
	minter Minter
}

var _ customshttp.OperatorDecisionAuthenticator = (*OperatorDecisionAuthenticator)(nil)

func NewOperatorDecisionAuthenticator(minter Minter) (*OperatorDecisionAuthenticator, error) {
	if minter == nil {
		return nil, errors.New("customs accessidentity: operator decision authenticator needs a minter")
	}
	return &OperatorDecisionAuthenticator{minter: minter}, nil
}

// AuthenticateOperatorDecision 交回信封上的租户与主体。主体写成「发行方#sub」：sub 只在它的发行方之内唯一（ADR-0100 决定二
// 第三条），单拿 sub 会让两个发行方的同一个 sub 在授权依据里读成一个人；授权确认把它记作确认人。
func (authenticator *OperatorDecisionAuthenticator) AuthenticateOperatorDecision(
	ctx context.Context,
	bearerToken string,
	decision customshttp.OperatorDecision,
) (customshttp.OperatorIdentity, error) {
	kind, err := identity.ParseDecisionKind(string(decision))
	if err != nil {
		return customshttp.OperatorIdentity{}, fmt.Errorf("customs accessidentity: decision %q: %w", decision, err)
	}
	envelope, err := authenticator.minter.MintOperator(ctx, identity.NewOperatorCredential(bearerToken), identity.OperatorRequest{
		Face:         identity.CapabilityOperationDecision,
		DecisionKind: kind,
		Admission:    &identity.AdmissionRequirement{Capability: identity.CapabilityOperationDecision.String(), FactKind: kind.String()},
	})
	if err != nil {
		return customshttp.OperatorIdentity{}, answer(err)
	}
	tenant, err := domain.NewTenantID(envelope.TenantID())
	if err != nil {
		return customshttp.OperatorIdentity{}, fmt.Errorf("customs accessidentity: tenant: %w", err)
	}
	subject := envelope.Subject()
	return customshttp.OperatorIdentity{Tenant: tenant, Operator: subject.Issuer() + "#" + subject.Subject()}, nil
}
