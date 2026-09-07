package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
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
// 未立，票 03），授权过了阶段按关务与装袋两面真读面判出（ADR-0118；票 05 接线前这里停在判不出阶段），
// 再停在矩阵未登记（提供方矩阵那半未立，票 02）；两处都不形成版本、不入队「资料版本已形成」意图。另一条钉住交接壳：
// 意图在自己的事务里入队恰好一封、重放不翻倍——版本与意图不同事务是编排今天的形状，壳只负责把
// RequireExecutor 那一格接对。最后一条把第一个停点推到 HTTP 面上：端点用例里带业务结果的字段未导出、
// 逐格映射说好由本包经真编排补，补在这里。

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

// acceptedOnRealAssembly 在 submittedOnRealAssembly 之上把委托推到`已接受`并落库：读回真提交的聚合，
// 经领域 Decide（形照 application 包用例的 acceptedRequest：一条已通过的可达性校验、一份合成商业
// 依据快照）再经真仓储 Save。阶段判断与矩阵两个停点都在`已接受`之后（判阶段先核状态），要证它们
// 就得有一份已接受委托在库里；本包没有走完整接受链的夹具，直接落决定是取证捷径，不是生产路径。
func acceptedOnRealAssembly(t *testing.T, db *bentopg.DB) {
	t.Helper()
	submittedOnRealAssembly(t, db)

	requests, err := pspostgres.NewShipmentRequests(db)
	if err != nil {
		t.Fatalf("构造委托仓储：%v", err)
	}
	command := submissionCommand(t)
	request, found, err := requests.FindBySourceIdentity(t.Context(), command.Identity)
	if err != nil || !found {
		t.Fatalf("读回委托：err=%v found=%v", err, found)
	}
	applicable, err := domain.NewApplicableCheckGroups(domain.NetworkReachabilityCheck)
	if err != nil {
		t.Fatalf("new applicable check groups: %v", err)
	}
	basis, err := domain.NewCommercialBasisSnapshot(domain.CommercialBasisSnapshotSpec{
		ResolutionID: mustValue(t, domain.NewCommercialResolutionID, "SYN-RES-1"),
		RulePackage:  mustValue(t, domain.NewRulePackageReference, "SYN-RULES-1/v1"),
		ViewRevision: mustValue(t, domain.NewCommercialViewRevision, "SYN-VIEW-1"),
		Applicable:   applicable,
		ManualReview: domain.ManualReviewNotRequiredByRules,
	})
	if err != nil {
		t.Fatalf("new commercial basis snapshot: %v", err)
	}
	checks := make([]domain.AcceptanceCheck, 0, len(command.DeclaredParcelIDs))
	for _, parcel := range command.DeclaredParcelIDs {
		check, err := domain.NewAcceptanceCheck(domain.NetworkReachabilityCheck, parcel, domain.CheckPassed, domain.CheckReason{})
		if err != nil {
			t.Fatalf("new acceptance check: %v", err)
		}
		checks = append(checks, check)
	}
	accepted, err := request.Decide(domain.AcceptanceDecisionSpec{
		DecisionID: mustValue(t, domain.NewAcceptanceDecisionID, "SYN-DECISION-1"),
		Checks:     checks,
		Basis:      basis,
		DecidedAt:  time.Date(2026, 8, 21, 10, 30, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("decide: %v", err)
	}
	if accepted.State() != domain.ShipmentRequestAccepted {
		t.Fatalf("state = %q, want ACCEPTED", accepted.State())
	}
	err = db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
		saved, err := requests.Save(txCtx, command.Identity, accepted)
		if err != nil {
			return err
		}
		if saved != ports.ShipmentRequestSaved {
			return fmt.Errorf("save outcome = %v", saved)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("落已接受委托：%v", err)
	}
}

// recordingAmendmentRules 是生产矩阵答复外面的一层记录壳（隔离合成 `S`）：把编排交来的查询记下再原样
// 转给 UnconfiguredSourceDataRuleDeclaration。它只为让用例看见「编排问矩阵时带的是哪个阶段」——结果
// 仍是生产那只的待复核，阶段这一维在结果上本就不可见，只有查询能证它判对了。
type recordingAmendmentRules struct {
	inner   ports.SourceDataRuleDeclaration
	queries []ports.SourceDataAmendmentQuery
}

func (rules *recordingAmendmentRules) DeclareSourceDataAmendment(
	ctx context.Context,
	query ports.SourceDataAmendmentQuery,
) (ports.SourceDataAmendmentAllowance, error) {
	rules.queries = append(rules.queries, query)
	return rules.inner.DeclareSourceDataAmendment(ctx, query)
}

// lastStageAskedOfTheRules 交回编排最近一次问矩阵时带的阶段；没问过即失败——问都没问到矩阵，
// 说明停点比预期更早。
func (rules *recordingAmendmentRules) lastStageAskedOfTheRules(t *testing.T) domain.AmendmentStage {
	t.Helper()
	if len(rules.queries) == 0 {
		t.Fatal("编排没有问到矩阵——阶段判断在它之前就停了")
	}
	return rules.queries[len(rules.queries)-1].Stage
}

// seedCustomsUnitFor 在关务自己的表里为该包裹落一个尚无提交版本的申报单元（案件先于单元存在，
// 外键要它在）。播种走显式 SQL 而不借道关务的写口：装配用例证的是「PS 经读面读到了关务的事实」，
// 关务写口自己的往返在它自己的包里证。
func seedCustomsUnitFor(t *testing.T, pool *pgxpool.Pool, tenant, parcel string) {
	t.Helper()
	ctx := t.Context()
	if _, err := pool.Exec(ctx,
		`INSERT INTO customs_compliance.customs_case
			(tenant_id, jurisdiction_ref, direction, procedure_ref, obligation_ref, case_id, parcels, roles, established_at)
		 VALUES ($1, 'CN', 'EXPORT', 'SYN-PROC-1', 'SYN-OBL-1', 'SYN-CASE-1',
		         jsonb_build_array(jsonb_build_object('parcel', $2::text, 'customer', 'SYN-CUST-1', 'sourceRef', 'SYN-SRC-1')),
		         '[]'::jsonb, $3)`,
		tenant, parcel, time.Date(2026, 8, 21, 11, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("播种关务案件：%v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO customs_compliance.declaration_unit
			(tenant_id, unit_id, case_id, procedure_ref, members, formed_at)
		 VALUES ($1, 'SYN-UNIT-1', 'SYN-CASE-1', 'SYN-PROC-1', jsonb_build_array($2::text), $3)`,
		tenant, parcel, time.Date(2026, 8, 21, 11, 5, 0, 0, time.UTC)); err != nil {
		t.Fatalf("播种申报单元：%v", err)
	}
}

// seedBaggedIntakeFor 在节点作业自己的表里落一次识别成功的收寄（版本化关联指向该包裹）并把那件
// 作业实物移入一个开放的集运单元：容纳索引里的一行就是「此刻在袋里」。
func seedBaggedIntakeFor(t *testing.T, pool *pgxpool.Pool, tenant, parcel string) {
	t.Helper()
	ctx := t.Context()
	receivedAt := time.Date(2026, 8, 21, 10, 45, 0, 0, time.UTC)
	if _, err := pool.Exec(ctx,
		`INSERT INTO node_operations.reception
			(tenant_id, source_id, content_digest, kind, intake, control, candidates, identity_conflict, service_markers, recorded_at)
		 VALUES ($1, 'SYN-SCAN-1', 'syn-digest-1', 'INTAKE_FORMED',
		         jsonb_build_object('unit', 'SYN-HU-1', 'node', 'SYN-NODE-1', 'deliveredBy', 'SYN-COURIER-1',
		                            'evidence', 'SYN-EVIDENCE-1', 'version', 'SYN-INTAKE-V1',
		                            'association', $2::text, 'receivedAt', $3::timestamptz),
		         jsonb_build_object('unit', 'SYN-HU-1', 'node', 'SYN-NODE-1', 'kind', 'NODE_INTAKE',
		                            'basis', 'SYN-INTAKE-V1', 'establishedAt', $3::timestamptz),
		         '[]'::jsonb, false, '[]'::jsonb, $3)`,
		tenant, parcel, receivedAt); err != nil {
		t.Fatalf("播种节点收寄：%v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO node_operations.consolidation_unit
			(tenant_id, unit_id, asset_ref, phase, members, snapshots, opened_source)
		 VALUES ($1, 'SYN-BAG-1', 'SYN-ASSET-1', 'OPEN', '["SYN-HU-1"]'::jsonb, '[]'::jsonb,
		         jsonb_build_object('sourceId', 'SYN-OPEN-1', 'performedBy', 'SYN-PACKER-1',
		                            'evidence', 'SYN-WORK-1', 'occurredAt', $2::timestamptz))`,
		tenant, receivedAt.Add(10*time.Minute)); err != nil {
		t.Fatalf("播种集运单元：%v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO node_operations.containment_current (tenant_id, member_id, unit_id)
		 VALUES ($1, 'SYN-HU-1', 'SYN-BAG-1')`,
		tenant); err != nil {
		t.Fatalf("播种容纳索引：%v", err)
	}
}

// Covers: 票 04 装配用例②（票 05 接线后改写）——授权过了（放行替身），关务与装袋两口接的是**真读面**
// （生产装配用的同两只适配器，套在同一个库里 CC/NO 自己的表上），阶段按事实判出、编排走到矩阵，停在
// 生产矩阵答复的**待复核**（`AWAITING_REVIEW`：「还没人说这处资料能不能改」）；不形成版本、不入队意图。
// 接真之前这一格停在`判不出阶段`，接真之后停点后移一格——这正是票 05 的意义，用例钉的就是这次后移。
//
// 阶段在结果上不可见，由记录壳从矩阵查询里取出来证三步：CC/NO 空册 → 已接受尚未收寄（读面答`不在`，
// 不是`不知道`，阶段才判得出）；节点作业落一次识别成功的收寄并装袋 → 已制签或已装袋；关务再为它形成
// 一个尚无提交版本的申报单元 → 关务资料形成中、尚未提交（靠后的格压过靠前的）。三步各是一次新的修订
// 请求身份——停在未决的请求也保全了来源身份，同一身份重放读回的是上一次的答案。
func TestTheAmendmentAssemblyJudgesTheStageFromTheRealReadFacesOnceAuthorized(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	acceptedOnRealAssembly(t, db)

	customs, consolidation, err := buildAmendmentStageFactViews(db)
	if err != nil {
		t.Fatalf("装配阶段事实读口：%v", err)
	}
	rules := &recordingAmendmentRules{inner: pspartycommercial.UnconfiguredSourceDataRuleDeclaration{}}
	amendment, err := assembleCustomerAmendmentOrchestration(db,
		grantingAmendmentAuthorizer{t: t},
		rules,
		customs,
		consolidation,
	)
	if err != nil {
		t.Fatalf("装配资料修订编排：%v", err)
	}

	amend := func(key string, wantStage domain.AmendmentStage) {
		t.Helper()
		result, err := amendment.Handle(t.Context(), amendmentCommand(t, key))
		if err != nil {
			t.Fatalf("%s：资料修订：%v", key, err)
		}
		if got := result.Outcome(); got != shipmentapp.AmendmentAwaitingReview {
			t.Fatalf("%s：outcome = %q（未决原因 %q）, want AWAITING_REVIEW——阶段已按真读面判出，停点该在矩阵未登记，不在判不出阶段",
				key, got, result.PendingReason())
		}
		if got := rules.lastStageAskedOfTheRules(t); got != wantStage {
			t.Fatalf("%s：问矩阵时带的阶段 = %v, want %v", key, got, wantStage)
		}
		assertNoSourceDataVersionFormed(t, db)
	}

	tenant := submissionCommand(t).Identity.TenantID().String()
	const parcel = "syn-parcel-1"

	amend("SYN-KEY-1-AMENDMENT", domain.StageAcceptedNotYetReceived)

	seedBaggedIntakeFor(t, pool, tenant, parcel)
	amend("SYN-KEY-2-AMENDMENT", domain.StageLabelledOrBagged)

	seedCustomsUnitFor(t, pool, tenant, parcel)
	amend("SYN-KEY-3-AMENDMENT", domain.StageCustomsDataFormingNotSubmitted)
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
