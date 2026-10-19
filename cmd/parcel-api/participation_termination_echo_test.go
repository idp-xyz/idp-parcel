package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	tfhttp "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/http"
	tfpg "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/postgres"
	tfapp "go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	tfdomain "go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	tfports "go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// 票 tf-face-algebra/03 的真渠道真库回归：终止口经操作者决定渠道（生产准入桥、生产编排、真段登记册）进来，
// 已离场的答复回显在册终点，坏命令不随参与状态改答，并发输家读回再答。

const terminationPath = "/transport-fulfillment-participation-terminations"

type terminationChannel struct {
	pool        *pgxpool.Pool
	db          *bentopg.DB
	decisions   operatorDecisionIntakes
	terminate   http.Handler
	pickups     http.Handler
	handovers   http.Handler
	corrections http.Handler
}

func newTerminationChannel(t *testing.T) terminationChannel {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatal(err)
	}
	registerOperationDecisionAdmission(t, db, isolatedSelfAuthority, accessidentity.DecisionParticipationTermination)
	admission, err := buildAdmissionScope(db, isolatedSelfAuthority)
	if err != nil {
		t.Fatal(err)
	}
	minter, err := buildOperatorMinter(acceptingVerifier{}, decisionRegister{kinds: []accessidentity.DecisionKind{accessidentity.DecisionParticipationTermination}}, admission)
	if err != nil {
		t.Fatal(err)
	}
	decisions, err := buildOperatorDecisionIntakes(minter, unwiredDecisionTargets{}, unwiredDecisionTargets{})
	if err != nil {
		t.Fatal(err)
	}
	segmentOps, err := buildSegmentOperations(db)
	if err != nil {
		t.Fatal(err)
	}
	controlFacts, err := buildControlFactOrchestrations(db)
	if err != nil {
		t.Fatal(err)
	}
	work := workFactTransportForTest(t)
	return terminationChannel{
		pool:        pool,
		db:          db,
		decisions:   decisions,
		terminate:   tfhttp.NewTerminateFulfillmentParticipationEndpoint(decisions.transport, segmentOps.enderOf),
		pickups:     tfhttp.NewRegisterOffsitePickupEndpoint(work, controlFacts.pickupRegistration),
		handovers:   tfhttp.NewRegisterTransportHandoverEndpoint(work, controlFacts.handover),
		corrections: tfhttp.NewCorrectTransportHandoverEndpoint(work, controlFacts.handover),
	}
}

func (channel terminationChannel) post(endpoint http.Handler, path, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	stampWorkFact(request, "OFFSITE", "SYN-OFFSITE-01", "SYN-DEVICE-OFFSITE")
	recorder := httptest.NewRecorder()
	endpoint.ServeHTTP(recorder, request)
	return recorder
}

// mustPost 只用于铺垫：铺垫没成，后面的断言就无从谈起。
func (channel terminationChannel) mustPost(t *testing.T, endpoint http.Handler, path, body string, want int) {
	t.Helper()
	if response := channel.post(endpoint, path, body); response.Code != want {
		t.Fatalf("铺垫 %s：status = %d body = %s", path, response.Code, response.Body.String())
	}
}

// fingerprint 取每张基表整表内容的摘要与全部序列的 last_value：行数抓不到 UPDATE，序列抓得到「签了号又回滚」。
func (channel terminationChannel) fingerprint(t *testing.T) map[string]string {
	t.Helper()
	ctx := t.Context()
	rows, err := channel.pool.Query(ctx, `SELECT format('%I.%I', table_schema, table_name) FROM information_schema.tables
		WHERE table_type = 'BASE TABLE' AND table_schema NOT IN ('pg_catalog', 'information_schema') ORDER BY 1`)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatal(err)
		}
		names = append(names, name)
	}
	rows.Close()
	fingerprint := make(map[string]string, len(names))
	for _, name := range names {
		var digest string
		if err := channel.pool.QueryRow(ctx, "SELECT md5(coalesce(string_agg(x::text, '|' ORDER BY x::text), '')) FROM "+name+" x").Scan(&digest); err != nil {
			t.Fatal(err)
		}
		fingerprint[name] = digest
	}
	sequences, err := channel.pool.Query(ctx, `SELECT schemaname || '.' || sequencename, coalesce(last_value, -1) FROM pg_sequences`)
	if err != nil {
		t.Fatal(err)
	}
	defer sequences.Close()
	for sequences.Next() {
		var name string
		var last int64
		if err := sequences.Scan(&name, &last); err != nil {
			t.Fatal(err)
		}
		fingerprint["sequence "+name] = fmt.Sprint(last)
	}
	if _, present := fingerprint["bento.outbox"]; !present {
		t.Fatal("指纹里没有 bento.outbox——发意图那一侧没被看住")
	}
	return fingerprint
}

