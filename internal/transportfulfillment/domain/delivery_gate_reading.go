package domain

import (
	"errors"
	"fmt"
	"strings"
)

// 本文件是交付门禁读数（ADR-0213 决定五）：一份有效交付落册时与它同一笔保存的「当时关务对这次交付怎么答」。读数不挡
// 交付——对象已经交到收件方手里，挡下是派送出发之前那一问的事；读数只把登记那一刻问来的答复如实记下，阻断也照记（与
// ADR-0205 决定一同一纪律）。
//
// 五格与逐对象那一格是共用内核（gate_reading.go）。本文件自己定义的是：问的是哪一份交付意图（at 取对象进入这次交付所在
// 那一段时那次交接的交出节点），以及本上下文这一侧没走到「问关务」时停在了哪。有效交付是对象级事实，一份恰一件对象，
// 所以读数恰是一个停点或一格答复，不需要再压。

var (
	ErrInvalidDeliveryGateReading = errors.New("transport fulfillment: invalid delivery gate reading")
	// ErrDeliveryGateReadingMisplaced：读数不属于这一份交付——新形成的交付没带读数，或读数问的是另一件对象。这是编排
	// 接错，不是登记方的输入有误；答「未受理」会让离线队列把一次已经发生的交付出队丢掉（ADR-0022），所以单独具名，编排
	// 据此上抛而不是答一格。
	ErrDeliveryGateReadingMisplaced = errors.New("transport fulfillment: the delivery gate reading does not belong to this effective delivery")
)

// DeliveryGateStop 是本上下文这一侧没走到「问关务」那一步的原因。每种只属于一格（Answer）：答复由停点推出，不另存一份
// 可与停点对不上的结论。
//
// 未决停点里除存量回填之外，都是 ADR-0213 决定五说的「交付节点不可得」，按缺在哪一环分开具名：续办动作两两不同——去查
// 对象为什么不在段里、承认这一程本就没有交出节点、去补交接上的声明、去查交接册与段册为什么对不上、去问登记方哪一版才是。
type DeliveryGateStop uint8

const (
	DeliveryGateStopInvalid DeliveryGateStop = iota

	// 未决：缺什么
	//
	// DeliveryNoActiveSegment：对象此刻不在任何段里在场。交付照登，结束参与那一半会答 NO_ACTIVE_PARTICIPATION；没有这次
	// 交付所在的段，也就没有进段的那次交接可取节点。
	DeliveryNoActiveSegment
	// DeliveryEnteredByPickup：对象凭收寄（场外揽收或实际承运商首次有效收寄）进的这一段，进段那一刻没有交出节点可取
	// （ADR-0213 决定五）。不拿收寄地点或段别的成员顶一个节点——那是推导（ADR-0096）。
	DeliveryEnteredByPickup
	// DeliveryReleasingNodeNotDeclared：对象凭`已交接`进的这一段，那次交接没声明交出节点（ADR-0213 决定五）。不从交出方、
	// 接收方或范围补一个节点。
	DeliveryReleasingNodeNotDeclared
	// DeliveryEntryHandoverNotOnRegister：参与说凭`已交接`进段，入场依据却指不到交接登记册上的一版`已交接`——依据拆不出
	// 版本，或那一版不在册、在册的业务时间对不上。段上的参与与那次交接由交接登记同一笔写下，正常对得上，这一格说的是库面
	// 不一致，重读也不会变；与下一格分开具名，查数据的人与读数派生的信号才分得出「数据坏了」与「登记方的版本口径撞了」。
	// 仍只记未决、不挡交付：交付已经发生，挡下它修不好段册。
	DeliveryEntryHandoverNotOnRegister
	// DeliveryEntryHandoverAmbiguous：入场依据指名的那一版在册上对上不止一版`已交接`——同一对象在不同交接范围用了同一个
	// 版本号、业务时间也相同（入场依据只带版本、不带范围）。不猜其中一版。
	DeliveryEntryHandoverAmbiguous
	// DeliveryRecordedBeforeGateReading 只出现在迁移前就已落册的有效交付上（TF 迁移 0043 的存量回填）：那时没问过关务，
	// 如实答未决而不是猜一格。新登记的交付不会停在这里，构造门拒它。
	DeliveryRecordedBeforeGateReading

	// 读不动：哪一口
	//
	// DeliveryAttemptViewUnavailable：写事务之前读派送尝试时读不动，不知道交付的业务时间，也就不知道该按哪一刻问关务。
	// 写事务里若仍读不动，登记编排答未决、这份读数用不上；写事务里读得动，交付照登、记这一格。
	DeliveryAttemptViewUnavailable
	DeliverySegmentRegistryUnavailable
	DeliveryHandoverRegistryUnavailable
)

