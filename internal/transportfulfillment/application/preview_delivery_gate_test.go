package application_test

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// 本文件证派送出发前交付门禁问询的编排半边（ADR-0238）：问询与随后同参的交付登记走同一条读数路径，本上下文零写入。段册、交接册、
// 关务与派送尝试读面都是替身，对象与节点都是合成串，证据只记 `S`。

// deliveredAtFixture 是 registerCommand 那一件在派送尝试替身里的结果时刻：交付登记按它问关务，同参的问询就按它问。
var deliveredAtFixture = deliveryArrivedAt.Add(10 * time.Minute)

func previewDeliveryOf(t *testing.T, at time.Time) application.PreviewDeliveryGateCommand {
	t.Helper()
	command := registerCommand(t)
	return application.PreviewDeliveryGateCommand{TenantID: command.TenantID, Object: command.Object, DeliversAt: at}
}

func deliveryPreviewHandler(t *testing.T, fixture *deliveryGateFixture) *application.PreviewDeliveryGateHandler {
	t.Helper()
	handler, err := application.NewPreviewDeliveryGateHandler(fixture.reader)
	if err != nil {
		t.Fatalf("装问询编排：%v", err)
	}
	return handler
}

// Covers: AT-TF-322、AT-TF-323 编排半边——问询交回的读数与随后同参登记的那次交付同格：整格、at、停点、那一格连同关务判断标识与依据
// 都相同，问关务的问法也逐问相同。关务的五格与两种没形成答复、本侧取不到节点的各停点各至少一例：同一条路径，任何一格都不该分岔。
// 受限的那一件答阻断，放行的答放行或不受门禁。派送尝试读面那一格只属登记：问询不读派送尝试，见本文件末一条用例。
func TestADeliveryPreviewAnswersTheSameReadingAsTheDeliveryWithTheSameParameters(t *testing.T) {
	type arrangement func(*testing.T, *deliveryGateFixture)
	entered := func(t *testing.T, fixture *deliveryGateFixture) { fixture.enteredByHandover(t, "SYN-NODE-SIN-HUB") }
	cases := map[string]struct {
		arrange arrangement
		want    domain.GateAnswer
	}{
		"不受门禁": {arrange: entered, want: domain.GateNotGuarded},
		"放行": {arrange: func(t *testing.T, fixture *deliveryGateFixture) {
			entered(t, fixture)
			fixture.customs.answer = domain.GateAdmitted
		}, want: domain.GateAdmitted},
		"受限答阻断": {arrange: func(t *testing.T, fixture *deliveryGateFixture) {
			entered(t, fixture)
			fixture.customs.answer = domain.GateBlocked
		}, want: domain.GateBlocked},
		"关务答未决": {arrange: func(t *testing.T, fixture *deliveryGateFixture) {
			entered(t, fixture)
			fixture.customs.answer = domain.GateUndecided
		}, want: domain.GateUndecided},
		"关务调用失败": {arrange: func(t *testing.T, fixture *deliveryGateFixture) {
			entered(t, fixture)
			fixture.customs.err = errors.New("customs down")
		}, want: domain.GateUnreadable},
		"关务答复译不回来": {arrange: func(t *testing.T, fixture *deliveryGateFixture) {
			entered(t, fixture)
			fixture.customs.err = fmt.Errorf("drifted: %w", ports.ErrDeliveryGateAnswerUntranslatable)
		}, want: domain.GateUnreadable},
		"此刻不在任何段里": {arrange: func(*testing.T, *deliveryGateFixture) {}, want: domain.GateUndecided},
		"进段那次交接没声明交出节点": {arrange: func(t *testing.T, fixture *deliveryGateFixture) {
			fixture.enteredByHandover(t, "")
		}, want: domain.GateUndecided},
		"凭收寄进段": {arrange: func(t *testing.T, fixture *deliveryGateFixture) {
			segment, err := domain.EstablishSegmentWithPickup(deliverySegmentReference(t), entryPickup(t), domain.PlannedSegmentReference{})
			if err != nil {
				t.Fatalf("立段：%v", err)
			}
			fixture.placeSegment(t, segment)
		}, want: domain.GateUndecided},
		"进段那次交接不在册": {arrange: func(t *testing.T, fixture *deliveryGateFixture) {
			entered(t, fixture)
			fixture.handovers.handovers = nil
		}, want: domain.GateUndecided},
		"段册读不动": {arrange: func(t *testing.T, fixture *deliveryGateFixture) {
			entered(t, fixture)
			fixture.segments.findErr = errors.New("segments down")
		}, want: domain.GateUnreadable},
		"交接册读不动": {arrange: func(t *testing.T, fixture *deliveryGateFixture) {
			entered(t, fixture)
			fixture.handovers.err = errors.New("handovers down")
		}, want: domain.GateUnreadable},
	}
	covered := map[domain.GateAnswer]bool{}
	for name, tc := range cases {
		covered[tc.want] = true
		t.Run(name, func(t *testing.T) {
			fixture := newDeliveryGateFixture(t)
			tc.arrange(t, fixture)

			previewed, err := deliveryPreviewHandler(t, fixture).Preview(context.Background(), previewDeliveryOf(t, deliveredAtFixture))
			if err != nil {
				t.Fatalf("问询：%v", err)
			}
			if previewed.Outcome() != application.DeliveryGatePreviewed {
				t.Fatalf("outcome = %s, want DELIVERY_GATE_PREVIEWED", previewed.Outcome())
			}
			reading, read := previewed.Reading()
			if !read || reading.Answer() != tc.want {
				t.Fatalf("问询读数 = %s（%v），want %s", reading.Answer(), read, tc.want)
			}
			if len(fixture.store.records) != 0 {
				t.Fatalf("问询之后交付登记册有 %d 条，want 零写入", len(fixture.store.records))
			}
			asked := append([]ports.DeliveryGateQuery(nil), fixture.customs.queries...)

			fixture.customs.queries = nil
			registered := fixture.registerAndRead(t)
			if !reflect.DeepEqual(registered, reading) {
				t.Fatalf("问询读数 %+v 与同参交付读数 %+v 不是同一份", reading, registered)
			}
			if !reflect.DeepEqual(fixture.customs.queries, asked) {
				t.Fatalf("问关务的问法不同：问询 %+v，交付 %+v", asked, fixture.customs.queries)
			}
		})
	}
	for _, answer := range []domain.GateAnswer{
		domain.GateNotGuarded, domain.GateAdmitted, domain.GateUndecided, domain.GateUnreadable, domain.GateBlocked,
	} {
		if !covered[answer] {
			t.Errorf("用例表没有一例读出 %s", answer)
		}
	}
}

