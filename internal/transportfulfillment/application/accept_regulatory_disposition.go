// accept_regulatory_disposition.go 编排 UC-TF-001 的承接入口：监管运输协作事项的
// 三格承接决定。判断形状在领域（三格各守其形、移动授权门），这里只做受理、纯节点
// 分流、幂等与回执意图；旅程建立不在此——承接决定产出的监管依据引用由
// StartAlternateJourneyHandler 消费，那条监管双链不重建。
package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// ErrUnexpectedDispositionSave 说明承接库交回了封闭集合以外的写入结果。
var ErrUnexpectedDispositionSave = errors.New("transport fulfillment: unexpected disposition acceptance save outcome")

// AcceptDispositionOutcome 是承接提交的应用处理结果。`纯节点动作`与`待补充`是业务
// 答复不是失败：前者的续办在 UC-NO-001，后者的续办是把授权或对象补齐（ADR-0029，
// 恢复动作不同的结果不共格）。
type AcceptDispositionOutcome uint8

const (
	AcceptDispositionOutcomeInvalid AcceptDispositionOutcome = iota
	DispositionDecided
	DispositionExistingDecision
	DispositionDecisionConflict
	DispositionNodeOnly
	DispositionSupplementRequired
	AcceptDispositionNotAccepted
	AcceptDispositionUndecided
)

// SupplementReason 指名待补充缺的是协作事项的哪一项（ADR-0265 决定四）。续办都是协作事项再交，
// 但关务要知道去补哪一样；答复只带一个原因，按 itemSupplementReason 与领域门的顺序报第一项缺口。
type SupplementReason uint8

const (
	SupplementReasonNone SupplementReason = iota
	SupplementMovementAuthorityMissing
	SupplementObjectMissing
	SupplementMovementActionMissing
	SupplementResponsibilityMissing
)

func (reason SupplementReason) String() string {
	switch reason {
	case SupplementMovementAuthorityMissing:
		return "MOVEMENT_AUTHORITY_MISSING"
	case SupplementObjectMissing:
		return "OBJECT_MISSING"
	case SupplementMovementActionMissing:
		return "MOVEMENT_ACTION_MISSING"
	case SupplementResponsibilityMissing:
		return "RESPONSIBILITY_MISSING"
	default:
		return ""
	}
}

func (outcome AcceptDispositionOutcome) String() string {
	switch outcome {
	case DispositionDecided:
		return "DISPOSITION_DECIDED"
	case DispositionExistingDecision:
		return "EXISTING_DECISION"
	case DispositionDecisionConflict:
		return "DECISION_CONFLICT"
	case DispositionNodeOnly:
		return "NODE_ONLY_DISPOSITION"
	case DispositionSupplementRequired:
		return "SUPPLEMENT_REQUIRED"
	case AcceptDispositionNotAccepted:
		return "NOT_ACCEPTED"
	case AcceptDispositionUndecided:
		return "UNDECIDED"
	default:
		return ""
	}
}

// AcceptDispositionUndecidedReason 指名提交停在哪一步。
type AcceptDispositionUndecidedReason uint8

const (
	AcceptDispositionUndecidedReasonNone AcceptDispositionUndecidedReason = iota
	DispositionAcceptanceStoreUnavailable
)

func (reason AcceptDispositionUndecidedReason) String() string {
	switch reason {
	case DispositionAcceptanceStoreUnavailable:
		return "DISPOSITION_ACCEPTANCE_STORE_UNAVAILABLE"
	default:
		return ""
	}
}

