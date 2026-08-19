// Package nodeoperations 是 visibility-exception 消费 node-operations 节点收寄结果
// 的适配器（ADR-0025 消费方侧）。它只翻译不判断：收寄记录译成已接受源事实命令，
// 归类与派生由 DeriveProjectionHandler 回答。
package nodeoperations

import (
	"context"
	"errors"
	"fmt"

	nodomain "go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	noports "go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
	veinbox "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/inbox"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/veconsume"
	veapplication "go.idp.xyz/idp-parcel/internal/visibilityexception/application"
	vedomain "go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
)

var (
	// ErrReceptionNotVisible 表示按信封引用还读不回形成收寄，或读到的是未决格。
	// 可见性滞后是续办，重投会改变结果；不当毒丸拒收。
	ErrReceptionNotVisible = errors.New(
		"visibility exception nodeoperations adapter: node intake reception is not yet visible")
	// ErrUnidentifiedHandlingUnit 说明这份收寄还挂在待识别实物上，或形成格没有版本化
	// 包裹关联。投影锚在包裹身份上——识别成功形成新版本后再来。等识别是业务续办。
	ErrUnidentifiedHandlingUnit = errors.New(
		"visibility exception nodeoperations adapter: the handling unit is not yet identified")
	// ErrReceptionRecordInconsistent 表示按键取回的记录指着另一个键，或三时间缺席。
	// 仓储不变量已破（ADR-0029），不是等谁。禁止进 WithUndecidedSentinels。
	ErrReceptionRecordInconsistent = errors.New(
		"visibility exception nodeoperations adapter: node intake reception disagrees with its key")
	// ErrUntranslatableAnswer 表示引用译不成领域标识。引用坏了，不是等谁。
	ErrUntranslatableAnswer = errors.New(
		"visibility exception nodeoperations adapter: untranslatable answer")
	// ErrProjectionUndecided 与 ErrProjectionHandoffPending 是 veconsume 同名哨兵的
	// 别名：派生结果如何落成消费两格由 veconsume 一处回答，本包只暴露口。
	ErrProjectionUndecided      = veconsume.ErrProjectionUndecided
	ErrProjectionHandoffPending = veconsume.ErrProjectionHandoffPending
)

// ReceptionFinder 按收寄幂等键取回判断记录。由 NO 的 ReceptionStore 满足。
type ReceptionFinder interface {
	FindByKey(ctx context.Context, key noports.ReceptionKey) (noports.ReceptionRecord, bool, error)
}

// ProjectionHandler 是派生编排在本适配器侧的窄口。真实装配交给
// *application.DeriveProjectionHandler。
type ProjectionHandler interface {
	Handle(
		ctx context.Context,
		command veapplication.DeriveProjectionCommand,
	) (veapplication.DeriveProjectionResult, error)
}

// DeriveOnNodeIntakeAdapter 是 veinbox.NodeIntakeConsumer 的真实处理方：按引用重读
// NO 收寄记录，译成已接受源事实命令，再交给派生编排。
//
// 跨上下文翻译留在消费方（ADR-0025）。投影只引用源事实，不复制节点收寄。
type DeriveOnNodeIntakeAdapter struct {
	receptions ReceptionFinder
	derive     ProjectionHandler
}

func NewDeriveOnNodeIntakeAdapter(
	receptions ReceptionFinder,
	derive ProjectionHandler,
) (*DeriveOnNodeIntakeAdapter, error) {
	if receptions == nil {
		return nil, fmt.Errorf("visibility exception nodeoperations adapter: reception finder is nil")
	}
	if derive == nil {
		return nil, fmt.Errorf("visibility exception nodeoperations adapter: projection handler is nil")
	}
	return &DeriveOnNodeIntakeAdapter{receptions: receptions, derive: derive}, nil
}

var _ veinbox.FormedNodeIntakeHandler = (*DeriveOnNodeIntakeAdapter)(nil)

// HandleFormedNodeIntake 按信封引用取回收寄并派生投影。
//
// 信封只做唤醒指针：业务发生时间取 NodeIntake.ReceivedAt，接收时间取记录
// RecordedAt，禁止用信封 OccurredAt/RecordedAt 顶业务时间。
func (adapter *DeriveOnNodeIntakeAdapter) HandleFormedNodeIntake(
	ctx context.Context,
	formed veinbox.FormedNodeIntake,
) error {
	noTenant, err := nodomain.NewTenantID(formed.TenantID)
	if err != nil {
		return fmt.Errorf("%w: tenant: %v", ErrUntranslatableAnswer, err)
	}
	record, found, err := adapter.receptions.FindByKey(ctx, noports.ReceptionKey{
		TenantID: noTenant,
		SourceID: formed.SourceID,
	})
	if err != nil {
		return fmt.Errorf("%w: %v", ErrReceptionNotVisible, err)
	}
	if !found {
		return fmt.Errorf("%w: source %q", ErrReceptionNotVisible, formed.SourceID)
	}

	switch record.Kind {
	case noports.RecordIntakeNotFormed:
		// 不是已接受收寄事实：入账跳过，不派生。
		return nil
	case noports.RecordPendingIdentification:
		return ErrUnidentifiedHandlingUnit
	case noports.RecordIntakeFormed:
	default:
		return fmt.Errorf("%w: source %q", ErrReceptionNotVisible, formed.SourceID)
	}

	if record.Key.TenantID != noTenant || record.Key.SourceID != formed.SourceID ||
		record.Intake.TenantID() != noTenant {
		return fmt.Errorf("%w: source %q", ErrReceptionRecordInconsistent, formed.SourceID)
	}
	if record.Intake.ReceivedAt().IsZero() || record.RecordedAt.IsZero() {
		return fmt.Errorf("%w: source %q", ErrReceptionRecordInconsistent, formed.SourceID)
	}

	association, identified := record.Intake.Association()
	if !identified {
		return ErrUnidentifiedHandlingUnit
	}

	command, err := projectionCommand(formed, record, association)
	if err != nil {
		return err
	}
	result, err := adapter.derive.Handle(ctx, command)
	if err != nil {
		return err
	}
	return veconsume.Consumption(result)
}

func projectionCommand(
	formed veinbox.FormedNodeIntake,
	record noports.ReceptionRecord,
	association nodomain.ParcelAssociationReference,
) (veapplication.DeriveProjectionCommand, error) {
	none := veapplication.DeriveProjectionCommand{}
	tenant, err := vedomain.NewTenantID(formed.TenantID)
	if err != nil {
		return none, fmt.Errorf("%w: tenant: %v", ErrUntranslatableAnswer, err)
	}
	parcel, err := vedomain.NewTrackedParcelReference(association.String())
	if err != nil {
		return none, fmt.Errorf("%w: parcel association: %v", ErrUntranslatableAnswer, err)
	}
	fact, err := vedomain.NewSourceFactReference(formed.SourceID)
	if err != nil {
		return none, fmt.Errorf("%w: source fact: %v", ErrUntranslatableAnswer, err)
	}
	version, err := vedomain.NewSourceFactVersion(record.Intake.Version().String())
	if err != nil {
		return none, fmt.Errorf("%w: intake version: %v", ErrUntranslatableAnswer, err)
	}
	kind, err := vedomain.NewSourceFactKind("node-intake")
	if err != nil {
		return none, fmt.Errorf("%w: source fact kind: %v", ErrUntranslatableAnswer, err)
	}
	occurred := record.Intake.ReceivedAt()
	return veapplication.DeriveProjectionCommand{
		TenantID: tenant,
		Fact: vedomain.AcceptedSourceFactSpec{
			Source:      vedomain.SourceNodeOperations,
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
