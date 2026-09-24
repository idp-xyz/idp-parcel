package application

import (
	"fmt"
	"time"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

// registrationNumberRefusal 把目录对一个号的判断译成登记方看得懂的拒绝理由，空串即合格。身份层的终身注册号
// 与资料层的税务登记号共用这一份：各格的续办两边相同，只有`层不符`要说清号该填在哪一层。when 说明判号用的
// 是谁的生效时点，`不在用`那一格要把它说出来，登记方才知道该挪哪个时点。
func registrationNumberRefusal(
	check domain.RegistrationNumberCheck,
	country domain.RegistrationCountryCode,
	typeCode domain.RegistrationNumberTypeCode,
	number domain.RegistrationNumber,
	want domain.RegistrationNumberLayer,
	when string,
	at time.Time,
) (string, error) {
	switch check.Outcome() {
	case domain.RegistrationNumberAccepted:
		return "", nil
	case domain.RegistrationCountryNotRegistered:
		return fmt.Sprintf("注册国家 / 地区 %s 在注册号类型目录里未登记：先在目录登记它的注册号类型", country), nil
	case domain.RegistrationNumberTypeNotRegistered:
		return fmt.Sprintf("注册号类型 %s 在 %s 的注册号类型目录里未登记", typeCode, country), nil
	case domain.RegistrationNumberTypeNotEffective:
		return fmt.Sprintf("注册号类型 %s 在%s %s 不在用", typeCode, when, at.Format(time.RFC3339)), nil
	case domain.RegistrationNumberLayerMismatch:
		if want == domain.RegistrationNumberIdentityLayer {
			return fmt.Sprintf("注册号类型 %s 属资料层（税务登记号那一类），身份层只收终身注册号", typeCode), nil
		}
		return fmt.Sprintf("注册号类型 %s 属身份层（终身注册号那一类），资料层只收税务登记号", typeCode), nil
	case domain.RegistrationNumberFormatMismatch:
		return fmt.Sprintf("注册号 %q 不合类型 %s 登记的格式", number, typeCode), nil
	default:
		return "", fmt.Errorf("注册号类型目录答出了未知结果 %d", check.Outcome())
	}
}
