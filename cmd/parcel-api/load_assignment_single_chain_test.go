package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	tfhttp "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/http"
	tfpostgres "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// 本文件在生产端点构造与生产准入桥上证票 tf-face-algebra/02：装载分配登记口只形成一次分配的首版，该分配已有任何
// 版本时答`分配已形成`并回显链头，不另起根——顺序到达、撤回之后、并发到达三条路各答其格。测试输入是隔离合成，只记 `S`。

type chainFaceView struct {
	Outcome    string   `json:"outcome"`
	Assignment string   `json:"assignment"`
	Schedule   string   `json:"schedule"`
	Members    []string `json:"members"`
	Version    string   `json:"version"`
}

type chainFaceFixture struct {
	pool       *pgxpool.Pool
	db         *bentopg.DB
	endpoint   http.Handler
	repository *tfpostgres.LoadAssignments
}

func newChainFaceFixture(t *testing.T) chainFaceFixture {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	registerOperationDecisionAdmission(t, db, isolatedSelfAuthority, accessidentity.DecisionLoadAssignment)
	admission, err := buildAdmissionScope(db, isolatedSelfAuthority)
	if err != nil {
		t.Fatal(err)
	}
	minter, err := buildOperatorMinter(acceptingVerifier{},
		decisionRegister{kinds: []accessidentity.DecisionKind{accessidentity.DecisionLoadAssignment}}, admission)
	if err != nil {
		t.Fatal(err)
	}
	decisions, err := buildOperatorDecisionIntakes(minter, unwiredDecisionTargets{}, unwiredDecisionTargets{})
	if err != nil {
		t.Fatal(err)
	}
	operations, err := buildSegmentOperations(db)
	if err != nil {
		t.Fatal(err)
	}
	repository, err := tfpostgres.NewLoadAssignments(db)
	if err != nil {
		t.Fatal(err)
	}
	return chainFaceFixture{
		pool:       pool,
		db:         db,
		endpoint:   tfhttp.NewFormLoadAssignmentEndpoint(decisions.transport, operations.assign),
		repository: repository,
	}
}

func chainFaceBody(assignment, schedule, version, assignedAt string, members ...string) string {
	quoted := make([]string, 0, len(members))
	for _, member := range members {
		quoted = append(quoted, `"`+member+`"`)
	}
	return fmt.Sprintf(`{"assignment":%q,"schedule":%q,"members":[%s],"version":%q,"assignedAt":%q}`,
		assignment, schedule, strings.Join(quoted, ","), version, assignedAt)
}

func (fixture chainFaceFixture) post(t *testing.T, body string) (int, chainFaceView, string) {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, "/transport-fulfillment-load-assignment-registrations", strings.NewReader(body))
	request.Header.Set("Authorization", "Bearer presented.operator.token")
	response := httptest.NewRecorder()
	fixture.endpoint.ServeHTTP(response, request)
	var view chainFaceView
	_ = json.Unmarshal(response.Body.Bytes(), &view)
	return response.Code, view, response.Body.String()
}

func (fixture chainFaceFixture) count(t *testing.T, assignment, where string) int {
	t.Helper()
	var rows int
	if err := fixture.pool.QueryRow(t.Context(),
		`SELECT count(*) FROM transport_fulfillment.load_assignment
		  WHERE tenant_id = 'SYN-TENANT-01' AND assignment_ref = $1 AND `+where, assignment).Scan(&rows); err != nil {
		t.Fatalf("数版本行：%v", err)
	}
	return rows
}

func chainFaceKey(t *testing.T, assignment, version string) ports.LoadAssignmentKey {
	t.Helper()
	tenant, err := domain.NewTenantID("SYN-TENANT-01")
	if err != nil {
		t.Fatal(err)
	}
	ref, err := domain.NewLoadAssignmentReference(assignment)
	if err != nil {
		t.Fatal(err)
	}
	ver, err := domain.NewLoadAssignmentVersion(version)
	if err != nil {
		t.Fatal(err)
	}
	return ports.LoadAssignmentKey{TenantID: tenant, Assignment: ref, Version: ver}
}

