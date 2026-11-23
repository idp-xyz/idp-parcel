package registrationjson

import (
	"io"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

type agentConnectorDocument struct {
	carriedIdentity
	Warehouse *string `json:"warehouse"`
}

// AgentConnectorWarehouse 把「这个仓采用模拟源」译成仓编码。凭据不在载荷里。
func AgentConnectorWarehouse(reader io.Reader, tenant domain.TenantID, _ domain.Registrant) (domain.WarehouseCode, error) {
	var none domain.WarehouseCode
	document, err := decodeDocument[agentConnectorDocument](reader, "agent connector")
	if err != nil {
		return none, err
	}
	if err := document.refuseCarriedIdentity(); err != nil {
		return none, err
	}
	_ = tenant
	return requiredText("warehouse", document.Warehouse, domain.NewWarehouseCode)
}

type agentStatementDocument struct {
	carriedIdentity
	Source     *string `json:"source"`
	StatedAt   *string `json:"statedAt"`
	ValidUntil *string `json:"validUntil"`
	Warehouse  *string `json:"warehouse"`
	Owner      *string `json:"ownerAccount"`
	GoodsCode  *string `json:"goodsCode"`
	Unit       *string `json:"unit"`
	Quantity   *int64  `json:"quantity"`
}

// AgentStatement 把代理仓报来的声明译成要保全的那一份。
func AgentStatement(reader io.Reader, _ domain.TenantID, _ domain.Registrant) (ports.AgentStatement, error) {
	var none ports.AgentStatement
	document, err := decodeDocument[agentStatementDocument](reader, "agent statement")
	if err != nil {
		return none, err
	}
	if err := document.refuseCarriedIdentity(); err != nil {
		return none, err
	}
	source, err := requiredText("source", document.Source, nonBlank("source"))
	if err != nil {
		return none, err
	}
	stated, err := requiredConfirmInstant("statedAt", document.StatedAt)
	if err != nil {
		return none, err
	}
	until, err := requiredConfirmInstant("validUntil", document.ValidUntil)
	if err != nil {
		return none, err
	}
	warehouse, err := requiredText("warehouse", document.Warehouse, domain.NewWarehouseCode)
	if err != nil {
		return none, err
	}
	owner, err := requiredText("ownerAccount", document.Owner, domain.NewOwnerAccountID)
	if err != nil {
		return none, err
	}
	goods, err := requiredText("goodsCode", document.GoodsCode, domain.NewGoodsCode)
	if err != nil {
		return none, err
	}
	unit, err := requiredText("unit", document.Unit, domain.NewGoodsUnitName)
	if err != nil {
		return none, err
	}
	if document.Quantity == nil || *document.Quantity < 0 {
		return none, malformed("quantity is required")
	}
	return ports.AgentStatement{
		Source: source, StatedAt: stated, ValidUntil: until, Warehouse: warehouse, Owner: owner, Goods: goods, Unit: unit, Quantity: *document.Quantity,
	}, nil
}

type agentResultDocument struct {
	carriedIdentity
	ResultID   *string `json:"resultId"`
	RequestID  *string `json:"requestId"`
	Kind       *string `json:"kind"`
	Weight     *string `json:"weight"`
	WeightUnit *string `json:"weightUnit"`
	Length     *string `json:"length"`
	Width      *string `json:"width"`
	Height     *string `json:"height"`
	Dimension  *string `json:"dimension"`
}

// AgentResult 把代理仓报来的结果译成要保全的那一份。转出只带请求与种类 FORWARDED。
func AgentResult(reader io.Reader, _ domain.TenantID, _ domain.Registrant) (ports.AgentResult, error) {
	var none ports.AgentResult
	document, err := decodeDocument[agentResultDocument](reader, "agent result")
	if err != nil {
		return none, err
	}
	if err := document.refuseCarriedIdentity(); err != nil {
		return none, err
	}
	request, err := requiredText("requestId", document.RequestID, domain.NewOutboundRequestID)
	if err != nil {
		return none, err
	}
	kind, err := requiredText("kind", document.Kind, nonBlank("kind"))
	if err != nil {
		return none, err
	}
	id := ""
	if document.ResultID != nil {
		id = *document.ResultID
	}
	if kind != "FORWARDED" && id == "" {
		return none, malformed("resultId is required")
	}
	return ports.AgentResult{
		ID: id, Request: request, Kind: kind, Weight: textOrEmpty(document.Weight), WeightUnit: textOrEmpty(document.WeightUnit),
		Length: textOrEmpty(document.Length), Width: textOrEmpty(document.Width), Height: textOrEmpty(document.Height), Dimension: textOrEmpty(document.Dimension),
	}, nil
}

func nonBlank(path string) func(string) (string, error) {
	return func(value string) (string, error) {
		if value == "" {
			return "", malformed("%s is required", path)
		}
		return value, nil
	}
}
