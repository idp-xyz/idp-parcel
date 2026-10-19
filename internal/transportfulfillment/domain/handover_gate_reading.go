package domain

import (
	"errors"
	"fmt"
)

// 本文件是交出门禁读数（ADR-0213 决定四）：一版`已交接`落册时与它同一笔保存的「当时关务对这次交出怎么答」。读数不挡
// 交接——控制已经转出，挡下是交出之前那一问的事；读数只把登记那一刻问来的答复如实记下，阻断也照记（与 ADR-0205 决定一
// 同一纪律）。
//
// 五格与逐对象那一格是共用内核（gate_reading.go）。本文件自己定义的是：问的是哪一份交出意图（at 取交出节点、onward 取
// 去向节点，都是这一版交接自己的声明），以及本上下文这一侧没走到「问关务」时停在了哪。交接是对象级判断，一版恰一件
// 对象，所以读数恰是一个停点或一格答复，不需要再压。

var (
	ErrInvalidHandoverGateReading = errors.New("transport fulfillment: invalid handover gate reading")
	// ErrHandoverGateReadingMisplaced：读数不属于这一版交接——不是`已交接`却带了读数，`已交接`要问关务却没带，或读数
	// 问的对象、意图与这一版的声明对不上。这是编排接错，不是登记方的输入有误；答「未受理」会让离线队列把一次已经发生的
	// 交接出队丢掉（ADR-0022），所以单独具名，编排据此上抛而不是答一格。
	ErrHandoverGateReadingMisplaced = errors.New("transport fulfillment: the handover gate reading does not belong to this handover")
)

// HandoverGateStop 是本上下文这一侧没走到「问关务」那一步的原因。每种只属于一格（Answer）：答复由停点推出，不另存一份
// 可与停点对不上的结论。
type HandoverGateStop uint8

const (
	HandoverGateStopInvalid HandoverGateStop = iota

	// 未决：缺什么
	//
	// HandoverReleasingNodeNotDeclared：说不出在哪个节点交出，就没有交出意图可问（ADR-0213 决定四）。不从交出方、
	// 接收方或范围补一个节点——那是推导（ADR-0096）。
	HandoverReleasingNodeNotDeclared
	// HandoverRecordedBeforeGateReading 只出现在迁移前就已落册的`已交接`上（TF 迁移 0041 的存量回填）：那时没问过
	// 关务，如实答未决而不是猜一格。新登记的交接不会停在这里，构造门拒它。
	HandoverRecordedBeforeGateReading

	// 读不动：哪一口
	//
	// HandoverGateRegistryUnavailable 只出现在更正上：更正成`已交接`的那一版要照前版的节点与业务时间问，写事务之前读
	// 前版时交接登记册读不动，就不知道该问哪一份意图。读不动不挡更正落册，与出发那一侧段册、班次册读不动同一纪律。
	HandoverGateRegistryUnavailable
)

func (stop HandoverGateStop) valid() bool {
	return stop >= HandoverReleasingNodeNotDeclared && stop <= HandoverGateRegistryUnavailable
}

// Answer 是这种停点所属的那一格。
func (stop HandoverGateStop) Answer() GateAnswer {
	switch stop {
	case HandoverReleasingNodeNotDeclared, HandoverRecordedBeforeGateReading:
		return GateUndecided
	case HandoverGateRegistryUnavailable:
		return GateUnreadable
	default:
		return GateAnswerInvalid
	}
}

func (stop HandoverGateStop) String() string {
	switch stop {
	case HandoverReleasingNodeNotDeclared:
		return "RELEASING_NODE_NOT_DECLARED"
	case HandoverRecordedBeforeGateReading:
		return "RECORDED_BEFORE_GATE_READING"
	case HandoverGateRegistryUnavailable:
		return "HANDOVER_REGISTRY_UNAVAILABLE"
	default:
		return ""
	}
}

