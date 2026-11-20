package registrationjson

import (
	"io"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/application"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

type createCountDocument struct {
	carriedIdentity
	DecisionID   *string   `json:"decisionId"`
	Warehouse    *string   `json:"warehouse"`
	Locations    *[]string `json:"locations"`
	OwnerAccount *string   `json:"ownerAccount"`
	Goods        *[]string `json:"goods"`
	Note         *string   `json:"note"`
}

// CreateCountCommand 把创建盘点译成命令。库位至少一个。货主与商品可以不给。
func CreateCountCommand(reader io.Reader) (application.CreateCountCommand, error) {
	var none application.CreateCountCommand
	document, err := decodeDocument[createCountDocument](reader, "stock count")
	if err != nil {
		return none, err
	}
	if err := document.refuseCarriedIdentity(); err != nil {
		return none, err
	}
	decision, err := requiredText("decisionId", document.DecisionID, domain.NewRequestKey)
	if err != nil {
		return none, err
	}
	warehouse, err := requiredText("warehouse", document.Warehouse, domain.NewWarehouseCode)
	if err != nil {
		return none, err
	}
	if document.Locations == nil || len(*document.Locations) == 0 {
		return none, malformed("locations is required")
	}
	locations := make([]domain.LocationCode, 0, len(*document.Locations))
	for _, raw := range *document.Locations {
		location, err := domain.NewLocationCode(raw)
		if err != nil {
			return none, malformed("locations must not contain a blank code")
		}
		locations = append(locations, location)
	}
	var owner domain.OwnerAccountID
	if document.OwnerAccount != nil && *document.OwnerAccount != "" {
		owner, err = domain.NewOwnerAccountID(*document.OwnerAccount)
		if err != nil {
			return none, malformed("ownerAccount must not be blank")
		}
	}
	var goods []domain.GoodsCode
	if document.Goods != nil {
		for _, raw := range *document.Goods {
			code, err := domain.NewGoodsCode(raw)
			if err != nil {
				return none, malformed("goods must not contain a blank code")
			}
			goods = append(goods, code)
		}
	}
	note := ""
	if document.Note != nil {
		note = *document.Note
	}
	return application.CreateCountCommand{DecisionID: decision, Warehouse: warehouse, Locations: locations, Owner: owner, Goods: goods, Note: note}, nil
}

type countStepDocument struct {
	carriedIdentity
	CountID *string `json:"countId"`
	Reason  *string `json:"reason"`
}

// CountCommand 把开始、完成或作废译成命令。作废的原因可空。
func CountCommand(reader io.Reader) (application.CountCommand, error) {
	var none application.CountCommand
	document, err := decodeDocument[countStepDocument](reader, "stock count")
	if err != nil {
		return none, err
	}
	if err := document.refuseCarriedIdentity(); err != nil {
		return none, err
	}
	id, err := requiredText("countId", document.CountID, domain.NewCountID)
	if err != nil {
		return none, err
	}
	reason := ""
	if document.Reason != nil {
		reason = *document.Reason
	}
	return application.CountCommand{ID: id, Reason: reason}, nil
}

type approvalDutyDocument struct {
	carriedIdentity
	Revision         *int  `json:"revision"`
	DistinctSubjects *bool `json:"distinctSubjects"`
}

// ApprovalDutyCommand 把批准职责规则译成命令。租户与登记人由 Intake 填。
func ApprovalDutyCommand(reader io.Reader, tenant domain.TenantID, operator domain.Registrant) (application.RegisterApprovalDutyCommand, error) {
	var none application.RegisterApprovalDutyCommand
	document, err := decodeDocument[approvalDutyDocument](reader, "approval duty")
	if err != nil {
		return none, err
	}
	if err := document.refuseCarriedIdentity(); err != nil {
		return none, err
	}
	if document.Revision == nil || *document.Revision < 1 {
		return none, malformed("revision must be a positive integer")
	}
	if document.DistinctSubjects == nil {
		return none, malformed("distinctSubjects is required")
	}
	return application.RegisterApprovalDutyCommand{Tenant: tenant, Revision: *document.Revision, Distinct: *document.DistinctSubjects, Operator: operator}, nil
}

type countApprovalDocument struct {
	carriedIdentity
	refusedOwner
	DifferenceID *string `json:"differenceId"`
	Note         *string `json:"note"`
	Reason       *string `json:"reason"`
}

// ApproveCountCommand 把批准一份盘点差额译成命令。
func ApproveCountCommand(reader io.Reader) (application.ApproveCountCommand, error) {
	document, id, err := readCountApproval(reader, false)
	if err != nil {
		return application.ApproveCountCommand{}, err
	}
	note := ""
	if document.Note != nil {
		note = *document.Note
	}
	return application.ApproveCountCommand{ID: id, Note: note}, nil
}

// RejectCountCommand 把驳回一份盘点差额译成命令。原因必带。
func RejectCountCommand(reader io.Reader) (application.RejectCountCommand, error) {
	document, id, err := readCountApproval(reader, true)
	if err != nil {
		return application.RejectCountCommand{}, err
	}
	if document.Reason == nil || *document.Reason == "" {
		return application.RejectCountCommand{}, malformed("reason is required")
	}
	return application.RejectCountCommand{ID: id, Reason: *document.Reason}, nil
}

func readCountApproval(reader io.Reader, reject bool) (countApprovalDocument, domain.StockDiscrepancyID, error) {
	var none countApprovalDocument
	document, err := decodeDocument[countApprovalDocument](reader, "count approval")
	if err != nil {
		return none, domain.StockDiscrepancyID{}, err
	}
	if err := document.refuseCarriedIdentity(); err != nil {
		return none, domain.StockDiscrepancyID{}, err
	}
	if len(document.OwnerAccount) > 0 {
		return none, domain.StockDiscrepancyID{}, malformed("ownerAccount must not be carried in the payload")
	}
	if reject && document.Note != nil {
		return none, domain.StockDiscrepancyID{}, malformed("note must not be carried in the payload")
	}
	id, err := requiredText("differenceId", document.DifferenceID, domain.NewStockDiscrepancyID)
	if err != nil {
		return none, domain.StockDiscrepancyID{}, err
	}
	return document, id, nil
}

type jobDiscrepancyDecisionDocument struct {
	carriedIdentity
	refusedOwner
	DiscrepancyID *string `json:"discrepancyId"`
	Note          *string `json:"note"`
	Reason        *string `json:"reason"`
}

// ApproveJobDiscrepancyCommand 把批准一份库内作业差异译成命令。
func ApproveJobDiscrepancyCommand(reader io.Reader) (application.ApproveCountCommand, error) {
	document, id, err := readJobDiscrepancyDecision(reader, false)
	if err != nil {
		return application.ApproveCountCommand{}, err
	}
	note := ""
	if document.Note != nil {
		note = *document.Note
	}
	return application.ApproveCountCommand{ID: id, Note: note}, nil
}

// RejectJobDiscrepancyCommand 把驳回一份库内作业差异译成命令。原因必带。
func RejectJobDiscrepancyCommand(reader io.Reader) (application.RejectCountCommand, error) {
	document, id, err := readJobDiscrepancyDecision(reader, true)
	if err != nil {
		return application.RejectCountCommand{}, err
	}
	if document.Reason == nil || *document.Reason == "" {
		return application.RejectCountCommand{}, malformed("reason is required")
	}
	return application.RejectCountCommand{ID: id, Reason: *document.Reason}, nil
}

func readJobDiscrepancyDecision(reader io.Reader, reject bool) (jobDiscrepancyDecisionDocument, domain.StockDiscrepancyID, error) {
	var none jobDiscrepancyDecisionDocument
	document, err := decodeDocument[jobDiscrepancyDecisionDocument](reader, "job discrepancy decision")
	if err != nil {
		return none, domain.StockDiscrepancyID{}, err
	}
	if err := document.refuseCarriedIdentity(); err != nil {
		return none, domain.StockDiscrepancyID{}, err
	}
	if len(document.OwnerAccount) > 0 {
		return none, domain.StockDiscrepancyID{}, malformed("ownerAccount must not be carried in the payload")
	}
	if reject && document.Note != nil {
		return none, domain.StockDiscrepancyID{}, malformed("note must not be carried in the payload")
	}
	id, err := requiredText("discrepancyId", document.DiscrepancyID, domain.NewStockDiscrepancyID)
	if err != nil {
		return none, domain.StockDiscrepancyID{}, err
	}
	return document, id, nil
}

type adjustmentDocument struct {
	carriedIdentity
	AdjustmentID *string              `json:"adjustmentId"`
	OwnerAccount *string              `json:"ownerAccount"`
	Warehouse    *string              `json:"warehouse"`
	Reason       *string              `json:"reason"`
	Lines        *[]adjustmentLineDoc `json:"lines"`
}

type adjustmentLineDoc struct {
	Kind           *string `json:"kind"`
	LocationCode   *string `json:"locationCode"`
	GoodsCode      *string `json:"goodsCode"`
	Unit           *string `json:"unit"`
	StockState     *string `json:"stockState"`
	Quantity       *int64  `json:"quantity"`
	InboundBatchID *string `json:"inboundBatchId"`
	Basis          *string `json:"basis"`
}

// SubmitAdjustmentCommand 把库存调整译成命令。
func SubmitAdjustmentCommand(reader io.Reader) (application.SubmitAdjustmentCommand, error) {
	var none application.SubmitAdjustmentCommand
	document, err := decodeDocument[adjustmentDocument](reader, "stock adjustment")
	if err != nil {
		return none, err
	}
	if err := document.refuseCarriedIdentity(); err != nil {
		return none, err
	}
	id, err := requiredText("adjustmentId", document.AdjustmentID, domain.NewRequestKey)
	if err != nil {
		return none, err
	}
	owner, err := requiredText("ownerAccount", document.OwnerAccount, domain.NewOwnerAccountID)
	if err != nil {
		return none, err
	}
	warehouse, err := requiredText("warehouse", document.Warehouse, domain.NewWarehouseCode)
	if err != nil {
		return none, err
	}
	if document.Reason == nil || *document.Reason == "" {
		return none, malformed("reason is required")
	}
	if document.Lines == nil || len(*document.Lines) == 0 {
		return none, malformed("lines is required")
	}
	lines := make([]ports.AdjustmentLine, 0, len(*document.Lines))
	for _, item := range *document.Lines {
		if item.Kind == nil || (*item.Kind != "INCREASE" && *item.Kind != "DECREASE" && *item.Kind != "DESTRUCTION") {
			return none, malformed("kind must be INCREASE, DECREASE or DESTRUCTION")
		}
		if item.Quantity == nil || *item.Quantity <= 0 {
			return none, malformed("quantity must be a positive integer")
		}
		location, err := requiredText("locationCode", item.LocationCode, domain.NewLocationCode)
		if err != nil {
			return none, err
		}
		goods, err := requiredText("goodsCode", item.GoodsCode, domain.NewGoodsCode)
		if err != nil {
			return none, err
		}
		unit, err := requiredText("unit", item.Unit, domain.NewGoodsUnitName)
		if err != nil {
			return none, err
		}
		if item.StockState == nil {
			return none, malformed("stockState is required")
		}
		state, err := domain.NewStockState(*item.StockState)
		if err != nil {
			return none, malformed("stockState is not a stock state")
		}
		var inbound domain.InboundBatchID
		if item.InboundBatchID != nil && *item.InboundBatchID != "" {
			inbound, err = domain.NewInboundBatchID(*item.InboundBatchID)
			if err != nil {
				return none, malformed("inboundBatchId must not be blank")
			}
		}
		basis := ""
		if item.Basis != nil {
			basis = *item.Basis
		}
		lines = append(lines, ports.AdjustmentLine{
			Kind: ports.AdjustmentKind(*item.Kind), Location: location, Goods: goods, Unit: unit, State: state,
			Inbound: inbound, Quantity: *item.Quantity, Basis: basis,
		})
	}
	return application.SubmitAdjustmentCommand{ID: id, Owner: owner, Warehouse: warehouse, Reason: *document.Reason, Lines: lines}, nil
}

type adjustmentDecisionDocument struct {
	carriedIdentity
	refusedOwner
	AdjustmentID *string `json:"adjustmentId"`
	Note         *string `json:"note"`
	Reason       *string `json:"reason"`
}

// ApproveAdjustmentCommand 把批准一份库存调整译成命令。
func ApproveAdjustmentCommand(reader io.Reader) (application.ApproveAdjustmentCommand, error) {
	id, note, _, err := readAdjustmentDecision(reader, false)
	if err != nil {
		return application.ApproveAdjustmentCommand{}, err
	}
	return application.ApproveAdjustmentCommand{ID: id, Note: note}, nil
}

// RejectAdjustmentCommand 把驳回一份库存调整译成命令。
func RejectAdjustmentCommand(reader io.Reader) (application.RejectAdjustmentCommand, error) {
	id, _, reason, err := readAdjustmentDecision(reader, true)
	if err != nil {
		return application.RejectAdjustmentCommand{}, err
	}
	if reason == "" {
		return application.RejectAdjustmentCommand{}, malformed("reason is required")
	}
	return application.RejectAdjustmentCommand{ID: id, Reason: reason}, nil
}

func readAdjustmentDecision(reader io.Reader, reject bool) (domain.RequestKey, string, string, error) {
	var none domain.RequestKey
	document, err := decodeDocument[adjustmentDecisionDocument](reader, "stock adjustment decision")
	if err != nil {
		return none, "", "", err
	}
	if err := document.refuseCarriedIdentity(); err != nil {
		return none, "", "", err
	}
	if len(document.OwnerAccount) > 0 {
		return none, "", "", malformed("ownerAccount must not be carried in the payload")
	}
	id, err := requiredText("adjustmentId", document.AdjustmentID, domain.NewRequestKey)
	if err != nil {
		return none, "", "", err
	}
	note, reason := "", ""
	if document.Note != nil {
		note = *document.Note
	}
	if document.Reason != nil {
		reason = *document.Reason
	}
	if reject && note != "" {
		return none, "", "", malformed("note must not be carried in the payload")
	}
	return id, note, reason, nil
}

type countEntryDocument struct {
	carriedIdentity
	FactID         *string              `json:"factId"`
	OccurredAt     *string              `json:"occurredAt"`
	DeviceSequence *int64               `json:"deviceSequence"`
	Warehouse      *string              `json:"warehouse"`
	CountID        *string              `json:"countId"`
	LocationCode   *string              `json:"locationCode"`
	Lines          *[]countEntryLineDoc `json:"lines"`
}

type countEntryLineDoc struct {
	Observed    *string `json:"observed"`
	Owner       *string `json:"ownerAccount"`
	GoodsCode   *string `json:"goodsCode"`
	Unit        *string `json:"unit"`
	StockState  *string `json:"stockState"`
	Quantity    *int64  `json:"quantity"`
	LabelAlias  *string `json:"labelAlias"`
	BatchNumber *string `json:"batchNumber"`
	ExpiryDate  *string `json:"expiryDate"`
}

// RecordCountCommand 把清点结果译成命令。不收入库批次。
func RecordCountCommand(reader io.Reader) (application.RecordCountCommand, error) {
	var none application.RecordCountCommand
	document, err := decodeDocument[countEntryDocument](reader, "count entry")
	if err != nil {
		return none, err
	}
	if err := document.refuseCarriedIdentity(); err != nil {
		return none, err
	}
	fact, err := requiredText("factId", document.FactID, domain.NewInternalJobFactID)
	if err != nil {
		return none, err
	}
	occurred, err := requiredInstant("occurredAt", document.OccurredAt)
	if err != nil {
		return none, err
	}
	if document.DeviceSequence == nil || *document.DeviceSequence < 1 {
		return none, malformed("deviceSequence must be a positive integer")
	}
	warehouse, err := requiredText("warehouse", document.Warehouse, domain.NewWarehouseCode)
	if err != nil {
		return none, err
	}
	countID, err := requiredText("countId", document.CountID, domain.NewCountID)
	if err != nil {
		return none, err
	}
	location, err := requiredText("locationCode", document.LocationCode, domain.NewLocationCode)
	if err != nil {
		return none, err
	}
	if document.Lines == nil {
		return none, malformed("lines is required")
	}
	lines := make([]ports.CountEntryLine, 0, len(*document.Lines))
	for _, item := range *document.Lines {
		if item.Quantity == nil || *item.Quantity < 0 {
			return none, malformed("quantity must not be negative")
		}
		line := ports.CountEntryLine{Quantity: *item.Quantity}
		if item.Observed != nil && *item.Observed != "" {
			line.Observed = *item.Observed
			lines = append(lines, line)
			continue
		}
		line.Owner, err = requiredText("ownerAccount", item.Owner, domain.NewOwnerAccountID)
		if err != nil {
			return none, err
		}
		line.Goods, err = requiredText("goodsCode", item.GoodsCode, domain.NewGoodsCode)
		if err != nil {
			return none, err
		}
		line.Unit, err = requiredText("unit", item.Unit, domain.NewGoodsUnitName)
		if err != nil {
			return none, err
		}
		if item.StockState == nil {
			return none, malformed("stockState is required")
		}
		line.State, err = domain.NewStockState(*item.StockState)
		if err != nil {
			return none, malformed("stockState is not a stock state")
		}
		line.Alias, err = optionalDomain("labelAlias", item.LabelAlias, domain.NewGoodsAliasCode)
		if err != nil {
			return none, err
		}
		line.Batch, err = optionalDomain("batchNumber", item.BatchNumber, domain.NewBatchNumber)
		if err != nil {
			return none, err
		}
		if item.ExpiryDate != nil && *item.ExpiryDate != "" {
			line.Expiry, err = domain.NewExpiryDate(*item.ExpiryDate)
			if err != nil {
				return none, malformed("expiryDate must be a calendar date")
			}
		}
		lines = append(lines, line)
	}
	return application.RecordCountCommand{
		FactID: fact, OccurredAt: occurred, Sequence: *document.DeviceSequence, Warehouse: warehouse, CountID: countID, Location: location, Lines: lines,
	}, nil
}
