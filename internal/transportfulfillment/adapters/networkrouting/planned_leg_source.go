package networkrouting

import (
	"context"
	"errors"
	"fmt"
	"strings"

	nrdomain "go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	nrports "go.idp.xyz/idp-parcel/internal/networkrouting/ports"
	tfinbox "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/inbox"
	tfapplication "go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	tfdomain "go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// 本文件是计划履约段进入运输准备的那一缝（ADR-0201 决定二、七，票 demo-chain-gaps/18）：初始路由信封到了之后，按信封
// 上的键回读 NR 的判断，把计划段译成本上下文的读法交给准备编排。NR 拼写怎样读成连接代码与起讫节点只在这里——领域里的
// 准备评估只见本上下文的方向类型，不认识 NR 的任何结构。

var (
	// ErrInitialRouteNotVisible 表示按信封所指的键还读不回判断，或 NR 的库此刻答不出。判断与信封在提供方同一事务落库，
	// 同库同进程下读不回只剩可见性滞后，重投会改变结果。生产装配把它登进 WithUndecidedSentinels。
	ErrInitialRouteNotVisible = errors.New(
		"transport fulfillment networkrouting adapter: initial route judgment is not yet visible")
	// ErrPreparationUndecided 表示准备编排停在自己的未决上（申报读口、班次册、池册或准备结果库）。同样是续办，同样进
	// WithUndecidedSentinels；停在哪一口留在错误正文里。
	ErrPreparationUndecided = errors.New(
		"transport fulfillment networkrouting adapter: transport preparation is undecided")
	// ErrInitialRouteRecordInconsistent 表示按键取回的记录指着另一个键，或计划与无路线两支都带、都缺。提供方仓储不变量
	// 已破，不是等谁；禁止进 WithUndecidedSentinels（ADR-0029）。
	ErrInitialRouteRecordInconsistent = errors.New(
		"transport fulfillment networkrouting adapter: initial route record disagrees with its key")
	// ErrUnexpectedPreparationOutcome 表示编排交回了封闭集合以外的结果。静默入账等于替编排作判断。
	ErrUnexpectedPreparationOutcome = errors.New(
		"transport fulfillment networkrouting adapter: unexpected transport preparation outcome")
)

// connectionBasisPrefix 是 NR 计划段窗口依据里指名连接的那一格的前缀。窗口依据写作 `CONNECTION/<代码>@<版本>`，可能与
// 别的依据以 `+` 连成一串；拼写归 NR（ADR-0201 决定二），本缝只按它读出连接代码，不读版本——班次的方向按不带版本的
// 连接代码相认，连接换版不让班次失配（ADR-0201 越权风险点 6）。
const (
	connectionBasisPrefix    = "CONNECTION/"
	windowBasisSeparator     = "+"
	connectionVersionDivider = "@"
)

// InitialRouteFinder 按判断键整条取回记录，由 NR 的 InitialRouteStore 满足；只声明读的这一口——「TF 引用有效计划形成
// 执行准备，不修改计划」要在类型上就看得出来。
type InitialRouteFinder interface {
	FindByKey(ctx context.Context, key nrdomain.InitialRouteJudgmentKey) (nrports.InitialRouteRecord, bool, error)
}

// PreparationTrigger 是准备编排的那一口。真实装配接 tfapplication.PrepareOnPlannedLegsHandler。
type PreparationTrigger interface {
	Prepare(
		ctx context.Context,
		command tfapplication.PrepareOnPlannedLegsCommand,
	) (tfapplication.PrepareOnPlannedLegsResult, error)
}

// PrepareOnInitialRoute 是 tfinbox.InitialRouteFormedConsumer 的真实处理方。
type PrepareOnInitialRoute struct {
	routes  InitialRouteFinder
	prepare PreparationTrigger
}

func NewPrepareOnInitialRoute(routes InitialRouteFinder, prepare PreparationTrigger) (*PrepareOnInitialRoute, error) {
	if routes == nil {
		return nil, fmt.Errorf("transport fulfillment networkrouting adapter: initial route finder is nil")
	}
	if prepare == nil {
		return nil, fmt.Errorf("transport fulfillment networkrouting adapter: preparation trigger is nil")
	}
	return &PrepareOnInitialRoute{routes: routes, prepare: prepare}, nil
}

var _ tfinbox.FormedInitialRouteHandler = (*PrepareOnInitialRoute)(nil)

// HandleFormedInitialRoute 回读 → 译段 → 交编排 → 把结果落成消费两格。无路线判断也交编排，由编排答零写入：「无路线就
// 不准备」是本上下文的判断，不该在翻译层里悄悄跳过。
func (adapter *PrepareOnInitialRoute) HandleFormedInitialRoute(ctx context.Context, formed tfinbox.FormedInitialRoute) error {
	key, err := initialRouteKeyOf(formed)
	if err != nil {
		return err
	}
	record, found, err := adapter.routes.FindByKey(ctx, key)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrInitialRouteNotVisible, err)
	}
	if !found {
		return fmt.Errorf("%w: parcel %q purpose %q baseline %q",
			ErrInitialRouteNotVisible, formed.Parcel, formed.Purpose, formed.Baseline)
	}
	if record.Key != key || record.HasPlan == record.HasNoRoute ||
		(record.HasPlan && record.Plan.Key() != key) || (record.HasNoRoute && record.NoRoute.Key() != key) {
		return fmt.Errorf("%w: parcel %q purpose %q baseline %q",
			ErrInitialRouteRecordInconsistent, formed.Parcel, formed.Purpose, formed.Baseline)
	}

	command, err := preparationCommandOf(record)
	if err != nil {
		return err
	}
	result, err := adapter.prepare.Prepare(ctx, command)
	if err != nil {
		return err
	}
	return preparationConsumption(result)
}

