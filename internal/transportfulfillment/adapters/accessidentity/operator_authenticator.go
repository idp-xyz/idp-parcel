// Package accessidentity 是运输履约对共享接入身份能力（internal/accessidentity）的消费侧适配器：
// 铸造操作者信封，把结果译成本上下文的租户与答复格（ADR-0072 决定一、ADR-0151）。
package accessidentity

import (
	"context"
	"errors"
	"fmt"

	identity "go.idp.xyz/idp-parcel/internal/accessidentity"
	tfhttp "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/http"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// operationDecisionCapability 是运营决定口在治理登记册上的能力格；事实类型取各口的决定种类（票 operator-channel/15）。
const operationDecisionCapability = "OPERATION_DECISION"

// Minter 是本适配器对铸造操作者信封的最小依赖，identity.OperatorMinter 满足它。
type Minter interface {
	MintOperator(ctx context.Context, credential identity.OperatorCredential, request identity.OperatorRequest) (identity.OperatorEnvelope, error)
}

// OperatorAuthenticator 实现 tfhttp.OperatorAuthenticator：按本口的决定种类铸信封，并判准入范围（ADR-0151 决定三：
// 运营决定形成生产事实，照 ADR-0149 决定四判）。
type OperatorAuthenticator struct {
	minter Minter
}

var _ tfhttp.OperatorAuthenticator = (*OperatorAuthenticator)(nil)

func NewOperatorAuthenticator(minter Minter) (*OperatorAuthenticator, error) {
	if minter == nil {
		return nil, errors.New("transport fulfillment accessidentity: operator authenticator needs a minter")
	}
	return &OperatorAuthenticator{minter: minter}, nil
}

func (authenticator *OperatorAuthenticator) AuthenticateOperatorDecision(
	ctx context.Context,
	bearerToken string,
	decision tfhttp.OperatorDecision,
) (domain.TenantID, error) {
	kind, err := identity.ParseDecisionKind(string(decision))
	if err != nil {
		return domain.TenantID{}, fmt.Errorf("transport fulfillment accessidentity: decision %q: %w", decision, err)
	}
	envelope, err := authenticator.minter.MintOperator(ctx, identity.NewOperatorCredential(bearerToken), identity.OperatorRequest{
		Face:         identity.CapabilityOperationDecision,
		DecisionKind: kind,
		Admission:    &identity.AdmissionRequirement{Capability: operationDecisionCapability, FactKind: kind.String()},
	})
	if err != nil {
		return domain.TenantID{}, answer(err)
	}
	return domain.NewTenantID(envelope.TenantID())
}

// answer 把共享接入身份能力的格译成 tfhttp 的格：处理器只认本上下文的哨兵。
func answer(err error) error {
	switch {
	case errors.Is(err, identity.ErrAccessChannelNotConfigured):
		return fmt.Errorf("%w: %w", tfhttp.ErrAccessChannelNotConfigured, err)
	case errors.Is(err, identity.ErrCredentialRejected):
		return fmt.Errorf("%w: %w", tfhttp.ErrOperatorCredentialRejected, err)
	case errors.Is(err, identity.ErrOperatorNotGranted):
		return fmt.Errorf("%w: %w", tfhttp.ErrOperatorNotGranted, err)
	case errors.Is(err, identity.ErrOutsideAdmissionScope):
		return fmt.Errorf("%w: %w", tfhttp.ErrOutsideAdmissionScope, err)
	case errors.Is(err, identity.ErrCredentialVerifierUnavailable),
		errors.Is(err, identity.ErrOperatorRegistryUnavailable),
		errors.Is(err, identity.ErrAdmissionScopeUnavailable):
		return fmt.Errorf("%w: %w", tfhttp.ErrIdentityDependencyUnavailable, err)
	}
	return err
}
