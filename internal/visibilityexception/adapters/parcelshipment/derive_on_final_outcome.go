// Package parcelshipment 是 visibility-exception 消费 parcel-shipment 的适配器
// （ADR-0025 消费方侧）。它只翻译不判断：终局采用记录译成已接受源事实命令，归类与
// 派生由 DeriveProjectionHandler 回答；包裹反查答复译成货主客户账户引用，采认与
// 委托归属由 PS 回答。
//
// 终局投影是各投影路里唯一一条来源为 parcel-shipment 的：终局是 PS 拥有的事实，
// 不是履约侧事实。有效交付登记是它的**上游来源**，已有自己的投影账，两者各记各的。
package parcelshipment

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	psports "go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	veinbox "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/inbox"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/veconsume"
	veapplication "go.idp.xyz/idp-parcel/internal/visibilityexception/application"
	vedomain "go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
)

var (
	// ErrFinalNotVisible 表示按信封引用还读不回终局采用登记，或重派生那一代的前一代读时
	// 依赖调不通。可见性滞后与依赖不通都是续办，重投会改变结果；不当毒丸拒收。
	ErrFinalNotVisible = errors.New(
		"visibility exception parcelshipment adapter: final outcome is not yet visible")
	// ErrFinalRecordInconsistent 表示按键取回的登记指着另一个键、采认时刻为零，或重派生
	// 指回的前一代找不到、不是本包裹同一责任结果种类的已采用终局。仓储不变量已破
	// （ADR-0029），不是等谁。禁止进 WithUndecidedSentinels。
	ErrFinalRecordInconsistent = errors.New(
		"visibility exception parcelshipment adapter: final outcome disagrees with its key")
	// ErrFinalUntranslatableAnswer 表示引用译不成领域标识。引用坏了，不是等谁。
	ErrFinalUntranslatableAnswer = errors.New(
		"visibility exception parcelshipment adapter: untranslatable final outcome answer")
	// ErrFinalProjectionUndecided 与 ErrFinalProjectionHandoffPending 是 veconsume
	// 同名哨兵的终局侧别名。
	ErrFinalProjectionUndecided      = veconsume.ErrProjectionUndecided
	ErrFinalProjectionHandoffPending = veconsume.ErrProjectionHandoffPending
)

// FinalOutcomeFinder 按终局采用幂等键取回登记，并按终局版本找回重派生指回的前一代。由 PS
// 的 FinalOutcomes 满足。只有读法：本适配器不写 PS 的库。
type FinalOutcomeFinder interface {
	FindByKey(
		ctx context.Context,
		key psports.FinalAdoptionKey,
	) (psports.FinalOutcomeRecord, bool, error)
	psports.FinalOutcomeByVersionView
}

// FinalProjectionHandler 是派生编排在终局适配器侧的窄口。
type FinalProjectionHandler interface {
	Handle(
		ctx context.Context,
		command veapplication.DeriveProjectionCommand,
	) (veapplication.DeriveProjectionResult, error)
}

// DeriveOnFinalOutcomeAdapter 是 veinbox.FinalOutcomeConsumer 的真实处理方：按四维
// 引用重读 PS 终局采用判断，译成已接受源事实命令，再交给派生编排。
//
// 跨上下文翻译留在消费方（ADR-0025）。投影只引用源事实，不复制终局，也不回写终局。
type DeriveOnFinalOutcomeAdapter struct {
	finals FinalOutcomeFinder
	derive FinalProjectionHandler
}

func NewDeriveOnFinalOutcomeAdapter(
	finals FinalOutcomeFinder,
	derive FinalProjectionHandler,
) (*DeriveOnFinalOutcomeAdapter, error) {
	if finals == nil {
		return nil, fmt.Errorf("visibility exception parcelshipment adapter: final outcome finder is nil")
	}
	if derive == nil {
		return nil, fmt.Errorf("visibility exception parcelshipment adapter: projection handler is nil")
	}
	return &DeriveOnFinalOutcomeAdapter{finals: finals, derive: derive}, nil
}

var _ veinbox.FormedFinalOutcomeHandler = (*DeriveOnFinalOutcomeAdapter)(nil)

