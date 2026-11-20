package registrationjson

import (
	"io"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/application"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
)

// unclaimedDisposalRuleDocument 是一笔待认领处置规则修订的线格式（登记册配置写面）。
// 租户与登记责任方取自信封，载荷里再带即坏载荷；认领期限不为正、大到乘进 time.Duration 会回绕，以及方式为空/重复，留给应用层答 NOT_ACCEPTED/CONTENT_INVALID。
type unclaimedDisposalRuleDocument struct {
	carriedIdentity
	WarehouseCode      *string   `json:"warehouseCode"`
	Revision           *int      `json:"revision"`
	ClaimWindowSeconds *int64    `json:"claimWindowSeconds"`
	DisposalMethods    *[]string `json:"disposalMethods"`
}

// UnclaimedDisposalRuleCommand 把一笔待认领处置规则修订的 JSON 译成登记命令。
func UnclaimedDisposalRuleCommand(
	reader io.Reader,
	tenant domain.TenantID,
	registrant domain.Registrant,
) (application.RegisterDisposalRuleCommand, error) {
	var none application.RegisterDisposalRuleCommand
	document, err := decodeDocument[unclaimedDisposalRuleDocument](reader, "unclaimed disposal rule revision")
	if err != nil {
		return none, err
	}
	if err := document.refuseCarriedIdentity(); err != nil {
		return none, err
	}
	warehouse, err := requiredText("warehouseCode", document.WarehouseCode, domain.NewWarehouseCode)
	if err != nil {
		return none, err
	}
	if document.Revision == nil {
		return none, malformed("revision is required")
	}
	if document.ClaimWindowSeconds == nil {
		return none, malformed("claimWindowSeconds is required")
	}
	if document.DisposalMethods == nil {
		return none, malformed("disposalMethods is required")
	}
	methods := make([]domain.UnclaimedDisposalMethod, 0, len(*document.DisposalMethods))
	for _, raw := range *document.DisposalMethods {
		method, methodErr := domain.NewUnclaimedDisposalMethod(raw)
		if methodErr != nil {
			// 空白方式在应用层标成 disposalMethods 内容不合，而不是 400：保持与空数组同一格。
			methods = append(methods, domain.UnclaimedDisposalMethod{})
			continue
		}
		methods = append(methods, method)
	}
	return application.RegisterDisposalRuleCommand{
		Tenant:     tenant,
		Warehouse:  warehouse,
		Revision:   *document.Revision,
		Content:    domain.NewDisposalRuleContent(*document.ClaimWindowSeconds, methods),
		Registrant: registrant,
	}, nil
}
