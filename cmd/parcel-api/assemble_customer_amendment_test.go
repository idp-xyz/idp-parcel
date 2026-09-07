package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	shipmenthttp "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/http"
	pspartycommercial "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/partycommercial"
	pspostgres "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres"
	shipmentapp "go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件对真实 PostgreSQL 16 证资料修订编排的装配（票 ps-port-remainder/04，形照 ADR-0106 决定四）：
// 入口接上之后编排不是从此能改资料，而是**说得出停在哪**——生产装配下停在授权未决（提供方授权那半
// 未立，票 03），授权过了停在矩阵未登记（提供方矩阵那半未立，票 02）；两处都不形成版本、不入队
// 「资料版本已形成」意图。第三条钉住交接壳：意图在自己的事务里入队恰好一封、重放不翻倍——版本与
// 意图不同事务是编排今天的形状，壳只负责把 RequireExecutor 那一格接对。第四条把第一个停点推到
// HTTP 面上：端点用例里带业务结果的字段未导出、逐格映射说好由本包经真编排补，补在这里。

// sourceDataVersionEnvelopeType 与发布侧适配器的类型常量同字面（手抄，理由同 submittedEnvelopeType）。
const sourceDataVersionEnvelopeType = "parcel-shipment.source-data-version.formed"

// amendmentCommand 是对 submissionCommand 那份委托的一次资料修订：修订请求有自己的来源身份与载荷
// 摘要，范围取整份委托的一个资料组，意图为补充，基准为接受基线。
func amendmentCommand(t *testing.T, amendmentKey string) shipmentapp.AmendCustomerSourceDataCommand {
	t.Helper()
	original := submissionCommand(t)
	amendmentIdentity, err := domain.NewSourceIdentity(
		original.Identity.TenantID(),
		original.Identity.CustomerAccountID(),
		original.Identity.Source(),
		mustValue(t, domain.NewSourceRequestKey, amendmentKey),
	)
	if err != nil {
		t.Fatalf("new amendment identity: %v", err)
	}
	scope, err := domain.NewShipmentScopedSourceData(
		original.ShipmentRequestID,
		mustValue(t, domain.NewSourceDataGroupReference, "SYN-GROUP-CONSIGNEE"),
	)
	if err != nil {
		t.Fatalf("new source data scope: %v", err)
	}
	return shipmentapp.AmendCustomerSourceDataCommand{
		Identity:          original.Identity,
		AmendmentIdentity: amendmentIdentity,
		PayloadDigest:     mustValue(t, domain.NewPayloadDigest, "syn-amendment-digest-1"),
		OccurredAt:        time.Date(2026, 8, 21, 12, 0, 0, 0, time.UTC),
		ReceivedAt:        time.Date(2026, 8, 21, 12, 0, 1, 0, time.UTC),
		Scope:             scope,
		Basis:             domain.NewSupplementOnAcceptanceBaseline(),
		Intent:            domain.SupplementIntent,
		Reason:            mustValue(t, domain.NewAmendmentReasonReference, "SYN-REASON-SUPPLEMENT"),
		Requester:         mustValue(t, domain.NewRequesterReference, "SYN-CUSTOMER-REQUESTER"),
	}
}

// submittedOnRealAssembly 经真提交装出一份`已提交`委托（归属用放行替身，理由见 envelopeMintingSubmission）。
// 两个诚实停点都在编排读到委托之后、改写委托之前，`已提交`就够用；接受那一段不在本包装配点内。
func submittedOnRealAssembly(t *testing.T, db *bentopg.DB) {
	t.Helper()
	submission, _ := envelopeMintingSubmission(t, db)
	submitted, err := submission.Handle(t.Context(), submissionCommand(t))
	if err != nil {
		t.Fatalf("首次提交：%v", err)
	}
	if got := submitted.Outcome(); got != shipmentapp.OutcomeSubmitted {
		t.Fatalf("outcome = %v, want SUBMITTED", got)
	}
}

func assertNoSourceDataVersionFormed(t *testing.T, db *bentopg.DB) {
	t.Helper()
	if got := envelopeCountOfType(t, db, sourceDataVersionEnvelopeType); got != 0 {
		t.Fatalf("入队了 %d 封「资料版本已形成」意图——没形成版本就没有可交的意图", got)
	}
	requests, err := pspostgres.NewShipmentRequests(db)
	if err != nil {
		t.Fatalf("构造委托仓储：%v", err)
	}
	request, found, err := requests.FindBySourceIdentity(t.Context(), submissionCommand(t).Identity)
	if err != nil || !found {
		t.Fatalf("读回委托：err=%v found=%v", err, found)
	}
	if versions := request.CustomerSourceDataVersions(); len(versions) != 0 {
		t.Fatalf("委托上多了 %d 份资料版本——停点之后不该有任何写入", len(versions))
	}
}

