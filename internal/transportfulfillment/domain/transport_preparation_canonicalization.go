package domain

// CanonicalizePreparationInput 定形「准备运输机会」的输入摘要，供同一身份再来时分`已有结果`与`结果冲突`（ADR-0201
// 决定七）。进摘要的是这一判的依据：段的方向与窗口、需求量的来源、锚与逐维数量。身份四维（租户、段引用、对象、规则
// 版本）不进——它们是键。判断结论与候选不进：结论是这些依据在当时的登记册上判出来的，班次与池此后会变，同一身份按原依据
// 再来就是重放，不因登记册变了而读成冲突；依据本身变了（例如客户更正了申报、提供方答出另一版锚）才是冲突。
//
// 准备结果是新聚合，它的规范化与既有聚合的各立一处，不混进 payload_canonicalization.go。形状版本仍用 TFC-1：本口立形
// 时没有任何已存摘要（ADR-0014 的规范化版本纪律）。
func CanonicalizePreparationInput(leg PreparationLeg, demand DemandSnapshot) (string, error) {
	type quantity struct {
		Dimension string `json:"dimension"`
		Amount    string `json:"amount"`
		Unit      string `json:"unit"`
	}
	quantities := make([]quantity, 0, len(demand.quantities))
	for _, each := range demand.quantities {
		quantities = append(quantities, quantity{
			Dimension: each.Dimension.String(),
			Amount:    each.Amount,
			Unit:      each.Unit.String(),
		})
	}
	_, digest, err := marshalPayload(struct {
		Canonicalization string     `json:"canonicalization"`
		Face             string     `json:"face"`
		Connection       string     `json:"connection"`
		FromNode         string     `json:"from_node"`
		ToNode           string     `json:"to_node"`
		WindowStart      string     `json:"window_start"`
		WindowEnd        string     `json:"window_end"`
		DemandSource     string     `json:"demand_source"`
		DemandAnchor     string     `json:"demand_anchor"`
		Quantities       []quantity `json:"quantities"`
	}{
		Canonicalization: payloadCanonicalizationVersion,
		Face:             "PREPARE_TRANSPORT_OPPORTUNITY",
		Connection:       leg.direction.Connection().String(),
		FromNode:         leg.direction.From().String(),
		ToNode:           leg.direction.To().String(),
		WindowStart:      canonicalInstant(leg.windowStart),
		WindowEnd:        canonicalInstant(leg.windowEnd),
		DemandSource:     demand.source.String(),
		DemandAnchor:     demand.anchor,
		Quantities:       quantities,
	})
	return digest, err
}
