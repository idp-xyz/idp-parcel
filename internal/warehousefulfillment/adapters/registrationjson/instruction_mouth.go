package registrationjson

import (
	"io"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
)

// OwnerInstructionFields 是货主提交加工或换标指令解出来的内容。货主与来源请求键不在这里。
type OwnerInstructionFields struct {
	AccessRequestID string
	Warehouse       domain.WarehouseCode
	Goods           domain.GoodsCode
	Unit            domain.GoodsUnitName
	Quantity        int64
	FromAlias       domain.GoodsAliasCode
	ToAlias         domain.GoodsAliasCode
	Batch           domain.BatchNumber
	Expiry          domain.ExpiryDate
	Reference       string
	NamedRevision   int
	Disassemble     bool
}

type ownerInstructionDocument struct {
	carriedIdentity
	refusedOwner
	AccessRequestID *string `json:"accessRequestId"`
	Warehouse       *string `json:"warehouse"`
	GoodsCode       *string `json:"goodsCode"`
	Unit            *string `json:"unit"`
	Quantity        *int64  `json:"quantity"`
	FromAlias       *string `json:"fromAlias"`
	ToAlias         *string `json:"toAlias"`
	BatchNumber     *string `json:"batchNumber"`
	ExpiryDate      *string `json:"expiryDate"`
	Reference       *string `json:"reference"`
	GoodsRevision   *int    `json:"goodsRevision"`
	Disassemble     *bool   `json:"disassemble"`
}

// ReadOwnerProcessingInstruction 把货主加工指令译成字段。拆分由 disassemble 点出，缺省是加工。
func ReadOwnerProcessingInstruction(reader io.Reader) (OwnerInstructionFields, error) {
	return readOwnerInstruction(reader, "processing instruction", true)
}

// ReadOwnerRelabelInstruction 把货主换标指令译成字段。这份报文没有 disassemble。
func ReadOwnerRelabelInstruction(reader io.Reader) (OwnerInstructionFields, error) {
	return readOwnerInstruction(reader, "relabel instruction", false)
}

func readOwnerInstruction(reader io.Reader, subject string, allowDisassemble bool) (OwnerInstructionFields, error) {
	var none OwnerInstructionFields
	document, err := decodeDocument[ownerInstructionDocument](reader, subject)
	if err != nil {
		return none, err
	}
	if !allowDisassemble && document.Disassemble != nil {
		return none, malformed("disassemble must not be carried in the payload")
	}
	if err := document.refuseCarriedIdentity(); err != nil {
		return none, err
	}
	if err := refuseInstructionOwner(document.refusedOwner); err != nil {
		return none, err
	}
	fields, err := instructionBody(document)
	if err != nil {
		return none, err
	}
	fields.AccessRequestID = rawAccessRequestID(document.AccessRequestID)
	if document.Disassemble != nil {
		fields.Disassemble = *document.Disassemble
	}
	return fields, nil
}

type ownerInstructionWithdrawalDocument struct {
	carriedIdentity
	refusedOwner
	AccessRequestID *string `json:"accessRequestId"`
	InstructionID   *string `json:"instructionId"`
}

// OwnerInstructionWithdrawalFields 是货主撤回解出来的指名。来源请求键由铸造器加前缀。
type OwnerInstructionWithdrawalFields struct {
	AccessRequestID string
	ID              domain.InstructionID
}

func ReadOwnerInstructionWithdrawal(reader io.Reader) (OwnerInstructionWithdrawalFields, error) {
	var none OwnerInstructionWithdrawalFields
	document, err := decodeDocument[ownerInstructionWithdrawalDocument](reader, "instruction withdrawal")
	if err != nil {
		return none, err
	}
	if err := document.refuseCarriedIdentity(); err != nil {
		return none, err
	}
	if err := refuseInstructionOwner(document.refusedOwner); err != nil {
		return none, err
	}
	id, err := requiredText("instructionId", document.InstructionID, domain.NewInstructionID)
	if err != nil {
		return none, err
	}
	return OwnerInstructionWithdrawalFields{AccessRequestID: rawAccessRequestID(document.AccessRequestID), ID: id}, nil
}

type operatorInstructionDocument struct {
	carriedIdentity
	DecisionID    *string `json:"decisionId"`
	OwnerAccount  *string `json:"ownerAccount"`
	Warehouse     *string `json:"warehouse"`
	GoodsCode     *string `json:"goodsCode"`
	Unit          *string `json:"unit"`
	Quantity      *int64  `json:"quantity"`
	FromAlias     *string `json:"fromAlias"`
	ToAlias       *string `json:"toAlias"`
	BatchNumber   *string `json:"batchNumber"`
	ExpiryDate    *string `json:"expiryDate"`
	Reference     *string `json:"reference"`
	GoodsRevision *int    `json:"goodsRevision"`
	Reason        *string `json:"reason"`
	Basis         *string `json:"basis"`
	Disassemble   *bool   `json:"disassemble"`
}

// OperatorInstructionFields 是运营发起一份指令解出来的内容。决定标识在载荷里，租户与操作者取自信封。
type OperatorInstructionFields struct {
	OwnerInstructionFields
	Owner      domain.OwnerAccountID
	DecisionID domain.RequestKey
	Reason     string
	Basis      string
}

func ReadOperatorProcessingInstruction(reader io.Reader) (OperatorInstructionFields, error) {
	return readOperatorInstruction(reader, "processing instruction", true)
}