func (stop DeliveryGateStop) valid() bool {
	return stop >= DeliveryNoActiveSegment && stop <= DeliveryHandoverRegistryUnavailable
}

// Answer 是这种停点所属的那一格。
func (stop DeliveryGateStop) Answer() GateAnswer {
	switch stop {
	case DeliveryNoActiveSegment, DeliveryEnteredByPickup, DeliveryReleasingNodeNotDeclared,
		DeliveryEntryHandoverNotOnRegister, DeliveryEntryHandoverAmbiguous, DeliveryRecordedBeforeGateReading:
		return GateUndecided
	case DeliveryAttemptViewUnavailable, DeliverySegmentRegistryUnavailable, DeliveryHandoverRegistryUnavailable:
		return GateUnreadable
	default:
		return GateAnswerInvalid
	}
}

func (stop DeliveryGateStop) String() string {
	switch stop {
	case DeliveryNoActiveSegment:
		return "NO_ACTIVE_SEGMENT"
	case DeliveryEnteredByPickup:
		return "ENTERED_BY_PICKUP"
	case DeliveryReleasingNodeNotDeclared:
		return "RELEASING_NODE_NOT_DECLARED"
	case DeliveryEntryHandoverNotOnRegister:
		return "ENTRY_HANDOVER_NOT_ON_REGISTER"
	case DeliveryEntryHandoverAmbiguous:
		return "ENTRY_HANDOVER_AMBIGUOUS"
	case DeliveryRecordedBeforeGateReading:
		return "RECORDED_BEFORE_GATE_READING"
	case DeliveryAttemptViewUnavailable:
		return "DELIVERY_VIEW_UNAVAILABLE"
	case DeliverySegmentRegistryUnavailable:
		return "SEGMENT_REGISTRY_UNAVAILABLE"
	case DeliveryHandoverRegistryUnavailable:
		return "HANDOVER_REGISTRY_UNAVAILABLE"
	default:
		return ""
	}
}

// DeliveryGateReadingSpec 是一次交付读数的全部内容。停点与那一格答复二者恰有其一：停在本上下文这一侧就没问关务，问了
// 关务就没有停点。Node 是问关务时那一份交付意图的 at；没问关务时为空。
type DeliveryGateReadingSpec struct {
	Node    NetworkNodeReference
	Stop    DeliveryGateStop
	Finding GateFinding
}

// DeliveryGateReading 是一份有效交付的交付门禁读数。
type DeliveryGateReading struct {
	answer  GateAnswer
	node    NetworkNodeReference
	stop    DeliveryGateStop
	finding GateFinding
}

// FormDeliveryGateReading 形成一次新读数。存量回填那一种停点只由重建门收：新登记的交付都读过，不能声称没读。
func FormDeliveryGateReading(spec DeliveryGateReadingSpec) (DeliveryGateReading, error) {
	if spec.Stop == DeliveryRecordedBeforeGateReading {
		return DeliveryGateReading{}, ErrInvalidDeliveryGateReading
	}
	return formDeliveryGateReading(spec)
}

// RehydrateDeliveryGateReading 从库面重建读数，并核对存下的那一格正是停点或那一格答复推出的那一格，理由同
// RehydrateHandoverGateReading。
func RehydrateDeliveryGateReading(spec DeliveryGateReadingSpec, recorded GateAnswer) (DeliveryGateReading, error) {
	reading, err := formDeliveryGateReading(spec)
	if err != nil {
		return DeliveryGateReading{}, err
	}
	if reading.answer != recorded {
		return DeliveryGateReading{}, fmt.Errorf("%w: recorded %q, the stop or finding says %q",
			ErrInvalidDeliveryGateReading, recorded, reading.answer)
	}
	return reading, nil
}

func formDeliveryGateReading(spec DeliveryGateReadingSpec) (DeliveryGateReading, error) {
	stopped, asked := spec.Stop != DeliveryGateStopInvalid, spec.Finding.formed()
	if stopped == asked {
		return DeliveryGateReading{}, ErrInvalidDeliveryGateReading
	}
	if stopped {
		if !spec.Stop.valid() || spec.Node.valid() {
			// 没问关务就没有意图：停在这一侧的读数不声称问过哪一份。
			return DeliveryGateReading{}, ErrInvalidDeliveryGateReading
		}
		return DeliveryGateReading{answer: spec.Stop.Answer(), stop: spec.Stop}, nil
	}
	if !spec.Node.valid() {
		// 问了关务就一定有 at：关务的交付意图 at 必填，没有它的答复不是这次交付问来的。
		return DeliveryGateReading{}, ErrInvalidDeliveryGateReading
	}
	return DeliveryGateReading{answer: spec.Finding.answer, node: spec.Node, finding: spec.Finding}, nil
}