// preservedSourceRows 数某个来源身份在来源保全表里的行数。停点不抹掉「请求到达过」：编排在问授权
// 之前先保全修订请求自己的来源身份（AmendCustomerSourceDataHandler.Handle 头几步），停在未决之后
// 这一行仍在，重放同一身份才认得出是重放。
func preservedSourceRows(t *testing.T, db *bentopg.DB, identity domain.SourceIdentity) int {
	t.Helper()
	querier, err := db.ReadExecutor(t.Context())
	if err != nil {
		t.Fatalf("取读执行器：%v", err)
	}
	var count int
	err = querier.QueryRow(t.Context(),
		`SELECT count(*) FROM parcel_shipment.source_submission
		  WHERE tenant_id = $1 AND customer_account_id = $2 AND source = $3 AND source_request_key = $4`,
		identity.TenantID().String(), identity.CustomerAccountID().String(),
		identity.Source().String(), identity.RequestKey().String(),
	).Scan(&count)
	if err != nil {
		t.Fatalf("统计来源保全行数：%v", err)
	}
	return count
}

// Covers: 票 04 装配用例①——生产装配下修订请求越过 Intake 后停在**授权未决**，原因是
// `SourceDataAmendmentAuthorityRulesNotConfigured`（提供方那半未立），带续办引用；不形成版本、不入队意图；
// 停点之前来源保全已留痕。这一格与「客户越权」（NOT_AUTHORIZED）必须分得开：没有规则不是有人被拒。
func TestTheProductionAmendmentAssemblyStopsAtUnconfiguredAuthorization(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	submittedOnRealAssembly(t, db)

	amendment, err := buildCustomerAmendmentOrchestration(db)
	if err != nil {
		t.Fatalf("装配资料修订编排：%v", err)
	}
	command := amendmentCommand(t, "SYN-KEY-1-AMENDMENT")
	result, err := amendment.Handle(t.Context(), command)
	if err != nil {
		t.Fatalf("资料修订：%v", err)
	}
	if got := result.Outcome(); got != shipmentapp.AmendmentUndecided {
		t.Fatalf("outcome = %q, want UNDECIDED——提供方授权那半没立，既不能放行也不能判越权", got)
	}
	if got := result.PendingReason(); got != shipmentapp.SourceDataAmendmentAuthorityRulesNotConfigured {
		t.Fatalf("pending reason = %q, want SOURCE_DATA_AMENDMENT_AUTHORITY_RULES_NOT_CONFIGURED", got)
	}
	if result.ContinuationReference().String() == "" {
		t.Fatal("未决没有带续办引用")
	}
	assertNoSourceDataVersionFormed(t, db)
	if got := preservedSourceRows(t, db, command.AmendmentIdentity); got != 1 {
		t.Fatalf("修订请求的来源保全行数 = %d, want 1——停点不抹掉「请求到达过」", got)
	}
}

// fixedAmendmentIntake 把任何请求都译成同一条命令（隔离合成 `S`，不进生产装配）：它只为让请求越过
// Intake 到达真编排，采信身份那半仍是 `PAR-INT-01` / `BD-PS-009` 待提供，本替身不读请求。
type fixedAmendmentIntake struct {
	command shipmentapp.AmendCustomerSourceDataCommand
}

func (intake fixedAmendmentIntake) IntakeSourceDataAmendment(
	context.Context,
	*http.Request,
) (shipmentapp.AmendCustomerSourceDataCommand, error) {
	return intake.command, nil
}