func ReadOperatorRelabelInstruction(reader io.Reader) (OperatorInstructionFields, error) {
	return readOperatorInstruction(reader, "relabel instruction", false)
}

func readOperatorInstruction(reader io.Reader, subject string, allowDisassemble bool) (OperatorInstructionFields, error) {
	var none OperatorInstructionFields
	document, err := decodeDocument[operatorInstructionDocument](reader, subject)
	if err != nil {
		return none, err
	}
	if !allowDisassemble && document.Disassemble != nil {
		return none, malformed("disassemble must not be carried in the payload")
	}
	if err := document.refuseCarriedIdentity(); err != nil {
		return none, err
	}
	decisionID, err := requiredText("decisionId", document.DecisionID, domain.NewRequestKey)
	if err != nil {
		return none, err
	}
	owner, err := requiredText("ownerAccount", document.OwnerAccount, domain.NewOwnerAccountID)
	if err != nil {
		return none, err
	}
	if document.Reason == nil || *document.Reason == "" {
		return none, malformed("reason is required")
	}
	body, err := instructionBody(ownerInstructionDocument{
		Warehouse: document.Warehouse, GoodsCode: document.GoodsCode, Unit: document.Unit, Quantity: document.Quantity,
		FromAlias: document.FromAlias, ToAlias: document.ToAlias, BatchNumber: document.BatchNumber, ExpiryDate: document.ExpiryDate,
		Reference: document.Reference, GoodsRevision: document.GoodsRevision,
	})
	if err != nil {
		return none, err
	}
	body.Disassemble = document.Disassemble != nil && *document.Disassemble
	return OperatorInstructionFields{
		OwnerInstructionFields: body, Owner: owner, DecisionID: decisionID, Reason: *document.Reason, Basis: valueOrEmptyText(document.Basis),
	}, nil
}

func refuseInstructionOwner(refused refusedOwner) error {
	if len(refused.OwnerAccount) > 0 {
		return malformed("ownerAccount must not be carried in the payload; the owner comes from the customer envelope")
	}
	return nil
}

func valueOrEmptyText(raw *string) string {
	if raw == nil {
		return ""
	}
	return *raw
}

type operatorInstructionVoidDocument struct {
	carriedIdentity
	InstructionID *string `json:"instructionId"`
	Reason        *string `json:"reason"`
}

// OperatorInstructionVoidFields 是运营作废解出来的指名。作废不另带决定标识，重放看指令自己的状态。
type OperatorInstructionVoidFields struct {
	ID     domain.InstructionID
	Reason string
}

func ReadOperatorInstructionVoid(reader io.Reader) (OperatorInstructionVoidFields, error) {
	var none OperatorInstructionVoidFields
	document, err := decodeDocument[operatorInstructionVoidDocument](reader, "instruction void")
	if err != nil {
		return none, err
	}
	if err := document.refuseCarriedIdentity(); err != nil {
		return none, err
	}
	id, err := requiredText("instructionId", document.InstructionID, domain.NewInstructionID)
	if err != nil {
		return none, err
	}
	if document.Reason == nil || *document.Reason == "" {
		return none, malformed("reason is required")
	}
	return OperatorInstructionVoidFields{ID: id, Reason: *document.Reason}, nil
}

func instructionBody(document ownerInstructionDocument) (OwnerInstructionFields, error) {
	warehouse, err := requiredText("warehouse", document.Warehouse, domain.NewWarehouseCode)
	if err != nil {
		return OwnerInstructionFields{}, err
	}
	goods, err := requiredText("goodsCode", document.GoodsCode, domain.NewGoodsCode)
	if err != nil {
		return OwnerInstructionFields{}, err
	}
	unit, err := optionalDomain("unit", document.Unit, domain.NewGoodsUnitName)
	if err != nil {
		return OwnerInstructionFields{}, err
	}
	if document.Quantity == nil || *document.Quantity <= 0 {
		return OwnerInstructionFields{}, malformed("quantity must be a positive integer")
	}
	fromAlias, err := optionalDomain("fromAlias", document.FromAlias, domain.NewGoodsAliasCode)
	if err != nil {
		return OwnerInstructionFields{}, err
	}
	toAlias, err := optionalDomain("toAlias", document.ToAlias, domain.NewGoodsAliasCode)
	if err != nil {
		return OwnerInstructionFields{}, err
	}
	batch, err := optionalDomain("batchNumber", document.BatchNumber, domain.NewBatchNumber)
	if err != nil {
		return OwnerInstructionFields{}, err
	}
	var expiry domain.ExpiryDate
	if document.ExpiryDate != nil && *document.ExpiryDate != "" {
		expiry, err = domain.NewExpiryDate(*document.ExpiryDate)
		if err != nil {
			return OwnerInstructionFields{}, malformed("expiryDate must be a calendar date")
		}
	}
	if document.GoodsRevision != nil && *document.GoodsRevision < 0 {
		return OwnerInstructionFields{}, malformed("goodsRevision must not be negative")
	}
	revision := 0
	if document.GoodsRevision != nil {
		revision = *document.GoodsRevision
	}
	reference := ""
	if document.Reference != nil {
		reference = *document.Reference
	}
	return OwnerInstructionFields{
		Warehouse: warehouse, Goods: goods, Unit: unit, Quantity: *document.Quantity, FromAlias: fromAlias, ToAlias: toAlias,
		Batch: batch, Expiry: expiry, Reference: reference, NamedRevision: revision,
	}, nil
}
