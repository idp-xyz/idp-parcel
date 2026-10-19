// Package accessidentity 是 collection-remittance 对共享接入身份能力（internal/accessidentity）的消费侧适配器：铸造操作者
// 信封，把结果译成本上下文的租户与答复格（ADR-0072 决定一、ADR-0100 决定四）。
package accessidentity

import (
	"context"
	"errors"
	"fmt"

	identity "go.idp.xyz/idp-parcel/internal/accessidentity"
	collectionhttp "go.idp.xyz/idp-parcel/internal/collectionremittance/adapters/http"
	"go.idp.xyz/idp-parcel/internal/collectionremittance/domain"
)

// Minter 是查阅读认证方对铸造操作者信封的最小依赖，identity.OperatorMinter 满足它。
type Minter interface {
	MintOperator(ctx context.Context, credential identity.OperatorCredential, request identity.OperatorRequest) (identity.OperatorEnvelope, error)
}

// OperatorOperationsReadAuthenticator 实现 collectionhttp.OperatorOperationsReadAuthenticator：以主数据与运营查阅读能力面铸
// 信封，不带准入要求——查阅不形成生产事实（ADR-0100 决定四）。
type OperatorOperationsReadAuthenticator struct {
	minter Minter
}

var _ collectionhttp.OperatorOperationsReadAuthenticator = (*OperatorOperationsReadAuthenticator)(nil)

func NewOperatorOperationsReadAuthenticator(minter Minter) (*OperatorOperationsReadAuthenticator, error) {
	if minter == nil {
		return nil, errors.New("collection remittance accessidentity: operator operations read authenticator needs a minter")
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
		return domain.TenantID{}, fmt.Errorf("collection remittance accessidentity: tenant: %w", err)
	}
	return tenant, nil
}

// answerOperationsRead 把操作者铸造的格译成 collectionhttp 的格：处理器只认本上下文的哨兵。
func answerOperationsRead(err error) error {
	switch {
	case errors.Is(err, identity.ErrAccessChannelNotConfigured):
		return fmt.Errorf("%w: %w", collectionhttp.ErrAccessChannelNotConfigured, err)
	case errors.Is(err, identity.ErrCredentialRejected):
		return fmt.Errorf("%w: %w", collectionhttp.ErrOperatorCredentialRejected, err)
	case errors.Is(err, identity.ErrOperatorNotGranted):
		return fmt.Errorf("%w: %w", collectionhttp.ErrOperatorNotGranted, err)
	case errors.Is(err, identity.ErrCredentialVerifierUnavailable),
		errors.Is(err, identity.ErrOperatorRegistryUnavailable):
		return fmt.Errorf("%w: %w", collectionhttp.ErrIdentityDependencyUnavailable, err)
	}
	return err
}
