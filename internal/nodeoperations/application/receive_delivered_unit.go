// Package application 编排 node-operations 的用例。判断规则在领域，这里只做受理、
// 幂等、身份核对分派、提交与发布意图的协调。
package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
)

// ErrUnexpectedReceptionSave 说明收寄库交回了封闭集合以外的写入结果。
var ErrUnexpectedReceptionSave = errors.New("node operations: unexpected reception save outcome")

// ReceptionOutcome 是收寄请求的应用处理结果，对应 UC-NO-002 结果语义契约的六格。
type ReceptionOutcome uint8

const (
	ReceptionOutcomeInvalid ReceptionOutcome = iota
	NodeIntakeFormed
	UnitPendingIdentification
	NodeIntakeNotFormed
	ReceptionUndecided
	ReceptionExistingResult
	ReceptionSourceConflict
)

func (outcome ReceptionOutcome) String() string {
	switch outcome {
	case NodeIntakeFormed:
		return "INTAKE_FORMED"
	case UnitPendingIdentification:
		return "PENDING_IDENTIFICATION"
	case NodeIntakeNotFormed:
		return "INTAKE_NOT_FORMED"
	case ReceptionUndecided:
		return "RECEPTION_UNDECIDED"
	case ReceptionExistingResult:
		return "EXISTING_RESULT"
	case ReceptionSourceConflict:
		return "SOURCE_CONFLICT"
	default:
		return ""
	}
}

// ReceptionClaimKind 是本次交付的接收观察三态：明确接收（带证据）、明确拒收（带原因）、
// 只有扫描或卸载。三态是输入观察不是判断——判断（收寄形成/未形成/待确认）由编排按
// 观察加身份核对得出。
type ReceptionClaimKind uint8

const (
	ReceptionClaimKindInvalid ReceptionClaimKind = iota
	ExplicitReception
	ExplicitRefusal
	ScanOnlyObservation
)

// ReceiveDeliveredUnitCommand 携带一件实物的交付受理输入。批量交付由调用方逐件分发：
// 每件实物独立形成结果，整批状态吞并成员差异被用例明禁。
type ReceiveDeliveredUnitCommand struct {
	TenantID    domain.TenantID
	SourceID    string
	Node        domain.NodeReference
	DeliveredBy domain.DeliveringPartyReference
	Unit        domain.HandlingUnitID
	Mark        ports.ExternalMarkObservation
	Claim       ReceptionClaimKind
	Evidence    domain.ReceptionEvidenceReference
	Refusal     string
	// ServiceMarkers 是 PS 服务结果引用（已取消/终局/无路由/受限），由接入层查好带入。
	// 编排原样标记不判断——它们不阻止接收，只限制后续方向性作业（AT-NO-020/021/022）。
	ServiceMarkers []string
	OccurredAt     time.Time
}

type ReceiveDeliveredUnitResult struct {
	outcome      ReceptionOutcome
	record       ports.ReceptionRecord
	hasRecord    bool
	continuation string
	handoff      string
}

func (result ReceiveDeliveredUnitResult) Outcome() ReceptionOutcome {
	return result.outcome
}

func (result ReceiveDeliveredUnitResult) Record() (ports.ReceptionRecord, bool) {
	return result.record, result.hasRecord
}

func (result ReceiveDeliveredUnitResult) ContinuationReference() string {
	return result.continuation
}

// IntakeHandoffReference 非空说明结果已提交但意图还没交出去，重放会重发同一份。
func (result ReceiveDeliveredUnitResult) IntakeHandoffReference() string {
	return result.handoff
}

type ReceiveDeliveredUnitDeps struct {
	Identity   ports.ParcelIdentityView
	Receptions ports.ReceptionStore
	Versions   ports.IntakeIdentityFactory
	Downstream ports.NodeIntakeHandoff
	Clock      ports.Clock
}

type ReceiveDeliveredUnitHandler struct {
	deps ReceiveDeliveredUnitDeps
}

func NewReceiveDeliveredUnitHandler(deps ReceiveDeliveredUnitDeps) *ReceiveDeliveredUnitHandler {
	return &ReceiveDeliveredUnitHandler{deps: deps}
}

