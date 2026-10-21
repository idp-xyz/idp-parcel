package tfhttp_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// 本文件证交付门禁读数（ADR-0213 决定五）在线格式上的样子：在册那一版的答复带 gateReading，与出发、交出读数同形，at 代
// from；阻断照样 201；重放回显首次读数；POD 更正回显承继来的那一份；登不出交付的答复不带。读数在组合根问，这里照组合根的
// 样子交给 RegisterWithGateReading，证据只记 `S`。

// gatedDelivery 把一份给定的读数交给 RegisterWithGateReading，代组合根（cmd/parcel-api 的 transactionalDelivery）在写事务
// 之前问好的那一份。本包不碰事务，也不问关务。
type gatedDelivery struct {
	inner   *application.RegisterEffectiveDeliveryHandler
	reading domain.DeliveryGateReading
}

func (handler gatedDelivery) Register(
	ctx context.Context,
	command application.RegisterEffectiveDeliveryCommand,
) (application.RegisterEffectiveDeliveryResult, error) {
	return handler.inner.RegisterWithGateReading(ctx, command, handler.reading)
}

func (handler gatedDelivery) Correct(
	ctx context.Context,
	command application.CorrectDeliveryProofCommand,
) (application.RegisterEffectiveDeliveryResult, error) {
	return handler.inner.Correct(ctx, command)
}

func stoppedDeliveryGateReading(t *testing.T) domain.DeliveryGateReading {
	t.Helper()
	return deliveryGateReadingOf(t, domain.DeliveryGateReadingSpec{Stop: domain.DeliveryNoActiveSegment})
}

func askedDeliveryGateReading(t *testing.T, answer domain.GateAnswer) domain.DeliveryGateReading {
	t.Helper()
	finding, err := domain.NewGateFinding(httpValue(t, domain.NewCarriedObjectReference, "parcel-1"), answer, "AADJ-STUB",
		[]domain.GateBasis{{Action: "FINAL_DELIVERY", Cause: "OUTSIDE_CUSTOMS_BOOKS"}})
	if err != nil {
		t.Fatalf("finding: %v", err)
	}
	return deliveryGateReadingOf(t, domain.DeliveryGateReadingSpec{
		Node:    httpValue(t, domain.NewNetworkNodeReference, "SYN-NODE-SIN-HUB"),
		Finding: finding,
	})
}

func deliveryGateReadingOf(t *testing.T, spec domain.DeliveryGateReadingSpec) domain.DeliveryGateReading {
	t.Helper()
	reading, err := domain.FormDeliveryGateReading(spec)
	if err != nil {
		t.Fatalf("delivery gate reading: %v", err)
	}
	return reading
}

type deliveryGateReadingWire struct {
	Answer  string   `json:"answer"`
	At      string   `json:"at"`
	Stops   []string `json:"stops"`
	Objects []struct {
		Object string `json:"object"`
		Answer string `json:"answer"`
	} `json:"objects"`
}

