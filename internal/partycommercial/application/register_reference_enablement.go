package application

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
)

// ReferenceEnablementOutcome 是启用范围登记的结果代数，判据同 RegistrationNumberTypeOutcome：`已登记`是本次落库；
// `重复`按同键同内容重放返回原结果；`内容冲突`要登记方对着册面修正、绝不覆盖（ADR-0031）；`未受理`是输入被领域门或
// 连续性检查拒绝，一个字节没写，并且一定带一个封闭的拒绝理由。
type ReferenceEnablementOutcome uint8

const (
	ReferenceEnablementOutcomeInvalid ReferenceEnablementOutcome = iota
	ReferenceEnablementRegistered
	ReferenceEnablementAlreadyRegistered
	ReferenceEnablementContentConflict
	ReferenceEnablementNotAccepted
)

func (outcome ReferenceEnablementOutcome) String() string {
	switch outcome {
	case ReferenceEnablementRegistered:
		return "REGISTERED"
	case ReferenceEnablementAlreadyRegistered:
		return "ALREADY_REGISTERED"
	case ReferenceEnablementContentConflict:
		return "CONTENT_CONFLICT"
	case ReferenceEnablementNotAccepted:
		return "NOT_ACCEPTED"
	default:
		return ""
	}
}

// ReferenceEnablementRefusal 指名`未受理`差在哪一格。逐格分开是因为恢复动作各不相同：目录里没有这个码要换码，
// 修订错位要重读册面，生效时点早于前一修订要改时点；折成一格会让人去改错东西。
type ReferenceEnablementRefusal uint8

const (
	ReferenceEnablementRefusalNone ReferenceEnablementRefusal = iota
	ReferenceEnablementCodeNotInCatalogue
	ReferenceEnablementRevisionOutOfSequence
	ReferenceEnablementEffectiveBeforePrior
	ReferenceEnablementInputNotWellFormed
)

func (refusal ReferenceEnablementRefusal) String() string {
	switch refusal {
	case ReferenceEnablementCodeNotInCatalogue:
		return "CODE_NOT_IN_CATALOGUE"
	case ReferenceEnablementRevisionOutOfSequence:
		return "REVISION_OUT_OF_SEQUENCE"
	case ReferenceEnablementEffectiveBeforePrior:
		return "EFFECTIVE_BEFORE_PRIOR"
	case ReferenceEnablementInputNotWellFormed:
		return "INPUT_NOT_WELL_FORMED"
	default:
		return ""
	}
}

// ReferenceEnablementResult 携带落点与`未受理`的理由（输入被拒不是技术失败，事务保持可用）。
type ReferenceEnablementResult struct {
	outcome ReferenceEnablementOutcome
	refusal ReferenceEnablementRefusal
	cause   error
}

func (result ReferenceEnablementResult) Outcome() ReferenceEnablementOutcome { return result.outcome }

// Refusal 只在`未受理`时非空。
func (result ReferenceEnablementResult) Refusal() ReferenceEnablementRefusal { return result.refusal }

// Cause 是散文，只在`未受理`时非空；判别靠 Refusal，不靠它。
func (result ReferenceEnablementResult) Cause() error { return result.cause }

