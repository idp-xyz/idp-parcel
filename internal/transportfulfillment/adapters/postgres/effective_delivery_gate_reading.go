package postgres

import (
	"encoding/json"
	"errors"
	"fmt"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// deliveryGateReadingDetail 是有效交付登记 gate_reading_detail 列的形状（迁移 0043）。一份交付恰一件对象，所以停点与那一格
// 答复各至多一个；那一格与出发、交出读数逐对象那一格同一形状（gateFindingDetailOf）。词都取领域枚举自己的 String()，读回时
// 按封闭集认，认不出的行不装回——那是库面被绕过构造门写过，不是一格可读的答复。
type deliveryGateReadingDetail struct {
	Node    *string                     `json:"node"`
	Stop    *string                     `json:"stop"`
	Finding *departureGateFindingDetail `json:"finding"`
}

// errDeliveryWithoutGateReading：要落册的那一版没带读数。编排在形成交付时就拒了这一种（withDeliveryGateReading），落到这里
// 是绕过了编排的写入，在发 SQL 之前拒，不靠库面那道 NOT NULL 报一个看不出缘由的约束名。
var errDeliveryWithoutGateReading = errors.New("effective delivery carries no gate reading")

// deliveryGateReadingColumns 把一版交付带的读数折成两列。每一版都有读数。
func deliveryGateReadingColumns(delivery domain.EffectiveDelivery) (string, []byte, error) {
	reading, read := delivery.GateReading()
	if !read {
		return "", nil, errDeliveryWithoutGateReading
	}
	var detail deliveryGateReadingDetail
	if node, asked := reading.Node(); asked {
		text := node.String()
		detail.Node = &text
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
		return "", nil, fmt.Errorf("delivery gate reading detail: %w", err)
	}
	return reading.Answer().String(), raw, nil
}

// deliveryGateReadingFrom 从两列重建读数；由交付的重建门判它属不属于这一份。
func deliveryGateReadingFrom(answerText string, raw []byte) (domain.DeliveryGateReading, error) {
	none := domain.DeliveryGateReading{}
	answer, err := gateAnswerFrom(answerText)
	if err != nil {
		return none, err
	}
	var detail deliveryGateReadingDetail
	if err := json.Unmarshal(raw, &detail); err != nil {
		return none, fmt.Errorf("delivery gate reading detail: %w", err)
	}
	var spec domain.DeliveryGateReadingSpec
	if detail.Node != nil {
		if spec.Node, err = domain.NewNetworkNodeReference(*detail.Node); err != nil {
			return none, fmt.Errorf("delivery gate reading detail: %w", err)
		}
	}
	if detail.Stop != nil {
		if spec.Stop, err = deliveryGateStopFrom(*detail.Stop); err != nil {
			return none, err
		}
	}
	if detail.Finding != nil {
		if spec.Finding, err = gateFindingFrom(*detail.Finding); err != nil {
			return none, err
		}
	}
	return domain.RehydrateDeliveryGateReading(spec, answer)
}

func deliveryGateStopFrom(raw string) (domain.DeliveryGateStop, error) {
	for _, stop := range []domain.DeliveryGateStop{
		domain.DeliveryNoActiveSegment, domain.DeliveryEnteredByPickup, domain.DeliveryReleasingNodeNotDeclared,
		domain.DeliveryEntryHandoverUnresolved, domain.DeliveryRecordedBeforeGateReading,
		domain.DeliveryAttemptViewUnavailable, domain.DeliverySegmentRegistryUnavailable,
		domain.DeliveryHandoverRegistryUnavailable,
	} {
		if stop.String() == raw {
			return stop, nil
		}
	}
	return domain.DeliveryGateStopInvalid, fmt.Errorf("unknown delivery gate stop %q", raw)
}
