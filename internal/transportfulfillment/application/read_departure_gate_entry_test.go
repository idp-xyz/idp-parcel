package application_test

import (
	"reflect"
	"slices"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// 本文件证出发读数核「这一班载的是不是这一段」的编排半边（ADR-0237 决定五）：段里在控各件进段那次`已交接`声明的交出、去向节点与
// 班次的起讫对不上就停在本侧、不问关务；说不出（凭收寄进段、没声明全、认不出）照问关务、读数记全，读数标「未绑定」，回给执行方的
// 一件不列。班次替身的方向是 NODE-SHA → NODE-SZX，段里两件都受关务替身问。

// declare 把段里各件换成进段交接声明了 releasing → onward 的那一份。
func (fixture *departureGateFixture) declare(t *testing.T, releasing, onward string, objects ...string) {
	t.Helper()
	segments := departureSegmentsDeclaring(t, "segment-1", releasing, onward, objects...)
	fixture.segments.segments = segments.segments
	fixture.handovers.handovers = segments.handovers
}

// enteredByPickup 把段换成 parcel-1、parcel-2 都凭场外揽收进段的那一段：揽收事实只有客户地点、没有节点。
func (fixture *departureGateFixture) enteredByPickup(t *testing.T) {
	t.Helper()
	var participations []domain.RehydrateParticipationSpec
	for _, object := range []string{"parcel-1", "parcel-2"} {
		participations = append(participations, domain.RehydrateParticipationSpec{
			Object:     mustRefValue(t, domain.NewCarriedObjectReference, object),
			EntryKind:  domain.EnteredByOffsitePickup,
			EntryBasis: mustRefValue(t, domain.NewParticipationBasisReference, "PICKUP/"+object),
			EnteredAt:  movedAtFixture.Add(-time.Hour),
		})
	}
	segment, err := domain.RehydrateActualFulfillmentSegment(domain.RehydrateActualFulfillmentSegmentSpec{
		TenantID:       mustRefValue(t, domain.NewTenantID, "tenant-1"),
		Segment:        mustRefValue(t, domain.NewFulfillmentSegmentReference, "segment-1"),
		Participations: participations,
	})
	if err != nil {
		t.Fatalf("立段：%v", err)
	}
	fixture.segments.segments["segment-1"] = segment
}

// Covers: AT-TF-114、AT-TF-319 编排半边——设备给的班次与段对不上（进段交接声明的是 NODE-SZX → NODE-SIN；两件里一件对不上；只声明
// 了一格而那一格不同；一件对不上而另一件说不出）答未决、停点「班次不服务该段」；交接册读不动答读不动。几种都不问集运成员关系、不问
// 关务，读数没有逐对象那一格；出发登记与出发前问询同答。
func TestADepartureOnAScheduleThatDoesNotCarryTheSegmentStopsBeforeAskingCustoms(t *testing.T) {
	cases := map[string]struct {
		arrange func(*testing.T, *departureGateFixture)
		want    domain.DepartureGateStop
	}{
		"班次起讫与进段声明对不上": {
			arrange: func(t *testing.T, fixture *departureGateFixture) {
				fixture.declare(t, "NODE-SZX", "NODE-SIN", "parcel-1", "parcel-2")
			},
			want: domain.DepartureScheduleNotServingSegment,
		},
		"两件里一件对不上": {
			arrange: func(t *testing.T, fixture *departureGateFixture) {
				fixture.handovers.handovers[1] = departureEntryHandover(t, "parcel-2", "NODE-SHA", "NODE-SIN")
			},
			want: domain.DepartureScheduleNotServingSegment,
		},
		"只声明交出节点且对不上": {
			arrange: func(t *testing.T, fixture *departureGateFixture) {
				fixture.declare(t, "NODE-SZX", "", "parcel-1", "parcel-2")
			},
			want: domain.DepartureScheduleNotServingSegment,
		},
		"一件说不出、一件对不上": {
			arrange: func(t *testing.T, fixture *departureGateFixture) {
				fixture.handovers.handovers = []domain.TransportHandover{departureEntryHandover(t, "parcel-1", "NODE-SZX", "NODE-SIN")}
			},
			want: domain.DepartureScheduleNotServingSegment,
		},
		"交接册读不动": {
			arrange: func(_ *testing.T, fixture *departureGateFixture) { fixture.handovers.err = departureGateDown },
			want:    domain.DepartureHandoverRegistryUnavailable,
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			fixture := newDepartureGateFixture(t, "parcel-1", "parcel-2")
			tc.arrange(t, fixture)

			departed, err := fixture.reader(t).Read(t.Context(), departureCommand(t))
			if err != nil {
				t.Fatalf("读数：%v", err)
			}
			want := []string{tc.want.String()}
			if !slices.Equal(stopWords(departed), want) || departed.Answer() != tc.want.Answer() || len(departed.Findings()) != 0 ||
				departed.EntryUnbound() {
				t.Fatalf("读数 = %s %v，%d 格，未绑定 %v，want %s %v、没有逐对象那一格", departed.Answer(), stopWords(departed),
					len(departed.Findings()), departed.EntryUnbound(), tc.want.Answer(), want)
			}
			if from, to, directed := departed.Direction(); !directed || from.String() != "NODE-SHA" || to.String() != "NODE-SZX" {
				t.Fatalf("起讫 = %s → %s（%v），want 照实写班次的方向", from, to, directed)
			}
			if len(fixture.customs.queries) != 0 || len(fixture.memberships.queries) != 0 {
				t.Fatalf("停在本侧却问了：关务 %d 次、成员关系 %d 次", len(fixture.customs.queries), len(fixture.memberships.queries))
			}
			if cells := departed.ExecutorCells(); len(cells) != 0 {
				t.Fatalf("回给设备的 = %+v，want 一件不列", cells)
			}

			previewed, err := previewHandler(t, fixture).Preview(t.Context(), previewOf(departureCommand(t)))
			if err != nil {
				t.Fatalf("问询：%v", err)
			}
			if reading, _ := previewed.Reading(); !reflect.DeepEqual(reading, departed) || len(fixture.customs.queries) != 0 {
				t.Fatalf("问询读数 = %v（关务 %d 次），want 与出发同停在 %v", stopWords(reading), len(fixture.customs.queries), want)
			}
		})
	}
}

