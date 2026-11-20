package partycommercial

import (
	"context"
	"errors"
	"fmt"
	"time"

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

// Allow 报告这次代办是否获准。false 且 err 为空是未配置。
func (authorizer *WarehouseAgencyAuthorizer) Allow(ctx context.Context, tenantID, ownerID string) (bool, error) {
	row, found, err := authorizer.keys.FindResolutionKey(ctx, tenantID, ownerID)
	if err != nil {
		return false, fmt.Errorf("warehouse agency: %w", err)
	}
	if !found {
		return false, nil
	}
	tenant, err := pcdomain.NewTenantID(tenantID)
	if err != nil {
		return false, err
	}
	account, err := pcdomain.NewCustomerAccountID(ownerID)
	if err != nil {
		return false, err
	}
	requester, err := pcdomain.RequestedByCustomerAccount(account)
	if err != nil {
		return false, err
	}
	legalEntity, err := pcdomain.NewLegalEntityReference(row.LegalEntity)
	if err != nil {
		return false, err
	}
	scope, err := pcdomain.NewCommercialScopeReference(row.Scope)
	if err != nil {
		return false, err
	}
	level, err := pcdomain.NewAuthorityLevel(WarehouseFulfillmentAgentLevel)
	if err != nil {
		return false, err
	}
	reason, err := pcdomain.NewStructuredReason("WAREHOUSE_OUTBOUND_SUBMISSION")
	if err != nil {
		return false, err
	}
	evidence, err := pcdomain.NewEvidenceReference("warehouse-outbound:" + tenantID + "/" + ownerID)
	if err != nil {
		return false, err
	}
	request, err := pcdomain.NewAuthorizationRequestBy(
		requester, pcdomain.WarehouseFulfillmentAgencyAction, legalEntity, level, scope, reason, evidence, authorizer.clock.Now().UTC(),
	)
	if err != nil {
		return false, err
	}
	if _, err := authorizer.adjudicator.Handle(ctx, tenant, request); err != nil {
		if errors.Is(err, pcdomain.ErrAuthorityRulesNotConfigured) || errors.Is(err, pcdomain.ErrNotAuthorized) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}
