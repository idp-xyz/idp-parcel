package application_test

import (
	"slices"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// 本文件证出发读数核「这一班载的是不是这一段」的编排半边（ADR-0237 决定五）：班次的起讫要就是段里在控各件进段那次`已交接`
// 声明的交出、去向节点；对不上或说不出就停在本侧、不问关务，逐对象那一格一件不列。班次替身的方向是 NODE-SHA → NODE-SZX。

// Covers: AT-TF-114、AT-TF-319 编排半边——设备给的班次与段对不上（进段交接声明的是 NODE-SZX → NODE-SIN，或两件里有一件对不上）
// 答未决、停点「班次不服务该段」；进段交接没声明去向或交出节点、凭收寄进段、那次交接在册上认不出，答未决、停点「段的进段节点
// 说不出」；交接册读不动答读不动。几种都不问集运成员关系、不问关务，读数没有逐对象那一格；出发登记与出发前问询同答。对得上的
// 照问关务。
func TestADepartureOnAScheduleThatDoesNotCarryTheSegmentStopsBeforeAskingCustoms(t *testing.T) {
	cases := map[string]struct {
		arrange func(*testing.T, *departureGateFixture)
		want    []domain.DepartureGateStop
	}{
		"班次起讫与进段声明对不上": {
			arrange: func(t *testing.T, fixture *departureGateFixture) {
				fixture.declare(t, "NODE-SZX", "NODE-SIN", "parcel-1", "parcel-2")
			},
			want: []domain.DepartureGateStop{domain.DepartureScheduleNotServingSegment},
		},
		"两件里一件对不上": {
			arrange: func(t *testing.T, fixture *departureGateFixture) {
				fixture.handovers.handovers[1] = departureEntryHandover(t, "parcel-2", "NODE-SHA", "NODE-SIN")
			},
			want: []domain.DepartureGateStop{domain.DepartureScheduleNotServingSegment},
		},
		"进段交接没声明去向": {
			arrange: func(t *testing.T, fixture *departureGateFixture) {
				fixture.declare(t, "NODE-SHA", "", "parcel-1", "parcel-2")
			},
			want: []domain.DepartureGateStop{domain.DepartureSegmentEntryNodesUnknown},
		},
		"进段交接一格都没声明": {
			arrange: func(t *testing.T, fixture *departureGateFixture) {
				fixture.declare(t, "", "", "parcel-1", "parcel-2")
			},
			want: []domain.DepartureGateStop{domain.DepartureSegmentEntryNodesUnknown},
		},
		"一件说不出、一件对不上": {
			arrange: func(t *testing.T, fixture *departureGateFixture) {
				fixture.handovers.handovers = []domain.TransportHandover{departureEntryHandover(t, "parcel-1", "NODE-SZX", "NODE-SIN")}
			},
			want: []domain.DepartureGateStop{domain.DepartureSegmentEntryNodesUnknown, domain.DepartureScheduleNotServingSegment},
		},
		"凭收寄进段": {
			arrange: func(t *testing.T, fixture *departureGateFixture) {
				segment, err := domain.RehydrateActualFulfillmentSegment(domain.RehydrateActualFulfillmentSegmentSpec{
					TenantID: mustRefValue(t, domain.NewTenantID, "tenant-1"),
					Segment:  mustRefValue(t, domain.NewFulfillmentSegmentReference, "segment-1"),
					Participations: []domain.RehydrateParticipationSpec{{
						Object:     mustRefValue(t, domain.NewCarriedObjectReference, "parcel-1"),
						EntryKind:  domain.EnteredByOffsitePickup,
						EntryBasis: mustRefValue(t, domain.NewParticipationBasisReference, "PICKUP/parcel-1"),
						EnteredAt:  movedAtFixture.Add(-time.Hour),
					}},
				})
				if err != nil {
					t.Fatalf("立段：%v", err)
				}
				fixture.segments.segments["segment-1"] = segment
			},
			want: []domain.DepartureGateStop{domain.DepartureSegmentEntryNodesUnknown},
		},
		"进段那次交接在册上认不出": {
			arrange: func(_ *testing.T, fixture *departureGateFixture) { fixture.handovers.handovers = nil },
			want:    []domain.DepartureGateStop{domain.DepartureSegmentEntryNodesUnknown},
		},
		"交接册读不动": {
			arrange: func(_ *testing.T, fixture *departureGateFixture) { fixture.handovers.err = departureGateDown },
			want:    []domain.DepartureGateStop{domain.DepartureHandoverRegistryUnavailable},
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
			wantWords := make([]string, 0, len(tc.want))
			answer := domain.DepartureGateAnswerInvalid
			for _, stop := range tc.want {
				wantWords = append(wantWords, stop.String())
				answer = max(answer, stop.Answer())
			}
			if !slices.Equal(stopWords(departed), wantWords) || departed.Answer() != answer || len(departed.Findings()) != 0 {
				t.Fatalf("读数 = %s %v，%d 格，want %s %v、没有逐对象那一格", departed.Answer(), stopWords(departed),
					len(departed.Findings()), answer, wantWords)
			}
			if from, to, directed := departed.Direction(); !directed || from.String() != "NODE-SHA" || to.String() != "NODE-SZX" {
				t.Fatalf("起讫 = %s → %s（%v），want 照实写班次的方向", from, to, directed)
			}
			if len(fixture.customs.queries) != 0 || len(fixture.memberships.queries) != 0 {
				t.Fatalf("停在本侧却问了：关务 %d 次、成员关系 %d 次", len(fixture.customs.queries), len(fixture.memberships.queries))
			}
			if cells := domain.ExecutorGateCells(departed.Findings()); len(cells) != 0 {
				t.Fatalf("回给设备的 = %+v，want 一件不列", cells)
			}

			previewed, err := previewHandler(t, fixture).Preview(t.Context(), previewOf(departureCommand(t)))
			if err != nil {
				t.Fatalf("问询：%v", err)
			}
			if reading, _ := previewed.Reading(); !slices.Equal(stopWords(reading), wantWords) || len(fixture.customs.queries) != 0 {
				t.Fatalf("问询读数 = %v（关务 %d 次），want 与出发同停在 %v", stopWords(reading), len(fixture.customs.queries), wantWords)
			}
		})
	}

	t.Run("对得上的照问关务", func(t *testing.T) {
		fixture := newDepartureGateFixture(t, "parcel-1", "parcel-2")
		reading, err := fixture.reader(t).Read(t.Context(), departureCommand(t))
		if err != nil || len(reading.Stops()) != 0 || len(fixture.customs.queries) != 2 {
			t.Fatalf("读数 %v 停点 %v，关务 %d 次", err, stopWords(reading), len(fixture.customs.queries))
		}
	})
}

// declare 把段里各件换成进段交接声明了 releasing → onward 的那一份。
func (fixture *departureGateFixture) declare(t *testing.T, releasing, onward string, objects ...string) {
	t.Helper()
	segments := departureSegmentsDeclaring(t, "segment-1", releasing, onward, objects...)
	fixture.segments.segments = segments.segments
	fixture.handovers.handovers = segments.handovers
}
