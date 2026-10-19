package domain

import (
	"strconv"
	"strings"
	"time"
)

// 权威交接作参与起点或终点时，参与上记下的依据（ADR-0221）。拼的（进段、结束、重派生）、拆的（EntryHandoverVersion）与认的
// （EnteredByHandover、EndedByHandover、重派生找被替代的那一条）都在本文件，拼写只定义在这一处。

// transportHandoverBasisHead 是旧拼法 `TRANSPORT-HANDOVER/<版本>` 的起头。node-operations 的转出引用至今用它
// （TransferOutBasis）；本上下文的参与依据在 ADR-0221 之前也用它，那些存量照读、不改写，所以认的那一侧要留着它。
const transportHandoverBasisHead = "TRANSPORT-HANDOVER/"

// transportHandoverKeyBasisRoot 起头新铸法的参与依据。不沿用旧起头再加形状标记：旧拼法的版本是任意非空串，凡以
// `TRANSPORT-HANDOVER/` 起头的串都可能是一条旧依据，新铸法只要还在那个起头之下，就有构造得出的旧串与新串逐字相同。
// 换一个不是它前缀延伸的起头，新旧两种拼法的串集合不相交，认的时候不必另设防线。
const transportHandoverKeyBasisRoot = "TRANSPORT-HANDOVER-KEY/"

// handoverKeyBasis 由交接键铸参与依据：对象、范围、版本三件各写成「字节长度 + 冒号 + 原串」，以斜杠相连。
//
// 交接的键是（租户，对象，范围，版本），版本只在（对象，范围）之内唯一——同一件包裹在两个范围下各登一版 `v1` 是合法的。
// 依据只带版本，就有两次交接拼出同一串，「这件事实结束了哪一条参与」就答不唯一（票 demo-chain-gaps/111）。租户不进串：
// 参与与它的依据都在租户之内。对象进串：段上的委托开始依据、派送任务引用都只抄这一串，不带对象就指不回交接。三件都是
// 只拒空白的不透明串，可以含斜杠，长度前缀才让不同的键铸不出同一串；不取摘要，依据出现在答复与查阅上，人要读得出它是
// 哪一次交接。铸法与 NO 转入依据（ADR-0214）、派送任务引用（票 tf-face-algebra/09）同一种长度前缀。
func handoverKeyBasis(object CarriedObjectReference, scope HandoverScopeReference, version HandoverResultVersion) (ParticipationBasisReference, error) {
	if !object.valid() || !scope.valid() || !version.valid() {
		return ParticipationBasisReference{}, ErrInvalidFulfillmentSegment
	}
	var basis strings.Builder
	basis.WriteString(transportHandoverKeyBasisRoot)
	for index, part := range []string{object.String(), scope.String(), version.String()} {
		if index > 0 {
			basis.WriteByte('/')
		}
		basis.WriteString(strconv.Itoa(len(part)))
		basis.WriteByte(':')
		basis.WriteString(part)
	}
	return NewParticipationBasisReference(basis.String())
}

// parseHandoverKeyBasis 把新铸法的依据读回对象、范围、版本三件；串不合新形状答 false。长度只认不带前导零的十进制：一个键
// 只有一种写法，读回才与铸法一一对应。
func parseHandoverKeyBasis(basis string) ([3]string, bool) {
	var parts [3]string
	rest, ok := strings.CutPrefix(basis, transportHandoverKeyBasisRoot)
	if !ok {
		return parts, false
	}
	for index := range parts {
		if index > 0 {
			if rest, ok = strings.CutPrefix(rest, "/"); !ok {
				return parts, false
			}
		}
		digits, after, found := strings.Cut(rest, ":")
		if !found || digits == "" || (len(digits) > 1 && digits[0] == '0') || strings.Trim(digits, "0123456789") != "" {
			return parts, false
		}
		length, err := strconv.Atoi(digits)
		if err != nil || length > len(after) {
			return parts, false
		}
		parts[index], rest = after[:length], after[length:]
	}
	return parts, rest == ""
}

// ControlFactMatch 说参与上记下的一条依据怎样对上一件控制事实。分两格是因为旧拼法的依据认不全：
// 逐字对上的一定是那一件；旧拼法只对上了版本与时刻，同一版本、同一时刻、不同范围的两次交接仍分不开。
type ControlFactMatch uint8

