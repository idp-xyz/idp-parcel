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

// String 是接收观察写进来源文档的名字，取在线口与受控批量口共用的那份词表（两口逐字一致，见 nodeopshttp 的
// receptionDocument）：来源文档存的是交付观察，用设备那头的词，看的人不必再翻一道。内容指纹不用它，照旧按封闭码记。
func (kind ReceptionClaimKind) String() string {
	switch kind {
	case ExplicitReception:
		return "RECEIVED"
	case ExplicitRefusal:
		return "REFUSED"
	case ScanOnlyObservation:
		return "SCAN_ONLY"
	default:
		return ""
	}
}

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
	outcome        ReceptionOutcome
	record         ports.ReceptionRecord
	hasRecord      bool
	currentVersion domain.IntakeResultVersion
	mergedInto     domain.IntakeResultVersion
	handoff        string
	handoffErr     error
}

func (result ReceiveDeliveredUnitResult) Outcome() ReceptionOutcome {
	return result.outcome
}

func (result ReceiveDeliveredUnitResult) Record() (ports.ReceptionRecord, bool) {
	return result.record, result.hasRecord
}

// RecordedOutcome 是已有结果所指那条记录的原判断种类，CurrentVersion 是那条判断链此刻的当前有效版本（ADR-0199 决定二）。两格
// 只随已有结果在场；它们是信息格，Outcome 仍是已有结果——设备照 outcome 出队（ADR-0022），原判断后来被接续成什么不改变这次
// 重投的处置。没有判断版本的旧行（来源保全之前的未收寄与待确认）CurrentVersion 缺席。
func (result ReceiveDeliveredUnitResult) RecordedOutcome() ReceptionOutcome {
	if result.outcome != ReceptionExistingResult {
		return ReceptionOutcomeInvalid
	}
	return outcomeOfKind(result.record.Kind)
}

func (result ReceiveDeliveredUnitResult) CurrentVersion() domain.IntakeResultVersion {
	return result.currentVersion
}

// MergedInto 是这次送站并入的那一版判断（ADR-0199 决定四），只随已有结果在场：首次并入与并入行的重投都带，同一来源的其余
// 重投不带。它让读续办与报告的一方分得开「这一来源已在册」与「这一来源刚并进了别人的收寄」；设备照 outcome 出队，两者同一种
// 处置，线上不另立一格。
func (result ReceiveDeliveredUnitResult) MergedInto() domain.IntakeResultVersion {
	return result.mergedInto
}

// IntakeHandoffReference 非空说明收寄已写下而意图没交出去：调用方照常提交时，重放会重发同一份。
func (result ReceiveDeliveredUnitResult) IntakeHandoffReference() string {
	return result.handoff
}

