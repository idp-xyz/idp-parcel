package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	tfhttp "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/http"
	tfpg "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/postgres"
	tfapp "go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
)

// 票 tf-face-algebra/05 的真库并发回归：明确终止与一次仍为已交接的交接更正同时落在同一条参与上，终点不得丢在
// 被替代的旧链尾上。三种交错各用钩子确定地摆出来——一方在自己的事务里挂住，另一方在另一条连接上走生产编排；
// 要等对方卡在行锁上时，先看见它在等锁再放行，没卡住（修复前）就等它自己走完。

// lockWaitDeadline 是一方等另一方的上限：超过即判死锁或卡死，用例失败而不是挂到 go test 的总超时。
const lockWaitDeadline = 10 * time.Second

// awaitLockWaitOrDone 等到另一条连接上有后端在等锁，或另一方已经走完（没被挡住），并在日志里记下是哪一种——
// 修好之后应当是前者，竞争路径才算走到。两者都不发生即失败。
//
// 观察走一条池外的独立连接：测试池只有两条连接，挂住的一方与等锁的一方各占一条，从池里再取就会排在它们后面。
func (channel terminationChannel) awaitLockWaitOrDone(t *testing.T, done <-chan struct{}) {
	t.Helper()
	observer, err := pgx.ConnectConfig(t.Context(), channel.pool.Config().ConnConfig.Copy())
	if err != nil {
		t.Errorf("开观察连接：%v", err)
		return
	}
	defer observer.Close(context.Background())
	deadline := time.Now().Add(lockWaitDeadline)
	for time.Now().Before(deadline) {
		select {
		case <-done:
			t.Logf("另一方没被挡住，已走完")
			return
		default:
		}
		var waiting int
		if err := observer.QueryRow(t.Context(),
			`SELECT count(*) FROM pg_stat_activity WHERE datname = current_database() AND wait_event_type = 'Lock'`,
		).Scan(&waiting); err != nil {
			t.Errorf("查等锁：%v", err)
			return
		}
		if waiting > 0 {
			t.Logf("另一方卡在行锁上，放行")
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Errorf("另一方既没走完也没在等锁")
}

func awaitDone(t *testing.T, done <-chan struct{}) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(lockWaitDeadline):
		t.Fatal("另一方没在时限内走完：死锁或卡死")
	}
}

// inBackground 在另一条 goroutine（因而另一条连接、另一笔事务）里投一次请求，走完时关 done。
func (channel terminationChannel) inBackground(endpoint http.Handler, path, body string) (*httptest.ResponseRecorder, <-chan struct{}) {
	response := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		defer close(done)
		request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
		stampWorkFact(request, "OFFSITE", "SYN-OFFSITE-01", "SYN-DEVICE-OFFSITE")
		endpoint.ServeHTTP(response, request)
	}()
	return response, done
}

// racingCorrections 与生产装配同形（同一个事务包装、同一组依赖），只把段登记册换成带钩子的那一只。
func (channel terminationChannel) racingCorrections(t *testing.T, segments *racingSegments) http.Handler {
	t.Helper()
	clock := systemClock{}
	handovers, err := tfpg.NewTransportHandovers(channel.db)
	if err != nil {
		t.Fatal(err)
	}
	judgments, err := tfpg.NewActualCarrierJudgments(channel.db)
	if err != nil {
		t.Fatal(err)
	}
	store, err := outbox.NewStore(channel.db)
	if err != nil {
		t.Fatal(err)
	}
	downstream, err := tfpg.NewOutboxTransportHandoverRegistrationHandoff(channel.db, store, clock)
	if err != nil {
		t.Fatal(err)
	}
	participationEnds, err := buildParticipationEnder(channel.db, clock)
	if err != nil {
		t.Fatal(err)
	}
	handler := transactionalHandover{
		transactor: channel.db.Transactor(),
		inner: tfapp.NewRegisterTransportHandoverHandler(tfapp.RegisterTransportHandoverDeps{
			Handovers:         handovers,
			Segments:          segments,
			Judgments:         judgments,
			Downstream:        downstream,
			Clock:             clock,
			ParticipationEnds: participationEnds,
		}),
	}
	return tfhttp.NewCorrectTransportHandoverEndpoint(workFactTransportForTest(t), handler)
}

// handoverRaceSegment 立一个凭交接 v1 进段的对象，交回段、对象与范围。
func (channel terminationChannel) handoverRaceSegment(t *testing.T, suffix string) (string, string, string) {
	t.Helper()
	segment, object, scope := "SYN-SEGMENT-TFA05-"+suffix, "SYN-PARCEL-TFA05-"+suffix, "SYN-SCOPE/hub-tfa05-"+suffix
	channel.mustPost(t, channel.handovers, "/transport-fulfillment/handovers",
		handedOverBody(object, scope, "v1", "2026-09-24T09:00:00+08:00", segment), http.StatusCreated)
	return segment, object, scope
}

