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

// ErrReceptionCommandRejected 说明这条命令本身有缺陷（必填缺失、接收观察不在三态之内），重发同样的内容不会变好；
// 入口据此答坏报文、前线导入记被拒，而不是当成没形成答案留给重投（ADR-0186）。
var ErrReceptionCommandRejected = errors.New("node operations: reception command rejected")

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
	OccurredAt  time.Time
}

type ReceiveDeliveredUnitResult struct {
	outcome   ReceptionOutcome
	record    ports.ReceptionRecord
	hasRecord bool
	handoff   string
}

func (result ReceiveDeliveredUnitResult) Outcome() ReceptionOutcome {
	return result.outcome
}

func (result ReceiveDeliveredUnitResult) Record() (ports.ReceptionRecord, bool) {
	return result.record, result.hasRecord
}

// IntakeHandoffReference 非空说明结果已提交但意图还没交出去，重放会重发同一份。
func (result ReceiveDeliveredUnitResult) IntakeHandoffReference() string {
	return result.handoff
}

// ReceiveDeliveredUnitDeps 是收寄编排的全部依赖，缺任一个构造即拒。
type ReceiveDeliveredUnitDeps struct {
	Identity ports.ParcelIdentityView
	// ServiceResults 在身份唯一成立时被问一次，答出的取消与终局落成收寄记录上的服务结果标记。它报错时编排照常落册，
	// 只把核对态记成读不动（ADR-0186）；装配须保证读口失败不连带收寄的写，见 transactionalReception。
	ServiceResults ports.ParcelServiceResultView
	Receptions     ports.ReceptionStore
	Versions       ports.IntakeIdentityFactory
	Downstream     ports.NodeIntakeHandoff
	Clock          ports.Clock
}

type ReceiveDeliveredUnitHandler struct {
	deps ReceiveDeliveredUnitDeps
}

// NewReceiveDeliveredUnitHandler 在构造期拒缺依赖的装配：留到运行期，缺件会在第一次收寄时冒充一次依赖故障（ADR-0186）。
func NewReceiveDeliveredUnitHandler(deps ReceiveDeliveredUnitDeps) (*ReceiveDeliveredUnitHandler, error) {
	switch {
	case deps.Identity == nil:
		return nil, errors.New("node operations: reception needs a parcel identity view")
	case deps.ServiceResults == nil:
		return nil, errors.New("node operations: reception needs a parcel service result view")
	case deps.Receptions == nil:
		return nil, errors.New("node operations: reception needs a reception store")
	case deps.Versions == nil:
		return nil, errors.New("node operations: reception needs an intake result version factory")
	case deps.Downstream == nil:
		return nil, errors.New("node operations: reception needs a node intake handoff")
	case deps.Clock == nil:
		return nil, errors.New("node operations: reception needs a clock")
	}
	return &ReceiveDeliveredUnitHandler{deps: deps}, nil
}

// Handle 把一件送达实物推进到收寄判断：幂等/冲突按内容指纹分界 → 接收观察三态分派
// （拒收不建控制、仅扫描不建控制）→ 身份核对分关联/待识别/身份冲突 → 收寄与控制
// 同一提交 → 发布意图。物理事实全程只增不删。
//
// 没形成答案（身份核对读不动、收寄库读写失败、已存摘要认不出、版本签发失败）一律交回 error，不答一格业务结果：
// 什么都没落册时答`收寄待确认`，设备收到 2xx 就出队，这一次接收从此没有记录（ADR-0186、ADR-0022）。
func (handler *ReceiveDeliveredUnitHandler) Handle(
	ctx context.Context,
	command ReceiveDeliveredUnitCommand,
) (ReceiveDeliveredUnitResult, error) {
	if strings.TrimSpace(command.TenantID.String()) == "" ||
		strings.TrimSpace(command.SourceID) == "" ||
		strings.TrimSpace(command.Node.String()) == "" ||
		strings.TrimSpace(command.Unit.String()) == "" ||
		command.OccurredAt.IsZero() {
		return ReceiveDeliveredUnitResult{}, fmt.Errorf("%w: incomplete delivery", ErrReceptionCommandRejected)
	}

	key := ports.ReceptionKey{TenantID: command.TenantID, SourceID: command.SourceID}
	_, digest, err := domain.CanonicalizeDeliveryPayload(command.Unit, command.Mark.Mark, uint8(command.Claim), command.Node, command.OccurredAt)
	if err != nil {
		return ReceiveDeliveredUnitResult{}, err
	}
	existing, found, err := handler.deps.Receptions.FindByKey(ctx, key)
	if err != nil {
		return ReceiveDeliveredUnitResult{}, fmt.Errorf("look up reception: %w", err)
	}
	if found {
		switch domain.CompareStoredDigest(existing.ContentDigest, digest, unversionedDeliveryContentDigest(command)) {
		case domain.SamePayload:
			return handler.existingResult(ctx, existing), nil
		case domain.DifferentPayload:
			// 同一来源身份携带不同对象、时间或内容：冲突保留原结果，停止自动采用
			// （AT-NO-017）。
			return ReceiveDeliveredUnitResult{outcome: ReceptionSourceConflict}, nil
		default:
			// 已存摘要的形状版本认不出：无从判断重放还是冲突，不猜，也不另落一条。
			return ReceiveDeliveredUnitResult{}, errors.New("node operations: stored reception digest is not comparable")
		}
	}

	switch command.Claim {
	case ExplicitReception:
		return handler.receive(ctx, command, key, digest)
	case ExplicitRefusal:
		// 明确拒收且未取得控制：未收寄结果带原因，不形成委托拒绝、取消或终局
		// （AT-NO-024）。
		record := ports.ReceptionRecord{
			Key:           key,
			ContentDigest: digest,
			Kind:          ports.RecordIntakeNotFormed,
			RefusalReason: command.Refusal,
			RecordedAt:    handler.deps.Clock.Now(),
		}
		return handler.commit(ctx, record)
	case ScanOnlyObservation:
		// 只有到站扫描，没有明确接收或控制证据：判断待确认、不建立控制；扫描来源
		// 已随记录保全（AT-NO-023）。
		record := ports.ReceptionRecord{
			Key:           key,
			ContentDigest: digest,
			Kind:          ports.RecordReceptionUndecided,
			RecordedAt:    handler.deps.Clock.Now(),
		}
		return handler.commit(ctx, record)
	default:
		return ReceiveDeliveredUnitResult{}, fmt.Errorf("%w: unknown reception claim %d", ErrReceptionCommandRejected, command.Claim)
	}
}