const (
	ControlFactUnmatched ControlFactMatch = iota
	// ControlFactMatchedExactly：依据逐字就是这件事实交出的那一串。
	ControlFactMatchedExactly
	// ControlFactMatchedByVersionAndTime：依据是 ADR-0221 之前的旧拼法，只带版本；版本相同、记下的时刻又正是这一版交接的
	// 业务时间（进段与结束都按它立时刻）才算对上。
	ControlFactMatchedByVersionAndTime
)

// basisNamesHandover 答参与上记下的一条依据（连同按它立下的时刻）是不是指这一版交接所在的（对象，范围）下的 version 那一版。
// version 不一定是 handover 自己的版本：重派生要认的是被更正的那一版，它与更正版同对象、同范围、同业务时间（更正不改这三件）。
func (handover TransportHandover) basisNamesHandover(
	recorded ParticipationBasisReference,
	recordedAt time.Time,
	version HandoverResultVersion,
) ControlFactMatch {
	if keyed, err := handoverKeyBasis(handover.object, handover.scope, version); err == nil && recorded == keyed {
		return ControlFactMatchedExactly
	}
	if version.valid() && recorded.String() == transportHandoverBasisHead+version.String() && sameOnRegistry(recordedAt, handover.judgedAt) {
		return ControlFactMatchedByVersionAndTime
	}
	return ControlFactUnmatched
}

// ParticipationBasis 交回这一版交接作参与起点或终点时记下的依据——只有`已交接`给得出：拒收与待确认不转出控制，立不起
// 也结束不了参与。
func (handover TransportHandover) ParticipationBasis() (ParticipationBasisReference, bool) {
	if !handover.TransfersControl() {
		return ParticipationBasisReference{}, false
	}
	basis, err := handoverKeyBasis(handover.object, handover.scope, handover.version)
	return basis, err == nil
}

// EntryHandoverVersion 交回这条参与凭`已交接`进段时那一版交接的版本：入场种类是权威交接，入场依据是新铸法或旧拼法之一。
// 别的入场种类答 false。新铸法里的对象不是这条参与的对象，也答 false——那不是它进段的那一次。旧拼法只有版本、没有范围，
// 按版本找回的交接还要经 EnteredByHandover 认一遍；新铸法按版本找回的各版里，EnteredByHandover 也只认得出范围相同的那一版。
func (participation FulfillmentParticipation) EntryHandoverVersion() (HandoverResultVersion, bool) {
	if participation.entryKind != EnteredByTransportHandover {
		return HandoverResultVersion{}, false
	}
	raw := participation.entryBasis.String()
	if parts, keyed := parseHandoverKeyBasis(raw); keyed {
		if parts[0] != participation.object.String() {
			return HandoverResultVersion{}, false
		}
		version, err := NewHandoverResultVersion(parts[2])
		return version, err == nil
	}
	legacy, cut := strings.CutPrefix(raw, transportHandoverBasisHead)
	if !cut {
		return HandoverResultVersion{}, false
	}
	version, err := NewHandoverResultVersion(legacy)
	return version, err == nil
}

// EnteredByHandover 说这条参与是不是凭这一版交接进的段：这一版是`已交接`、同一件对象，入场依据指的正是它（新铸法逐字，
// 旧拼法版本与起点都对上——JoinWithHandover 与替代版本都按交接的业务时间立起点）。
func (participation FulfillmentParticipation) EnteredByHandover(handover TransportHandover) bool {
	return handover.TransfersControl() &&
		participation.entryKind == EnteredByTransportHandover &&
		participation.object == handover.object &&
		handover.basisNamesHandover(participation.entryBasis, participation.enteredAt, handover.version) != ControlFactUnmatched
}

// EndedByHandover 说这条参与的终点是不是这一版`已交接`：终点种类是下一次交接、同一件对象，终点依据指的正是它（旧拼法
// 再要终点时刻对上——EndParticipationWithNextHandover 按交接的业务时间立终点，替代版本照抄前版的终点）。
func (participation FulfillmentParticipation) EndedByHandover(handover TransportHandover) ControlFactMatch {
	if !handover.TransfersControl() || participation.object != handover.object {
		return ControlFactUnmatched
	}
	kind, basis, endedAt, ended := participation.End()
	if !ended || kind != EndedByNextHandover {
		return ControlFactUnmatched
	}
	return handover.basisNamesHandover(basis, endedAt, handover.version)
}
