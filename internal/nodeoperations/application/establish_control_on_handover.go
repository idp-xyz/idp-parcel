package application

import (
	"context"
	"errors"
	"fmt"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
)

// AdvanceControlOnHandoverResult 是一版权威运输交接在两个方向上的结果：交出节点那一侧的转出与接收节点那一侧的转入（UC-NO-005）。
// 交接登记册读不动或读不回时两半都没跑，两格同答那一格未决。
type AdvanceControlOnHandoverResult struct {
	out TransferControlOnHandoverResult
	in  TransferControlOnHandoverResult
}

func (result AdvanceControlOnHandoverResult) Out() TransferControlOnHandoverResult {
	return result.out
}

func (result AdvanceControlOnHandoverResult) In() TransferControlOnHandoverResult {
	return result.in
}

// Disposition 把两半折成对信封的一个处置：两半同在一笔事务里，信封只能整封入账或整封重投。任一半落在封闭集合以外就是缺陷，
// 任一半要等交接登记册就重投——重投时已落的那一半在同一笔里一起回滚，重做时照样答同一格。
func (result AdvanceControlOnHandoverResult) Disposition() ControlOnHandoverDisposition {
	out, in := result.out.Disposition(), result.in.Disposition()
	switch {
	case out == ControlOnHandoverDefect || in == ControlOnHandoverDefect:
		return ControlOnHandoverDefect
	case out == ControlOnHandoverRetry || in == ControlOnHandoverRetry:
		return ControlOnHandoverRetry
	default:
		return ControlOnHandoverSettled
	}
}

// Advance 跑一版交接：读回一次，先转出、后转入（ADR-0214 决定四、七）。
//
// 先转出是因为节点之间直接交接时同一版既声明交出节点又声明接收节点：交出节点那一份控制先结束，接收节点才立得起控制，否则转入会读到
// 它仍在身而停在「仍有在身控制」。两半同在消费门那一笔事务里，实物锁可重入，转入读得到转出刚落的那一行。
func (handler *TransferControlOnHandoverHandler) Advance(
	ctx context.Context,
	command TransferControlOnHandoverCommand,
) (AdvanceControlOnHandoverResult, error) {
	tenant, reference := command.TenantID, command.Handover
	handover, found, err := handler.deps.Handovers.FindAuthoritativeHandover(ctx, tenant, reference)
	if errors.Is(err, ports.ErrAuthoritativeHandoverUnreadable) {
		undecided := undecidedControlOnHandover(ControlOnHandoverHandoverUnreadable)
		return AdvanceControlOnHandoverResult{out: undecided, in: undecided}, nil
	}
	if err != nil {
		return AdvanceControlOnHandoverResult{}, err
	}
	if !found {
		undecided := undecidedControlOnHandover(ControlOnHandoverHandoverNotVisible)
		return AdvanceControlOnHandoverResult{out: undecided, in: undecided}, nil
	}
	if handover.Reference != reference {
		return AdvanceControlOnHandoverResult{}, fmt.Errorf(
			"node operations: handover source answered version %q for %q", handover.Reference.Version(), reference.Version())
	}
	out, err := handler.transferOut(ctx, tenant, reference, handover)
	if err != nil {
		return AdvanceControlOnHandoverResult{}, err
	}
	in, err := handler.establishIn(ctx, tenant, reference, handover)
	if err != nil {
		return AdvanceControlOnHandoverResult{}, err
	}
	return AdvanceControlOnHandoverResult{out: out, in: in}, nil
}

