package customscompliance

import (
	"context"
	"errors"
	"fmt"

	ccdomain "go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	ccports "go.idp.xyz/idp-parcel/internal/customscompliance/ports"
	veinbox "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/inbox"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/veconsume"
	veapplication "go.idp.xyz/idp-parcel/internal/visibilityexception/application"
	vedomain "go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
)

var (
	// ErrExternalResultNotVisible 表示按信封键还读不回接收记录。可见性滞后是续办，重投
	// 会改变结果；不当毒丸拒收。
	ErrExternalResultNotVisible = errors.New(
		"visibility exception customscompliance adapter: external result is not yet visible")
	// ErrResultScopeUnitNotVisible 表示结果范围指名的申报单元读不回。CC 接收时已沿同一条
	// 范围→单元链取过辖区，单元只建立不更新；读不回是依赖或可见性的事，续办格。
	ErrResultScopeUnitNotVisible = errors.New(
		"visibility exception customscompliance adapter: declaration unit named by the result scope is not yet visible")
	// ErrExternalResultRecordInconsistent 表示按键取回的记录指着另一个键、是归属不上的
	// 留存、时间为零、放行三件与层次不配，或范围单元答非所问、组成为空。仓储不变量已破
	// （ADR-0029），不是等谁。禁止进 WithUndecidedSentinels。
	ErrExternalResultRecordInconsistent = errors.New(
		"visibility exception customscompliance adapter: external result disagrees with its key")
	// ErrExternalResultUntranslatableAnswer 表示引用译不成领域标识。引用坏了，不是等谁。
	ErrExternalResultUntranslatableAnswer = errors.New(
		"visibility exception customscompliance adapter: untranslatable external result answer")
)

// ExternalResultFinder 按接收幂等键取回接收记录。由 CC 的 ExternalResultStore 满足。
// 只取一法：本适配器不写 CC 的库。
type ExternalResultFinder interface {
	FindByKey(
		ctx context.Context,
		key ccports.ExternalResultKey,
	) (ccports.ExternalResultRecord, bool, error)
}

// DeclarationUnitFinder 按单元标识取回申报单元。由 CC 的 DeclarationUnitStore 满足。
type DeclarationUnitFinder interface {
	FindByID(
		ctx context.Context,
		tenant ccdomain.TenantID,
		unit ccdomain.DeclarationUnitID,
	) (ccdomain.DeclarationUnit, bool, error)
}

// ResultProjectionHandler 是派生编排在外部结果适配器侧的窄口。与同包案件侧、提交侧的
// 窄口同形不同名：各适配器的窄口各自具名，装配处谁接谁一眼可见。
type ResultProjectionHandler interface {
	Handle(
		ctx context.Context,
		command veapplication.DeriveProjectionCommand,
	) (veapplication.DeriveProjectionResult, error)
}

// DeriveOnExternalResultAdapter 是 veinbox.ExternalResultConsumer 的真实处理方：按接收
// 幂等键重读 CC 的接收记录，把这一次接收实际形成的那一层译成本层自己的事实类型，沿
// 结果范围→申报单元取回组成，逐成员交给派生编排（ADR-0066 消费侧循环拆分）。
//
// 一封外部结果只说它自己那一层：类型词按层分开、放行层再按全部 / 部分 / 附条件分开，
// 不存在「清关成功」这样一个能盖住各层的类型词（CC CONTEXT「不能使用一个“清关成功”
// 状态覆盖各层事实」）。哪一层映成哪个标准里程碑由版本化映射目录决定，未登记即未归类。
// 类型词表与展开口径的取舍见 ADR-0197。
type DeriveOnExternalResultAdapter struct {
	results ExternalResultFinder
	units   DeclarationUnitFinder
	derive  ResultProjectionHandler
}

func NewDeriveOnExternalResultAdapter(
	results ExternalResultFinder,
	units DeclarationUnitFinder,
	derive ResultProjectionHandler,
) (*DeriveOnExternalResultAdapter, error) {
	if results == nil {
		return nil, fmt.Errorf("visibility exception customscompliance adapter: external result finder is nil")
	}
	if units == nil {
		return nil, fmt.Errorf("visibility exception customscompliance adapter: declaration unit finder is nil")
	}
	if derive == nil {
		return nil, fmt.Errorf("visibility exception customscompliance adapter: projection handler is nil")
	}
	return &DeriveOnExternalResultAdapter{results: results, units: units, derive: derive}, nil
}

var _ veinbox.ReceivedExternalResultHandler = (*DeriveOnExternalResultAdapter)(nil)

