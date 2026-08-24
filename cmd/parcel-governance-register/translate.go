package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"time"

	"go.idp.xyz/idp-parcel/internal/pilotgovernance/domain"
)

// 本文件把三种登记输入 JSON 折成领域规格。翻译严格且零默认：未知字段拒收（打错
// 字段名不得静默变成「没给」）、有构造门的标识在这里就拒、其余内容原样递给领域门
// ——九件缺一、恢复四件那类判据在域，这里绝不代填。
//
// 输入里没有任何通道技术身份字段：那是身份双轨的第①轨，由入口自取（见 main.go
// 的 currentChannelIdentity），不可由参数传入或覆盖；这里翻译的 executedBy /
// decidedBy 是第②轨——登记内容，册面语义是「登记者声明了谁」。

type authorityIntervalDocument struct {
	ObjectScope string     `json:"objectScope"`
	Capability  string     `json:"capability"`
	FactKind    string     `json:"factKind"`
	Authority   string     `json:"authority"`
	FromAt      time.Time  `json:"fromAt"`
	ToAt        *time.Time `json:"toAt,omitempty"`
}

func authorityIntervalFromJSON(raw []byte) (domain.AuthorityInterval, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var document authorityIntervalDocument
	if err := decoder.Decode(&document); err != nil {
		return domain.AuthorityInterval{}, fmt.Errorf("权威区间输入不是本入口的形状：%w", err)
	}
	interval := domain.AuthorityInterval{
		ObjectScope: document.ObjectScope,
		Capability:  document.Capability,
		FactKind:    document.FactKind,
		Authority:   document.Authority,
		From:        document.FromAt,
	}
	if document.ToAt != nil {
		interval.To = *document.ToAt
	}
	return interval, nil
}

type suspensionDocument struct {
	SuspensionID  string    `json:"suspensionId"`
	TriggerSource string    `json:"triggerSource"`
	Basis         string    `json:"basis"`
	Evidence      string    `json:"evidence"`
	Scope         string    `json:"scope"`
	ExecutedBy    string    `json:"executedBy"`
	OccurredAt    time.Time `json:"occurredAt"`
	EffectiveAt   time.Time `json:"effectiveAt"`
	InTransitNote string    `json:"inTransitNote"`
}

func suspensionSpecFromJSON(raw []byte) (domain.SuspensionDecisionSpec, error) {
	none := domain.SuspensionDecisionSpec{}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var document suspensionDocument
	if err := decoder.Decode(&document); err != nil {
		return none, fmt.Errorf("暂停输入不是本入口的形状：%w", err)
	}

	spec := domain.SuspensionDecisionSpec{
		TriggerSource: document.TriggerSource,
		Basis:         document.Basis,
		Evidence:      document.Evidence,
		ExecutedBy:    document.ExecutedBy,
		OccurredAt:    document.OccurredAt,
		EffectiveAt:   document.EffectiveAt,
		InTransitNote: document.InTransitNote,
	}
	var err error
	if spec.ID, err = domain.NewSuspensionID(document.SuspensionID); err != nil {
		return none, err
	}
	if spec.Scope, err = domain.NewScopeVersionReference(document.Scope); err != nil {
		return none, err
	}
	return spec, nil
}

type inventoryEntryDocument struct {
	ObjectIdentity   string    `json:"objectIdentity"`
	CurrentFacts     string    `json:"currentFacts"`
	CurrentAuthority string    `json:"currentAuthority"`
	ResponsibleParty string    `json:"responsibleParty"`
	NextAction       string    `json:"nextAction"`
	ReviewBy         time.Time `json:"reviewBy"`
}

type inventoryDocument struct {
	TakenAt time.Time                `json:"takenAt"`
	Entries []inventoryEntryDocument `json:"entries"`
}

type resumptionDocument struct {
	SuspensionID     string            `json:"suspensionId"`
	ReleaseEvidence  string            `json:"releaseEvidence"`
	ConsistencyCheck string            `json:"consistencyCheck"`
	Inventory        inventoryDocument `json:"inventory"`
	DecidedBy        string            `json:"decidedBy"`
	DecidedAt        time.Time         `json:"decidedAt"`
	EffectiveAt      time.Time         `json:"effectiveAt"`
}

func resumptionSpecFromJSON(raw []byte) (domain.ResumptionDecisionSpec, error) {
	none := domain.ResumptionDecisionSpec{}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var document resumptionDocument
	if err := decoder.Decode(&document); err != nil {
		return none, fmt.Errorf("恢复输入不是本入口的形状：%w", err)
	}

	entries := make([]domain.InventoryEntry, 0, len(document.Inventory.Entries))
	for _, entry := range document.Inventory.Entries {
		entries = append(entries, domain.InventoryEntry{
			ObjectIdentity:   entry.ObjectIdentity,
			CurrentFacts:     entry.CurrentFacts,
			CurrentAuthority: entry.CurrentAuthority,
			ResponsibleParty: entry.ResponsibleParty,
			NextAction:       entry.NextAction,
			ReviewBy:         entry.ReviewBy,
		})
	}
	// 盘点门在翻译处就过：条目六件齐全、身份不重、盘点时刻在场——恢复四件里的
	// 这一件不给任何「只填标识」的简化路径。
	inventory, err := domain.TakeInventory(entries, document.Inventory.TakenAt)
	if err != nil {
		return none, err
	}

	spec := domain.ResumptionDecisionSpec{
		ReleaseEvidence:  document.ReleaseEvidence,
		ConsistencyCheck: document.ConsistencyCheck,
		Inventory:        inventory,
		DecidedBy:        document.DecidedBy,
		DecidedAt:        document.DecidedAt,
		EffectiveAt:      document.EffectiveAt,
	}
	if spec.Suspension, err = domain.NewSuspensionID(document.SuspensionID); err != nil {
		return none, err
	}
	return spec, nil
}
