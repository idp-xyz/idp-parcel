package application

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
)

// ErrUnexpectedCaseSave 说明案件库交回了封闭集合以外的写入结果。
var ErrUnexpectedCaseSave = errors.New("customs compliance: unexpected customs case save outcome")

// EstablishCaseOutcome 是建案请求的应用处理结果（UC-CC-001 的裁决分格：已建立、重复、
// 冲突、不适用、未受理、未决各占一格）。`包裹未接受`是未受理里单拆出来的一格：恢复动作不同——
// 改请求换不来它，要等那件包裹所在的委托被接受（UC-CC-001 启动条件），或指名对包裹。
type EstablishCaseOutcome uint8

const (
	EstablishCaseOutcomeInvalid EstablishCaseOutcome = iota
	CaseEstablished
	CaseExisting
	CaseScopeConflict
	CaseNotRequired
	EstablishCaseNotAccepted
	EstablishCaseUndecided
	CaseParcelNotAccepted
)

func (outcome EstablishCaseOutcome) String() string {
	switch outcome {
	case CaseEstablished:
		return "ESTABLISHED"
	case CaseExisting:
		return "EXISTING"
	case CaseScopeConflict:
		return "SCOPE_CONFLICT"
	case CaseNotRequired:
		return "NOT_REQUIRED"
	case EstablishCaseNotAccepted:
		return "NOT_ACCEPTED"
	case EstablishCaseUndecided:
		return "UNDECIDED"
	case CaseParcelNotAccepted:
		return "PARCEL_NOT_ACCEPTED"
	default:
		return ""
	}
}

// EstablishCaseUndecidedReason 指名建案停在哪一步等谁。建案要求规则未登记与读不动分两格：前者是租户还没配，
// 后者是依赖故障，续办的人不同。
type EstablishCaseUndecidedReason uint8

const (
	EstablishCaseUndecidedReasonNone EstablishCaseUndecidedReason = iota
	CaseRequirementUnavailable
	CaseRequirementUnconfigured
	CaseStoreUnavailable
	CaseIdentityUnavailable
	AcceptedParcelSourceUnavailable
)

func (reason EstablishCaseUndecidedReason) String() string {
	switch reason {
	case CaseRequirementUnavailable:
		return "CASE_REQUIREMENT_UNAVAILABLE"
	case CaseRequirementUnconfigured:
		return "CASE_REQUIREMENT_UNCONFIGURED"
	case CaseStoreUnavailable:
		return "CASE_STORE_UNAVAILABLE"
	case CaseIdentityUnavailable:
		return "CASE_IDENTITY_UNAVAILABLE"
	case AcceptedParcelSourceUnavailable:
		return "ACCEPTED_PARCEL_SOURCE_UNAVAILABLE"
	default:
		return ""
	}
}

// EstablishCaseCommand 携带一次建案请求：拟建监管范围四维、包裹关联与初始角色快照。
type EstablishCaseCommand struct {
	TenantID     domain.TenantID
	Jurisdiction domain.RegulatoryJurisdictionReference
	Direction    domain.ManifestDirection
	Procedure    domain.CustomsProcedureReference
	Obligation   domain.ObligationScopeReference
	Parcels      []domain.CaseParcelAssociation
	Roles        []domain.CaseRoleSnapshot
	RequestedAt  time.Time
}

type EstablishCaseResult struct {
	outcome     EstablishCaseOutcome
	reason      EstablishCaseUndecidedReason
	customsCase domain.CustomsCase
	hasCase     bool
	basis       string
	handoffRef  string
	unaccepted  []string
}

func (result EstablishCaseResult) Outcome() EstablishCaseOutcome {
	return result.outcome
}

// UndecidedReason 只在`未决`时非零。
func (result EstablishCaseResult) UndecidedReason() EstablishCaseUndecidedReason {
	return result.reason
}

// UnacceptedParcels 只在`包裹未接受`时非空：请求里不在任何当前已接受委托里的那几件，按请求顺序。
func (result EstablishCaseResult) UnacceptedParcels() []string {
	return append([]string(nil), result.unaccepted...)
}

func (result EstablishCaseResult) Case() (domain.CustomsCase, bool) {
	return result.customsCase, result.hasCase
}

// Basis 只在不适用格给出——当前服务责任不要求建案的判断依据。
func (result EstablishCaseResult) Basis() string {
	return result.basis
}

// HandoffReference 非空说明案件已入册但意图还没交出去，重放会重发同一份。
func (result EstablishCaseResult) HandoffReference() string {
	return result.handoffRef
}

type EstablishCaseDeps struct {
	Requirement ports.CaseRequirementView
	Store       ports.CustomsCaseStore
	Identity    ports.CaseIdentityFactory
	Downstream  ports.CustomsCaseHandoff
	Clock       ports.Clock
}

type EstablishCaseHandler struct {
	deps EstablishCaseDeps
}

func NewEstablishCaseHandler(deps EstablishCaseDeps) *EstablishCaseHandler {
	return &EstablishCaseHandler{deps: deps}
}

