package registrationjson

import (
	"io"
	"strings"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/application"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

type waveRuleDocument struct {
	carriedIdentity
	Warehouse *string `json:"warehouse"`
	RuleCode  *string `json:"ruleCode"`
	Revision  *int    `json:"revision"`
	Name      *string `json:"name"`
	// OwnerAccount 与 Dispatch 不带是不限；带了就得是一个值，空串不是「不限」的写法。
	OwnerAccount *string `json:"ownerAccount"`
	Dispatch     *string `json:"dispatch"`
	// Profile 不带是不限件型。
	Profile     *string `json:"profile"`
	MaxRequests *int    `json:"maxRequests"`
	// MaxLines 与 MaxUnits 不带是不限；带了就得是正整数。
	MaxLines    *int   `json:"maxLines"`
	MaxUnits    *int64 `json:"maxUnits"`
	BatchPick   *bool  `json:"batchPick"`
	Deactivated *bool  `json:"deactivated"`
	// ReleaseTimes 是每天到点放的时点（HH:MM，按仓的业务时区）；不带是只由人点。格式由用例核。
	ReleaseTimes *[]string `json:"releaseTimes"`
}

// RegisterWaveRuleCommand 把成波规则的登记译成命令。一个波次最多几张（maxRequests）必须带：
// 它是租户的取值，产品不替他填一个数。
func RegisterWaveRuleCommand(reader io.Reader, tenant domain.TenantID, operator domain.Registrant) (application.RegisterWaveRuleCommand, error) {
	var none application.RegisterWaveRuleCommand
	document, err := decodeDocument[waveRuleDocument](reader, "wave rule")
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
	code, err := requiredText("ruleCode", document.RuleCode, domain.NewWaveRuleCode)
	if err != nil {
		return none, err
	}
	if document.Revision == nil || *document.Revision < 1 {
		return none, malformed("revision must be a positive integer")
	}
	if document.Name == nil || strings.TrimSpace(*document.Name) == "" {
		return none, malformed("name is required")
	}
	if document.MaxRequests == nil || *document.MaxRequests < 1 {
		return none, malformed("maxRequests must be a positive integer")
	}
	command := application.RegisterWaveRuleCommand{
		Tenant: tenant, Warehouse: warehouse, Code: code, Revision: *document.Revision, Name: strings.TrimSpace(*document.Name),
		Profile: ports.WaveRuleProfileAny, MaxRequests: *document.MaxRequests,
		BatchPick:   document.BatchPick != nil && *document.BatchPick,
		Deactivated: document.Deactivated != nil && *document.Deactivated,
		Operator:    operator,
	}
	if document.ReleaseTimes != nil {
		command.ReleaseTimes = *document.ReleaseTimes
	}
	if document.MaxLines != nil {
		if *document.MaxLines < 1 {
			return none, malformed("maxLines must be a positive integer")
		}
		command.MaxLines = *document.MaxLines
	}
	if document.MaxUnits != nil {
		if *document.MaxUnits < 1 {
			return none, malformed("maxUnits must be a positive integer")
		}
		command.MaxUnits = *document.MaxUnits
	}
	if document.OwnerAccount != nil {
		owner, err := domain.NewOwnerAccountID(*document.OwnerAccount)
		if err != nil {
			return none, malformed("ownerAccount must not be blank")
		}
		command.Owner = owner.String()
	}
	if document.Dispatch != nil {
		dispatch, err := domain.NewDispatchMethod(*document.Dispatch)
		if err != nil {
			return none, malformed("dispatch is not a dispatch method")
		}
		command.Dispatch = dispatch.String()
	}
	if document.Profile != nil {
		switch *document.Profile {
		case ports.WaveRuleProfileAny, ports.WaveRuleProfileSingleUnit, ports.WaveRuleProfileMultiUnit:
			command.Profile = *document.Profile
		default:
			return none, malformed("profile is not a wave rule profile")
		}
	}
	return command, nil
}

type dueWaveRulesDocument struct {
	carriedIdentity
	// Warehouse 不带是本租户各仓的规则都看。
	Warehouse *string `json:"warehouse"`
}

// RunDueWaveRulesCommand 把「放到点的规则」译成命令。此刻是几点由服务端的钟定，载荷里不带时刻。
func RunDueWaveRulesCommand(reader io.Reader) (application.RunDueWaveRulesCommand, error) {
	var none application.RunDueWaveRulesCommand
	document, err := decodeDocument[dueWaveRulesDocument](reader, "due wave rules")
	if err != nil {
		return none, err
	}
	if err := document.refuseCarriedIdentity(); err != nil {
		return none, err
	}
	var command application.RunDueWaveRulesCommand
	if document.Warehouse != nil {
		warehouse, err := domain.NewWarehouseCode(*document.Warehouse)
		if err != nil {
			return none, malformed("warehouse must not be blank")
		}
		command.Warehouse = warehouse.String()
	}
	return command, nil
}

type waveRuleRunDocument struct {
	carriedIdentity
	Warehouse *string `json:"warehouse"`
	RuleCode  *string `json:"ruleCode"`
	RunID     *string `json:"runId"`
}

// RunWaveRuleCommand 把按规则成波译成命令。租户与操作者取自身份。
func RunWaveRuleCommand(reader io.Reader) (application.RunWaveRuleCommand, error) {
	var none application.RunWaveRuleCommand
	document, err := decodeDocument[waveRuleRunDocument](reader, "wave rule run")
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
	rule, err := requiredText("ruleCode", document.RuleCode, domain.NewWaveRuleCode)
	if err != nil {
		return none, err
	}
	run, err := requiredText("runId", document.RunID, domain.NewWaveID)
	if err != nil {
		return none, err
	}
	return application.RunWaveRuleCommand{Warehouse: warehouse, Rule: rule, RunID: run}, nil
}
