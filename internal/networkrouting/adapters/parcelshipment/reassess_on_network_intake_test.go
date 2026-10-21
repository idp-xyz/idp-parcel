package parcelshipment_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	nrinbox "go.idp.xyz/idp-parcel/internal/networkrouting/adapters/inbox"
	adapter "go.idp.xyz/idp-parcel/internal/networkrouting/adapters/parcelshipment"
	nrapplication "go.idp.xyz/idp-parcel/internal/networkrouting/application"
	nrdomain "go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	nrports "go.idp.xyz/idp-parcel/internal/networkrouting/ports"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	psports "go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// adoptionSourceDouble 顶 PS 的采用仓储：信封只带四维键，记录本体按键重新取。
type adoptionSourceDouble struct {
	records map[psports.IntakeAdoptionKey]psports.IntakeAdoptionRecord
	asked   []psports.IntakeAdoptionKey
	err     error
}

func (double *adoptionSourceDouble) FindByKey(
	_ context.Context,
	key psports.IntakeAdoptionKey,
) (psports.IntakeAdoptionRecord, bool, error) {
	double.asked = append(double.asked, key)
	if double.err != nil {
		return psports.IntakeAdoptionRecord{}, false, double.err
	}
	record, found := double.records[key]
	return record, found, nil
}

// envelopeFor 造出与 adoptedRecord 同键的信封引用（PS 侧 outbox 载荷就是这四维）。
func envelopeFor(t *testing.T) nrinbox.AdoptedNetworkIntake {
	t.Helper()
	return nrinbox.AdoptedNetworkIntake{
		TenantID: "tenant-1",
		Parcel:   "parcel-1",
		Kind:     psdomain.NodeIntakeSource.String(),
		Version:  "intake-result/v1",
	}
}

// sourceHolding 把一份采用记录放进仓储替身。
func sourceHolding(record psports.IntakeAdoptionRecord) *adoptionSourceDouble {
	return &adoptionSourceDouble{
		records: map[psports.IntakeAdoptionKey]psports.IntakeAdoptionRecord{record.Key: record},
	}
}

// reassessorWithHistory 造一个真复核编排：路由历史里放着一份计划，收寄点就在计划首
// 节点上，因此复核答`仍适用`。
func reassessorWithHistory(t *testing.T) *adapter.ReassessOnIntakeAdapter {
	t.Helper()
	return reassessorSeeing(t, nil)
}

// reassessorSeeing 同 reassessorWithHistory，另把复核取证据时交进来的携带内容记进 seen（非空时）。
func reassessorSeeing(t *testing.T, seen *nrports.RequestCarriedContent) *adapter.ReassessOnIntakeAdapter {
	t.Helper()
	handler := nrapplication.NewReassessRouteHandler(nrapplication.ReassessRouteDeps{
		Routes: &routeStoreDouble{records: map[nrdomain.InitialRouteJudgmentKey]nrports.InitialRouteRecord{
			reassessKey(t): planOnFile(t),
		}},
		Evidence:      evidenceDouble{evidence: evidenceWithOpenFreeze(t), seen: seen},
		Applicability: &applicabilityStoreDouble{byPlan: map[nrdomain.RoutePlanVersionID]nrdomain.PlanApplicability{}},
		Store:         &reassessStoreDouble{byCorrelation: map[nrdomain.RequestCorrelationID]nrports.ReassessmentRecord{}},
		Log:           &logDouble{digests: map[nrdomain.RequestCorrelationID]string{}},
		Identities:    &identityDouble{},
		Clock:         fixedClock{at: intakeAt.Add(2 * time.Minute)},
	})
	return adapter.NewReassessOnIntakeAdapter(handler, value(t, nrdomain.NewServicePurpose, "NETWORK_SERVICE"))
}

