package domain

import (
	"errors"
	"time"
)

var (
	ErrInvalidChannelAccountUseAuthorization = errors.New("party commercial: invalid channel account use authorization")
	// ErrChannelAccountBusinessUnauthorized 是 AT-PC-009：账号技术上可用，但业务使用授权
	// 未确认。恢复动作是取得持有人的业务授权；不得压成「字段不全」，也不得因通道可达而发布
	// （ADR-0039）。
	ErrChannelAccountBusinessUnauthorized = errors.New("party commercial: channel account business use is not authorized")
)

// ChannelAccountID 是面向明确渠道服务方的业务账号身份，不是登录凭据。
type ChannelAccountID struct{ requiredValue }

func NewChannelAccountID(value string) (ChannelAccountID, error) {
	required, err := newRequiredValue("channel account ID", value)
	return ChannelAccountID{required}, err
}

// ChannelAccountTechnicalStanding 回答「此刻技术上能否访问该账号」。它不授予业务使用许可。
type ChannelAccountTechnicalStanding uint8

const (
	ChannelAccountTechnicalStandingInvalid ChannelAccountTechnicalStanding = iota
	ChannelAccountTechnicallyUnavailable
	ChannelAccountTechnicallyAvailable
)

func (standing ChannelAccountTechnicalStanding) String() string {
	switch standing {
	case ChannelAccountTechnicallyUnavailable:
		return "TECHNICALLY_UNAVAILABLE"
	case ChannelAccountTechnicallyAvailable:
		return "TECHNICALLY_AVAILABLE"
	default:
		return ""
	}
}

// ChannelAccountBusinessStanding 回答「持有人是否已授予业务使用」。零值 = 未确认。
type ChannelAccountBusinessStanding uint8

const (
	ChannelAccountBusinessStandingInvalid ChannelAccountBusinessStanding = iota
	ChannelAccountBusinessUnauthorized
	ChannelAccountBusinessAuthorized
)

func (standing ChannelAccountBusinessStanding) String() string {
	switch standing {
	case ChannelAccountBusinessUnauthorized:
		return "BUSINESS_UNAUTHORIZED"
	case ChannelAccountBusinessAuthorized:
		return "BUSINESS_AUTHORIZED"
	default:
		return ""
	}
}

// ChannelAccountUseAuthorizationStatus 是使用授权自身的发布位置，不是渠道技术状态。
type ChannelAccountUseAuthorizationStatus uint8

const (
	ChannelAccountUseAuthorizationStatusInvalid ChannelAccountUseAuthorizationStatus = iota
	ChannelAccountUseAuthorizationPublished
)

func (status ChannelAccountUseAuthorizationStatus) String() string {
	switch status {
	case ChannelAccountUseAuthorizationPublished:
		return "PUBLISHED"
	default:
		return ""
	}
}

// ChannelAccountUseAuthorization 是渠道账号持有人允许运营企业在明确范围与期间使用该
// 账号的业务授权。凭据可达、映射候选、接受侧权限等级都不能顶替它（ADR-0039）。
type ChannelAccountUseAuthorization struct {
	account     ChannelAccountID
	grantor     PartyID
	grantee     PartyID
	channel     ChannelProductReference
	scope       CommercialScopeReference
	effective   EffectiveInterval
	status      ChannelAccountUseAuthorizationStatus
	publishedAt time.Time
}

// PublishChannelAccountUseAuthorization 在业务授权已确认时发布一条使用授权。
// 技术可用只是并存事实，不能单独让本函数成功。
func PublishChannelAccountUseAuthorization(
	account ChannelAccountID,
	grantor PartyID,
	grantee PartyID,
	channel ChannelProductReference,
	scope CommercialScopeReference,
	effective EffectiveInterval,
	technical ChannelAccountTechnicalStanding,
	business ChannelAccountBusinessStanding,
	publishedAt time.Time,
) (ChannelAccountUseAuthorization, error) {
	if !account.valid() || !grantor.valid() || !grantee.valid() ||
		!channel.valid() || !scope.valid() || !effective.valid() ||
		publishedAt.IsZero() || grantor == grantee {
		return ChannelAccountUseAuthorization{}, ErrInvalidChannelAccountUseAuthorization
	}
	// 技术存续必须有明确答复；未答不算「技术可用」。本闸门仍以业务授权为发布条件。
	if technical != ChannelAccountTechnicallyAvailable && technical != ChannelAccountTechnicallyUnavailable {
		return ChannelAccountUseAuthorization{}, ErrInvalidChannelAccountUseAuthorization
	}
	if business != ChannelAccountBusinessAuthorized {
		return ChannelAccountUseAuthorization{}, ErrChannelAccountBusinessUnauthorized
	}
	return ChannelAccountUseAuthorization{
		account:     account,
		grantor:     grantor,
		grantee:     grantee,
		channel:     channel,
		scope:       scope,
		effective:   effective,
		status:      ChannelAccountUseAuthorizationPublished,
		publishedAt: publishedAt.UTC(),
	}, nil
}

func (authorization ChannelAccountUseAuthorization) Account() ChannelAccountID {
	return authorization.account
}

func (authorization ChannelAccountUseAuthorization) Grantor() PartyID {
	return authorization.grantor
}

func (authorization ChannelAccountUseAuthorization) Grantee() PartyID {
	return authorization.grantee
}

func (authorization ChannelAccountUseAuthorization) Channel() ChannelProductReference {
	return authorization.channel
}

func (authorization ChannelAccountUseAuthorization) Scope() CommercialScopeReference {
	return authorization.scope
}

func (authorization ChannelAccountUseAuthorization) Effective() EffectiveInterval {
	return authorization.effective
}

func (authorization ChannelAccountUseAuthorization) Status() ChannelAccountUseAuthorizationStatus {
	return authorization.status
}

func (authorization ChannelAccountUseAuthorization) PublishedAt() time.Time {
	return authorization.publishedAt
}

// AllowsUseAt 只在已发布且落在有效期内时为真。技术可达不参与本判定——那是发布闸门
// 另一轴的事。
func (authorization ChannelAccountUseAuthorization) AllowsUseAt(at time.Time) bool {
	return authorization.status == ChannelAccountUseAuthorizationPublished &&
		authorization.effective.Contains(at)
}