func chainFaceRoot(t *testing.T, assignment, version string) ports.LoadAssignmentRecord {
	t.Helper()
	key := chainFaceKey(t, assignment, version)
	member, err := domain.NewCarriedObjectReference("SYN-PARCEL-CHAIN-HOLD")
	if err != nil {
		t.Fatal(err)
	}
	schedule, err := domain.NewScheduleReference("SYN-SCHEDULE-CHAIN-HOLD")
	if err != nil {
		t.Fatal(err)
	}
	formed, err := domain.FormLoadAssignment(domain.LoadAssignmentSpec{
		TenantID:   key.TenantID,
		Assignment: key.Assignment,
		Schedule:   schedule,
		Members:    []domain.CarriedObjectReference{member},
		Version:    key.Version,
		AssignedAt: time.Date(2026, 10, 18, 8, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("形成首版夹具：%v", err)
	}
	return ports.LoadAssignmentRecord{Key: key, Assignment: formed, RecordedAt: time.Date(2026, 10, 18, 8, 1, 0, 0, time.UTC)}
}

// Covers: 同一分配换版本号再投——答 200 `LOAD_ASSIGNMENT_ALREADY_FORMED` 并回显链头 v1（班次与版本都是 v1 的），
// 库里仍只有 v1 一行；换版本号不是变化，变化走领域 ReviseMembers 的入口。
func TestTheLoadAssignmentFaceAnswersAlreadyFormedForASecondVersion(t *testing.T) {
	fixture := newChainFaceFixture(t)
	const assignment = "SYN-LA-CHAIN-SEQ"

	code, view, raw := fixture.post(t, chainFaceBody(assignment, "SYN-SCHEDULE-S1", "v1", "2026-10-18T08:00:00Z", "SYN-PARCEL-X", "SYN-PARCEL-Y"))
	if code != http.StatusCreated || view.Outcome != "LOAD_ASSIGNMENT_FORMED" {
		t.Fatalf("首版：%d %s", code, raw)
	}

	code, view, raw = fixture.post(t, chainFaceBody(assignment, "SYN-SCHEDULE-S2", "v2", "2026-10-17T00:00:00Z", "SYN-PARCEL-Z"))
	if code != http.StatusOK || view.Outcome != "LOAD_ASSIGNMENT_ALREADY_FORMED" {
		t.Fatalf("同一分配换版本号：%d %s, want 200 LOAD_ASSIGNMENT_ALREADY_FORMED", code, raw)
	}
	if view.Version != "v1" || view.Schedule != "SYN-SCHEDULE-S1" || len(view.Members) != 2 {
		t.Fatalf("没有回显链头 v1：%s", raw)
	}
	if rows := fixture.count(t, assignment, "true"); rows != 1 {
		t.Fatalf("版本行 %d, want 1——本口另起了一个根", rows)
	}

	// 本口没占掉的 v2 留给领域「变化」：回指 v1、沿用分配时刻的那一版照常登记。
	v1, found, err := fixture.repository.FindByKey(t.Context(), chainFaceKey(t, assignment, "v1"))
	if err != nil || !found {
		t.Fatalf("读回 v1：found=%v err=%v", found, err)
	}
	member, err := domain.NewCarriedObjectReference("SYN-PARCEL-Z")
	if err != nil {
		t.Fatal(err)
	}
	revised, err := v1.Assignment.ReviseMembers([]domain.CarriedObjectReference{member},
		chainFaceKey(t, assignment, "v2").Version, time.Date(2026, 10, 18, 9, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("领域变化：%v", err)
	}
	if err := fixture.db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
		saved, saveErr := fixture.repository.Save(txCtx, ports.LoadAssignmentRecord{
			Key: chainFaceKey(t, assignment, "v2"), Assignment: revised, RecordedAt: time.Date(2026, 10, 18, 9, 1, 0, 0, time.UTC),
		})
		if saveErr == nil && saved != ports.LoadAssignmentSaved {
			saveErr = fmt.Errorf("变化版 outcome = %d", saved)
		}
		return saveErr
	}); err != nil {
		t.Fatalf("存变化版：%v", err)
	}
}

// Covers: 撤回链（经生产登记册存入领域 Withdraw 形成的撤回版）之后再投新版本——答`分配已形成`并回显链头即撤回版，
// 不在已撤回的分配上起活跃新根；同版本重放仍按键答`版本已在册`回显原版（票 20 判据不动）。
func TestTheLoadAssignmentFaceAnswersAlreadyFormedAfterAWithdrawnChain(t *testing.T) {
	fixture := newChainFaceFixture(t)
	const assignment = "SYN-LA-CHAIN-WD"
	first := chainFaceBody(assignment, "SYN-SCHEDULE-W", "v1", "2026-10-18T08:00:00Z", "SYN-PARCEL-W1")

	if code, view, raw := fixture.post(t, first); code != http.StatusCreated || view.Outcome != "LOAD_ASSIGNMENT_FORMED" {
		t.Fatalf("首版：%d %s", code, raw)
	}
	v1, found, err := fixture.repository.FindByKey(t.Context(), chainFaceKey(t, assignment, "v1"))
	if err != nil || !found {
		t.Fatalf("读回 v1：found=%v err=%v", found, err)
	}
	withdrawn, err := v1.Assignment.Withdraw(chainFaceKey(t, assignment, "v2").Version, time.Date(2026, 10, 18, 9, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("领域撤回：%v", err)
	}
	if err := fixture.db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
		saved, saveErr := fixture.repository.Save(txCtx, ports.LoadAssignmentRecord{
			Key: chainFaceKey(t, assignment, "v2"), Assignment: withdrawn, RecordedAt: time.Date(2026, 10, 18, 9, 1, 0, 0, time.UTC),
		})
		if saveErr == nil && saved != ports.LoadAssignmentSaved {
			saveErr = fmt.Errorf("撤回版 outcome = %d", saved)
		}
		return saveErr
	}); err != nil {
		t.Fatalf("存撤回版：%v", err)
	}

	code, view, raw := fixture.post(t, chainFaceBody(assignment, "SYN-SCHEDULE-W", "v3", "2026-10-18T07:00:00Z", "SYN-PARCEL-W1"))
	if code != http.StatusOK || view.Outcome != "LOAD_ASSIGNMENT_ALREADY_FORMED" || view.Version != "v2" {
		t.Fatalf("撤回之后再投：%d %s, want 200 LOAD_ASSIGNMENT_ALREADY_FORMED 回显撤回版 v2", code, raw)
	}
	if rows := fixture.count(t, assignment, "true"); rows != 2 {
		t.Fatalf("版本行 %d, want 2", rows)
	}

	code, view, raw = fixture.post(t, first)
	if code != http.StatusOK || view.Outcome != "LOAD_ASSIGNMENT_VERSION_EXISTS" || view.Version != "v1" {
		t.Fatalf("同版本重放：%d %s, want 200 LOAD_ASSIGNMENT_VERSION_EXISTS 回显 v1", code, raw)
	}
}

