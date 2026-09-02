// Package outbound 拥有出向集成的结果代数与调用前置约束（ADR-0090）。
//
// 它被两条链共用：取面单在 `parcel-shipment`，轨迹源在 `transport-fulfillment`。落平台而
// 不落某一个上下文的 `adapters/` 是 ADR-0090 的要求——藏进一处实现，另一条链会重新发明
// 一遍，而分叉之后再统一要动两处已经接了真渠道的适配器。
//
// **本包不发起任何调用，也不认识 HTTP。** 它只定「一次出向调用可能落在哪几格」与「发起
// 之前必须齐备什么」。判定某次应答落在哪一格是各家适配器唯一不可替代的职责，理由见
// Disposition 的注释。
package outbound

// Disposition 是一次出向调用的处置格，按**调用方的恢复动作**分格（ADR-0090 决定一，分格
// 维度继承 ADR-0029）。
//
// 超时、连接被拒、TLS 握手失败、DNS 解不出、对端 5xx 都不在这里——那些是我方观察到的现象，
// 而现象与「对端受理了没有」不是一一对应：超时可能已受理，连接被拒通常未受理，对端 5xx
// 两种都可能。按现象分格，调用方会拿着一份精确的故障描述却仍然答不出该不该重发。要分辨
// 现象去看适配器自己的日志与指标，不从这里读。
//
// 失败三格之外另有两格，两格各有出处，都不是本包私自加的。Accepted 只作判别用，**答复的
// 内容仍由各端口自己的成功类型承载**，本包不认识它；NotConfigured 是 ADR-0090 决定五明写
// 要「自成一格作答」的未配置格。
//
// 把它们收进同一个封闭集合而不是另设一个布尔，是为了让矛盾状态**表示不出来**：分开写就有
// `已受理 == true` 与`对端拒绝`同时成立的那一格，而类型允许的状态迟早会有人写出来。这也是
// 「失败代数只有三格」与本集合有五格并不冲突的地方——失败的仍然只有三格。
type Disposition uint8

const (
	DispositionInvalid Disposition = iota
	Accepted
	Rejected
	AnswerUndetermined
	ProvenNotAccepted
	NotConfigured
)

func (disposition Disposition) String() string {
	switch disposition {
	case Accepted:
		return "ACCEPTED"
	case Rejected:
		return "REJECTED"
	case AnswerUndetermined:
		return "ANSWER_UNDETERMINED"
	case ProvenNotAccepted:
		return "PROVEN_NOT_ACCEPTED"
	case NotConfigured:
		return "NOT_CONFIGURED"
	default:
		return ""
	}
}

// AdmitsResend 说这一格准不准重发同一请求。只有 ProvenNotAccepted 准。
//
// 把 ADR-0090 决定二与决定六做进结构而不是留给调用方记住，是因为判错的两个方向代价完全
// 不对称：把 ProvenNotAccepted 误当 AnswerUndetermined 只是多走一次查询，反过来会触发重发，
// 而重发一次取面单产生的是真实的供应商成本与第二个运输标识。那是整条出向链上唯一有资金
// 后果的判错方向。
//
// NotConfigured 答 false 不是因为重发不安全，是因为重发解决不了它——配置没到位，再发一次
// 仍然发不出去，续办是去配渠道。
func (disposition Disposition) AdmitsResend() bool {
	return disposition == ProvenNotAccepted
}

// RequiresQueryToSettle 说这一格只能靠查询或对账收口。只有 AnswerUndetermined 是。
//
// ADR-0090 决定六据此要求每个出向端口都欠一个配套的查询能力：这一格若没有查询口就是死的，
// 聚合会积下永远停在`结果不确定`的交易而没有出路。它与 AdmitsResend 一起，把「不得重发、
// 只能查询」这条纪律的两半都变成可问的，而不是两句要人记住的话。
func (disposition Disposition) RequiresQueryToSettle() bool {
	return disposition == AnswerUndetermined
}
