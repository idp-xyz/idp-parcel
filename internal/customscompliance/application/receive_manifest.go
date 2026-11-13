package application

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
)

// ErrUnexpectedManifestSave 说明舱单库交回了封闭集合以外的写入结果。
var ErrUnexpectedManifestSave = errors.New("customs compliance: unexpected manifest save outcome")

// ManifestOutcome 是外部舱单引用管理请求的应用处理结果。
type ManifestOutcome uint8

const (
	ManifestOutcomeInvalid ManifestOutcome = iota
	ManifestAssociated
	ManifestPendingAssociation
	ManifestExisting
	ManifestVersionConflict
	ManifestRevised
	ManifestNotFound
	ManifestNotAccepted
	ManifestUndecided
	// ManifestContentConflict：同一来源身份与版本再次到达，程序、方向或范围却与在册的不同。不按后到的覆盖，也不当重放
	// 答已有——来源要么走版本推进口声明变更关系，要么由人复核。
	ManifestContentConflict
)

func (outcome ManifestOutcome) String() string {
	switch outcome {
	case ManifestAssociated:
		return "ASSOCIATED"
	case ManifestPendingAssociation:
		return "PENDING_ASSOCIATION"
	case ManifestExisting:
		return "EXISTING"
	case ManifestVersionConflict:
		return "VERSION_CONFLICT"
	case ManifestRevised:
		return "REVISED"
	case ManifestNotFound:
		return "NOT_FOUND"
	case ManifestNotAccepted:
		return "NOT_ACCEPTED"
	case ManifestUndecided:
		return "UNDECIDED"
	case ManifestContentConflict:
		return "CONTENT_CONFLICT"
	default:
		return ""
	}
}

// ReceiveManifestCommand 携带一次舱单引用接收请求。
type ReceiveManifestCommand struct {
	TenantID domain.TenantID
	Spec     domain.ExternalManifestReferenceSpec
}

// ReviseManifestCommand 携带一次来源版本推进请求（承运商明确的更正、撤销、替代或
// 范围变化）。
type ReviseManifestCommand struct {
	TenantID domain.TenantID
	// Carrier 是这次推进的来源所代表的承运商责任。引用按（承运商责任，舱单身份）成行，所以别家承运商推进同一个号时
	// 读到的是自己编号空间里的那一格：没有就答不在册，碰不到别家的引用。
	Carrier    domain.CarrierResponsibilityReference
	Manifest   domain.ExternalManifestID
	Version    domain.ManifestSourceVersion
	Scope      domain.DecisionScopeReference
	SourceFact string
	At         time.Time
}

type ManifestResult struct {
	outcome    ManifestOutcome
	reference  domain.ExternalManifestReference
	hasRecord  bool
	handoffRef string
}

func (result ManifestResult) Outcome() ManifestOutcome {
	return result.outcome
}

func (result ManifestResult) Reference() (domain.ExternalManifestReference, bool) {
	return result.reference, result.hasRecord
}

// HandoffReference 非空说明引用已入册但意图还没交出去，重放会重发同一份。
func (result ManifestResult) HandoffReference() string {
	return result.handoffRef
}

type ReceiveManifestDeps struct {
	Candidates ports.ManifestCandidateView
	Store      ports.ManifestStore
	Downstream ports.ManifestHandoff
	Clock      ports.Clock
}

type ReceiveManifestHandler struct {
	deps ReceiveManifestDeps
}

func NewReceiveManifestHandler(deps ReceiveManifestDeps) *ReceiveManifestHandler {
	return &ReceiveManifestHandler{deps: deps}
}

// Receive 接受一份外部舱单引用并尝试唯一匹配关联：程序、方向与范围三维都相符的候选
// 恰一个才关联，零个或多个都保持待关联入册（不创建占位对象、不按最近客户或班次猜测
// ——258）；同舱单同版本重放返原、同舱单异版本经 Receive 是冲突（版本推进走 Revise，
// 承运商变化要走它自己的入口才留得下前版指回）。
func (handler *ReceiveManifestHandler) Receive(
	ctx context.Context,
	command ReceiveManifestCommand,
) (ManifestResult, error) {
	if strings.TrimSpace(command.TenantID.String()) == "" {
		return ManifestResult{outcome: ManifestNotAccepted}, nil
	}
	reference, err := domain.AcceptManifestReference(command.Spec)
	if err != nil {
		return ManifestResult{outcome: ManifestNotAccepted}, nil
	}

	// 引用按（承运商责任，舱单身份）找：别家承运商用同一个号是它自己编号空间里的另一条引用，在这里读不到，也就既探不出
	// 别家的舱单在不在册，也占不住别家的号（UC-CC-012「越权来源未受理且不得确认候选对象是否存在」）。
	existing, found, err := handler.deps.Store.FindByManifest(ctx, command.TenantID, reference.Carrier(), reference.Manifest())
	if err != nil {
		return ManifestResult{outcome: ManifestUndecided}, nil
	}
	if found {
		return answerRecordedManifest(existing, reference), nil
	}

	associated, outcome, err := handler.associate(ctx, command.TenantID, reference, reference.Procedure())
	if err != nil {
		return ManifestResult{outcome: ManifestUndecided}, nil
	}

	saved, err := handler.deps.Store.Save(ctx, command.TenantID, associated)
	if err != nil {
		return ManifestResult{outcome: ManifestUndecided}, nil
	}
	switch saved {
	case ports.ManifestSaved:
		result := ManifestResult{outcome: outcome, reference: associated, hasRecord: true}
		result.handoffRef = handler.handOffManifest(ctx, command.TenantID, associated)
		return result, nil
	case ports.ManifestAlreadyRecorded:
		winner, found, err := handler.deps.Store.FindByManifest(ctx, command.TenantID, reference.Carrier(), reference.Manifest())
		if err != nil || !found {
			return ManifestResult{outcome: ManifestUndecided}, nil
		}
		return answerRecordedManifest(winner, reference), nil
	default:
		return ManifestResult{}, fmt.Errorf("%w: %d", ErrUnexpectedManifestSave, saved)
	}
}

