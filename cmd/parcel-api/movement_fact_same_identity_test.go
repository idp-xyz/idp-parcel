package main

// 票 tf-face-algebra/08 的真渠道真库回归：`/transport-fulfillment/movement-facts` 同身份按内容分重放与 SOURCE_CONFLICT。
// Intake 走生产 buildWorkFactIntakes，操作者册与设备册是真库上的生产登记册，只有令牌核验方是替身（workFactVerifier）；
// 编排走生产 buildMovementFactOrchestration。测试输入是隔离合成，只记 `S`。

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

	"github.com/jackc/pgx/v5/pgxpool"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	aipostgres "go.idp.xyz/idp-parcel/internal/accessidentity/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	tfhttp "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/http"
	tfpostgres "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/postgres"
	tfapp "go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	tfdomain "go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

type sameIdentityAnswer struct {
	status int
	body   map[string]any
	raw    string
}

func (answer sameIdentityAnswer) outcome() string {
	value, _ := answer.body["outcome"].(string)
	return value
}

// onlyOutcome 断言冲突答复不带在册内容：那份内容可能来自另一作业范围。
func (answer sameIdentityAnswer) onlyOutcome(t *testing.T, want string) {
	t.Helper()
	if answer.status != http.StatusOK || answer.outcome() != want || len(answer.body) != 1 {
		t.Fatalf("HTTP %d %s, want 200 且只有 outcome=%s", answer.status, answer.raw, want)
	}
}

func postSameIdentity(endpoint http.Handler, scopeRef, device, body string) sameIdentityAnswer {
	request := httptest.NewRequest(http.MethodPost, "/transport-fulfillment/movement-facts", strings.NewReader(body))
	stampWorkFact(request, "OFFSITE", scopeRef, device)
	recorder := httptest.NewRecorder()
	endpoint.ServeHTTP(recorder, request)
	answer := sameIdentityAnswer{status: recorder.Code, raw: strings.TrimSpace(recorder.Body.String()), body: map[string]any{}}
	_ = json.Unmarshal(recorder.Body.Bytes(), &answer.body)
	return answer
}