// Covers: 票 04 装配用例④——第一个诚实停点在 HTTP 面上看得见：生产编排接在真端点后面，请求越过
// Intake 后答 200，`outcome` 是 UNDECIDED、带封闭原因与续办引用、不带版本号。接上入口的意义正是
// 「停点从没有入口变成说得出停在哪」，说得出要在线上说；端点用例只能证传输失败分流，这一格由这里补。
func TestTheHonestStopIsObservableAtTheAmendmentEndpoint(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	submittedOnRealAssembly(t, db)

	amendment, err := buildCustomerAmendmentOrchestration(db)
	if err != nil {
		t.Fatalf("装配资料修订编排：%v", err)
	}
	endpoint := shipmenthttp.NewAmendCustomerSourceDataEndpoint(
		fixedAmendmentIntake{command: amendmentCommand(t, "SYN-KEY-1-AMENDMENT")},
		amendment,
	)
	response := httptest.NewRecorder()
	endpoint.ServeHTTP(response, httptest.NewRequest(
		http.MethodPost, "/shipment-requests/source-data-amendments", strings.NewReader("{}"),
	))
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s；未决是已形成的业务答案，不是 5xx", response.Code, response.Body)
	}
	var body struct {
		Outcome               string `json:"outcome"`
		SourceDataVersionID   string `json:"sourceDataVersionId"`
		PendingReason         string `json:"pendingReason"`
		ContinuationReference string `json:"continuationReference"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode %s: %v", response.Body, err)
	}
	if body.Outcome != "UNDECIDED" || body.PendingReason != "SOURCE_DATA_AMENDMENT_AUTHORITY_RULES_NOT_CONFIGURED" {
		t.Fatalf("线上没说出停在哪：%s", response.Body)
	}
	if body.ContinuationReference == "" || body.SourceDataVersionID != "" {
		t.Fatalf("未决要带续办引用、不带版本号：%s", response.Body)
	}
	assertNoSourceDataVersionFormed(t, db)
}

// Covers: 票 04 装配用例②——授权过了（放行替身），矩阵仍是生产的未配置答复 → 停在**待复核**
// （`AWAITING_REVIEW`：「还没人说这处资料能不能改」），不是业务拒绝；同样不形成版本、不入队意图。
// 生产装配自己走不到这一格（授权先停），所以由形状半边注入替身来证。
func TestTheAmendmentAssemblyStopsAtUndeclaredRulesOnceAuthorized(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	submittedOnRealAssembly(t, db)

	amendment, err := assembleCustomerAmendmentOrchestration(db,
		grantingAmendmentAuthorizer{t: t},
		productionAmendmentRules(),
	)
	if err != nil {
		t.Fatalf("装配资料修订编排：%v", err)
	}
	result, err := amendment.Handle(t.Context(), amendmentCommand(t, "SYN-KEY-1-AMENDMENT"))
	if err != nil {
		t.Fatalf("资料修订：%v", err)
	}
	if got := result.Outcome(); got != shipmentapp.AmendmentAwaitingReview {
		t.Fatalf("outcome = %q, want AWAITING_REVIEW——矩阵未登记是待复核，不是拒绝也不是放行", got)
	}
	assertNoSourceDataVersionFormed(t, db)
}

// Covers: 票 04 装配用例③——交接壳在自己的事务里把资料版本意图入队恰好一封；同一版本再交一次
// 仍是一封（意图由版本标识认领，`AT-PS-031`「仅重试同一发布意图」）。编排在事务之外调它，壳若漏了
// 事务，OutboxSourceDataHandoff 会以 RequireExecutor 拒绝，装配点上的症状是每次修订都停在
// SourceDataVersionNotHandedOff。
func TestTheSourceDataHandoffBoundaryEnqueuesOnceWithinItsOwnTransaction(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	store, err := outbox.NewStore(db)
	if err != nil {
		t.Fatalf("构造 Outbox Store：%v", err)
	}
	inner, err := pspostgres.NewOutboxSourceDataHandoff(db, store, systemClock{})
	if err != nil {
		t.Fatalf("构造资料版本交接：%v", err)
	}
	boundary := sourceDataHandoffBoundary{transactor: db.Transactor(), inner: inner}

	command := amendmentCommand(t, "SYN-KEY-1-AMENDMENT")
	intent := ports.SourceDataVersionHandoffIntent{
		Identity: command.Identity,
		Version:  mustValue(t, domain.NewSourceDataVersionID, "SYN-SDV-1"),
		Scope:    command.Scope,
	}
	// 事务之外直接调：壳的全部意义就是替编排开这个事务。
	if err := boundary.HandOffSourceDataVersion(t.Context(), intent); err != nil {
		t.Fatalf("首次交接：%v", err)
	}
	if got := envelopeCountOfType(t, db, sourceDataVersionEnvelopeType); got != 1 {
		t.Fatalf("意图 = %d 封, want 恰好 1", got)
	}
	if err := boundary.HandOffSourceDataVersion(t.Context(), intent); err != nil {
		t.Fatalf("重交同一版本：%v", err)
	}
	if got := envelopeCountOfType(t, db, sourceDataVersionEnvelopeType); got != 1 {
		t.Fatalf("重交后意图 = %d 封——同一版本的意图至多一份", got)
	}
}

// productionAmendmentRules 交回生产装配用的那只矩阵答复，与 buildCustomerAmendmentOrchestration 接的
// 是同一个类型——用例②要证的正是「生产的矩阵答复让编排停在待复核」。
func productionAmendmentRules() ports.SourceDataRuleDeclaration {
	return pspartycommercial.UnconfiguredSourceDataRuleDeclaration{}
}

// grantingAmendmentAuthorizer 是放行的授权替身（隔离合成 `S`）：只为让编排走到矩阵那一步。带一份
// 合成的授权快照与决定方——都以 SYN- 开头，不是任何真实角色。
type grantingAmendmentAuthorizer struct{ t *testing.T }

func (double grantingAmendmentAuthorizer) AuthorizeSourceDataAmendment(
	context.Context,
	ports.SourceDataAmendmentAuthorizationQuery,
) (ports.SourceDataAmendmentAuthorization, error) {
	double.t.Helper()
	return ports.SourceDataAmendmentAuthorization{
		Outcome:   ports.AuthorizationGranted,
		Authority: mustValue(double.t, domain.NewAmendmentAuthoritySnapshot, "SYN-AUTH-RULE-01/v1"),
		Decider:   mustValue(double.t, domain.NewDeciderReference, "SYN-CUSTOMER-01"),
	}, nil
}

var _ ports.SourceDataAmendmentAuthorizer = grantingAmendmentAuthorizer{}
