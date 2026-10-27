package application_test

import (
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/networkrouting/application"
	"go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	"go.idp.xyz/idp-parcel/internal/networkrouting/ports"
)

// 本文件钉复核给候选出价的包裹事实依据（CONTEXT「预路由使用客户声明快照；首次收寄复核使用当时已经取得的物理事实，到站
// 复核使用当前有效物理实测」）：判断点由触发所携的控制依据定，取数侧交回的依据留在复核记录上——不形成计划的复核不留成本，
// 候选评估是按声明还是按实测判的，只有记录上这一格查得出。

// physicalPriced 是取数侧按物理事实给两条候选出的价。
func physicalPriced(t *testing.T) *ports.RouteCandidateCosts {
	t.Helper()
	answer := pricedBySource(t)
	answer.FactsBasis = domain.ParcelFactsPhysical
	return answer
}

// Covers: 判断点由触发的控制依据定（判据 2）——节点收寄与场外揽收是首次收寄复核，物理事实截到收寄的业务时间；权威运输交接
// 进接收节点是到站复核。每种控制依据都向取数侧指名自己的判断点，不落进默认。
func TestEachControlEvidenceNamesItsOwnJudgmentPointToTheCostSource(t *testing.T) {
	for name, test := range map[string]struct {
		control domain.ControlEvidenceKind
		want    domain.ParcelFactsPointKind
	}{
		"节点收寄": {domain.NodeIntakeControl, domain.FirstIntakeFactsPoint},
		"场外揽收": {domain.OffsitePickupControl, domain.FirstIntakeFactsPoint},
		"权威交接": {domain.TransportHandoverControl, domain.ArrivalFactsPoint},
	} {
		t.Run(name, func(t *testing.T) {
			fixture := newReassessFixture(t)
			fixture.costs.answer = physicalPriced(t)
			record := currentPlanRecord(t)
			fixture.routes.records[record.Key] = record
			fixture.evidence.byParcel["parcel-1"] = withoutCosts(restrictedSelectedCandidate(t, cheaperAlternative(t, 100)))
			command := reassessCommand(t, "node-origin")
			command.Trigger.Control = test.control

			result := handleOK(t, fixture, command)
			if result.Outcome() != application.ReassessedPlanLapsed || len(fixture.costs.points) == 0 {
				t.Fatalf("outcome = %q、取成本 %d 次，want 失效线向取数侧要过成本", result.Outcome(), len(fixture.costs.points))
			}
			point := fixture.costs.points[len(fixture.costs.points)-1]
			if point.Kind() != test.want {
				t.Fatalf("判断点 = %v，want %v", point.Kind(), test.want)
			}
			if test.want == domain.FirstIntakeFactsPoint {
				if asOf, _ := point.PhysicalFactsAsOf(reassessedAt.Add(24 * time.Hour)); !asOf.Equal(command.Trigger.OccurredAt) {
					t.Fatalf("物理事实截至 %s，want 收寄的业务时间 %s", asOf, command.Trigger.OccurredAt)
				}
			}
		})
	}
}

