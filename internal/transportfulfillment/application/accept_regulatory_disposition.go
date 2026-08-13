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
	"sort"
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
// 协作事项声明的移动动作——为空即纯节点动作，本用例不承接（UC-TF-001：「原地查验、
// 原地扣留、节点开封……由 UC-NO-001 承接，不建立运输履约」）。
type AcceptRegulatoryDispositionCommand struct {
	TenantID          domain.TenantID
	Item              domain.CollaborationItemReference
	Basis             domain.DispositionBasisReference
	MovementAction    string
	Decision          domain.DispositionAcceptanceKind
	AcceptedObjects   []string
	DeclineBasis      string
	MovementAuthority string
	DecidedAt         time.Time
}

type AcceptDispositionResult struct {
	outcome    AcceptDispositionOutcome
	reason     AcceptDispositionUndecidedReason
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
	if strings.TrimSpace(command.MovementAction) == "" {
		// 事项没有真实移动动作：原地查验、原地扣留与节点处置归 UC-NO-001，这里连
		// 运输对象都不建。
		return AcceptDispositionResult{outcome: DispositionNodeOnly}, nil
	}

	key := ports.DispositionAcceptanceKey{Tenant: command.TenantID, Item: command.Item}
	digest := dispositionDigest(command)
	existing, found, err := handler.deps.Acceptances.FindByKey(ctx, key)
	if err != nil {
		return AcceptDispositionResult{outcome: AcceptDispositionUndecided, reason: DispositionAcceptanceStoreUnavailable}, nil
	}
	if found {
		if existing.ContentDigest != digest {
			// 同一事项携带不同决定内容：改决定走事项方的更正/替代，不经承接入口顶替。
			return AcceptDispositionResult{outcome: DispositionDecisionConflict}, nil
		}
		return AcceptDispositionResult{
			outcome:    DispositionExistingDecision,
			record:     existing,
			hasRecord:  true,
			handoffRef: handler.handOffReceipt(ctx, existing),
		}, nil
	}

	decision, result, formed := handler.form(command)
	if !formed {
		return result, nil
	}

	record := ports.DispositionAcceptanceRecord{Key: key, ContentDigest: digest, Decision: decision}
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
		if winner.ContentDigest != digest {
			return AcceptDispositionResult{outcome: DispositionDecisionConflict}, nil
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

// form 把命令译成领域承接决定。缺移动授权落待补充格（续办是补授权，恢复引用即事项
// 本身）；其余领域拒绝是形状错，答未受理。
func (handler *AcceptRegulatoryDispositionHandler) form(
	command AcceptRegulatoryDispositionCommand,
) (domain.RegulatoryTransportDisposition, AcceptDispositionResult, bool) {
	objects := make([]domain.CarriedObjectReference, 0, len(command.AcceptedObjects))
	for _, raw := range command.AcceptedObjects {
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
			AcceptDispositionResult{outcome: DispositionSupplementRequired}, false
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

// dispositionDigest 折叠承接决定的全部业务内容：同键异指纹即冒名冲突。
func dispositionDigest(command AcceptRegulatoryDispositionCommand) string {
	objects := append([]string(nil), command.AcceptedObjects...)
	sort.Strings(objects)
	parts := []string{
		command.Basis.String(),
		strings.TrimSpace(command.MovementAction),
		command.Decision.String(),
		strings.Join(objects, ","),
		command.DeclineBasis,
		strings.TrimSpace(command.MovementAuthority),
		command.DecidedAt.UTC().Format(time.RFC3339Nano),
	}
	digest := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(digest[:8])
}

func dispositionReceiptDigest(key ports.DispositionAcceptanceKey) string {
	digest := sha256.Sum256([]byte("REGULATORY_ACCEPTANCE\x00" + key.Tenant.String() + "\x00" + key.Item.String()))
	return hex.EncodeToString(digest[:8])
}
