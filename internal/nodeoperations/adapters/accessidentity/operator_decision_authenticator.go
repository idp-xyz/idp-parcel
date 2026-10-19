package accessidentity

import (
	"context"
	"errors"
	"fmt"

	identity "go.idp.xyz/idp-parcel/internal/accessidentity"
	nodeopshttp "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/http"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
)

// OperatorDecisionAuthenticator 实现 nodeopshttp.OperatorDecisionAuthenticator：按本口的决定种类铸「运营决定」信封，并判准入
// 范围——重核可能形成一版关联收寄、交 PS 采用，是生产事实（ADR-0151 决定三）。
type OperatorDecisionAuthenticator struct {
	minter Minter
}

var _ nodeopshttp.OperatorDecisionAuthenticator = (*OperatorDecisionAuthenticator)(nil)

func NewOperatorDecisionAuthenticator(minter Minter) (*OperatorDecisionAuthenticator, error) {
	if minter == nil {
		return nil, errors.New("node operations accessidentity: operator decision authenticator needs a minter")
	}
	return &OperatorDecisionAuthenticator{minter: minter}, nil
}

func (authenticator *OperatorDecisionAuthenticator) AuthenticateOperatorDecision(
	ctx context.Context,
	bearerToken string,
	decision nodeopshttp.OperatorDecision,
) (domain.TenantID, error) {
	kind, err := identity.ParseDecisionKind(string(decision))
	if err != nil {
		return domain.TenantID{}, fmt.Errorf("node operations accessidentity: decision %q: %w", decision, err)
	}
	envelope, err := authenticator.minter.MintOperator(ctx, identity.NewOperatorCredential(bearerToken), identity.OperatorRequest{
		Face:         identity.CapabilityOperationDecision,
		DecisionKind: kind,
		Admission:    &identity.AdmissionRequirement{Capability: identity.CapabilityOperationDecision.String(), FactKind: kind.String()},
	})
	if err != nil {
		return domain.TenantID{}, answerOperatorDecision(err)
	}
	tenant, err := domain.NewTenantID(envelope.TenantID())
	if err != nil {
		return domain.TenantID{}, fmt.Errorf("node operations accessidentity: tenant: %w", err)
	}
	return tenant, nil
}

// answerOperatorDecision 把操作者铸造的格译成 nodeopshttp 的格。与 answerOperationsRead 分开：运营决定判准入范围，多一格「不在
// 准入范围」，准入范围读不动也归身份依赖读不动。
func answerOperatorDecision(err error) error {
	switch {
	case errors.Is(err, identity.ErrAccessChannelNotConfigured):
		return fmt.Errorf("%w: %w", nodeopshttp.ErrAccessChannelNotConfigured, err)
	case errors.Is(err, identity.ErrCredentialRejected):
		return fmt.Errorf("%w: %w", nodeopshttp.ErrOperatorCredentialRejected, err)
	case errors.Is(err, identity.ErrOperatorNotGranted):
		return fmt.Errorf("%w: %w", nodeopshttp.ErrOperatorNotGranted, err)
	case errors.Is(err, identity.ErrOutsideAdmissionScope):
		return fmt.Errorf("%w: %w", nodeopshttp.ErrOutsideAdmissionScope, err)
	case errors.Is(err, identity.ErrCredentialVerifierUnavailable),
		errors.Is(err, identity.ErrOperatorRegistryUnavailable),
		errors.Is(err, identity.ErrAdmissionScopeUnavailable):
		return fmt.Errorf("%w: %w", nodeopshttp.ErrIdentityDependencyUnavailable, err)
	}
	return err
}