// IntakeHandoffError 是意图没交出去时那次入队报的错，与 IntakeHandoffReference 同在场同缺席。续办引用只对有人读它的调用方
// 有用（前线导入把它写进报告）；没人读的调用方据此整笔回滚，不提交意图没跟上的收寄（UC-NO-002 步骤 8，见 parcel-api 的
// transactionalReception）。
func (result ReceiveDeliveredUnitResult) IntakeHandoffError() error {
	return result.handoffErr
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

// Handle 把一件送达实物推进到收寄判断：幂等/冲突按内容指纹分界 → 认同一作业实物在这个节点上的当前收寄与待确认（ADR-0199
// 决定四：有当前收寄即并入，有待确认即续办）→ 接收观察三态分派（拒收不建控制、仅扫描不建控制）→ 身份核对分关联/待识别/
// 身份冲突 → 收寄与控制同一提交 → 发布意图。物理事实全程只增不删。
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
			return handler.existingResult(ctx, existing)
		case domain.DifferentPayload:
			// 同一来源身份携带不同对象、时间或内容：冲突保留原结果，停止自动采用
			// （AT-NO-017）。
			return ReceiveDeliveredUnitResult{outcome: ReceptionSourceConflict}, nil
		default:
			// 已存摘要的形状版本认不出：无从判断重放还是冲突，不猜，也不另落一条。
			return ReceiveDeliveredUnitResult{}, errors.New("node operations: stored reception digest is not comparable")
		}
	}

	// 接收观察不在三态之内的命令先拒：认实物在前，一条坏命令会被答成并入的已有结果。
	if command.Claim.String() == "" {
		return ReceiveDeliveredUnitResult{}, fmt.Errorf("%w: unknown reception claim %d", ErrReceptionCommandRejected, command.Claim)
	}
	// 同一作业实物按租户内的作业实物标识认，在身份核对之前认出（ADR-0199 决定四）：三种观察都先问，这一问在真库上取实物锁，
	// 同一件实物的另一次送站等到本笔提交。读不动就不形成答案——认不出当前收寄还往下走，就是又起一个控制起点。
	judgments, err := handler.deps.Receptions.FindCurrentJudgmentsAtNode(ctx, command.TenantID, command.Unit, command.Node)
	if err != nil {
		return ReceiveDeliveredUnitResult{}, fmt.Errorf("find current judgments of the unit at the node: %w", err)
	}
	if current, found := currentIntakeAmong(judgments); found {
		return handler.merge(ctx, command, key, digest, current)
	}
	predecessor := undecidedToContinue(judgments)

	switch command.Claim {
	case ExplicitReception:
		return handler.receive(ctx, command, key, digest, predecessor)
	case ExplicitRefusal:
		// 明确拒收且未取得控制：未收寄结果带原因，不形成委托拒绝、取消或终局
		// （AT-NO-024）。
		return handler.commitWithoutIntake(ctx, command, ports.ReceptionRecord{
			Key:           key,
			ContentDigest: digest,
			Kind:          ports.RecordIntakeNotFormed,
			Predecessor:   predecessor,
			RefusalReason: command.Refusal,
		})
	case ScanOnlyObservation:
		// 只有到站扫描，没有明确接收或控制证据：判断待确认、不建立控制；扫描来源
		// 随记录保全（AT-NO-023）。
		return handler.commitWithoutIntake(ctx, command, ports.ReceptionRecord{
			Key:           key,
			ContentDigest: digest,
			Kind:          ports.RecordReceptionUndecided,
			Predecessor:   predecessor,
		})
	default:
		return ReceiveDeliveredUnitResult{}, fmt.Errorf("%w: unknown reception claim %d", ErrReceptionCommandRejected, command.Claim)
	}
}

// commitWithoutIntake 落未收寄与待确认两格：签判断版本、带来源文档，再提交。这两格没有收寄结果版本可借，判断版本向同一个
// 签发器另签（ADR-0199 决定二）；签不出就不形成答案，同明确接收那一支。
func (handler *ReceiveDeliveredUnitHandler) commitWithoutIntake(
	ctx context.Context,
	command ReceiveDeliveredUnitCommand,
	record ports.ReceptionRecord,
) (ReceiveDeliveredUnitResult, error) {
	version, err := handler.deps.Versions.NextIntakeResultVersion(ctx)
	if err != nil {
		return ReceiveDeliveredUnitResult{}, fmt.Errorf("next judgment version: %w", err)
	}
	record.JudgmentVersion = version
	record.Source = deliverySourceOf(command)
	record.RecordedAt = handler.deps.Clock.Now()
	return handler.commit(ctx, record)
}

// deliverySourceOf 取本次交付的来源文档（ADR-0199 决定一）。逐格照抄命令，外部标识观察不改写：入口交来什么就存什么。
func deliverySourceOf(command ReceiveDeliveredUnitCommand) domain.DeliverySource {
	return domain.DeliverySource{
		Unit:        command.Unit,
		Mark:        command.Mark.Mark,
		Claim:       command.Claim.String(),
		Evidence:    command.Evidence,
		Refusal:     command.Refusal,
		DeliveredBy: command.DeliveredBy,
		Node:        command.Node,
		OccurredAt:  command.OccurredAt.UTC(),
	}
}