// reassessorWithoutHistory 造一个查不到路由历史的复核编排：复核答`未判`（NO_ROUTING_HISTORY），原因落进 store。
func reassessorWithoutHistory(t *testing.T, store *reassessStoreDouble) *adapter.ReassessOnIntakeAdapter {
	t.Helper()
	handler := nrapplication.NewReassessRouteHandler(nrapplication.ReassessRouteDeps{
		Routes:        &routeStoreDouble{records: map[nrdomain.InitialRouteJudgmentKey]nrports.InitialRouteRecord{}},
		Evidence:      evidenceDouble{},
		Applicability: &applicabilityStoreDouble{byPlan: map[nrdomain.RoutePlanVersionID]nrdomain.PlanApplicability{}},
		Store:         store,
		Log:           &logDouble{digests: map[nrdomain.RequestCorrelationID]string{}},
		Identities:    &identityDouble{},
		Clock:         fixedClock{at: intakeAt},
	})
	return adapter.NewReassessOnIntakeAdapter(handler, value(t, nrdomain.NewServicePurpose, "NETWORK_SERVICE"))
}

// reassessorWithUnreadableEvidence 造一个取证据时依赖故障的复核编排：复核停在`未决`（REASSESS_EVIDENCE_UNAVAILABLE），
// 这是消费门那一格「回滚重投」的真实来源。
func reassessorWithUnreadableEvidence(t *testing.T) *adapter.ReassessOnIntakeAdapter {
	t.Helper()
	handler := nrapplication.NewReassessRouteHandler(nrapplication.ReassessRouteDeps{
		Routes: &routeStoreDouble{records: map[nrdomain.InitialRouteJudgmentKey]nrports.InitialRouteRecord{
			reassessKey(t): planOnFile(t),
		}},
		Evidence:      evidenceDouble{err: errors.New("catalog read timed out")},
		Applicability: &applicabilityStoreDouble{byPlan: map[nrdomain.RoutePlanVersionID]nrdomain.PlanApplicability{}},
		Store:         &reassessStoreDouble{byCorrelation: map[nrdomain.RequestCorrelationID]nrports.ReassessmentRecord{}},
		Log:           &logDouble{digests: map[nrdomain.RequestCorrelationID]string{}},
		Identities:    &identityDouble{},
		Clock:         fixedClock{at: intakeAt},
	})
	return adapter.NewReassessOnIntakeAdapter(handler, value(t, nrdomain.NewServicePurpose, "NETWORK_SERVICE"))
}

// acceptedParcelDouble 顶 PS 已接受委托的读口：交回判断键各维与委托接受的时刻。
type acceptedParcelDouble struct {
	accepted adapter.AcceptedCarriedGeo
	found    bool
	err      error
	asked    int
}

func (double *acceptedParcelDouble) LoadAcceptedCarriedGeo(
	_ context.Context, _ nrdomain.TenantID, _ nrdomain.DeclaredParcelID,
) (adapter.AcceptedCarriedGeo, bool, error) {
	double.asked++
	return double.accepted, double.found, double.err
}

// acceptedParcelAt 是 adoptedRecord 那件委托的已接受读回，接受时刻取 at。
func acceptedParcelAt(t *testing.T, at time.Time) *acceptedParcelDouble {
	t.Helper()
	return &acceptedParcelDouble{found: true, accepted: adapter.AcceptedCarriedGeo{
		CustomerAccountID:  value(t, nrdomain.NewCustomerAccountID, "customer-1"),
		ShipmentRequestID:  value(t, nrdomain.NewShipmentRequestID, "request-1"),
		AcceptanceBaseline: value(t, nrdomain.NewAcceptanceBaselineReference, "version-1"),
		AcceptedAt:         at,
	}}
}

func subjectFor(
	t *testing.T,
	source *adoptionSourceDouble,
	reassess *adapter.ReassessOnIntakeAdapter,
) *adapter.ReassessOnNetworkIntakeAdapter {
	t.Helper()
	return subjectAccepting(t, source, acceptedParcelAt(t, intakeAcceptedAt), reassess)
}

func subjectAccepting(
	t *testing.T,
	source *adoptionSourceDouble,
	accepted *acceptedParcelDouble,
	reassess *adapter.ReassessOnIntakeAdapter,
) *adapter.ReassessOnNetworkIntakeAdapter {
	t.Helper()
	subject, err := adapter.NewReassessOnNetworkIntakeAdapter(source, accepted, reassess)
	if err != nil {
		t.Fatalf("construct adapter: %v", err)
	}
	return subject
}