// Covers: AT-TF-317 编排半边——说不出段里各件是在哪个节点交进来、往哪去的（凭收寄进段、进段交接没声明去向或一格都没声明、那次交接在
// 册上认不出），照班次方向问关务：每件一格、整格照推（一件被扣就是阻断），读数标「未绑定」，回给执行方的一件不列；出发登记与出发前
// 问询同一份读数、同样问法。两格都对得上的照问关务，不标「未绑定」，回给执行方照常。
func TestADepartureWhoseSegmentCannotBeBoundStillAsksCustomsButShowsTheExecutorNothing(t *testing.T) {
	cases := map[string]func(*testing.T, *departureGateFixture){
		"凭收寄进段": func(t *testing.T, fixture *departureGateFixture) { fixture.enteredByPickup(t) },
		"进段交接没声明去向": func(t *testing.T, fixture *departureGateFixture) {
			fixture.declare(t, "NODE-SHA", "", "parcel-1", "parcel-2")
		},
		"进段交接一格都没声明": func(t *testing.T, fixture *departureGateFixture) {
			fixture.declare(t, "", "", "parcel-1", "parcel-2")
		},
		"进段那次交接在册上认不出": func(_ *testing.T, fixture *departureGateFixture) { fixture.handovers.handovers = nil },
	}
	for name, arrange := range cases {
		t.Run(name, func(t *testing.T) {
			fixture := newDepartureGateFixture(t, "parcel-1", "parcel-2")
			arrange(t, fixture)
			fixture.customs.answers["parcel-2"] = domain.DepartureGateBlocked

			departed, err := fixture.reader(t).Read(t.Context(), departureCommand(t))
			if err != nil {
				t.Fatalf("读数：%v", err)
			}
			if !departed.EntryUnbound() || departed.Answer() != domain.DepartureGateBlocked || len(departed.Stops()) != 0 ||
				len(departed.Findings()) != 2 || len(fixture.customs.queries) != 2 {
				t.Fatalf("读数 = %s %v，%d 格，未绑定 %v，关务 %d 次，want 照问两件、阻断、标未绑定", departed.Answer(), stopWords(departed),
					len(departed.Findings()), departed.EntryUnbound(), len(fixture.customs.queries))
			}
			if cells := departed.ExecutorCells(); len(cells) != 0 {
				t.Fatalf("回给设备的 = %+v，want 一件不列", cells)
			}
			asked := slices.Clone(fixture.customs.queries)

			fixture.customs.queries = nil
			previewed, err := previewHandler(t, fixture).Preview(t.Context(), previewOf(departureCommand(t)))
			if err != nil {
				t.Fatalf("问询：%v", err)
			}
			if reading, _ := previewed.Reading(); !reflect.DeepEqual(reading, departed) || !reflect.DeepEqual(fixture.customs.queries, asked) {
				t.Fatalf("问询读数 %+v 与出发读数 %+v 不是同一份，或问法不同", reading, departed)
			}
		})
	}

	t.Run("对得上的照问关务、照常回给执行方", func(t *testing.T) {
		fixture := newDepartureGateFixture(t, "parcel-1", "parcel-2")
		fixture.customs.answers["parcel-2"] = domain.DepartureGateBlocked
		reading, err := fixture.reader(t).Read(t.Context(), departureCommand(t))
		if err != nil || reading.EntryUnbound() || len(fixture.customs.queries) != 2 {
			t.Fatalf("读数 %v，未绑定 %v，关务 %d 次", err, reading.EntryUnbound(), len(fixture.customs.queries))
		}
		if cells := reading.ExecutorCells(); len(cells) != 1 || cells[0].Object.String() != "parcel-2" {
			t.Fatalf("回给设备的 = %+v，want 被挡那一件", cells)
		}
	})
}