// HandleFormedFinalOutcome 按信封四维取回终局并派生投影。
//
// 两支都派生：不采用同样是已提交的终局判断，追踪要据它知道有一份来源没被采认（PS 侧
// FormParcelFinalHandler 的 handOff 注释就是这么定的）。省掉它会让迟到来源在追踪上
// 无声消失。
//
// FindByKey 必须带版本——更正走新版本新登记，按三维读「当前版」会把更正与原判断叠成
// 一次查找，重派生场景下静默拿错版本。
func (adapter *DeriveOnFinalOutcomeAdapter) HandleFormedFinalOutcome(
	ctx context.Context,
	formed veinbox.FormedFinalOutcome,
) error {
	key, err := finalKeyFor(formed)
	if err != nil {
		return err
	}
	record, found, err := adapter.finals.FindByKey(ctx, key)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrFinalNotVisible, err)
	}
	if !found {
		return fmt.Errorf("%w: parcel %q kind %q version %q",
			ErrFinalNotVisible, formed.Parcel, formed.Kind, formed.Version)
	}
	if record.Key != key {
		return fmt.Errorf("%w: parcel %q kind %q version %q",
			ErrFinalRecordInconsistent, formed.Parcel, formed.Kind, formed.Version)
	}
	// 采认时刻同时是本事实的业务时间，为零就没有可用的时间维——不是等谁，是那一行坏了。
	if record.AdoptedAt.IsZero() {
		return fmt.Errorf("%w: parcel %q kind %q version %q",
			ErrFinalRecordInconsistent, formed.Parcel, formed.Kind, formed.Version)
	}

	predecessor, superseding, err := adapter.predecessorOf(ctx, record)
	if err != nil {
		return err
	}
	command, err := finalProjectionCommand(record, predecessor, superseding, formed.VoidGeneration)
	if err != nil {
		return err
	}
	result, err := adapter.derive.Handle(ctx, command)
	if err != nil {
		return err
	}
	return veconsume.Consumption(result)
}

// predecessorOf 交回本代终局在投影那一维上的前身：前一代的采用版本。
//
// PS 的重派生只以终局版本（PriorVersion）指回前一代，投影的事实版本却是采用键的责任结果
// 版本——两套号。按终局版本找回前一代的采用记录、取它键上的版本，才是同一事实引用下 VE
// 认得的前身。不译，同种重派生之后两代已采用终局在当前有效集里同时在场，CONTEXT「被替代
// 的条目……不参与派生」就破了；替代关系只能由源上下文给出，所以这里只译 PS 指名的那一代，
// 不按到达先后或业务时间去猜。
//
// 前一代必须是同包裹、同责任结果种类的已采用终局。种类进事实引用，换了种类两代就不在同一
// 引用下，照 CONTEXT「只在同一源上下文、同一事实引用的版本之间成立」替代不成立，那是冲突。
// 今天 FormParcelFinalHandler 只在来源同种时重派生，异种一律不采用，所以走到那一格只能是账
// 坏了，按不一致拒收；ADR-0138（Proposed）的终局失效后按剩余依据改判若让异种接续成为正常
// 路径，那时另裁怎样进投影，不在这里静默译成一份无前身的首登。
func (adapter *DeriveOnFinalOutcomeAdapter) predecessorOf(
	ctx context.Context,
	record psports.FinalOutcomeRecord,
) (psdomain.ResponsibilityOutcomeVersion, bool, error) {
	none := psdomain.ResponsibilityOutcomeVersion{}
	if !record.Finalized {
		return none, false, nil
	}
	prior, rederived := record.Final.PriorVersion()
	if !rederived {
		return none, false, nil
	}
	key := record.Key
	predecessor, found, err := adapter.finals.FindByFinalVersion(ctx, key.TenantID, key.Parcel, prior)
	if err != nil {
		return none, false, fmt.Errorf("%w: predecessor %q of parcel %q kind %q version %q: %v",
			ErrFinalNotVisible, prior, key.Parcel, key.Kind, key.Version, err)
	}
	// 读不到不是滞后：前一代在本代提交之前就已在账上（本代正是从它那一行翻过来的），本代
	// 读得到而它读不到，只能是账坏了。
	if !found {
		return none, false, fmt.Errorf("%w: predecessor %q of parcel %q kind %q version %q is missing",
			ErrFinalRecordInconsistent, prior, key.Parcel, key.Kind, key.Version)
	}
	if !predecessor.Finalized ||
		predecessor.Final.Version() != prior ||
		predecessor.Key.TenantID != key.TenantID ||
		predecessor.Key.Parcel != key.Parcel ||
		predecessor.Key.Kind != key.Kind ||
		predecessor.Key.Version == key.Version {
		return none, false, fmt.Errorf("%w: predecessor %q of parcel %q kind %q version %q is parcel %q kind %q version %q",
			ErrFinalRecordInconsistent, prior, key.Parcel, key.Kind, key.Version,
			predecessor.Key.Parcel, predecessor.Key.Kind, predecessor.Key.Version)
	}
	return predecessor.Key.Version, true, nil
}