// Covers: 「信封只传引用，消费方按引用重新取」 — 四维译回采用键、整行取回记录、走完
// 真实复核编排，结论落定即入账（nil）。同时钉住问的是哪把键：拿错键会取回另一份事实，
// 而那份事实照样能跑完复核，错得毫无征兆。
func TestAnAdoptionEnvelopeIsLookedUpByItsKeyAndReassessed(t *testing.T) {
	record := adoptedRecord(t)
	source := sourceHolding(record)
	subject := subjectFor(t, source, reassessorWithHistory(t))

	if err := subject.HandleAdoptedNetworkIntake(context.Background(), envelopeFor(t)); err != nil {
		t.Fatalf("handle adopted network intake: %v", err)
	}

	if len(source.asked) != 1 {
		t.Fatalf("取回次数 = %d, want 1", len(source.asked))
	}
	if source.asked[0] != record.Key {
		t.Fatalf("取回用的键 = %+v, want %+v", source.asked[0], record.Key)
	}
}

// Covers: 信封上的地理投影一路到复核取证据那一步（ADR-0075）：信封一跳丢了它，复核问关务就只剩两端未知。
func TestTheEnvelopesGeoProjectionReachesTheReassessmentEvidence(t *testing.T) {
	var seen nrports.RequestCarriedContent
	subject := subjectFor(t, sourceHolding(adoptedRecord(t)), reassessorSeeing(t, &seen))

	intake := envelopeFor(t)
	intake.Geo = nrinbox.CarriedGeo{
		Carried: true, SenderCountry: "CN", SenderCountryPresent: true,
		DeliveryCountry: "SG", DeliveryCountryPresent: true,
	}
	if err := subject.HandleAdoptedNetworkIntake(context.Background(), intake); err != nil {
		t.Fatalf("handle adopted network intake: %v", err)
	}
	want := nrdomain.NewGeoResolutionProjection(
		nrdomain.NewGeoResolutionSide("CN", true, "", false),
		nrdomain.NewGeoResolutionSide("SG", true, "", false),
	)
	if seen.Geo.ContentDigest() != want.ContentDigest() {
		t.Fatalf("复核取证据时的投影 = %q，want 信封带来的 %q", seen.Geo.ContentDigest(), want.ContentDigest())
	}
}

// Covers: 采用记录不可见即回滚重投 — 采用结果与它的意图在 PS 侧同一事务落库，读不着
// 通常是可见性滞后。当成终局入账会把这次复核永久丢掉，而复核义务没有别的东西会来补。
func TestAnInvisibleAdoptionRollsBackForRedelivery(t *testing.T) {
	subject := subjectFor(t, &adoptionSourceDouble{}, reassessorWithHistory(t))

	err := subject.HandleAdoptedNetworkIntake(context.Background(), envelopeFor(t))
	if !errors.Is(err, adapter.ErrEnvelopeContradictsAuthority) {
		t.Fatalf("err = %v, want ErrEnvelopeContradictsAuthority", err)
	}
}

// Covers: ADR-0025 「逐格翻译，default 报错不吸收」 — 来源类型是封闭集合，认不得的
// 那一格响亮报错且不去取记录。吸收成某个缺省会让控制依据译错格（节点收寄 vs 权威运输
// 交接），而那两格在 NR 侧是不同的控制证据。
func TestAnUnknownSourceKindIsLoudAndNeverQueries(t *testing.T) {
	source := sourceHolding(adoptedRecord(t))
	subject := subjectFor(t, source, reassessorWithHistory(t))

	intake := envelopeFor(t)
	intake.Kind = "SOMETHING_ELSE"

	err := subject.HandleAdoptedNetworkIntake(context.Background(), intake)
	if !errors.Is(err, adapter.ErrUntranslatableAnswer) {
		t.Fatalf("err = %v, want ErrUntranslatableAnswer", err)
	}
	if len(source.asked) != 0 {
		t.Fatal("译不出键就不该去取记录")
	}
}

