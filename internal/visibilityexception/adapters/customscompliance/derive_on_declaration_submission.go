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
	// ErrDeclarationSubmissionNotVisible 表示按信封引用还读不回提交版本。可见性滞后
	// 是续办，重投会改变结果；不当毒丸拒收。
	ErrDeclarationSubmissionNotVisible = errors.New(
		"visibility exception customscompliance adapter: declaration submission is not yet visible")
	// ErrDeclarationSubmissionRecordInconsistent 表示按键取回的提交指着另一个键、版本
	// 身份与载荷宣告不符、时间为零或成员快照为空。仓储不变量已破（ADR-0029），不是
	// 等谁。禁止进 WithUndecidedSentinels。
	ErrDeclarationSubmissionRecordInconsistent = errors.New(
		"visibility exception customscompliance adapter: declaration submission disagrees with its key")
	// ErrDeclarationSubmissionUntranslatableAnswer 表示引用译不成领域标识。引用坏了，
	// 不是等谁。
	ErrDeclarationSubmissionUntranslatableAnswer = errors.New(
		"visibility exception customscompliance adapter: untranslatable declaration submission answer")
)

// DeclarationSubmissionFinder 按信封宣告的提交版本取回留存版本。由 CC 的
// DeclarationSubmissionStore 满足。只取一法：本适配器不写 CC 的库。按版本而不按
// 目标键读——原案内更正落地后同一目标容纳多版本（CC 迁移 0012），按键只答当前版，
// 迟到重放旧版信封会读到新版内容撞出假冲突；按版本读回的所指不随当前版推进漂移。
type DeclarationSubmissionFinder interface {
	FindByVersion(
		ctx context.Context,
		tenant ccdomain.TenantID,
		version ccdomain.SubmissionVersionID,
	) (ccports.DeclarationSubmissionRecord, bool, error)
}

// SubmissionProjectionHandler 是派生编排在提交申报适配器侧的窄口。与同包案件侧的
// CaseProjectionHandler 同形不同名：各适配器的窄口各自具名，装配处谁接谁一眼可见。
type SubmissionProjectionHandler interface {
	Handle(
		ctx context.Context,
		command veapplication.DeriveProjectionCommand,
	) (veapplication.DeriveProjectionResult, error)
}

// DeriveOnDeclarationSubmissionAdapter 是 veinbox.DeclarationSubmissionConsumer 的
// 真实处理方：按载荷版本维重读 CC 留存的那一版、以三维目标键交叉核对，对成员快照
// 逐包裹译成已接受源事实命令，交给派生编排（ADR-0066 消费侧循环拆分）。原案内更正
// 版携带的前身（CorrectedFrom）译成来源事实替代关系——由源上下文随更正一并给出
// （VE CONTEXT 词条），本适配器只登记不推断。
//
// 成员快照携带的卷宗、角色与授权引用不译进事实——投影按包裹立键，只引用源事实，
// 不复制申报本体。
type DeriveOnDeclarationSubmissionAdapter struct {
	submissions DeclarationSubmissionFinder
	derive      SubmissionProjectionHandler
}

func NewDeriveOnDeclarationSubmissionAdapter(
	submissions DeclarationSubmissionFinder,
	derive SubmissionProjectionHandler,
) (*DeriveOnDeclarationSubmissionAdapter, error) {
	if submissions == nil {
		return nil, fmt.Errorf("visibility exception customscompliance adapter: submission finder is nil")
	}
	if derive == nil {
		return nil, fmt.Errorf("visibility exception customscompliance adapter: projection handler is nil")
	}
	return &DeriveOnDeclarationSubmissionAdapter{submissions: submissions, derive: derive}, nil
}

var _ veinbox.FormedDeclarationSubmissionHandler = (*DeriveOnDeclarationSubmissionAdapter)(nil)