// AcceptRegulatoryDispositionCommand 携带一次承接决定的全部输入。MovementAction 取自
// 协作事项声明的移动动作；动作与起止都为空即纯节点动作，本用例不承接（UC-TF-001：「原地查验、
// 原地扣留、节点开封……由 UC-NO-001 承接，不建立运输履约」）。
//
// ItemObjects、Origin、Destination 是协作事项本体里承接要核的几格（载运对象、起止责任），由提交
// 的运营人员据关务协作事项抄录：关务一侧今天没有按事项引用读回本体的提供方，读口另议。它们
// 只是承接的前提，不进决定记录与内容摘要。DecidedBy 是作出承接决定的操作者，只进审计
// （ADR-0303 的待遇）：不进重放比对，也不进任何门。
type AcceptRegulatoryDispositionCommand struct {
	TenantID          domain.TenantID
	Item              domain.CollaborationItemReference
	Basis             domain.DispositionBasisReference
	MovementAction    string
	ItemObjects       []string
	Origin            string
	Destination       string
	Decision          domain.DispositionAcceptanceKind
	AcceptedObjects   []string
	DeclineBasis      string
	MovementAuthority string
	DecidedAt         time.Time
	DecidedBy         string
}

type AcceptDispositionResult struct {
	outcome    AcceptDispositionOutcome
	reason     AcceptDispositionUndecidedReason
	supplement SupplementReason
	record     ports.DispositionAcceptanceRecord
	hasRecord  bool
	handoffRef string
}

func (result AcceptDispositionResult) Outcome() AcceptDispositionOutcome {
	return result.outcome
}

func (result AcceptDispositionResult) UndecidedReason() AcceptDispositionUndecidedReason {
	return result.reason
}

// SupplementReason 只在`待补充`时非零。
func (result AcceptDispositionResult) SupplementReason() SupplementReason {
	return result.supplement
}

// Acceptance 只在决定成立（本轮或此前）时给出。
func (result AcceptDispositionResult) Acceptance() (ports.DispositionAcceptanceRecord, bool) {
	return result.record, result.hasRecord
}

// HandoffReference 非空说明承接回执还没交出去，重放会重发同一份。
func (result AcceptDispositionResult) HandoffReference() string {
	return result.handoffRef
}

type AcceptRegulatoryDispositionDeps struct {
	Acceptances ports.DispositionAcceptanceStore
	Receipt     ports.RegulatoryAcceptanceHandoff
	Clock       ports.Clock
}

type AcceptRegulatoryDispositionHandler struct {
	deps AcceptRegulatoryDispositionDeps
}

func NewAcceptRegulatoryDispositionHandler(
	deps AcceptRegulatoryDispositionDeps,
) *AcceptRegulatoryDispositionHandler {
	return &AcceptRegulatoryDispositionHandler{deps: deps}
}

