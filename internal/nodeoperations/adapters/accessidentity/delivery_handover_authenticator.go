package accessidentity

import (
	"context"
	"errors"
	"fmt"

	identity "go.idp.xyz/idp-parcel/internal/accessidentity"
	nodeopshttp "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/http"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
)

var _ nodeopshttp.HandoverNoteWorkFactAuthenticator = (*WorkFactAuthenticator)(nil)

// AuthenticateDeliveryHandoverFact 以作业事实登记能力面铸交接单这一种事实的信封，并判这一种事实的准入范围：开立、归入、关闭都改变
// 可验证交付范围的证据，是生产事实（ADR-0149 决定四第二条）。节点取作业范围的引用，种类必须是 NODE。词表外的事实不铸：那是接入口
// 递错了常量，答哪一格都会把人指去错的恢复动作。
func (authenticator *WorkFactAuthenticator) AuthenticateDeliveryHandoverFact(
	ctx context.Context,
	bearerToken, deviceID, scopeKind, scopeReference string,
	fact nodeopshttp.DeliveryHandoverFact,
) (domain.TenantID, domain.NodeReference, error) {
	none := domain.NodeReference{}
	kind, err := identity.ParseWorkFactKind(string(fact))
	if err != nil {
		return domain.TenantID{}, none, fmt.Errorf("node operations accessidentity: work fact %q: %w", fact, err)
	}
	scope, err := identity.NewWorkScope(identity.WorkScopeKind(scopeKind), scopeReference)
	if err != nil {
		return domain.TenantID{}, none, fmt.Errorf("%w: %v", nodeopshttp.ErrMalformedRequest, err)
	}
	envelope, err := authenticator.minter.MintWorkFact(ctx, identity.NewOperatorCredential(bearerToken), identity.WorkFactRequest{
		WorkScope: scope,
		DeviceID:  deviceID,
		Admission: identity.WorkFactAdmission(kind),
	})
	if err != nil {
		return domain.TenantID{}, none, answerAdmittedWorkFact(err)
	}
	tenant, err := domain.NewTenantID(envelope.TenantID())
	if err != nil {
		return domain.TenantID{}, none, fmt.Errorf("node operations accessidentity: tenant: %w", err)
	}
	node, err := domain.NewNodeReference(envelope.WorkScope().Reference())
	if err != nil {
		return domain.TenantID{}, none, fmt.Errorf("%w: %v", nodeopshttp.ErrMalformedRequest, err)
	}
	return tenant, node, nil
}

// answerAdmittedWorkFact 在 answerWorkFact 的各格之上多译准入范围两格：不在准入范围自成一格，准入范围读不动归身份依赖读不动。
func answerAdmittedWorkFact(err error) error {
	switch {
	case errors.Is(err, identity.ErrOutsideAdmissionScope):
		return fmt.Errorf("%w: %w", nodeopshttp.ErrOutsideAdmissionScope, err)
	case errors.Is(err, identity.ErrAdmissionScopeUnavailable):
		return fmt.Errorf("%w: %w", nodeopshttp.ErrIdentityDependencyUnavailable, err)
	}
	return answerWorkFact(err)
}

// CustomerHandoverConclusionMinter 是本适配器对铸造送站交接确认信封的最小依赖，identity.CustomerClientMinter 满足它。
type CustomerHandoverConclusionMinter interface {
	MintDeliveryHandoverConfirmation(ctx context.Context, credential identity.IntegrationClientCredential, readRequest func() (identity.ChannelRequest, error)) (identity.DeliveryHandoverConfirmationEnvelope, error)
}

// CustomerHandoverConclusionAuthenticator 实现 nodeopshttp.CustomerHandoverConclusionAuthenticator：铸首方客户族的送站交接确认信封
// （ADR-0139 决定二；ADR-0212 决定三），把四要素译成本上下文的客户身份。准入范围在铸造侧判（ADR-0139 决定八），这里只译它的格；各格
// 的译法与预报两口同一份。
type CustomerHandoverConclusionAuthenticator struct {
	minter CustomerHandoverConclusionMinter
}

var _ nodeopshttp.CustomerHandoverConclusionAuthenticator = (*CustomerHandoverConclusionAuthenticator)(nil)

func NewCustomerHandoverConclusionAuthenticator(minter CustomerHandoverConclusionMinter) (*CustomerHandoverConclusionAuthenticator, error) {
	if minter == nil {
		return nil, errors.New("node operations accessidentity: customer handover conclusion authenticator needs a minter")
	}
	return &CustomerHandoverConclusionAuthenticator{minter: minter}, nil
}

func (authenticator *CustomerHandoverConclusionAuthenticator) AuthenticateDeliveryHandoverConclusion(
	ctx context.Context,
	credential nodeopshttp.PresentedCustomerCredential,
	accessRequestID func() (string, error),
) (nodeopshttp.CustomerIdentity, error) {
	envelope, err := authenticator.minter.MintDeliveryHandoverConfirmation(ctx, customerCredential(credential), channelRequestOf(accessRequestID))
	if err != nil {
		return nodeopshttp.CustomerIdentity{}, answerCustomer(err)
	}
	return customerIdentityOf(envelope.Envelope())
}