// formed 说这是不是一份读数；零值不是——交付缺读数与「读数答了某一格」必须分得开。
func (reading DeliveryGateReading) formed() bool {
	return reading.answer.valid()
}

func (reading DeliveryGateReading) Answer() GateAnswer { return reading.answer }

// Node 交回问关务时交付意图的 at；停在本上下文这一侧时答 false。
func (reading DeliveryGateReading) Node() (NetworkNodeReference, bool) {
	return reading.node, reading.node.valid()
}

// Stop 交回本上下文这一侧的停点；问了关务时答 false。
func (reading DeliveryGateReading) Stop() (DeliveryGateStop, bool) {
	return reading.stop, reading.stop.valid()
}

// Finding 交回关务对这件对象的那一格；停在本上下文这一侧时答 false。
func (reading DeliveryGateReading) Finding() (GateFinding, bool) {
	return reading.finding, reading.finding.formed()
}

// GateReading 交回这份交付登记时随记的交付门禁读数。在册的每一版都有：首登时问来，POD 更正照前版承继（CorrectProof 值
// 拷贝）——更正改的是证据，不是在哪、何时、交了哪件，问关务的那一份意图没有变。
func (delivery EffectiveDelivery) GateReading() (DeliveryGateReading, bool) {
	return delivery.gateReading, delivery.gateReading.formed()
}

// WithGateReading 交回带上读数的这一份。读数要属于这一份，否则是 ErrDeliveryGateReadingMisplaced：问了关务的，问的
// 对象就是这一份的对象；存量回填那一种停点只由重建门收。读数的节点取自段册与交接册，不在这一份交付上，这里核不到——
// 由写事务之前的读数那一侧一处定（application.DeliveryGateReader）。
func (delivery EffectiveDelivery) WithGateReading(reading DeliveryGateReading) (EffectiveDelivery, error) {
	if err := reading.placedOn(delivery, false); err != nil {
		return EffectiveDelivery{}, err
	}
	delivery.gateReading = reading
	return delivery, nil
}

func (reading DeliveryGateReading) placedOn(delivery EffectiveDelivery, rehydrating bool) error {
	if !reading.formed() {
		return ErrDeliveryGateReadingMisplaced
	}
	if reading.stop == DeliveryRecordedBeforeGateReading && !rehydrating {
		return ErrDeliveryGateReadingMisplaced
	}
	if finding, asked := reading.Finding(); asked && finding.object != delivery.object {
		return ErrDeliveryGateReadingMisplaced
	}
	return nil
}

// EntryHandoverVersion 交回这条参与凭`已交接`进段时那一版交接的版本：入场种类是权威交接、入场依据是那一截拼写加版本。
// 别的入场种类答 false。依据里只有版本、没有交接范围——按版本找回的交接还要经 EnteredByHandover 认一遍。
func (participation FulfillmentParticipation) EntryHandoverVersion() (HandoverResultVersion, bool) {
	if participation.entryKind != EnteredByTransportHandover {
		return HandoverResultVersion{}, false
	}
	raw, cut := strings.CutPrefix(participation.entryBasis.String(), transportHandoverBasisHead)
	if !cut {
		return HandoverResultVersion{}, false
	}
	version, err := NewHandoverResultVersion(raw)
	return version, err == nil
}

// EnteredByHandover 说这条参与是不是凭这一版交接进的段：这一版是`已交接`、同一件对象，参与的入场依据正是它的转出依据、
// 起点正是它的业务时间（JoinWithHandover 与替代版本都按它立起点）。入场依据只带版本，同一对象在两个交接范围用了同一
// 版本号时，业务时间再分一次。
func (participation FulfillmentParticipation) EnteredByHandover(handover TransportHandover) bool {
	reference, transfers := handover.TransferOutBasis()
	return transfers &&
		participation.entryKind == EnteredByTransportHandover &&
		participation.object == handover.object &&
		participation.entryBasis.String() == reference &&
		sameOnRegistry(participation.enteredAt, handover.judgedAt)
}
