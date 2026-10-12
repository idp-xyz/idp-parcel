// Package accessidentity 是商业对共享接入身份能力（internal/accessidentity）的消费侧适配器：铸造操作者信封，把结果
// 译成本上下文的租户、操作者主体与答复格（ADR-0072 决定一、ADR-0100、ADR-0126 决定五）。
package accessidentity

import (
	"context"
	"errors"
	"fmt"

	identity "go.idp.xyz/idp-parcel/internal/accessidentity"
	commercialhttp "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/http"
	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

// Minter 是本适配器对铸造操作者信封的最小依赖，identity.OperatorMinter 满足它。
type Minter interface {
	MintOperator(ctx context.Context, credential identity.OperatorCredential, request identity.OperatorRequest) (identity.OperatorEnvelope, error)
}

// OperatorRegistryAuthenticator 实现 commercialhttp.OperatorRegistryAuthenticator：以登记册配置写能力面铸信封。
// 不带准入要求——登记一个版本不形成生产事实，ADR-0055 决定五两项对登记写面不适用（ADR-0100 决定四）。
type OperatorRegistryAuthenticator struct {
	minter Minter
}

var _ commercialhttp.OperatorRegistryAuthenticator = (*OperatorRegistryAuthenticator)(nil)

func NewOperatorRegistryAuthenticator(minter Minter) (*OperatorRegistryAuthenticator, error) {
	if minter == nil {
		return nil, errors.New("party commercial accessidentity: operator registry authenticator needs a minter")
	}
	return &OperatorRegistryAuthenticator{minter: minter}, nil
}

func (authenticator *OperatorRegistryAuthenticator) AuthenticateRegistryWrite(ctx context.Context, bearerToken string) (commercialhttp.OperatorIdentity, error) {
	envelope, err := authenticator.minter.MintOperator(ctx, identity.NewOperatorCredential(bearerToken),
		identity.OperatorRequest{Face: identity.CapabilityRegistryConfigurationWrite})
	if err != nil {
		return commercialhttp.OperatorIdentity{}, answer(err)
	}
	tenant, err := domain.NewTenantID(envelope.TenantID())
	if err != nil {
		return commercialhttp.OperatorIdentity{}, fmt.Errorf("party commercial accessidentity: tenant: %w", err)
	}
	operator, err := operatorSubjectOf(envelope)
	if err != nil {
		return commercialhttp.OperatorIdentity{}, err
	}
	return commercialhttp.OperatorIdentity{Tenant: tenant, Operator: operator}, nil
}

// operatorSubjectOf 把信封译成本上下文的操作者主体。引用取（发行方、sub）这一对：sub 只在它的发行方之内唯一（ADR-0100
// 决定二第三条）；写法与 parcel-pricing 那一侧相同，同一个操作者在两个上下文的载体上记的是同一个串。
func operatorSubjectOf(envelope identity.OperatorEnvelope) (domain.OperatorSubject, error) {
	subject := envelope.Subject()
	reference, err := domain.NewOperatorSubjectReference(subject.Issuer() + "#" + subject.Subject())
	if err != nil {
		return domain.OperatorSubject{}, fmt.Errorf("party commercial accessidentity: operator subject: %w", err)
	}
	var levels []domain.AuthorityLevel
	for _, face := range grantableFaces {
		if !envelope.Holds(face) {
			continue
		}
		level, err := domain.NewAuthorityLevel(face.String())
		if err != nil {
			return domain.OperatorSubject{}, fmt.Errorf("party commercial accessidentity: grant %s: %w", face, err)
		}
		levels = append(levels, level)
	}
	return domain.NewOperatorSubject(reference, levels)
}

// grantableFaces 是译进授予集的能力面。信封只答「持不持某一格」、不交名单，所以逐格问；运营决定那一格按决定种类另答、
// 不进 Holds，不在这里。接入身份能力的授权模型加了能力面，这里要跟着加一格，否则批准门看不见它。
//
// 拿能力面当商业权限等级是暂定译法，与 parcel-pricing 批准门那一侧同一个（票 price-card-import/04）：能力面说的是操作者
// 能在哪一族端点上做什么，等级说的是谁有权批。租户自定的等级名经这里永远对不上，批准门恒答批准者不合格；而批准口本身以
// 登记册配置写铸信封，规则若要求的恰是这一格，到得了批准门的人都持有它，等于不要求。角色模型里有哪些授予格归
// product-strategy-boundary/07。
var grantableFaces = []identity.CapabilityFace{
	identity.CapabilityRegistryConfigurationWrite,
	identity.CapabilityMasterDataAndOperationsRead,
}

// answer 把共享接入身份能力的格译成 commercialhttp 的格：处理器只认本上下文的哨兵。
func answer(err error) error {
	switch {
	case errors.Is(err, identity.ErrAccessChannelNotConfigured):
		return fmt.Errorf("%w: %w", commercialhttp.ErrAccessChannelNotConfigured, err)
	case errors.Is(err, identity.ErrCredentialRejected):
		return fmt.Errorf("%w: %w", commercialhttp.ErrOperatorCredentialRejected, err)
	case errors.Is(err, identity.ErrOperatorNotGranted):
		return fmt.Errorf("%w: %w", commercialhttp.ErrOperatorNotGranted, err)
	case errors.Is(err, identity.ErrCredentialVerifierUnavailable),
		errors.Is(err, identity.ErrOperatorRegistryUnavailable):
		return fmt.Errorf("%w: %w", commercialhttp.ErrIdentityDependencyUnavailable, err)
	}
	return err
}
