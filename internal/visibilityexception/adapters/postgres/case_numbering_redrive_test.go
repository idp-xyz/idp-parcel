package postgres_test

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/platform/docnumber"
	adapter "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/application"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/ports"
)

// 本文件对真实 PostgreSQL 16 证案件撞号册的重驱在并发下只立一件案（AT-VE-228）：两个派发进程同时重驱同一行、重驱与同一原因的
// 正常命中交叠，两种次序都只落一个发作期、一件案件，撞号册清空；同一原因撞两次，重驱按首次命中开期。分诊用真编排、真
// 发作期库、真取号与真撞号册，分诊规则、标识签发与意图交出用替身。测试输入是隔离合成，只记 S。

type autoEstablishTriage struct{}

func (autoEstablishTriage) TriageSignal(context.Context, ports.TriageQuery) (ports.TriageAnswer, bool, error) {
	rule, err := domain.NewSignalRuleVersionReference("SYN-TRIAGE/v1")
	if err != nil {
		return ports.TriageAnswer{}, false, err
	}
	team, err := domain.NewResponsibleTeamReference("SYN-TEAM-EXC")
	if err != nil {
		return ports.TriageAnswer{}, false, err
	}
	return ports.TriageAnswer{Outcome: domain.AutoEstablishCase, Rule: rule, Team: team}, true, nil
}

type sequentialIdentities struct{ next atomic.Int64 }

func (ids *sequentialIdentities) NextEpisodeID(context.Context) (domain.EpisodeID, error) {
	return domain.NewEpisodeID(fmt.Sprintf("SYN-EP-%d", ids.next.Add(1)))
}

func (ids *sequentialIdentities) NextCaseID(context.Context) (domain.CaseID, error) {
	return domain.NewCaseID(fmt.Sprintf("SYN-CASE-%d", ids.next.Add(1)))
}

type quietTriageHandoff struct{}

func (quietTriageHandoff) HandOffTriage(context.Context, ports.TriageHandoffIntent) error { return nil }

type redriveRig struct {
	*numberingFixture
	raiser  *application.RaiseSignalHandler
	redrive *application.CaseNumberingRedrive
}

const redriveTenant = "SYN-TENANT-01"

func newRedriveRig(t *testing.T) *redriveRig {
	t.Helper()
	fixture := newNumberingFixture(t)
	fixture.registerTimeZone(t, redriveTenant)
	var registerErr error
	_ = fixture.transactor.WithinTransaction(t.Context(), func(ctx context.Context) error {
		_, registerErr = fixture.registrar.RegisterRule(ctx, docnumber.RegisterRuleCommand{
			Tenant: redriveTenant, Type: domain.ExceptionCaseDocumentNumberType.Code, Template: "SYN-{seq:5}",
			EffectiveFrom: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), RegisteredBy: "SYN-TEST",
		})
		return registerErr
	})
	if registerErr != nil {
		t.Fatalf("登案件规则：%v", registerErr)
	}
	collisions, err := adapter.NewCaseNumberingCollisions(fixture.db)
	if err != nil {
		t.Fatal(err)
	}
	ids := &sequentialIdentities{}
	raiser := application.NewRaiseSignalHandler(application.RaiseSignalDeps{
		Episodes: fixture.episodes, Triage: autoEstablishTriage{}, Identities: ids, Cases: ids,
		CaseNumbers: fixture.numbers, Collisions: collisions, Downstream: quietTriageHandoff{}, Clock: fixture.clock,
	})
	redrive, err := application.NewCaseNumberingRedrive(collisions, fixture.clock)
	if err != nil {
		t.Fatal(err)
	}
	return &redriveRig{numberingFixture: fixture, raiser: raiser, redrive: redrive}
}

func (rig *redriveRig) command(t *testing.T, hit domain.SignalHit) application.RaiseSignalCommand {
	t.Helper()
	return application.RaiseSignalCommand{
		TenantID: claimValue(t, domain.NewTenantID, redriveTenant), Kind: hit.Kind, Parcel: hit.Parcel, Cause: hit.Cause,
		Rule: hit.Rule, Confidence: hit.Confidence, Basis: hit.Basis, HitAt: hit.At,
	}
}