// referenceEnablementNotAccepted 把领域门的拒绝译成封闭理由，散文原因写给登记方看（中文，带上他送来的值）。
func referenceEnablementNotAccepted(
	err error,
	command RegisterReferenceEnablementCommand,
	latest domain.ReferenceEnablementRegistration,
	hasLatest bool,
) ReferenceEnablementResult {
	refusal := ReferenceEnablementInputNotWellFormed
	cause := fmt.Errorf("登记内容不成立：%w", err)
	switch {
	case errors.Is(err, domain.ErrReferenceCodeNotInCatalogue):
		refusal = ReferenceEnablementCodeNotInCatalogue
		cause = fmt.Errorf("目录里没有这个码：%s %s——登记的码只能取自产品内置的国家 / 地区或币种目录", command.Kind.Label(), command.Code)
	case errors.Is(err, domain.ErrReferenceEnablementOutOfSequence):
		refusal = ReferenceEnablementRevisionOutOfSequence
		if hasLatest {
			cause = fmt.Errorf("修订必须连续：册上最新为 %d，收到 %d", latest.Revision(), command.Revision)
		} else {
			cause = fmt.Errorf("首笔登记修订必须是 1，收到 %d", command.Revision)
		}
	case errors.Is(err, domain.ErrReferenceEnablementEffectiveBeforePrior):
		refusal = ReferenceEnablementEffectiveBeforePrior
		cause = fmt.Errorf("生效时点不得早于前一修订的 %s，收到 %s",
			latest.EffectiveFrom().Format(time.RFC3339), command.EffectiveFrom.UTC().Format(time.RFC3339))
	}
	return ReferenceEnablementResult{outcome: ReferenceEnablementNotAccepted, refusal: refusal, cause: cause}
}

// RegisterReferenceEnablementCommand 登记启用范围的一笔修订。Registrant 取自认证信封，不取自载荷。
type RegisterReferenceEnablementCommand struct {
	Tenant        domain.TenantID
	Kind          domain.ReferenceKind
	Code          domain.ReferenceCode
	Revision      int
	Status        domain.ReferenceEnablementStatus
	EffectiveFrom time.Time
	Source        domain.ReferenceEnablementSource
	Registrant    domain.OperatorSubjectReference
}

// RegisterReferenceEnablementHandler 是启用范围登记册的写侧编排：领域门（含目录成员）→ 修订连续性 → 落库。
// 调用方逐命令各起事务。
type RegisterReferenceEnablementHandler struct {
	registry ports.ReferenceEnablementRegistry
	clock    ports.Clock
}

func NewRegisterReferenceEnablementHandler(
	registry ports.ReferenceEnablementRegistry,
	clock ports.Clock,
) *RegisterReferenceEnablementHandler {
	return &RegisterReferenceEnablementHandler{registry: registry, clock: clock}
}

// Register 登记一笔启用或停用。只登记、读回与展示：本用例不拦任何别处的写入——某个码启用没启用，此刻没有哪个写口读它
// （启用范围的强制是后续轮次）。
//
// 修订号小于等于册上最新的，是对一个已在册修订的重放或冲突，交给落库那一步按内容比；大于的才查连续性与生效时点。
// 目录成员这道门有一个例外：册上已有这个码的行、而它后来被目录删了（ISO 废止），重放与追加停用照常受理，新启用与首次
// 登记不受理（RetiredCodeMayStillBeTouched）。
func (handler *RegisterReferenceEnablementHandler) Register(
	ctx context.Context,
	command RegisterReferenceEnablementCommand,
) (ReferenceEnablementResult, error) {
	spec := domain.ReferenceEnablementSpec{
		Status:        command.Status,
		EffectiveFrom: command.EffectiveFrom,
		Source:        command.Source,
	}
	recordedAt := handler.clock.Now()
	candidate, err := domain.NewReferenceEnablementRegistration(
		command.Tenant, command.Kind, command.Code, command.Revision, spec, command.Registrant, recordedAt)
	retired := errors.Is(err, domain.ErrReferenceCodeNotInCatalogue)
	if retired {
		// 先只验形状：是不是目录外的老码、放不放行，要看册上有没有它的行。
		candidate, err = domain.RehydrateReferenceEnablementRegistration(
			command.Tenant, command.Kind, command.Code, command.Revision, spec, command.Registrant, recordedAt)
	}
	if err != nil {
		return referenceEnablementNotAccepted(err, command, domain.ReferenceEnablementRegistration{}, false), nil
	}

	latest, found, err := handler.registry.LoadLatestReferenceEnablement(ctx, command.Tenant, command.Kind, command.Code)
	if err != nil {
		return ReferenceEnablementResult{}, fmt.Errorf("register reference enablement: %w", err)
	}
	if retired && !(found && domain.RetiredCodeMayStillBeTouched(latest, command.Revision, command.Status)) {
		return referenceEnablementNotAccepted(domain.ErrReferenceCodeNotInCatalogue, command, latest, found), nil
	}
	if !found || command.Revision > latest.Revision() {
		if err := candidate.FollowsFrom(latest, found); err != nil {
			return referenceEnablementNotAccepted(err, command, latest, found), nil
		}
	}

	outcome, err := handler.registry.SaveReferenceEnablement(ctx, candidate)
	if err != nil {
		return ReferenceEnablementResult{}, fmt.Errorf("register reference enablement: %w", err)
	}
	switch outcome {
	case ports.ReferenceEnablementSaved:
		return ReferenceEnablementResult{outcome: ReferenceEnablementRegistered}, nil
	case ports.ReferenceEnablementAlreadyRegistered:
		return ReferenceEnablementResult{outcome: ReferenceEnablementAlreadyRegistered}, nil
	case ports.ReferenceEnablementContentConflict:
		return ReferenceEnablementResult{outcome: ReferenceEnablementContentConflict}, nil
	default:
		return ReferenceEnablementResult{}, fmt.Errorf("register reference enablement: unexpected save outcome %d", outcome)
	}
}

