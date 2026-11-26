package registrationjson

import (
	"io"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/application"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
)

type toteRegistrationDocument struct {
	carriedIdentity
	Warehouse *string   `json:"warehouse"`
	ToteCodes *[]string `json:"toteCodes"`
}

// RegisterTotesCommand 把周转箱登记译成命令。租户与操作者取自身份，不从载荷来。
func RegisterTotesCommand(reader io.Reader, tenant domain.TenantID, operator domain.Registrant) (application.RegisterTotesCommand, error) {
	var none application.RegisterTotesCommand
	document, err := decodeDocument[toteRegistrationDocument](reader, "tote registration")
	if err != nil {
		return none, err
	}
	if err := document.refuseCarriedIdentity(); err != nil {
		return none, err
	}
	warehouse, err := requiredText("warehouse", document.Warehouse, domain.NewWarehouseCode)
	if err != nil {
		return none, err
	}
	if document.ToteCodes == nil || len(*document.ToteCodes) == 0 {
		return none, malformed("toteCodes is required")
	}
	codes := make([]domain.ToteCode, 0, len(*document.ToteCodes))
	for _, raw := range *document.ToteCodes {
		code, err := domain.NewToteCode(raw)
		if err != nil {
			return none, malformed("toteCodes must not be blank")
		}
		codes = append(codes, code)
	}
	return application.RegisterTotesCommand{Tenant: tenant, Warehouse: warehouse, Codes: codes, Operator: operator}, nil
}

type toteRetirementDocument struct {
	carriedIdentity
	Warehouse *string `json:"warehouse"`
	ToteCode  *string `json:"toteCode"`
}

// RetireToteCommand 把周转箱停用译成命令。
func RetireToteCommand(reader io.Reader, tenant domain.TenantID, operator domain.Registrant) (application.RetireToteCommand, error) {
	var none application.RetireToteCommand
	document, err := decodeDocument[toteRetirementDocument](reader, "tote retirement")
	if err != nil {
		return none, err
	}
	if err := document.refuseCarriedIdentity(); err != nil {
		return none, err
	}
	warehouse, err := requiredText("warehouse", document.Warehouse, domain.NewWarehouseCode)
	if err != nil {
		return none, err
	}
	code, err := requiredText("toteCode", document.ToteCode, domain.NewToteCode)
	if err != nil {
		return none, err
	}
	return application.RetireToteCommand{Tenant: tenant, Warehouse: warehouse, Code: code, Operator: operator}, nil
}
