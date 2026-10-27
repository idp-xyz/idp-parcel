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
	// ErrETAPredictionUntranslatable 表示 ETA 预测信封的引用译不成领域标识。引用坏了，不是等谁；
	// 禁止进 WithUndecidedSentinels。
	ErrETAPredictionUntranslatable = errors.New(
		"visibility exception parcelshipment adapter: untranslatable eta prediction reference")
	// ErrETAPredictionInconsistent 表示信封在而按（租户+包裹+里程碑）读不到任何当前预测——预测行与
	// 它的信封同一事务入库，信封在而行不在只能是仓储不变量已破。不是等谁，禁止进
	// WithUndecidedSentinels。
	ErrETAPredictionInconsistent = errors.New(
		"visibility exception parcelshipment adapter: eta prediction has no current row")
	// ErrETAVisibilityUndecided 表示判断编排停在自己的未决上（预测库、规则册、判断库或标识签发调不
	// 通）。依赖故障是续办，重投会改变结果；生产装配把它登记进 WithUndecidedSentinels。
	ErrETAVisibilityUndecided = errors.New(
		"visibility exception parcelshipment adapter: eta visibility judgement is undecided")
	// ErrUnexpectedETAVisibilityOutcome 表示判断编排交回了封闭集合以外的结果——含 NOT_ACCEPTED：本适配器
	// 把受理门要的四维全部填满才调编排，未受理只能是装配或编程错误。静默入账等于替编排作判断。
	ErrUnexpectedETAVisibilityOutcome = errors.New(
		"visibility exception parcelshipment adapter: unexpected eta visibility outcome")
)

// ETAVisibilityJudgeHandler 是 ETA 客户可见性判断编排的窄口。
type ETAVisibilityJudgeHandler interface {
	Handle(
		ctx context.Context,
		command veapplication.JudgeETAVisibilityCommand,
	) (veapplication.JudgeETAVisibilityResult, error)
}

// JudgeETAVisibilityOnPredictionAdapter 是 veinbox.ETAPredictionConsumer 的真实处理方：一份内部 ETA 预测
// 新版本形成时，按包裹经账户反查口取唯一货主客户账户，对这个账户作 ETA 客户可见性判断（UC-VE-003），
// 判断落库之后在同一事务里重走客户视图派生，ETA 一维随判断换版（CONTEXT「ETA 新版本……必须重新派生客户
// 视图并形成追加更正或明确替代关系」，`AT-VE-187`）。
//
// 它与账户反查译名同住本包：账户维不归 VE 拥有（PS 的成员关系 + PC 的账户身份，CONTEXT-MAP），歧义哨兵的
// 词汇表在这里，消费链据同一份词汇分格。
//
// 锁序：先锁当前投影行（与投影派生、客户归属补派生同向），再写判断，再写客户视图。
type JudgeETAVisibilityOnPredictionAdapter struct {
	projections CurrentProjectionView
	accounts    veports.ParcelCustomerAccountView
	judge       ETAVisibilityJudgeHandler
	derive      CustomerViewDeriveHandler
}

func NewJudgeETAVisibilityOnPredictionAdapter(
	projections CurrentProjectionView,
	accounts veports.ParcelCustomerAccountView,
	judge ETAVisibilityJudgeHandler,
	derive CustomerViewDeriveHandler,
) (*JudgeETAVisibilityOnPredictionAdapter, error) {
	if projections == nil {
		return nil, fmt.Errorf("visibility exception parcelshipment adapter: projection view is nil")
	}
	if accounts == nil {
		return nil, fmt.Errorf("visibility exception parcelshipment adapter: account view is nil")
	}
	if judge == nil {
		return nil, fmt.Errorf("visibility exception parcelshipment adapter: eta visibility judge handler is nil")
	}
	if derive == nil {
		return nil, fmt.Errorf("visibility exception parcelshipment adapter: view derive handler is nil")
	}
	return &JudgeETAVisibilityOnPredictionAdapter{
		projections: projections,
		accounts:    accounts,
		judge:       judge,
		derive:      derive,
	}, nil
}

