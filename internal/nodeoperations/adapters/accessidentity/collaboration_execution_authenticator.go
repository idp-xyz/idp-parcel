package accessidentity

import (
	"context"
	"fmt"

	identity "go.idp.xyz/idp-parcel/internal/accessidentity"
	nodeopshttp "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/http"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
)

var _ nodeopshttp.CollaborationExecutionAuthenticator = (*WorkFactAuthenticator)(nil)

// AuthenticateCollaborationExecution 以作业事实登记能力面铸协作执行事实的信封，并判这一种事实的准入范围：执行事实是不可逆现场动作
// 的记录，进的是关务处置执行核对的依据，是生产事实（ADR-0338 决定二）。节点取作业范围的引用，种类必须是 NODE；这个节点随后还要与
// 承接决定里的节点对上（编排判）。
func (authenticator *WorkFactAuthenticator) AuthenticateCollaborationExecution(
	ctx context.Context,
	bearerToken, deviceID, scopeKind, scopeReference string,
) (domain.TenantID, domain.NodeReference, error) {
	none := domain.NodeReference{}
	scope, err := identity.NewWorkScope(identity.WorkScopeKind(scopeKind), scopeReference)
	if err != nil {
		return domain.TenantID{}, none, fmt.Errorf("%w: %v", nodeopshttp.ErrMalformedRequest, err)
	}
	envelope, err := authenticator.minter.MintWorkFact(ctx, identity.NewOperatorCredential(bearerToken), identity.WorkFactRequest{
		WorkScope: scope,
		DeviceID:  deviceID,
		Admission: identity.WorkFactAdmission(identity.WorkFactNodeCollaborationExecution),
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