// assertTheEndSurvived 是三种交错共同的判据：终止答了 201，则链尾（不论是不是更正长出的那一版）带着这次终止的
// 终点；此后同一终止重放答已离场且不再写库。
func (channel terminationChannel) assertTheEndSurvived(t *testing.T, segment, object, basis string, terminated *httptest.ResponseRecorder) {
	t.Helper()
	rows := channel.participationRows(t, segment, object)
	if len(rows) < 2 {
		t.Fatalf("更正没在段上长出替代版本：%+v", rows)
	}
	tail := channel.chainTail(t, segment, object)
	if terminated.Code == http.StatusCreated && (tail.endedAt == nil || tail.endBasis != basis) {
		t.Fatalf("终止答 201，链尾却没有它的终点：%+v", rows)
	}

	before := channel.fingerprint(t)
	replay := channel.post(channel.terminate, terminationPath, terminationRequestBody(segment, object, basis, "2026-09-24T12:00:00+08:00"))
	if replay.Code != http.StatusOK || decodeTerminationAnswer(t, replay).Outcome != "PARTICIPATION_ALREADY_ENDED" {
		t.Fatalf("同一终止重放：status = %d body = %s, want 200 已离场；库面 %+v", replay.Code, replay.Body.String(), channel.participationRows(t, segment, object))
	}
	if changed := changedSince(before, channel.fingerprint(t)); len(changed) != 0 {
		t.Fatalf("重放动了库：%v", changed)
	}
}

// 交错一（票面现象）：终止的窄口写已执行、尚未提交，交接更正此时到来。更正要么排在终止之后按它的终点派生，
// 要么终止不答 201。
func TestAHandoverCorrectionArrivingInsideAnOpenTerminationKeepsTheEnd(t *testing.T) {
	channel := newTerminationChannel(t)
	segment, object, scope := channel.handoverRaceSegment(t, "A")
	const basis = "SYN-DISPOSITION/tfa05-a"

	var corrected *httptest.ResponseRecorder
	var correctionDone <-chan struct{}
	segments := channel.racingSegments(t)
	segments.afterEnd = func() {
		corrected, correctionDone = channel.inBackground(channel.corrections, "/transport-fulfillment/handover-corrections",
			stillHandedOverCorrectionBody(object, scope, "v1", "v2", "2026-09-24T09:30:00+08:00"))
		channel.awaitLockWaitOrDone(t, correctionDone)
	}
	terminated := channel.post(channel.racingTerminate(segments), terminationPath,
		terminationRequestBody(segment, object, basis, "2026-09-24T12:00:00+08:00"))
	awaitDone(t, correctionDone)
	if corrected.Code != http.StatusCreated {
		t.Fatalf("交接更正：status = %d body = %s", corrected.Code, corrected.Body.String())
	}
	channel.assertTheEndSurvived(t, segment, object, basis, terminated)
}

// 交错二（反序）：更正先读完段、派生好替代版本，终止此时到来并提交，更正再落替代版本。
func TestATerminationCommittedBetweenACorrectionsReadAndItsSupersedeIsKept(t *testing.T) {
	channel := newTerminationChannel(t)
	segment, object, scope := channel.handoverRaceSegment(t, "B")
	const basis = "SYN-DISPOSITION/tfa05-b"

	var terminated *httptest.ResponseRecorder
	segments := channel.racingSegments(t)
	segments.beforeSupersede = func() {
		var done <-chan struct{}
		terminated, done = channel.inBackground(channel.terminate, terminationPath,
			terminationRequestBody(segment, object, basis, "2026-09-24T12:00:00+08:00"))
		awaitDone(t, done)
	}
	corrected := channel.post(channel.racingCorrections(t, segments), "/transport-fulfillment/handover-corrections",
		stillHandedOverCorrectionBody(object, scope, "v1", "v2", "2026-09-24T09:30:00+08:00"))
	if corrected.Code != http.StatusCreated {
		t.Fatalf("交接更正：status = %d body = %s", corrected.Code, corrected.Body.String())
	}
	channel.assertTheEndSurvived(t, segment, object, basis, terminated)
}

// 交错三（反序）：更正已落替代版本、尚未提交，终止此时到来。终止要么等更正提交后结束新链尾，要么不答 201。
func TestATerminationArrivingWhileACorrectionHoldsTheTailKeepsTheEnd(t *testing.T) {
	channel := newTerminationChannel(t)
	segment, object, scope := channel.handoverRaceSegment(t, "C")
	const basis = "SYN-DISPOSITION/tfa05-c"

	var terminated *httptest.ResponseRecorder
	var terminationDone <-chan struct{}
	segments := channel.racingSegments(t)
	segments.afterSupersede = func() {
		terminated, terminationDone = channel.inBackground(channel.terminate, terminationPath,
			terminationRequestBody(segment, object, basis, "2026-09-24T12:00:00+08:00"))
		channel.awaitLockWaitOrDone(t, terminationDone)
	}
	corrected := channel.post(channel.racingCorrections(t, segments), "/transport-fulfillment/handover-corrections",
		stillHandedOverCorrectionBody(object, scope, "v1", "v2", "2026-09-24T09:30:00+08:00"))
	awaitDone(t, terminationDone)
	if corrected.Code != http.StatusCreated {
		t.Fatalf("交接更正：status = %d body = %s", corrected.Code, corrected.Body.String())
	}
	channel.assertTheEndSurvived(t, segment, object, basis, terminated)
}