// Covers: 并发两首版的确定交错——另一位写入方的首版在事务里落下未提交，本口此刻读链头读空、照常插入，在首版唯一
// 索引上等那一方；那一方提交后本口撞上它，按分配读回链头答`分配已形成`回显赢家，库面只有一个首版。
func TestALosingConcurrentFirstVersionAnswersAlreadyFormedWithTheWinner(t *testing.T) {
	fixture := newChainFaceFixture(t)
	const assignment = "SYN-LA-CHAIN-RACE"

	observer, err := pgx.Connect(t.Context(), pgtest.AdminDSN(t))
	if err != nil {
		t.Fatalf("开观察连接：%v", err)
	}
	t.Cleanup(func() { _ = observer.Close(context.Background()) })
	database := fixture.pool.Config().ConnConfig.Database

	winner := chainFaceRoot(t, assignment, "vA")
	held, release, committed := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	go func() {
		committed <- fixture.db.Transactor().WithinTransaction(context.Background(), func(txCtx context.Context) error {
			saved, saveErr := fixture.repository.Save(txCtx, winner)
			if saveErr == nil && saved != ports.LoadAssignmentSaved {
				saveErr = fmt.Errorf("赢家 outcome = %d", saved)
			}
			close(held)
			<-release
			return saveErr
		})
	}()
	<-held

	type answer struct {
		code int
		view chainFaceView
		raw  string
	}
	answered := make(chan answer, 1)
	go func() {
		code, view, raw := fixture.post(t, chainFaceBody(assignment, "SYN-SCHEDULE-R", "vB", "2026-10-18T08:00:00Z", "SYN-PARCEL-R"))
		answered <- answer{code, view, raw}
	}()

	// 等本口那笔插入真的挂在锁上再放行赢家；修复前它不等任何人、直接答 201，下面就在答案上红。
	var early *answer
	for deadline := time.Now().Add(10 * time.Second); early == nil; {
		select {
		case got := <-answered:
			early = &got
			continue
		default:
		}
		var waiting int
		if err := observer.QueryRow(t.Context(),
			`SELECT count(*) FROM pg_stat_activity WHERE datname = $1 AND wait_event_type = 'Lock'`, database).Scan(&waiting); err != nil {
			t.Fatalf("看锁等待：%v", err)
		}
		if waiting > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("本口既没答也没挂在锁上")
		}
		time.Sleep(10 * time.Millisecond)
	}
	close(release)
	if err := <-committed; err != nil {
		t.Fatalf("赢家提交：%v", err)
	}
	got := early
	if got == nil {
		received := <-answered
		got = &received
	}
	if got.code != http.StatusOK || got.view.Outcome != "LOAD_ASSIGNMENT_ALREADY_FORMED" || got.view.Version != "vA" {
		t.Fatalf("并发落败的首版：%d %s, want 200 LOAD_ASSIGNMENT_ALREADY_FORMED 回显赢家 vA", got.code, got.raw)
	}
	if roots := fixture.count(t, assignment, "corrects_version IS NULL"); roots != 1 {
		t.Fatalf("首版 %d 个, want 1", roots)
	}
}

