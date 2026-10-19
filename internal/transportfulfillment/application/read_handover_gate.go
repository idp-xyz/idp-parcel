package application

import (
	"context"
	"errors"
	"fmt"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// 关务那一问没有形成本上下文认得的答复时，那一件记读不动，依据种类与出发那一侧同一套词（read_departure_gate.go 头上的
// departureGateCustoms* 常量）：救法相同，读者不必为交出另学一套。

var errHandoverGateViewMisanswered = errors.New("transport fulfillment: the handover gate view answered for another object")

// HandoverGateReadDeps 是交出门禁读数要问的两口：本上下文的交接登记册（只为更正取前版），以及关务的动作准入判断。
type HandoverGateReadDeps struct {
	Handovers ports.HandoverGatePredecessorView
	Customs   ports.HandoverGateView
}

// HandoverGateReader 为一版`已交接`形成交出门禁读数，供 RegisterWithGateReading / CorrectWithGateReading 与交接同一笔落册
// （ADR-0213 决定四）。
//
// 它由组合根在写事务之前、用外层 ctx 调：关务的判断在它自己的事务里记下（ADR-0206 决定六），嵌进交接登记那一笔会随它
// 回滚；读在池上，不在写事务里，任何一口读不动都只是读数里的一格，挡不住交接落册。
type HandoverGateReader struct {
	deps HandoverGateReadDeps
}

// NewHandoverGateReader 构造期拒缺件：漏装的一口到第一次交出才暴露，会被记成一格读不动，把装配缺陷演成依赖故障。
func NewHandoverGateReader(deps HandoverGateReadDeps) (*HandoverGateReader, error) {
	if deps.Handovers == nil || deps.Customs == nil {
		return nil, errors.New("transport fulfillment: the handover gate reader needs the handover registry and the customs view")
	}
	return &HandoverGateReader{deps: deps}, nil
}

// ReadRegistration 为一条首登命令问读数。只为`已交接`问：拒收、待确认与受理不了的命令交回零值读数，由登记编排照常答
// （拒收与待确认不转出控制，没有交出可读；受理不了的答未受理）。
func (reader *HandoverGateReader) ReadRegistration(
	ctx context.Context,
	command RegisterTransportHandoverCommand,
) (domain.HandoverGateReading, error) {
	handover, err := handoverFrom(command)
	if err != nil || !handover.TransfersControl() {
		return domain.HandoverGateReading{}, nil
	}
	return reader.read(ctx, handover)
}

// ReadCorrection 为一条更正命令问读数。更正成`已交接`的那一版照前版承继节点与业务时间，所以先在池上读前版、照领域的
// Correct 立出那一版，再问；更正成拒收或待确认、立不起那一版、前版不在册，都交回零值读数，由更正编排照常答。
//
// 前版读不动时没法知道该问哪一份意图：读数停在「交接登记册读不动」，更正照登——与出发那一侧段册、班次册读不动同一纪律
// （ADR-0205 决定二）。写事务里若同一口仍读不动，更正编排答未决，这份读数用不上；写事务里读得动而那一版不必问关务时，
// 更正编排落不问就定得出的那一份，不落这一格。
func (reader *HandoverGateReader) ReadCorrection(
	ctx context.Context,
	command CorrectTransportHandoverCommand,
) (domain.HandoverGateReading, error) {
	if command.Verdict != domain.ObjectHandedOver {
		return domain.HandoverGateReading{}, nil
	}
	predecessorKey, err := correctionKey(command)
	if err != nil {
		return domain.HandoverGateReading{}, nil
	}
	correction, err := correctionFrom(command)
	if err != nil {
		return domain.HandoverGateReading{}, nil
	}
	existing, found, err := reader.deps.Handovers.FindByKey(ctx, predecessorKey)
	switch {
	case err != nil:
		return domain.FormHandoverGateReading(domain.HandoverGateReadingSpec{Stop: domain.HandoverGateRegistryUnavailable})
	case !found:
		return domain.HandoverGateReading{}, nil
	}
	corrected, err := existing.Handover.Correct(correction)
	if err != nil || !corrected.TransfersControl() {
		return domain.HandoverGateReading{}, nil
	}
	return reader.read(ctx, corrected)
}

// read 问一版`已交接`：没声明交出节点就停在本上下文这一侧、不问关务（不从交出方或范围补一个节点，ADR-0096）；声明了
// 就按交出意图问——at 取交出节点，onward 取去向节点，没声明去向照问，时点取交接的业务时间（离线补登的交接按它交出那一刻
// 判）。关务那一问没形成答复时那一件记读不动，交接照登。error 只留给读数自己拼不出来（实现坏了）。
func (reader *HandoverGateReader) read(
	ctx context.Context,
	handover domain.TransportHandover,
) (domain.HandoverGateReading, error) {
	if unasked, determined := handover.UnaskedGateReading(); determined {
		return unasked, nil
	}
	node, _ := handover.ReleasingNode()
	onward, _ := handover.OnwardNode()
	object := handover.Object()
	finding, err := reader.deps.Customs.ReadHandoverGate(ctx, ports.HandoverGateQuery{
		TenantID: handover.TenantID(),
		Object:   object,
		Node:     node,
		Onward:   onward,
		At:       handover.JudgedAt(),
	})
	if err != nil {
		cause := departureGateCustomsUnavailable
		if errors.Is(err, ports.ErrHandoverGateAnswerUntranslatable) {
			cause = departureGateCustomsUntranslatable
		}
		if finding, err = domain.NewGateFinding(object, domain.GateUnreadable, "",
			[]domain.GateBasis{{Cause: cause}}); err != nil {
			return domain.HandoverGateReading{}, err
		}
	}
	if finding.Object() != object {
		// 端口答的是另一件、或交回零值却不报错：那是实现坏了，不是关务读不动，记成一格只会把它藏起来。
		return domain.HandoverGateReading{}, fmt.Errorf("%w: the customs view answered for %q when asked about %q",
			errHandoverGateViewMisanswered, finding.Object(), object)
	}
	return domain.FormHandoverGateReading(domain.HandoverGateReadingSpec{Node: node, Onward: onward, Finding: finding})
}