// collide 让这一原因撞一次号：先把格式接下来要出的二十个号都占上（流水从不留空号，接着库里最大的号数），走一次真分诊，
// 再腾出旧号，撞号册留一行等重驱。
func (rig *redriveRig) collide(t *testing.T, hits ...domain.SignalHit) {
	t.Helper()
	var next int
	if err := rig.pool.QueryRow(t.Context(),
		`SELECT COALESCE(max(substr(document_number, 5)::int), 0) FROM visibility_exception.exception_case WHERE tenant_id = $1`,
		redriveTenant).Scan(&next); err != nil {
		t.Fatal(err)
	}
	for seq := next + 1; seq <= next+docnumber.MaxSkippedNumbers; seq++ {
		if _, err := rig.pool.Exec(t.Context(),
			`INSERT INTO visibility_exception.exception_case
				(tenant_id, case_id, root_parcel, impact_scope, responsible_team, phase, established_at, document_number)
			 VALUES ($1, $2, 'SYN-PARCEL-OLD', 'SYN-PARCEL-OLD', 'SYN-TEAM', 'AWAITING_RESPONSE', now(), $3)`,
			redriveTenant, fmt.Sprintf("SYN-OLD-%05d", seq), fmt.Sprintf("SYN-%05d", seq)); err != nil {
			t.Fatalf("占旧号：%v", err)
		}
	}
	for _, hit := range hits {
		var result application.RaiseSignalResult
		if err := rig.transactor.WithinTransaction(t.Context(), func(ctx context.Context) error {
			var err error
			result, err = rig.raiser.Handle(ctx, rig.command(t, hit))
			return err
		}); err != nil || result.Outcome() != application.CaseNumberingCollided {
			t.Fatalf("应撞号：%v %q", err, result.Outcome())
		}
	}
	if _, err := rig.pool.Exec(t.Context(),
		`DELETE FROM visibility_exception.exception_case WHERE tenant_id = $1 AND root_parcel = 'SYN-PARCEL-OLD'`, redriveTenant); err != nil {
		t.Fatal(err)
	}
}

func (rig *redriveRig) counts(t *testing.T, parcel string) (episodes, cases, rows int) {
	t.Helper()
	if err := rig.pool.QueryRow(t.Context(),
		`SELECT (SELECT count(*) FROM visibility_exception.signal_episode WHERE tenant_id = $1 AND parcel_ref = $2),
		        (SELECT count(*) FROM visibility_exception.exception_case WHERE tenant_id = $1 AND root_parcel = $2),
		        (SELECT count(*) FROM visibility_exception.case_numbering_collision WHERE tenant_id = $1 AND parcel_ref = $2)`,
		redriveTenant, parcel).Scan(&episodes, &cases, &rows); err != nil {
		t.Fatal(err)
	}
	return episodes, cases, rows
}

func (rig *redriveRig) dueRow(t *testing.T, parcel string) ports.CaseNumberingCollision {
	t.Helper()
	due, err := rig.redrive.Due(t.Context(), 20)
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range due {
		if row.Hit.Parcel.String() == parcel {
			return row
		}
	}
	t.Fatalf("%s 不在到期页上：%+v", parcel, due)
	return ports.CaseNumberingCollision{}
}

// held 在一笔事务里跑 work，跑完先不提交，等 release 关上才提交；交回的通道收这笔事务的结果。
func (rig *redriveRig) held(work func(context.Context) error, release <-chan struct{}) <-chan error {
	done := make(chan error, 1)
	go func() {
		done <- rig.transactor.WithinTransaction(context.Background(), func(ctx context.Context) error {
			if err := work(ctx); err != nil {
				return err
			}
			<-release
			return nil
		})
	}()
	return done
}

