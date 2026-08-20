package parcelshipment

import (
	"context"
	"errors"
	"fmt"

	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	psports "go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	veinbox "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/inbox"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/veconsume"
	veapplication "go.idp.xyz/idp-parcel/internal/visibilityexception/application"
	vedomain "go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
	veports "go.idp.xyz/idp-parcel/internal/visibilityexception/ports"
)

var (
	// ErrAcceptanceDecisionUntranslatable 表示接受决定信封的引用译不成领域标识，或
	// 状态字落在本类型承载的封闭集合（ACCEPTED/REJECTED）之外。引用坏了，不是等谁；
	// 禁止进 WithUndecidedSentinels。
	ErrAcceptanceDecisionUntranslatable = errors.New(
		"visibility exception parcelshipment adapter: untranslatable acceptance decision")
	// ErrDeclaredParcelsUnavailable 表示 PS 声明清单读口调不通。依赖故障是续办，
	// 重投会改变结果；生产装配把它登记进 WithUndecidedSentinels。
	ErrDeclaredParcelsUnavailable = errors.New(
		"visibility exception parcelshipment adapter: declared parcels view is unavailable")
	// ErrAcceptanceRecordInconsistent 表示信封与 PS 当前投影列自相矛盾：接受信封与
	// 委托行同一事务入库，信封在而行不在；或成员刚从这份已接受委托自己的清单里读出，
	// 反方向按包裹反查却答零行。仓储不变量已破（ADR-0029），不是等谁，重投不自愈；
	// 禁止进 WithUndecidedSentinels。
	ErrAcceptanceRecordInconsistent = errors.New(
		"visibility exception parcelshipment adapter: acceptance decision disagrees with the delegation records")
	// ErrCustomerAccountMismatch 表示信封自报的账户与反查口答的账户不同。反查口是
	// 账户维的权威，信封值只作一致性校验；已接受委托撤不了也换不了代、修订不动成员
	// 不动账户，这一格在基线上结构不可达——真到达只能是绕过领域直写库，同样是不变量
	// 已破，硬失败让人来看。禁止进 WithUndecidedSentinels。
	ErrCustomerAccountMismatch = errors.New(
		"visibility exception parcelshipment adapter: envelope customer account disagrees with the authoritative lookup")
)

// DeriveCustomerViewOnAcceptanceAdapter 是 veinbox.AcceptanceDecisionConsumer 的真实
// 处理方：客户归属确立（接受决定）时按（租户+委托）取声明包裹清单，逐成员读当前投影，
// 有投影的成员重走客户视图派生（UC-VE-008 AT-VE-169——归属迟于源事实时视图在归属确立
// 时按当前投影形成，不等下一份源事实）。
//
// 成员循环按 ADR-0066 在消费侧拆分（先例：申报提交路）。账户维仍走本包的反查译名
// （veports.ParcelCustomerAccountView）——载荷的 CustomerAccountID 回答的是「这份委托
// 属于谁」，客户视图要的是「这件包裹此刻唯一归谁」，两者只在无歧义时重合；绕开反查
// 就丢了 ADR-0060 的多行歧义闸，跨账户泄露正是从那里进来。
type DeriveCustomerViewOnAcceptanceAdapter struct {
	parcels     psports.CurrentDeclaredParcelsView
	projections CurrentProjectionView
	accounts    veports.ParcelCustomerAccountView
	derive      CustomerViewDeriveHandler
}

func NewDeriveCustomerViewOnAcceptanceAdapter(
	parcels psports.CurrentDeclaredParcelsView,
	projections CurrentProjectionView,
	accounts veports.ParcelCustomerAccountView,
	derive CustomerViewDeriveHandler,
) (*DeriveCustomerViewOnAcceptanceAdapter, error) {
	if parcels == nil {
		return nil, fmt.Errorf("visibility exception parcelshipment adapter: declared parcels view is nil")
	}
	if projections == nil {
		return nil, fmt.Errorf("visibility exception parcelshipment adapter: projection view is nil")
	}
	if accounts == nil {
		return nil, fmt.Errorf("visibility exception parcelshipment adapter: account view is nil")
	}
	if derive == nil {
		return nil, fmt.Errorf("visibility exception parcelshipment adapter: view derive handler is nil")
	}
	return &DeriveCustomerViewOnAcceptanceAdapter{
		parcels:     parcels,
		projections: projections,
		accounts:    accounts,
		derive:      derive,
	}, nil
}

var _ veinbox.FormedAcceptanceDecisionHandler = (*DeriveCustomerViewOnAcceptanceAdapter)(nil)