var _ veinbox.FormedETAPredictionHandler = (*JudgeETAVisibilityOnPredictionAdapter)(nil)

// HandleFormedETAPrediction 把一份预测新版本推进到判断与客户视图。
//
// 账户反查三格（ADR-0060 的零/一/多）：零行不判断、不发明账户，入账收工——ETA 预测面向包裹，没有归属的
// 包裹没有可见性判断的对象；恰一行取账户判断；多行是机制拒绝自动采认（AT-VE-152），哨兵原样上抛落
// 未决，不任选。
//
// 判断按当前预测版，不按信封宣告的版本：信封晚于后继预测到达时，判当前版才是对的，后继信封再到只是
// 重放。投影还不存在时只判断不派生——判断留库，视图在第一份投影派生时读它，不欠任何动作；有投影才在
// 同一事务里重派生，且标明这次是判断变化触发的，免得被「同一投影版本重放」的短路吞掉。
func (adapter *JudgeETAVisibilityOnPredictionAdapter) HandleFormedETAPrediction(
	ctx context.Context,
	formed veinbox.FormedETAPrediction,
) error {
	tenant, err := vedomain.NewTenantID(formed.TenantID)
	if err != nil {
		return fmt.Errorf("%w: tenant: %v", ErrETAPredictionUntranslatable, err)
	}
	parcel, err := vedomain.NewTrackedParcelReference(formed.Parcel)
	if err != nil {
		return fmt.Errorf("%w: parcel: %v", ErrETAPredictionUntranslatable, err)
	}
	milestone, err := vedomain.NewMilestoneReference(formed.Milestone)
	if err != nil {
		return fmt.Errorf("%w: milestone: %v", ErrETAPredictionUntranslatable, err)
	}

	projection, projectionFound, err := adapter.projections.LockCurrent(ctx, tenant, parcel)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrDerivedProjectionUnreadable, err)
	}

	account, accountFound, err := adapter.accounts.FindCustomerAccount(ctx, tenant, parcel)
	if err != nil {
		if errors.Is(err, ErrAmbiguousCustomerAccount) || errors.Is(err, ErrCustomerAccountUntranslatable) {
			return err
		}
		return fmt.Errorf("%w: %v", ErrCustomerAccountUnavailable, err)
	}
	if !accountFound {
		return nil
	}

	judged, err := adapter.judge.Handle(ctx, veapplication.JudgeETAVisibilityCommand{
		TenantID:  tenant,
		Customer:  account,
		Parcel:    parcel,
		Milestone: milestone,
	})
	if err != nil {
		return err
	}
	switch judged.Outcome() {
	case veapplication.ETAVisibilityJudged,
		veapplication.ETAVisibilityRejudged,
		veapplication.ETAVisibilityExistingResult:
	case veapplication.ETAVisibilityOutsideRuleTarget:
		// 规则指名的客户可见 ETA 不是这个里程碑的：业务终局，没有判断变化，视图无须重派生。
		return nil
	case veapplication.ETAVisibilityNoCandidate:
		return fmt.Errorf("%w: parcel %q milestone %q version %q",
			ErrETAPredictionInconsistent, formed.Parcel, formed.Milestone, formed.VersionID)
	case veapplication.ETAVisibilityUndecided:
		return fmt.Errorf("%w: %s", ErrETAVisibilityUndecided, judged.UndecidedReason())
	default:
		return fmt.Errorf("%w: %q", ErrUnexpectedETAVisibilityOutcome, judged.Outcome())
	}
	if !projectionFound {
		return nil
	}

	result, err := adapter.derive.Handle(ctx, veapplication.DeriveCustomerViewCommand{
		TenantID:            tenant,
		Customer:            account,
		Projection:          projection,
		ETAJudgementChanged: true,
	})
	if err != nil {
		return err
	}
	return veconsume.ViewConsumption(result)
}
