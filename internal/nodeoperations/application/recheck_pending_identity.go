package application

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
)

// ErrParcelIdentityUnreadable 说明身份核对读口没取得 PS 的答复。收寄与重核都据此不形成答案；重核的消费门把它记成等依赖，
// 信封留在 inbox 退避重投（ADR-0199 决定三「身份核对仍读不动 → 不追加，交回 error」）。
var ErrParcelIdentityUnreadable = errors.New("node operations: parcel identity could not be read")

// ErrRecheckSubjectNotFound 说明指名重核的那一版判断不在本租户的收寄册上。不区分「没有」与「属于别的租户」（ADR-0022 统一
// 不可见结果）。
var ErrRecheckSubjectNotFound = errors.New("node operations: the named judgment is not in this tenant's reception register")

// ErrRecheckCommandRejected 说明重核命令本身缺件（租户、判断版本或号码），重发同样的内容不会变好。
var ErrRecheckCommandRejected = errors.New("node operations: identity recheck command rejected")

// RecheckOutcome 是一次重核的应用结果（ADR-0199 决定三分格）。前两格追加了判断版本，第三格判断不变，第四格是重放，后三格
// 说的是为什么不重核——它们都是形成了的答案，不是错误。
type RecheckOutcome uint8

const (
	RecheckOutcomeInvalid RecheckOutcome = iota
	// RecheckIntakeFormed：唯一且包裹没有关联到别的实物，追加了一版`节点收寄已形成`。
	RecheckIntakeFormed
	// RecheckIdentityConflict：多候选、带歧义信号的一个候选，或唯一但包裹已关联到别的实物，追加了冲突版。
	RecheckIdentityConflict
	// RecheckStillUnidentified：仍是零候选（带不带歧义信号），判断不变、不追加。
	RecheckStillUnidentified
	// RecheckExistingResult：被重核那一版已有后继，或派生键已在册（并发里另一方先追加）；答当前有效那一版，不再追加。
	RecheckExistingResult
	// RecheckIdentityConflictRecorded：指名的待识别记着身份冲突。身份冲突只经人工处置解除（ADR-0210 决定一），不经重核。
	RecheckIdentityConflictRecorded
	// RecheckSourceNotPreserved：指名的是来源保全之前落下的行，号码从未留存，无从按号再问。
	RecheckSourceNotPreserved
	// RecheckNotPendingIdentification：指名的那一版不是待识别实物（已形成、未收寄或待确认）。
	RecheckNotPendingIdentification
)

func (outcome RecheckOutcome) String() string {
	switch outcome {
	case RecheckIntakeFormed:
		return "INTAKE_FORMED"
	case RecheckIdentityConflict:
		return "IDENTITY_CONFLICT"
	case RecheckStillUnidentified:
		return "STILL_UNIDENTIFIED"
	case RecheckExistingResult:
		return "EXISTING_RESULT"
	case RecheckIdentityConflictRecorded:
		return "IDENTITY_CONFLICT_RECORDED"
	case RecheckSourceNotPreserved:
		return "SOURCE_NOT_PRESERVED"
	case RecheckNotPendingIdentification:
		return "NOT_PENDING_IDENTIFICATION"
	default:
		return ""
	}
}

// RecheckPendingIdentityCommand 指名一件待识别实物重核：租户只取认证结果或信封，判断版本是收寄册上那一版。
type RecheckPendingIdentityCommand struct {
	TenantID domain.TenantID
	Version  domain.IntakeResultVersion
}

// RecheckPendingIdentityResult 是一次重核的结果。Record 交回这一格答复所说的那一版：追加的那一版（前两格），已有结果的当前有效
// 那一版（第四格），其余是被重核的那一版本身。
type RecheckPendingIdentityResult struct {
	outcome   RecheckOutcome
	rechecked domain.IntakeResultVersion
	record    ports.ReceptionRecord
	hasRecord bool
}

func (result RecheckPendingIdentityResult) Outcome() RecheckOutcome { return result.outcome }

// Rechecked 是被重核的那一版判断版本。
func (result RecheckPendingIdentityResult) Rechecked() domain.IntakeResultVersion {
	return result.rechecked
}

