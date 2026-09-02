package application

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
)

// ChannelAccountUseOutcome 是渠道账号使用授权登记与撤销的结果代数，判据同
// ProductChannelOutcome，但**多一格**：`业务未授权`不并进 `未受理`。
//
// 分开的理由是恢复动作不同。`未受理`要商业责任方改输入（字段不全、修订错位、想偷换双方）；
// `业务未授权`要去取得持有人的业务授权，那是一件本系统之外的事，改多少遍输入都不会成功
// （ADR-0039：取得账号凭据不等于取得业务使用授权）。压成一格之后，调用方读到「被拒」会
// 去检查自己填的字段，而该做的是去要授权。
type ChannelAccountUseOutcome uint8

const (
	ChannelAccountUseOutcomeInvalid ChannelAccountUseOutcome = iota
	ChannelAccountUseRegistered
	ChannelAccountUseAlreadyRegistered
	ChannelAccountUseContentConflict
	ChannelAccountUseNotAccepted
	ChannelAccountUseBusinessUnauthorized
)

func (outcome ChannelAccountUseOutcome) String() string {
	switch outcome {
	case ChannelAccountUseRegistered:
		return "REGISTERED"
	case ChannelAccountUseAlreadyRegistered:
		return "ALREADY_REGISTERED"
	case ChannelAccountUseContentConflict:
		return "CONTENT_CONFLICT"
	case ChannelAccountUseNotAccepted:
		return "NOT_ACCEPTED"
	case ChannelAccountUseBusinessUnauthorized:
		return "BUSINESS_UNAUTHORIZED"
	default:
		return ""
	}
}

// ChannelAccountUseResult 携带落点与被拒的原因（判据同 ProductChannelResult：输入被拒不是
// 技术失败，事务保持可用）。
type ChannelAccountUseResult struct {
	outcome ChannelAccountUseOutcome
	cause   error
}

func (result ChannelAccountUseResult) Outcome() ChannelAccountUseOutcome {
	return result.outcome
}

func (result ChannelAccountUseResult) Cause() error {
	return result.cause
}

func channelAccountUseNotAccepted(cause error) ChannelAccountUseResult {
	return ChannelAccountUseResult{outcome: ChannelAccountUseNotAccepted, cause: cause}
}

// RegisterChannelAccountUseCommand 登记一笔渠道账号使用授权修订。
//
// 技术存续与业务存续两个发布门入参在命令上而不在领域对象上：它们是**发布当时**的答复，
// 聚合不留（ADR-0039 决定四）。调用方必须两个都答——技术存续未答同样过不了门，因为
// 「没人问过通道」与「问过且不通」是两件事。
type RegisterChannelAccountUseCommand struct {
	Tenant      domain.TenantID
	ID          domain.ChannelAccountUseAuthorizationID
	Revision    int
	Account     domain.ChannelAccountID
	Grantor     domain.PartyID
	Grantee     domain.PartyID
	Channel     domain.ChannelProductReference
	Scope       domain.CommercialScopeReference
	Effective   domain.EffectiveInterval
	Technical   domain.ChannelAccountTechnicalStanding
	Business    domain.ChannelAccountBusinessStanding
	PublishedAt time.Time
}

// RevokeChannelAccountUseCommand 以新修订追加一次撤销。它不带授权正文——撤销继续的是册上
// 已有的那一笔，正文从最新修订取回，不由调用方重述。让调用方重述正文就等于给了它一次改换
// 账号或双方的机会，而那正是 ADR-0093 决定六要挡的。
type RevokeChannelAccountUseCommand struct {
	Tenant    domain.TenantID
	ID        domain.ChannelAccountUseAuthorizationID
	Basis     domain.ChannelAccountRevocationBasisReference
	RevokedAt time.Time
}

// RegisterChannelAccountUseHandler 是渠道账号使用授权登记册的写侧编排：发布门 → 修订连续性
// 与承继检查 → 登记册落库（0018、ADR-0093）。调用方逐命令各起事务。
type RegisterChannelAccountUseHandler struct {
	registry ports.ChannelAccountUseAuthorizationRegistry
}

func NewRegisterChannelAccountUseHandler(
	registry ports.ChannelAccountUseAuthorizationRegistry,
) *RegisterChannelAccountUseHandler {
	return &RegisterChannelAccountUseHandler{registry: registry}
}