// Handle 把一次建案请求推进到固定监管范围的责任容器：要不要建案先判（规则未登记
// 未决——不是「不要求」；明确不要求是不适用格带依据）→ 幂等按身份键四维（同一法律
// 行为一案：同键同包裹集重放返原，同键异包裹集是范围冲突不顶替——同袋同总单同班次
// 都不能自动证明同一案件，扩大范围要走案件自己的变更）→ 领域建立 → 提交与意图。
func (handler *EstablishCaseHandler) Handle(
	ctx context.Context,
	command EstablishCaseCommand,
) (EstablishCaseResult, error) {
	if strings.TrimSpace(command.TenantID.String()) == "" ||
		strings.TrimSpace(command.Jurisdiction.String()) == "" ||
		strings.TrimSpace(command.Procedure.String()) == "" ||
		strings.TrimSpace(command.Obligation.String()) == "" {
		return EstablishCaseResult{outcome: EstablishCaseNotAccepted}, nil
	}

	judgment, configured, err := handler.deps.Requirement.JudgeCaseRequirement(
		ctx, command.TenantID, command.Jurisdiction, command.Direction, command.Procedure)
	if err != nil {
		return establishUndecided(CaseRequirementUnavailable), nil
	}
	if !configured {
		return establishUndecided(CaseRequirementUnconfigured), nil
	}
	if !judgment.Required {
		return EstablishCaseResult{outcome: CaseNotRequired, basis: judgment.Basis}, nil
	}

	key := ports.CustomsCaseKey{
		TenantID:     command.TenantID,
		Jurisdiction: command.Jurisdiction,
		Direction:    command.Direction,
		Procedure:    command.Procedure,
		Obligation:   command.Obligation,
	}
	existing, found, err := handler.deps.Store.FindByKey(ctx, key)
	if err != nil {
		return establishUndecided(CaseStoreUnavailable), nil
	}
	if found {
		return handler.settleAgainstExisting(existing, command), nil
	}

	caseID, err := handler.deps.Identity.MintCaseID(ctx)
	if err != nil {
		return establishUndecided(CaseIdentityUnavailable), nil
	}
	customsCase, err := domain.EstablishCustomsCase(domain.CustomsCaseSpec{
		ID:            caseID,
		Jurisdiction:  command.Jurisdiction,
		Direction:     command.Direction,
		Procedure:     command.Procedure,
		Obligation:    command.Obligation,
		Parcels:       command.Parcels,
		Roles:         command.Roles,
		EstablishedAt: handler.deps.Clock.Now(),
	})
	if err != nil {
		return EstablishCaseResult{outcome: EstablishCaseNotAccepted}, nil
	}

	saved, err := handler.deps.Store.Save(ctx, key, customsCase)
	if err != nil {
		return establishUndecided(CaseStoreUnavailable), nil
	}
	switch saved {
	case ports.CustomsCaseSaved:
		result := EstablishCaseResult{outcome: CaseEstablished, customsCase: customsCase, hasCase: true}
		result.handoffRef = handler.handOffCase(ctx, key, customsCase)
		return result, nil
	case ports.CustomsCaseAlreadyRecorded:
		winner, found, err := handler.deps.Store.FindByKey(ctx, key)
		if err != nil || !found {
			return establishUndecided(CaseStoreUnavailable), nil
		}
		return handler.settleAgainstExisting(winner, command), nil
	default:
		return EstablishCaseResult{}, fmt.Errorf("%w: %d", ErrUnexpectedCaseSave, saved)
	}
}

func establishUndecided(reason EstablishCaseUndecidedReason) EstablishCaseResult {
	return EstablishCaseResult{outcome: EstablishCaseUndecided, reason: reason}
}

// settleAgainstExisting 分辨重复与范围冲突：同包裹集是重放（返原案件），异包裹集是
// 范围冲突——建案入口不吸收范围扩大。
func (handler *EstablishCaseHandler) settleAgainstExisting(
	existing domain.CustomsCase,
	command EstablishCaseCommand,
) EstablishCaseResult {
	if parcelSetOf(existing.Parcels()) != parcelSetOf(command.Parcels) {
		return EstablishCaseResult{outcome: CaseScopeConflict}
	}
	return EstablishCaseResult{outcome: CaseExisting, customsCase: existing, hasCase: true}
}

// parcelSetOf 交回包裹关联集的顺序无关指纹。
func parcelSetOf(associations []domain.CaseParcelAssociation) string {
	parcels := make([]string, 0, len(associations))
	for _, association := range associations {
		parcels = append(parcels, association.Parcel)
	}
	sort.Strings(parcels)
	return strings.Join(parcels, "\x00")
}

// handOffCase 交发布意图。失败不翻案件，留续办引用重发同一份。
func (handler *EstablishCaseHandler) handOffCase(
	ctx context.Context,
	key ports.CustomsCaseKey,
	customsCase domain.CustomsCase,
) string {
	if err := handler.deps.Downstream.HandOffCase(ctx, ports.CustomsCaseHandoffIntent{
		Key:  key,
		Case: customsCase,
	}); err == nil {
		return ""
	}
	return "CONT-CASE/" + customsCase.ID().String()
}