// finalKeyFor 把信封四维译回采用键。种类是封闭枚举，反解析用 PS 领域包导出的那一份
// ——字符串形式在那里定义，本包再抄一份 switch 会让日后新增一个责任结果种类时无人
// 提醒。
func finalKeyFor(formed veinbox.FormedFinalOutcome) (psports.FinalAdoptionKey, error) {
	none := psports.FinalAdoptionKey{}
	tenant, err := psdomain.NewTenantID(formed.TenantID)
	if err != nil {
		return none, fmt.Errorf("%w: tenant: %v", ErrFinalUntranslatableAnswer, err)
	}
	parcel, err := psdomain.NewDeclaredParcelID(formed.Parcel)
	if err != nil {
		return none, fmt.Errorf("%w: parcel: %v", ErrFinalUntranslatableAnswer, err)
	}
	kind, err := psdomain.NewResponsibilityOutcomeKind(formed.Kind)
	if err != nil {
		return none, fmt.Errorf("%w: responsibility outcome kind: %v", ErrFinalUntranslatableAnswer, err)
	}
	version, err := psdomain.NewResponsibilityOutcomeVersion(formed.Version)
	if err != nil {
		return none, fmt.Errorf("%w: responsibility outcome version: %v", ErrFinalUntranslatableAnswer, err)
	}
	return psports.FinalAdoptionKey{
		TenantID: tenant,
		Parcel:   parcel,
		Kind:     kind,
		Version:  version,
	}, nil
}

// finalFactVersion 是终局事实在投影那一维上的版本：作废代次为零时就是采用键的责任结果版本；代次大于零时在其后追加 `#void-<代次>`。
// 同一采用键因取消被认定不成立而重新形成记录（不采用行被删、续办重新形成终局，或再被另一份取消挡成不采用）时，事实键
// （租户、来源、事实引用、版本）若仍是同一把，新记录与此前那一份只在种类和时间上不同，会被判成来源冲突、保留原事实。
// 带代次的版本是确定性的：同一代次重放得同一个版本，答已有结果。
func finalFactVersion(adoption psdomain.ResponsibilityOutcomeVersion, voidGeneration int) string {
	if voidGeneration <= 0 {
		return adoption.String()
	}
	return adoption.String() + "#void-" + strconv.Itoa(voidGeneration)
}