// Handle 把一项监管运输协作推进到承接决定：受理（租户/事项/监管依据/决定时间缺一即
// 未受理）→ 纯节点动作分流（不建任何运输对象）→ 幂等按（租户+事项）一事项一决定，
// 同键同内容重放返原并重发回执，同键异内容是冒名冲突不顶替 → 领域三格把门（缺移动
// 授权是待补充不是形状错——扣留决定本身不是移动授权）→ 提交与回执意图。承接不建
// 旅程：已承接范围的旅程由 StartAlternateJourneyHandler 凭本决定携带的监管依据引用
// 另行启动。
func (handler *AcceptRegulatoryDispositionHandler) Handle(
	ctx context.Context,
	command AcceptRegulatoryDispositionCommand,
) (AcceptDispositionResult, error) {
	if command.TenantID.String() == "" ||
		command.Item.String() == "" ||
		command.Basis.String() == "" ||
		command.DecidedAt.IsZero() {
		return AcceptDispositionResult{outcome: AcceptDispositionNotAccepted}, nil
	}
	if strings.TrimSpace(command.MovementAction) == "" &&
		strings.TrimSpace(command.Origin) == "" &&
		strings.TrimSpace(command.Destination) == "" {
		// 事项没有真实移动动作、也没有起止：原地查验、原地扣留与节点处置归 UC-NO-001，这里连
		// 运输对象都不建。只有起止而没有动作的事项不在此列——它声称要移动，缺的是动作，
		// 走待补充。
		return AcceptDispositionResult{outcome: DispositionNodeOnly}, nil
	}
	decidedBy, err := domain.OptionalRegistrantReference(command.DecidedBy)
	if err != nil {
		return AcceptDispositionResult{outcome: AcceptDispositionNotAccepted}, nil
	}

	key := ports.DispositionAcceptanceKey{Tenant: command.TenantID, Item: command.Item}
	_, digest, err := domain.CanonicalizeRegulatoryDispositionPayload(
		command.Basis.String(),
		command.MovementAction,
		command.Decision,
		command.AcceptedObjects,
		command.DeclineBasis,
		command.MovementAuthority,
		command.DecidedAt,
	)
	if err != nil {
		return AcceptDispositionResult{}, err
	}
	existing, found, err := handler.deps.Acceptances.FindByKey(ctx, key)
	if err != nil {
		return AcceptDispositionResult{outcome: AcceptDispositionUndecided, reason: DispositionAcceptanceStoreUnavailable}, nil
	}
	unversioned := unversionedDispositionDigest(command)
	if found {
		switch domain.CompareStoredDigest(existing.ContentDigest, digest, unversioned) {
		case domain.DifferentPayload:
			// 同一事项携带不同决定内容：改决定走事项方的更正/替代，不经承接入口顶替。
			return AcceptDispositionResult{outcome: DispositionDecisionConflict}, nil
		case domain.UnknownPayloadShape:
			return AcceptDispositionResult{outcome: AcceptDispositionUndecided, reason: DispositionAcceptanceStoreUnavailable}, nil
		}
		return AcceptDispositionResult{
			outcome:    DispositionExistingDecision,
			record:     existing,
			hasRecord:  true,
			handoffRef: handler.handOffReceipt(ctx, existing),
		}, nil
	}

	// 协作事项本体的核对排在重放判定之后：同一事项已有决定时，事项后来怎么变不改变它已落的决定
	// （改决定走事项方的更正或替代）。
	if reason := itemSupplementReason(command); reason != SupplementReasonNone {
		return AcceptDispositionResult{outcome: DispositionSupplementRequired, supplement: reason}, nil
	}
	decision, result, formed := handler.form(command)
	if !formed {
		return result, nil
	}

	record := ports.DispositionAcceptanceRecord{Key: key, ContentDigest: digest, Decision: decision, DecidedBy: decidedBy}
	saved, err := handler.deps.Acceptances.Save(ctx, record)
	if err != nil {
		return AcceptDispositionResult{outcome: AcceptDispositionUndecided, reason: DispositionAcceptanceStoreUnavailable}, nil
	}
	switch saved {
	case ports.DispositionAcceptanceSaved:
		return AcceptDispositionResult{
			outcome:    DispositionDecided,
			record:     record,
			hasRecord:  true,
			handoffRef: handler.handOffReceipt(ctx, record),
		}, nil
	case ports.DispositionAcceptanceAlreadyRecorded:
		// 并发输家读回赢家：谁先越过提交边界谁是决定，本轮内容一致则等价重放，不一致
		// 由下一轮 FindByKey 答冲突。
		winner, found, err := handler.deps.Acceptances.FindByKey(ctx, key)
		if err != nil || !found {
			return AcceptDispositionResult{outcome: AcceptDispositionUndecided, reason: DispositionAcceptanceStoreUnavailable}, nil
		}
		switch domain.CompareStoredDigest(winner.ContentDigest, digest, unversioned) {
		case domain.DifferentPayload:
			return AcceptDispositionResult{outcome: DispositionDecisionConflict}, nil
		case domain.UnknownPayloadShape:
			return AcceptDispositionResult{outcome: AcceptDispositionUndecided, reason: DispositionAcceptanceStoreUnavailable}, nil
		}
		return AcceptDispositionResult{
			outcome:    DispositionExistingDecision,
			record:     winner,
			hasRecord:  true,
			handoffRef: handler.handOffReceipt(ctx, winner),
		}, nil
	default:
		return AcceptDispositionResult{}, fmt.Errorf("%w: %d", ErrUnexpectedDispositionSave, saved)
	}
}