// movementFactRows 把一条事实在库面的全部版本成串，比前后两次库面用；recorded_at 在内，重写一次也看得出。
func movementFactRows(t *testing.T, pool *pgxpool.Pool, fact string) []string {
	t.Helper()
	rows, err := pool.Query(context.Background(),
		`SELECT version, schedule_ref, kind, location_ref, source_ref, occurred_at, recorded_at, coalesce(gate_clearance, '<NULL>')
		   FROM transport_fulfillment.transport_movement_fact
		  WHERE tenant_id = 'SYN-TENANT-01' AND fact_ref = $1 ORDER BY version`, fact)
	if err != nil {
		t.Fatalf("查库：%v", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var version, schedule, kind, location, source, gate string
		var occurredAt, recordedAt time.Time
		if err := rows.Scan(&version, &schedule, &kind, &location, &source, &occurredAt, &recordedAt, &gate); err != nil {
			t.Fatalf("扫行：%v", err)
		}
		out = append(out, fmt.Sprintf("%s|%s|%s|%s|%s|%s|%s|%s", version, schedule, kind, location, source,
			occurredAt.UTC().Format(time.RFC3339Nano), recordedAt.UTC().Format(time.RFC3339Nano), gate))
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("行错：%v", err)
	}
	return out
}

// sameIdentityProductionEndpoint 在真库上登一名操作者、两个作业范围的授予与三台设备：A、B 在 SYN-OFFSITE-01，C 在 SYN-OFFSITE-02。
func sameIdentityProductionEndpoint(t *testing.T) (*pgxpool.Pool, *bentopg.DB, http.Handler) {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	operators, err := aipostgres.NewOperatorRegistry(db)
	if err != nil {
		t.Fatal(err)
	}
	devices, err := aipostgres.NewDeviceRegistry(db)
	if err != nil {
		t.Fatal(err)
	}
	admission, err := buildAdmissionScope(db, "")
	if err != nil {
		t.Fatal(err)
	}
	_, transport, err := buildWorkFactIntakes(workFactVerifier{}, operators, devices, admission)
	if err != nil {
		t.Fatal(err)
	}
	movement, err := buildMovementFactOrchestration(db)
	if err != nil {
		t.Fatal(err)
	}

	subject, err := accessidentity.NewOperatorSubject("https://id.syn.example/dex", "SYN-OPERATOR-01")
	if err != nil {
		t.Fatal(err)
	}
	binding, err := accessidentity.NewOperatorBinding(subject, "SYN-TENANT-01", "SYN-BASIS")
	if err != nil {
		t.Fatal(err)
	}
	interval, err := accessidentity.NewEffectiveInterval(time.Now().Add(-time.Hour), time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	err = db.Transactor().WithinTransaction(context.Background(), func(txCtx context.Context) error {
		if _, err := operators.RegisterOperator(txCtx, binding); err != nil {
			return err
		}
		for _, scopeRef := range []string{"SYN-OFFSITE-01", "SYN-OFFSITE-02"} {
			grant, err := accessidentity.NewWorkFactRegistrationGrant("SYN-TENANT-01", "SYN-GRANT-"+scopeRef, subject, mustScope("OFFSITE", scopeRef), interval, "SYN-BASIS")
			if err != nil {
				return err
			}
			if _, err := operators.RegisterGrant(txCtx, grant); err != nil {
				return err
			}
		}
		for device, scopeRef := range map[string]string{"SYN-DEVICE-A": "SYN-OFFSITE-01", "SYN-DEVICE-B": "SYN-OFFSITE-01", "SYN-DEVICE-C": "SYN-OFFSITE-02"} {
			registration, err := accessidentity.NewDeviceRegistration("SYN-TENANT-01", device, mustScope("OFFSITE", scopeRef), "SYN-BASIS")
			if err != nil {
				return err
			}
			if _, err := devices.RegisterDevice(txCtx, registration); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("登记：%v", err)
	}
	return pool, db, tfhttp.NewRecordMovementFactEndpoint(transport, movement)
}

// Covers: 票 tf-face-algebra/08——身份键是（租户、事实、版本），不含设备与作业范围。原样重放与只差发生时间（ADR-0023：业务
// 时间不进摘要）答已有版本；同作业范围另一设备、另一作业范围另一设备带同一（事实、版本）而内容不同，各答 SOURCE_CONFLICT，
// 答复不含在册内容，库面一字不变。
func TestMovementFactSameIdentityIsJudgedByContentOnTheProductionChannel(t *testing.T) {
	pool, _, endpoint := sameIdentityProductionEndpoint(t)
	const fact = "SYN-MOVE-08-01"
	byA := `{"fact":"SYN-MOVE-08-01","schedule":"SYN-SCHEDULE/linehaul-08","kind":"ARRIVAL","location":"SYN-NODE-SIN-HUB","version":"1","occurredAt":"2026-09-24T10:00:00.250Z"}`

	if first := postSameIdentity(endpoint, "SYN-OFFSITE-01", "SYN-DEVICE-A", byA); first.status != http.StatusCreated {
		t.Fatalf("首登 HTTP %d %s", first.status, first.raw)
	}
	recorded := movementFactRows(t, pool, fact)
	if len(recorded) != 1 {
		t.Fatalf("首登后库面 %v", recorded)
	}

	replays := map[string]string{
		"原样重放":     byA,
		"只差亚秒发生时间": strings.Replace(byA, "10:00:00.250Z", "10:00:00.750Z", 1),
		"只差整秒发生时间": strings.Replace(byA, "10:00:00.250Z", "10:00:01.250Z", 1),
	}
	for name, body := range replays {
		answer := postSameIdentity(endpoint, "SYN-OFFSITE-01", "SYN-DEVICE-A", body)
		if answer.status != http.StatusOK || answer.outcome() != "MOVEMENT_FACT_VERSION_EXISTS" {
			t.Fatalf("%s：HTTP %d %s, want 200 MOVEMENT_FACT_VERSION_EXISTS", name, answer.status, answer.raw)
		}
	}

	postSameIdentity(endpoint, "SYN-OFFSITE-01", "SYN-DEVICE-B",
		`{"fact":"SYN-MOVE-08-01","schedule":"SYN-SCHEDULE/linehaul-09","kind":"DEPARTURE","location":"SYN-NODE-SHA-HUB","version":"1","occurredAt":"2026-09-24T09:00:00Z"}`).
		onlyOutcome(t, "SOURCE_CONFLICT")
	postSameIdentity(endpoint, "SYN-OFFSITE-02", "SYN-DEVICE-C",
		`{"fact":"SYN-MOVE-08-01","schedule":"SYN-SCHEDULE/linehaul-10","kind":"IN_TRANSIT","location":"SYN-POINT/sea-07","version":"1","occurredAt":"2026-09-24T11:30:00Z"}`).
		onlyOutcome(t, "SOURCE_CONFLICT")

	if after := movementFactRows(t, pool, fact); strings.Join(after, "\n") != strings.Join(recorded, "\n") {
		t.Fatalf("库面变了：\n前 %v\n后 %v", recorded, after)
	}
}

// Covers: 票 tf-face-algebra/08——自然竞态多轮：同一（事实、版本）各带一个地点同时到，每轮恰一方 201，其余都答 SOURCE_CONFLICT
// 且不回显赢家；库面每轮一行。输家可能走首读即见赢家那一支，也可能走撞键读回那一支，两支都得比内容。
func TestMovementFactRaceLosersWithDifferentContentAnswerSourceConflict(t *testing.T) {
	pool, _, endpoint := sameIdentityProductionEndpoint(t)
	const rounds, contenders = 6, 12
	for round := 0; round < rounds; round++ {
		fact := fmt.Sprintf("SYN-MOVE-08-RACE-%02d", round)
		start := make(chan struct{})
		answers := make([]sameIdentityAnswer, contenders)
		var wait sync.WaitGroup
		for index := 0; index < contenders; index++ {
			wait.Add(1)
			go func(index int) {
				defer wait.Done()
				body := fmt.Sprintf(`{"fact":%q,"schedule":"SYN-SCHEDULE/linehaul-08","kind":"ARRIVAL","location":"SYN-NODE-%02d","version":"1","occurredAt":"2026-09-24T10:00:00Z"}`, fact, index)
				<-start
				answers[index] = postSameIdentity(endpoint, "SYN-OFFSITE-01", "SYN-DEVICE-A", body)
			}(index)
		}
		close(start)
		wait.Wait()

		created := 0
		for _, answer := range answers {
			if answer.status == http.StatusCreated {
				created++
				continue
			}
			answer.onlyOutcome(t, "SOURCE_CONFLICT")
		}
		if created != 1 {
			t.Fatalf("第 %d 轮 201 = %d, want 1", round, created)
		}
		if rows := movementFactRows(t, pool, fact); len(rows) != 1 {
			t.Fatalf("第 %d 轮库面 %v", round, rows)
		}
	}
}

// staleFirstReadFacts 让首读答「没有」，把输家确定地送进撞键读回那一支——自然竞态里那一支走不走到要看调度。
type staleFirstReadFacts struct {
	ports.MovementFactRegistry
	read bool
}

func (stale *staleFirstReadFacts) FindByKey(ctx context.Context, key ports.MovementFactKey) (ports.MovementFactRecord, bool, error) {
	if !stale.read {
		stale.read = true
		return ports.MovementFactRecord{}, false, nil
	}
	return stale.MovementFactRegistry.FindByKey(ctx, key)
}

// Covers: 票 tf-face-algebra/08——撞键读回那一支在真库上同样按内容分格：异内容答 SOURCE_CONFLICT 不带记录，同内容答已有版本
// 交回赢家；库面始终是赢家那一行。
func TestMovementFactCollisionLoserIsComparedOnARealDatabase(t *testing.T) {
	pool, db, _ := sameIdentityProductionEndpoint(t)
	winner, err := buildMovementFactOrchestration(db)
	if err != nil {
		t.Fatal(err)
	}
	facts, err := tfpostgres.NewMovementFacts(db)
	if err != nil {
		t.Fatal(err)
	}
	command := tfapp.RecordMovementFactCommand{
		TenantID:   mustValue(t, tfdomain.NewTenantID, "SYN-TENANT-01"),
		Fact:       "SYN-MOVE-08-20",
		Schedule:   "SYN-SCHEDULE/linehaul-08",
		Kind:       tfdomain.ArrivalFact,
		Location:   "SYN-NODE-SIN-HUB",
		Source:     frontlineMovementSource,
		Version:    "1",
		OccurredAt: time.Date(2026, 9, 24, 10, 0, 0, 0, time.UTC),
	}
	if won, err := winner.Record(t.Context(), command); err != nil || won.Outcome() != tfapp.MovementFactRecorded {
		t.Fatalf("赢家：outcome=%q err=%v", won.Outcome(), err)
	}
	recorded := movementFactRows(t, pool, "SYN-MOVE-08-20")

	different := command
	different.Location = "SYN-NODE-SHA-HUB"
	for name, testCase := range map[string]struct {
		command tfapp.RecordMovementFactCommand
		want    tfapp.MovementFactOutcome
	}{
		"异内容": {different, tfapp.MovementFactSourceConflict},
		"同内容": {command, tfapp.MovementFactVersionExists},
	} {
		loser := transactionalMovementFact{
			transactor: db.Transactor(),
			inner: tfapp.NewRecordMovementFactHandler(tfapp.RecordMovementFactDeps{
				Facts: &staleFirstReadFacts{MovementFactRegistry: facts},
				Clock: systemClock{},
			}),
		}
		lost, err := loser.Record(t.Context(), testCase.command)
		if err != nil {
			t.Fatalf("%s：%v", name, err)
		}
		if lost.Outcome() != testCase.want {
			t.Fatalf("%s：outcome = %q, want %q", name, lost.Outcome(), testCase.want)
		}
		if _, echoed := lost.Record(); echoed != (testCase.want == tfapp.MovementFactVersionExists) {
			t.Fatalf("%s：回显 = %v：重放交回赢家，冲突不回显", name, echoed)
		}
	}
	if after := movementFactRows(t, pool, "SYN-MOVE-08-20"); strings.Join(after, "\n") != strings.Join(recorded, "\n") {
		t.Fatalf("库面变了：\n前 %v\n后 %v", recorded, after)
	}
}
