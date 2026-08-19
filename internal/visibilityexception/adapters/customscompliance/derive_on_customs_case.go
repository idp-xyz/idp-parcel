// Package customscompliance 承载 VE 消费 customs-compliance 事实的翻译适配器。
// 跨上下文翻译留在消费方（ADR-0025）：这里按信封键重读 CC 本体、译成已接受源事实
// 命令，不写 CC 的库，也不复制 CC 的权威事实。
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
	// ErrCustomsCaseNotVisible 表示按信封引用还读不回案件。可见性滞后是续办，重投
	// 会改变结果；不当毒丸拒收。
	ErrCustomsCaseNotVisible = errors.New(
		"visibility exception customscompliance adapter: customs case is not yet visible")
	// ErrCustomsCaseRecordInconsistent 表示按键取回的案件指着另一个键、身份或时间
	// 为零、或成员关联为空。仓储不变量已破（ADR-0029），不是等谁。禁止进
	// WithUndecidedSentinels。
	ErrCustomsCaseRecordInconsistent = errors.New(
		"visibility exception customscompliance adapter: customs case disagrees with its key")
	// ErrCustomsCaseUntranslatableAnswer 表示引用译不成领域标识。引用坏了，不是等谁。
	ErrCustomsCaseUntranslatableAnswer = errors.New(
		"visibility exception customscompliance adapter: untranslatable customs case answer")
	// ErrProjectionUndecided 与 ErrProjectionHandoffPending 是 veconsume 同名哨兵的
	// 本包别名，生产装配据此登记未决哨兵。
	ErrProjectionUndecided      = veconsume.ErrProjectionUndecided
	ErrProjectionHandoffPending = veconsume.ErrProjectionHandoffPending
)

// CustomsCaseFinder 按案件身份键取回案件。由 CC 的 CustomsCaseStore 满足。只取一法：
// 本适配器不写 CC 的库。
type CustomsCaseFinder interface {
	FindByKey(
		ctx context.Context,
		key ccports.CustomsCaseKey,
	) (ccdomain.CustomsCase, bool, error)
}

// CaseProjectionHandler 是派生编排在案件适配器侧的窄口。
type CaseProjectionHandler interface {
	Handle(
		ctx context.Context,
		command veapplication.DeriveProjectionCommand,
	) (veapplication.DeriveProjectionResult, error)
}

// DeriveOnCustomsCaseAdapter 是 veinbox.CustomsCaseConsumer 的真实处理方：按五维
// 引用重读 CC 案件，对成员关联逐包裹译成已接受源事实命令，交给派生编排（ADR-0066
// 消费侧循环拆分）。
//
// 成员关联自带的客户归属与来源资料引用不译进事实——投影按包裹立键，客户维属客户
// 视图派生那条链，由它按需回读 CC，不在这里复制第二份。
type DeriveOnCustomsCaseAdapter struct {
	cases  CustomsCaseFinder
	derive CaseProjectionHandler
}

func NewDeriveOnCustomsCaseAdapter(
	cases CustomsCaseFinder,
	derive CaseProjectionHandler,
) (*DeriveOnCustomsCaseAdapter, error) {
	if cases == nil {
		return nil, fmt.Errorf("visibility exception customscompliance adapter: case finder is nil")
	}
	if derive == nil {
		return nil, fmt.Errorf("visibility exception customscompliance adapter: projection handler is nil")
	}
	return &DeriveOnCustomsCaseAdapter{cases: cases, derive: derive}, nil
}

var _ veinbox.EstablishedCustomsCaseHandler = (*DeriveOnCustomsCaseAdapter)(nil)

// HandleEstablishedCustomsCase 按信封五维取回案件，对成员关联逐包裹派生投影。
//
// 一封信一笔事务：任一成员未决或意图未交即整封报错回滚、重投从头再跑（头端阻塞是
// ADR-0066 认下的代价，换整封原子）；单成员业务终局（冲突、不接受）入账继续。业务
// 发生时间取案件 EstablishedAt；CC 的读口只交回聚合、不带登记时刻，接收时间如实取
// 提供方唯一拥有的同一时刻，不拿适配器时钟顶替。
func (adapter *DeriveOnCustomsCaseAdapter) HandleEstablishedCustomsCase(
	ctx context.Context,
	established veinbox.EstablishedCustomsCase,
) error {
	key, err := customsCaseKeyFor(established)
	if err != nil {
		return err
	}
	customsCase, found, err := adapter.cases.FindByKey(ctx, key)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrCustomsCaseNotVisible, err)
	}
	if !found {
		return fmt.Errorf("%w: jurisdiction %q procedure %q obligation %q",
			ErrCustomsCaseNotVisible, established.Jurisdiction, established.Procedure, established.Obligation)
	}
	if customsCase.Jurisdiction() != key.Jurisdiction ||
		customsCase.Direction() != key.Direction ||
		customsCase.Procedure() != key.Procedure ||
		customsCase.Obligation() != key.Obligation {
		return fmt.Errorf("%w: jurisdiction %q procedure %q obligation %q",
			ErrCustomsCaseRecordInconsistent, established.Jurisdiction, established.Procedure, established.Obligation)
	}
	if customsCase.ID().String() == "" || customsCase.EstablishedAt().IsZero() {
		return fmt.Errorf("%w: jurisdiction %q procedure %q obligation %q",
			ErrCustomsCaseRecordInconsistent, established.Jurisdiction, established.Procedure, established.Obligation)
	}
	associations := customsCase.Parcels()
	if len(associations) == 0 {
		return fmt.Errorf("%w: case %q has no parcel associations",
			ErrCustomsCaseRecordInconsistent, customsCase.ID())
	}

	for _, association := range associations {
		command, err := customsCaseProjectionCommand(key.TenantID, customsCase, association)
		if err != nil {
			return err
		}
		result, err := adapter.derive.Handle(ctx, command)
		if err != nil {
			return err
		}
		if err := veconsume.Consumption(result); err != nil {
			return err
		}
	}
	return nil
}