func (result RecheckPendingIdentityResult) Record() (ports.ReceptionRecord, bool) {
	return result.record, result.hasRecord
}

// RecheckPendingIdentityDeps 是重核编排的全部依赖，缺任一个构造即拒。身份核对、服务结果、收寄库、签发与意图交付与收寄编排
// 是同一组口：重核走的就是收寄步骤 4 那一份身份判断，追加的也是同一本册上的判断版本。
type RecheckPendingIdentityDeps struct {
	Identity       ports.ParcelIdentityView
	ServiceResults ports.ParcelServiceResultView
	Receptions     ports.ReceptionStore
	Pending        ports.PendingIdentityLookup
	Versions       ports.IntakeIdentityFactory
	Downstream     ports.NodeIntakeHandoff
	Clock          ports.Clock
}

// RecheckPendingIdentityHandler 按存下的号重核待识别实物（ADR-0199 决定三，`AT-NO-026`）。两个入口：Recheck 只看指名那一件
// （复核页人工发起；落未核对行时自投的门铃也走它），RecheckByMark 看号码相等的每一件（PS 外部标识关联已形成的门铃）。两个
// 入口落到同一个 recheck，结果同形。
//
// 调用方负责事务：身份判断里的包裹锁、追加的那一版与它的意图必须同生共死，见 ports.ReceptionStore.FindOtherUnitIntakes。
type RecheckPendingIdentityHandler struct {
	deps RecheckPendingIdentityDeps
}

func NewRecheckPendingIdentityHandler(deps RecheckPendingIdentityDeps) (*RecheckPendingIdentityHandler, error) {
	switch {
	case deps.Identity == nil:
		return nil, errors.New("node operations: identity recheck needs a parcel identity view")
	case deps.ServiceResults == nil:
		return nil, errors.New("node operations: identity recheck needs a parcel service result view")
	case deps.Receptions == nil:
		return nil, errors.New("node operations: identity recheck needs a reception store")
	case deps.Pending == nil:
		return nil, errors.New("node operations: identity recheck needs a pending identity lookup")
	case deps.Versions == nil:
		return nil, errors.New("node operations: identity recheck needs an intake result version factory")
	case deps.Downstream == nil:
		return nil, errors.New("node operations: identity recheck needs a node intake handoff")
	case deps.Clock == nil:
		return nil, errors.New("node operations: identity recheck needs a clock")
	}
	return &RecheckPendingIdentityHandler{deps: deps}, nil
}

// Recheck 重核指名的那一件。重核对象的条件（当前有效版本是待识别、不带关联、不记身份冲突、来源已保全）在这里逐条判，判不过
// 的各答一格业务结果；指名本租户册上没有的版本交回 ErrRecheckSubjectNotFound。
func (handler *RecheckPendingIdentityHandler) Recheck(
	ctx context.Context,
	command RecheckPendingIdentityCommand,
) (RecheckPendingIdentityResult, error) {
	if strings.TrimSpace(command.TenantID.String()) == "" || strings.TrimSpace(command.Version.String()) == "" {
		return RecheckPendingIdentityResult{}, fmt.Errorf("%w: tenant and judgment version are required", ErrRecheckCommandRejected)
	}
	record, found, err := handler.deps.Pending.FindByJudgmentVersion(ctx, command.TenantID, command.Version)
	if err != nil {
		return RecheckPendingIdentityResult{}, fmt.Errorf("find the named judgment: %w", err)
	}
	if !found {
		return RecheckPendingIdentityResult{}, ErrRecheckSubjectNotFound
	}
	return handler.recheck(ctx, record)
}