// receive 走明确接收支：身份核对分派关联收寄或待识别，两格都成立收寄与控制。
func (handler *ReceiveDeliveredUnitHandler) receive(
	ctx context.Context,
	command ReceiveDeliveredUnitCommand,
	key ports.ReceptionKey,
	digest string,
) (ReceiveDeliveredUnitResult, error) {
	// 身份核对读不动不拿空候选顶替（ADR-0089）：那会把可能已知的包裹落成待识别。
	candidates, err := handler.deps.Identity.ResolveParcelIdentity(ctx, command.TenantID, command.Mark)
	if err != nil {
		return ReceiveDeliveredUnitResult{}, fmt.Errorf("resolve parcel identity: %w", err)
	}
	// 号码唯一指向一件包裹，还要问这件包裹是否已关联到另一件作业实物：是，就是「同一外部条码出现在多个实物」（NO
	// CONTEXT「实物身份与事实完整性」、ADR-0196）。按关联到的包裹认而不按号码认，不论先到那件此刻是否仍在节点控制下；
	// 同一作业实物再来不在此列，同一份送站重投在上面按幂等键走已有结果。读不动与身份核对读不动同样不形成答案。
	var others []ports.ReceptionRecord
	if len(candidates) == 1 {
		others, err = handler.deps.Receptions.FindOtherUnitIntakes(ctx, command.TenantID, candidates[0], command.Unit)
		if err != nil {
			return ReceiveDeliveredUnitResult{}, fmt.Errorf("find other units associated with the parcel: %w", err)
		}
	}
	identified := len(candidates) == 1 && len(others) == 0

	// 身份唯一成立才有可问的包裹：向 PS 取它此刻的服务结果，落成标记（UC-NO-002「对已取消、终局、无路由或受限实物
	// 标记相应引用」）。读不动不挡已经发生的物理事实——照常落册，核对态记读不动、标记留空，不补查（ADR-0186）；空标记
	// 因此只在已查时才说「没有取消与终局」。
	var markers []string
	check := ports.ServiceResultCheckAbsent
	if identified {
		results, err := handler.deps.ServiceResults.FindParcelServiceResults(ctx, command.TenantID, candidates[0])
		if err != nil {
			check = ports.ServiceResultsUnreadable
		} else {
			check = ports.ServiceResultsChecked
			markers = results.Markers()
		}
	}
	version, err := handler.deps.Versions.NextIntakeResultVersion(ctx)
	if err != nil {
		return ReceiveDeliveredUnitResult{}, fmt.Errorf("next intake result version: %w", err)
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
	switch {
	case identified:
		// 唯一且证据充分：关联正式包裹，形成可交 PS 采用的收寄（AT-NO-015）。
		spec.Association = candidates[0]
		kind = ports.RecordIntakeFormed
	case len(candidates) == 0:
		// 未知：待识别实物——收寄与控制真实成立，正式身份等版本化识别（AT-NO-018）。
	default:
		// 号码在册里标识多件包裹，或唯一指向的那件包裹已关联到另一件作业实物（一码多物，AT-NO-019）：各实物独立、
		// 身份冲突暂停方向性作业；不猜身份，候选照记。
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
		Key:                key,
		ContentDigest:      digest,
		Kind:               kind,
		Intake:             intake,
		Control:            control,
		Candidates:         candidates,
		IdentityConflict:   conflict,
		ServiceMarkers:     markers,
		ServiceResultCheck: check,
		RecordedAt:         handler.deps.Clock.Now(),
	}
	result, err := handler.commit(ctx, record)
	// 只在本次真落了册时追加：并发里输给另一方的那次读回的是赢家，赢家已经追加过。
	if err != nil || len(others) == 0 || result.Outcome() != UnitPendingIdentification {
		return result, err
	}
	if err := handler.appendIdentityConflicts(ctx, record, others); err != nil {
		return ReceiveDeliveredUnitResult{}, err
	}
	return result, nil
}

// appendIdentityConflicts 给关联到同一件包裹的先到实物各追加一版身份冲突判断（ADR-0196）。原判断照留——它可能已被 PS
// 采用，那次采用不在这里回滚，交 VE 协调；只追加、不覆盖（CONTEXT：更正与冲突必须关联原事实和原判断，不能通过覆盖历史
// 整理出单一结果）。方向依据的身份一格读到这一版就落成身份冲突。
//
// 追加的一版是那件实物的又一行待识别收寄：收寄事实与控制照原判断，换一个结果版本、去掉关联、候选记那件包裹、记身份
// 冲突。它不交 PS 意图（待识别没有可采用的关联）。来源标识由后到那次送站与先到那件实物派生：同一次送站重放不会再追加，
// 第三件再来时各追加各的。
func (handler *ReceiveDeliveredUnitHandler) appendIdentityConflicts(
	ctx context.Context,
	late ports.ReceptionRecord,
	others []ports.ReceptionRecord,
) error {
	latest := map[domain.HandlingUnitID]ports.ReceptionRecord{}
	var order []domain.HandlingUnitID
	for _, other := range others {
		unit := other.Intake.Unit()
		current, seen := latest[unit]
		if !seen {
			order = append(order, unit)
		}
		// 同一件先到实物收过多次时，依后一次追加——与方向依据取所依收寄的次序同一个口径（domain.JudgeIdentityBasis）。
		if !seen || other.Intake.ReceivedAt().After(current.Intake.ReceivedAt()) {
			latest[unit] = other
		}
	}
	for _, unit := range order {
		earlier := latest[unit]
		version, err := handler.deps.Versions.NextIntakeResultVersion(ctx)
		if err != nil {
			return fmt.Errorf("next intake result version for identity conflict: %w", err)
		}
		intake, err := domain.FormNodeIntake(domain.NodeIntakeSpec{
			TenantID:    late.Key.TenantID,
			Unit:        unit,
			Node:        earlier.Intake.Node(),
			DeliveredBy: earlier.Intake.DeliveredBy(),
			Evidence:    earlier.Intake.Evidence(),
			Version:     version,
			ReceivedAt:  earlier.Intake.ReceivedAt(),
		})
		if err != nil {
			return fmt.Errorf("form identity conflict version: %w", err)
		}
		record := ports.ReceptionRecord{
			Key: ports.ReceptionKey{
				TenantID: late.Key.TenantID,
				SourceID: identityConflictSourceID(late.Key.SourceID, unit),
			},
			// 摘要沿用后到那次送站的：这一版由那次送站引起；万一有设备撞上派生的来源标识，指纹对不上，答来源冲突而不是重放。
			ContentDigest:    late.ContentDigest,
			Kind:             ports.RecordPendingIdentification,
			Intake:           intake,
			Control:          earlier.Control,
			Candidates:       late.Candidates,
			IdentityConflict: true,
			RecordedAt:       late.RecordedAt,
		}
		saved, err := handler.deps.Receptions.Save(ctx, record)
		if err != nil {
			return fmt.Errorf("record identity conflict version: %w", err)
		}
		// 后到那件这次才真落册，派生键不该已经在册；在册就说明派生键被占了，不拿占位的那一行顶替这一版冲突。
		if saved != ports.ReceptionSaved {
			return fmt.Errorf("%w: identity conflict version %q", ErrUnexpectedReceptionSave, record.Key.SourceID)
		}
	}
	return nil
}

// identityConflictSourceID 派生追加版的来源标识。前缀把它与设备和导入送来的来源标识分开（风险见 ADR-0196）。
func identityConflictSourceID(lateSource string, earlierUnit domain.HandlingUnitID) string {
	return "IDENTITY-CONFLICT/" + lateSource + "/" + earlierUnit.String()
}

// commit 提交判断并交发布意图；并发下另一方先提交时读回赢家。
func (handler *ReceiveDeliveredUnitHandler) commit(
	ctx context.Context,
	record ports.ReceptionRecord,
) (ReceiveDeliveredUnitResult, error) {
	saved, err := handler.deps.Receptions.Save(ctx, record)
	if err != nil {
		return ReceiveDeliveredUnitResult{}, fmt.Errorf("record reception: %w", err)
	}
	switch saved {
	case ports.ReceptionSaved:
		result := resultForRecord(record)
		result.handoff = handler.handOff(ctx, record)
		return result, nil
	case ports.ReceptionAlreadyRecorded:
		winner, found, err := handler.deps.Receptions.FindByKey(ctx, record.Key)
		if err != nil {
			return ReceiveDeliveredUnitResult{}, fmt.Errorf("read back reception winner: %w", err)
		}
		if !found {
			return ReceiveDeliveredUnitResult{}, errors.New("node operations: reception already recorded but not readable")
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
