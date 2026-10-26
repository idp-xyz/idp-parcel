package accessidentity

import (
	"context"
	"errors"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// directCustomerAuthorityLabel 是直客授权引用的前缀：撤回留痕里「凭什么算数」记的是渠道绑定，不是 PC 授权版本。
const directCustomerAuthorityLabel = "FIRST-PARTY-DIRECT-CUSTOMER-BINDING"

// DirectCustomerWithdrawalAuthorizer 是首方直客 1:1 的撤回授权（ADR-0310 取舍 B）：撤回无 PC 裁定，授权就是册上的账户绑定
// 加撤回授予与准入范围——这三样已在铸造撤回信封时判过，信封里的租户、账户与客户端来源又是撤回命令里被撤委托身份的来源，
// 命令到这里时授权已经成立，本授权器只把它译成撤回留痕要的授权引用。
//
// 它不问任何外部权威，所以没有「答不出」「未配置」「适用冲突」三格；也不是对 WithdrawalAuthorizer 的默认放行——只有首方撤回口
// 的装配会用它，操作员侧与代理人（1:N）要有自己的授权器，不得复用（UC-PS-005 明禁默认任何角色有撤回权）。
type DirectCustomerWithdrawalAuthorizer struct{}

var _ ports.WithdrawalAuthorizer = DirectCustomerWithdrawalAuthorizer{}

func (DirectCustomerWithdrawalAuthorizer) AuthorizeWithdrawal(
	_ context.Context,
	query ports.WithdrawalAuthorizationQuery,
) (ports.WithdrawalAuthorization, error) {
	identity := query.Identity
	// 撤回者必须就是被撤委托所属的客户端来源：Intake 两处都取自同一信封，这里再核一遍，免得别的装配把任意撤回者递进来。
	if query.Requester.String() != identity.Source().String() {
		return ports.WithdrawalAuthorization{}, errors.New("direct customer withdrawal: requester is not the bound client")
	}
	authority, err := domain.NewWithdrawalAuthorityReference(
		directCustomerAuthorityLabel + "/" + identity.TenantID().String() + "/" + identity.CustomerAccountID().String())
	if err != nil {
		return ports.WithdrawalAuthorization{}, err
	}
	return ports.WithdrawalAuthorization{Outcome: ports.AuthorizationGranted, Authority: authority}, nil
}