// Covers: 取数侧交回的包裹事实依据留在复核记录上（判据 3）——失效线、仍适用线（含改善不够格没切换的）、无路由线（含再形成
// 首个计划的）各一格。
func TestTheCandidateFactsBasisStaysOnEveryRecordThatPricedCandidates(t *testing.T) {
	t.Run("失效线", func(t *testing.T) {
		fixture := rerouteFixture(t)
		fixture.costs.answer = physicalPriced(t)
		fixture.evidence.byParcel["parcel-1"] = withoutCosts(cheaperAlternative(t, 100))

		record, _ := handleOK(t, fixture, reassessCommand(t, "node-actual-elsewhere")).Record()
		if record.CandidateFactsBasis != domain.ParcelFactsPhysical {
			t.Fatalf("依据 = %s，want PHYSICAL", record.CandidateFactsBasis)
		}
	})
	t.Run("仍适用线，改善够格切换", func(t *testing.T) {
		fixture := improvementFixture(t, 500)
		fixture.costs.answer = physicalPriced(t)

		record, _ := handleOK(t, fixture, reassessCommand(t, "node-origin")).Record()
		if record.Conclusion != ports.ReassessmentRerouted || record.CandidateFactsBasis != domain.ParcelFactsPhysical {
			t.Fatalf("结论 %v、依据 %s，want 已改路、PHYSICAL", record.Conclusion, record.CandidateFactsBasis)
		}
	})
	t.Run("仍适用线，改善不够格没切换", func(t *testing.T) {
		fixture := improvementFixture(t, 900)
		fixture.costs.answer = physicalPriced(t)

		result := handleOK(t, fixture, reassessCommand(t, "node-origin"))
		record, _ := result.Record()
		if result.Outcome() != application.ReassessedStillApplicable || record.CandidateFactsBasis != domain.ParcelFactsPhysical {
			t.Fatalf("outcome %q、依据 %s，want 仍适用、PHYSICAL——没切换的复核也出过价", result.Outcome(), record.CandidateFactsBasis)
		}
	})
	t.Run("无路由线再形成首个计划", func(t *testing.T) {
		fixture := newReassessFixture(t)
		fixture.costs.answer = physicalPriced(t)
		record := currentPlanRecord(t)
		fixture.routes.records[record.Key] = record
		fixture.evidence.byParcel["parcel-1"] = withoutCosts(restrictedSelectedCandidate(t, evidenceBeforeFreeze(t)))
		lapsedOnPickup(t, fixture)

		fixture.evidence.byParcel["parcel-1"] = withoutCosts(cheaperAlternative(t, 100))
		formed, _ := handleOK(t, fixture, reassessCommandAt(t, "trigger-2", "node-origin", reassessedAt.Add(time.Hour))).Record()
		if formed.Conclusion != ports.ReassessmentFirstPlanFormed || formed.CandidateFactsBasis != domain.ParcelFactsPhysical {
			t.Fatalf("结论 %v、依据 %s，want 首个计划、PHYSICAL", formed.Conclusion, formed.CandidateFactsBasis)
		}
	})
	t.Run("无路由线仍无路由", func(t *testing.T) {
		fixture := newReassessFixture(t)
		fixture.costs.answer = physicalPriced(t)
		record := currentPlanRecord(t)
		fixture.routes.records[record.Key] = record
		restricted := withoutCosts(restrictedSelectedCandidate(t, evidenceBeforeFreeze(t)))
		fixture.evidence.byParcel["parcel-1"] = restricted
		lapsedOnPickup(t, fixture)

		still, _ := handleOK(t, fixture, reassessCommandAt(t, "trigger-2", "node-origin", reassessedAt.Add(time.Hour))).Record()
		if still.CandidateState != ports.NoQualifiedCandidates || still.CandidateFactsBasis != domain.ParcelFactsPhysical {
			t.Fatalf("候选评估 %q、依据 %s，want 无合格候选、PHYSICAL", still.CandidateState, still.CandidateFactsBasis)
		}
	})
}

// Covers: 没向取数侧要过成本的复核不带依据——场外揽收在仍适用线上不试改善、排序形态没声明时问都不问；零值如实表示
// 「没出过价」，不记成某一格。
func TestARecordThatNeverPricedCandidatesCarriesNoBasis(t *testing.T) {
	t.Run("场外揽收仍适用", func(t *testing.T) {
		fixture := newReassessFixture(t)
		fixture.costs.answer = physicalPriced(t)
		record := currentPlanRecord(t)
		fixture.routes.records[record.Key] = record
		fixture.evidence.byParcel["parcel-1"] = withoutCosts(declareCostImprovement(cheaperAlternative(t, 100), 0))

		got, _ := handleOK(t, fixture, pickupCommand(t)).Record()
		if len(fixture.costs.points) != 0 || got.CandidateFactsBasis != domain.ParcelFactsBasisInvalid {
			t.Fatalf("取成本 %d 次、依据 %s，want 没问过、没有依据", len(fixture.costs.points), got.CandidateFactsBasis)
		}
	})
	t.Run("排序形态没声明", func(t *testing.T) {
		fixture := rerouteFixture(t)
		fixture.costs.answer = physicalPriced(t)
		evidence := withoutCosts(cheaperAlternative(t, 100))
		evidence.RankingForm = domain.RankingFormUndeclared
		fixture.evidence.byParcel["parcel-1"] = evidence

		got, _ := handleOK(t, fixture, reassessCommand(t, "node-actual-elsewhere")).Record()
		if len(fixture.costs.points) != 0 || got.CandidateFactsBasis != domain.ParcelFactsBasisInvalid {
			t.Fatalf("取成本 %d 次、依据 %s，want 没问过、没有依据", len(fixture.costs.points), got.CandidateFactsBasis)
		}
	})
}
