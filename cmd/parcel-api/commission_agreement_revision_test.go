package main

import (
	"context"
	"net/http"
	"sort"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	tfhttp "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/http"
	tfpostgres "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/postgres"
	tfapp "go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	tfdomain "go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	tfports "go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// 本文件对真实 PostgreSQL 证委托协议换版口（ADR-0231 决定三）：生产装配、PC 真发布的演示干线协议（v1 不带条件、v2 声明实际履约按件计）、
// 操作者渠道真准入。换版成立落一版并同笔交一封门铃；重放、冲突与各格不落行不交门铃；持别的决定的人进不来；两笔并发的换版由换版锁排成
// 一前一后。夹具全部合成，证据只记 S。

const (
	revisionPath          = "/transport-fulfillment-transport-commission-agreement-revisions"
	revisionCommission    = "SYN-COMMISSION-TRUNK-DCG88"
	revisionLockSpace     = "transport_fulfillment.transport_commission.agreement_revision"
	revisionSubmittedAtTS = "2026-10-19T08:00:00Z"
	// revisionOperator 是 acceptingVerifier 认出的那一位操作者，写成「发行方#sub」。
	revisionOperator = "https://id.syn.example/dex#SYN-OPERATOR-01"
)

var revisionSubmittedAt = time.Date(2026, 10, 19, 8, 0, 0, 0, time.UTC)

type revisionFaces struct {
	commission http.Handler
	revision   http.Handler
}

func revisionFacesForTest(t *testing.T, db *bentopg.DB, kinds ...accessidentity.DecisionKind) revisionFaces {
	t.Helper()
	registerOperationDecisionAdmission(t, db, isolatedSelfAuthority,
		accessidentity.DecisionTransportCommissionSubmission, accessidentity.DecisionTransportCommissionAgreementRevision)
	admission, err := buildAdmissionScope(db, isolatedSelfAuthority)
	if err != nil {
		t.Fatal(err)
	}
	minter, err := buildOperatorMinter(acceptingVerifier{}, decisionRegister{kinds: kinds}, admission)
	if err != nil {
		t.Fatal(err)
	}
	decisions, err := buildOperatorDecisionIntakes(minter, unwiredDecisionTargets{}, unwiredDecisionTargets{})
	if err != nil {
		t.Fatal(err)
	}
	submitter, err := buildTransportCommissionSubmission(db)
	if err != nil {
		t.Fatalf("装配委托提交：%v", err)
	}
	reviser, err := buildCommissionAgreementRevision(db)
	if err != nil {
		t.Fatalf("装配换版：%v", err)
	}
	return revisionFaces{
		commission: tfhttp.NewSubmitTransportCommissionEndpoint(decisions.transport, submitter),
		revision:   tfhttp.NewReviseCommissionAgreementEndpoint(decisions.transport, reviser),
	}
}

func revisionBody(successorObject, successorVersion, appliesFrom, basis string) string {
	return `{"commission":"` + revisionCommission + `","successorAgreementObject":"` + successorObject +
		`","successorAgreementVersion":"` + successorVersion + `","appliesFrom":"` + appliesFrom + `","basis":"` + basis + `"}`
}

// submittedTrunkCommissionUnderV1 发布演示干线协议，经生产提交口提交一份指名 v1 的委托。
func submittedTrunkCommissionUnderV1(t *testing.T, db *bentopg.DB, faces revisionFaces) (string, string) {
	t.Helper()
	trunk := publishSeededSupplierAgreements(t, db)["SYN-SUPPLIER-TRUNK-CN-SG-01"]
	provider := trunk.Declarations.SupplierAgreementBody.Supplier
	if status, answer := postWithToken(t, faces.commission, commissionSubmissionPath, "presented.operator.token",
		commissionBody(revisionCommission, provider, trunk.ObjectID, "v1", revisionSubmittedAtTS)); status != http.StatusCreated {
		t.Fatalf("委托 = %d %v", status, answer)
	}
	return trunk.ObjectID, provider
}

func revisionRows(t *testing.T, pool *pgxpool.Pool) (rows int, doorbells int) {
	t.Helper()
	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM transport_fulfillment.transport_commission_agreement_revision`).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM `+migrate.SchemaBento+`.outbox WHERE event_type = $1`,
		tfpostgres.CommissionAgreementRevisedEventType).Scan(&doorbells); err != nil {
		t.Fatal(err)
	}
	return rows, doorbells
}