// receive 走明确接收支：身份核对分派关联收寄或待识别，两格都成立收寄与控制。predecessor 非空即这次接收续办那条待确认。
func (handler *ReceiveDeliveredUnitHandler) receive(
	ctx context.Context,
	command ReceiveDeliveredUnitCommand,
	key ports.ReceptionKey,
	digest string,
	predecessor domain.IntakeResultVersion,
) (ReceiveDeliveredUnitResult, error) {
	judgment, err := identityJudge{
		identity:   handler.deps.Identity,
		services:   handler.deps.ServiceResults,
		receptions: handler.deps.Receptions,
	}.judge(ctx, command.TenantID, command.Mark, command.Unit)
	if err != nil {
		return ReceiveDeliveredUnitResult{}, err
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
	// 唯一且证据充分：关联正式包裹，形成可交 PS 采用的收寄（AT-NO-015）。未知（零候选）：待识别实物——收寄与控制真实成立，
	// 正式身份等版本化识别（AT-NO-018）。身份冲突（AT-NO-019）：各实物独立、暂停方向性作业；不猜身份，候选照记。
	kind := ports.RecordPendingIdentification
	if judgment.identified {
		spec.Association = judgment.parcel
		kind = ports.RecordIntakeFormed
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
		JudgmentVersion:    version,
		Predecessor:        predecessor,
		Source:             deliverySourceOf(command),
		Intake:             intake,
		Control:            control,
		Candidates:         judgment.candidates,
		IdentityConflict:   judgment.conflict(),
		ServiceMarkers:     judgment.markers,
		ServiceResultCheck: judgment.check,
		RecordedAt:         handler.deps.Clock.Now(),
	}
	result, err := handler.commit(ctx, record)
	// 只在本次真落了册时追加：并发里输给另一方的那次读回的是赢家，赢家已经追加过。
	if err != nil || len(judgment.others) == 0 || result.Outcome() != UnitPendingIdentification {
		return result, err
	}
	if err := appendIdentityConflicts(ctx, handler.deps.Receptions, handler.deps.Versions, record, judgment.others); err != nil {
		return ReceiveDeliveredUnitResult{}, err
	}
	return result, nil
}

// appendIdentityConflicts 给关联到同一件包裹的先到实物各追加一版身份冲突判断（ADR-0196）。原判断照留——它可能已被 PS
// 采用，那次采用不在这里回滚，交 VE 协调；只追加、不覆盖（CONTEXT：更正与冲突必须关联原事实和原判断，不能通过覆盖历史
// 整理出单一结果）。方向依据的身份一格只读各条判断链的当前有效版本（ADR-0210 决定五）：这一版接续了被依的那条链，那条链
// 的链头从此是这一版——不带关联、记身份冲突；先到那件没有别的链时，身份一格落成缺少·{待识别, 身份冲突}，原判断上的关联
// 不再参与。
//
// 追加的一版是那件实物的又一行待识别收寄：收寄事实与控制照原判断，换一个结果版本、去掉关联、候选记那件包裹、记身份
// 冲突。它不交 PS 意图（待识别没有可采用的关联）。来源标识由后到那次送站与先到那件实物派生：同一次送站重放不会再追加，
// 第三件再来时各追加各的。
//
// 这一版指名前版：被依的那一行所在判断链此刻的当前有效版本（ADR-0199 决定二）。第三件再来时那条链已由上一版冲突版接续，
// 前版就是上一版冲突版，不是原判断——指原判断就成了一版两个后继。读链在包裹锁之后（FindOtherUnitIntakes 取的锁），并发的
// 另一件送站已追加的那一版此刻读得到。来源保全之前追加的冲突版没有指名前版，链上看不见它，这里会从原判断接下去（ADR-0199
// 决定二「不回填」）。
func appendIdentityConflicts(
	ctx context.Context,
	receptions ports.ReceptionStore,
	versions ports.IntakeIdentityFactory,
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
		version, err := versions.NextIntakeResultVersion(ctx)
		if err != nil {
			return fmt.Errorf("next intake result version for identity conflict: %w", err)
		}
		// 被依的那一行带着关联，是形成格，它的判断版本就是收寄结果版本。
		predecessor, err := receptions.CurrentVersion(ctx, late.Key.TenantID, earlier.Intake.Version())
		if err != nil {
			return fmt.Errorf("current version before identity conflict: %w", err)
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
			// 摘要与来源文档都沿用后到那次送站的：这一版由那次送站引起；万一有设备撞上派生的来源标识，指纹对不上，答来源冲突
			// 而不是重放。来源文档里的作业实物因此是后到那件。
			ContentDigest:    late.ContentDigest,
			Kind:             ports.RecordPendingIdentification,
			JudgmentVersion:  version,
			Predecessor:      predecessor,
			Source:           late.Source,
			Intake:           intake,
			Control:          earlier.Control,
			Candidates:       late.Candidates,
			IdentityConflict: true,
			RecordedAt:       late.RecordedAt,
		}
		saved, err := receptions.Save(ctx, record)
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
		result.handoff, result.handoffErr = handOffIntake(ctx, handler.deps.Downstream, record)
		return result, nil
	case ports.ReceptionAlreadyRecorded:
		winner, found, err := handler.deps.Receptions.FindByKey(ctx, record.Key)
		if err != nil {
			return ReceiveDeliveredUnitResult{}, fmt.Errorf("read back reception winner: %w", err)
		}
		if !found {
			return ReceiveDeliveredUnitResult{}, errors.New("node operations: reception already recorded but not readable")
		}
		return handler.existingResult(ctx, winner)
	default:
		return ReceiveDeliveredUnitResult{}, fmt.Errorf("%w: %d", ErrUnexpectedReceptionSave, saved)
	}
}