// Covers: 不采用是终局业务答案，入账不重投 — 没有责任起点成立，路由没有要复核的收寄
// 事实。让它回滚重投会把一份合法的`不采用`永远卡在投递队列里。
func TestARefusedAdoptionIsRecordedNotRetried(t *testing.T) {
	refused := adoptedRecord(t)
	refused.Adopted = false
	accepted := acceptedParcelAt(t, intakeAcceptedAt)
	subject := subjectAccepting(t, sourceHolding(refused), accepted, reassessorWithHistory(t))

	if err := subject.HandleAdoptedNetworkIntake(context.Background(), envelopeFor(t)); err != nil {
		t.Fatalf("`不采用`应入账收工，而不是回滚重投：%v", err)
	}
	if accepted.asked != 0 {
		t.Fatalf("读已接受委托 %d 次, want 0——不采用没有要复核的收寄事实", accepted.asked)
	}
}

// Covers: 复核未决即回滚重投 — 未决是「依赖还答不出」，重投会改变结果；就此入账等于
// 把复核义务永久丢掉。折叠规则与接受决定那条线一致：按「重投会不会改变结果」分。
func TestAnUndecidedReassessmentRollsBackForRedelivery(t *testing.T) {
	subject := subjectFor(t, sourceHolding(adoptedRecord(t)), reassessorWithUnreadableEvidence(t))

	err := subject.HandleAdoptedNetworkIntake(context.Background(), envelopeFor(t))
	if !errors.Is(err, adapter.ErrReassessmentUndecided) {
		t.Fatalf("err = %v, want ErrReassessmentUndecided", err)
	}
}

// Covers: AT-NR-074（收寄采用一路）——没有路由历史：重投推不动，入账（交回 nil，不登本路未决哨兵），原因落未判登记。
// 与交接一路同一份判据，判据只在复核编排里。
func TestAReassessmentWithoutRoutingHistorySettlesWithItsReason(t *testing.T) {
	store := &reassessStoreDouble{byCorrelation: map[nrdomain.RequestCorrelationID]nrports.ReassessmentRecord{}}
	subject := subjectFor(t, sourceHolding(adoptedRecord(t)), reassessorWithoutHistory(t, store))

	if err := subject.HandleAdoptedNetworkIntake(context.Background(), envelopeFor(t)); err != nil {
		t.Fatalf("没有路由历史应入账，而不是回滚重投：%v", err)
	}
	if len(store.byCorrelation) != 0 || len(store.notJudged) != 1 {
		t.Fatalf("复核 %d 条、未判 %d 行, want 0 与 1", len(store.byCorrelation), len(store.notJudged))
	}
	for _, noted := range store.notJudged {
		if noted.Reason != nrports.NotJudgedNoRoutingHistory {
			t.Fatalf("未判原因 = %s, want NO_ROUTING_HISTORY", noted.Reason)
		}
	}
}