func decodeDeliveryGateReading(t *testing.T, raw []byte) (deliveryGateReadingWire, bool) {
	t.Helper()
	var body struct {
		GateReading *deliveryGateReadingWire `json:"gateReading"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.GateReading == nil {
		return deliveryGateReadingWire{}, false
	}
	return *body.GateReading, true
}

// Covers: `AT-TF-263`、`AT-TF-265`（线格式半边）——首登 201 带 gateReading：整格与 at；那件对象不受门禁，回给登记方的逐对象那一格
// 不列它（`AT-TF-321` 线格式半边，ADR-0237）；关务判断标识与依据原文不回给登记方。原样重放 200 `EXISTING_VERSION` 回显同一份；
// POD 更正 201 回显从前版承继的那一份。
func TestTheDeliveryAnswerCarriesTheGateReadingAndItsReplayEchoesIt(t *testing.T) {
	fixture := newTFFixtureReading(t, askedDeliveryGateReading(t, domain.GateNotGuarded))

	first := postJSON(t, fixture.register, defaultRegisterBody())
	if first.Code != http.StatusCreated {
		t.Fatalf("首登 status = %d body = %s", first.Code, first.Body.String())
	}
	reading, present := decodeDeliveryGateReading(t, first.Body.Bytes())
	if !present || reading.Answer != "NOT_GUARDED" || reading.At != "SYN-NODE-SIN-HUB" || len(reading.Stops) != 0 ||
		len(reading.Objects) != 0 {
		t.Fatalf("gateReading = %+v (present %v)", reading, present)
	}
	if strings.Contains(first.Body.String(), `"objects"`) {
		t.Fatalf("得出放行的那件对象出现在答复里：%s", first.Body.String())
	}
	if strings.Contains(first.Body.String(), "AADJ-STUB") || strings.Contains(first.Body.String(), "OUTSIDE_CUSTOMS_BOOKS") {
		t.Fatalf("答复带出了关务判断标识或依据原文：%s", first.Body.String())
	}

	replay := postJSON(t, fixture.register, defaultRegisterBody())
	if replay.Code != http.StatusOK || decodeDelivery(t, replay).Outcome != "EXISTING_VERSION" {
		t.Fatalf("重放 status = %d body = %s", replay.Code, replay.Body.String())
	}
	if echoed, _ := decodeDeliveryGateReading(t, replay.Body.Bytes()); echoed.Answer != "NOT_GUARDED" || echoed.At != "SYN-NODE-SIN-HUB" ||
		len(echoed.Objects) != 0 {
		t.Fatalf("重放回显的读数 = %+v", echoed)
	}

	correction := postJSON(t, fixture.correct, map[string]any{
		"attempt":            "attempt-1",
		"object":             "parcel-1",
		"predecessorVersion": decodeDelivery(t, first).DeliveryVersion,
		"newProof":           "pod-photo-2",
		"correctedAt":        time.Date(2026, 9, 5, 13, 0, 0, 0, time.UTC).Format(time.RFC3339),
	})
	if correction.Code != http.StatusCreated {
		t.Fatalf("更正 status = %d body = %s", correction.Code, correction.Body.String())
	}
	if inherited, _ := decodeDeliveryGateReading(t, correction.Body.Bytes()); inherited.Answer != "NOT_GUARDED" || inherited.At != "SYN-NODE-SIN-HUB" ||
		len(inherited.Objects) != 0 {
		t.Fatalf("更正回显的读数 = %+v，want 承继前版那一份", inherited)
	}
}

// Covers: `AT-TF-257`、`AT-TF-260`、`AT-TF-265`（线格式半边）——阻断照样 201，那件对象列在逐对象那一格里（`AT-TF-321` 线格式半边）；
// 停在本上下文这一侧的读数只带整格与停点，不带 at 与对象。登不出交付的答复（未生效）不带 gateReading。
func TestABlockedOrStoppedDeliveryGateReadingStillRegisters(t *testing.T) {
	blocked := newTFFixtureReading(t, askedDeliveryGateReading(t, domain.GateBlocked))
	response := postJSON(t, blocked.register, defaultRegisterBody())
	if response.Code != http.StatusCreated {
		t.Fatalf("阻断 status = %d body = %s", response.Code, response.Body.String())
	}
	if reading, _ := decodeDeliveryGateReading(t, response.Body.Bytes()); reading.Answer != "BLOCKED" ||
		len(reading.Objects) != 1 || reading.Objects[0].Object != "parcel-1" || reading.Objects[0].Answer != "BLOCKED" {
		t.Fatalf("阻断读数 = %+v", reading)
	}

	stopped := newTFFixtureReading(t, deliveryGateReadingOf(t, domain.DeliveryGateReadingSpec{Stop: domain.DeliveryEnteredByPickup}))
	response = postJSON(t, stopped.register, defaultRegisterBody())
	if response.Code != http.StatusCreated {
		t.Fatalf("停点 status = %d body = %s", response.Code, response.Body.String())
	}
	reading, _ := decodeDeliveryGateReading(t, response.Body.Bytes())
	if reading.Answer != "UNDECIDED" || len(reading.Stops) != 1 || reading.Stops[0] != "ENTERED_BY_PICKUP" ||
		reading.At != "" || len(reading.Objects) != 0 {
		t.Fatalf("停点读数 = %+v", reading)
	}

	notEffective := newTFFixtureReading(t, stoppedDeliveryGateReading(t))
	notEffective.view.outcome = domain.NoOneToReceive
	response = postJSON(t, notEffective.register, defaultRegisterBody())
	if response.Code != http.StatusOK || decodeDelivery(t, response).Outcome != "NOT_EFFECTIVE" {
		t.Fatalf("未生效 status = %d body = %s", response.Code, response.Body.String())
	}
	if _, present := decodeDeliveryGateReading(t, response.Body.Bytes()); present {
		t.Fatalf("登不出交付的答复带了 gateReading：%s", response.Body.String())
	}
}
