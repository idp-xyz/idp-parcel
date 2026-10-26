package parcelshipment

import (
	"context"
	"errors"
	"fmt"

	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	psports "go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	vedomain "go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
	veports "go.idp.xyz/idp-parcel/internal/visibilityexception/ports"
)

var (
	// ErrCustomerAccountUntranslatable 是端口哨兵在本译名包的同名引用：本包派生适配器按这个
	// 名字认它。引用坏了，不是等谁；禁止进 WithUndecidedSentinels。
	ErrCustomerAccountUntranslatable = veports.ErrCustomerAccountUntranslatable
	// ErrAmbiguousCustomerAccount 是端口哨兵在本译名包的同名引用：PS 的
	// ErrAmbiguousParcelTarget（同一租户下多份当前已接受委托都声明了这件对象）在本上下文
	// 译成它，本包派生适配器与 parcel-dispatch 的未决哨兵名单按这个名字认它。PS 机制拒绝
	// 自动采认，本侧同样不得按时间或行序任选一个账户；两侧各怎么分格见端口。
	ErrAmbiguousCustomerAccount = veports.ErrAmbiguousCustomerAccount
)

// ParcelCustomerAccountLookup 把 PS 的按包裹反查读口（CurrentAcceptedParcelTargetView）
// 译成 VE 的账户读口（veports.ParcelCustomerAccountView）。只翻译不判断：包裹属于
// 哪份委托、委托属于哪个账户都由 PS 的当前已接受投影回答，本适配器不缓存第二份映射，
// 也不写 PS 的库。
type ParcelCustomerAccountLookup struct {
	targets psports.CurrentAcceptedParcelTargetView
}

func NewParcelCustomerAccountLookup(
	targets psports.CurrentAcceptedParcelTargetView,
) (*ParcelCustomerAccountLookup, error) {
	if targets == nil {
		return nil, fmt.Errorf(
			"visibility exception parcelshipment adapter: parcel target view is nil")
	}
	return &ParcelCustomerAccountLookup{targets: targets}, nil
}

var _ veports.ParcelCustomerAccountView = (*ParcelCustomerAccountLookup)(nil)

// FindCustomerAccount 按（租户+追踪对象）反查当前货主客户账户。答案三格照端口注释：
// 零行如实交回 found=false 不分成因——追踪包裹引用可能装着集运单元号，反查零行不是
// 缺陷，成因区分留给接线票；恰一行取来源身份上的账户；多行译成 ErrAmbiguousCustomerAccount。
//
// PS 哨兵不外泄：多行那格用 %v 折叠底层错误，调用方只见 VE 具名哨兵——放行 %w 会让
// 编排绕过本上下文的词汇直接判 PS 的错误，跨上下文翻译就白做了。
func (lookup *ParcelCustomerAccountLookup) FindCustomerAccount(
	ctx context.Context,
	tenant vedomain.TenantID,
	parcel vedomain.TrackedParcelReference,
) (vedomain.CustomerAccountReference, bool, error) {
	none := vedomain.CustomerAccountReference{}
	psTenant, err := psdomain.NewTenantID(tenant.String())
	if err != nil {
		return none, false, fmt.Errorf("%w: tenant: %v", ErrCustomerAccountUntranslatable, err)
	}
	declared, err := psdomain.NewDeclaredParcelID(parcel.String())
	if err != nil {
		return none, false, fmt.Errorf("%w: parcel: %v", ErrCustomerAccountUntranslatable, err)
	}

	target, found, err := lookup.targets.FindCurrentAcceptedByParcel(ctx, psTenant, declared)
	if err != nil {
		if errors.Is(err, psdomain.ErrAmbiguousParcelTarget) {
			return none, false, fmt.Errorf("%w: parcel %q: %v",
				ErrAmbiguousCustomerAccount, parcel.String(), err)
		}
		// 依赖调不通原样上抛（包一层定位前缀），由调用方形成未决；它不是三格里的
		// 任何一格，译成 found=false 会把「没查成」说成「查过没有」。
		return none, false, fmt.Errorf(
			"visibility exception parcelshipment adapter: find current accepted by parcel: %w", err)
	}
	if !found {
		return none, false, nil
	}

	account, err := vedomain.NewCustomerAccountReference(
		target.Identity().CustomerAccountID().String())
	if err != nil {
		return none, false, fmt.Errorf(
			"%w: customer account: %v", ErrCustomerAccountUntranslatable, err)
	}
	return account, true, nil
}
