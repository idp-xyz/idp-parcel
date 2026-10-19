package domain

import (
	"slices"
	"strings"
	"time"
)

// 送站预报两口载荷的规范化形状（ADR-0139 决定八「每个客户面写口由所属上下文定一版带形状版本的规范化形状」；ADR-0014）。两口
// 都在 NOC-1 下各立一面，与本上下文别的命令口同一个形状版本、同一种摘要前缀：形状只描述「业务内容是否同一份」，不随入口换版。
//
// 进摘要的只有客户声明的内容。租户、客户账户与客户端取自信封，本来就是判重的键；接入请求标识是信封元数据，变成了来源请求键
// （ADR-0139 决定五）——它要是进了摘要，同一份内容换标识重发就永远比不出是同一份。签发的预报标识与形成时刻是本上下文现签的，
// 也不进：重投的那一次根本没签到同一个。

type preAdviceSubmissionDocument struct {
	Canonicalization  string   `json:"canonicalization"`
	Face              string   `json:"face"`
	Node              string   `json:"node"`
	ExpectedArrivalAt string   `json:"expected_arrival_at"`
	Numbers           []string `json:"numbers"`
	DeliveringParty   string   `json:"delivering_party"`
}

// CanonicalizeDeliveryPreAdviceSubmissionPayload 定预报提交口的 NOC-1 形状，交回文档与摘要。
//
// 号码是集合（UC-NO-006 输入语义契约「号码非空、不重复」），排序后编码：交来的次序不是内容。号码本身逐字，带空白的是另一个号。
// 时刻按 UTC、截到微秒——册的精度就是库的精度，摘要描述的是册上那一份。交付方线索缺席写空串：在场的线索不会是空白（构造即拒），
// 两种写法分得开。
func CanonicalizeDeliveryPreAdviceSubmissionPayload(
	node NodeReference,
	expectedArrivalAt time.Time,
	numbers []PreAdvisedParcelNumber,
	deliveringParty DeliveringPartyReference,
) ([]byte, string, error) {
	if !node.valid() || expectedArrivalAt.IsZero() || len(numbers) == 0 {
		return nil, "", ErrInvalidDeliveryPreAdvice
	}
	values := make([]string, 0, len(numbers))
	for _, number := range numbers {
		if !number.valid() {
			return nil, "", ErrInvalidDeliveryPreAdvice
		}
		values = append(values, number.value)
	}
	slices.Sort(values)
	return marshalPayload(preAdviceSubmissionDocument{
		Canonicalization:  payloadCanonicalizationVersion,
		Face:              "SUBMIT_DELIVERY_PRE_ADVICE",
		Node:              node.String(),
		ExpectedArrivalAt: registerInstant(expectedArrivalAt).Format(time.RFC3339Nano),
		Numbers:           values,
		DeliveringParty:   deliveringParty.String(),
	})
}

type preAdviceWithdrawalDocument struct {
	Canonicalization string `json:"canonicalization"`
	Face             string `json:"face"`
	PreAdvice        string `json:"pre_advice"`
}

// CanonicalizeDeliveryPreAdviceWithdrawalPayload 定预报撤回口的 NOC-1 形状：撤回的内容只有它指名的那份预报。
func CanonicalizeDeliveryPreAdviceWithdrawalPayload(preAdvice DeliveryPreAdviceID) ([]byte, string, error) {
	if !preAdvice.valid() {
		return nil, "", ErrInvalidDeliveryPreAdvice
	}
	return marshalPayload(preAdviceWithdrawalDocument{
		Canonicalization: payloadCanonicalizationVersion,
		Face:             "WITHDRAW_DELIVERY_PRE_ADVICE",
		PreAdvice:        preAdvice.String(),
	})
}

// CompareVersionedDigest 比一个自立起就只写带版本摘要的口：已存摘要与本次命令的 NOC-1 摘要同串是同一份，同形状异串是另一份。
// 不带前缀或前缀认不出的已存摘要无从比，答 UnknownPayloadShape——那不是这几口写下的东西，不当冲突、也证明不了是重放。与
// CompareStoredDigest 分开：那一个要替 NOC-1 之前的无版本摘要按旧口径重算，这几口没有那段历史，拿它比会把无版本串当成可比。
func CompareVersionedDigest(stored, current string) StoredDigestComparison {
	shape, _, versioned := strings.Cut(stored, ":")
	if !versioned || shape != payloadCanonicalizationVersion {
		return UnknownPayloadShape
	}
	return payloadComparison(stored == current)
}