func changedSince(before, after map[string]string) []string {
	var changed []string
	for name, value := range after {
		if before[name] != value {
			changed = append(changed, name)
		}
	}
	for name := range before {
		if _, present := after[name]; !present {
			changed = append(changed, name)
		}
	}
	sort.Strings(changed)
	return changed
}

// participationRow 是库面上一行参与的终点与链位置；tail 说它是不是链尾（没有任何一版回指它）。
type participationRow struct {
	entryBasis string
	endKind    string
	endBasis   string
	endedAt    *time.Time
	tail       bool
}

func (channel terminationChannel) participationRows(t *testing.T, segment, object string) []participationRow {
	t.Helper()
	rows, err := channel.pool.Query(t.Context(), `SELECT p.entry_basis, coalesce(p.end_kind, ''), coalesce(p.end_basis, ''), p.ended_at,
		       NOT EXISTS (SELECT 1 FROM transport_fulfillment.fulfillment_participation s
		                    WHERE s.tenant_id = p.tenant_id AND s.segment_ref = p.segment_ref
		                      AND s.object_ref = p.object_ref AND s.supersedes_entry_basis = p.entry_basis)
		  FROM transport_fulfillment.fulfillment_participation p
		 WHERE p.tenant_id = 'SYN-TENANT-01' AND p.segment_ref = $1 AND p.object_ref = $2
		 ORDER BY p.recorded_at, p.entry_basis`, segment, object)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []participationRow
	for rows.Next() {
		var row participationRow
		if err := rows.Scan(&row.entryBasis, &row.endKind, &row.endBasis, &row.endedAt, &row.tail); err != nil {
			t.Fatal(err)
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func (channel terminationChannel) chainTail(t *testing.T, segment, object string) participationRow {
	t.Helper()
	for _, row := range channel.participationRows(t, segment, object) {
		if row.tail {
			return row
		}
	}
	t.Fatalf("%s 在 %s 里没有链尾", object, segment)
	return participationRow{}
}

type terminationAnswer struct {
	Outcome  string `json:"outcome"`
	EndKind  string `json:"endKind"`
	EndBasis string `json:"endBasis"`
	EndedAt  string `json:"endedAt"`
}

func decodeTerminationAnswer(t *testing.T, response *httptest.ResponseRecorder) terminationAnswer {
	t.Helper()
	var answer terminationAnswer
	if err := json.Unmarshal(response.Body.Bytes(), &answer); err != nil {
		t.Fatalf("解答复 %s：%v", response.Body.Bytes(), err)
	}
	return answer
}

func offsitePickupBody(object, segment string) string {
	return fmt.Sprintf(`{"object":%q,"task":"SYN-TASK-TFA03","attempt":"SYN-ATTEMPT-%s","place":"SYN-PLACE/dock-tfa03",`+
		`"control":"SYN-CONTROL/signed-%s","executedBy":"SYN-COURIER-TFA03","occurredAt":"2026-09-24T08:30:00+08:00","segment":%q}`,
		object, object, object, segment)
}

func handedOverBody(object, scope, version, judgedAt, segment string) string {
	return fmt.Sprintf(`{"object":%q,"scope":%q,"releasedBy":"SYN-PARTY/shipper-tfa","receivedBy":"SYN-PARTY/linehaul-tfa",`+
		`"verdict":"HANDED_OVER","releasingEvidence":"SYN-EVIDENCE/rel-tfa","receivingEvidence":"SYN-EVIDENCE/rec-tfa",`+
		`"rule":"SYN-RULE/handover@v1","version":%q,"judgedAt":%q,"segment":%q}`, object, scope, version, judgedAt, segment)
}

// stillHandedOverCorrectionBody 是一次裁决仍为已交接的交接更正：它在段上长出一版仍在场的替代版本。
func stillHandedOverCorrectionBody(object, scope, predecessor, version, correctedAt string) string {
	return fmt.Sprintf(`{"object":%q,"scope":%q,"predecessorVersion":%q,"verdict":"HANDED_OVER","releasingEvidence":"SYN-EVIDENCE/rel-tfa",`+
		`"receivingEvidence":"SYN-EVIDENCE/rec-tfa","rule":"SYN-RULE/handover@v1","basis":"",`+
		`"newVersion":%q,"correctedAt":%q}`, object, scope, predecessor, version, correctedAt)
}

func terminationRequestBody(segment, object, basis, endedAt string) string {
	return fmt.Sprintf(`{"segment":%q,"object":%q,"basis":%q,"endedAt":%q}`, segment, object, basis, endedAt)
}

// 首次终止 201；此后原样重放、异依据、异时刻都答已离场并回显在册那一份（依据仍是首投的）；坏命令落在已离场的
// 参与上答 INPUT_NOT_ACCEPTED；这一串答复之后全库表内容与序列一概不变。
func TestATerminationReplayEchoesTheRecordedEndAndWritesNothing(t *testing.T) {
	channel := newTerminationChannel(t)
	const segment, object = "SYN-SEGMENT-TFA03-1", "SYN-PARCEL-TFA03-1"
	channel.mustPost(t, channel.pickups, "/transport-fulfillment/offsite-pickups", offsitePickupBody(object, segment), http.StatusCreated)

	first := terminationRequestBody(segment, object, "SYN-DISPOSITION/tfa03-1", "2026-09-24T12:00:00+08:00")
	if ended := channel.post(channel.terminate, terminationPath, first); ended.Code != http.StatusCreated {
		t.Fatalf("首次终止：status = %d body = %s", ended.Code, ended.Body.String())
	}

	before := channel.fingerprint(t)
	for name, body := range map[string]string{
		"原样重放": first,
		"异依据":  terminationRequestBody(segment, object, "SYN-DISPOSITION/tfa03-2", "2026-09-24T12:00:00+08:00"),
		"异时刻":  terminationRequestBody(segment, object, "SYN-DISPOSITION/tfa03-1", "2026-09-24T18:00:00+08:00"),
	} {
		response := channel.post(channel.terminate, terminationPath, body)
		answer := decodeTerminationAnswer(t, response)
		recorded := terminationAnswer{Outcome: "PARTICIPATION_ALREADY_ENDED", EndKind: "CONTROL_TERMINATED", EndBasis: "SYN-DISPOSITION/tfa03-1", EndedAt: "2026-09-24T04:00:00Z"}
		if response.Code != http.StatusOK || answer != recorded {
			t.Fatalf("%s：status = %d answer = %+v, want 200 %+v", name, response.Code, answer, recorded)
		}
	}
	for name, body := range map[string]string{
		"缺依据":    terminationRequestBody(segment, object, "", "2026-09-24T12:00:00+08:00"),
		"依据全空白":  terminationRequestBody(segment, object, "   ", "2026-09-24T12:00:00+08:00"),
		"缺时刻":    terminationRequestBody(segment, object, "SYN-DISPOSITION/tfa03-1", ""),
		"终止早于入场": terminationRequestBody(segment, object, "SYN-DISPOSITION/tfa03-1", "2020-01-01T00:00:00Z"),
	} {
		response := channel.post(channel.terminate, terminationPath, body)
		if response.Code != http.StatusOK || decodeTerminationAnswer(t, response).Outcome != "INPUT_NOT_ACCEPTED" {
			t.Fatalf("已离场·%s：status = %d body = %s, want 200 INPUT_NOT_ACCEPTED", name, response.Code, response.Body.String())
		}
	}
	if changed := changedSince(before, channel.fingerprint(t)); len(changed) != 0 {
		t.Fatalf("重放与坏命令动了库：%v", changed)
	}
}

// 参与先被下一次权威交接结束，终止答已离场，回显的是那次交接的终点，不是本次终止的依据。
func TestATerminationAfterTheNextHandoverEndedItEchoesThatHandover(t *testing.T) {
	channel := newTerminationChannel(t)
	const object = "SYN-PARCEL-TFA03-2"
	channel.mustPost(t, channel.handovers, "/transport-fulfillment/handovers",
		handedOverBody(object, "SYN-SCOPE/hub-tfa03-a", "v1", "2026-09-24T09:00:00+08:00", "SYN-SEGMENT-TFA03-A"), http.StatusCreated)
	channel.mustPost(t, channel.handovers, "/transport-fulfillment/handovers",
		handedOverBody(object, "SYN-SCOPE/hub-tfa03-b", "v1", "2026-09-24T13:00:00+08:00", "SYN-SEGMENT-TFA03-B"), http.StatusCreated)

	response := channel.post(channel.terminate, terminationPath,
		terminationRequestBody("SYN-SEGMENT-TFA03-A", object, "SYN-DISPOSITION/tfa03-3", "2026-09-24T14:00:00+08:00"))
	answer := decodeTerminationAnswer(t, response)
	if response.Code != http.StatusOK || answer.Outcome != "PARTICIPATION_ALREADY_ENDED" || answer.EndKind != "NEXT_HANDOVER" ||
		answer.EndBasis == "SYN-DISPOSITION/tfa03-3" || answer.EndedAt != "2026-09-24T05:00:00Z" {
		t.Fatalf("status = %d answer = %+v, want 200 已离场并回显那次交接", response.Code, answer)
	}
}

// racingSegments 包住生产段登记册，在指定一步之后跑一次钩子。钩子跑在另一条连接、另一笔事务里，用来把
// 「另一事务恰在此刻提交了什么」确定地摆进来，而不是靠撞运气的并发。
type racingSegments struct {
	*tfpg.FulfillmentSegments
	afterFind       func()
	afterEnd        func()
	beforeSupersede func()
	afterSupersede  func()
}

func (racing *racingSegments) FindByKey(ctx context.Context, key tfports.FulfillmentSegmentKey) (tfports.FulfillmentSegmentRecord, bool, error) {
	record, found, err := racing.FulfillmentSegments.FindByKey(ctx, key)
	runOnce(&racing.afterFind)
	return record, found, err
}

func (racing *racingSegments) EndParticipation(ctx context.Context, key tfports.FulfillmentSegmentKey, participation tfdomain.FulfillmentParticipation) (tfports.ParticipationEndOutcome, error) {
	outcome, err := racing.FulfillmentSegments.EndParticipation(ctx, key, participation)
	runOnce(&racing.afterEnd)
	return outcome, err
}

func (racing *racingSegments) Supersede(ctx context.Context, key tfports.FulfillmentSegmentKey, participation tfdomain.FulfillmentParticipation, recordedAt time.Time) (tfports.ParticipationSupersedeOutcome, error) {
	runOnce(&racing.beforeSupersede)
	outcome, err := racing.FulfillmentSegments.Supersede(ctx, key, participation, recordedAt)
	runOnce(&racing.afterSupersede)
	return outcome, err
}

func runOnce(hook *func()) {
	if run := *hook; run != nil {
		*hook = nil
		run()
	}
}

func (channel terminationChannel) racingSegments(t *testing.T) *racingSegments {
	t.Helper()
	segments, err := tfpg.NewFulfillmentSegments(channel.db)
	if err != nil {
		t.Fatal(err)
	}
	return &racingSegments{FulfillmentSegments: segments}
}

// racingTerminate 与生产装配同形（同一个事务包装、同一个 Intake），只把段登记册换成带钩子的那一只。
func (channel terminationChannel) racingTerminate(segments *racingSegments) http.Handler {
	handler := tfapp.NewEndFulfillmentParticipationHandler(tfapp.EndFulfillmentParticipationDeps{Segments: segments, Clock: systemClock{}})
	ender := transactionalParticipationEnder{transactor: channel.db.Transactor(), inner: handler}
	return tfhttp.NewTerminateFulfillmentParticipationEndpoint(channel.decisions.transport, ender)
}

// 终止读完段、还没做窄口写时，另一事务提交了一次仍为已交接的交接更正：链尾被替代、仍在场。终止不得答已离场；
// 事后库面链尾带终点，或者答复是未决。
func TestAConcurrentTerminationLoserBehindASupersedingCorrectionDoesNotClaimTheEnd(t *testing.T) {
	channel := newTerminationChannel(t)
	const segment, object, scope = "SYN-SEGMENT-TFA03-R", "SYN-PARCEL-TFA03-R", "SYN-SCOPE/hub-tfa03-r"
	channel.mustPost(t, channel.handovers, "/transport-fulfillment/handovers",
		handedOverBody(object, scope, "v1", "2026-09-24T09:00:00+08:00", segment), http.StatusCreated)

	segments := channel.racingSegments(t)
	segments.afterFind = func() {
		channel.mustPost(t, channel.corrections, "/transport-fulfillment/handover-corrections",
			stillHandedOverCorrectionBody(object, scope, "v1", "v2", "2026-09-24T09:30:00+08:00"), http.StatusCreated)
	}
	response := channel.post(channel.racingTerminate(segments), terminationPath,
		terminationRequestBody(segment, object, "SYN-DISPOSITION/tfa03-r", "2026-09-24T12:00:00+08:00"))
	answer := decodeTerminationAnswer(t, response)
	if answer.Outcome == "PARTICIPATION_ALREADY_ENDED" {
		t.Fatalf("并发输家没读回就答已离场：%s；库面 %+v", response.Body.String(), channel.participationRows(t, segment, object))
	}
	tail := channel.chainTail(t, segment, object)
	if answer.Outcome != "PARTICIPATION_END_UNDECIDED" && tail.endedAt == nil {
		t.Fatalf("答复 %s，链尾却没有终点：%+v", response.Body.String(), channel.participationRows(t, segment, object))
	}
}