// Covers: AT-VE-228 真库半边（并发去重）——两个派发进程同一拍取到同一行：先抢到的重驱建案并销账，后到的抢不到（SKIP LOCKED）
// 答跳过、不等也不分诊；先到的提交之后再来的同样答跳过。只落一个发作期、一件案件，撞号册清空。
func TestTwoRedrivesOfOneCollisionEstablishOneCase(t *testing.T) {
	rig := newRedriveRig(t)
	hit := episodeHit(t, "SYN-PARCEL-R2", "DEPARTURE_GATE_BLOCKED", "movement-fact/SYN-PARCEL-R2/MF-1", numberingNow)
	rig.collide(t, hit)
	row := rig.dueRow(t, "SYN-PARCEL-R2")

	release := make(chan struct{})
	var first application.CaseNumberingRedriveOutcome
	firstDone := rig.held(func(ctx context.Context) error {
		var err error
		first, err = rig.redrive.Redrive(ctx, rig.raiser, row)
		return err
	}, release)
	time.Sleep(300 * time.Millisecond)
	// 后到的不许等先到的：给它一个期限，等住了就是没跳过（期限到了答错，用例不挂死）。
	bounded, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	var second application.CaseNumberingRedriveOutcome
	secondErr := rig.transactor.WithinTransaction(bounded, func(ctx context.Context) error {
		var err error
		second, err = rig.redrive.Redrive(ctx, rig.raiser, row)
		return err
	})
	close(release)
	if secondErr != nil {
		t.Fatalf("后到的重驱：%v", secondErr)
	}
	if err := <-firstDone; err != nil {
		t.Fatalf("先到的重驱：%v", err)
	}
	if first != application.CaseNumberingRedriven || second != application.CaseNumberingRedriveSkipped {
		t.Fatalf("先到 %q、后到 %q，want 建成与跳过", first, second)
	}
	var late application.CaseNumberingRedriveOutcome
	if err := rig.transactor.WithinTransaction(t.Context(), func(ctx context.Context) error {
		var err error
		late, err = rig.redrive.Redrive(ctx, rig.raiser, row)
		return err
	}); err != nil || late != application.CaseNumberingRedriveSkipped {
		t.Fatalf("提交之后再来：%v %q", err, late)
	}
	if e, c, r := rig.counts(t, "SYN-PARCEL-R2"); e != 1 || c != 1 || r != 0 {
		t.Fatalf("发作期 %d、案件 %d、撞号册 %d，want 1/1/0", e, c, r)
	}
}

// Covers: AT-VE-228 真库半边（并发去重）——重驱与同一原因的正常命中交叠，两种次序都只立一件案：正常命中先锁到撞号册那一行，
// 重驱抢不到答跳过；重驱先抢到，正常命中在找发作期之前等它提交，再看见它建的发作期，只记一次命中。
func TestARedriveAndANewHitOfTheSameCauseEstablishOneCase(t *testing.T) {
	rig := newRedriveRig(t)
	ctx := t.Context()

	// 正常命中先到。
	hit := episodeHit(t, "SYN-PARCEL-N1", "DEPARTURE_GATE_BLOCKED", "movement-fact/SYN-PARCEL-N1/MF-1", numberingNow)
	rig.collide(t, hit)
	row := rig.dueRow(t, "SYN-PARCEL-N1")
	later := hit
	later.At = numberingNow.Add(10 * time.Minute)
	release := make(chan struct{})
	var normal application.RaiseSignalResult
	normalDone := rig.held(func(txCtx context.Context) error {
		var err error
		normal, err = rig.raiser.Handle(txCtx, rig.command(t, later))
		return err
	}, release)
	time.Sleep(300 * time.Millisecond)
	bounded, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	var redriven application.CaseNumberingRedriveOutcome
	redriveErr := rig.transactor.WithinTransaction(bounded, func(txCtx context.Context) error {
		var err error
		redriven, err = rig.redrive.Redrive(txCtx, rig.raiser, row)
		return err
	})
	close(release)
	if redriveErr != nil {
		t.Fatalf("重驱：%v", redriveErr)
	}
	if err := <-normalDone; err != nil {
		t.Fatalf("正常命中：%v", err)
	}
	if normal.Outcome() != application.SignalEpisodeOpened || redriven != application.CaseNumberingRedriveSkipped {
		t.Fatalf("正常命中 %q、重驱 %q", normal.Outcome(), redriven)
	}
	if e, c, r := rig.counts(t, "SYN-PARCEL-N1"); e != 1 || c != 1 || r != 0 {
		t.Fatalf("正常命中先到：发作期 %d、案件 %d、撞号册 %d，want 1/1/0", e, c, r)
	}

	// 重驱先到。
	hit2 := episodeHit(t, "SYN-PARCEL-N2", "DEPARTURE_GATE_BLOCKED", "movement-fact/SYN-PARCEL-N2/MF-1", numberingNow)
	rig.collide(t, hit2)
	row2 := rig.dueRow(t, "SYN-PARCEL-N2")
	later2 := hit2
	later2.At = numberingNow.Add(10 * time.Minute)
	release2 := make(chan struct{})
	var redriven2 application.CaseNumberingRedriveOutcome
	redriveDone := rig.held(func(txCtx context.Context) error {
		var err error
		redriven2, err = rig.redrive.Redrive(txCtx, rig.raiser, row2)
		return err
	}, release2)
	time.Sleep(300 * time.Millisecond)
	normalResult := make(chan application.RaiseSignalResult, 1)
	normalErr := make(chan error, 1)
	go func() {
		var result application.RaiseSignalResult
		normalErr <- rig.transactor.WithinTransaction(context.Background(), func(txCtx context.Context) error {
			var err error
			result, err = rig.raiser.Handle(txCtx, rig.command(t, later2))
			return err
		})
		normalResult <- result
	}()
	time.Sleep(300 * time.Millisecond)
	close(release2)
	if err := <-redriveDone; err != nil || redriven2 != application.CaseNumberingRedriven {
		t.Fatalf("重驱：%v %q", err, redriven2)
	}
	if err := <-normalErr; err != nil {
		t.Fatalf("正常命中：%v", err)
	}
	if outcome := (<-normalResult).Outcome(); outcome != application.SignalHitRecorded {
		t.Fatalf("正常命中答 %q，want 等重驱提交后只记一次命中", outcome)
	}
	if e, c, r := rig.counts(t, "SYN-PARCEL-N2"); e != 1 || c != 1 || r != 0 {
		t.Fatalf("重驱先到：发作期 %d、案件 %d、撞号册 %d，want 1/1/0", e, c, r)
	}
}

