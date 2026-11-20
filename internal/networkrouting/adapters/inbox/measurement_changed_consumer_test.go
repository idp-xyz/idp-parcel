package nrinbox

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-bento-go/eventing"
)

// 译码结果就是消费门交给处理方的那一份。毒信封在门上被拒收、调用方看不到这个错误，所以直接钉译码。

func measurementEnvelope(payload string, at time.Time) eventing.Envelope {
	return eventing.Envelope{
		OccurredAt: at,
		Payload:    []byte(payload),
	}
}

const wellFormedMeasurementPayload = `{"tenantId":"tenant-1","sourceId":"src-1","objectKind":"handling-unit","objectId":"unit-1"}`

// Covers: 载荷四个字段各缺一个都是毒信封。重投同样内容不会长出字段。
func TestDecodeChangedActualMeasurementRejectsAPayloadMissingAnyField(t *testing.T) {
	at := time.Date(2026, 3, 2, 4, 5, 6, 0, time.UTC)
	for name, payload := range map[string]string{
		"缺 tenantId":   `{"sourceId":"src-1","objectKind":"handling-unit","objectId":"unit-1"}`,
		"缺 sourceId":   `{"tenantId":"tenant-1","objectKind":"handling-unit","objectId":"unit-1"}`,
		"缺 objectKind": `{"tenantId":"tenant-1","sourceId":"src-1","objectId":"unit-1"}`,
		"缺 objectId":   `{"tenantId":"tenant-1","sourceId":"src-1","objectKind":"handling-unit"}`,
	} {
		t.Run(name, func(t *testing.T) {
			_, err := decodeChangedActualMeasurement(measurementEnvelope(payload, at))
			if !errors.Is(err, ErrPoisonEnvelope) {
				t.Fatalf("err = %v, want ErrPoisonEnvelope", err)
			}
		})
	}
}

// Covers: 信封没有业务时间是毒信封。业务时间不在载荷里，重投补不上。
func TestDecodeChangedActualMeasurementRejectsAZeroBusinessTime(t *testing.T) {
	_, err := decodeChangedActualMeasurement(measurementEnvelope(wellFormedMeasurementPayload, time.Time{}))
	if !errors.Is(err, ErrPoisonEnvelope) {
		t.Fatalf("err = %v, want ErrPoisonEnvelope", err)
	}
}

// Covers: 正常信封交给处理方的业务时间就是信封上的那一刻，不取处理时刻。
func TestDecodeChangedActualMeasurementHandsTheEnvelopeBusinessTimeToTheHandler(t *testing.T) {
	at := time.Date(1999, 7, 8, 9, 10, 11, 0, time.UTC)
	got, err := decodeChangedActualMeasurement(measurementEnvelope(wellFormedMeasurementPayload, at))
	if err != nil {
		t.Fatalf("译码：%v", err)
	}
	if !got.OccurredAt.Equal(at) || got.SourceID != "src-1" || got.ObjectID != "unit-1" {
		t.Fatalf("译码 = %+v，want 业务时间 %s、来源 src-1、实物 unit-1", got, at)
	}
}