// HandleFormedAcceptanceDecision 把一次客户归属确立推进到客户视图。
//
// 一封信一笔事务：任一成员未决或意图未交即整封报错回滚、重投从头再跑（头端阻塞是
// ADR-0066 认下的代价，换整封原子）；已派生成员靠派生编排按投影版本幂等（同版本答
// 已有结果）扛重跑。常态下本处理方空转——正常序（先接受委托、后包裹流转）时按包裹
// 读不到当前投影，什么也不做；只有「末次源事实已入账、其后才建立委托」的倒序才真正
// 派生。
func (adapter *DeriveCustomerViewOnAcceptanceAdapter) HandleFormedAcceptanceDecision(
	ctx context.Context,
	formed veinbox.FormedAcceptanceDecision,
) error {
	// 同一个事件类型承载接受与拒绝（PS 侧 acceptance-decision.formed），按封闭集合
	// 逐格分派。拒绝没有归属确立，是终局答案不是失败：入账收工，不重投。集合外的
	// 状态字说不出该不该派生，响亮报错让投递卡住看得见。
	switch formed.State {
	case psdomain.ShipmentRequestAccepted.String():
	case psdomain.ShipmentRequestRejected.String():
		return nil
	default:
		return fmt.Errorf("%w: decision state %q is not carried by this event type",
			ErrAcceptanceDecisionUntranslatable, formed.State)
	}

	psTenant, err := psdomain.NewTenantID(formed.TenantID)
	if err != nil {
		return fmt.Errorf("%w: tenant: %v", ErrAcceptanceDecisionUntranslatable, err)
	}
	requestID, err := psdomain.NewShipmentRequestID(formed.ShipmentRequestID)
	if err != nil {
		return fmt.Errorf("%w: shipment request: %v", ErrAcceptanceDecisionUntranslatable, err)
	}
	tenant, err := vedomain.NewTenantID(formed.TenantID)
	if err != nil {
		return fmt.Errorf("%w: tenant: %v", ErrAcceptanceDecisionUntranslatable, err)
	}
	// 信封账户先译后用：译不动是引用坏了，与比对不匹配分属两格。
	announced, err := vedomain.NewCustomerAccountReference(formed.CustomerAccountID)
	if err != nil {
		return fmt.Errorf("%w: customer account: %v", ErrAcceptanceDecisionUntranslatable, err)
	}

	members, found, err := adapter.parcels.FindCurrentDeclaredParcels(ctx, psTenant, requestID)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrDeclaredParcelsUnavailable, err)
	}
	if !found {
		// 接受信封与委托行同一事务入库，信封在而行不在只能是仓储不变量已破——
		// 不当可见性滞后等下去。
		return fmt.Errorf("%w: shipment request %q has no current row",
			ErrAcceptanceRecordInconsistent, formed.ShipmentRequestID)
	}

	for _, member := range members {
		parcel, err := vedomain.NewTrackedParcelReference(member.String())
		if err != nil {
			return fmt.Errorf("%w: declared parcel: %v", ErrAcceptanceDecisionUntranslatable, err)
		}

		projection, projectionFound, err := adapter.projections.FindCurrent(ctx, tenant, parcel)
		if err != nil {
			return fmt.Errorf("%w: %v", ErrDerivedProjectionUnreadable, err)
		}
		if !projectionFound {
			// 正常序：包裹还没流转，无投影可补派生。视图等它的第一份源事实经投影
			// 派生路形成，本封不欠它任何动作。
			continue
		}

		account, accountFound, err := adapter.accounts.FindCustomerAccount(ctx, tenant, parcel)
		if err != nil {
			if errors.Is(err, ErrAmbiguousCustomerAccount) || errors.Is(err, ErrCustomerAccountUntranslatable) {
				// 多账户歧义与译不动都带着本包具名哨兵，原样上抛：前者由装配登记成
				// 未决（AT-VE-152 不任选），后者是响亮的编程错误。
				return err
			}
			return fmt.Errorf("%w: %v", ErrCustomerAccountUnavailable, err)
		}
		if !accountFound {
			// 成员刚从这份已接受委托自己的清单里读出来，反方向读同一投影列却答零行
			// ——两次读自相矛盾，是不变量已破，不是「还没有委托」。
			return fmt.Errorf("%w: parcel %q has no current accepted target",
				ErrAcceptanceRecordInconsistent, member.String())
		}
		if account.String() != announced.String() {
			return fmt.Errorf("%w: parcel %q lookup %q envelope %q",
				ErrCustomerAccountMismatch, member.String(), account.String(), announced.String())
		}

		result, err := adapter.derive.Handle(ctx, veapplication.DeriveCustomerViewCommand{
			TenantID:   tenant,
			Customer:   account,
			Projection: projection,
		})
		if err != nil {
			return err
		}
		if err := veconsume.ViewConsumption(result); err != nil {
			return err
		}
	}
	return nil
}
