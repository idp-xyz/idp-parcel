// Package accessidentity 是可见性与异常对共享接入身份能力（internal/accessidentity）的消费侧适配器：铸造操作者信封，
// 把结果译成本上下文的租户与答复格（ADR-0072 决定一、ADR-0100）。
package accessidentity

import (
	"context"
	"errors"
	"fmt"

	identity "go.idp.xyz/idp-parcel/internal/accessidentity"
	visibilityhttp "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/http"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
)

// Minter 是本适配器对铸造操作者信封的最小依赖，identity.OperatorMinter 满足它。
type Minter interface {
	MintOperator(ctx context.Context, credential identity.OperatorCredential, request identity.OperatorRequest) (identity.OperatorEnvelope, error)
}

// OperatorRegistryAuthenticator 实现 visibilityhttp.OperatorRegistryAuthenticator：以登记册配置写能力面铸信封。
// 不带准入要求——登记一个版本不形成生产事实，ADR-0055 决定五两项对登记写面不适用（ADR-0100 决定四）。
type OperatorRegistryAuthenticator struct {
	minter Minter
}

var _ visibilityhttp.OperatorRegistryAuthenticator = (*OperatorRegistryAuthenticator)(nil)

func NewOperatorRegistryAuthenticator(minter Minter) (*OperatorRegistryAuthenticator, error) {
	if minter == nil {
		return nil, errors.New("visibility accessidentity: operator registry authenticator needs a minter")
	}
	return &OperatorRegistryAuthenticator{minter: minter}, nil
}

func (authenticator *OperatorRegistryAuthenticator) AuthenticateRegistryWrite(ctx context.Context, bearerToken string) (domain.TenantID, error) {
	envelope, err := authenticator.minter.MintOperator(ctx, identity.NewOperatorCredential(bearerToken),
		identity.OperatorRequest{Face: identity.CapabilityRegistryConfigurationWrite})
	if err != nil {
		return domain.TenantID{}, answer(err)
	}
	return domain.NewTenantID(envelope.TenantID())
}

// answer 把共享接入身份能力的格译成 visibilityhttp 的格：处理器只认本上下文的哨兵。
func answer(err error) error {
	switch {
	case errors.Is(err, identity.ErrAccessChannelNotConfigured):
		return fmt.Errorf("%w: %w", visibilityhttp.ErrAccessChannelNotConfigured, err)
	case errors.Is(err, identity.ErrCredentialRejected):
		return fmt.Errorf("%w: %w", visibilityhttp.ErrOperatorCredentialRejected, err)
	case errors.Is(err, identity.ErrOperatorNotGranted):
		return fmt.Errorf("%w: %w", visibilityhttp.ErrOperatorNotGranted, err)
	case errors.Is(err, identity.ErrCredentialVerifierUnavailable),
		errors.Is(err, identity.ErrOperatorRegistryUnavailable):
		return fmt.Errorf("%w: %w", visibilityhttp.ErrIdentityDependencyUnavailable, err)
	}
	return err
}