// preparationCommandOf 把判断译成准备命令。对象取判断键上的声明包裹：TF 的载运对象引用就是正式包裹身份的引用，拼写
// 不改。段引用由 NR 一处定义的「版本#序位」拼出（ADR-0131 决定一），序位自首段起计。
func preparationCommandOf(record nrports.InitialRouteRecord) (tfapplication.PrepareOnPlannedLegsCommand, error) {
	none := tfapplication.PrepareOnPlannedLegsCommand{}
	tenant, err := tfdomain.NewTenantID(record.Key.TenantID.String())
	if err != nil {
		return none, fmt.Errorf("%w: tenant: %v", ErrUntranslatableAnswer, err)
	}
	command := tfapplication.PrepareOnPlannedLegsCommand{
		TenantID: tenant,
		Object:   record.Key.DeclaredParcelID.String(),
	}
	if record.HasNoRoute {
		command.Route = tfapplication.PlannedRouteNotFormed
		return command, nil
	}
	command.Route = tfapplication.PlannedRouteFormed
	for index, leg := range record.Plan.Legs() {
		translated, err := preparationLegOf(record.Plan.Version(), index+1, leg)
		if err != nil {
			return none, err
		}
		command.Legs = append(command.Legs, translated)
	}
	return command, nil
}

func preparationLegOf(version nrdomain.RoutePlanVersionID, ordinal int, leg nrdomain.PlannedLeg) (tfdomain.PreparationLeg, error) {
	reference, err := nrdomain.NewPlannedLegReference(version, ordinal)
	if err != nil {
		return tfdomain.PreparationLeg{}, fmt.Errorf("%w: planned leg reference: %v", ErrUntranslatableAnswer, err)
	}
	segment, err := tfdomain.NewPlannedSegmentReference(reference.String())
	if err != nil {
		return tfdomain.PreparationLeg{}, fmt.Errorf("%w: planned leg reference: %v", ErrUntranslatableAnswer, err)
	}
	connection, err := connectionOf(leg.Window().Basis())
	if err != nil {
		return tfdomain.PreparationLeg{}, fmt.Errorf("%w: leg %s: %v", ErrUntranslatableAnswer, reference, err)
	}
	direction, err := tfdomain.NewScheduleDirection(connection, leg.From().String(), leg.To().String())
	if err != nil {
		return tfdomain.PreparationLeg{}, fmt.Errorf("%w: leg %s direction: %v", ErrUntranslatableAnswer, reference, err)
	}
	translated, err := tfdomain.NewPreparationLeg(segment, direction, leg.Window().Earliest(), leg.Window().Latest())
	if err != nil {
		return tfdomain.PreparationLeg{}, fmt.Errorf("%w: leg %s window: %v", ErrUntranslatableAnswer, reference, err)
	}
	return translated, nil
}

