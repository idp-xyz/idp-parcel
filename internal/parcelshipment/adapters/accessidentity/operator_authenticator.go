// Package accessidentity 是 parcel-shipment 对共享接入身份能力（internal/accessidentity）的消费侧适配器：
// 铸造操作者信封，把结果译成本上下文的租户、提交操作者与答复格（ADR-0072 决定一、ADR-0151）。
package accessidentity

import (
	"context"
	"errors"
	"fmt"

	identity "go.idp.xyz/idp-parcel/internal/accessidentity"
	shipmenthttp "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/http"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

// operationDecisionCapability 是运营决定口在治理登记册上的能力格；事实类型取各口的决定种类（票 operator-channel/15）。
const operationDecisionCapability = "OPERATION_DECISION"

// Minter 是本适配器对铸造操作者信封的最小依赖，identity.OperatorMinter 满足它。
type Minter interface {
	MintOperator(ctx context.Context, credential identity.OperatorCredential, request identity.OperatorRequest) (identity.OperatorEnvelope, error)
}

// OperatorAuthenticator 实现 shipmenthttp.OperatorAuthenticator：按本口的决定种类铸信封，并判准入范围
// （ADR-0151 决定三：运营决定形成生产事实，照 ADR-0149 决定四判）。
type OperatorAuthenticator struct {
	minter Minter
}

var _ shipmenthttp.OperatorAuthenticator = (*OperatorAuthenticator)(nil)

func NewOperatorAuthenticator(minter Minter) (*OperatorAuthenticator, error) {
	if minter == nil {
		return nil, errors.New("parcel shipment accessidentity: operator authenticator needs a minter")
	}
	return &OperatorAuthenticator{minter: minter}, nil
}

func (authenticator *OperatorAuthenticator) AuthenticateOperatorDecision(
	ctx context.Context,
	bearerToken string,
	decision shipmenthttp.OperatorDecision,
) (shipmenthttp.OperatorIdentity, error) {
	kind, err := identity.ParseDecisionKind(string(decision))
	if err != nil {
		return shipmenthttp.OperatorIdentity{}, fmt.Errorf("parcel shipment accessidentity: decision %q: %w", decision, err)
	}
	envelope, err := authenticator.minter.MintOperator(ctx, identity.NewOperatorCredential(bearerToken), identity.OperatorRequest{
		Face:         identity.CapabilityOperationDecision,
		DecisionKind: kind,
		Admission:    &identity.AdmissionRequirement{Capability: operationDecisionCapability, FactKind: kind.String()},
	})
	if err != nil {
		return shipmenthttp.OperatorIdentity{}, answer(err)
	}
	tenant, err := domain.NewTenantID(envelope.TenantID())
	if err != nil {
		return shipmenthttp.OperatorIdentity{}, fmt.Errorf("parcel shipment accessidentity: tenant: %w", err)
	}
	// 提交操作者的引用取（发行方、sub）这一对：sub 只在它的发行方之内唯一，单拿 sub 会让两个发行方的同一个 sub
	// 在留痕里读成一个人（ADR-0100 决定二第三条）。
	subject := envelope.Subject()
	return shipmenthttp.OperatorIdentity{Tenant: tenant, Operator: subject.Issuer() + "#" + subject.Subject()}, nil
}

// answer 把共享接入身份能力的格译成 shipmenthttp 的格：处理器只认本上下文的哨兵。
func answer(err error) error {
	switch {
	case errors.Is(err, identity.ErrAccessChannelNotConfigured):
		return fmt.Errorf("%w: %w", shipmenthttp.ErrAccessChannelNotConfigured, err)
	case errors.Is(err, identity.ErrCredentialRejected):
		return fmt.Errorf("%w: %w", shipmenthttp.ErrOperatorCredentialRejected, err)
	case errors.Is(err, identity.ErrOperatorNotGranted):
		return fmt.Errorf("%w: %w", shipmenthttp.ErrOperatorNotGranted, err)
	case errors.Is(err, identity.ErrOutsideAdmissionScope):
		return fmt.Errorf("%w: %w", shipmenthttp.ErrOutsideAdmissionScope, err)
	case errors.Is(err, identity.ErrCredentialVerifierUnavailable),
		errors.Is(err, identity.ErrOperatorRegistryUnavailable),
		errors.Is(err, identity.ErrAdmissionScopeUnavailable):
		return fmt.Errorf("%w: %w", shipmenthttp.ErrIdentityDependencyUnavailable, err)
	}
	return err
}
