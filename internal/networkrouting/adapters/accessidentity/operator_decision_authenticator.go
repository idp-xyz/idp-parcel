package accessidentity

import (
	"context"
	"errors"
	"fmt"

	identity "go.idp.xyz/idp-parcel/internal/accessidentity"
	networkhttp "go.idp.xyz/idp-parcel/internal/networkrouting/adapters/http"
	"go.idp.xyz/idp-parcel/internal/networkrouting/domain"
)

// OperatorDecisionAuthenticator 实现 networkhttp.OperatorDecisionAuthenticator：按本口的决定种类铸信封，并判准入范围——授权改路
// 形成生产事实（新计划进路由历史），准入照 ADR-0151 决定三、ADR-0149 决定四判（ADR-0284 决定一）。
type OperatorDecisionAuthenticator struct {
	minter Minter
}

var _ networkhttp.OperatorDecisionAuthenticator = (*OperatorDecisionAuthenticator)(nil)

func NewOperatorDecisionAuthenticator(minter Minter) (*OperatorDecisionAuthenticator, error) {
	if minter == nil {
		return nil, errors.New("network accessidentity: operator decision authenticator needs a minter")
	}
	return &OperatorDecisionAuthenticator{minter: minter}, nil
}

// AuthenticateOperatorDecision 交回信封上的租户与主体。主体写成「发行方#sub」：sub 只在它的发行方之内唯一（ADR-0100 决定二），
// 单拿 sub 会让两个发行方的同一个 sub 在决定上读成一个人。
func (authenticator *OperatorDecisionAuthenticator) AuthenticateOperatorDecision(
	ctx context.Context,
	bearerToken string,
	decision networkhttp.OperatorDecision,
) (networkhttp.OperatorIdentity, error) {
	kind, err := identity.ParseDecisionKind(string(decision))
	if err != nil {
		return networkhttp.OperatorIdentity{}, fmt.Errorf("network accessidentity: decision %q: %w", decision, err)
	}
	envelope, err := authenticator.minter.MintOperator(ctx, identity.NewOperatorCredential(bearerToken), identity.OperatorRequest{
		Face:         identity.CapabilityOperationDecision,
		DecisionKind: kind,
		Admission:    &identity.AdmissionRequirement{Capability: identity.CapabilityOperationDecision.String(), FactKind: kind.String()},
	})
	if err != nil {
		return networkhttp.OperatorIdentity{}, answerOperatorDecision(err)
	}
	tenant, err := domain.NewTenantID(envelope.TenantID())
	if err != nil {
		return networkhttp.OperatorIdentity{}, fmt.Errorf("network accessidentity: tenant: %w", err)
	}
	subject := envelope.Subject()
	return networkhttp.OperatorIdentity{Tenant: tenant, Operator: subject.Issuer() + "#" + subject.Subject()}, nil
}

// answerOperatorDecision 比登记写面那一份（answer）多出准入范围的两格：运营决定形成生产事实，登记与查阅不形成。
func answerOperatorDecision(err error) error {
	switch {
	case errors.Is(err, identity.ErrOutsideAdmissionScope):
		return fmt.Errorf("%w: %w", networkhttp.ErrOutsideAdmissionScope, err)
	case errors.Is(err, identity.ErrAdmissionScopeUnavailable):
		return fmt.Errorf("%w: %w", networkhttp.ErrIdentityDependencyUnavailable, err)
	}
	return answer(err)
}
