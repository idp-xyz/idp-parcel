package registrationjson

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	pcapplication "go.idp.xyz/idp-parcel/internal/partycommercial/application"
	pcdomain "go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

// PartyRelationshipEndingBatchDocument 是参与方关系结束的批文外壳（票 ps-external-mark-relations/17）：受控批量口
// end-party-relationships 与在线三口（`/commercial-party-relationship-revocations`、`-expirations`、`-supersessions`）共用。
// 撤销、到期、替代分三节，形照 VersionClosureBatchDocument：在线口各收自己那一节恰一项。TenantID 的两种用法同
// ProductBatchDocument。
//
// 不并进参与方登记批（register-parties 的外壳）：登记与结束在册上是不同方向的修订，合成一份外壳会让「立起一段关系」与
// 「结束这段关系」躺在同一个文档里，判据同停用批单列。
type PartyRelationshipEndingBatchDocument struct {
	TenantID      json.RawMessage                         `json:"tenantId"`
	Revocations   []PartyRelationshipRevocationDocument   `json:"revocations,omitempty"`
	Expirations   []PartyRelationshipExpirationDocument   `json:"expirations,omitempty"`
	Supersessions []PartyRelationshipSupersessionDocument `json:"supersessions,omitempty"`
}

// ItemCount 数外壳里三节的项。在线口一口只收本节的项：别节的数组在这份外壳里解得开，解得开不等于可以忽略。
func (document PartyRelationshipEndingBatchDocument) ItemCount() int {
	return len(document.Revocations) + len(document.Expirations) + len(document.Supersessions)
}

// PartyRelationshipRevocationDocument 是一次撤销。Revision 是终止修订的落点（册上最新修订 + 1），At 是撤销时刻。
type PartyRelationshipRevocationDocument struct {
	RelationshipID string    `json:"relationshipId"`
	Revision       int       `json:"revision"`
	Basis          string    `json:"basis"`
	At             time.Time `json:"at"`
}

// PartyRelationshipExpirationDocument 是一次到期登记，形同撤销：At 是到期时刻，不早于关系的区间终点。
type PartyRelationshipExpirationDocument struct {
	RelationshipID string    `json:"relationshipId"`
	Revision       int       `json:"revision"`
	Basis          string    `json:"basis"`
	At             time.Time `json:"at"`
}

// PartyRelationshipSupersessionDocument 是一次替代。接替关系的正文不在形状里——持有方是 successor，其余照原关系，
// 理由在 application.SupersedePartyRelationshipCommand；这里只给它的标识、依据与批准。批准必带：键缺席即拒。
type PartyRelationshipSupersessionDocument struct {
	RelationshipID          string                        `json:"relationshipId"`
	Revision                int                           `json:"revision"`
	Successor               string                        `json:"successor"`
	At                      time.Time                     `json:"at"`
	SuccessorRelationshipID string                        `json:"successorRelationshipId"`
	Basis                   string                        `json:"basis"`
	Approval                *RelationshipApprovalDocument `json:"approval"`
}

// RelationshipApprovalDocument 与参与方登记批里关系项的 approval 同形。
type RelationshipApprovalDocument struct {
	Reference  string    `json:"reference"`
	ApprovedAt time.Time `json:"approvedAt"`
}

// PartyRelationshipRevocationCommand 把一项译成撤销命令。只过形状与值对象构造门；关系在不在册、生没生效、修订对不对，由
// 用例对着册面判。时刻缺席即零时刻，照原样交给领域去拒，不补成此刻——重放靠它逐字相同来认。
func PartyRelationshipRevocationCommand(
	tenant pcdomain.TenantID,
	item PartyRelationshipRevocationDocument,
) (pcapplication.RevokePartyRelationshipCommand, error) {
	id, basis, err := endingTargetFrom(item.RelationshipID, item.Basis)
	if err != nil {
		return pcapplication.RevokePartyRelationshipCommand{}, err
	}
	return pcapplication.RevokePartyRelationshipCommand{
		Tenant: tenant, ID: id, Revision: item.Revision, Basis: basis, At: item.At,
	}, nil
}

// PartyRelationshipExpirationCommand 把一项译成到期命令，判据同 PartyRelationshipRevocationCommand。
func PartyRelationshipExpirationCommand(
	tenant pcdomain.TenantID,
	item PartyRelationshipExpirationDocument,
) (pcapplication.ExpirePartyRelationshipCommand, error) {
	id, basis, err := endingTargetFrom(item.RelationshipID, item.Basis)
	if err != nil {
		return pcapplication.ExpirePartyRelationshipCommand{}, err
	}
	return pcapplication.ExpirePartyRelationshipCommand{
		Tenant: tenant, ID: id, Revision: item.Revision, Basis: basis, At: item.At,
	}, nil
}

// PartyRelationshipSupersessionCommand 把一项译成替代命令，判据同 PartyRelationshipRevocationCommand；接替方在不在册、
// 接替关系立不立得住，由用例经关系登记的门判。
func PartyRelationshipSupersessionCommand(
	tenant pcdomain.TenantID,
	item PartyRelationshipSupersessionDocument,
) (pcapplication.SupersedePartyRelationshipCommand, error) {
	none := pcapplication.SupersedePartyRelationshipCommand{}
	id, basis, err := endingTargetFrom(item.RelationshipID, item.Basis)
	if err != nil {
		return none, err
	}
	successor, err := pcdomain.NewPartyID(item.Successor)
	if err != nil {
		return none, fmt.Errorf("successor: %w", err)
	}
	successorRelationship, err := pcdomain.NewRelationshipID(item.SuccessorRelationshipID)
	if err != nil {
		return none, fmt.Errorf("successorRelationshipId: %w", err)
	}
	if item.Approval == nil {
		return none, errors.New("approval: 替代必带接替关系的批准——原关系在替代时刻就不再支持新决定，接替关系只是候选会空出一段无人持有的时间")
	}
	reference, err := pcdomain.NewApprovalReference(item.Approval.Reference)
	if err != nil {
		return none, fmt.Errorf("approval.reference: %w", err)
	}
	return pcapplication.SupersedePartyRelationshipCommand{
		Tenant:                tenant,
		ID:                    id,
		Revision:              item.Revision,
		Successor:             successor,
		At:                    item.At,
		SuccessorRelationship: successorRelationship,
		Basis:                 basis,
		Approval: pcapplication.RelationshipApproval{
			Reference:  reference,
			ApprovedAt: item.Approval.ApprovedAt,
		},
	}, nil
}

func endingTargetFrom(rawID, rawBasis string) (pcdomain.RelationshipID, pcdomain.RelationshipBasisReference, error) {
	id, err := pcdomain.NewRelationshipID(rawID)
	if err != nil {
		return pcdomain.RelationshipID{}, pcdomain.RelationshipBasisReference{}, fmt.Errorf("relationshipId: %w", err)
	}
	basis, err := pcdomain.NewRelationshipBasisReference(rawBasis)
	if err != nil {
		return pcdomain.RelationshipID{}, pcdomain.RelationshipBasisReference{}, fmt.Errorf("basis: %w", err)
	}
	return id, basis, nil
}
