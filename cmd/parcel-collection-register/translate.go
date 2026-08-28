package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"time"

	"go.idp.xyz/idp-parcel/internal/collectionremittance/application"
	"go.idp.xyz/idp-parcel/internal/collectionremittance/domain"
)

// 本文件把七种登记输入 JSON 折成用例命令。翻译严格且零默认：未知字段拒收（打错字段名
// 不得静默变成「没给」）、有构造门的标识在这里就拒、封闭词表按落库词逐字匹配，其余判据
// 留给领域与编排——这里绝不代填。
//
// 记账输入里**没有分户账键，也没有币种**：键由依据推出，币种取自那本账（见
// application.PostCommand 的注释）。给它们留字段就等于给「记进别人的账」「记错币种」
// 各留一个说得通的入口。

func decodeStrict(raw []byte, into any, what string) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(into); err != nil {
		return fmt.Errorf("%s输入不是本入口的形状：%w", what, err)
	}
	return nil
}

func tenantFrom(value string) (domain.TenantID, error) {
	return domain.NewTenantID(value)
}

type instructionDocument struct {
	TenantID       string    `json:"tenantId"`
	InstructionRef string    `json:"instructionRef"`
	ParcelRef      string    `json:"parcelRef"`
	RequirementRef string    `json:"requirementRef"`
	CustomerRef    string    `json:"customerRef"`
	LegalEntityRef string    `json:"legalEntityRef"`
	Currency       string    `json:"currency"`
	ChannelRef     string    `json:"channelRef"`
	AmountMinor    int64     `json:"amountMinor"`
	InstructedAt   time.Time `json:"instructedAt"`
}

func instructionCommandFromJSON(raw []byte) (application.RegisterInstructionCommand, error) {
	none := application.RegisterInstructionCommand{}
	var document instructionDocument
	if err := decodeStrict(raw, &document, "代收指令"); err != nil {
		return none, err
	}

	tenant, err := tenantFrom(document.TenantID)
	if err != nil {
		return none, err
	}
	id, err := domain.NewCollectionInstructionID(document.InstructionRef)
	if err != nil {
		return none, err
	}
	parcel, err := domain.NewParcelReference(document.ParcelRef)
	if err != nil {
		return none, err
	}
	requirement, err := domain.NewServiceRequirementReference(document.RequirementRef)
	if err != nil {
		return none, err
	}
	ledger, err := ledgerKeyFrom(
		document.CustomerRef, document.LegalEntityRef, document.Currency, document.ChannelRef)
	if err != nil {
		return none, err
	}
	amount, err := domain.NewMoney(ledger.Currency(), document.AmountMinor)
	if err != nil {
		return none, err
	}
	instruction, err := domain.IssueCollectionInstruction(domain.CollectionInstructionSpec{
		ID:           id,
		Parcel:       parcel,
		Requirement:  requirement,
		Ledger:       ledger,
		Amount:       amount,
		InstructedAt: document.InstructedAt,
	})
	if err != nil {
		return none, err
	}
	return application.RegisterInstructionCommand{Tenant: tenant, Instruction: instruction}, nil
}

type subledgerDocument struct {
	TenantID        string    `json:"tenantId"`
	CustomerRef     string    `json:"customerRef"`
	LegalEntityRef  string    `json:"legalEntityRef"`
	Currency        string    `json:"currency"`
	ChannelRef      string    `json:"channelRef"`
	CustodyBasisRef string    `json:"custodyBasisRef"`
	OpenedAt        time.Time `json:"openedAt"`
}

func subledgerCommandFromJSON(raw []byte) (application.OpenSubledgerCommand, error) {
	none := application.OpenSubledgerCommand{}
	var document subledgerDocument
	if err := decodeStrict(raw, &document, "分户账开立"); err != nil {
		return none, err
	}

	tenant, err := tenantFrom(document.TenantID)
	if err != nil {
		return none, err
	}
	key, err := ledgerKeyFrom(
		document.CustomerRef, document.LegalEntityRef, document.Currency, document.ChannelRef)
	if err != nil {
		return none, err
	}
	custody, err := domain.NewCustodyBasisReference(document.CustodyBasisRef)
	if err != nil {
		return none, err
	}
	ledger, err := domain.OpenSubledger(key, custody, document.OpenedAt)
	if err != nil {
		return none, err
	}
	return application.OpenSubledgerCommand{Tenant: tenant, Ledger: ledger}, nil
}

