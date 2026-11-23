package partycommercial

import (
	"context"
	"errors"
	"fmt"
	"time"

	psports "go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	pcapplication "go.idp.xyz/idp-parcel/internal/partycommercial/application"
	pcdomain "go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

// WarehouseFulfillmentAgentLevel 是仓配代办在授权判断里用的商业权限等级（ADR-0371），产品常量、只在这一处定义。
// 租户是否放行，由授权册里有没有登一条 WAREHOUSE_FULFILLMENT_AGENCY × 本等级的授予来决定。没登答未配置。产品不给默认授予。
const WarehouseFulfillmentAgentLevel = "WAREHOUSE_FULFILLMENT_AGENT"

// WarehouseAgencyAuthorizer 问这个货主有没有获准由仓配履约代为提交来源为仓配出库的委托。
// 解析键没登记，或这个范围没有现行的仓配代办授予，都是未配置。适用冲突照上抛，不折成未配置。
type WarehouseAgencyAuthorizer struct {
	keys        ResolutionKeyStore
	adjudicator *pcapplication.AdjudicateCommercialAuthorizationHandler
	clock       interface{ Now() time.Time }
}

func NewWarehouseAgencyAuthorizer(
	keys ResolutionKeyStore,
	adjudicator *pcapplication.AdjudicateCommercialAuthorizationHandler,
	clock interface{ Now() time.Time },
) (*WarehouseAgencyAuthorizer, error) {
	if keys == nil || adjudicator == nil || clock == nil {
		return nil, fmt.Errorf("parcel shipment party commercial: warehouse agency authorizer needs keys, an adjudicator and a clock")
	}
	return &WarehouseAgencyAuthorizer{keys: keys, adjudicator: adjudicator, clock: clock}, nil
}

// 提交与撤回问的是同一个动作。原因只标明这次询问是哪一件事，不另造授权。
const (
	warehouseOutboundSubmissionReason = "WAREHOUSE_OUTBOUND_SUBMISSION"
	warehouseOutboundWithdrawalReason = "WAREHOUSE_OUTBOUND_WITHDRAWAL"
)

// Allow 报告这次代办是否获准。false 且 err 为空是未配置或未获准，提交口都答未配置。
func (authorizer *WarehouseAgencyAuthorizer) Allow(ctx context.Context, tenantID, ownerID string) (bool, error) {
	_, outcome, err := authorizer.Reference(ctx, tenantID, ownerID, warehouseOutboundSubmissionReason, "warehouse-outbound:"+tenantID+"/"+ownerID)
	if err != nil {
		return false, err
	}
	return outcome == psports.AuthorizationGranted, nil
}

// Reference 问仓配代办，并在获准时带回所采用授予版本。解析键没登记与规则没登记都是未配置；适用冲突原样上抛。
func (authorizer *WarehouseAgencyAuthorizer) Reference(
	ctx context.Context, tenantID, ownerID, reasonText, evidenceText string,
) (string, psports.AuthorizationOutcome, error) {
	row, found, err := authorizer.keys.FindResolutionKey(ctx, tenantID, ownerID)
	if err != nil {
		return "", 0, fmt.Errorf("warehouse agency: %w", err)
	}
	if !found {
		return "", psports.AuthorizationRulesNotConfigured, nil
	}
	tenant, err := pcdomain.NewTenantID(tenantID)
	if err != nil {
		return "", 0, err
	}
	account, err := pcdomain.NewCustomerAccountID(ownerID)
	if err != nil {
		return "", 0, err
	}
	requester, err := pcdomain.RequestedByCustomerAccount(account)
	if err != nil {
		return "", 0, err
	}
	legalEntity, err := pcdomain.NewLegalEntityReference(row.LegalEntity)
	if err != nil {
		return "", 0, err
	}
	scope, err := pcdomain.NewCommercialScopeReference(row.Scope)
	if err != nil {
		return "", 0, err
	}
	level, err := pcdomain.NewAuthorityLevel(WarehouseFulfillmentAgentLevel)
	if err != nil {
		return "", 0, err
	}
	reason, err := pcdomain.NewStructuredReason(reasonText)
	if err != nil {
		return "", 0, err
	}
	evidence, err := pcdomain.NewEvidenceReference(evidenceText)
	if err != nil {
		return "", 0, err
	}
	request, err := pcdomain.NewAuthorizationRequestBy(
		requester, pcdomain.WarehouseFulfillmentAgencyAction, legalEntity, level, scope, reason, evidence, authorizer.clock.Now().UTC(),
	)
	if err != nil {
		return "", 0, err
	}
	authorized, err := authorizer.adjudicator.Handle(ctx, tenant, request)
	if err != nil {
		switch {
		case errors.Is(err, pcdomain.ErrAuthorityRulesNotConfigured):
			return "", psports.AuthorizationRulesNotConfigured, nil
		case errors.Is(err, pcdomain.ErrNotAuthorized):
			return "", psports.AuthorizationRefused, nil
		default:
			return "", 0, err
		}
	}
	return authorized.GrantVersion().ObjectID().String() + "/" + authorized.GrantVersion().Version().String(), psports.AuthorizationGranted, nil
}