func finalProjectionCommand(
	record psports.FinalOutcomeRecord,
	predecessor psdomain.ResponsibilityOutcomeVersion,
	superseding bool,
	voidGeneration int,
) (veapplication.DeriveProjectionCommand, error) {
	none := veapplication.DeriveProjectionCommand{}
	tenant, err := vedomain.NewTenantID(record.Key.TenantID.String())
	if err != nil {
		return none, fmt.Errorf("%w: tenant: %v", ErrFinalUntranslatableAnswer, err)
	}
	parcel, err := vedomain.NewTrackedParcelReference(record.Key.Parcel.String())
	if err != nil {
		return none, fmt.Errorf("%w: parcel: %v", ErrFinalUntranslatableAnswer, err)
	}
	// 前缀把终局事实键与同包揽收/交付/交接错开；责任结果种类进引用，因为同一包裹可有
	// 多条彼此独立的终局流（各自的来源责任不同）。版本不进引用——它已经是版本维，
	// 一维一职。
	fact, err := vedomain.NewSourceFactReference(
		"final-outcome/" + record.Key.Parcel.String() + "/" + record.Key.Kind.String())
	if err != nil {
		return none, fmt.Errorf("%w: source fact: %v", ErrFinalUntranslatableAnswer, err)
	}
	version, err := vedomain.NewSourceFactVersion(finalFactVersion(record.Key.Version, voidGeneration))
	if err != nil {
		return none, fmt.Errorf("%w: responsibility outcome version: %v", ErrFinalUntranslatableAnswer, err)
	}
	kind, err := kindForFinalRecord(record)
	if err != nil {
		return none, err
	}
	// 前身与本代同一事实引用，版本维同译法：首派生与不采用都没有前身，留零值。重派生沿用 PS 指名的前一代采用版本；
	// 不是重派生而作废代次大于零的，本代取代的是同一采用键上上一代次的那一份（代次为一时就是代次为零的原版本）——被删的不采用行
	// 当初投进来的事实，与重新形成的终局或再次不采用同一事实引用、不同版本，替代关系由此成立，被取代的不参与派生。
	var supersedes vedomain.SourceFactVersion
	switch {
	case superseding:
		supersedes, err = vedomain.NewSourceFactVersion(predecessor.String())
	case voidGeneration > 0:
		supersedes, err = vedomain.NewSourceFactVersion(finalFactVersion(record.Key.Version, voidGeneration-1))
	}
	if err != nil {
		return none, fmt.Errorf("%w: superseded responsibility outcome version: %v", ErrFinalUntranslatableAnswer, err)
	}

	// 采认判定本身就是这份事实的业务时刻——同 JudgedAt 之于交接，不是拿处理时间顶替
	// 业务时间。AdoptedAt 由 PS 应用层的 Clock 端口记录，不是库默认时间戳。
	occurred := record.AdoptedAt
	// 有效时间两支分开取，这是四路里第一条 EffectiveAt 可以不等于 OccurredAt 的：
	// 已采认支的源侧给得出第三时间（被采用责任结果的业务时间），翻译一步把它塌成采认
	// 时刻等于丢掉源真值；不采用支没有 Final 对象因而给不出，只能同 OccurredAt。
	effective := occurred
	if record.Finalized {
		effective = record.Final.EffectiveAt()
	}
	return veapplication.DeriveProjectionCommand{
		TenantID: tenant,
		Fact: vedomain.AcceptedSourceFactSpec{
			Source:      vedomain.SourceParcelShipment,
			Parcel:      parcel,
			Fact:        fact,
			Kind:        kind,
			Version:     version,
			Supersedes:  supersedes,
			OccurredAt:  occurred,
			EffectiveAt: effective,
			ReceivedAt:  record.AdoptedAt,
		},
	}, nil
}

// kindForFinalRecord 把采用/不采用两格译成两个映射类型字面量。已采认与未采认语义
// 相反，不能共用一个 final-outcome 类型——否则目录一行会把两格都归进同一里程碑
// （同 kindForHandoverVerdict 的理由）。
//
// 分格止于 Finalized：终局本体上的 FinalKindReference 属实例半边（租户合同定义
// 终局类型目录），把它炼进字面量等于让机制半边依赖一份还不存在的实例值。它作为不透明
// 身份串留在终局本体里，不进这里。
func kindForFinalRecord(record psports.FinalOutcomeRecord) (vedomain.SourceFactKind, error) {
	raw := vedomain.FinalOutcomeNotAdoptedFactKind
	if record.Finalized {
		raw = vedomain.FinalOutcomeAdoptedFactKind
	}
	kind, err := vedomain.NewSourceFactKind(raw)
	if err != nil {
		return vedomain.SourceFactKind{}, fmt.Errorf("%w: source fact kind: %v", ErrFinalUntranslatableAnswer, err)
	}
	return kind, nil
}
