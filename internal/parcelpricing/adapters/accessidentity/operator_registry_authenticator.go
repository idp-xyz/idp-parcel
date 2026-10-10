// Package accessidentity 是计价对共享接入身份能力（internal/accessidentity）的消费侧适配器：铸造操作者信封，把结果译成
// 本上下文的租户、提交操作者与答复格（ADR-0072 决定一、ADR-0100）。
package accessidentity

import (
	"context"
	"errors"
	"fmt"

	identity "go.idp.xyz/idp-parcel/internal/accessidentity"
	pricinghttp "go.idp.xyz/idp-parcel/internal/parcelpricing/adapters/http"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
)

// Minter 是本适配器对铸造操作者信封的最小依赖，identity.OperatorMinter 满足它。
type Minter interface {
	MintOperator(ctx context.Context, credential identity.OperatorCredential, request identity.OperatorRequest) (identity.OperatorEnvelope, error)
}

// OperatorRegistryAuthenticator 实现 pricinghttp.OperatorRegistryAuthenticator：以登记册配置写能力面铸信封，不带准入要求
// （ADR-0100 决定四）。
type OperatorRegistryAuthenticator struct {
	minter Minter
}

var _ pricinghttp.OperatorRegistryAuthenticator = (*OperatorRegistryAuthenticator)(nil)

func NewOperatorRegistryAuthenticator(minter Minter) (*OperatorRegistryAuthenticator, error) {
	if minter == nil {
		return nil, errors.New("pricing accessidentity: operator registry authenticator needs a minter")
	}
	return &OperatorRegistryAuthenticator{minter: minter}, nil
}

func (authenticator *OperatorRegistryAuthenticator) AuthenticateRegistryWrite(ctx context.Context, bearerToken string) (pricinghttp.OperatorIdentity, error) {
	envelope, err := authenticator.minter.MintOperator(ctx, identity.NewOperatorCredential(bearerToken),
		identity.OperatorRequest{Face: identity.CapabilityRegistryConfigurationWrite})
	if err != nil {
		return pricinghttp.OperatorIdentity{}, answer(err)
	}
	tenant, err := domain.NewTenantID(envelope.TenantID())
	if err != nil {
		return pricinghttp.OperatorIdentity{}, fmt.Errorf("pricing accessidentity: tenant: %w", err)
	}
	// 登记责任方与复核人的引用取（发行方、sub）这一对：sub 只在它的发行方之内唯一（ADR-0100 决定二第三条）。
	subject := envelope.Subject()
	var grants []string
	for _, face := range grantableFaces {
		if envelope.Holds(face) {
			grants = append(grants, face.String())
		}
	}
	return pricinghttp.OperatorIdentity{Tenant: tenant, Operator: subject.Issuer() + "#" + subject.Subject(), Grants: grants}, nil
}

// grantableFaces 是译进授予集的能力面。信封只答「持不持某一格」、不交名单，所以这里逐格问；运营决定那一格按决定种类
// 另答、不进 Holds，不在这里。接入身份能力的授权模型加了能力面，这里要跟着加一格，否则批准门看不见它。
//
// 名字里的 grantable 说的是「译进本上下文的授予集」，不是接入身份能力 checkGrantable 的「可授」（能力面能不能被授出）：
// 运营决定那一格在那边可授，在这里不译。能力面的原义是操作者能在哪一族端点上做什么，拿它当审批要求的授予格是暂定的
// 译法：租户自定的等级名经这里永远对不上，批准门恒答批准者不合格；而批准口本身以登记册配置写铸信封，规则若要求的
// 恰是这一格，到得了批准门的人都持有它，等于不要求。
var grantableFaces = []identity.CapabilityFace{
	identity.CapabilityRegistryConfigurationWrite,
	identity.CapabilityMasterDataAndOperationsRead,
}

// answer 把共享接入身份能力的格译成 pricinghttp 的格：处理器只认本上下文的哨兵。
func answer(err error) error {
	switch {
	case errors.Is(err, identity.ErrAccessChannelNotConfigured):
		return fmt.Errorf("%w: %w", pricinghttp.ErrAccessChannelNotConfigured, err)
	case errors.Is(err, identity.ErrCredentialRejected):
		return fmt.Errorf("%w: %w", pricinghttp.ErrOperatorCredentialRejected, err)
	case errors.Is(err, identity.ErrOperatorNotGranted):
		return fmt.Errorf("%w: %w", pricinghttp.ErrOperatorNotGranted, err)
	case errors.Is(err, identity.ErrCredentialVerifierUnavailable),
		errors.Is(err, identity.ErrOperatorRegistryUnavailable):
		return fmt.Errorf("%w: %w", pricinghttp.ErrIdentityDependencyUnavailable, err)
	}
	return err
}