// itemSupplementReason 核协作事项本体够不够承接（UC-TF-001 步骤 3、ADR-0265 决定四）：缺动作、
// 缺对象、缺起止责任，按此序报第一项；缺移动授权由领域门在 form 里答。拒绝一样要事项立得住——
// 一件缺对象的事项连拒绝的范围都指不出来。
func itemSupplementReason(command AcceptRegulatoryDispositionCommand) SupplementReason {
	if strings.TrimSpace(command.MovementAction) == "" {
		return SupplementMovementActionMissing
	}
	hasObject := false
	for _, object := range command.ItemObjects {
		if strings.TrimSpace(object) != "" {
			hasObject = true
		}
	}
	if !hasObject {
		return SupplementObjectMissing
	}
	if strings.TrimSpace(command.Origin) == "" || strings.TrimSpace(command.Destination) == "" {
		return SupplementResponsibilityMissing
	}
	return SupplementReasonNone
}

// form 把命令译成领域承接决定。缺移动授权落待补充格（续办是补授权，恢复引用即事项
// 本身）；其余领域拒绝是形状错，答未受理。
func (handler *AcceptRegulatoryDispositionHandler) form(
	command AcceptRegulatoryDispositionCommand,
) (domain.RegulatoryTransportDisposition, AcceptDispositionResult, bool) {
	inItem := make(map[string]struct{}, len(command.ItemObjects))
	for _, raw := range command.ItemObjects {
		inItem[strings.TrimSpace(raw)] = struct{}{}
	}
	objects := make([]domain.CarriedObjectReference, 0, len(command.AcceptedObjects))
	for _, raw := range command.AcceptedObjects {
		// 承接的对象不得超出事项指名的范围：事项说了算，运输方不扩大移动范围（UC-TF-001
		// 「不重新解释监管原文或扩大移动范围」）。
		if _, within := inItem[strings.TrimSpace(raw)]; !within {
			return domain.RegulatoryTransportDisposition{},
				AcceptDispositionResult{outcome: AcceptDispositionNotAccepted}, false
		}
		object, err := domain.NewCarriedObjectReference(raw)
		if err != nil {
			return domain.RegulatoryTransportDisposition{},
				AcceptDispositionResult{outcome: AcceptDispositionNotAccepted}, false
		}
		objects = append(objects, object)
	}
	var authority domain.MovementAuthorityReference
	if strings.TrimSpace(command.MovementAuthority) != "" {
		built, err := domain.NewMovementAuthorityReference(command.MovementAuthority)
		if err != nil {
			return domain.RegulatoryTransportDisposition{},
				AcceptDispositionResult{outcome: AcceptDispositionNotAccepted}, false
		}
		authority = built
	}

	decision, err := domain.FormRegulatoryTransportDisposition(domain.RegulatoryTransportDispositionSpec{
		Tenant:            command.TenantID,
		Item:              command.Item,
		Basis:             command.Basis,
		Kind:              command.Decision,
		AcceptedObjects:   objects,
		DeclineBasis:      command.DeclineBasis,
		MovementAuthority: authority,
		DecidedAt:         command.DecidedAt,
	})
	switch {
	case err == nil:
		return decision, AcceptDispositionResult{}, true
	case errors.Is(err, domain.ErrMovementAuthorityMissing):
		return domain.RegulatoryTransportDisposition{},
			AcceptDispositionResult{outcome: DispositionSupplementRequired, supplement: SupplementMovementAuthorityMissing}, false
	default:
		return domain.RegulatoryTransportDisposition{},
			AcceptDispositionResult{outcome: AcceptDispositionNotAccepted}, false
	}
}

// handOffReceipt 把承接决定回执给关务协作链，交不出去时交回发布续办引用（ADR-0043）。
func (handler *AcceptRegulatoryDispositionHandler) handOffReceipt(
	ctx context.Context,
	record ports.DispositionAcceptanceRecord,
) string {
	if err := handler.deps.Receipt.HandOffRegulatoryAcceptance(ctx, ports.RegulatoryAcceptanceHandoffIntent{
		Record: record,
	}); err != nil {
		return "CONT-" + dispositionReceiptDigest(record.Key)
	}
	return ""
}

func dispositionReceiptDigest(key ports.DispositionAcceptanceKey) string {
	digest := sha256.Sum256([]byte("REGULATORY_ACCEPTANCE\x00" + key.Tenant.String() + "\x00" + key.Item.String()))
	return hex.EncodeToString(digest[:8])
}
