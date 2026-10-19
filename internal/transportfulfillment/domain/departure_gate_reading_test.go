package domain_test

import (
	"errors"
	"slices"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// stoppedReading 是一份停在本上下文这一侧、没问关务的读数。
func stoppedReading(t *testing.T, stops ...domain.DepartureGateStop) domain.DepartureGateReading {
	t.Helper()
	reading, err := domain.FormDepartureGateReading(domain.DepartureGateReadingSpec{Stops: stops})
	if err != nil {
		t.Fatalf("停点读数：%v", err)
	}
	return reading
}

// askedReading 是一份问到了关务、只有一件对象的读数。
func askedReading(t *testing.T, answer domain.DepartureGateAnswer) domain.DepartureGateReading {
	t.Helper()
	reading, err := domain.FormDepartureGateReading(domain.DepartureGateReadingSpec{
		From:     mustValue(t, domain.NewNetworkNodeReference, "NODE-A"),
		To:       mustValue(t, domain.NewNetworkNodeReference, "NODE-B"),
		Findings: []domain.DepartureGateFinding{gateFinding(t, "parcel-1", answer)},
	})
	if err != nil {
		t.Fatalf("问到关务的读数：%v", err)
	}
	return reading
}

func gateFinding(t *testing.T, object string, answer domain.DepartureGateAnswer) domain.DepartureGateFinding {
	t.Helper()
	judgement := "AADJ-" + object
	basis := domain.DepartureGateBasis{Action: "LOADING_DEPARTURE", Cause: "SOME_CAUSE", Sources: []string{"SRC-1"}}
	if answer == domain.DepartureGateUnreadable {
		judgement, basis = "", domain.DepartureGateBasis{Cause: "RELEASE_OUTCOME_VIEW_UNAVAILABLE"}
	}
	finding, err := domain.NewDepartureGateFinding(mustValue(t, domain.NewCarriedObjectReference, object), answer, judgement,
		[]domain.DepartureGateBasis{basis})
	if err != nil {
		t.Fatalf("逐对象那一格：%v", err)
	}
	return finding
}

// Covers: AT-TF-115 领域半边——一条出发上多个对象，整格按压制次序取最重：阻断压过读不动，读不动压过未决，未决压过放行，
// 放行压过不受门禁（ADR-0205 决定二）。逐对象那一格原样保留、按对象排好，读的人看得见是哪一件。
func TestTheHeaviestObjectAnswerIsTheDepartureAnswer(t *testing.T) {
	order := []domain.DepartureGateAnswer{
		domain.DepartureGateNotGuarded, domain.DepartureGateAdmitted, domain.DepartureGateUndecided,
		domain.DepartureGateUnreadable, domain.DepartureGateBlocked,
	}
	for heavier := 1; heavier < len(order); heavier++ {
		for lighter := 0; lighter < heavier; lighter++ {
			reading, err := domain.FormDepartureGateReading(domain.DepartureGateReadingSpec{
				From: mustValue(t, domain.NewNetworkNodeReference, "NODE-A"),
				To:   mustValue(t, domain.NewNetworkNodeReference, "NODE-B"),
				Findings: []domain.DepartureGateFinding{
					gateFinding(t, "parcel-2", order[lighter]),
					gateFinding(t, "parcel-1", order[heavier]),
				},
			})
			if err != nil {
				t.Fatalf("%s + %s：%v", order[lighter], order[heavier], err)
			}
			if reading.Answer() != order[heavier] {
				t.Fatalf("%s + %s 压成 %s", order[lighter], order[heavier], reading.Answer())
			}
			findings := reading.Findings()
			if len(findings) != 2 || findings[0].Object().String() != "parcel-1" || findings[1].Answer() != order[lighter] {
				t.Fatalf("逐对象那一格没有原样按对象排好：%+v", findings)
			}
		}
	}
}

// Covers: AT-TF-114 领域半边——停在本上下文这一侧时，每种停点只属一格：缺段、缺对象、缺班次答未决，段册与班次册读不动答读不动，
// 两者并存时读不动压过未决。停点与逐对象答复恰有其一：停下了就没问关务，问了关务就没有停点。
func TestStopsBelongToOneAnswerAndExcludeFindings(t *testing.T) {
	for stop, want := range map[domain.DepartureGateStop]domain.DepartureGateAnswer{
		domain.DepartureSegmentNotDeclared:          domain.DepartureGateUndecided,
		domain.DepartureSegmentNotRegistered:        domain.DepartureGateUndecided,
		domain.DepartureNoObjectUnderControl:        domain.DepartureGateUndecided,
		domain.DepartureScheduleNotRegistered:       domain.DepartureGateUndecided,
		domain.DepartureSegmentRegistryUnavailable:  domain.DepartureGateUnreadable,
		domain.DepartureScheduleRegistryUnavailable: domain.DepartureGateUnreadable,
	} {
		if got := stoppedReading(t, stop).Answer(); got != want {
			t.Errorf("%s 答 %s, want %s", stop, got, want)
		}
	}
	if got := stoppedReading(t, domain.DepartureSegmentNotRegistered, domain.DepartureScheduleRegistryUnavailable).Answer(); got != domain.DepartureGateUnreadable {
		t.Errorf("缺段 + 班次册读不动答 %s, want UNREADABLE", got)
	}

	for name, spec := range map[string]domain.DepartureGateReadingSpec{
		"既无停点也无对象": {},
		"停点与对象并存": {
			From: mustValue(t, domain.NewNetworkNodeReference, "NODE-A"), To: mustValue(t, domain.NewNetworkNodeReference, "NODE-B"),
			Stops:    []domain.DepartureGateStop{domain.DepartureSegmentNotRegistered},
			Findings: []domain.DepartureGateFinding{gateFinding(t, "parcel-1", domain.DepartureGateNotGuarded)},
		},
		"问了关务却没有起讫": {Findings: []domain.DepartureGateFinding{gateFinding(t, "parcel-1", domain.DepartureGateNotGuarded)}},
		"半截起讫":      {From: mustValue(t, domain.NewNetworkNodeReference, "NODE-A"), Stops: []domain.DepartureGateStop{domain.DepartureSegmentNotDeclared}},
		"起讫同一节点": {
			From: mustValue(t, domain.NewNetworkNodeReference, "NODE-A"), To: mustValue(t, domain.NewNetworkNodeReference, "NODE-A"),
			Findings: []domain.DepartureGateFinding{gateFinding(t, "parcel-1", domain.DepartureGateNotGuarded)},
		},
		"同一件问了两次": {
			From: mustValue(t, domain.NewNetworkNodeReference, "NODE-A"), To: mustValue(t, domain.NewNetworkNodeReference, "NODE-B"),
			Findings: []domain.DepartureGateFinding{
				gateFinding(t, "parcel-1", domain.DepartureGateNotGuarded), gateFinding(t, "parcel-1", domain.DepartureGateBlocked),
			},
		},
		"停点重复":    {Stops: []domain.DepartureGateStop{domain.DepartureSegmentNotDeclared, domain.DepartureSegmentNotDeclared}},
		"停点越界":    {Stops: []domain.DepartureGateStop{domain.DepartureGateStopInvalid}},
		"新读数声称没问": {Stops: []domain.DepartureGateStop{domain.DepartureRecordedBeforeGateReading}},
	} {
		if _, err := domain.FormDepartureGateReading(spec); !errors.Is(err, domain.ErrInvalidDepartureGateReading) {
			t.Errorf("%s：err = %v, want ErrInvalidDepartureGateReading", name, err)
		}
	}
}

// Covers: 迁移 0026 的存量回填——本迁移之前落册的出发由重建门装回为未决、停点「登记于门禁读数之前」，且只能是这唯一一个停点、
// 没有起讫；整格与停点推出的那一格对不上的行不装回。
func TestTheRehydrationDoorChecksTheRecordedAnswerAndAdmitsTheBackfill(t *testing.T) {
	backfill := domain.DepartureGateReadingSpec{Stops: []domain.DepartureGateStop{domain.DepartureRecordedBeforeGateReading}}
	reading, err := domain.RehydrateDepartureGateReading(backfill, domain.DepartureGateUndecided)
	if err != nil || reading.Answer() != domain.DepartureGateUndecided {
		t.Fatalf("存量回填装不回：%v answer=%s", err, reading.Answer())
	}
	if _, err := domain.RehydrateDepartureGateReading(backfill, domain.DepartureGateNotGuarded); !errors.Is(err, domain.ErrInvalidDepartureGateReading) {
		t.Fatalf("整格对不上停点却装回了：%v", err)
	}
	mixed := domain.DepartureGateReadingSpec{Stops: []domain.DepartureGateStop{
		domain.DepartureRecordedBeforeGateReading, domain.DepartureSegmentNotDeclared,
	}}
	if _, err := domain.RehydrateDepartureGateReading(mixed, domain.DepartureGateUndecided); !errors.Is(err, domain.ErrInvalidDepartureGateReading) {
		t.Fatalf("存量回填带了别的停点却装回了：%v", err)
	}

	asked := domain.DepartureGateReadingSpec{
		From: mustValue(t, domain.NewNetworkNodeReference, "NODE-A"), To: mustValue(t, domain.NewNetworkNodeReference, "NODE-B"),
		Findings: []domain.DepartureGateFinding{gateFinding(t, "parcel-1", domain.DepartureGateBlocked)},
	}
	if _, err := domain.RehydrateDepartureGateReading(asked, domain.DepartureGateNotGuarded); !errors.Is(err, domain.ErrInvalidDepartureGateReading) {
		t.Fatalf("阻断的依据装成了不受门禁：%v", err)
	}

	// 存量回填装回的出发本体照常成立：那时没有段可对，不拿「没声明段」去要求它。
	spec := movementSpec(t, domain.DepartureFact)
	spec.Segment = mustValue(t, domain.NewFulfillmentSegmentReference, "segment-legacy")
	spec.GateReading = reading
	if _, err := domain.RecordMovementFact(spec); err != nil {
		t.Fatalf("带存量回填读数的出发立不成：%v", err)
	}
}

// Covers: 逐对象那一格的构造门——关务形成了判断才有标识，读不动时没有；依据至少一条；原文不许有 NUL（库面 jsonb 存不下）。
func TestADepartureGateFindingCarriesAJudgementExactlyWhenCustomsJudged(t *testing.T) {
	object := mustValue(t, domain.NewCarriedObjectReference, "parcel-1")
	basis := []domain.DepartureGateBasis{{Action: "LOADING_DEPARTURE", Cause: "OUTSIDE_CUSTOMS_BOOKS"}}
	for name, try := range map[string]func() (domain.DepartureGateFinding, error){
		"判断了却没有标识": func() (domain.DepartureGateFinding, error) {
			return domain.NewDepartureGateFinding(object, domain.DepartureGateNotGuarded, " ", basis)
		},
		"读不动却带了标识": func() (domain.DepartureGateFinding, error) {
			return domain.NewDepartureGateFinding(object, domain.DepartureGateUnreadable, "AADJ-1", basis)
		},
		"没有依据": func() (domain.DepartureGateFinding, error) {
			return domain.NewDepartureGateFinding(object, domain.DepartureGateNotGuarded, "AADJ-1", nil)
		},
		"依据种类空着": func() (domain.DepartureGateFinding, error) {
			return domain.NewDepartureGateFinding(object, domain.DepartureGateNotGuarded, "AADJ-1", []domain.DepartureGateBasis{{Action: "X"}})
		},
		"来源含 NUL": func() (domain.DepartureGateFinding, error) {
			return domain.NewDepartureGateFinding(object, domain.DepartureGateBlocked, "AADJ-1",
				[]domain.DepartureGateBasis{{Cause: "RESTRICTION_IN_FORCE", Sources: []string{"RST-\x00"}}})
		},
		"答复越界": func() (domain.DepartureGateFinding, error) {
			return domain.NewDepartureGateFinding(object, domain.DepartureGateAnswerInvalid, "AADJ-1", basis)
		},
	} {
		if _, err := try(); !errors.Is(err, domain.ErrInvalidDepartureGateReading) {
			t.Errorf("%s：err = %v, want ErrInvalidDepartureGateReading", name, err)
		}
	}
	finding, err := domain.NewDepartureGateFinding(object, domain.DepartureGateNotGuarded, "AADJ-1", basis)
	if err != nil {
		t.Fatalf("成形的那一格：%v", err)
	}
	if judgement, judged := finding.Judgement(); !judged || judgement != "AADJ-1" {
		t.Fatalf("标识 = %q judged=%v", judgement, judged)
	}
}

// Covers: AT-TF-115 领域半边——对象取出发那一刻段里在控的参与：链尾那一版（未被替代、未失效），出发时点在它的控制起止之内；
// 出发前已离场的、出发后才入场的都不问；不按此刻，迟到的出发也问得到随车出发的那几件。
func TestObjectsUnderControlAreTakenAtTheDepartureInstant(t *testing.T) {
	departure := time.Date(2026, 9, 24, 14, 0, 0, 0, time.UTC)
	basis := func(value string) domain.ParticipationBasisReference {
		return mustValue(t, domain.NewParticipationBasisReference, value)
	}
	object := func(value string) domain.CarriedObjectReference {
		return mustValue(t, domain.NewCarriedObjectReference, value)
	}
	entered := func(name string, at time.Time) domain.RehydrateParticipationSpec {
		return domain.RehydrateParticipationSpec{
			Object: object(name), EntryKind: domain.EnteredByTransportHandover, EntryBasis: basis("HO/" + name), EnteredAt: at,
		}
	}
	endedAt := func(spec domain.RehydrateParticipationSpec, at time.Time) domain.RehydrateParticipationSpec {
		spec.EndKind, spec.EndBasis, spec.EndedAt = domain.EndedByNextHandover, basis("HO-NEXT/"+spec.Object.String()), at
		return spec
	}
	superseding := entered("parcel-corrected", departure.Add(-2*time.Hour))
	superseding.EntryBasis, superseding.Supersedes = basis("HO/parcel-corrected/v2"), basis("HO/parcel-corrected")
	voiding := entered("parcel-voided", departure.Add(-2*time.Hour))
	voiding.EntryBasis, voiding.Supersedes, voiding.Voided = basis("HO/parcel-voided/v2"), basis("HO/parcel-voided"), true

	segment, err := domain.RehydrateActualFulfillmentSegment(domain.RehydrateActualFulfillmentSegmentSpec{
		TenantID: mustValue(t, domain.NewTenantID, "tenant-1"),
		Segment:  mustValue(t, domain.NewFulfillmentSegmentReference, "segment-1"),
		Participations: []domain.RehydrateParticipationSpec{
			entered("parcel-on-board", departure.Add(-time.Hour)),
			endedAt(entered("parcel-arrived-later", departure.Add(-time.Hour)), departure.Add(6*time.Hour)),
			endedAt(entered("parcel-left-before", departure.Add(-3*time.Hour)), departure.Add(-time.Minute)),
			entered("parcel-joined-after", departure.Add(time.Minute)),
			entered("parcel-corrected", departure.Add(-3*time.Hour)),
			superseding,
			entered("parcel-voided", departure.Add(-3*time.Hour)),
			voiding,
		},
	})
	if err != nil {
		t.Fatalf("重建段：%v", err)
	}

	var got []string
	for _, controlled := range segment.ObjectsUnderControlAt(departure) {
		got = append(got, controlled.String())
	}
	want := []string{"parcel-arrived-later", "parcel-corrected", "parcel-on-board"}
	if !slices.Equal(got, want) {
		t.Fatalf("出发那一刻在控 = %v, want %v", got, want)
	}
}
