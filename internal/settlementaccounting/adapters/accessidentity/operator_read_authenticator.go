package accessidentity

import (
	"context"
	"errors"
	"fmt"

	identity "go.idp.xyz/idp-parcel/internal/accessidentity"
	settlementhttp "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/http"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

// OperatorMinter 是查阅读认证方对铸造操作者信封的最小依赖，identity.OperatorMinter 满足它。
type OperatorMinter interface {
	MintOperator(ctx context.Context, credential identity.OperatorCredential, request identity.OperatorRequest) (identity.OperatorEnvelope, error)
}

// OperatorOperationsReadAuthenticator 实现 settlementhttp.OperatorOperationsReadAuthenticator：以主数据与运营查阅读能力面铸
// 信封，不带准入要求——查阅不形成生产事实（ADR-0100 决定四）。
type OperatorOperationsReadAuthenticator struct {
	minter OperatorMinter
}

var _ settlementhttp.OperatorOperationsReadAuthenticator = (*OperatorOperationsReadAuthenticator)(nil)

func NewOperatorOperationsReadAuthenticator(minter OperatorMinter) (*OperatorOperationsReadAuthenticator, error) {
	if minter == nil {
		return nil, errors.New("settlement accounting accessidentity: operator operations read authenticator needs a minter")
	}
	return &OperatorOperationsReadAuthenticator{minter: minter}, nil
}

func (authenticator *OperatorOperationsReadAuthenticator) AuthenticateOperationsRead(ctx context.Context, bearerToken string) (domain.TenantID, error) {
	envelope, err := authenticator.minter.MintOperator(ctx, identity.NewOperatorCredential(bearerToken),
		identity.OperatorRequest{Face: identity.CapabilityMasterDataAndOperationsRead})
	if err != nil {
		return domain.TenantID{}, answerOperationsRead(err)
	}
	tenant, err := domain.NewTenantID(envelope.TenantID())
	if err != nil {
		return domain.TenantID{}, fmt.Errorf("settlement accounting accessidentity: tenant: %w", err)
	}
	return tenant, nil
}

// answerOperationsRead 把操作者铸造的格译成 settlementhttp 的格：处理器只认本上下文的哨兵。
func answerOperationsRead(err error) error {
	switch {
	case errors.Is(err, identity.ErrAccessChannelNotConfigured):
		return fmt.Errorf("%w: %w", settlementhttp.ErrAccessChannelNotConfigured, err)
	case errors.Is(err, identity.ErrCredentialRejected):
		return fmt.Errorf("%w: %w", settlementhttp.ErrOperatorCredentialRejected, err)
	case errors.Is(err, identity.ErrOperatorNotGranted):
		return fmt.Errorf("%w: %w", settlementhttp.ErrOperatorNotGranted, err)
	case errors.Is(err, identity.ErrCredentialVerifierUnavailable),
		errors.Is(err, identity.ErrOperatorRegistryUnavailable):
		return fmt.Errorf("%w: %w", settlementhttp.ErrIdentityDependencyUnavailable, err)
	}
	return err
}