// Covers: AT-TF-298（S，ADR-0231 决定三）——委托指名 v1，换版到同一协议对象的 v2、自委托提交起用：201 COMMISSION_AGREEMENT_REVISED，回显第 2
// 版（PC 拼回的快照、采购责任法人、决定人是认证出的操作者），委托行原样、换版一行、门铃一封。同一份依据原样重交答 EXISTING_REVISION，换了
// 起用时刻答 REVISION_CONFLICT；换了协议对象、起用早于提交、后继不在册各答一格；这几次都不多落一行、不多交一封门铃。
func TestTheWiredRevisionFaceRevisesTheCommissionAndRingsOnce(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatal(err)
	}
	faces := revisionFacesForTest(t, db, accessidentity.DecisionTransportCommissionSubmission, accessidentity.DecisionTransportCommissionAgreementRevision)
	trunk, _ := submittedTrunkCommissionUnderV1(t, db, faces)
	const token = "presented.operator.token"

	status, answer := postWithToken(t, faces.revision, revisionPath, token, revisionBody(trunk, "v2", revisionSubmittedAtTS, "SYN-DECISION-REV-DCG88-01"))
	revision, _ := answer["revision"].(map[string]any)
	if status != http.StatusCreated || answer["outcome"] != "COMMISSION_AGREEMENT_REVISED" || revision["revision"] != float64(2) ||
		revision["agreementVersion"] != "v2" || revision["agreement"] == "" || revision["legalEntity"] == "" ||
		revision["decidedBy"] != revisionOperator || revision["appliesFrom"] != revisionSubmittedAtTS {
		t.Fatalf("换版 = %d %v", status, answer)
	}
	var agreementVersion string
	if err := pool.QueryRow(t.Context(), `SELECT agreement_version FROM transport_fulfillment.transport_commission
	                                       WHERE tenant_id = 'SYN-TENANT-01' AND commission_id = $1`, revisionCommission).Scan(&agreementVersion); err != nil ||
		agreementVersion != "v1" {
		t.Fatalf("委托行的协议版本 = %q %v, want 原样 v1", agreementVersion, err)
	}
	if rows, doorbells := revisionRows(t, pool); rows != 1 || doorbells != 1 {
		t.Fatalf("换版 %d 行、门铃 %d 封, want 各 1", rows, doorbells)
	}

	for body, want := range map[string]string{
		revisionBody(trunk, "v2", revisionSubmittedAtTS, "SYN-DECISION-REV-DCG88-01"):                         "EXISTING_REVISION",
		revisionBody(trunk, "v2", "2026-10-19T09:00:00Z", "SYN-DECISION-REV-DCG88-01"):                        "REVISION_CONFLICT",
		revisionBody("SYN-SUPPLIER-LASTMILE-SG-01", "v1", revisionSubmittedAtTS, "SYN-DECISION-REV-DCG88-02"): "AGREEMENT_OBJECT_MISMATCH",
		revisionBody(trunk, "v2", "2026-10-19T07:00:00Z", "SYN-DECISION-REV-DCG88-03"):                        "APPLIES_BEFORE_SUBMISSION",
		revisionBody(trunk, "v9", revisionSubmittedAtTS, "SYN-DECISION-REV-DCG88-04"):                         "AGREEMENT_NOT_REGISTERED",
	} {
		if status, answer := postWithToken(t, faces.revision, revisionPath, token, body); status != http.StatusOK || answer["outcome"] != want {
			t.Fatalf("%s = %d %v, want 200 %s", body, status, answer, want)
		}
	}
	if rows, doorbells := revisionRows(t, pool); rows != 1 || doorbells != 1 {
		t.Fatalf("各格之后换版 %d 行、门铃 %d 封, want 仍各 1", rows, doorbells)
	}
}

// Covers: AT-TF-297 的授予一半（S）——只持提交委托授予的操作者打不了换版口：403 OPERATOR_NOT_GRANTED，什么也不落。
func TestTheRevisionFaceRefusesAnOperatorWithoutTheRevisionGrant(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatal(err)
	}
	faces := revisionFacesForTest(t, db, accessidentity.DecisionTransportCommissionSubmission)
	trunk, _ := submittedTrunkCommissionUnderV1(t, db, faces)
	status, answer := postWithToken(t, faces.revision, revisionPath, "presented.operator.token",
		revisionBody(trunk, "v2", revisionSubmittedAtTS, "SYN-DECISION-REV-DCG88-01"))
	problem, _ := answer["error"].(map[string]any)
	if status != http.StatusForbidden || problem["code"] != "OPERATOR_NOT_GRANTED" {
		t.Fatalf("没有换版授予 = %d %v", status, answer)
	}
	if rows, doorbells := revisionRows(t, pool); rows != 0 || doorbells != 0 {
		t.Fatalf("被拒却落了 %d 行、%d 封门铃", rows, doorbells)
	}
}

