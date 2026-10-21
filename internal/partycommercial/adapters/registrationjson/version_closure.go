package registrationjson

import (
	"encoding/json"
	"fmt"
	"time"

	pcapplication "go.idp.xyz/idp-parcel/internal/partycommercial/application"
	pcdomain "go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

// VersionClosureBatchDocument 是按后继收尾旧版的批文外壳（ADR-0233，票 demo-chain-gaps/50）：受控批量口
// register-version-closures 与在线两口（`/commercial-version-successor-cuts`、`/commercial-version-retirements`）共用。
// 截止与退役分两节，形照 RegistrationNumberTypeBatchDocument 的登记与停用：在线口各收自己那一节恰一项。TenantID 的两种
// 用法同 ProductBatchDocument。
type VersionClosureBatchDocument struct {
	TenantID    json.RawMessage               `json:"tenantId"`
	Cuts        []SuccessorCutDocument        `json:"cuts,omitempty"`
	Retirements []SameStartRetirementDocument `json:"retirements,omitempty"`
}

// VersionClosureTargetDocument 是两节共用的那一半：哪一范围、哪一对象的哪一版，让位给同一对象的哪一版。对象只写一次。
type VersionClosureTargetDocument struct {
	Kind             string `json:"kind"`
	Scope            string `json:"scope"`
	ObjectID         string `json:"objectId"`
	Version          string `json:"version"`
	SuccessorVersion string `json:"successorVersion"`
}

// SuccessorCutDocument 是一次截止。截止点不在形状里——理由在 application.CutCommercialVersionAtSuccessorCommand。
type SuccessorCutDocument struct {
	VersionClosureTargetDocument
	Reference   string    `json:"reference"`
	CorrectedAt time.Time `json:"correctedAt"`
}

// SameStartRetirementDocument 是一次同起点退役。没有退役时刻一格：它取登记时刻，载荷里带了即按未知键拒。
type SameStartRetirementDocument struct {
	VersionClosureTargetDocument
	Reference string `json:"reference"`
}

// SuccessorCutCommand 把一项译成截止命令。只过形状与值对象构造门；两版在不在册、生没生效、截止点落不落在区间里，
// 由用例对着整册判。更正时刻缺席即零时刻，照原样交给领域去拒，不补成此刻——重放靠它逐字相同来认。
func SuccessorCutCommand(
	tenant pcdomain.TenantID,
	item SuccessorCutDocument,
) (pcapplication.CutCommercialVersionAtSuccessorCommand, error) {
	target, err := closureTargetFrom(tenant, item.VersionClosureTargetDocument)
	if err != nil {
		return pcapplication.CutCommercialVersionAtSuccessorCommand{}, err
	}
	reference, err := pcdomain.NewValidityCorrectionReference(item.Reference)
	if err != nil {
		return pcapplication.CutCommercialVersionAtSuccessorCommand{}, fmt.Errorf("reference: %w", err)
	}
	return pcapplication.CutCommercialVersionAtSuccessorCommand{
		VersionClosureTarget: target,
		Reference:            reference,
		CorrectedAt:          item.CorrectedAt,
	}, nil
}

// SameStartRetirementCommand 把一项译成同起点退役命令，判据同 SuccessorCutCommand。
func SameStartRetirementCommand(
	tenant pcdomain.TenantID,
	item SameStartRetirementDocument,
) (pcapplication.RetireCommercialVersionAtSameStartCommand, error) {
	target, err := closureTargetFrom(tenant, item.VersionClosureTargetDocument)
	if err != nil {
		return pcapplication.RetireCommercialVersionAtSameStartCommand{}, err
	}
	reference, err := pcdomain.NewRetirementReference(item.Reference)
	if err != nil {
		return pcapplication.RetireCommercialVersionAtSameStartCommand{}, fmt.Errorf("reference: %w", err)
	}
	return pcapplication.RetireCommercialVersionAtSameStartCommand{VersionClosureTarget: target, Reference: reference}, nil
}

func closureTargetFrom(tenant pcdomain.TenantID, item VersionClosureTargetDocument) (pcapplication.VersionClosureTarget, error) {
	var none pcapplication.VersionClosureTarget
	kind, err := CommercialKindFrom(item.Kind)
	if err != nil {
		return none, fmt.Errorf("kind: %w", err)
	}
	scope, err := pcdomain.NewCommercialScopeReference(item.Scope)
	if err != nil {
		return none, fmt.Errorf("scope: %w", err)
	}
	objectID, err := pcdomain.NewCommercialObjectID(item.ObjectID)
	if err != nil {
		return none, fmt.Errorf("objectId: %w", err)
	}
	version, err := pcdomain.NewCommercialVersionLabel(item.Version)
	if err != nil {
		return none, fmt.Errorf("version: %w", err)
	}
	successor, err := pcdomain.NewCommercialVersionLabel(item.SuccessorVersion)
	if err != nil {
		return none, fmt.Errorf("successorVersion: %w", err)
	}
	return pcapplication.VersionClosureTarget{
		Tenant: tenant, Scope: scope, Kind: kind, ObjectID: objectID, Version: version, Successor: successor,
	}, nil
}
