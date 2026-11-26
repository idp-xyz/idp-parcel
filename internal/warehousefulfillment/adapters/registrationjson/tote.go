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

type toteRequirementDocument struct {
	carriedIdentity
	Warehouse *string `json:"warehouse"`
	Revision  *int    `json:"revision"`
	Required  *bool   `json:"required"`
}

// RegisterToteRequirementCommand 把「拣货必须用周转箱」的登记译成命令。required 必须明说是真是假：
// 不带不当成假——假是租户明说不要求，与没登记是两回事。
func RegisterToteRequirementCommand(reader io.Reader, tenant domain.TenantID, operator domain.Registrant) (application.RegisterToteRequirementCommand, error) {
	var none application.RegisterToteRequirementCommand
	document, err := decodeDocument[toteRequirementDocument](reader, "tote requirement")
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
	if document.Revision == nil || *document.Revision < 1 {
		return none, malformed("revision must be a positive integer")
	}
	if document.Required == nil {
		return none, malformed("required is required")
	}
	return application.RegisterToteRequirementCommand{
		Tenant: tenant, Warehouse: warehouse, Revision: *document.Revision, Required: *document.Required, Operator: operator,
	}, nil
}

// ReleaseToteCommand 把由人放开周转箱译成命令。载荷与停用同形：仓与一个箱号。
func ReleaseToteCommand(reader io.Reader) (application.ReleaseToteCommand, error) {
	var none application.ReleaseToteCommand
	document, err := decodeDocument[toteRetirementDocument](reader, "tote release")
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
	return application.ReleaseToteCommand{Warehouse: warehouse, Code: code}, nil
}
