package main

import (
	"context"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	tfhttp "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/http"
	tfpostgres "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/postgres"
	tfapp "go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	tfdomain "go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	tfports "go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// 本文件对真实 PostgreSQL 证更正两口的生产装配（票 demo-chain-gaps/57，ADR-0232）：委托提交、段采用、发生项登记、发生项失效与解除
// 采用都经操作者渠道「运营决定」能力面走到真编排；干线协议照种子发布批经 PC 发布。与生产装配的差别只在令牌核验方与操作者册是
// 替身。夹具全为合成（S 级）。

const (
	occurrenceInvalidationPath = "/transport-fulfillment-charge-occurrence-invalidations"
	adoptionWithdrawalPath     = "/transport-fulfillment-segment-commission-adoption-withdrawals"
)

type correctionEndpoints struct {
	commission   http.Handler
	cancellation http.Handler
	adoption     http.Handler
	charge       http.Handler
	invalidation http.Handler
	withdrawal   http.Handler
}

// correctionKinds 是更正用例里操作者持有的全部决定种类。
var correctionKinds = []accessidentity.DecisionKind{
	accessidentity.DecisionTransportCommissionSubmission,
	accessidentity.DecisionTransportCommissionCancellation,
	accessidentity.DecisionFulfillmentSegmentCommissionAdoption,
	accessidentity.DecisionTransportChargeOccurrenceRegistration,
	accessidentity.DecisionTransportChargeOccurrenceInvalidation,
	accessidentity.DecisionFulfillmentSegmentCommissionAdoptionWithdrawal,
}

// correctionFacesForTest 装生产的委托提交与取消、段采用、发生项登记与两口更正；操作者持有 kinds 里的决定。
func correctionFacesForTest(t *testing.T, db *bentopg.DB, kinds ...accessidentity.DecisionKind) correctionEndpoints {
	t.Helper()
	if len(kinds) == 0 {
		kinds = correctionKinds
	}
	registerOperationDecisionAdmission(t, db, isolatedSelfAuthority, kinds...)
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
	commission, err := buildTransportCommissionSubmission(db)
	if err != nil {
		t.Fatalf("装配委托提交：%v", err)
	}
	adopter, err := buildSegmentCommissionAdoption(db)
	if err != nil {
		t.Fatalf("装配采用：%v", err)
	}
	registrar, err := buildChargeOccurrenceRegistration(db)
	if err != nil {
		t.Fatalf("装配发生项登记：%v", err)
	}
	invalidator, err := buildChargeOccurrenceInvalidation(db)
	if err != nil {
		t.Fatalf("装配发生项失效：%v", err)
	}
	withdrawer, err := buildSegmentCommissionAdoptionWithdrawal(db)
	if err != nil {
		t.Fatalf("装配解除采用：%v", err)
	}
	cancellations, err := buildCancellationFaces(db)
	if err != nil {
		t.Fatalf("装配取消口：%v", err)
	}
	return correctionEndpoints{
		commission:   tfhttp.NewSubmitTransportCommissionEndpoint(decisions.transport, commission),
		cancellation: tfhttp.NewCancelTransportCommissionEndpoint(decisions.transport, cancellations),
		adoption:     tfhttp.NewAdoptSegmentCommissionEndpoint(decisions.transport, adopter),
		charge:       tfhttp.NewRegisterChargeOccurrenceEndpoint(decisions.transport, registrar),
		invalidation: tfhttp.NewInvalidateChargeOccurrenceEndpoint(decisions.transport, invalidator),
		withdrawal:   tfhttp.NewWithdrawSegmentCommissionAdoptionEndpoint(decisions.transport, withdrawer),
	}
}

// adoptedTrunkOccurrence 在真库上立干线段、提交指名 v3 的干线委托、采用它并登一项实际履约，交回那一项的引用。
func adoptedTrunkOccurrence(t *testing.T, db *bentopg.DB, endpoints correctionEndpoints) (occurrence, provider string) {
	t.Helper()
	seeded := publishSeededSupplierAgreements(t, db)
	trunk := seeded["SYN-SUPPLIER-TRUNK-CN-SG-01"]
	provider = trunk.Declarations.SupplierAgreementBody.Supplier
	mustPost(t, endpoints.commission, commissionSubmissionPath,
		commissionBody(adoptedChargeCommiss, provider, trunk.ObjectID, trunk.Version, "2026-10-19T08:00:00Z"),
		http.StatusCreated, "COMMISSION_SUBMITTED")
	saveEndedTrunkSegment(t, db)
	mustPost(t, endpoints.adoption, adoptionPath, adoptionBody(adoptedChargeSegment, adoptedChargeCommiss, provider),
		http.StatusCreated, "COMMISSION_ADOPTED")
	answer := mustPost(t, endpoints.charge, chargeOccurrencePath, adoptedFulfillmentBody(), http.StatusCreated, "REGISTERED")
	registered, _ := answer["occurrence"].(map[string]any)
	occurrence, _ = registered["occurrence"].(string)
	return occurrence, provider
}

func adoptedFulfillmentBody() string {
	return `{"reason": "ACTUAL_FULFILLMENT", "segment": "` + adoptedChargeSegment + `", "objects": ["` + adoptedChargeParcel + `"],` +
		` "journey": "SYN-JRN-` + adoptedChargeParcel + `"}`
}

func invalidationBody(occurrence, version, basis string) string {
	return `{"occurrence": "` + occurrence + `", "version": "` + version + `", "basis": "` + basis + `"}`
}

// Covers: AT-TF-311、AT-TF-312 的生产装配半边——已采用委托的段上登了一项实际履约（V1），经失效口答 201、长出 V2 失效版本并同笔交出
// 一封信封；原样再交答 200 EXISTING_INVALIDATION、不多一行也不多一封。同一件事实再登不再答已存在，答 201、长出 V3 替代版本并交出
// 信封；再登答 EXISTING 回显 V3。
func TestAnOccurrenceIsInvalidatedAndReestablishedThroughTheProductionFaces(t *testing.T) {
	db := migratedChargeDB(t)
	endpoints := correctionFacesForTest(t, db)
	occurrence, _ := adoptedTrunkOccurrence(t, db, endpoints)

	answer := mustPost(t, endpoints.invalidation, occurrenceInvalidationPath,
		invalidationBody(occurrence, "V1", "SYN-EVIDENCE-DCG57"), http.StatusCreated, "OCCURRENCE_INVALIDATED")
	if invalidated, _ := answer["occurrence"].(map[string]any); invalidated["version"] != "V2" || invalidated["corrects"] != "V1" ||
		invalidated["revisionKind"] != "INVALIDATED" || invalidated["revisedBy"] == nil {
		t.Fatalf("失效回显 = %v", answer)
	}
	mustPost(t, endpoints.invalidation, occurrenceInvalidationPath,
		invalidationBody(occurrence, "V1", "SYN-EVIDENCE-DCG57"), http.StatusOK, "EXISTING_INVALIDATION")
	if occurrences, envelopes := countChargeRows(t, db); occurrences != 2 || envelopes != 2 {
		t.Fatalf("失效后 %d 行、%d 封，want 2 / 2", occurrences, envelopes)
	}

	answer = mustPost(t, endpoints.charge, chargeOccurrencePath, adoptedFulfillmentBody(), http.StatusCreated, "REGISTERED")
	if reestablished, _ := answer["occurrence"].(map[string]any); reestablished["occurrence"] != occurrence ||
		reestablished["version"] != "V3" || reestablished["corrects"] != "V2" || reestablished["revisionKind"] != "SUPERSEDED" {
		t.Fatalf("重新成立 = %v", answer)
	}
	answer = mustPost(t, endpoints.charge, chargeOccurrencePath, adoptedFulfillmentBody(), http.StatusOK, "EXISTING")
	if echoed, _ := answer["occurrence"].(map[string]any); echoed["version"] != "V3" {
		t.Fatalf("重放回显 = %v", answer)
	}
	if occurrences, envelopes := countChargeRows(t, db); occurrences != 3 || envelopes != 3 {
		t.Fatalf("重新成立后 %d 行、%d 封，want 3 / 3", occurrences, envelopes)
	}
	mustPost(t, endpoints.invalidation, occurrenceInvalidationPath,
		invalidationBody(occurrence, "V2", "SYN-EVIDENCE-DCG57"), http.StatusOK, "VERSION_NOT_CURRENT")
}

func withdrawalBody(segment, commission string, generation int) string {
	return `{"segment": "` + segment + `", "commission": "` + commission + `", "generation": ` + strconv.Itoa(generation) + `}`
}

// readoptionBody 是解除之后的再采用：点名新一代接在哪一代之后。
func readoptionBody(segment, commission, provider string, after int) string {
	return `{"segment": "` + segment + `", "commission": "` + commission + `", "provider": "` + provider +
		`", "afterGeneration": ` + strconv.Itoa(after) + `}`
}

func countAdoptionRows(t *testing.T, db *bentopg.DB) (adoptions, withdrawals int) {
	t.Helper()
	querier, err := db.ReadExecutor(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if err := querier.QueryRow(t.Context(), `SELECT count(*) FROM transport_fulfillment.segment_commission_adoption`).Scan(&adoptions); err != nil {
		t.Fatal(err)
	}
	if err := querier.QueryRow(t.Context(), `SELECT count(*) FROM transport_fulfillment.segment_commission_adoption_withdrawal`).Scan(&withdrawals); err != nil {
		t.Fatal(err)
	}
	return adoptions, withdrawals
}

// Covers: AT-TF-306、AT-TF-307、AT-TF-309、AT-TF-310 的生产装配半边——段上已按委托形成了发生项时解除答 200 OCCURRENCES_FORMED 并回显那一项；
// 经失效口失效之后解除答 201，委托撤回到已提交（回显的委托不带开始），原采用与解除都留在册上；原样再交答 EXISTING_WITHDRAWAL。撤回
// 之后的委托取消得了——这正是解除要放行的那一步。另一路：解除之后再采用同一份委托落成第二代，同一件事实再登从失效版本长出替代版本。
func TestAnAdoptionIsWithdrawnOnlyAfterItsOccurrencesAreInvalidated(t *testing.T) {
	db := migratedChargeDB(t)
	endpoints := correctionFacesForTest(t, db)
	occurrence, provider := adoptedTrunkOccurrence(t, db, endpoints)

	refused := mustPost(t, endpoints.withdrawal, adoptionWithdrawalPath, withdrawalBody(adoptedChargeSegment, adoptedChargeCommiss, 1),
		http.StatusOK, "OCCURRENCES_FORMED")
	if formed, _ := refused["formedOccurrences"].([]any); len(formed) != 1 || formed[0].(map[string]any)["occurrence"] != occurrence {
		t.Fatalf("已形成发生项 = %v", refused)
	}
	mustPost(t, endpoints.invalidation, occurrenceInvalidationPath, invalidationBody(occurrence, "V1", "SYN-EVIDENCE-DCG57"),
		http.StatusCreated, "OCCURRENCE_INVALIDATED")

	answer := mustPost(t, endpoints.withdrawal, adoptionWithdrawalPath, withdrawalBody(adoptedChargeSegment, adoptedChargeCommiss, 1),
		http.StatusCreated, "ADOPTION_WITHDRAWN")
	withdrawal, _ := answer["withdrawal"].(map[string]any)
	commission, _ := answer["commission"].(map[string]any)
	if withdrawal["startChange"] != "WITHDRAWN" || withdrawal["generation"] != float64(1) || withdrawal["startBeforeAt"] != "2026-10-19T09:00:00Z" ||
		withdrawal["withdrawnBy"] == nil || commission["startedAt"] != nil {
		t.Fatalf("解除 = %v", answer)
	}
	mustPost(t, endpoints.withdrawal, adoptionWithdrawalPath, withdrawalBody(adoptedChargeSegment, adoptedChargeCommiss, 1),
		http.StatusOK, "EXISTING_WITHDRAWAL")
	if adoptions, withdrawals := countAdoptionRows(t, db); adoptions != 1 || withdrawals != 1 {
		t.Fatalf("采用 %d 行、解除 %d 行，want 1 / 1", adoptions, withdrawals)
	}

	mustPost(t, endpoints.adoption, adoptionPath, readoptionBody(adoptedChargeSegment, adoptedChargeCommiss, provider, 1),
		http.StatusCreated, "COMMISSION_ADOPTED")
	answer = mustPost(t, endpoints.charge, chargeOccurrencePath, adoptedFulfillmentBody(), http.StatusCreated, "REGISTERED")
	if reestablished, _ := answer["occurrence"].(map[string]any); reestablished["version"] != "V3" || reestablished["corrects"] != "V2" {
		t.Fatalf("再采用之后重登 = %v", answer)
	}
	if adoptions, withdrawals := countAdoptionRows(t, db); adoptions != 2 || withdrawals != 1 {
		t.Fatalf("再采用后采用 %d 行、解除 %d 行，want 2 / 1", adoptions, withdrawals)
	}
}

// Covers: AT-TF-307 末句的生产装配半边——错采用的委托解除之后回到已提交，取消委托答 201；解除之前同一份取消答运输已开始。
func TestAWithdrawnCommissionCanBeCancelledAgain(t *testing.T) {
	db := migratedChargeDB(t)
	endpoints := correctionFacesForTest(t, db)
	seeded := publishSeededSupplierAgreements(t, db)
	trunk := seeded["SYN-SUPPLIER-TRUNK-CN-SG-01"]
	provider := trunk.Declarations.SupplierAgreementBody.Supplier
	mustPost(t, endpoints.commission, commissionSubmissionPath,
		commissionBody(adoptedChargeCommiss, provider, trunk.ObjectID, trunk.Version, "2026-10-19T08:00:00Z"),
		http.StatusCreated, "COMMISSION_SUBMITTED")
	saveEndedTrunkSegment(t, db)
	mustPost(t, endpoints.adoption, adoptionPath, adoptionBody(adoptedChargeSegment, adoptedChargeCommiss, provider),
		http.StatusCreated, "COMMISSION_ADOPTED")
	cancellation := `{"commission": "` + adoptedChargeCommiss + `", "cancelledAt": "2026-10-19T13:00:00Z"}`
	mustPost(t, endpoints.cancellation, "/transport-fulfillment-transport-commission-cancellations", cancellation,
		http.StatusOK, "TRANSPORT_ALREADY_STARTED")
	mustPost(t, endpoints.withdrawal, adoptionWithdrawalPath, withdrawalBody(adoptedChargeSegment, adoptedChargeCommiss, 1),
		http.StatusCreated, "ADOPTION_WITHDRAWN")
	mustPost(t, endpoints.cancellation, "/transport-fulfillment-transport-commission-cancellations", cancellation,
		http.StatusCreated, "COMMISSION_CANCELLED")
}

// Covers: AT-TF-306 授予那一半——只持有采用与登记的人进不来解除口，答 403 OPERATOR_NOT_GRANTED；失效口同理。
func TestTheCorrectionFacesRefuseAnOperatorWithoutTheirGrants(t *testing.T) {
	db := migratedChargeDB(t)
	endpoints := correctionFacesForTest(t, db,
		accessidentity.DecisionFulfillmentSegmentCommissionAdoption, accessidentity.DecisionTransportChargeOccurrenceRegistration)
	for path, endpoint := range map[string]http.Handler{
		adoptionWithdrawalPath:     endpoints.withdrawal,
		occurrenceInvalidationPath: endpoints.invalidation,
	} {
		status, answer := postWithToken(t, endpoint, path, operatorTokenForTests, withdrawalBody(adoptedChargeSegment, adoptedChargeCommiss, 1))
		if problem, _ := answer["error"].(map[string]any); status != http.StatusForbidden || problem["code"] != "OPERATOR_NOT_GRANTED" {
			t.Fatalf("%s 无授予 = %d %v", path, status, answer)
		}
	}
}

// adoptedTrunkSegmentForRace 立干线段、提交委托并采用它，交回池与库；并发用例在它上面让解除与登记赛跑。
func adoptedTrunkSegmentForRace(t *testing.T) (*pgxpool.Pool, *bentopg.DB) {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	endpoints := correctionFacesForTest(t, db)
	seeded := publishSeededSupplierAgreements(t, db)
	trunk := seeded["SYN-SUPPLIER-TRUNK-CN-SG-01"]
	provider := trunk.Declarations.SupplierAgreementBody.Supplier
	mustPost(t, endpoints.commission, commissionSubmissionPath,
		commissionBody(adoptedChargeCommiss, provider, trunk.ObjectID, trunk.Version, "2026-10-19T08:00:00Z"),
		http.StatusCreated, "COMMISSION_SUBMITTED")
	saveEndedTrunkSegment(t, db)
	mustPost(t, endpoints.adoption, adoptionPath, adoptionBody(adoptedChargeSegment, adoptedChargeCommiss, provider),
		http.StatusCreated, "COMMISSION_ADOPTED")
	return pool, db
}

func raceRegistrationCommand(t *testing.T) tfapp.RegisterChargeOccurrenceCommand {
	t.Helper()
	return tfapp.RegisterChargeOccurrenceCommand{
		TenantID:   chargeValue(t, tfdomain.NewTenantID, chargeTenant),
		Registrant: "https://id.syn.example/dex#SYN-OPERATOR-RACE",
		Reason:     tfdomain.ActualFulfillmentOccurrence,
		Segment:    adoptedChargeSegment,
		Objects:    []string{adoptedChargeParcel},
		Journey:    "SYN-JRN-" + adoptedChargeParcel,
	}
}

func raceWithdrawalCommand(t *testing.T) tfapp.WithdrawSegmentCommissionAdoptionCommand {
	t.Helper()
	return tfapp.WithdrawSegmentCommissionAdoptionCommand{
		TenantID:   chargeValue(t, tfdomain.NewTenantID, chargeTenant),
		Operator:   "https://id.syn.example/dex#SYN-OPERATOR-RACE",
		Segment:    adoptedChargeSegment,
		Commission: adoptedChargeCommiss,
		Generation: 1,
	}
}

// Covers: AT-TF-314——解除与发生项形成并发、登记先握锁：登记落了册还没提交时经生产装配起一次解除，解除等在这一段的发生项范围锁上；
// 登记提交之后解除读到那一项，答 OCCURRENCES_FORMED、什么都不写。两者不会都判通过：拆掉解除那一步取锁，解除看不见尚未提交的那一项，
// 就会答解除成立。
func TestAWithdrawalRacingARegistrationWaitsAndSeesTheOccurrence(t *testing.T) {
	pool, db := adoptedTrunkSegmentForRace(t)
	withdrawer, err := buildSegmentCommissionAdoptionWithdrawal(db)
	if err != nil {
		t.Fatal(err)
	}
	deps, err := chargeOccurrenceDeps(db)
	if err != nil {
		t.Fatal(err)
	}
	var (
		withdrawn   tfapp.AdoptionWithdrawalResult
		withdrawErr error
		done        chan struct{}
		waited      bool
	)
	deps.Handoff = pausingChargeHandoff{ChargeOccurrenceHandoff: deps.Handoff, beforeHandOff: func() {
		if done != nil {
			return
		}
		done = make(chan struct{})
		go func(finished chan struct{}) {
			defer close(finished)
			withdrawn, withdrawErr = withdrawer.Withdraw(t.Context(), raceWithdrawalCommand(t))
		}(done)
		waited = awaitAdvisoryWaiters(t, pool, map[string]int{chargeOccurrenceLockSpace: 1}, done)
	}}
	registrar := transactionalChargeOccurrenceRegistrar{transactor: db.Transactor(), inner: tfapp.NewRegisterChargeOccurrenceHandler(deps)}
	registered, err := registrar.Register(t.Context(), raceRegistrationCommand(t))
	if done == nil {
		t.Fatalf("登记没走到交意图那一步：%q/%q %v", registered.Outcome(), registered.Detail(), err)
	}
	awaitDone(t, done)
	if err != nil || withdrawErr != nil {
		t.Fatalf("登记 err = %v，解除 err = %v", err, withdrawErr)
	}
	if !waited {
		t.Fatal("解除没在发生项范围锁上等就走完了")
	}
	if registered.Outcome() != tfapp.ChargeOccurrenceRegistered || withdrawn.Outcome() != tfapp.AdoptionWithdrawalOccurrencesFormed {
		t.Fatalf("登记 %q、解除 %q，want REGISTERED / OCCURRENCES_FORMED", registered.Outcome(), withdrawn.Outcome())
	}
	if adoptions, withdrawals := countAdoptionRows(t, db); adoptions != 1 || withdrawals != 0 {
		t.Fatalf("采用 %d 行、解除 %d 行，want 1 / 0", adoptions, withdrawals)
	}
}

// pausingStartCorrection 是生产改开始写口外面一层：写之前先跑 before。
type pausingStartCorrection struct {
	tfports.TransportCommissionStartCorrection
	before func()
}

func (correction pausingStartCorrection) CorrectStart(
	ctx context.Context, record tfports.TransportCommissionRecord, basis tfdomain.ParticipationBasisReference, at time.Time,
) (bool, error) {
	correction.before()
	return correction.TransportCommissionStartCorrection.CorrectStart(ctx, record, basis, at)
}

// Covers: AT-TF-314 的另一个方向——解除先握锁：解除落了解除行还没提交时经生产装配起一次登记，登记读到的仍是那条采用、等在范围锁上；
// 解除提交之后登记在锁下重读采用，已不在，答未决 PROCUREMENT_SOURCE_CHANGED、一行不落。拆掉登记在锁下的重读，登记就按已解除的那份
// 委托落一项发生项。
func TestARegistrationRacingAWithdrawalWaitsAndRereadsTheAdoption(t *testing.T) {
	pool, db := adoptedTrunkSegmentForRace(t)
	registrar, err := buildChargeOccurrenceRegistration(db)
	if err != nil {
		t.Fatal(err)
	}
	adoptions, err := tfpostgres.NewSegmentCommissionAdoptions(db)
	if err != nil {
		t.Fatal(err)
	}
	occurrences, err := tfpostgres.NewChargeOccurrences(db)
	if err != nil {
		t.Fatal(err)
	}
	commissions, err := tfpostgres.NewTransportCommissions(db)
	if err != nil {
		t.Fatal(err)
	}
	var (
		registered  tfapp.RegisterChargeOccurrenceResult
		registerErr error
		done        chan struct{}
		waited      bool
	)
	paused := pausingStartCorrection{TransportCommissionStartCorrection: commissions, before: func() {
		done = make(chan struct{})
		go func(finished chan struct{}) {
			defer close(finished)
			registered, registerErr = registrar.Register(t.Context(), raceRegistrationCommand(t))
		}(done)
		waited = awaitAdvisoryWaiters(t, pool, map[string]int{chargeOccurrenceLockSpace: 1}, done)
	}}
	withdrawer := transactionalSegmentCommissionWithdrawer{transactor: db.Transactor(), inner: tfapp.NewWithdrawSegmentCommissionAdoptionHandler(
		tfapp.WithdrawSegmentCommissionAdoptionDeps{
			Adoptions: adoptions, Withdrawals: adoptions, CommissionLinks: adoptions,
			Occurrences: occurrences, OccurrenceScope: occurrences,
			Commissions: commissions, CommissionHold: commissions, StartCorrection: paused,
			Clock: systemClock{},
		})}
	withdrawn, err := withdrawer.Withdraw(t.Context(), raceWithdrawalCommand(t))
	if done == nil {
		t.Fatalf("解除没走到写委托开始那一步：%q %v", withdrawn.Outcome(), err)
	}
	awaitDone(t, done)
	if err != nil || registerErr != nil {
		t.Fatalf("解除 err = %v，登记 err = %v", err, registerErr)
	}
	if !waited {
		t.Fatal("登记没在发生项范围锁上等就走完了")
	}
	if withdrawn.Outcome() != tfapp.AdoptionWithdrawn || registered.Outcome() != tfapp.ChargeOccurrenceUndecided ||
		registered.Detail() != tfapp.ProcurementSourceChanged {
		t.Fatalf("解除 %q、登记 %q/%q，want ADOPTION_WITHDRAWN / UNDECIDED PROCUREMENT_SOURCE_CHANGED",
			withdrawn.Outcome(), registered.Outcome(), registered.Detail())
	}
	if rows, envelopes := countChargeRows(t, db); rows != 0 || envelopes != 0 {
		t.Fatalf("发生项 %d 行、信封 %d 封，want 0 / 0", rows, envelopes)
	}
}

// commissionStartLockSpace 是委托开始锁的锁空间名，与 PG 适配器的同一个串。
const commissionStartLockSpace = "transport_fulfillment.transport_commission.start"

// saveEndedSegmentOf 在真库上立一个已结束的干线段：一件包裹在 enteredAt 凭交接入场、12:00 离场。
func saveEndedSegmentOf(t *testing.T, db *bentopg.DB, segment, parcel string, enteredAt time.Time) {
	t.Helper()
	built, err := tfdomain.RehydrateActualFulfillmentSegment(tfdomain.RehydrateActualFulfillmentSegmentSpec{
		TenantID: chargeValue(t, tfdomain.NewTenantID, chargeTenant),
		Segment:  chargeValue(t, tfdomain.NewFulfillmentSegmentReference, segment),
		Participations: []tfdomain.RehydrateParticipationSpec{{
			Object:     chargeValue(t, tfdomain.NewCarriedObjectReference, parcel),
			EntryKind:  tfdomain.EnteredByTransportHandover,
			EntryBasis: chargeValue(t, tfdomain.NewParticipationBasisReference, "TRANSPORT-HANDOVER/SYN-HO-"+segment),
			EnteredAt:  enteredAt,
			EndKind:    tfdomain.EndedByNextHandover,
			EndBasis:   chargeValue(t, tfdomain.NewParticipationBasisReference, "TRANSPORT-HANDOVER/SYN-HO-END-"+segment),
			EndedAt:    time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC),
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	segments, err := tfpostgres.NewFulfillmentSegments(db)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
		_, saveErr := segments.Save(txCtx, tfports.FulfillmentSegmentRecord{
			Key: tfports.FulfillmentSegmentKey{TenantID: built.TenantID(), Segment: built.Segment()}, Segment: built,
			RecordedAt: time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC),
		})
		return saveErr
	}); err != nil {
		t.Fatal(err)
	}
}

// pausingAdoptionSave 是生产采用册外面一层：落了连线之后先跑 after。
type pausingAdoptionSave struct {
	*tfpostgres.SegmentCommissionAdoptions
	after func()
}

func (store pausingAdoptionSave) Save(ctx context.Context, record tfports.SegmentCommissionAdoptionRecord) (tfports.AdoptionSaveOutcome, error) {
	outcome, err := store.SegmentCommissionAdoptions.Save(ctx, record)
	if store.after != nil {
		store.after()
	}
	return outcome, err
}

// Covers: AT-TF-315——采用与解除在同一份委托上并发：一段开始得更晚的采用（委托的开始不动、不写委托）落了连线还没提交时经生产装配起
// 一次解除，解除等在委托的开始锁上；采用提交之后解除读得到那一段，开始改按它重新派生，不撤回到已提交。拆掉采用那一步取锁，解除看不见
// 尚未提交的那一段，就把委托撤回到已提交，而那一段随即挂在一份未开始的委托上。
func TestAnAdoptionRacingAWithdrawalOfTheSameCommissionSerializesOnItsStart(t *testing.T) {
	pool, db := adoptedTrunkSegmentForRace(t)
	const laterSegment, laterParcel = "SYN-SEG-SZX-SIN-DCG57-LATER", "SYN-PARCEL-DCG57-LATER"
	laterAt := time.Date(2026, 10, 19, 10, 0, 0, 0, time.UTC)
	saveEndedSegmentOf(t, db, laterSegment, laterParcel, laterAt)
	withdrawer, err := buildSegmentCommissionAdoptionWithdrawal(db)
	if err != nil {
		t.Fatal(err)
	}
	adoptions, err := tfpostgres.NewSegmentCommissionAdoptions(db)
	if err != nil {
		t.Fatal(err)
	}
	segments, err := tfpostgres.NewFulfillmentSegments(db)
	if err != nil {
		t.Fatal(err)
	}
	commissions, err := tfpostgres.NewTransportCommissions(db)
	if err != nil {
		t.Fatal(err)
	}
	var (
		withdrawn   tfapp.AdoptionWithdrawalResult
		withdrawErr error
		done        chan struct{}
		waited      bool
	)
	paused := pausingAdoptionSave{SegmentCommissionAdoptions: adoptions, after: func() {
		done = make(chan struct{})
		go func(finished chan struct{}) {
			defer close(finished)
			withdrawn, withdrawErr = withdrawer.Withdraw(t.Context(), raceWithdrawalCommand(t))
		}(done)
		waited = awaitAdvisoryWaiters(t, pool, map[string]int{commissionStartLockSpace: 1}, done)
	}}
	store, err := outbox.NewStore(db)
	if err != nil {
		t.Fatal(err)
	}
	doorbell, err := tfpostgres.NewOutboxSegmentCommissionAdoptionHandoff(db, store, systemClock{})
	if err != nil {
		t.Fatal(err)
	}
	adopter := transactionalSegmentCommissionAdopter{transactor: db.Transactor(), inner: tfapp.NewAdoptSegmentCommissionHandler(tfapp.AdoptSegmentCommissionDeps{
		Adoptions: paused, Segments: segments, Commissions: commissions, StartAdvancement: commissions, CommissionHold: commissions,
		Withdrawals: adoptions, Doorbell: doorbell, Clock: systemClock{},
	})}
	adopted, err := adopter.Adopt(t.Context(), tfapp.AdoptSegmentCommissionCommand{
		TenantID: chargeValue(t, tfdomain.NewTenantID, chargeTenant), Segment: laterSegment, Commission: adoptedChargeCommiss,
		Provider: "SYN-PARTY-CARRIER-TRUNK-01",
	})
	if done == nil {
		t.Fatalf("采用没走到落连线那一步：%q %v", adopted.Outcome(), err)
	}
	awaitDone(t, done)
	if err != nil || withdrawErr != nil {
		t.Fatalf("采用 err = %v，解除 err = %v", err, withdrawErr)
	}
	if !waited {
		t.Fatal("解除没在委托的开始锁上等就走完了")
	}
	record, _ := withdrawn.Withdrawal()
	afterBasis, afterAt, present := record.Withdrawal.StartAfter()
	if adopted.Outcome() != tfapp.CommissionAdopted || withdrawn.Outcome() != tfapp.AdoptionWithdrawn ||
		record.Withdrawal.StartChange() != tfdomain.CommissionStartRederived || !present ||
		afterBasis.String() != "TRANSPORT-HANDOVER/SYN-HO-"+laterSegment || !afterAt.Equal(laterAt) {
		t.Fatalf("采用 %q、解除 %q %+v", adopted.Outcome(), withdrawn.Outcome(), record.Withdrawal)
	}
}

// Covers: AT-TF-310 的生产装配半边（ADR-0232 决定一，评审 B1 复现）——采用答回显带代次；解除之后，解除之前那一次采用请求原样重交答
// 200 ADOPTION_ALREADY_WITHDRAWN 并回显那一次解除，不落新一代；点名接在第 1 代之后的再采用落成第 2 代、委托重新开始；再采用之后第 1 代
// 那一次解除原样重交答 200 EXISTING_WITHDRAWAL，不解除第 2 代。册上采用 2 行、解除 1 行。
func TestReplayingAnOldAdoptionOrWithdrawalHasNoNewEffect(t *testing.T) {
	db := migratedChargeDB(t)
	endpoints := correctionFacesForTest(t, db)
	seeded := publishSeededSupplierAgreements(t, db)
	trunk := seeded["SYN-SUPPLIER-TRUNK-CN-SG-01"]
	provider := trunk.Declarations.SupplierAgreementBody.Supplier
	mustPost(t, endpoints.commission, commissionSubmissionPath,
		commissionBody(adoptedChargeCommiss, provider, trunk.ObjectID, trunk.Version, "2026-10-19T08:00:00Z"),
		http.StatusCreated, "COMMISSION_SUBMITTED")
	saveEndedTrunkSegment(t, db)
	adopt := adoptionBody(adoptedChargeSegment, adoptedChargeCommiss, provider)
	answer := mustPost(t, endpoints.adoption, adoptionPath, adopt, http.StatusCreated, "COMMISSION_ADOPTED")
	if adoption, _ := answer["adoption"].(map[string]any); adoption["generation"] != float64(1) {
		t.Fatalf("首次采用回显 = %v", answer)
	}
	withdraw := withdrawalBody(adoptedChargeSegment, adoptedChargeCommiss, 1)
	mustPost(t, endpoints.withdrawal, adoptionWithdrawalPath, withdraw, http.StatusCreated, "ADOPTION_WITHDRAWN")

	answer = mustPost(t, endpoints.adoption, adoptionPath, adopt, http.StatusOK, "ADOPTION_ALREADY_WITHDRAWN")
	if withdrawn, _ := answer["withdrawal"].(map[string]any); withdrawn["generation"] != float64(1) {
		t.Fatalf("已被解除回显 = %v", answer)
	}
	if adoptions, withdrawals := countAdoptionRows(t, db); adoptions != 1 || withdrawals != 1 {
		t.Fatalf("旧采用重放之后采用 %d 行、解除 %d 行，want 1 / 1", adoptions, withdrawals)
	}

	answer = mustPost(t, endpoints.adoption, adoptionPath, readoptionBody(adoptedChargeSegment, adoptedChargeCommiss, provider, 1),
		http.StatusCreated, "COMMISSION_ADOPTED")
	if adoption, _ := answer["adoption"].(map[string]any); adoption["generation"] != float64(2) {
		t.Fatalf("再采用回显 = %v", answer)
	}
	mustPost(t, endpoints.withdrawal, adoptionWithdrawalPath, withdraw, http.StatusOK, "EXISTING_WITHDRAWAL")
	answer = mustPost(t, endpoints.withdrawal, adoptionWithdrawalPath, withdrawalBody(adoptedChargeSegment, adoptedChargeCommiss, 3),
		http.StatusOK, "GENERATION_NOT_CURRENT")
	if adoption, _ := answer["adoption"].(map[string]any); adoption["generation"] != float64(2) {
		t.Fatalf("代次不是当前回显 = %v", answer)
	}
	if adoptions, withdrawals := countAdoptionRows(t, db); adoptions != 2 || withdrawals != 1 {
		t.Fatalf("采用 %d 行、解除 %d 行，want 2 / 1", adoptions, withdrawals)
	}
}
