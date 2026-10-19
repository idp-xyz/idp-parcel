package domain

// CanonicalizeSegmentCommissionAdoptionPayload 定形「履约段采用运输委托」，供同一段再来时分`已有采用`与`采用冲突`。段是键，
// 不进摘要；进摘要的是登记方声明的那一对（委托、服务方）。开始依据与时刻不进：它们是采用当时从段上读出来的，段的参与此后
// 可能被更正，同一份声明原样再来仍是重放。
//
// 采用连线是新聚合，规范化另立一处，不混进 payload_canonicalization.go。形状版本用 TFC-1：本口立形时没有任何已存摘要
// （ADR-0014 的规范化版本纪律）。
func CanonicalizeSegmentCommissionAdoptionPayload(commission, provider string) (string, error) {
	_, digest, err := marshalPayload(struct {
		Canonicalization string `json:"canonicalization"`
		Face             string `json:"face"`
		Commission       string `json:"commission"`
		Provider         string `json:"provider"`
	}{
		Canonicalization: payloadCanonicalizationVersion,
		Face:             "ADOPT_SEGMENT_COMMISSION",
		Commission:       commission,
		Provider:         provider,
	})
	return digest, err
}