// Handle 把一件送达实物推进到收寄判断：幂等/冲突按内容指纹分界 → 接收观察三态分派
// （拒收不建控制、仅扫描不建控制）→ 身份核对分关联/待识别/身份冲突 → 收寄与控制
// 同一提交 → 发布意图。物理事实全程只增不删。
func (handler *ReceiveDeliveredUnitHandler) Handle(
	ctx context.Context,
	command ReceiveDeliveredUnitCommand,
) (ReceiveDeliveredUnitResult, error) {
	if strings.TrimSpace(command.TenantID.String()) == "" ||
		strings.TrimSpace(command.SourceID) == "" ||
		strings.TrimSpace(command.Node.String()) == "" ||
		strings.TrimSpace(command.Unit.String()) == "" ||
		command.OccurredAt.IsZero() {
		return ReceiveDeliveredUnitResult{outcome: ReceptionUndecided,
			continuation: receptionContinuation("INCOMPLETE_DELIVERY", command.SourceID)}, nil
	}

	key := ports.ReceptionKey{TenantID: command.TenantID, SourceID: command.SourceID}
	digest := deliveryContentDigest(command)
	existing, found, err := handler.deps.Receptions.FindByKey(ctx, key)
	if err != nil {
		return ReceiveDeliveredUnitResult{outcome: ReceptionUndecided,
			continuation: receptionContinuation("RECEPTION_STORE_UNAVAILABLE", command.SourceID)}, nil
	}
	if found {
		if existing.ContentDigest != digest {
			// 同一来源身份携带不同对象、时间或内容：冲突保留原结果，停止自动采用
			// （AT-NO-017）。
			return ReceiveDeliveredUnitResult{outcome: ReceptionSourceConflict}, nil
		}
		return handler.existingResult(ctx, existing), nil
	}

	switch command.Claim {
	case ExplicitReception:
		return handler.receive(ctx, command, key, digest)
	case ExplicitRefusal:
		// 明确拒收且未取得控制：未收寄结果带原因，不形成委托拒绝、取消或终局
		// （AT-NO-024）。
		record := ports.ReceptionRecord{
			Key:            key,
			ContentDigest:  digest,
			Kind:           ports.RecordIntakeNotFormed,
			RefusalReason:  command.Refusal,
			ServiceMarkers: command.ServiceMarkers,
			RecordedAt:     handler.deps.Clock.Now(),
		}
		return handler.commit(ctx, record)
	case ScanOnlyObservation:
		// 只有到站扫描，没有明确接收或控制证据：判断待确认、不建立控制；扫描来源
		// 已随记录保全（AT-NO-023）。
		record := ports.ReceptionRecord{
			Key:            key,
			ContentDigest:  digest,
			Kind:           ports.RecordReceptionUndecided,
			ServiceMarkers: command.ServiceMarkers,
			RecordedAt:     handler.deps.Clock.Now(),
		}
		return handler.commit(ctx, record)
	default:
		return ReceiveDeliveredUnitResult{outcome: ReceptionUndecided,
			continuation: receptionContinuation("UNKNOWN_RECEPTION_CLAIM", command.SourceID)}, nil
	}
}

// receive 走明确接收支：身份核对分派关联收寄或待识别，两格都成立收寄与控制。
func (handler *ReceiveDeliveredUnitHandler) receive(
	ctx context.Context,
	command ReceiveDeliveredUnitCommand,
	key ports.ReceptionKey,
	digest string,
) (ReceiveDeliveredUnitResult, error) {
	candidates, err := handler.deps.Identity.ResolveParcelIdentity(ctx, command.TenantID, command.Mark)
	if err != nil {
		return ReceiveDeliveredUnitResult{outcome: ReceptionUndecided,
			continuation: receptionContinuation("IDENTITY_VIEW_UNAVAILABLE", command.SourceID)}, nil
	}
	version, err := handler.deps.Versions.NextIntakeResultVersion(ctx)
	if err != nil {
		return ReceiveDeliveredUnitResult{outcome: ReceptionUndecided,
			continuation: receptionContinuation("INTAKE_IDENTITY_UNAVAILABLE", command.SourceID)}, nil
	}

	spec := domain.NodeIntakeSpec{
		TenantID:    command.TenantID,
		Unit:        command.Unit,
		Node:        command.Node,
		DeliveredBy: command.DeliveredBy,
		Evidence:    command.Evidence,
		Version:     version,
		ReceivedAt:  command.OccurredAt,
	}
	kind := ports.RecordPendingIdentification
	conflict := false
	switch len(candidates) {
	case 1:
		// 唯一且证据充分：关联正式包裹，形成可交 PS 采用的收寄（AT-NO-015）。
		spec.Association = candidates[0]
		kind = ports.RecordIntakeFormed
	case 0:
		// 未知：待识别实物——收寄与控制真实成立，正式身份等版本化识别（AT-NO-018）。
	default:
		// 一码多物或多候选：各实物独立、身份冲突暂停方向性作业；不猜身份（AT-NO-019）。
		conflict = true
	}

	intake, err := domain.FormNodeIntake(spec)
	if err != nil {
		return ReceiveDeliveredUnitResult{}, fmt.Errorf("form node intake: %w", err)
	}
	basis, err := domain.NewControlBasisReference("NODE-INTAKE/" + version.String())
	if err != nil {
		return ReceiveDeliveredUnitResult{}, fmt.Errorf("control basis: %w", err)
	}
	control, err := domain.EstablishPhysicalControl(domain.PhysicalControlSpec{
		TenantID:      command.TenantID,
		Unit:          command.Unit,
		Node:          command.Node,
		Kind:          domain.EstablishedByNodeIntake,
		Basis:         basis,
		EstablishedAt: command.OccurredAt,
	})
	if err != nil {
		return ReceiveDeliveredUnitResult{}, fmt.Errorf("establish physical control: %w", err)
	}

	record := ports.ReceptionRecord{
		Key:              key,
		ContentDigest:    digest,
		Kind:             kind,
		Intake:           intake,
		Control:          control,
		Candidates:       candidates,
		IdentityConflict: conflict,
		ServiceMarkers:   command.ServiceMarkers,
		RecordedAt:       handler.deps.Clock.Now(),
	}
	return handler.commit(ctx, record)
}