type factDocument struct {
	TenantID       string    `json:"tenantId"`
	FactRef        string    `json:"factRef"`
	InstructionRef string    `json:"instructionRef"`
	SourceLayer    string    `json:"sourceLayer"`
	EvidenceRef    string    `json:"evidenceRef"`
	Currency       string    `json:"currency"`
	AmountMinor    int64     `json:"amountMinor"`
	OccurredAt     time.Time `json:"occurredAt"`
}

func factCommandFromJSON(raw []byte) (application.AcceptFactCommand, error) {
	none := application.AcceptFactCommand{}
	var document factDocument
	if err := decodeStrict(raw, &document, "代收事实"); err != nil {
		return none, err
	}

	tenant, err := tenantFrom(document.TenantID)
	if err != nil {
		return none, err
	}
	id, err := domain.NewCollectionFactID(document.FactRef)
	if err != nil {
		return none, err
	}
	instruction, err := domain.NewCollectionInstructionID(document.InstructionRef)
	if err != nil {
		return none, err
	}
	// 陌生来源层级在这里就拒，不折成任何一层：四层不得互相推导那条，靠的正是
	// 「不认识就说不认识」。
	layer, known := domain.ParseCollectionSourceLayer(document.SourceLayer)
	if !known {
		return none, fmt.Errorf("未知来源层级 %q", document.SourceLayer)
	}
	evidence, err := domain.NewEvidenceReference(document.EvidenceRef)
	if err != nil {
		return none, err
	}
	amount, err := moneyFrom(document.Currency, document.AmountMinor)
	if err != nil {
		return none, err
	}
	fact, err := domain.AcceptCollectionFact(domain.CollectionFactSpec{
		ID:          id,
		Instruction: instruction,
		Layer:       layer,
		Evidence:    evidence,
		Amount:      amount,
		OccurredAt:  document.OccurredAt,
	})
	if err != nil {
		return none, err
	}
	return application.AcceptFactCommand{Tenant: tenant, Fact: fact}, nil
}

type discrepancyDocument struct {
	TenantID       string    `json:"tenantId"`
	DiscrepancyRef string    `json:"discrepancyRef"`
	InstructionRef string    `json:"instructionRef"`
	Kind           string    `json:"kind"`
	Currency       string    `json:"currency"`
	AmountMinor    int64     `json:"amountMinor"`
	BasisRef       string    `json:"basisRef"`
	ObservedAt     time.Time `json:"observedAt"`
}

func discrepancyCommandFromJSON(raw []byte) (application.RegisterDiscrepancyCommand, error) {
	none := application.RegisterDiscrepancyCommand{}
	var document discrepancyDocument
	if err := decodeStrict(raw, &document, "差异事项"); err != nil {
		return none, err
	}

	tenant, err := tenantFrom(document.TenantID)
	if err != nil {
		return none, err
	}
	id, err := domain.NewDiscrepancyItemID(document.DiscrepancyRef)
	if err != nil {
		return none, err
	}
	instruction, err := domain.NewCollectionInstructionID(document.InstructionRef)
	if err != nil {
		return none, err
	}
	kind, known := domain.ParseDiscrepancyKind(document.Kind)
	if !known {
		return none, fmt.Errorf("未知差异方向 %q", document.Kind)
	}
	amount, err := moneyFrom(document.Currency, document.AmountMinor)
	if err != nil {
		return none, err
	}
	basis, err := domain.NewBasisReference(document.BasisRef)
	if err != nil {
		return none, err
	}
	item, err := domain.RegisterDiscrepancyItem(domain.DiscrepancyItemSpec{
		ID:          id,
		Instruction: instruction,
		Kind:        kind,
		Amount:      amount,
		Basis:       basis,
		ObservedAt:  document.ObservedAt,
	})
	if err != nil {
		return none, err
	}
	return application.RegisterDiscrepancyCommand{Tenant: tenant, Item: item}, nil
}

type batchDocument struct {
	TenantID         string    `json:"tenantId"`
	BatchRef         string    `json:"batchRef"`
	CustomerRef      string    `json:"customerRef"`
	LegalEntityRef   string    `json:"legalEntityRef"`
	Currency         string    `json:"currency"`
	ChannelRef       string    `json:"channelRef"`
	CollectedThrough time.Time `json:"collectedThrough"`
	FormedAt         time.Time `json:"formedAt"`
}

