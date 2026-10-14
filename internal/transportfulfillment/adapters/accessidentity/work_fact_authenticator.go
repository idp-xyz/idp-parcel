package accessidentity

import (
	"context"
	"errors"
	"fmt"

	identity "go.idp.xyz/idp-parcel/internal/accessidentity"
	tfhttp "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/http"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// WorkFactMinter 是本适配器对铸造作业事实信封的最小依赖。
type WorkFactMinter interface {
	MintWorkFact(ctx context.Context, credential identity.OperatorCredential, request identity.WorkFactRequest) (identity.WorkFactEnvelope, error)
}

// WorkFactAuthenticator 以作业事实登记能力面铸信封，不带准入要求。目录式的准入词表今天不收这一面，带上会在权威串
// 未配置时一律答不在准入范围。
type WorkFactAuthenticator struct {
	minter WorkFactMinter
}

var _ tfhttp.WorkFactAuthenticator = (*WorkFactAuthenticator)(nil)

func NewWorkFactAuthenticator(minter WorkFactMinter) (*WorkFactAuthenticator, error) {
	if minter == nil {
		return nil, errors.New("transport fulfillment accessidentity: work fact authenticator needs a minter")
	}
	return &WorkFactAuthenticator{minter: minter}, nil
}

func (authenticator *WorkFactAuthenticator) AuthenticateWorkFact(
	ctx context.Context,
	bearerToken, deviceID, scopeKind, scopeReference string,
) (domain.TenantID, error) {
	scope, err := identity.NewWorkScope(identity.WorkScopeKind(scopeKind), scopeReference)
	if err != nil {
		return domain.TenantID{}, fmt.Errorf("%w: %v", tfhttp.ErrMalformedRequest, err)
	}
	envelope, err := authenticator.minter.MintWorkFact(ctx, identity.NewOperatorCredential(bearerToken), identity.WorkFactRequest{
		WorkScope: scope,
		DeviceID:  deviceID,
	})
	if err != nil {
		return domain.TenantID{}, answerWorkFact(err)
	}
	tenant, err := domain.NewTenantID(envelope.TenantID())
	if err != nil {
		return domain.TenantID{}, fmt.Errorf("transport fulfillment accessidentity: tenant: %w", err)
	}
	return tenant, nil
}

func answerWorkFact(err error) error {
	switch {
	case errors.Is(err, identity.ErrDeviceNotInUse):
		return fmt.Errorf("%w: %w", tfhttp.ErrDeviceNotInUse, err)
	case errors.Is(err, identity.ErrDeviceRegistryUnavailable):
		return fmt.Errorf("%w: %w", tfhttp.ErrIdentityDependencyUnavailable, err)
	default:
		return answer(err)
	}
}