// Register 登记一笔使用授权修订。
//
// 首笔修订必须是 1；后续修订经领域信封的 Succeed 承继，账号与授权双方由它把门。这里不自己
// 比对那三个值——判据只该有一处，抄第二份迟早与领域那份分岔。
func (handler *RegisterChannelAccountUseHandler) Register(
	ctx context.Context,
	command RegisterChannelAccountUseCommand,
) (ChannelAccountUseResult, error) {
	const operation = "register channel account use authorization"

	authorization, err := domain.PublishChannelAccountUseAuthorization(
		command.Account, command.Grantor, command.Grantee,
		command.Channel, command.Scope, command.Effective,
		command.Technical, command.Business, command.PublishedAt,
	)
	if errors.Is(err, domain.ErrChannelAccountBusinessUnauthorized) {
		return ChannelAccountUseResult{
			outcome: ChannelAccountUseBusinessUnauthorized,
			cause:   err,
		}, nil
	}
	if err != nil {
		return channelAccountUseNotAccepted(err), nil
	}

	latest, found, err := handler.registry.LoadLatest(ctx, command.Tenant, command.ID)
	if err != nil {
		return ChannelAccountUseResult{}, fmt.Errorf("%s: %w", operation, err)
	}

	var registration domain.ChannelAccountUseAuthorizationRegistration
	switch {
	case !found:
		if command.Revision != 1 {
			return channelAccountUseNotAccepted(fmt.Errorf(
				"首笔登记修订必须是 1，收到 %d", command.Revision,
			)), nil
		}
		registration, err = domain.NewChannelAccountUseAuthorizationRegistration(
			command.Tenant, command.ID, 1, authorization,
		)
	case command.Revision != latest.Revision()+1:
		return channelAccountUseNotAccepted(fmt.Errorf(
			"修订必须连续：册上最新为 %d，收到 %d", latest.Revision(), command.Revision,
		)), nil
	default:
		registration, err = latest.Succeed(authorization)
	}
	if err != nil {
		return channelAccountUseNotAccepted(err), nil
	}

	return handler.save(ctx, operation, registration)
}

// Revoke 以新修订追加一次撤销。
//
// 册上没有这笔授权时答`未受理`而不是造一笔：撤销一件不存在的授权没有意义，而静默建一笔
// 已撤销的空壳会让册面多出一段从未发生过的历史。
func (handler *RegisterChannelAccountUseHandler) Revoke(
	ctx context.Context,
	command RevokeChannelAccountUseCommand,
) (ChannelAccountUseResult, error) {
	const operation = "revoke channel account use authorization"

	latest, found, err := handler.registry.LoadLatest(ctx, command.Tenant, command.ID)
	if err != nil {
		return ChannelAccountUseResult{}, fmt.Errorf("%s: %w", operation, err)
	}
	if !found {
		return channelAccountUseNotAccepted(fmt.Errorf(
			"授权 %s 不在册上，撤销不造它", command.ID,
		)), nil
	}

	// 已撤销的再撤一次由领域门拒。撤销时刻会不同，第二次撤销于是不是同内容重放而是一条
	// 与史实冲突的新修订——那会把「何时起不能用」改掉，而已经形成的交易正靠那一刻定依据。
	revoked, err := latest.Authorization().Revoke(command.Basis, command.RevokedAt)
	if err != nil {
		return channelAccountUseNotAccepted(err), nil
	}
	registration, err := latest.Succeed(revoked)
	if err != nil {
		return channelAccountUseNotAccepted(err), nil
	}

	return handler.save(ctx, operation, registration)
}

func (handler *RegisterChannelAccountUseHandler) save(
	ctx context.Context,
	operation string,
	registration domain.ChannelAccountUseAuthorizationRegistration,
) (ChannelAccountUseResult, error) {
	outcome, err := handler.registry.SaveChannelAccountUse(ctx, registration)
	if err != nil {
		return ChannelAccountUseResult{}, fmt.Errorf("%s: %w", operation, err)
	}
	switch outcome {
	case ports.ChannelAccountUseSaved:
		return ChannelAccountUseResult{outcome: ChannelAccountUseRegistered}, nil
	case ports.ChannelAccountUseAlreadyRegistered:
		return ChannelAccountUseResult{outcome: ChannelAccountUseAlreadyRegistered}, nil
	case ports.ChannelAccountUseContentConflict:
		return ChannelAccountUseResult{outcome: ChannelAccountUseContentConflict}, nil
	default:
		return ChannelAccountUseResult{}, fmt.Errorf(
			"%s: unexpected save outcome %d", operation, outcome)
	}
}
