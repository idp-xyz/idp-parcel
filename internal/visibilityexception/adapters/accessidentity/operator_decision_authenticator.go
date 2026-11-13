package accessidentity

import (
	"context"
	"errors"
	"fmt"

	identity "go.idp.xyz/idp-parcel/internal/accessidentity"
	visibilityhttp "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/http"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
)

// OperatorDecisionAuthenticator 实现 visibilityhttp.OperatorDecisionAuthenticator：按本口的决定种类铸信封，并判准入范围——
// 接单、关闭与归并都形成生产事实，所以带准入要求，这一点与登记写面不同。
type OperatorDecisionAuthenticator struct {
	minter Minter
}

var _ visibilityhttp.OperatorDecisionAuthenticator = (*OperatorDecisionAuthenticator)(nil)

func NewOperatorDecisionAuthenticator(minter Minter) (*OperatorDecisionAuthenticator, error) {
	if minter == nil {
		return nil, errors.New("visibility accessidentity: operator decision authenticator needs a minter")
	}
	return &OperatorDecisionAuthenticator{minter: minter}, nil
}

// AuthenticateOperatorDecision 交回信封上的租户与主体。主体写成「发行方#sub」：sub 只在它的发行方之内唯一，单拿 sub 会让
// 两个发行方的同一个 sub 在接单人、关闭人里读成一个人。
func (authenticator *OperatorDecisionAuthenticator) AuthenticateOperatorDecision(
	ctx context.Context,
	bearerToken string,
	decision visibilityhttp.OperatorDecision,
) (visibilityhttp.OperatorIdentity, error) {
	kind, err := identity.ParseDecisionKind(string(decision))
	if err != nil {
		return visibilityhttp.OperatorIdentity{}, fmt.Errorf("visibility accessidentity: decision %q: %w", decision, err)
	}
	envelope, err := authenticator.minter.MintOperator(ctx, identity.NewOperatorCredential(bearerToken), identity.OperatorRequest{
		Face:         identity.CapabilityOperationDecision,
		DecisionKind: kind,
		Admission:    &identity.AdmissionRequirement{Capability: identity.CapabilityOperationDecision.String(), FactKind: kind.String()},
	})
	if err != nil {
		return visibilityhttp.OperatorIdentity{}, answerDecision(err)
	}
	tenant, err := domain.NewTenantID(envelope.TenantID())
	if err != nil {
		return visibilityhttp.OperatorIdentity{}, fmt.Errorf("visibility accessidentity: tenant: %w", err)
	}
	subject := envelope.Subject()
	return visibilityhttp.OperatorIdentity{Tenant: tenant, Operator: subject.Issuer() + "#" + subject.Subject()}, nil
}

// answerDecision 在 answer 之上多译两格：运营决定带准入要求，所以会遇到「不在准入范围」与准入范围读不动。
func answerDecision(err error) error {
	switch {
	case errors.Is(err, identity.ErrOutsideAdmissionScope):
		return fmt.Errorf("%w: %w", visibilityhttp.ErrOutsideAdmissionScope, err)
	case errors.Is(err, identity.ErrAdmissionScopeUnavailable):
		return fmt.Errorf("%w: %w", visibilityhttp.ErrIdentityDependencyUnavailable, err)
	}
	return answer(err)
}
