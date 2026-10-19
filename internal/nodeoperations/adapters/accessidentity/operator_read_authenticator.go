package accessidentity

import (
	"context"
	"errors"
	"fmt"

	identity "go.idp.xyz/idp-parcel/internal/accessidentity"
	nodeopshttp "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/http"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
)

// Minter 是查阅读认证方对铸造操作者信封的最小依赖，identity.OperatorMinter 满足它。
type Minter interface {
	MintOperator(ctx context.Context, credential identity.OperatorCredential, request identity.OperatorRequest) (identity.OperatorEnvelope, error)
}

// OperatorOperationsReadAuthenticator 实现 nodeopshttp.OperatorOperationsReadAuthenticator：以主数据与运营查阅读能力面铸
// 信封，不带准入要求——查阅不形成作业事实（ADR-0100 决定四）。
type OperatorOperationsReadAuthenticator struct {
	minter Minter
}

var _ nodeopshttp.OperatorOperationsReadAuthenticator = (*OperatorOperationsReadAuthenticator)(nil)

func NewOperatorOperationsReadAuthenticator(minter Minter) (*OperatorOperationsReadAuthenticator, error) {
	if minter == nil {
		return nil, errors.New("node operations accessidentity: operator operations read authenticator needs a minter")
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
		return domain.TenantID{}, fmt.Errorf("node operations accessidentity: tenant: %w", err)
	}
	return tenant, nil
}

// answerOperationsRead 把操作者铸造的格译成 nodeopshttp 的格。不与 answerWorkFact 合一：查阅读既不核设备，也不判准入范围，
// 那两格在这条路上不会出现。
func answerOperationsRead(err error) error {
	switch {
	case errors.Is(err, identity.ErrAccessChannelNotConfigured):
		return fmt.Errorf("%w: %w", nodeopshttp.ErrAccessChannelNotConfigured, err)
	case errors.Is(err, identity.ErrCredentialRejected):
		return fmt.Errorf("%w: %w", nodeopshttp.ErrOperatorCredentialRejected, err)
	case errors.Is(err, identity.ErrOperatorNotGranted):
		return fmt.Errorf("%w: %w", nodeopshttp.ErrOperatorNotGranted, err)
	case errors.Is(err, identity.ErrCredentialVerifierUnavailable),
		errors.Is(err, identity.ErrOperatorRegistryUnavailable):
		return fmt.Errorf("%w: %w", nodeopshttp.ErrIdentityDependencyUnavailable, err)
	}
	return err
}
