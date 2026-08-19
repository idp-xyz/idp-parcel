package parcelshipment

import (
	"context"
	"errors"
	"fmt"

	veinbox "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/inbox"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/veconsume"
	veapplication "go.idp.xyz/idp-parcel/internal/visibilityexception/application"
	vedomain "go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
	veports "go.idp.xyz/idp-parcel/internal/visibilityexception/ports"
)

var (
	// ErrDerivedProjectionUntranslatable 表示投影派生信封的引用译不成领域标识。引用
	// 坏了，不是等谁；禁止进 WithUndecidedSentinels。
	ErrDerivedProjectionUntranslatable = errors.New(
		"visibility exception parcelshipment adapter: untranslatable derived projection reference")
	// ErrDerivedProjectionUnreadable 表示投影库调不通。依赖故障是续办，重投会改变
	// 结果；生产装配把它登记进 WithUndecidedSentinels。
	ErrDerivedProjectionUnreadable = errors.New(
		"visibility exception parcelshipment adapter: tracking projection store is unreadable")
	// ErrDerivedProjectionInconsistent 表示按（租户+包裹）读不到任何当前投影——派生
	// 信封与投影行同一事务入库，信封在而行不在只能是仓储不变量已破。不是等谁，禁止
	// 进 WithUndecidedSentinels。
	ErrDerivedProjectionInconsistent = errors.New(
		"visibility exception parcelshipment adapter: derived projection has no current row")
	// ErrCustomerAccountUnavailable 表示账户反查读口调不通（PS 侧库故障等）。依赖
	// 故障是续办；生产装配把它登记进 WithUndecidedSentinels。
	ErrCustomerAccountUnavailable = errors.New(
		"visibility exception parcelshipment adapter: customer account lookup is unavailable")
)

// CurrentProjectionView 按（租户+包裹）读回当前投影。由 vepostgres.Projections 满足。
// 只读当前版：客户视图只基于当前投影形成（UC-VE-008），历史版由投影库自己留存。
type CurrentProjectionView interface {
	FindCurrent(
		ctx context.Context,
		tenant vedomain.TenantID,
		parcel vedomain.TrackedParcelReference,
	) (vedomain.TrackingProjection, bool, error)
}

// CustomerViewDeriveHandler 是客户视图派生编排的窄口。真实装配接组合根的租户现绑
// 包装（披露策略视图在构造期绑租户，派发进程是多租户）。
type CustomerViewDeriveHandler interface {
	Handle(
		ctx context.Context,
		command veapplication.DeriveCustomerViewCommand,
	) (veapplication.DeriveCustomerViewResult, error)
}

// DeriveCustomerViewOnProjectionAdapter 是 veinbox.TrackingProjectionConsumer 的真实
// 处理方：按（租户+包裹）读回当前投影，经本包的账户反查口填
// DeriveCustomerViewCommand.Customer，交给客户视图派生编排，结果由
// veconsume.ViewConsumption 译成消费门两格。
//
// 它与账户反查译名同住本包：账户维不归 VE 拥有（PS 的成员关系 + PC 的账户身份，
// CONTEXT-MAP），歧义哨兵的词汇表在这里，消费链据同一份词汇分格。
type DeriveCustomerViewOnProjectionAdapter struct {
	projections CurrentProjectionView
	accounts    veports.ParcelCustomerAccountView
	derive      CustomerViewDeriveHandler
}

func NewDeriveCustomerViewOnProjectionAdapter(
	projections CurrentProjectionView,
	accounts veports.ParcelCustomerAccountView,
	derive CustomerViewDeriveHandler,
) (*DeriveCustomerViewOnProjectionAdapter, error) {
	if projections == nil {
		return nil, fmt.Errorf("visibility exception parcelshipment adapter: projection view is nil")
	}
	if accounts == nil {
		return nil, fmt.Errorf("visibility exception parcelshipment adapter: account view is nil")
	}
	if derive == nil {
		return nil, fmt.Errorf("visibility exception parcelshipment adapter: view derive handler is nil")
	}
	return &DeriveCustomerViewOnProjectionAdapter{
		projections: projections,
		accounts:    accounts,
		derive:      derive,
	}, nil
}

var _ veinbox.DerivedTrackingProjectionHandler = (*DeriveCustomerViewOnProjectionAdapter)(nil)

// HandleDerivedTrackingProjection 把一个投影新版本推进到客户视图。
//
// 版本判新旧：只为仍是当前版的信封派生视图。信封宣告的版本落后于当前版时业务终局
// 跳过——每个投影版本在同一事务里各入队一封信，后继信封必然在途或已到，为旧版派生
// 会让当前视图倒退到已被替代的投影内容。
//
// 账户反查三格（ADR-0060 的零/一/多，勘察报告已裁）：零行不派生视图、不发明账户，
// 投影照旧存在——未派生的记录就是本消费账上这封已入账而无视图行的信封，零行的两种
// 成因（包裹引用装着集运单元号 / 尚无已接受委托）今天在数据上分不开（TF 事实不带身
// 份种类），本适配器不发明区分也不落「无轨迹」；恰一行取账户派生；多行是机制拒绝
// 自动采认（AT-VE-152），哨兵原样上抛落未决，不任选。
func (adapter *DeriveCustomerViewOnProjectionAdapter) HandleDerivedTrackingProjection(
	ctx context.Context,
	derived veinbox.DerivedTrackingProjection,
) error {
	tenant, err := vedomain.NewTenantID(derived.TenantID)
	if err != nil {
		return fmt.Errorf("%w: tenant: %v", ErrDerivedProjectionUntranslatable, err)
	}
	parcel, err := vedomain.NewTrackedParcelReference(derived.Parcel)
	if err != nil {
		return fmt.Errorf("%w: parcel: %v", ErrDerivedProjectionUntranslatable, err)
	}
	announced, err := vedomain.NewProjectionVersionID(derived.VersionID)
	if err != nil {
		return fmt.Errorf("%w: projection version: %v", ErrDerivedProjectionUntranslatable, err)
	}

	current, found, err := adapter.projections.FindCurrent(ctx, tenant, parcel)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrDerivedProjectionUnreadable, err)
	}
	if !found {
		return fmt.Errorf("%w: parcel %q version %q",
			ErrDerivedProjectionInconsistent, derived.Parcel, derived.VersionID)
	}
	if current.Version() != announced {
		// 旧版本的信封：当前版已前进，视图由后继信封派生。这不是丢失——投影版本行
		// 与它的信封同一事务留存（ADR-0065 版本只增不改写）。
		return nil
	}

	account, accountFound, err := adapter.accounts.FindCustomerAccount(ctx, tenant, parcel)
	if err != nil {
		if errors.Is(err, ErrAmbiguousCustomerAccount) || errors.Is(err, ErrCustomerAccountUntranslatable) {
			// 多账户歧义与译不动都带着本包具名哨兵，原样上抛：前者由装配登记成未决，
			// 后者是响亮的编程错误。
			return err
		}
		return fmt.Errorf("%w: %v", ErrCustomerAccountUnavailable, err)
	}
	if !accountFound {
		return nil
	}

	result, err := adapter.derive.Handle(ctx, veapplication.DeriveCustomerViewCommand{
		TenantID:   tenant,
		Customer:   account,
		Projection: current,
	})
	if err != nil {
		return err
	}
	return veconsume.ViewConsumption(result)
}