// Covers: AT-VE-228 真库半边（原样重新分诊）——同一原因先后撞两次，册上留的是首次命中，重驱建出的发作期按首次命中开期、
// 命中一次；撞号次数记两次、格式是最近撞上的那份。
func TestARedriveReplaysTheFirstCollidedHit(t *testing.T) {
	rig := newRedriveRig(t)
	first := episodeHit(t, "SYN-PARCEL-R3", "DEPARTURE_GATE_BLOCKED", "movement-fact/SYN-PARCEL-R3/MF-1", numberingNow)
	second := first
	second.At = numberingNow.Add(30 * time.Minute)
	rig.collide(t, first, second)

	var collisions int
	var hitAt time.Time
	if err := rig.pool.QueryRow(t.Context(),
		`SELECT collisions, hit_at FROM visibility_exception.case_numbering_collision WHERE tenant_id = $1`, redriveTenant).
		Scan(&collisions, &hitAt); err != nil || collisions != 2 || !hitAt.Equal(first.At) {
		t.Fatalf("撞号册：%d 次、命中 %v %v，want 两次、首次命中", collisions, hitAt, err)
	}
	row := rig.dueRow(t, "SYN-PARCEL-R3")
	if err := rig.transactor.WithinTransaction(t.Context(), func(ctx context.Context) error {
		outcome, err := rig.redrive.Redrive(ctx, rig.raiser, row)
		if err == nil && outcome != application.CaseNumberingRedriven {
			err = errors.New("not redriven: " + outcome.String())
		}
		return err
	}); err != nil {
		t.Fatalf("重驱：%v", err)
	}
	var startedAt time.Time
	var hits int
	if err := rig.pool.QueryRow(t.Context(),
		`SELECT started_at, hits FROM visibility_exception.signal_episode WHERE tenant_id = $1 AND parcel_ref = 'SYN-PARCEL-R3'`,
		redriveTenant).Scan(&startedAt, &hits); err != nil || !startedAt.Equal(first.At) || hits != 1 {
		t.Fatalf("发作期 started_at %v、命中 %d %v，want 首次命中那一刻、一次", startedAt, hits, err)
	}
}