// Covers: AT-TF-324 编排半边——问询不读派送尝试、不登交付、不结束参与：派送尝试读面坏着也照问，段里那件对象的参与问询之后仍在场，
// 交付登记册一条不多。AT-TF-323 编排半边：读数不交给随后的交付——之后关务改答，交付按它自己那一刻重问、读到改答之后那一格。
func TestADeliveryPreviewWritesNothingAndIsNotHandedToTheDelivery(t *testing.T) {
	fixture := newDeliveryGateFixture(t)
	fixture.enteredByHandover(t, "SYN-NODE-SIN-HUB")
	fixture.reader = readerWithAttemptView(t, fixture, &deliveryViewDouble{err: errors.New("view down")})

	previewed, err := deliveryPreviewHandler(t, fixture).Preview(context.Background(), previewDeliveryOf(t, deliveredAtFixture))
	if err != nil || previewed.Outcome() != application.DeliveryGatePreviewed {
		t.Fatalf("问询：%s / %v", previewed.Outcome(), err)
	}
	if reading, _ := previewed.Reading(); reading.Answer() != domain.GateNotGuarded {
		t.Fatalf("派送尝试读面坏着，问询读数 = %s，want 照问关务", reading.Answer())
	}
	if len(fixture.store.records) != 0 {
		t.Fatalf("问询之后交付登记册有 %d 条", len(fixture.store.records))
	}
	record := fixture.segments.records["segment-1"]
	participation, present := record.Segment.ParticipationFor(mustObject(t, "parcel-1"))
	if !present || !participation.Active() {
		t.Fatal("问询之后那件对象不在段里在场了")
	}

	fixture.reader = readerWithAttemptView(t, fixture, fixture.view)
	fixture.customs.answer = domain.GateBlocked
	if registered := fixture.registerAndRead(t); registered.Answer() != domain.GateBlocked {
		t.Fatalf("交付读数 = %s，want 按它自己那一刻重问的阻断", registered.Answer())
	}
}

// Covers: AT-TF-325 编排半边——缺对象、对象立不起来、缺拟交付时点都答 `INPUT_NOT_ACCEPTED`、不问任何一口；同样那件对象的交付登记
// 同样受理不了（同一处判据）。时点缺席不取服务端此刻。
func TestADeliveryPreviewThatCannotBeAcceptedAsksNothing(t *testing.T) {
	cases := map[string]application.PreviewDeliveryGateCommand{
		"缺对象":    {Object: "", DeliversAt: deliveredAtFixture},
		"对象立不起来": {Object: "parcel\x00-1", DeliversAt: deliveredAtFixture},
		"缺拟交付时点": {Object: "parcel-1"},
	}
	for name, command := range cases {
		t.Run(name, func(t *testing.T) {
			fixture := newDeliveryGateFixture(t)
			fixture.enteredByHandover(t, "SYN-NODE-SIN-HUB")
			command.TenantID = registerCommand(t).TenantID

			result, err := deliveryPreviewHandler(t, fixture).Preview(context.Background(), command)
			if err != nil || result.Outcome() != application.DeliveryGatePreviewNotAccepted {
				t.Fatalf("outcome = %s / %v, want INPUT_NOT_ACCEPTED", result.Outcome(), err)
			}
			if _, read := result.Reading(); read {
				t.Fatal("未受理却带了读数")
			}
			if len(fixture.customs.queries) != 0 || fixture.handovers.asked != 0 {
				t.Fatal("未受理却问了交接册或关务")
			}
			if command.Object != "parcel-1" {
				registration := registerCommand(t)
				registration.Object = command.Object
				if delivered, err := fixture.gatedDeliveryRegister(t, registration); err != nil ||
					delivered.Outcome() != application.DeliveryNotAccepted {
					t.Fatalf("同一件对象的交付登记 = %s / %v, want 同样受理不了", delivered.Outcome(), err)
				}
			}
		})
	}
}

func mustObject(t *testing.T, raw string) domain.CarriedObjectReference {
	t.Helper()
	object, err := domain.NewCarriedObjectReference(raw)
	if err != nil {
		t.Fatalf("对象：%v", err)
	}
	return object
}