// ReferenceEnablementEntry 是读面上一个码的一行：此刻的状态（Standing，没有生效的行即未配置）加全部修订。
type ReferenceEnablementEntry struct {
	Code     domain.ReferenceCode
	Standing domain.ReferenceEnablementRegistration
	// HasStanding 为假就是“未配置”：册上有行但全部未到生效时点。它不是停用，也不是启用。
	HasStanding bool
	History     []domain.ReferenceEnablementRegistration
}

// QueryReferenceEnablementsHandler 是启用范围的读侧：按租户与种类读出册上登过的每个码此刻的状态与全部修订。
// 册上没有行的码不在结果里——它们的状态是“未配置”，由缺席表达。
type QueryReferenceEnablementsHandler struct {
	registry ports.ReferenceEnablementRegistry
	clock    ports.Clock
}

func NewQueryReferenceEnablementsHandler(
	registry ports.ReferenceEnablementRegistry,
	clock ports.Clock,
) *QueryReferenceEnablementsHandler {
	return &QueryReferenceEnablementsHandler{registry: registry, clock: clock}
}

// ListReferenceEnablements 以读的这一刻为界导出状态；读不回是错误，不折成空册。
func (handler *QueryReferenceEnablementsHandler) ListReferenceEnablements(
	ctx context.Context,
	tenant domain.TenantID,
	kind domain.ReferenceKind,
) (time.Time, []ReferenceEnablementEntry, error) {
	asOf := handler.clock.Now().UTC()
	rows, err := handler.registry.ListReferenceEnablements(ctx, tenant, kind)
	if err != nil {
		return time.Time{}, nil, fmt.Errorf("list reference enablements: %w", err)
	}

	byCode := map[string][]domain.ReferenceEnablementRegistration{}
	for _, row := range rows {
		byCode[row.Code().String()] = append(byCode[row.Code().String()], row)
	}
	codes := make([]string, 0, len(byCode))
	for code := range byCode {
		codes = append(codes, code)
	}
	sort.Strings(codes)

	entries := make([]ReferenceEnablementEntry, 0, len(codes))
	for _, code := range codes {
		history := byCode[code]
		sort.Slice(history, func(i, j int) bool { return history[i].Revision() < history[j].Revision() })
		standing, hasStanding := domain.EnablementStandingAt(history, asOf)
		entries = append(entries, ReferenceEnablementEntry{
			Code:        history[0].Code(),
			Standing:    standing,
			HasStanding: hasStanding,
			History:     history,
		})
	}
	return asOf, entries, nil
}
