package accessidentity

import (
	"context"
	"errors"
	"fmt"

	identity "go.idp.xyz/idp-parcel/internal/accessidentity"
	nodeopshttp "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/http"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
)

// WorkFactMinter 是本适配器对铸造作业事实信封的最小依赖。
type WorkFactMinter interface {
	MintWorkFact(ctx context.Context, credential identity.OperatorCredential, request identity.WorkFactRequest) (identity.WorkFactEnvelope, error)
}

// WorkFactAuthenticator 以作业事实登记能力面铸信封。节点取作业范围的引用，种类必须是 NODE。
type WorkFactAuthenticator struct {
	minter WorkFactMinter
}

var _ nodeopshttp.WorkFactAuthenticator = (*WorkFactAuthenticator)(nil)

func NewWorkFactAuthenticator(minter WorkFactMinter) (*WorkFactAuthenticator, error) {
	if minter == nil {
		return nil, errors.New("node operations accessidentity: work fact authenticator needs a minter")
	}
	return &WorkFactAuthenticator{minter: minter}, nil
}

func (authenticator *WorkFactAuthenticator) AuthenticateNodeReception(
	ctx context.Context,
	bearerToken, deviceID, scopeKind, scopeReference string,
) (domain.TenantID, domain.NodeReference, error) {
	none := domain.NodeReference{}
	scope, err := identity.NewWorkScope(identity.WorkScopeKind(scopeKind), scopeReference)
	if err != nil {
		return domain.TenantID{}, none, fmt.Errorf("%w: %v", nodeopshttp.ErrMalformedRequest, err)
	}
	envelope, err := authenticator.minter.MintWorkFact(ctx, identity.NewOperatorCredential(bearerToken), identity.WorkFactRequest{
		WorkScope: scope,
		DeviceID:  deviceID,
	})
	if err != nil {
		return domain.TenantID{}, none, answerWorkFact(err)
	}
	tenant, err := domain.NewTenantID(envelope.TenantID())
	if err != nil {
		return domain.TenantID{}, none, fmt.Errorf("node operations accessidentity: tenant: %w", err)
	}
	node, err := domain.NewNodeReference(envelope.WorkScope().Reference())
	if err != nil {
		return domain.TenantID{}, none, fmt.Errorf("%w: %v", nodeopshttp.ErrMalformedRequest, err)
	}
	return tenant, node, nil
}

func answerWorkFact(err error) error {
	switch {
	case errors.Is(err, identity.ErrAccessChannelNotConfigured):
		return fmt.Errorf("%w: %w", nodeopshttp.ErrAccessChannelNotConfigured, err)
	case errors.Is(err, identity.ErrCredentialRejected):
		return fmt.Errorf("%w: %w", nodeopshttp.ErrOperatorCredentialRejected, err)
	case errors.Is(err, identity.ErrOperatorNotGranted):
		return fmt.Errorf("%w: %w", nodeopshttp.ErrOperatorNotGranted, err)
	case errors.Is(err, identity.ErrDeviceNotInUse):
		return fmt.Errorf("%w: %w", nodeopshttp.ErrDeviceNotInUse, err)
	case errors.Is(err, identity.ErrCredentialVerifierUnavailable),
		errors.Is(err, identity.ErrOperatorRegistryUnavailable),
		errors.Is(err, identity.ErrDeviceRegistryUnavailable),
		errors.Is(err, identity.ErrAdmissionScopeUnavailable):
		return fmt.Errorf("%w: %w", nodeopshttp.ErrIdentityDependencyUnavailable, err)
	default:
		return err
	}
}
