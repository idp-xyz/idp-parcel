package accessidentity

import (
	"context"
	"errors"
	"fmt"

	identity "go.idp.xyz/idp-parcel/internal/accessidentity"
	settlementhttp "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/http"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

// OperatorDecisionAuthenticator 实现 settlementhttp.OperatorDecisionAuthenticator：按本口的决定种类铸信封，并判准入范围——采用
// 一版外部资金事实形成生产事实，准入照 ADR-0151 决定三、ADR-0149 决定四判（ADR-0236 决定二）。
type OperatorDecisionAuthenticator struct {
	minter OperatorMinter
}

var _ settlementhttp.OperatorDecisionAuthenticator = (*OperatorDecisionAuthenticator)(nil)

func NewOperatorDecisionAuthenticator(minter OperatorMinter) (*OperatorDecisionAuthenticator, error) {
	if minter == nil {
		return nil, errors.New("settlement accounting accessidentity: operator decision authenticator needs a minter")
	}
	return &OperatorDecisionAuthenticator{minter: minter}, nil
}

// AuthenticateOperatorDecision 交回信封上的租户与主体。主体写成「发行方#sub」：sub 只在它的发行方之内唯一（ADR-0100 决定二
// 第三条），单拿 sub 会让两个发行方的同一个 sub 在操作证据里读成一个人。
func (authenticator *OperatorDecisionAuthenticator) AuthenticateOperatorDecision(
	ctx context.Context,
	bearerToken string,
	decision settlementhttp.OperatorDecision,
) (settlementhttp.OperatorIdentity, error) {
	kind, err := identity.ParseDecisionKind(string(decision))
	if err != nil {
		return settlementhttp.OperatorIdentity{}, fmt.Errorf("settlement accounting accessidentity: decision %q: %w", decision, err)
	}
	envelope, err := authenticator.minter.MintOperator(ctx, identity.NewOperatorCredential(bearerToken), identity.OperatorRequest{
		Face:         identity.CapabilityOperationDecision,
		DecisionKind: kind,
		Admission:    &identity.AdmissionRequirement{Capability: identity.CapabilityOperationDecision.String(), FactKind: kind.String()},
	})
	if err != nil {
		return settlementhttp.OperatorIdentity{}, answerOperatorDecision(err)
	}
	tenant, err := domain.NewTenantID(envelope.TenantID())
	if err != nil {
		return settlementhttp.OperatorIdentity{}, fmt.Errorf("settlement accounting accessidentity: tenant: %w", err)
	}
	subject := envelope.Subject()
	return settlementhttp.OperatorIdentity{Tenant: tenant, Operator: subject.Issuer() + "#" + subject.Subject()}, nil
}

// answerOperatorDecision 把操作者铸造的格译成 settlementhttp 的格。比查阅读那一份多出准入范围的格：运营决定形成生产事实，查阅不形成。
func answerOperatorDecision(err error) error {
	switch {
	case errors.Is(err, identity.ErrAccessChannelNotConfigured):
		return fmt.Errorf("%w: %w", settlementhttp.ErrAccessChannelNotConfigured, err)
	case errors.Is(err, identity.ErrCredentialRejected):
		return fmt.Errorf("%w: %w", settlementhttp.ErrOperatorCredentialRejected, err)
	case errors.Is(err, identity.ErrOperatorNotGranted):
		return fmt.Errorf("%w: %w", settlementhttp.ErrOperatorNotGranted, err)
	case errors.Is(err, identity.ErrOutsideAdmissionScope):
		return fmt.Errorf("%w: %w", settlementhttp.ErrOutsideAdmissionScope, err)
	case errors.Is(err, identity.ErrCredentialVerifierUnavailable),
		errors.Is(err, identity.ErrOperatorRegistryUnavailable),
		errors.Is(err, identity.ErrAdmissionScopeUnavailable):
		return fmt.Errorf("%w: %w", settlementhttp.ErrIdentityDependencyUnavailable, err)
	}
	return err
}
