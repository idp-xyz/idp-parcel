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

// ChannelAccountRevocationBasisReference 指向一次撤销所依据的东西（持有人通知、协议终止
// 或合规决定）。它不与 RelationshipBasisReference 共用一个类型：撤销一份账号使用授权不是
// 终止一段合作关系，两者各自的依据取自不同的文件，混用会让追溯指向错的那一份。
type ChannelAccountRevocationBasisReference struct{ requiredValue }

func NewChannelAccountRevocationBasisReference(value string) (ChannelAccountRevocationBasisReference, error) {
	required, err := newRequiredValue("channel account revocation basis reference", value)
	return ChannelAccountRevocationBasisReference{required}, err
}

// ChannelAccountUseAuthorizationStatus 是使用授权自身的发布位置，不是渠道技术状态。
//
// 自然到期**不在**本集合里，这是有意的：到期由有效区间对时点导出，存成状态就等于存一份会
// 过期的推导。撤销必须是取值，因为它没有任何东西可供导出——它是一次外部决定。
type ChannelAccountUseAuthorizationStatus uint8

const (
	ChannelAccountUseAuthorizationStatusInvalid ChannelAccountUseAuthorizationStatus = iota
	ChannelAccountUseAuthorizationPublished
	ChannelAccountUseAuthorizationRevoked
)

func (status ChannelAccountUseAuthorizationStatus) String() string {
	switch status {
	case ChannelAccountUseAuthorizationPublished:
		return "PUBLISHED"
	case ChannelAccountUseAuthorizationRevoked:
		return "REVOKED"
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
	revokedAt   time.Time
	revokedOn   ChannelAccountRevocationBasisReference
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

// RehydrateChannelAccountUseAuthorization 从持久化快照重建一条使用授权。它只对持久化适配器
// 开放（ADR-0028 的重建门）。
//
// 不走 PublishChannelAccountUseAuthorization 重建，是因为那扇门要技术存续与业务存续两个入参
// 而聚合两者都不留（ADR-0039 决定四：本记录不拥有凭据）。走它就得在重建时凭空造两个答复，
// 而那两个答复是**发布当时**的事实——重建时手上没有它们，编出来的值会让「当时技术不可用但业务
// 已授权」这种正当情形在回读后长成另一副样子。发布门要防的是无授权发布，而一行存在本身就是
// 那道门当初放行过的证据，不必也不能在回读时重演。
func RehydrateChannelAccountUseAuthorization(
	account ChannelAccountID,
	grantor PartyID,
	grantee PartyID,
	channel ChannelProductReference,
	scope CommercialScopeReference,
	effective EffectiveInterval,
	publishedAt time.Time,
	revokedAt time.Time,
	revokedOn ChannelAccountRevocationBasisReference,
) (ChannelAccountUseAuthorization, error) {
	if !account.valid() || !grantor.valid() || !grantee.valid() ||
		!channel.valid() || !scope.valid() || !effective.valid() ||
		publishedAt.IsZero() || grantor == grantee {
		return ChannelAccountUseAuthorization{}, ErrInvalidChannelAccountUseAuthorization
	}
	// 撤销时刻与依据同在或同缺，与库上的耦合 CHECK 同判。重建门相信输入，但不相信到
	// 「让一条半截的撤销事实进来」——那样的值答得出「已撤销」却举不出凭什么撤。
	if revokedAt.IsZero() != !revokedOn.valid() {
		return ChannelAccountUseAuthorization{}, ErrInvalidChannelAccountUseAuthorization
	}
	authorization := ChannelAccountUseAuthorization{
		account:     account,
		grantor:     grantor,
		grantee:     grantee,
		channel:     channel,
		scope:       scope,
		effective:   effective,
		status:      ChannelAccountUseAuthorizationPublished,
		publishedAt: publishedAt.UTC(),
	}
	if !revokedAt.IsZero() {
		if revokedAt.Before(publishedAt) {
			return ChannelAccountUseAuthorization{}, ErrInvalidChannelAccountUseAuthorization
		}
		authorization.status = ChannelAccountUseAuthorizationRevoked
		authorization.revokedAt = revokedAt.UTC()
		authorization.revokedOn = revokedOn
	}
	return authorization, nil
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

// Revoke 自该时点起终止本授权支持新的业务使用。它不改写任何既有事实：在它之下已经形成的
// 委托与面单交易，继续引用当时有效的授权依据（CONTEXT「不删除已经形成的授权证据和交易快照」）。
//
// 时刻与依据同在或同缺。只记时刻答不出凭什么撤，只记依据答不出从哪一刻起不能用，而后续新使用
// 的判定要的正是那一刻。
func (authorization ChannelAccountUseAuthorization) Revoke(
	basis ChannelAccountRevocationBasisReference,
	at time.Time,
) (ChannelAccountUseAuthorization, error) {
	if authorization.status != ChannelAccountUseAuthorizationPublished ||
		!basis.valid() || at.IsZero() {
		return ChannelAccountUseAuthorization{}, ErrInvalidChannelAccountUseAuthorization
	}
	authorization.status = ChannelAccountUseAuthorizationRevoked
	authorization.revokedAt = at.UTC()
	authorization.revokedOn = basis
	return authorization, nil
}

// RevokedAt 的第二个返回值把「被撤销」与「自然到期」分开。到期的授权在这里报 false——
// 它没有撤销时刻，因为根本没有人撤过它。
func (authorization ChannelAccountUseAuthorization) RevokedAt() (time.Time, bool) {
	if authorization.revokedAt.IsZero() {
		return time.Time{}, false
	}
	return authorization.revokedAt, true
}

func (authorization ChannelAccountUseAuthorization) RevokedOn() ChannelAccountRevocationBasisReference {
	return authorization.revokedOn
}

// AllowsUseAt 回答一个新的业务使用能否依据本授权发起。技术可达不参与本判定——那是发布
// 闸门另一轴的事。
//
// 撤销自其自身时点起关闭后续使用，不回溯：撤销之前的时点仍答真，因为那时的交易确实有依据，
// 而 CONTEXT 要求它们保留当时有效的授权依据。判据同 SupplierAgreement.SupportsProcurementAt。
func (authorization ChannelAccountUseAuthorization) AllowsUseAt(at time.Time) bool {
	if authorization.status == ChannelAccountUseAuthorizationStatusInvalid {
		return false
	}
	if !authorization.revokedAt.IsZero() && !at.Before(authorization.revokedAt) {
		return false
	}
	return authorization.effective.Contains(at)
}
