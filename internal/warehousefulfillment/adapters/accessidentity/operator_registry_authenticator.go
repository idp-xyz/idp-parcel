// Package accessidentity 是仓配履约对共享接入身份能力（internal/accessidentity）的消费侧适配器：铸造操作者信封，把结果译成
// 本上下文的租户、登记责任方与答复格（ADR-0072 决定一、ADR-0100 决定四）。
package accessidentity

import (
	"context"
	"errors"
	"fmt"

	identity "go.idp.xyz/idp-parcel/internal/accessidentity"
	warehousehttp "go.idp.xyz/idp-parcel/internal/warehousefulfillment/adapters/http"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
)

// Minter 是本适配器对铸造操作者信封的最小依赖，identity.OperatorMinter 满足它。
type Minter interface {
	MintOperator(ctx context.Context, credential identity.OperatorCredential, request identity.OperatorRequest) (identity.OperatorEnvelope, error)
}

// OperatorRegistryAuthenticator 实现 warehousehttp.OperatorRegistryAuthenticator：以登记册配置写能力面铸信封，交回租户与登记
// 责任方。不带准入要求——登记一个修订不形成生产事实（ADR-0100 决定四；ADR-0384 决定五：登记仓不另开能力面）。
type OperatorRegistryAuthenticator struct {
	minter Minter
}

var _ warehousehttp.OperatorRegistryAuthenticator = (*OperatorRegistryAuthenticator)(nil)

func NewOperatorRegistryAuthenticator(minter Minter) (*OperatorRegistryAuthenticator, error) {
	if minter == nil {
		return nil, errors.New("warehouse fulfillment accessidentity: operator registry authenticator needs a minter")
	}
	return &OperatorRegistryAuthenticator{minter: minter}, nil
}

func (authenticator *OperatorRegistryAuthenticator) AuthenticateRegistryWrite(
	ctx context.Context,
	bearerToken string,
) (warehousehttp.OperatorIdentity, error) {
	envelope, err := authenticator.minter.MintOperator(ctx, identity.NewOperatorCredential(bearerToken),
		identity.OperatorRequest{Face: identity.CapabilityRegistryConfigurationWrite})
	if err != nil {
		return warehousehttp.OperatorIdentity{}, answer(err)
	}
	tenant, err := domain.NewTenantID(envelope.TenantID())
	if err != nil {
		return warehousehttp.OperatorIdentity{}, fmt.Errorf("warehouse fulfillment accessidentity: tenant: %w", err)
	}
	// 登记责任方取（发行方，sub）这一对：sub 只在它的发行方之内唯一（ADR-0100 决定二），写法与别的上下文的操作者引用相同，
	// 同一个操作者在各册上记的是同一个串。
	subject := envelope.Subject()
	registrant, err := domain.NewRegistrant(subject.Issuer() + "#" + subject.Subject())
	if err != nil {
		return warehousehttp.OperatorIdentity{}, fmt.Errorf("warehouse fulfillment accessidentity: registrant: %w", err)
	}
	return warehousehttp.OperatorIdentity{Tenant: tenant, Registrant: registrant}, nil
}

// answer 把共享接入身份能力的格译成 warehousehttp 的格：处理器只认本上下文的哨兵。登记册配置写与主数据与运营查阅读都不判准入
// 范围，不核设备，那两格在这两条路上不会出现，两个面共用这一份译法。
func answer(err error) error {
	switch {
	case errors.Is(err, identity.ErrAccessChannelNotConfigured):
		return fmt.Errorf("%w: %w", warehousehttp.ErrAccessChannelNotConfigured, err)
	case errors.Is(err, identity.ErrCredentialRejected):
		return fmt.Errorf("%w: %w", warehousehttp.ErrOperatorCredentialRejected, err)
	case errors.Is(err, identity.ErrOperatorNotGranted):
		return fmt.Errorf("%w: %w", warehousehttp.ErrOperatorNotGranted, err)
	case errors.Is(err, identity.ErrCredentialVerifierUnavailable),
		errors.Is(err, identity.ErrOperatorRegistryUnavailable):
		return fmt.Errorf("%w: %w", warehousehttp.ErrIdentityDependencyUnavailable, err)
	}
	return err
}