// RecheckByMark 重核号码与 mark 逐字相等的每一件待识别（PS 门铃）。号码不在任何一件的来源文档里就什么也不做：PS 每形成一条
// 关联都响一次，从没在节点出现过的号是常态（ADR-0199 越权风险点 1）。逐件重核在同一笔事务里，任一件没形成答案就整笔交回
// error，信封留在 inbox 重投；已经重核过的那几件重投时不再是重核对象。
func (handler *RecheckPendingIdentityHandler) RecheckByMark(
	ctx context.Context,
	tenant domain.TenantID,
	mark string,
) ([]RecheckPendingIdentityResult, error) {
	if strings.TrimSpace(tenant.String()) == "" || mark == "" {
		return nil, fmt.Errorf("%w: tenant and mark are required", ErrRecheckCommandRejected)
	}
	records, err := handler.deps.Pending.FindRecheckableByMark(ctx, tenant, mark)
	if err != nil {
		return nil, fmt.Errorf("find pending units by mark: %w", err)
	}
	results := make([]RecheckPendingIdentityResult, 0, len(records))
	for _, record := range records {
		result, err := handler.recheck(ctx, record)
		if err != nil {
			return nil, fmt.Errorf("recheck %q: %w", record.Key.SourceID, err)
		}
		results = append(results, result)
	}
	return results, nil
}

// recheck 拿被重核那一版来源文档里的外部标识观察，走收寄步骤 4 同一份身份判断，按 ADR-0199 决定三分格追加。
func (handler *RecheckPendingIdentityHandler) recheck(
	ctx context.Context,
	record ports.ReceptionRecord,
) (RecheckPendingIdentityResult, error) {
	answered := func(outcome RecheckOutcome, about ports.ReceptionRecord) RecheckPendingIdentityResult {
		return RecheckPendingIdentityResult{outcome: outcome, rechecked: record.JudgmentVersion, record: about, hasRecord: true}
	}
	_, associated := record.Intake.Association()
	switch {
	case record.Kind != ports.RecordPendingIdentification || associated:
		return answered(RecheckNotPendingIdentification, record), nil
	case record.IdentityConflict:
		// 记身份冲突的（多候选、一码两物）不重核：身份冲突只经人工处置解除（ADR-0210 决定一），重核再问一次 PS 也替不了那次处置。
		return answered(RecheckIdentityConflictRecorded, record), nil
	case !record.Source.Preserved():
		return answered(RecheckSourceNotPreserved, record), nil
	}
	tenant := record.Key.TenantID
	current, err := handler.deps.Receptions.CurrentVersion(ctx, tenant, record.JudgmentVersion)
	if err != nil {
		return RecheckPendingIdentityResult{}, fmt.Errorf("current version of the pending judgment: %w", err)
	}
	if current != record.JudgmentVersion {
		head, found, err := handler.deps.Pending.FindByJudgmentVersion(ctx, tenant, current)
		if err != nil {
			return RecheckPendingIdentityResult{}, fmt.Errorf("read the current judgment: %w", err)
		}
		if !found {
			return RecheckPendingIdentityResult{}, fmt.Errorf("node operations: current judgment %q is not readable", current)
		}
		return answered(RecheckExistingResult, head), nil
	}

	judgment, err := identityJudge{
		identity:   handler.deps.Identity,
		services:   handler.deps.ServiceResults,
		receptions: handler.deps.Receptions,
		excluded:   record.Disposition.ExcludedFor(record.Intake.Unit()),
	}.judge(ctx, tenant, ports.ExternalMarkObservation{Mark: record.Source.Mark}, record.Intake.Unit())
	if err != nil {
		return RecheckPendingIdentityResult{}, err
	}
	if !judgment.identified && !judgment.conflict() {
		// 零候选（带不带歧义信号）：判断不变、不追加（ADR-0199 决定三）。唯一命中身份冲突处置排除过的那件也落在这里（ADR-0210 决定九）。
		return answered(RecheckStillUnidentified, record), nil
	}

	appended, err := handler.appendedVersion(ctx, record, judgment)
	if err != nil {
		return RecheckPendingIdentityResult{}, err
	}
	saved, err := handler.deps.Receptions.Save(ctx, appended)
	if err != nil {
		return RecheckPendingIdentityResult{}, fmt.Errorf("record the rechecked judgment: %w", err)
	}
	switch saved {
	case ports.ReceptionSaved:
	case ports.ReceptionAlreadyRecorded:
		// 派生键已在册：并发里另一方先追加了这一版，读回它作答。它的意图与它同一笔交出，这里不重发。
		winner, found, err := handler.deps.Receptions.FindByKey(ctx, appended.Key)
		if err != nil {
			return RecheckPendingIdentityResult{}, fmt.Errorf("read back the rechecked judgment: %w", err)
		}
		if !found {
			return RecheckPendingIdentityResult{}, errors.New("node operations: rechecked judgment already recorded but not readable")
		}
		return answered(RecheckExistingResult, winner), nil
	default:
		return RecheckPendingIdentityResult{}, fmt.Errorf("%w: %d", ErrUnexpectedReceptionSave, saved)
	}

	if judgment.identified {
		// 意图交不出去就不形成答案：重核的两个调用方（命令口、inbox 消费门）都没有人读续办引用，提交一版没交出意图的关联
		// 收寄，PS 就永远等不到它。交回 error 让整笔回滚，人工发起答 5xx、门铃留在 inbox 重投。
		if _, err := handOffIntake(ctx, handler.deps.Downstream, appended); err != nil {
			return RecheckPendingIdentityResult{}, fmt.Errorf("hand off the rechecked intake: %w", err)
		}
		return answered(RecheckIntakeFormed, appended), nil
	}
	if len(judgment.others) > 0 {
		if err := appendIdentityConflicts(ctx, handler.deps.Receptions, handler.deps.Versions, appended, judgment.others); err != nil {
			return RecheckPendingIdentityResult{}, err
		}
	}
	return answered(RecheckIdentityConflict, appended), nil
}