// HandleReceivedExternalResult 按信封键取回接收记录，对范围单元的组成逐成员派生投影。
//
// 一封信一笔事务：任一成员未决或意图未交即整封报错回滚、重投从头再跑（ADR-0066 认下的
// 头端阻塞）；单成员业务终局（冲突、不接受）入账继续。
func (adapter *DeriveOnExternalResultAdapter) HandleReceivedExternalResult(
	ctx context.Context,
	received veinbox.ReceivedExternalResult,
) error {
	key, err := externalResultKeyFor(received)
	if err != nil {
		return err
	}
	record, found, err := adapter.results.FindByKey(ctx, key)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrExternalResultNotVisible, err)
	}
	if !found {
		return fmt.Errorf("%w: source %q", ErrExternalResultNotVisible, received.SourceID)
	}
	if record.Key != key {
		return fmt.Errorf("%w: source %q", ErrExternalResultRecordInconsistent, received.SourceID)
	}
	// CC 的交接口不为归属不上的留存入队（那不是监管事实）；信封指到这一格，说明记录在
	// 入队之后被改写过，不是这里该猜的事。
	if record.Unattributable {
		return fmt.Errorf("%w: source %q is an unattributable retention", ErrExternalResultRecordInconsistent, received.SourceID)
	}
	// 同层冲突的那一份留在 CC、双方事实都在册，而 CC 的当前判断不因它改变（CONTEXT「不得
	// 按最后到达直接改变当前判断」）。投影若收下它，映射目录一行就能让后到的一方改掉包裹
	// 当前的关务进展——正是那句话禁的事。冲突关系今天只有一个标记、指不出与谁冲突，投影
	// 立不起冲突关系，所以入账不派生。
	if record.LayerConflict {
		return nil
	}
	result := record.Result
	if record.RecordedAt.IsZero() || result.OccurredAt().IsZero() {
		return fmt.Errorf("%w: source %q", ErrExternalResultRecordInconsistent, received.SourceID)
	}
	kind, err := externalResultKind(record)
	if err != nil {
		return err
	}

	// 结果范围指名申报单元（CC 接收编排沿同一条链取辖区）。组成取单元本体而不取提交
	// 版本快照：放行与处置只作用于决定明确覆盖的范围，而范围指名的正是这个单元。
	unitID, err := ccdomain.NewDeclarationUnitID(result.Scope().String())
	if err != nil {
		return fmt.Errorf("%w: result scope: %v", ErrExternalResultUntranslatableAnswer, err)
	}
	unit, found, err := adapter.units.FindByID(ctx, key.TenantID, unitID)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrResultScopeUnitNotVisible, err)
	}
	if !found {
		return fmt.Errorf("%w: unit %q", ErrResultScopeUnitNotVisible, unitID)
	}
	if unit.ID() != unitID {
		return fmt.Errorf("%w: unit %q", ErrExternalResultRecordInconsistent, unitID)
	}
	members := unit.Members()
	if len(members) == 0 {
		return fmt.Errorf("%w: unit %q has no members", ErrExternalResultRecordInconsistent, unitID)
	}

	for _, member := range members {
		command, err := externalResultProjectionCommand(record, kind, member)
		if err != nil {
			return err
		}
		outcome, err := adapter.derive.Handle(ctx, command)
		if err != nil {
			return err
		}
		if err := veconsume.Consumption(outcome); err != nil {
			return err
		}
	}
	return nil
}

func externalResultKeyFor(received veinbox.ReceivedExternalResult) (ccports.ExternalResultKey, error) {
	tenant, err := ccdomain.NewTenantID(received.TenantID)
	if err != nil {
		return ccports.ExternalResultKey{}, fmt.Errorf("%w: tenant: %v", ErrExternalResultUntranslatableAnswer, err)
	}
	if received.SourceID == "" {
		return ccports.ExternalResultKey{}, fmt.Errorf("%w: source ID is blank", ErrExternalResultUntranslatableAnswer)
	}
	return ccports.ExternalResultKey{TenantID: tenant, SourceID: received.SourceID}, nil
}