// Revise 依据承运商明确的来源变化推进版本：原引用与历史关联由领域保留在新引用内，
// 新版本重新走唯一匹配（关联不随版本自动搬移）。
func (handler *ReceiveManifestHandler) Revise(
	ctx context.Context,
	command ReviseManifestCommand,
) (ManifestResult, error) {
	if strings.TrimSpace(command.TenantID.String()) == "" {
		return ManifestResult{outcome: ManifestNotAccepted}, nil
	}
	current, found, err := handler.deps.Store.FindByManifest(ctx, command.TenantID, command.Carrier, command.Manifest)
	if err != nil {
		return ManifestResult{outcome: ManifestUndecided}, nil
	}
	if !found {
		return ManifestResult{outcome: ManifestNotFound}, nil
	}
	if current.Version() == command.Version {
		return ManifestResult{outcome: ManifestExisting, reference: current, hasRecord: true}, nil
	}

	revised, err := current.Revise(command.Version, command.Scope, command.SourceFact, command.At)
	if err != nil {
		return ManifestResult{outcome: ManifestNotAccepted}, nil
	}
	// 新版本带着新范围重新走唯一匹配（关联不随版本自动搬移——领域已清空）。
	associated, _, err := handler.associate(ctx, command.TenantID, revised, revised.Procedure())
	if err != nil {
		return ManifestResult{outcome: ManifestUndecided}, nil
	}
	updated, err := handler.deps.Store.Update(ctx, command.TenantID, associated)
	if err != nil {
		return ManifestResult{outcome: ManifestUndecided}, nil
	}
	switch updated {
	case ports.ManifestAdvanced:
	case ports.ManifestVersionMoved:
		// 读到前版之后、落新版之前，另一笔推进先落了：这一笔指回的前版已经不是在册的那一版，落下去就会静默盖掉先到
		// 的版本。答版本冲突，由来源按现在的在册版本重新声明变更关系。
		return ManifestResult{outcome: ManifestVersionConflict}, nil
	default:
		return ManifestResult{}, fmt.Errorf("%w: update %d", ErrUnexpectedManifestSave, updated)
	}
	result := ManifestResult{outcome: ManifestRevised, reference: associated, hasRecord: true}
	result.handoffRef = handler.handOffManifest(ctx, command.TenantID, associated)
	return result, nil
}

// answerRecordedManifest 对「这家承运商的这份舱单已经在册」作答：版本不同是版本冲突（推进走 Revise）；版本相同而程序、
// 方向或范围不同是内容冲突，不交回在册内容也不覆盖；都相同才是重放，交回在册那一份。来源事实引用不比——同一份舱单
// 重发时报文身份本来就会换。
func answerRecordedManifest(recorded, arrived domain.ExternalManifestReference) ManifestResult {
	if recorded.Version() != arrived.Version() {
		return ManifestResult{outcome: ManifestVersionConflict}
	}
	if recorded.Procedure() != arrived.Procedure() || recorded.Direction() != arrived.Direction() || recorded.Scope() != arrived.Scope() {
		return ManifestResult{outcome: ManifestContentConflict}
	}
	return ManifestResult{outcome: ManifestExisting, reference: recorded, hasRecord: true}
}

// associate 盘候选并尝试唯一匹配。匹配不了不是错误——引用保持待关联入册。
func (handler *ReceiveManifestHandler) associate(
	ctx context.Context,
	tenant domain.TenantID,
	reference domain.ExternalManifestReference,
	procedure domain.CustomsProcedureReference,
) (domain.ExternalManifestReference, ManifestOutcome, error) {
	candidates, err := handler.deps.Candidates.LoadAssociationCandidates(
		ctx, tenant, procedure, reference.Direction())
	if err != nil {
		return domain.ExternalManifestReference{}, ManifestOutcomeInvalid, fmt.Errorf("load candidates: %w", err)
	}
	associated, err := reference.Associate(candidates)
	if err != nil {
		if errors.Is(err, domain.ErrManifestNotUniquelyMatched) {
			return reference, ManifestPendingAssociation, nil
		}
		return domain.ExternalManifestReference{}, ManifestOutcomeInvalid, err
	}
	return associated, ManifestAssociated, nil
}

// handOffManifest 交发布意图。失败不翻结果，留续办引用重发同一份。
func (handler *ReceiveManifestHandler) handOffManifest(
	ctx context.Context,
	tenant domain.TenantID,
	reference domain.ExternalManifestReference,
) string {
	if err := handler.deps.Downstream.HandOffManifest(ctx, ports.ManifestHandoffIntent{
		TenantID:  tenant,
		Reference: reference,
	}); err == nil {
		return ""
	}
	return "CONT-MANIFEST/" + reference.Manifest().String() + "/" + reference.Version().String()
}