func customsCaseKeyFor(established veinbox.EstablishedCustomsCase) (ccports.CustomsCaseKey, error) {
	none := ccports.CustomsCaseKey{}
	tenant, err := ccdomain.NewTenantID(established.TenantID)
	if err != nil {
		return none, fmt.Errorf("%w: tenant: %v", ErrCustomsCaseUntranslatableAnswer, err)
	}
	jurisdiction, err := ccdomain.NewRegulatoryJurisdictionReference(established.Jurisdiction)
	if err != nil {
		return none, fmt.Errorf("%w: jurisdiction: %v", ErrCustomsCaseUntranslatableAnswer, err)
	}
	direction, err := manifestDirectionFrom(established.Direction)
	if err != nil {
		return none, err
	}
	procedure, err := ccdomain.NewCustomsProcedureReference(established.Procedure)
	if err != nil {
		return none, fmt.Errorf("%w: procedure: %v", ErrCustomsCaseUntranslatableAnswer, err)
	}
	obligation, err := ccdomain.NewObligationScopeReference(established.Obligation)
	if err != nil {
		return none, fmt.Errorf("%w: obligation scope: %v", ErrCustomsCaseUntranslatableAnswer, err)
	}
	return ccports.CustomsCaseKey{
		TenantID:     tenant,
		Jurisdiction: jurisdiction,
		Direction:    direction,
		Procedure:    procedure,
		Obligation:   obligation,
	}, nil
}

// manifestDirectionFrom 把载荷里的方向字面量译回封闭二值。落在集合外是信封坏了，不是等谁。
func manifestDirectionFrom(raw string) (ccdomain.ManifestDirection, error) {
	switch raw {
	case ccdomain.ImportManifest.String():
		return ccdomain.ImportManifest, nil
	case ccdomain.ExportManifest.String():
		return ccdomain.ExportManifest, nil
	default:
		return ccdomain.ManifestDirectionInvalid, fmt.Errorf("%w: direction %q",
			ErrCustomsCaseUntranslatableAnswer, raw)
	}
}

func customsCaseProjectionCommand(
	tenantID ccdomain.TenantID,
	customsCase ccdomain.CustomsCase,
	association ccdomain.CaseParcelAssociation,
) (veapplication.DeriveProjectionCommand, error) {
	none := veapplication.DeriveProjectionCommand{}
	tenant, err := vedomain.NewTenantID(tenantID.String())
	if err != nil {
		return none, fmt.Errorf("%w: tenant: %v", ErrCustomsCaseUntranslatableAnswer, err)
	}
	parcel, err := vedomain.NewTrackedParcelReference(association.Parcel)
	if err != nil {
		return none, fmt.Errorf("%w: associated parcel: %v", ErrCustomsCaseRecordInconsistent, err)
	}
	// 成员维与案件维一并进引用（ADR-0066）：缺成员维，同一封信的第二个包裹就撞同键
	// 异内容的来源冲突；缺案件维，同一包裹关联的下一个独立案件会撞上一个案件的事实。
	// 案件维用铸造的案件标识，不重复五维键——标识已由建案编排钉在这个键上。
	fact, err := vedomain.NewSourceFactReference(
		"customs-case/" + association.Parcel + "/" + customsCase.ID().String())
	if err != nil {
		return none, fmt.Errorf("%w: source fact: %v", ErrCustomsCaseUntranslatableAnswer, err)
	}
	// 版本取案件标识：案件建立没有版本演进（固定身份变化建的是替代案件、新键新信封），
	// 同键即同案件；若同键将来真出现另一个案件身份，版本维会把两代分开而不是相互覆盖。
	version, err := vedomain.NewSourceFactVersion(customsCase.ID().String())
	if err != nil {
		return none, fmt.Errorf("%w: case ID: %v", ErrCustomsCaseUntranslatableAnswer, err)
	}
	// 建立是一件事、无语义分支，单一类型字面量；映不映成里程碑（或只进内部维度）由
	// 版本化映射目录决定，未配置即如实未归类。
	kind, err := vedomain.NewSourceFactKind("customs-case-established")
	if err != nil {
		return none, fmt.Errorf("%w: source fact kind: %v", ErrCustomsCaseUntranslatableAnswer, err)
	}
	occurred := customsCase.EstablishedAt()
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
			ReceivedAt:  occurred,
		},
	}, nil
}