// externalResultKind 把记录实际形成的那一层译成类型字面量。各层语义不同，放行的三种
// 也不同——部分放行未被放行的那部分仍受门禁约束，附条件放行说得出条件，共用一个类型词
// 就会让目录一行把它们归进同一里程碑（与 TF 适配器 kindForHandoverVerdict 按裁决分开类型词
// 同一条道理）。落在封闭集合外，
// 或放行三件与层次不配，都是仓储不变量已破：CC 接收编排对非放行层带放行三件不受理、对
// 放行层缺放行三件停在解释未决，两种形状都落不了库。
func externalResultKind(record ccports.ExternalResultRecord) (vedomain.SourceFactKind, error) {
	none := vedomain.SourceFactKind{}
	layer := record.Result.Layer()
	if layer != ccdomain.ReleaseResultLayer && record.Release != nil {
		return none, fmt.Errorf("%w: release outcome on layer %q", ErrExternalResultRecordInconsistent, layer)
	}
	var raw string
	switch layer {
	case ccdomain.RegulatoryReceiptLayer:
		raw = "external-result-regulatory-receipt"
	case ccdomain.BusinessAcceptanceLayer:
		raw = "external-result-business-acceptance"
	case ccdomain.ProcessDecisionLayer:
		raw = "external-result-process-decision"
	case ccdomain.AssessedDutyLayer:
		raw = "external-result-assessed-duty"
	case ccdomain.DispositionDecisionLayer:
		raw = "external-result-disposition-decision"
	case ccdomain.ReleaseResultLayer:
		release := record.Release
		if release == nil {
			return none, fmt.Errorf("%w: release layer without release outcome", ErrExternalResultRecordInconsistent)
		}
		if release.Scope() != record.Result.Scope() {
			return none, fmt.Errorf("%w: release scope %q differs from result scope %q",
				ErrExternalResultRecordInconsistent, release.Scope(), record.Result.Scope())
		}
		switch release.Kind() {
		case ccdomain.FullRelease:
			raw = "external-result-release-full"
		case ccdomain.PartialRelease:
			raw = "external-result-release-partial"
		case ccdomain.ConditionalRelease:
			raw = "external-result-release-conditional"
		default:
			return none, fmt.Errorf("%w: release kind", ErrExternalResultRecordInconsistent)
		}
	default:
		return none, fmt.Errorf("%w: layer", ErrExternalResultRecordInconsistent)
	}
	kind, err := vedomain.NewSourceFactKind(raw)
	if err != nil {
		return none, fmt.Errorf("%w: source fact kind: %v", ErrExternalResultUntranslatableAnswer, err)
	}
	return kind, nil
}

func externalResultProjectionCommand(
	record ccports.ExternalResultRecord,
	kind vedomain.SourceFactKind,
	member ccdomain.DeclaredParcelReference,
) (veapplication.DeriveProjectionCommand, error) {
	none := veapplication.DeriveProjectionCommand{}
	tenant, err := vedomain.NewTenantID(record.Key.TenantID.String())
	if err != nil {
		return none, fmt.Errorf("%w: tenant: %v", ErrExternalResultUntranslatableAnswer, err)
	}
	parcel, err := vedomain.NewTrackedParcelReference(member.String())
	if err != nil {
		return none, fmt.Errorf("%w: declared parcel: %v", ErrExternalResultRecordInconsistent, err)
	}
	// 成员维进引用（ADR-0066）：缺它，同一封信的第二个成员就撞同键异内容的来源冲突并被
	// 当业务终局静默吞掉。来源标识随后——一次接收的身份就是（租户，来源标识），同一包裹
	// 的下一层结果来自另一次接收、另一个来源标识，各占各的引用。
	fact, err := vedomain.NewSourceFactReference("external-result/" + member.String() + "/" + record.Key.SourceID)
	if err != nil {
		return none, fmt.Errorf("%w: source fact: %v", ErrExternalResultUntranslatableAnswer, err)
	}
	// 版本取结果所关联的提交版本。接收记录只建立不改写：来源更正在 CC 形成另一次接收、
	// 另一个来源标识，不在同一引用下换版，所以这里没有前身可登记。
	version, err := vedomain.NewSourceFactVersion(record.Result.Version().String())
	if err != nil {
		return none, fmt.Errorf("%w: submission version: %v", ErrExternalResultUntranslatableAnswer, err)
	}
	// 发生与有效同取 CC 保存的「业务发生或适用时间」：那是 CC 按已登记解释规则对这一层
	// 作出的时间判断，也是它选规则所用的评估时点；投影不替所有者另判一个有效时间。接收
	// 时间取 CC 落记录的时刻，禁止用信封时间顶。
	occurred := record.Result.OccurredAt()
	return veapplication.DeriveProjectionCommand{
		TenantID: tenant,
		Fact: vedomain.AcceptedSourceFactSpec{
			Source:      vedomain.SourceCustomsCompliance,
			Parcel:      parcel,
			Fact:        fact,
			Kind:        kind,
			Version:     version,
			OccurredAt:  occurred,
			EffectiveAt: occurred,
			ReceivedAt:  record.RecordedAt,
		},
	}, nil
}