// appendedVersion 拼出重核追加的那一版。收寄事实与控制照被重核那一版：业务时间是原收寄时刻，控制起点与控制依据不变
// （ADR-0199 决定三「不另起控制起点」）；换一个判断版本，前版指名被重核那一版。摘要与来源文档沿用被重核那一版——这一版
// 由那次送站引起，万一有设备撞上派生的来源标识，指纹对不上，答来源冲突而不是重放。
func (handler *RecheckPendingIdentityHandler) appendedVersion(
	ctx context.Context,
	record ports.ReceptionRecord,
	judgment identityJudgment,
) (ports.ReceptionRecord, error) {
	version, err := handler.deps.Versions.NextIntakeResultVersion(ctx)
	if err != nil {
		return ports.ReceptionRecord{}, fmt.Errorf("next judgment version for the recheck: %w", err)
	}
	spec := domain.NodeIntakeSpec{
		TenantID:    record.Key.TenantID,
		Unit:        record.Intake.Unit(),
		Node:        record.Intake.Node(),
		DeliveredBy: record.Intake.DeliveredBy(),
		Evidence:    record.Intake.Evidence(),
		Version:     version,
		ReceivedAt:  record.Intake.ReceivedAt(),
	}
	appended := ports.ReceptionRecord{
		Key:              ports.ReceptionKey{TenantID: record.Key.TenantID, SourceID: identityRecheckSourceID(record.JudgmentVersion)},
		ContentDigest:    record.ContentDigest,
		Kind:             ports.RecordPendingIdentification,
		JudgmentVersion:  version,
		Predecessor:      record.JudgmentVersion,
		Source:           record.Source,
		Control:          record.Control,
		Candidates:       judgment.candidates,
		IdentityConflict: judgment.conflict(),
		RecordedAt:       handler.deps.Clock.Now(),
	}
	if judgment.identified {
		spec.Association = judgment.parcel
		appended.Kind = ports.RecordIntakeFormed
		appended.ServiceMarkers = judgment.markers
		appended.ServiceResultCheck = judgment.check
	}
	if appended.Intake, err = domain.FormNodeIntake(spec); err != nil {
		return ports.ReceptionRecord{}, fmt.Errorf("form the rechecked intake: %w", err)
	}
	return appended, nil
}

// identityRecheckSourceID 派生重核追加版的来源标识：由被重核那一版的判断版本派生，同一版再重核撞键即重放（ADR-0199 决定三）。
// 派生自判断版本而不是原来源标识，是为了长度有界——来源标识原样进收寄意图信封的 Subject，原来源标识的长度归设备，判断版本
// 由本上下文签发。前缀把它与设备和导入送来的来源标识分开，与 ADR-0196 冲突版的派生键同一手法。
func identityRecheckSourceID(rechecked domain.IntakeResultVersion) string {
	return "IDENTITY-RECHECK/" + rechecked.String()
}