// existingResult 按已有记录作答并重发同一份意图（AT-NO-016/027），带回那条判断链此刻的当前有效版本（ADR-0199 决定二）。读不出
// 当前有效版本就不形成答案：答一格缺了它的已有结果，设备出队，原判断后来被接续这件事就没人看得到。
func (handler *ReceiveDeliveredUnitHandler) existingResult(
	ctx context.Context,
	record ports.ReceptionRecord,
) (ReceiveDeliveredUnitResult, error) {
	// 并入行不是判断，答的是它并入的那一版：重投并入的那一来源与首次并入同答。读不回那一版就不形成答案。
	if record.Kind == ports.RecordMergedIntoExistingIntake {
		merged, found, err := handler.deps.Receptions.FindByJudgmentVersion(ctx, record.Key.TenantID, record.MergedInto)
		if err != nil {
			return ReceiveDeliveredUnitResult{}, fmt.Errorf("read the judgment the delivery was merged into: %w", err)
		}
		if !found {
			return ReceiveDeliveredUnitResult{}, fmt.Errorf("node operations: merged judgment %q is not readable", record.MergedInto)
		}
		return handler.mergedResult(ctx, merged)
	}
	result := resultForRecord(record)
	result.outcome = ReceptionExistingResult
	if record.JudgmentVersion.String() != "" {
		current, err := handler.deps.Receptions.CurrentVersion(ctx, record.Key.TenantID, record.JudgmentVersion)
		if err != nil {
			return ReceiveDeliveredUnitResult{}, fmt.Errorf("current version of the recorded judgment: %w", err)
		}
		result.currentVersion = current
	}
	result.handoff, result.handoffErr = handOffIntake(ctx, handler.deps.Downstream, record)
	return result, nil
}

func resultForRecord(record ports.ReceptionRecord) ReceiveDeliveredUnitResult {
	return ReceiveDeliveredUnitResult{outcome: outcomeOfKind(record.Kind), record: record, hasRecord: true}
}

func outcomeOfKind(kind ports.ReceptionRecordKind) ReceptionOutcome {
	switch kind {
	case ports.RecordIntakeFormed:
		return NodeIntakeFormed
	case ports.RecordPendingIdentification:
		return UnitPendingIdentification
	case ports.RecordIntakeNotFormed:
		return NodeIntakeNotFormed
	case ports.RecordReceptionUndecided:
		return ReceptionUndecided
	default:
		return ReceptionOutcomeInvalid
	}
}

// handOffIntake 交发布意图。只有形成了可供下游消费的收寄（关联收寄）才交——待识别、未收寄
// 与待确认没有 PS 能采用的东西；失败不翻结果，留续办引用重发同一份（AT-NO-027），入队的错一并交出，提交还是回滚由调用方
// 按有没有人读续办来定。
func handOffIntake(
	ctx context.Context,
	downstream ports.NodeIntakeHandoff,
	record ports.ReceptionRecord,
) (string, error) {
	if record.Kind != ports.RecordIntakeFormed {
		return "", nil
	}
	err := downstream.HandOffNodeIntake(ctx, ports.NodeIntakeHandoffIntent{Record: record})
	if err == nil {
		return "", nil
	}
	return receptionContinuation("NODE_INTAKE_HANDOFF", record.Key.TenantID.String(), record.Key.SourceID), err
}

func receptionContinuation(parts ...string) string {
	digest := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return "CONT-" + hex.EncodeToString(digest[:8])
}
