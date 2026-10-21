package application

import (
	"context"
	"strings"

	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
)

// AuthorizationConfirmationOutcome 是一次提交授权确认的应用处理结果（UC-CC-004 首发人工确认那一格，ADR-0227 决定三）。
// `已有授权`与`授权不再有效`各占一格：前者照旧可用、后者不得再支持提交，续办相反；本册一单元一行，后者今天没有再次
// 申请的路（票 product-strategy-boundary/36）。`不再就绪`是业务负向，与等依赖的`未决`分开（ADR-0029）。
type AuthorizationConfirmationOutcome uint8

const (
	AuthorizationConfirmationOutcomeInvalid AuthorizationConfirmationOutcome = iota
	SubmissionAuthorizationConfirmed
	SubmissionAuthorizationExisting
	SubmissionAuthorizationNoLongerEffective
	AuthorizationReadinessNotEffective
	AuthorizationConfirmationNotAccepted
	AuthorizationConfirmationUndecided
)

func (outcome AuthorizationConfirmationOutcome) String() string {
	switch outcome {
	case SubmissionAuthorizationConfirmed:
		return "AUTHORIZATION_CONFIRMED"
	case SubmissionAuthorizationExisting:
		return "AUTHORIZATION_EXISTING"
	case SubmissionAuthorizationNoLongerEffective:
		return "AUTHORIZATION_NO_LONGER_EFFECTIVE"
	case AuthorizationReadinessNotEffective:
		return "NOT_READY"
	case AuthorizationConfirmationNotAccepted:
		return "NOT_ACCEPTED"
	case AuthorizationConfirmationUndecided:
		return "AUTHORIZATION_UNDECIDED"
	default:
		return ""
	}
}

// AuthorizationConfirmationUndecidedReason 指名确认停在哪一步等谁。就绪没登与就绪读不动分两格：前者是这个单元还没有
// 就绪判断（租户的资格目录与就绪规则属实例半边），后者是依赖故障。
type AuthorizationConfirmationUndecidedReason uint8

const (
	AuthorizationConfirmationUndecidedReasonNone AuthorizationConfirmationUndecidedReason = iota
	ConfirmationReadinessUnavailable
	ConfirmationReadinessUnconfigured
	ConfirmationAuthorityStoreUnavailable
)

func (reason AuthorizationConfirmationUndecidedReason) String() string {
	switch reason {
	case ConfirmationReadinessUnavailable:
		return "READINESS_UNAVAILABLE"
	case ConfirmationReadinessUnconfigured:
		return "READINESS_UNCONFIGURED"
	case ConfirmationAuthorityStoreUnavailable:
		return "AUTHORITY_STORE_UNAVAILABLE"
	default:
		return ""
	}
}

// ConfirmSubmissionAuthorizationCommand 是一次授权确认：确认人与所凭的授予由接入渠道给（信封里认证出的操作者与这一口要的
// 运营决定种类），请求方只指名申报单元——确认人不从请求体收，不然谁都能替别人确认。
type ConfirmSubmissionAuthorizationCommand struct {
	TenantID  domain.TenantID
	UnitID    string
	Confirmer string
	Grant     string
}

type AuthorizationConfirmationResult struct {
	outcome       AuthorizationConfirmationOutcome
	reason        AuthorizationConfirmationUndecidedReason
	authorization domain.SubmissionAuthorization
	present       bool
}

func (result AuthorizationConfirmationResult) Outcome() AuthorizationConfirmationOutcome {
	return result.outcome
}

// UndecidedReason 只在`未决`时非零。
func (result AuthorizationConfirmationResult) UndecidedReason() AuthorizationConfirmationUndecidedReason {
	return result.reason
}

// Authorization 在确认成立、已有授权与授权不再有效三格交回在册那一份：已有授权时它可能是另一位确认人确认的，
// 交回的是原确认，不是本次请求。
func (result AuthorizationConfirmationResult) Authorization() (domain.SubmissionAuthorization, bool) {
	return result.authorization, result.present
}

type ConfirmSubmissionAuthorizationDeps struct {
	Readiness     ports.ReadinessView
	Authorities   ports.SubmissionAuthorityRegistry
	AuthorityView ports.SubmissionAuthorityView
	Clock         ports.Clock
}

type ConfirmSubmissionAuthorizationHandler struct {
	deps ConfirmSubmissionAuthorizationDeps
}

func NewConfirmSubmissionAuthorizationHandler(deps ConfirmSubmissionAuthorizationDeps) *ConfirmSubmissionAuthorizationHandler {
	return &ConfirmSubmissionAuthorizationHandler{deps: deps}
}

