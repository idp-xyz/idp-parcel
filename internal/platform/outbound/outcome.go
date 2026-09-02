package outbound

// Outcome 是适配器交给编排的一次出向答复。
//
// 它只经本包的构造器产生，字段不导出：分格规则里有一条**需要举证**（`确证未受理`），而举证
// 这件事留一个可自由填的结构体就等于交给每家适配器各自记得遵守。适配器要逐家写，规则却只有
// 一条，强制点因此该在这里而不是在每一家的评审里。
//
// 零值是 DispositionInvalid，它 fail-closed：既不准重发也不要求查询。一个没被赋过值的
// Outcome 因此不会看起来像一个正经答复。
type Outcome struct {
	disposition Disposition
	evidence    string
}

// Accept 交出`对端已答复且答复是接受`。答复的内容不在这里——它由各端口自己的成功类型承载
// （ADR-0090 决定一）。
func Accept() Outcome {
	return Outcome{disposition: Accepted}
}

// Reject 交出`对端已答复且答复是拒绝`。
//
// 它不要求举证，与 NotAccepted 不同：这一格的前提是对端**确实答了**，答复本身就是实据。
// reason 是渠道给出的拒绝原因引用，可以为空——有渠道只回一个拒绝而不给原因，硬要求它会逼
// 适配器编一个出来。
func Reject(reason string) Outcome {
	return Outcome{disposition: Rejected, evidence: reason}
}

// Undetermined 交出`答案未确定`。它是默认格：说不出对端受理了没有时一律落这里，超时没有
// 例外（ADR-0090 决定二）。
func Undetermined() Outcome {
	return Outcome{disposition: AnswerUndetermined}
}

// NotAccepted 交出`确证未受理`，且**只在举得出实据时才交得出**。
//
// evidence 是「请求未被对端受理」的实据引用——例如连接根本没有建立，或对端在协议层明确回绝
// 且该回绝不可能发生在受理之后。举不出时降级为`答案未确定`，方向是 ADR-0090 决定二刻意选的：
// 判错的两个方向代价不对称，把`答案未确定`误报成`确证未受理`会触发重发，而重发一次取面单
// 调用产生的是真实的供应商成本与第二个运输标识。
//
// 降级而不是报错，是因为报错会让适配器在「举不出实据」时无路可走，多半就随手编一个实据
// 字符串填上——那样这道门就成了摆设。
func NotAccepted(evidence string) Outcome {
	if evidence == "" {
		return Outcome{disposition: AnswerUndetermined}
	}
	return Outcome{disposition: ProvenNotAccepted, evidence: evidence}
}

// Unconfigured 交出`出向未配置`，表示调用**没有发生过**。它与「发起了但失败了」是两件事，
// 后者要落失败代数的某一格。
func Unconfigured() Outcome {
	return Outcome{disposition: NotConfigured}
}

func (outcome Outcome) Disposition() Disposition {
	return outcome.disposition
}

// Evidence 是这一格的依据引用：`确证未受理`的实据，或`对端已答复且答复是拒绝`的原因。
// 其余各格为空。
func (outcome Outcome) Evidence() string {
	return outcome.evidence
}

func (outcome Outcome) AdmitsResend() bool {
	return outcome.disposition.AdmitsResend()
}

func (outcome Outcome) RequiresQueryToSettle() bool {
	return outcome.disposition.RequiresQueryToSettle()
}

// AdmitCall 在发起调用之前判一次：配置齐备才准发起。
//
// admitted 为假时 outcome 已经是`出向未配置`，调用方直接交回它，**不得发起调用**
// （ADR-0090 决定五）。齐备时 outcome 是零值，调用方接着去拿真答案。
func AdmitCall(configuration ChannelConfiguration) (outcome Outcome, admitted bool) {
	if !configuration.Configured() {
		return Unconfigured(), false
	}
	return Outcome{}, true
}