// HandleFormedDeclarationSubmission 按载荷版本取回留存版本，对成员快照逐包裹派生投影。
//
// 一封信一笔事务：任一成员未决或意图未交即整封报错回滚、重投从头再跑（头端阻塞是
// ADR-0066 认下的代价，换整封原子）；单成员业务终局（冲突、不接受）入账继续。业务
// 发生时间取版本 FixedAt，有效时间同发生时间，接收时间取记录 RecordedAt，禁止用
// 信封时间顶业务时间。
func (adapter *DeriveOnDeclarationSubmissionAdapter) HandleFormedDeclarationSubmission(
	ctx context.Context,
	formed veinbox.FormedDeclarationSubmission,
) error {
	key, declared, err := declarationSubmissionKeyFor(formed)
	if err != nil {
		return err
	}
	record, found, err := adapter.submissions.FindByVersion(ctx, key.TenantID, declared)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrDeclarationSubmissionNotVisible, err)
	}
	if !found {
		return fmt.Errorf("%w: unit %q procedure %q version %q",
			ErrDeclarationSubmissionNotVisible, formed.UnitID, formed.Procedure, formed.VersionID)
	}
	version := record.Version
	// 三维目标键交叉核对：按版本取回的行必须就是信封宣告的那个逻辑申报目标——
	// 不符即仓储不变量已破（版本标识按租户唯一，指到别的单元说明键被写串了）。
	if record.Key != key || version.Unit() != key.Unit || version.ID() != declared {
		return fmt.Errorf("%w: unit %q procedure %q version %q",
			ErrDeclarationSubmissionRecordInconsistent, formed.UnitID, formed.Procedure, formed.VersionID)
	}
	if version.FixedAt().IsZero() || record.RecordedAt.IsZero() {
		return fmt.Errorf("%w: unit %q procedure %q version %q",
			ErrDeclarationSubmissionRecordInconsistent, formed.UnitID, formed.Procedure, formed.VersionID)
	}
	members := version.Members()
	if len(members) == 0 {
		return fmt.Errorf("%w: version %q has no member snapshot",
			ErrDeclarationSubmissionRecordInconsistent, version.ID())
	}

	for _, member := range members {
		command, err := declarationSubmissionProjectionCommand(record, member)
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

func declarationSubmissionKeyFor(
	formed veinbox.FormedDeclarationSubmission,
) (ccports.DeclarationSubmissionKey, ccdomain.SubmissionVersionID, error) {
	none := ccports.DeclarationSubmissionKey{}
	noVersion := ccdomain.SubmissionVersionID{}
	tenant, err := ccdomain.NewTenantID(formed.TenantID)
	if err != nil {
		return none, noVersion, fmt.Errorf("%w: tenant: %v", ErrDeclarationSubmissionUntranslatableAnswer, err)
	}
	unit, err := ccdomain.NewDeclarationUnitID(formed.UnitID)
	if err != nil {
		return none, noVersion, fmt.Errorf("%w: declaration unit: %v", ErrDeclarationSubmissionUntranslatableAnswer, err)
	}
	procedure, err := ccdomain.NewCustomsProcedureReference(formed.Procedure)
	if err != nil {
		return none, noVersion, fmt.Errorf("%w: procedure: %v", ErrDeclarationSubmissionUntranslatableAnswer, err)
	}
	declared, err := ccdomain.NewSubmissionVersionID(formed.VersionID)
	if err != nil {
		return none, noVersion, fmt.Errorf("%w: submission version: %v", ErrDeclarationSubmissionUntranslatableAnswer, err)
	}
	return ccports.DeclarationSubmissionKey{
		TenantID:  tenant,
		Unit:      unit,
		Procedure: procedure,
	}, declared, nil
}

func declarationSubmissionProjectionCommand(
	record ccports.DeclarationSubmissionRecord,
	member ccdomain.DeclaredParcelReference,
) (veapplication.DeriveProjectionCommand, error) {
	none := veapplication.DeriveProjectionCommand{}
	tenant, err := vedomain.NewTenantID(record.Key.TenantID.String())
	if err != nil {
		return none, fmt.Errorf("%w: tenant: %v", ErrDeclarationSubmissionUntranslatableAnswer, err)
	}
	parcel, err := vedomain.NewTrackedParcelReference(member.String())
	if err != nil {
		return none, fmt.Errorf("%w: declared parcel: %v", ErrDeclarationSubmissionRecordInconsistent, err)
	}
	// 成员维进引用不进类型（ADR-0066）：缺成员维，同一封信的第二个成员就撞同键异
	// 内容的来源冲突并被当业务终局静默吞掉。单元与程序随后——同一逻辑申报目标
	// （租户+单元+程序）的引用跨版本稳定，版本演进走版本维，不走引用。
	fact, err := vedomain.NewSourceFactReference(
		"declaration-submission/" + member.String() + "/" + record.Key.Unit.String() +
			"/" + record.Key.Procedure.String())
	if err != nil {
		return none, fmt.Errorf("%w: source fact: %v", ErrDeclarationSubmissionUntranslatableAnswer, err)
	}
	// 版本取提交版本标识：「原案内更正」保留申报单元身份、同键形成新提交版本时，
	// 同一引用换版本，两代按 ADR-0065 各自留存不相互覆盖。
	factVersion, err := vedomain.NewSourceFactVersion(record.Version.ID().String())
	if err != nil {
		return none, fmt.Errorf("%w: submission version: %v", ErrDeclarationSubmissionUntranslatableAnswer, err)
	}
	// 替代关系由源上下文随更正一并给出（VE CONTEXT「来源事实替代关系」）：更正版
	// 记录携带被更正的前身版本，这里只转写不推断；首版无前身即首登事实。
	var supersedes vedomain.SourceFactVersion
	if record.CorrectedFrom.String() != "" {
		if supersedes, err = vedomain.NewSourceFactVersion(record.CorrectedFrom.String()); err != nil {
			return none, fmt.Errorf("%w: corrected from: %v", ErrDeclarationSubmissionUntranslatableAnswer, err)
		}
	}
	// 形成是一件事、无语义分支，单一类型字面量；映不映成里程碑由版本化映射目录决定，
	// 未配置即如实未归类。
	kind, err := vedomain.NewSourceFactKind("declaration-submission-formed")
	if err != nil {
		return none, fmt.Errorf("%w: source fact kind: %v", ErrDeclarationSubmissionUntranslatableAnswer, err)
	}
	occurred := record.Version.FixedAt()
	return veapplication.DeriveProjectionCommand{
		TenantID: tenant,
		Fact: vedomain.AcceptedSourceFactSpec{
			Source:      vedomain.SourceCustomsCompliance,
			Parcel:      parcel,
			Fact:        fact,
			Kind:        kind,
			Version:     factVersion,
			Supersedes:  supersedes,
			OccurredAt:  occurred,
			EffectiveAt: occurred,
			ReceivedAt:  record.RecordedAt,
		},
	}, nil
}
