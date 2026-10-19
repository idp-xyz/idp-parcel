// Package transportfulfillment 是 node-operations 消费 transport-fulfillment 权威运输交接结果的适配器（ADR-0025 消费方侧）。它按交接
// 信封的键读回那一版交接、译成本上下文的语言，再交给节点控制的推进编排（ADR-0214，UC-NO-005）。它只翻译不判断：交接对象怎样对上
// 作业实物、转出哪一份控制、在哪个节点立控制，都是编排与领域的事。
package transportfulfillment

import (
	"context"
	"errors"
	"fmt"
	"time"

	noinbox "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/inbox"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/application"
	nodomain "go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
	tfdomain "go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	tfports "go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

var (
	// ErrUntranslatableAnswer 语义与其他消费方适配器的同名哨兵一致：引用或取值译不成本上下文的值，不是等谁。
	ErrUntranslatableAnswer = errors.New("node operations transportfulfillment adapter: untranslatable answer")
	// ErrHandoverRecordInconsistent 说明按键读回的登记指着另一个键，或业务时间为零：提供方的仓储不变量已破（ADR-0029），不是等谁。
	ErrHandoverRecordInconsistent = errors.New("node operations transportfulfillment adapter: transport handover disagrees with its key")
	// ErrControlTransferUndecided 说明这一拍停在交接登记册上（读不动或还读不回）。重投可能就过；生产装配把它登进
	// WithUndecidedSentinels，停在哪一口留在错误正文里。
	ErrControlTransferUndecided = errors.New("node operations transportfulfillment adapter: control transfer on transport handover is undecided")
	// ErrControlTransferDefect 说明编排答出封闭集合以外的结果，或信封上的键过不了本上下文的构造门。重投不会变，禁止进
	// WithUndecidedSentinels（ADR-0029）：让它落发布失败，响给人看。
	ErrControlTransferDefect = errors.New("node operations transportfulfillment adapter: control transfer on transport handover hit a defect")
)

// TransportHandoverReader 是 TF 交接登记册按键取一版、按前版找后继的那两问，由 TF 的 TransportHandoverRegistry 满足。只读：本适配器
// 不写 TF 的库。
type TransportHandoverReader interface {
	FindByKey(ctx context.Context, key tfports.TransportHandoverKey) (tfports.TransportHandoverRecord, bool, error)
	FindSuccessor(ctx context.Context, predecessor tfports.TransportHandoverKey) (tfports.TransportHandoverRecord, bool, error)
}

// AuthoritativeHandoverSource 实现 ports.AuthoritativeHandoverSource：按键读回一版交接，译成本上下文的 AuthoritativeHandover。
type AuthoritativeHandoverSource struct {
	handovers TransportHandoverReader
}

var _ ports.AuthoritativeHandoverSource = (*AuthoritativeHandoverSource)(nil)

func NewAuthoritativeHandoverSource(handovers TransportHandoverReader) (*AuthoritativeHandoverSource, error) {
	if handovers == nil {
		return nil, errors.New("node operations transportfulfillment adapter: handover reader is nil")
	}
	return &AuthoritativeHandoverSource{handovers: handovers}, nil
}

// FindAuthoritativeHandover 按（租户，对象，范围，版本）取回那一版，再问它有没有被同链后一版更正。
//
// 必须带版本读：按三维读「当前版」会把更正与原判断叠成一次查找，晚到的前版就会被当成后一版处理。转出依据只取 TF 给的
// TransferOutBasis——那一格只有`已交接`给得出，拒收与待确认在源头就没有转出引用。对象只当不透明的引用传下去，它是包裹还是集运单元
// 由本上下文自己的册子认，不在这里拿字符串去比作业实物。业务时间折到微秒：转移册把它落在 timestamptz 上，转出前与控制成立时刻比较
// 的、与读回时折叠重放比较的必须是同一个时刻。交出与接收节点照 TF 的声明原样译过来，没声明的格留零值；TF 存的是 network-routing
// 的节点代码，与本上下文收寄所在的节点是同一套指称，所以拿来比、拿来立控制都不另换算。
func (source *AuthoritativeHandoverSource) FindAuthoritativeHandover(
	ctx context.Context,
	tenant nodomain.TenantID,
	reference nodomain.AuthoritativeHandoverReference,
) (ports.AuthoritativeHandover, bool, error) {
	key, err := handoverKeyFor(tenant, reference)
	if err != nil {
		return ports.AuthoritativeHandover{}, false, err
	}
	record, found, err := source.handovers.FindByKey(ctx, key)
	if err != nil {
		return ports.AuthoritativeHandover{}, false, fmt.Errorf("%w: %v", ports.ErrAuthoritativeHandoverUnreadable, err)
	}
	if !found {
		return ports.AuthoritativeHandover{}, false, nil
	}
	handover := record.Handover
	if record.Key != key || handover.TenantID() != key.TenantID || handover.Object() != key.Object ||
		handover.Scope() != key.Scope || handover.Version() != key.Version || handover.JudgedAt().IsZero() {
		return ports.AuthoritativeHandover{}, false, fmt.Errorf("%w: object %q scope %q version %q",
			ErrHandoverRecordInconsistent, reference.Object(), reference.Scope(), reference.Version())
	}
	_, superseded, err := source.handovers.FindSuccessor(ctx, key)
	if err != nil {
		return ports.AuthoritativeHandover{}, false, fmt.Errorf("%w: successor: %v", ports.ErrAuthoritativeHandoverUnreadable, err)
	}
	answer := ports.AuthoritativeHandover{
		Reference:  reference,
		JudgedAt:   handover.JudgedAt().UTC().Truncate(time.Microsecond),
		Superseded: superseded,
	}
	if basis, transfers := handover.TransferOutBasis(); transfers {
		if answer.TransferOut, err = nodomain.NewTransferOutReference(basis); err != nil {
			return ports.AuthoritativeHandover{}, false, fmt.Errorf("%w: transfer out basis: %v", ErrUntranslatableAnswer, err)
		}
		answer.HandedOver = true
	}
	if node, declared := handover.ReleasingNode(); declared {
		if answer.ReleasingNode, err = nodomain.NewNodeReference(node.String()); err != nil {
			return ports.AuthoritativeHandover{}, false, fmt.Errorf("%w: releasing node: %v", ErrUntranslatableAnswer, err)
		}
	}
	if node, declared := handover.ReceivingNode(); declared {
		if answer.ReceivingNode, err = nodomain.NewNodeReference(node.String()); err != nil {
			return ports.AuthoritativeHandover{}, false, fmt.Errorf("%w: receiving node: %v", ErrUntranslatableAnswer, err)
		}
	}
	return answer, true, nil
}

func handoverKeyFor(tenant nodomain.TenantID, reference nodomain.AuthoritativeHandoverReference) (tfports.TransportHandoverKey, error) {
	none := tfports.TransportHandoverKey{}
	tfTenant, err := tfdomain.NewTenantID(tenant.String())
	if err != nil {
		return none, fmt.Errorf("%w: tenant: %v", ErrUntranslatableAnswer, err)
	}
	object, err := tfdomain.NewCarriedObjectReference(reference.Object())
	if err != nil {
		return none, fmt.Errorf("%w: carried object: %v", ErrUntranslatableAnswer, err)
	}
	scope, err := tfdomain.NewHandoverScopeReference(reference.Scope())
	if err != nil {
		return none, fmt.Errorf("%w: handover scope: %v", ErrUntranslatableAnswer, err)
	}
	version, err := tfdomain.NewHandoverResultVersion(reference.Version())
	if err != nil {
		return none, fmt.Errorf("%w: handover version: %v", ErrUntranslatableAnswer, err)
	}
	return tfports.TransportHandoverKey{TenantID: tfTenant, Object: object, Scope: scope, Version: version}, nil
}

// ControlTransfer 是节点控制推进编排那一口（先转出、后转入），application.TransferControlOnHandoverHandler 满足它。
type ControlTransfer interface {
	Advance(
		ctx context.Context,
		command application.TransferControlOnHandoverCommand,
	) (application.AdvanceControlOnHandoverResult, error)
}

// TransferControlOnHandoverAdapter 是 noinbox.TransportHandoverRegisteredConsumer 的真实处理方：把信封的四维译成本上下文的命令交给
// 推进编排，再按编排答出的处置落成 inbox 的三种动作。
type TransferControlOnHandoverAdapter struct {
	transfer ControlTransfer
}

var _ noinbox.RegisteredTransportHandoverHandler = (*TransferControlOnHandoverAdapter)(nil)

func NewTransferControlOnHandoverAdapter(transfer ControlTransfer) (*TransferControlOnHandoverAdapter, error) {
	if transfer == nil {
		return nil, errors.New("node operations transportfulfillment adapter: control transfer orchestration is nil")
	}
	return &TransferControlOnHandoverAdapter{transfer: transfer}, nil
}

// HandleRegisteredTransportHandover 跑一版。两半都入账（推进了、入账不推进、零写入的几格）交回 nil；停在交接登记册上交回未决哨兵；
// 任一半落在封闭集合以外交回缺陷哨兵。编排自己的错误（转移册读写、实物锁）原样交回，落发布失败：那几格要运维看的是本上下文自己的
// 存储。
func (adapter *TransferControlOnHandoverAdapter) HandleRegisteredTransportHandover(
	ctx context.Context,
	registered noinbox.RegisteredTransportHandover,
) error {
	tenant, err := nodomain.NewTenantID(registered.TenantID)
	if err != nil {
		return fmt.Errorf("%w: tenant: %v", ErrControlTransferDefect, err)
	}
	reference, err := nodomain.NewAuthoritativeHandoverReference(registered.Object, registered.Scope, registered.Version)
	if err != nil {
		return fmt.Errorf("%w: handover key: %v", ErrControlTransferDefect, err)
	}
	result, err := adapter.transfer.Advance(ctx, application.TransferControlOnHandoverCommand{
		TenantID: tenant,
		Handover: reference,
	})
	if err != nil {
		return err
	}
	out, in := result.Out(), result.In()
	switch result.Disposition() {
	case application.ControlOnHandoverSettled:
		return nil
	case application.ControlOnHandoverRetry:
		return fmt.Errorf("%w: out %s %s, in %s %s", ErrControlTransferUndecided,
			out.Outcome(), out.UndecidedReason(), in.Outcome(), in.UndecidedReason())
	default:
		return fmt.Errorf("%w: out %s, in %s", ErrControlTransferDefect, out.Outcome(), in.Outcome())
	}
}