// Covers: AT-NR-087（收寄采用一路）——收寄的业务时间早于委托接受：采用信封不带接受时刻，本层按（租户，包裹）读回那份已接受
// 委托的接受时刻交给复核；复核入账为「触发早于委托接受」，交回 nil，不登本路未决哨兵，原因落未判登记。
func TestAnIntakeEarlierThanTheAcceptanceSettlesNotJudged(t *testing.T) {
	store := &reassessStoreDouble{byCorrelation: map[nrdomain.RequestCorrelationID]nrports.ReassessmentRecord{}}
	handler := nrapplication.NewReassessRouteHandler(nrapplication.ReassessRouteDeps{
		Routes: &routeStoreDouble{records: map[nrdomain.InitialRouteJudgmentKey]nrports.InitialRouteRecord{
			reassessKey(t): planOnFile(t),
		}},
		Evidence:      evidenceDouble{evidence: evidenceWithOpenFreeze(t)},
		Applicability: &applicabilityStoreDouble{byPlan: map[nrdomain.RoutePlanVersionID]nrdomain.PlanApplicability{}},
		Store:         store,
		Log:           &logDouble{digests: map[nrdomain.RequestCorrelationID]string{}},
		Identities:    &identityDouble{},
		Clock:         fixedClock{at: intakeAt.Add(3 * time.Hour)},
	})
	accepted := acceptedParcelAt(t, intakeAt.Add(time.Hour))
	subject := subjectAccepting(t, sourceHolding(adoptedRecord(t)), accepted,
		adapter.NewReassessOnIntakeAdapter(handler, value(t, nrdomain.NewServicePurpose, "NETWORK_SERVICE")))

	if err := subject.HandleAdoptedNetworkIntake(context.Background(), envelopeFor(t)); err != nil {
		t.Fatalf("早于委托接受应入账，而不是回滚重投：%v", err)
	}
	if len(store.byCorrelation) != 0 || len(store.notJudged) != 1 {
		t.Fatalf("复核 %d 条、未判 %d 行, want 0 与 1", len(store.byCorrelation), len(store.notJudged))
	}
	for _, noted := range store.notJudged {
		if noted.Reason != nrports.NotJudgedTriggerPrecedesShipmentAcceptance ||
			!noted.ShipmentAcceptedAt.Equal(intakeAt.Add(time.Hour)) || !noted.TriggerOccurredAt.Equal(intakeAt) {
			t.Fatalf("未判 = %+v, want 早于委托接受 %s", noted, intakeAt.Add(time.Hour))
		}
	}
}

// Covers: 读已接受委托的三格——读不动交 ErrAcceptedParcelUnreadable（本路未决哨兵，回滚重投）；读不着，或读到的委托、
// 基线与采用记录对不上，是不变量破了，交 ErrAcceptedRequestInconsistent（不进哨兵）。三格都不走到复核。
func TestTheAcceptanceReadOfAnAdoptedIntakeIsSplitByRecovery(t *testing.T) {
	mismatched := func(mutate func(*adapter.AcceptedCarriedGeo)) *acceptedParcelDouble {
		double := acceptedParcelAt(t, intakeAcceptedAt)
		mutate(&double.accepted)
		return double
	}
	for name, tc := range map[string]struct {
		accepted *acceptedParcelDouble
		want     error
	}{
		"读不动": {&acceptedParcelDouble{err: fmt.Errorf("%w: pool closed", adapter.ErrAcceptedParcelUnreadable)},
			adapter.ErrAcceptedParcelUnreadable},
		"读不着": {&acceptedParcelDouble{}, adapter.ErrAcceptedRequestInconsistent},
		"另一份委托": {mismatched(func(geo *adapter.AcceptedCarriedGeo) {
			geo.ShipmentRequestID = value(t, nrdomain.NewShipmentRequestID, "request-2")
		}), adapter.ErrAcceptedRequestInconsistent},
		"另一条基线": {mismatched(func(geo *adapter.AcceptedCarriedGeo) {
			geo.AcceptanceBaseline = value(t, nrdomain.NewAcceptanceBaselineReference, "version-2")
		}), adapter.ErrAcceptedRequestInconsistent},
	} {
		t.Run(name, func(t *testing.T) {
			store := &reassessStoreDouble{byCorrelation: map[nrdomain.RequestCorrelationID]nrports.ReassessmentRecord{}}
			subject := subjectAccepting(t, sourceHolding(adoptedRecord(t)), tc.accepted, reassessorWithoutHistory(t, store))
			err := subject.HandleAdoptedNetworkIntake(context.Background(), envelopeFor(t))
			if !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
			if retryable := errors.Is(err, adapter.ErrAcceptedParcelUnreadable); retryable != (tc.want == adapter.ErrAcceptedParcelUnreadable) {
				t.Fatalf("err = %v：可重投 %v，与格子不符", err, retryable)
			}
			if len(store.notJudged) != 0 || len(store.byCorrelation) != 0 {
				t.Fatal("读不回已接受委托就不该走到复核")
			}
		})
	}
}
