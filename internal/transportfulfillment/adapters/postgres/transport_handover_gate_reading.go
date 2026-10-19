package postgres

import (
	"encoding/json"
	"fmt"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// handoverGateReadingDetail 是交接登记册 gate_reading_detail 列的形状（迁移 0041）。一版交接恰一件对象，所以停点与那一格
// 答复各至多一个；那一格与出发读数逐对象那一格同一形状（gateFindingDetailOf）。词都取领域枚举自己的 String()，读回时按
// 封闭集认，认不出的行不装回——那是库面被绕过构造门写过，不是一格可读的答复。
type handoverGateReadingDetail struct {
	Node    *string                     `json:"node"`
	Onward  *string                     `json:"onward"`
	Stop    *string                     `json:"stop"`
	Finding *departureGateFindingDetail `json:"finding"`
}

// handoverGateReadingColumns 把一版交接带的读数折成两列；不是`已交接`的版本两列皆 NULL。
func handoverGateReadingColumns(handover domain.TransportHandover) (*string, []byte, error) {
	reading, read := handover.GateReading()
	if !read {
		return nil, nil, nil
	}
	var detail handoverGateReadingDetail
	if node, asked := reading.Node(); asked {
		text := node.String()
		detail.Node = &text
	}
	if onward, declared := reading.Onward(); declared {
		text := onward.String()
		detail.Onward = &text
	}
	if stop, stopped := reading.Stop(); stopped {
		text := stop.String()
		detail.Stop = &text
	}
	if finding, asked := reading.Finding(); asked {
		row := gateFindingDetailOf(finding)
		detail.Finding = &row
	}
	raw, err := json.Marshal(detail)
	if err != nil {
		return nil, nil, fmt.Errorf("gate reading detail: %w", err)
	}
	answer := reading.Answer().String()
	return &answer, raw, nil
}

// handoverGateReadingFrom 从两列重建读数；两列皆 NULL 交回零值，由交接的重建门判这一版该不该有读数。
func handoverGateReadingFrom(answerText *string, raw []byte) (domain.HandoverGateReading, error) {
	none := domain.HandoverGateReading{}
	if answerText == nil && raw == nil {
		return none, nil
	}
	if answerText == nil || raw == nil {
		return none, fmt.Errorf("gate reading detail: half a reading")
	}
	answer, err := gateAnswerFrom(*answerText)
	if err != nil {
		return none, err
	}
	var detail handoverGateReadingDetail
	if err := json.Unmarshal(raw, &detail); err != nil {
		return none, fmt.Errorf("gate reading detail: %w", err)
	}
	var spec domain.HandoverGateReadingSpec
	if detail.Node != nil {
		if spec.Node, err = domain.NewNetworkNodeReference(*detail.Node); err != nil {
			return none, fmt.Errorf("gate reading detail: %w", err)
		}
	}
	if detail.Onward != nil {
		if spec.Onward, err = domain.NewNetworkNodeReference(*detail.Onward); err != nil {
			return none, fmt.Errorf("gate reading detail: %w", err)
		}
	}
	if detail.Stop != nil {
		if spec.Stop, err = handoverGateStopFrom(*detail.Stop); err != nil {
			return none, err
		}
	}
	if detail.Finding != nil {
		if spec.Finding, err = gateFindingFrom(*detail.Finding); err != nil {
			return none, err
		}
	}
	return domain.RehydrateHandoverGateReading(spec, answer)
}

func handoverGateStopFrom(raw string) (domain.HandoverGateStop, error) {
	for _, stop := range []domain.HandoverGateStop{
		domain.HandoverReleasingNodeNotDeclared, domain.HandoverOnwardSameAsReleasing,
		domain.HandoverRecordedBeforeGateReading, domain.HandoverGateRegistryUnavailable,
	} {
		if stop.String() == raw {
			return stop, nil
		}
	}
	return domain.HandoverGateStopInvalid, fmt.Errorf("unknown handover gate stop %q", raw)
}