// commit 提交判断并交发布意图；并发下另一方先提交时读回赢家。
func (handler *ReceiveDeliveredUnitHandler) commit(
	ctx context.Context,
	record ports.ReceptionRecord,
) (ReceiveDeliveredUnitResult, error) {
	saved, err := handler.deps.Receptions.Save(ctx, record)
	if err != nil {
		return ReceiveDeliveredUnitResult{outcome: ReceptionUndecided,
			continuation: receptionContinuation("RECEPTION_STORE_UNAVAILABLE", record.Key.SourceID)}, nil
	}
	switch saved {
	case ports.ReceptionSaved:
		result := resultForRecord(record)
		result.handoff = handler.handOff(ctx, record)
		return result, nil
	case ports.ReceptionAlreadyRecorded:
		winner, found, err := handler.deps.Receptions.FindByKey(ctx, record.Key)
		if err != nil || !found {
			return ReceiveDeliveredUnitResult{outcome: ReceptionUndecided,
				continuation: receptionContinuation("RECEPTION_STORE_UNAVAILABLE", record.Key.SourceID)}, nil
		}
		return handler.existingResult(ctx, winner), nil
	default:
		return ReceiveDeliveredUnitResult{}, fmt.Errorf("%w: %d", ErrUnexpectedReceptionSave, saved)
	}
}

// existingResult 按已有记录作答并重发同一份意图（AT-NO-016/027）。
func (handler *ReceiveDeliveredUnitHandler) existingResult(
	ctx context.Context,
	record ports.ReceptionRecord,
) ReceiveDeliveredUnitResult {
	result := resultForRecord(record)
	result.outcome = ReceptionExistingResult
	result.handoff = handler.handOff(ctx, record)
	return result
}

func resultForRecord(record ports.ReceptionRecord) ReceiveDeliveredUnitResult {
	outcome := ReceptionOutcomeInvalid
	switch record.Kind {
	case ports.RecordIntakeFormed:
		outcome = NodeIntakeFormed
	case ports.RecordPendingIdentification:
		outcome = UnitPendingIdentification
	case ports.RecordIntakeNotFormed:
		outcome = NodeIntakeNotFormed
	case ports.RecordReceptionUndecided:
		outcome = ReceptionUndecided
	}
	return ReceiveDeliveredUnitResult{outcome: outcome, record: record, hasRecord: true}
}

// handOff 交发布意图。只有形成了可供下游消费的收寄（关联收寄）才交——待识别、未收寄
// 与待确认没有 PS 能采用的东西；失败不翻结果，留续办引用重发同一份（AT-NO-027）。
func (handler *ReceiveDeliveredUnitHandler) handOff(
	ctx context.Context,
	record ports.ReceptionRecord,
) string {
	if record.Kind != ports.RecordIntakeFormed {
		return ""
	}
	if err := handler.deps.Downstream.HandOffNodeIntake(ctx, ports.NodeIntakeHandoffIntent{Record: record}); err == nil {
		return ""
	}
	return receptionContinuation("NODE_INTAKE_HANDOFF", record.Key.TenantID.String(), record.Key.SourceID)
}

func receptionContinuation(parts ...string) string {
	digest := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return "CONT-" + hex.EncodeToString(digest[:8])
}

// deliveryContentDigest 是同一来源身份的内容比对锚：实物、标识观察、接收观察、节点与
// 业务时间任一不同即是另一份内容。
func deliveryContentDigest(command ReceiveDeliveredUnitCommand) string {
	digest := sha256.Sum256([]byte(strings.Join([]string{
		command.Unit.String(),
		command.Mark.Mark,
		fmt.Sprintf("%d", command.Claim),
		command.Node.String(),
		command.OccurredAt.UTC().Format(time.RFC3339Nano),
	}, "\x00")))
	return hex.EncodeToString(digest[:])
}