// Handle 把一次业务角色确认推进成该单元的提交授权：先看册上有没有——有就按它作答，不另形成、不顶替（同一确认的重放与
// 另一位有权角色的第二次确认都落在这里；一个单元同时只有一份当前授权，UC-CC-004「不得通过互相覆盖的允许与拒绝制造无法
// 解释的当前结果」）；再核单元当前就绪（授权前提，UC-CC-004 启动条件），就绪与授权仍是两条轨，这里只读就绪、不改它；最后
// 形成授权，授权依据写成「所凭授予@确认人」，提交版本固定时随依据一起快照下来，申报版本因此留得住确认人与授权依据（首发
// 范围「必须保留申报版本、确认人、授权依据」）。本编排不形成提交版本或尝试，也不入队：授权确认今天没有消费者。
func (handler *ConfirmSubmissionAuthorizationHandler) Handle(
	ctx context.Context,
	command ConfirmSubmissionAuthorizationCommand,
) (AuthorizationConfirmationResult, error) {
	unit, err := domain.NewDeclarationUnitID(command.UnitID)
	if err != nil ||
		strings.TrimSpace(command.TenantID.String()) == "" ||
		strings.TrimSpace(command.Confirmer) == "" ||
		strings.TrimSpace(command.Grant) == "" {
		return AuthorizationConfirmationResult{outcome: AuthorizationConfirmationNotAccepted}, nil
	}
	authority, err := domain.NewSubmissionAuthorityReference(command.Grant + "@" + command.Confirmer)
	if err != nil {
		return AuthorizationConfirmationResult{outcome: AuthorizationConfirmationNotAccepted}, nil
	}

	if result, settled := handler.settleAgainstRecorded(ctx, command.TenantID, unit); settled {
		return result, nil
	}

	readiness, configured, err := handler.deps.Readiness.LoadReadiness(ctx, command.TenantID, unit)
	if err != nil {
		return confirmationUndecided(ConfirmationReadinessUnavailable), nil
	}
	if !configured {
		return confirmationUndecided(ConfirmationReadinessUnconfigured), nil
	}
	if !readiness.Effective() {
		return AuthorizationConfirmationResult{outcome: AuthorizationReadinessNotEffective}, nil
	}

	authorization, err := domain.GrantSubmissionAuthority(unit, authority, handler.deps.Clock.Now())
	if err != nil {
		return AuthorizationConfirmationResult{outcome: AuthorizationConfirmationNotAccepted}, nil
	}
	saved, err := handler.deps.Authorities.GrantSubmissionAuthority(ctx, command.TenantID, authorization)
	if err != nil {
		return confirmationUndecided(ConfirmationAuthorityStoreUnavailable), nil
	}
	if saved == ports.CaseConfigurationRegistered {
		return AuthorizationConfirmationResult{
			outcome: SubmissionAuthorizationConfirmed, authorization: authorization, present: true,
		}, nil
	}
	// 并发下另一位确认人先落：读回赢家作答。
	if result, settled := handler.settleAgainstRecorded(ctx, command.TenantID, unit); settled {
		return result, nil
	}
	return confirmationUndecided(ConfirmationAuthorityStoreUnavailable), nil
}

// settleAgainstRecorded 在册上已有这个单元的授权时作答；没有时交回 settled=false 让编排往下走。读不动也算作答——
// 答未决，不当成「没有」去形成第二份。
func (handler *ConfirmSubmissionAuthorizationHandler) settleAgainstRecorded(
	ctx context.Context,
	tenant domain.TenantID,
	unit domain.DeclarationUnitID,
) (AuthorizationConfirmationResult, bool) {
	recorded, found, err := handler.deps.AuthorityView.LoadSubmissionAuthority(ctx, tenant, unit)
	if err != nil {
		return confirmationUndecided(ConfirmationAuthorityStoreUnavailable), true
	}
	if !found {
		return AuthorizationConfirmationResult{}, false
	}
	outcome := SubmissionAuthorizationExisting
	if !recorded.Effective() {
		outcome = SubmissionAuthorizationNoLongerEffective
	}
	return AuthorizationConfirmationResult{outcome: outcome, authorization: recorded, present: true}, true
}

func confirmationUndecided(reason AuthorizationConfirmationUndecidedReason) AuthorizationConfirmationResult {
	return AuthorizationConfirmationResult{outcome: AuthorizationConfirmationUndecided, reason: reason}
}
