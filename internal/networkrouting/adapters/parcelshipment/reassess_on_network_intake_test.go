package parcelshipment_test

import (
	"context"
	"errors"
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
	handler := nrapplication.NewReassessRouteHandler(nrapplication.ReassessRouteDeps{
		Routes: &routeStoreDouble{records: map[nrdomain.InitialRouteJudgmentKey]nrports.InitialRouteRecord{
			reassessKey(t): planOnFile(t),
		}},
		Evidence: evidenceDouble{evidence: nrports.InitialRouteEvidence{
			Strategy:     value(t, nrdomain.NewRouteStrategyReference, "strategy-1/v1"),
			ViewRevision: value(t, nrdomain.NewNetworkViewRevision, "net-view-rev-1"),
		}},
		Applicability: &applicabilityStoreDouble{byPlan: map[nrdomain.RoutePlanVersionID]nrdomain.PlanApplicability{}},
		Store:         &reassessStoreDouble{byCorrelation: map[nrdomain.RequestCorrelationID]nrports.ReassessmentRecord{}},
		Log:           &logDouble{digests: map[nrdomain.RequestCorrelationID]string{}},
		Identities:    &identityDouble{},
		Clock:         fixedClock{at: intakeAt.Add(2 * time.Minute)},
	})
	return adapter.NewReassessOnIntakeAdapter(handler, value(t, nrdomain.NewServicePurpose, "NETWORK_SERVICE"))
}

// reassessorWithoutHistory 造一个查不到路由历史的复核编排：复核停在`未决`
// （NO_ROUTING_HISTORY），这是消费门那一格「回滚重投」的真实来源。
func reassessorWithoutHistory(t *testing.T) *adapter.ReassessOnIntakeAdapter {
	t.Helper()
	handler := nrapplication.NewReassessRouteHandler(nrapplication.ReassessRouteDeps{
		Routes:        &routeStoreDouble{records: map[nrdomain.InitialRouteJudgmentKey]nrports.InitialRouteRecord{}},
		Evidence:      evidenceDouble{},
		Applicability: &applicabilityStoreDouble{byPlan: map[nrdomain.RoutePlanVersionID]nrdomain.PlanApplicability{}},
		Store:         &reassessStoreDouble{byCorrelation: map[nrdomain.RequestCorrelationID]nrports.ReassessmentRecord{}},
		Log:           &logDouble{digests: map[nrdomain.RequestCorrelationID]string{}},
		Identities:    &identityDouble{},
		Clock:         fixedClock{at: intakeAt},
	})
	return adapter.NewReassessOnIntakeAdapter(handler, value(t, nrdomain.NewServicePurpose, "NETWORK_SERVICE"))
}

func subjectFor(
	t *testing.T,
	source *adoptionSourceDouble,
	reassess *adapter.ReassessOnIntakeAdapter,
) *adapter.ReassessOnNetworkIntakeAdapter {
	t.Helper()
	subject, err := adapter.NewReassessOnNetworkIntakeAdapter(source, reassess)
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
	subject := subjectFor(t, sourceHolding(refused), reassessorWithHistory(t))

	if err := subject.HandleAdoptedNetworkIntake(context.Background(), envelopeFor(t)); err != nil {
		t.Fatalf("`不采用`应入账收工，而不是回滚重投：%v", err)
	}
}

// Covers: 复核未决即回滚重投 — 未决是「依赖还答不出」，重投会改变结果；就此入账等于
// 把复核义务永久丢掉。折叠规则与接受决定那条线一致：按「重投会不会改变结果」分。
func TestAnUndecidedReassessmentRollsBackForRedelivery(t *testing.T) {
	subject := subjectFor(t, sourceHolding(adoptedRecord(t)), reassessorWithoutHistory(t))

	err := subject.HandleAdoptedNetworkIntake(context.Background(), envelopeFor(t))
	if !errors.Is(err, adapter.ErrReassessmentUndecided) {
		t.Fatalf("err = %v, want ErrReassessmentUndecided", err)
	}
}