// batchCommandFromJSON 不收状态字段：批次一律以`已归集`进册，交出汇付主张是另一条
// 命令。收状态就等于允许输入直接造一个「已交出」的批次，而那一格的意思是主张已经
// 交出去了。
func batchCommandFromJSON(raw []byte) (application.FormBatchCommand, error) {
	none := application.FormBatchCommand{}
	var document batchDocument
	if err := decodeStrict(raw, &document, "回汇批次"); err != nil {
		return none, err
	}

	tenant, err := tenantFrom(document.TenantID)
	if err != nil {
		return none, err
	}
	id, err := domain.NewRemittanceBatchID(document.BatchRef)
	if err != nil {
		return none, err
	}
	key, err := ledgerKeyFrom(
		document.CustomerRef, document.LegalEntityRef, document.Currency, document.ChannelRef)
	if err != nil {
		return none, err
	}
	batch, err := domain.FormRemittanceBatch(domain.RemittanceBatchSpec{
		ID:               id,
		Ledger:           key,
		CollectedThrough: document.CollectedThrough,
		FormedAt:         document.FormedAt,
	})
	if err != nil {
		return none, err
	}
	return application.FormBatchCommand{Tenant: tenant, Batch: batch}, nil
}

type handOverDocument struct {
	TenantID string `json:"tenantId"`
	BatchRef string `json:"batchRef"`
}

func handOverCommandFromJSON(raw []byte) (application.HandOverBatchCommand, error) {
	none := application.HandOverBatchCommand{}
	var document handOverDocument
	if err := decodeStrict(raw, &document, "汇付主张交出"); err != nil {
		return none, err
	}

	tenant, err := tenantFrom(document.TenantID)
	if err != nil {
		return none, err
	}
	id, err := domain.NewRemittanceBatchID(document.BatchRef)
	if err != nil {
		return none, err
	}
	return application.HandOverBatchCommand{Tenant: tenant, Batch: id}, nil
}

type postingDocument struct {
	TenantID     string    `json:"tenantId"`
	PostingRef   string    `json:"postingRef"`
	FromPosition string    `json:"fromPosition"`
	ToPosition   string    `json:"toPosition"`
	AmountMinor  int64     `json:"amountMinor"`
	BasisKind    string    `json:"basisKind"`
	BasisRef     string    `json:"basisRef"`
	PostedAt     time.Time `json:"postedAt"`
}

func postingCommandFromJSON(raw []byte) (application.PostCommand, error) {
	none := application.PostCommand{}
	var document postingDocument
	if err := decodeStrict(raw, &document, "分户账记账"); err != nil {
		return none, err
	}

	tenant, err := tenantFrom(document.TenantID)
	if err != nil {
		return none, err
	}
	id, err := domain.NewPostingID(document.PostingRef)
	if err != nil {
		return none, err
	}
	from, known := domain.ParseFundPosition(document.FromPosition)
	if !known {
		return none, fmt.Errorf("未知资金位置 %q", document.FromPosition)
	}
	to, known := domain.ParseFundPosition(document.ToPosition)
	if !known {
		return none, fmt.Errorf("未知资金位置 %q", document.ToPosition)
	}
	basisKind, known := domain.ParsePostingBasisKind(document.BasisKind)
	if !known {
		return none, fmt.Errorf("未知依据种类 %q", document.BasisKind)
	}
	basis, err := domain.NewBasisReference(document.BasisRef)
	if err != nil {
		return none, err
	}
	return application.PostCommand{
		Tenant:      tenant,
		ID:          id,
		From:        from,
		To:          to,
		AmountMinor: document.AmountMinor,
		BasisKind:   basisKind,
		Basis:       basis,
		PostedAt:    document.PostedAt,
	}, nil
}

func ledgerKeyFrom(customerRaw, entityRaw, currencyRaw, channelRaw string) (domain.SubledgerKey, error) {
	customer, err := domain.NewCustomerReference(customerRaw)
	if err != nil {
		return domain.SubledgerKey{}, err
	}
	entity, err := domain.NewLegalEntityReference(entityRaw)
	if err != nil {
		return domain.SubledgerKey{}, err
	}
	currency, err := domain.NewCurrencyCode(currencyRaw)
	if err != nil {
		return domain.SubledgerKey{}, err
	}
	channel, err := domain.NewCollectionChannelReference(channelRaw)
	if err != nil {
		return domain.SubledgerKey{}, err
	}
	return domain.NewSubledgerKey(customer, entity, currency, channel)
}

func moneyFrom(currencyRaw string, amountMinor int64) (domain.Money, error) {
	currency, err := domain.NewCurrencyCode(currencyRaw)
	if err != nil {
		return domain.Money{}, err
	}
	return domain.NewMoney(currency, amountMinor)
}