// connectionOf 从窗口依据里读出唯一的连接代码。一格都没有或不止一格就答不出——计划段说不清自己走哪条连接，任何班次
// 都无从与它相认；那是 NR 计划段形状变了，重投不会变，所以响亮上抛而不是折成`未配置`。
func connectionOf(basis nrdomain.WindowBasisReference) (string, error) {
	var found []string
	for _, part := range strings.Split(basis.String(), windowBasisSeparator) {
		if reference, named := strings.CutPrefix(part, connectionBasisPrefix); named {
			found = append(found, reference)
		}
	}
	if len(found) != 1 {
		return "", fmt.Errorf("window basis %q names %d connections, want exactly one", basis.String(), len(found))
	}
	cut := strings.LastIndex(found[0], connectionVersionDivider)
	if cut <= 0 {
		return "", fmt.Errorf("window basis %q does not spell a versioned connection", basis.String())
	}
	return found[0][:cut], nil
}

// preparationConsumption 把编排结果落成消费两格，按「重投会不会改变结果」分：各段已作答（含已有结果、结果冲突与
// 未配置）与无路线零写入都是终局答案，入账不重投；未决重投；未受理在这里是本包译出的命令被拒，属适配器缺陷，响亮
// 上抛不入账。
func preparationConsumption(result tfapplication.PrepareOnPlannedLegsResult) error {
	switch result.Outcome() {
	case tfapplication.PreparationRunAnswered, tfapplication.PreparationRunNoRoute:
		return nil
	case tfapplication.PreparationRunUndecided:
		return fmt.Errorf("%w: %s (%s)", ErrPreparationUndecided, result.UndecidedReason(), result.ContinuationReference())
	case tfapplication.PreparationRunNotAccepted:
		return fmt.Errorf("%w: the command built from the initial route was refused", ErrUntranslatableAnswer)
	default:
		return fmt.Errorf("%w: %q", ErrUnexpectedPreparationOutcome, result.Outcome())
	}
}

func initialRouteKeyOf(formed tfinbox.FormedInitialRoute) (nrdomain.InitialRouteJudgmentKey, error) {
	none := nrdomain.InitialRouteJudgmentKey{}
	tenant, err := nrdomain.NewTenantID(formed.TenantID)
	if err != nil {
		return none, fmt.Errorf("%w: tenant: %v", ErrUntranslatableAnswer, err)
	}
	customer, err := nrdomain.NewCustomerAccountID(formed.CustomerAccountID)
	if err != nil {
		return none, fmt.Errorf("%w: customer account: %v", ErrUntranslatableAnswer, err)
	}
	shipment, err := nrdomain.NewShipmentRequestID(formed.Shipment)
	if err != nil {
		return none, fmt.Errorf("%w: shipment request: %v", ErrUntranslatableAnswer, err)
	}
	baseline, err := nrdomain.NewAcceptanceBaselineReference(formed.Baseline)
	if err != nil {
		return none, fmt.Errorf("%w: acceptance baseline: %v", ErrUntranslatableAnswer, err)
	}
	parcel, err := nrdomain.NewDeclaredParcelID(formed.Parcel)
	if err != nil {
		return none, fmt.Errorf("%w: declared parcel: %v", ErrUntranslatableAnswer, err)
	}
	purpose, err := nrdomain.NewServicePurpose(formed.Purpose)
	if err != nil {
		return none, fmt.Errorf("%w: service purpose: %v", ErrUntranslatableAnswer, err)
	}
	return nrdomain.InitialRouteJudgmentKey{
		TenantID:           tenant,
		CustomerAccountID:  customer,
		ShipmentRequestID:  shipment,
		AcceptanceBaseline: baseline,
		DeclaredParcelID:   parcel,
		ServicePurpose:     purpose,
	}, nil
}