// HandoverGateReadingSpec 是一次交出读数的全部内容。停点与那一格答复二者恰有其一：停在本上下文这一侧就没问关务，
// 问了关务就没有停点。Node 与 Onward 是问关务时那一份交出意图——交出节点与去向节点，取自这一版交接的声明；没问关务
// 时两格皆空。去向可缺：没声明去向照问，跨关务区域那一道由关务按 ADR-0206 决定二不列。
type HandoverGateReadingSpec struct {
	Node    NetworkNodeReference
	Onward  NetworkNodeReference
	Stop    HandoverGateStop
	Finding GateFinding
}

// HandoverGateReading 是一版`已交接`的交出门禁读数。
type HandoverGateReading struct {
	answer  GateAnswer
	node    NetworkNodeReference
	onward  NetworkNodeReference
	stop    HandoverGateStop
	finding GateFinding
}

// FormHandoverGateReading 形成一次新读数。存量回填那一种停点只由重建门收：新登记的交接都读过，不能声称没读。
func FormHandoverGateReading(spec HandoverGateReadingSpec) (HandoverGateReading, error) {
	if spec.Stop == HandoverRecordedBeforeGateReading {
		return HandoverGateReading{}, ErrInvalidHandoverGateReading
	}
	return formHandoverGateReading(spec)
}

// RehydrateHandoverGateReading 从库面重建读数，并核对存下的那一格正是停点或那一格答复推出的那一格——整格另成一列是
// 为了按格查不必解 jsonb，代价是它可能与依据对不上；对不上的那一行不装回。
func RehydrateHandoverGateReading(spec HandoverGateReadingSpec, recorded GateAnswer) (HandoverGateReading, error) {
	reading, err := formHandoverGateReading(spec)
	if err != nil {
		return HandoverGateReading{}, err
	}
	if reading.answer != recorded {
		return HandoverGateReading{}, fmt.Errorf("%w: recorded %q, the stop or finding says %q",
			ErrInvalidHandoverGateReading, recorded, reading.answer)
	}
	return reading, nil
}

func formHandoverGateReading(spec HandoverGateReadingSpec) (HandoverGateReading, error) {
	stopped, asked := spec.Stop != HandoverGateStopInvalid, spec.Finding.formed()
	if stopped == asked {
		return HandoverGateReading{}, ErrInvalidHandoverGateReading
	}
	if stopped {
		if !spec.Stop.valid() || spec.Node.valid() || spec.Onward.valid() {
			// 没问关务就没有意图：停在这一侧的读数不声称问过哪一份。
			return HandoverGateReading{}, ErrInvalidHandoverGateReading
		}
		return HandoverGateReading{answer: spec.Stop.Answer(), stop: spec.Stop}, nil
	}
	if !spec.Node.valid() {
		// 问了关务就一定有交出节点：关务的交出意图 at 必填，没有它的答复不是这次交出问来的。去向与交出节点相同不在这里
		// 拒——那是登记方声明的两格，本上下文不核；关务那边立不起这份意图，答复记成读不动（译不回来），读数照样成形。
		return HandoverGateReading{}, ErrInvalidHandoverGateReading
	}
	return HandoverGateReading{
		answer:  spec.Finding.answer,
		node:    spec.Node,
		onward:  spec.Onward,
		finding: spec.Finding,
	}, nil
}

// formed 说这是不是一份读数；零值不是——`已交接`缺读数与「读数答了某一格」必须分得开。
func (reading HandoverGateReading) formed() bool {
	return reading.answer.valid()
}

func (reading HandoverGateReading) Answer() GateAnswer { return reading.answer }

// Node 交回问关务时的交出节点；停在本上下文这一侧时答 false。
func (reading HandoverGateReading) Node() (NetworkNodeReference, bool) {
	return reading.node, reading.node.valid()
}