// establishIn 是转入一半（ADR-0214 决定七）。次序照转出一半，转入一侧自己的讲究是：
//
//   - 没声明接收节点最先答：没有节点可以转入，零写入。更正承继前版的节点声明，所以这条更正链在转入方向上从没推进过，后面几步都
//     不必再问。
//   - 撤回控制的更正带的是先前立起的那一份控制；立起的控制不撤，入账待重算。
//   - 对上一件实物之后读它有过的各份控制（在身的与已转出的），按领域判：没有在身的、最近一次转出不晚于这一版才立。
//
// 对不上作业实物就入账，不造待识别实物：待识别实物只能由现场观察建立。
func (handler *TransferControlOnHandoverHandler) establishIn(
	ctx context.Context,
	tenant domain.TenantID,
	reference domain.AuthoritativeHandoverReference,
	handover ports.AuthoritativeHandover,
) (TransferControlOnHandoverResult, error) {
	if handover.ReceivingNode == (domain.NodeReference{}) {
		return TransferControlOnHandoverResult{outcome: HandoverDeclaresNoReceivingNode}, nil
	}
	existing, recorded, err := handler.deps.Controls.FindTransition(ctx, tenant, reference, domain.ControlTransitionIn)
	if err != nil {
		return TransferControlOnHandoverResult{}, fmt.Errorf("find the control transition in of this handover: %w", err)
	}
	if recorded {
		return TransferControlOnHandoverResult{outcome: ControlTransitionAlreadyRecorded, transition: existing}, nil
	}
	if handover.Superseded {
		return TransferControlOnHandoverResult{outcome: HandoverVersionSuperseded}, nil
	}

	earlier, chained, err := handler.deps.Controls.FindAdvancedInHandoverChain(ctx, tenant, reference, domain.ControlTransitionIn)
	if err != nil {
		return TransferControlOnHandoverResult{}, fmt.Errorf("find an earlier transfer in in the handover chain: %w", err)
	}
	if chained && earlier.Transition.Handover() == reference {
		return TransferControlOnHandoverResult{outcome: ControlTransitionAlreadyRecorded, transition: earlier}, nil
	}
	if !handover.HandedOver {
		if !chained {
			return TransferControlOnHandoverResult{outcome: HandoverDoesNotTransferControl}, nil
		}
		pending, err := domain.PendingControlSourceRecompute(earlier.Transition, reference, handover.JudgedAt)
		if err != nil {
			return TransferControlOnHandoverResult{}, fmt.Errorf("record the control source chain pending recompute in: %w", err)
		}
		return handler.record(ctx, pending)
	}
	if chained {
		return TransferControlOnHandoverResult{outcome: ControlAlreadyEstablishedInChain}, nil
	}

	unit, unresolved, err := handler.resolve(ctx, tenant, reference)
	if err != nil {
		return TransferControlOnHandoverResult{}, err
	}
	if unresolved != domain.ControlNotAdvancedReasonAbsent {
		transition, err := domain.ObjectNotResolved(tenant, reference, domain.ControlTransitionIn, unresolved, handover.JudgedAt)
		if err != nil {
			return TransferControlOnHandoverResult{}, fmt.Errorf("record the unresolved handover object in: %w", err)
		}
		return handler.record(ctx, transition)
	}

	known, err := handler.deps.Controls.FindUnitControls(ctx, tenant, unit)
	if err != nil {
		return TransferControlOnHandoverResult{}, fmt.Errorf("find the controls of the handed in unit: %w", err)
	}
	if earlier, chained, err := handler.deps.Controls.FindAdvancedInHandoverChain(
		ctx, tenant, reference, domain.ControlTransitionIn); err != nil {
		return TransferControlOnHandoverResult{}, fmt.Errorf("find an earlier transfer in in the handover chain under the unit lock: %w", err)
	} else if chained && earlier.Transition.Handover() == reference {
		return TransferControlOnHandoverResult{outcome: ControlTransitionAlreadyRecorded, transition: earlier}, nil
	} else if chained {
		return TransferControlOnHandoverResult{outcome: ControlAlreadyEstablishedInChain}, nil
	}
	transition, err := domain.JudgeTransferIn(tenant, unit, known, reference, handover.ReceivingNode, handover.JudgedAt)
	if err != nil {
		return TransferControlOnHandoverResult{}, fmt.Errorf("judge the transfer in: %w", err)
	}
	return handler.record(ctx, transition)
}
