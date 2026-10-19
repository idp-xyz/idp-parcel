// Package accessidentity 是 pilot-governance 对共享接入身份能力（internal/accessidentity）的消费侧适配器：铸造操作者
// 信封，把结果译成本上下文的答复格（ADR-0072 决定一、ADR-0100 决定四）。
package accessidentity

import (
	"context"
	"errors"
	"fmt"

	identity "go.idp.xyz/idp-parcel/internal/accessidentity"
	governancehttp "go.idp.xyz/idp-parcel/internal/pilotgovernance/adapters/http"
)

// OperatorMinter 是查阅读认证方对铸造操作者信封的最小依赖，identity.OperatorMinter 满足它。
type OperatorMinter interface {
	MintOperator(ctx context.Context, credential identity.OperatorCredential, request identity.OperatorRequest) (identity.OperatorEnvelope, error)
}

// OperatorOperationsReadAuthenticator 实现 governancehttp.OperatorOperationsReadAuthenticator：以主数据与运营查阅读能力面铸
// 信封，不带准入要求——查阅不形成生产事实（ADR-0100 决定四）。
type OperatorOperationsReadAuthenticator struct {
	minter OperatorMinter
}

var _ governancehttp.OperatorOperationsReadAuthenticator = (*OperatorOperationsReadAuthenticator)(nil)

func NewOperatorOperationsReadAuthenticator(minter OperatorMinter) (*OperatorOperationsReadAuthenticator, error) {
	if minter == nil {
		return nil, errors.New("pilot governance accessidentity: operator operations read authenticator needs a minter")
	}
	return &OperatorOperationsReadAuthenticator{minter: minter}, nil
}

// AuthenticateOperationsRead 只认证，不交回租户：治理登记册是产品实例级的、没有租户维（ADR-0083 决定二、三），信封上
// 的租户只说明「这是谁家的操作者」，进作用域就是给一张没有租户列的表造一个过滤维。
func (authenticator *OperatorOperationsReadAuthenticator) AuthenticateOperationsRead(ctx context.Context, bearerToken string) error {
	if _, err := authenticator.minter.MintOperator(ctx, identity.NewOperatorCredential(bearerToken),
		identity.OperatorRequest{Face: identity.CapabilityMasterDataAndOperationsRead}); err != nil {
		return answerOperationsRead(err)
	}
	return nil
}

// answerOperationsRead 把操作者铸造的格译成 governancehttp 的格：处理器只认本上下文的哨兵。
func answerOperationsRead(err error) error {
	switch {
	case errors.Is(err, identity.ErrAccessChannelNotConfigured):
		return fmt.Errorf("%w: %w", governancehttp.ErrAccessChannelNotConfigured, err)
	case errors.Is(err, identity.ErrCredentialRejected):
		return fmt.Errorf("%w: %w", governancehttp.ErrOperatorCredentialRejected, err)
	case errors.Is(err, identity.ErrOperatorNotGranted):
		return fmt.Errorf("%w: %w", governancehttp.ErrOperatorNotGranted, err)
	case errors.Is(err, identity.ErrCredentialVerifierUnavailable),
		errors.Is(err, identity.ErrOperatorRegistryUnavailable):
		return fmt.Errorf("%w: %w", governancehttp.ErrIdentityDependencyUnavailable, err)
	}
	return err
}