// pausingRevisionDoorbell 是生产门铃口外面一层：换版行落下之后、门铃入队之前先跑 beforeHandOff——此刻握着换版锁、行还没提交。
type pausingRevisionDoorbell struct {
	tfports.CommissionAgreementRevisionHandoff
	beforeHandOff func()
}

func (doorbell pausingRevisionDoorbell) HandOffCommissionAgreementRevision(ctx context.Context, intent tfports.CommissionAgreementRevisedIntent) error {
	doorbell.beforeHandOff()
	return doorbell.CommissionAgreementRevisionHandoff.HandOffCommissionAgreementRevision(ctx, intent)
}

// Covers: AT-TF-298 的并发一半（S，ADR-0231 决定三「先取锁再读委托」）——A 落下第 2 版、还没提交时挂住；B 经生产装配换同一份委托的另一版，
// 必须等在换版锁上，A 提交后读到第 2 版、落第 3 版。拆掉换版锁，B 读到的仍是没换过版的委托、铸出同一个号，落行撞上 A 那一行，整口报错。
func TestConcurrentRevisionsOfOneCommissionAreSerializedByTheRevisionLock(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatal(err)
	}
	faces := revisionFacesForTest(t, db, accessidentity.DecisionTransportCommissionSubmission, accessidentity.DecisionTransportCommissionAgreementRevision)
	trunk, _ := submittedTrunkCommissionUnderV1(t, db, faces)
	production, err := buildCommissionAgreementRevision(db)
	if err != nil {
		t.Fatal(err)
	}
	deps, err := commissionAgreementRevisionDeps(db)
	if err != nil {
		t.Fatal(err)
	}
	tenant := mustValue(t, tfdomain.NewTenantID, "SYN-TENANT-01")
	command := func(version, basis string) tfapp.ReviseCommissionAgreementCommand {
		return tfapp.ReviseCommissionAgreementCommand{
			TenantID: tenant, DecidedBy: revisionOperator, Commission: revisionCommission,
			SuccessorObject: trunk, SuccessorVersion: version, AppliesFrom: revisionSubmittedAt, Basis: basis,
		}
	}
	var (
		secondResult tfapp.CommissionAgreementRevisionResult
		secondErr    error
		secondDone   chan struct{}
		waited       bool
	)
	deps.Doorbell = pausingRevisionDoorbell{CommissionAgreementRevisionHandoff: deps.Doorbell, beforeHandOff: func() {
		if secondDone != nil {
			return
		}
		secondDone = make(chan struct{})
		go func(done chan struct{}) {
			defer close(done)
			secondResult, secondErr = production.Revise(t.Context(), command("v3", "SYN-DECISION-REV-DCG88-B"))
		}(secondDone)
		waited = awaitAdvisoryWaiters(t, pool, map[string]int{revisionLockSpace: 1}, secondDone)
	}}
	paused := transactionalCommissionAgreementReviser{transactor: db.Transactor(), inner: tfapp.NewReviseCommissionAgreementHandler(deps)}

	firstResult, firstErr := paused.Revise(t.Context(), command("v2", "SYN-DECISION-REV-DCG88-A"))
	if secondDone == nil {
		t.Fatalf("A 没走到交门铃那一步：%s %v", firstResult.Outcome(), firstErr)
	}
	awaitDone(t, secondDone)
	if firstErr != nil || secondErr != nil {
		t.Fatalf("A err = %v，B err = %v", firstErr, secondErr)
	}
	if !waited {
		t.Fatal("B 没有等在换版锁上")
	}
	first, _ := firstResult.Revision()
	second, _ := secondResult.Revision()
	numbers := []int{first.Number(), second.Number()}
	sort.Ints(numbers)
	if firstResult.Outcome() != tfapp.CommissionAgreementRevised || secondResult.Outcome() != tfapp.CommissionAgreementRevised ||
		numbers[0] != 2 || numbers[1] != 3 || second.Number() != 3 {
		t.Fatalf("A = %s 第 %d 版，B = %s 第 %d 版, want 先 2 后 3", firstResult.Outcome(), first.Number(), secondResult.Outcome(), second.Number())
	}
	if rows, doorbells := revisionRows(t, pool); rows != 2 || doorbells != 2 {
		t.Fatalf("换版 %d 行、门铃 %d 封, want 各 2", rows, doorbells)
	}
}
