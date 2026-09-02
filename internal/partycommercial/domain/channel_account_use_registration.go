package domain

import "errors"

// ErrInvalidChannelAccountUseRegistration 拒绝立不住的账号使用授权登记信封。
var ErrInvalidChannelAccountUseRegistration = errors.New("party commercial: invalid channel account use authorization registration")

// ChannelAccountUseAuthorizationID 是一笔账号使用授权在登记册上的稳定标识。判据同
// ProductChannelMappingID：正文是账号×双方×区间，登记册需要一个可回指的键，键在信封上。
//
// 它不是 ChannelAccountID。同一个渠道账号可以在不同期间授给不同的被授权人，那是两笔授权
// 而不是一笔的两个修订；拿账号当登记键会把它们挤成一条链。
type ChannelAccountUseAuthorizationID struct{ requiredValue }

func NewChannelAccountUseAuthorizationID(value string) (ChannelAccountUseAuthorizationID, error) {
	required, err := newRequiredValue("channel account use authorization id", value)
	return ChannelAccountUseAuthorizationID{required}, err
}

// ChannelAccountUseAuthorizationRegistration 给一笔账号使用授权一个登记册身份：租户 +
// 登记标识 + 修订。修订从 1 起连续递增，撤销形成新修订，不原地改写（CONTEXT「不删除已经
// 形成的授权证据和交易快照」）。
//
// 授权本体整个存进信封而不是只存引用：与产品—渠道映射不同，这里没有版本册可指——账号使用
// 授权不是商业版本，没有 CommercialVersion，正文除了本册无处可放。
type ChannelAccountUseAuthorizationRegistration struct {
	tenant        TenantID
	id            ChannelAccountUseAuthorizationID
	revision      int
	authorization ChannelAccountUseAuthorization
}

func NewChannelAccountUseAuthorizationRegistration(
	tenant TenantID,
	id ChannelAccountUseAuthorizationID,
	revision int,
	authorization ChannelAccountUseAuthorization,
) (ChannelAccountUseAuthorizationRegistration, error) {
	if !tenant.valid() || !id.valid() || revision < 1 ||
		authorization.status == ChannelAccountUseAuthorizationStatusInvalid {
		return ChannelAccountUseAuthorizationRegistration{}, ErrInvalidChannelAccountUseRegistration
	}
	return ChannelAccountUseAuthorizationRegistration{
		tenant:        tenant,
		id:            id,
		revision:      revision,
		authorization: authorization,
	}, nil
}

// Succeed 追加一条后继修订。它是本册表达「撤销」与「调整」的唯一方式——前一修订原样留在
// 册上，因为它是那段时间里确实有效的授权证据。
//
// 账号与授权双方必须与前一修订一致。主键只守键唯一，守不住一条修订二把关系整个换掉，而那样
// 的行在册上仍像同一笔授权的后继：下游按登记标识取最新修订，会取到一份从没有人授权过的关系。
// 真要授给另一方或另一个账号，那是**另一笔**授权，另起登记标识。
func (registration ChannelAccountUseAuthorizationRegistration) Succeed(
	next ChannelAccountUseAuthorization,
) (ChannelAccountUseAuthorizationRegistration, error) {
	prior := registration.authorization
	if next.status == ChannelAccountUseAuthorizationStatusInvalid ||
		next.account != prior.account ||
		next.grantor != prior.grantor ||
		next.grantee != prior.grantee {
		return ChannelAccountUseAuthorizationRegistration{}, ErrInvalidChannelAccountUseRegistration
	}
	registration.revision++
	registration.authorization = next
	return registration, nil
}

func (registration ChannelAccountUseAuthorizationRegistration) Tenant() TenantID {
	return registration.tenant
}

func (registration ChannelAccountUseAuthorizationRegistration) ID() ChannelAccountUseAuthorizationID {
	return registration.id
}

func (registration ChannelAccountUseAuthorizationRegistration) Revision() int {
	return registration.revision
}

func (registration ChannelAccountUseAuthorizationRegistration) Authorization() ChannelAccountUseAuthorization {
	return registration.authorization
}