// Covers: 两份不同版本号的首版经本口同时到达——恰一份 201，另一份答`分配已形成`回显赢家，库面只有一个首版。
// 交错由调度决定，几轮里落在预检还是落在索引上都应同答；确定落在索引上的那条路见上一个用例。
func TestConcurrentFirstVersionsThroughTheFaceFormExactlyOne(t *testing.T) {
	fixture := newChainFaceFixture(t)

	for round := range 4 {
		assignment := fmt.Sprintf("SYN-LA-CHAIN-PAIR-%d", round)
		start := make(chan struct{})
		var wait sync.WaitGroup
		codes := make([]int, 2)
		views := make([]chainFaceView, 2)
		raws := make([]string, 2)
		for slot, version := range []string{"vA", "vB"} {
			wait.Add(1)
			go func() {
				defer wait.Done()
				<-start
				codes[slot], views[slot], raws[slot] = fixture.post(t,
					chainFaceBody(assignment, "SYN-SCHEDULE-P", version, "2026-10-18T08:00:00Z", "SYN-PARCEL-P-"+version))
			}()
		}
		close(start)
		wait.Wait()

		formed, other := 0, 1
		if codes[1] == http.StatusCreated {
			formed, other = 1, 0
		}
		if codes[formed] != http.StatusCreated || views[formed].Outcome != "LOAD_ASSIGNMENT_FORMED" {
			t.Fatalf("第 %d 轮没有一份形成：%v %v", round, raws[0], raws[1])
		}
		if codes[other] != http.StatusOK || views[other].Outcome != "LOAD_ASSIGNMENT_ALREADY_FORMED" ||
			views[other].Version != views[formed].Version {
			t.Fatalf("第 %d 轮另一份：%d %s, want 200 LOAD_ASSIGNMENT_ALREADY_FORMED 回显赢家 %s",
				round, codes[other], raws[other], views[formed].Version)
		}
		if roots := fixture.count(t, assignment, "corrects_version IS NULL"); roots != 1 {
			t.Fatalf("第 %d 轮首版 %d 个, want 1", round, roots)
		}
	}
}