// Onward 交回问关务时的去向节点；没声明去向或没问关务时答 false。
func (reading HandoverGateReading) Onward() (NetworkNodeReference, bool) {
	return reading.onward, reading.onward.valid()
}

// OnwardNotDeclared 只在问了关务而这一版没声明去向时为真。跨关务区域那一道因此不在答复里（ADR-0206 决定二），读数要把
// 这一点写明：读到「不受门禁」的人得知道那是没给去向时的答复，不是关务核过跨区（ADR-0213 越权风险点 7）。
func (reading HandoverGateReading) OnwardNotDeclared() bool {
	return reading.node.valid() && !reading.onward.valid()
}

// Stop 交回本上下文这一侧的停点；问了关务时答 false。
func (reading HandoverGateReading) Stop() (HandoverGateStop, bool) {
	return reading.stop, reading.stop.valid()
}

// Finding 交回关务对这件对象的那一格；停在本上下文这一侧时答 false。
func (reading HandoverGateReading) Finding() (GateFinding, bool) {
	return reading.finding, reading.finding.formed()
}

// GateReading 交回这一版登记时随记的交出门禁读数；不是`已交接`的版本没有——拒收与待确认不转出控制，没有交出可读。
func (handover TransportHandover) GateReading() (HandoverGateReading, bool) {
	return handover.gateReading, handover.gateReading.formed()
}

// UnaskedGateReading 交回不问关务就定得出的那一份读数：`已交接`而没声明交出节点，读数是未决「交出节点未声明」
// （ADR-0213 决定四）。别的情形答 false——不是`已交接`就没有读数，声明了交出节点就得问关务。
//
// 「没声明交出节点就不问」只在这里定义一处：写事务之前问读数的那一侧与登记编排的兜底都取它，两边给出的读数逐字相同。
func (handover TransportHandover) UnaskedGateReading() (HandoverGateReading, bool) {
	if !handover.TransfersControl() || handover.nodes.Releasing.valid() {
		return HandoverGateReading{}, false
	}
	return HandoverGateReading{answer: HandoverReleasingNodeNotDeclared.Answer(), stop: HandoverReleasingNodeNotDeclared}, true
}

// WithGateReading 交回带上读数的这一版。读数要属于这一版，否则是 ErrHandoverGateReadingMisplaced：
//   - 只有`已交接`带读数；
//   - 问了关务的，问的对象就是这一版的对象，交出节点与去向节点就是这一版的声明；
//   - 停在「交出节点未声明」的，这一版确实没声明交出节点；
//   - 停在「交接登记册读不动」的只能是更正版——首登问读数不读登记册；
//   - 存量回填那一种停点只由重建门收。
func (handover TransportHandover) WithGateReading(reading HandoverGateReading) (TransportHandover, error) {
	if err := reading.placedOn(handover, false); err != nil {
		return TransportHandover{}, err
	}
	handover.gateReading = reading
	return handover, nil
}

func (reading HandoverGateReading) placedOn(handover TransportHandover, rehydrating bool) error {
	if !handover.TransfersControl() || !reading.formed() {
		return ErrHandoverGateReadingMisplaced
	}
	switch reading.stop {
	case HandoverRecordedBeforeGateReading:
		// 回填的那一版可能是迁移 0039 之后、0041 之前登下的，声明了节点也照样没读过：不按节点核。
		if !rehydrating {
			return ErrHandoverGateReadingMisplaced
		}
		return nil
	case HandoverGateRegistryUnavailable:
		if !handover.corrects.valid() {
			return ErrHandoverGateReadingMisplaced
		}
		return nil
	case HandoverReleasingNodeNotDeclared:
		if handover.nodes.Releasing.valid() {
			return ErrHandoverGateReadingMisplaced
		}
		return nil
	}
	if reading.finding.object != handover.object ||
		reading.node != handover.nodes.Releasing ||
		reading.onward != handover.nodes.Onward {
		return ErrHandoverGateReadingMisplaced
	}
	return nil
}
