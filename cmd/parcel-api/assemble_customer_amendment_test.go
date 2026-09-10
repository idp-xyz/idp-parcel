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
	pspostgres "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres"
	shipmentapp "go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	pcpostgres "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/postgres"
	pcdomain "go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	pcports "go.idp.xyz/idp-parcel/internal/partycommercial/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件对真实 PostgreSQL 16 证资料修订编排的装配（票 ps-port-remainder/04，形照 ADR-0106 决定四）：
// 入口接上之后编排不是从此能改资料，而是**说得出停在哪**——生产装配下停在授权未决（授权口已接 PC 裁定
// 编排与真授权册 / 委派册，票 03；商业坐标映射是实例半边留 nil，适配器答未形成），授权过了阶段按关务与装袋两面真读面判出（ADR-0118；票 05 接线前这里停在判不出阶段），
// 再问矩阵：矩阵那一口已接 party-commercial 的资料修订允许声明（票 02 余段，ADR-0120）——闭包不在或
// 声明未登记停在待复核，登了声明就按声明答（允许则形成版本并入队意图、不允许是业务拒绝、封闭声明的
// 缺格读不允许）。另一条钉住交接壳：意图在自己的事务里入队恰好一封、重放不翻倍——版本与意图不同事务
// 是编排今天的形状，壳只负责把 RequireExecutor 那一格接对。再一条把第一个停点推到 HTTP 面上：端点用例
// 里带业务结果的字段未导出、逐格映射说好由本包经真编排补，补在这里。

// sourceDataVersionEnvelopeType 与发布侧适配器的类型常量同字面（手抄，理由同 submittedEnvelopeType）。
const sourceDataVersionEnvelopeType = "parcel-shipment.source-data-version.formed"

// amendmentCommand 是对 submissionCommand 那份委托的一次资料修订：修订请求有自己的来源身份与载荷
// 摘要，范围取整份委托的一个资料组，意图为补充，基准为接受基线。
func amendmentCommand(t *testing.T, amendmentKey string) shipmentapp.AmendCustomerSourceDataCommand {
	t.Helper()
	return amendmentCommandFor(t, amendmentKey, "SYN-GROUP-CONSIGNEE", domain.SupplementIntent)
}

// amendmentCommandFor 让资料组与意图可变：矩阵按（资料组 × 阶段 × 意图）答，证「按声明答」要能问到
// 声明里不同的格。基准仍是接受基线——意图与基准的配合是形成版本那一步的事，矩阵之前问不到它。
func amendmentCommandFor(
	t *testing.T,
	amendmentKey, dataGroup string,
	intent domain.AmendmentIntent,
) shipmentapp.AmendCustomerSourceDataCommand {
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
		mustValue(t, domain.NewSourceDataGroupReference, dataGroup),
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
		Intent:            intent,
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

// Covers: 票 04 装配用例①（票 03 PS 半边接真后改写）——生产装配下修订请求越过 Intake 后停在**授权未决**，
// 原因是 `SourceDataAmendmentAuthorityUnavailable`：授权口已接真适配器（PC 裁定编排 + 真授权册 + 真委派册），
// 而商业坐标映射是实例半边（`BD-PS-009` / `PAR-COM-14`）留 nil，适配器答未形成、编排把它留在 error 格。
// 接真之前这里停在`授权规则未配置`——那是提供方对自己登记册的答案，而今天没人问到登记册，说成未配置就是
// 假话；停点从「等 PC 立规则」后移到「等消费侧接映射」，是本票唯一可观察的生产变化。带续办引用；不形成
// 版本、不入队意图；停点之前来源保全已留痕。这一格与「客户越权」（NOT_AUTHORIZED）必须分得开：映射没接
// 不是有人被拒。
func TestTheProductionAmendmentAssemblyStopsAtUnformedAuthorization(t *testing.T) {
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
		t.Fatalf("outcome = %q, want UNDECIDED——坐标映射未接，既不能放行也不能判越权", got)
	}
	if got := result.PendingReason(); got != shipmentapp.SourceDataAmendmentAuthorityUnavailable {
		t.Fatalf("pending reason = %q, want SOURCE_DATA_AMENDMENT_AUTHORITY_UNAVAILABLE——映射未接是消费侧自己的缺口，不是提供方登记册未配置", got)
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
	if body.Outcome != "UNDECIDED" || body.PendingReason != "SOURCE_DATA_AMENDMENT_AUTHORITY_UNAVAILABLE" {
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

// recordingAmendmentRules 是生产矩阵读法外面的一层记录壳（隔离合成 `S`）：把编排交来的查询记下再原样
// 转给内层（生产装配用的同一只真读法）。它只为让用例看见「编排问矩阵时带的是哪个阶段」——阶段这一维
// 在结果上本就不可见，只有查询能证它判对了。
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

// Covers: 票 04 装配用例②（票 05 接线后改写，票 02 余段接真后矩阵那只换成生产读法）——授权过了（放行
// 替身），关务与装袋两口接的是**真读面**（生产装配用的同两只适配器，套在同一个库里 CC/NO 自己的表上），
// 阶段按事实判出、编排走到矩阵，停在生产矩阵读法的**待复核**（`AWAITING_REVIEW`：这份委托接受时引用的
// 解析 `SYN-RES-1` 在 PC 解析库里没有闭包——「还没人说这处资料能不能改」）；不形成版本、不入队意图。
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
	production, err := buildSourceDataAmendmentAllowance(db)
	if err != nil {
		t.Fatalf("装配矩阵读法：%v", err)
	}
	rules := &recordingAmendmentRules{inner: production}
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

// synAdoptedRulePackage 是 acceptedOnRealAssembly 那份决定引用的接单规则包版本（快照写的
// `SYN-RULES-1/v1`），以 PC 领域的重建门造成一份已生效版本。权威在闭包的 AdoptedFor，不在快照串
// （ResolvedAdoptedStageOwner 头注）——所以种子要把这一版真的放进闭包，而不是指望谁去拆那个串。
func synAdoptedRulePackage(t *testing.T) pcdomain.CommercialVersion {
	t.Helper()
	approvedAt := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	interval, err := pcdomain.NewEffectiveInterval(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Time{})
	if err != nil {
		t.Fatalf("有效区间：%v", err)
	}
	approval, err := pcdomain.NewApprovalBasis(
		mustValue(t, pcdomain.NewApprovalReference, "SYN-APPROVAL-RULES-1"),
		mustValue(t, pcdomain.NewCommercialSourceReference, "SYN-SOURCE-RULES-1"),
		approvedAt,
	)
	if err != nil {
		t.Fatalf("批准依据：%v", err)
	}
	live, err := pcdomain.RehydrateCommercialVersion(pcdomain.RehydrateCommercialVersionSpec{
		TenantID:      mustValue(t, pcdomain.NewTenantID, submissionCommand(t).Identity.TenantID().String()),
		Kind:          pcdomain.AcceptanceRulePackageObject,
		ObjectID:      mustValue(t, pcdomain.NewCommercialObjectID, "SYN-RULES-1"),
		Version:       mustValue(t, pcdomain.NewCommercialVersionLabel, "v1"),
		Scope:         mustValue(t, pcdomain.NewCommercialScopeReference, "SYN-PC-SCOPE-1"),
		ContentDigest: mustValue(t, pcdomain.NewCommercialContentDigest, "sha256:SYN-RULES-1"),
		Effective:     interval,
		Status:        pcdomain.CommercialVersionEffective,
		Approval:      approval,
		PublishedAt:   approvedAt,
		EffectiveAt:   approvedAt.Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("重建已生效接单规则包：%v", err)
	}
	return live
}

// seedAdoptedClosure 给已接受委托引用的解析 `SYN-RES-1` 配上一份真的 PC 闭包，采用 synAdoptedRulePackage
// 那一版（形照 parcel-dispatch 的 SYN-PC-SEED 夹具，只取本用例要的接单规则包一项）。生产路径上闭包由
// 接受链固定（ADR-0027 / ADR-0062）；本包没有走完整接受链的夹具，直接落闭包是取证捷径，不是生产路径。
func seedAdoptedClosure(t *testing.T, db *bentopg.DB, rules pcdomain.CommercialVersion) {
	t.Helper()
	identity := submissionCommand(t).Identity
	anchorAt := time.Date(2026, 8, 21, 10, 0, 0, 0, time.UTC)
	anchor, err := pcdomain.NewSelectionAnchor(anchorAt, mustValue(t, pcdomain.NewAnchorPolicyVersion, "SYN-ANCHOR-POLICY-1"))
	if err != nil {
		t.Fatalf("选用锚点：%v", err)
	}
	closure, err := pcdomain.RehydrateCommercialClosure(pcdomain.RehydrateCommercialClosureSpec{
		Outcome:      pcdomain.UniquelyResolved,
		ResolutionID: mustValue(t, pcdomain.NewResolutionID, "SYN-RES-1"),
		Key: pcdomain.ClosureResolutionKey{
			TenantID:             mustValue(t, pcdomain.NewTenantID, identity.TenantID().String()),
			CustomerAccountID:    mustValue(t, pcdomain.NewCustomerAccountID, identity.CustomerAccountID().String()),
			LegalEntityCandidate: mustValue(t, pcdomain.NewLegalEntityReference, "SYN-LEGAL-1"),
			Scope:                mustValue(t, pcdomain.NewCommercialScopeReference, "SYN-PC-SCOPE-1"),
			Purpose:              pcdomain.AcceptanceControlPurpose,
			Anchor:               anchor,
			RequiredBases:        []pcdomain.CommercialObjectKind{pcdomain.AcceptanceRulePackageObject},
		},
		Anchor:       anchor,
		ViewRevision: mustValue(t, pcdomain.NewAuthorityViewRevision, "SYN-VIEW-1"),
		Adopted:      []pcdomain.RehydrateAdoptedBasisSpec{{Kind: pcdomain.AcceptanceRulePackageObject, Version: rules}},
	})
	if err != nil {
		t.Fatalf("重建唯一已解析闭包：%v", err)
	}
	resolutions, err := pcpostgres.NewCommercialResolutions(db)
	if err != nil {
		t.Fatalf("构造解析库：%v", err)
	}
	var outcome pcports.ResolutionSaveOutcome
	if err := db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
		var saveErr error
		outcome, saveErr = resolutions.Save(txCtx, closure)
		return saveErr
	}); err != nil {
		t.Fatalf("落闭包：%v", err)
	}
	if outcome != pcports.ResolutionSaved {
		t.Fatalf("闭包 save outcome = %s, want 已写入", outcome)
	}
}

// declareAmendmentAllowance 经 PC 自己的写口登记一份资料修订允许声明——用真写口而不是裸 SQL，是让种子
// 也过一遍提供方的构造门（未封闭零格、集外格在这里就会被拒），装配用例证的才是「PS 读到了一份 PC 认可
// 的声明」。
func declareAmendmentAllowance(
	t *testing.T,
	db *bentopg.DB,
	rules pcdomain.CommercialVersion,
	closed bool,
	cells ...pcdomain.SourceDataAmendmentRule,
) {
	t.Helper()
	content, err := pcdomain.NewSourceDataAmendmentAllowanceContent(rules, closed, cells)
	if err != nil {
		t.Fatalf("资料修订允许声明：%v", err)
	}
	publications, err := pcpostgres.NewCommercialPublications(db)
	if err != nil {
		t.Fatalf("构造商业发布登记册：%v", err)
	}
	var outcome pcports.DeclarationSaveOutcome
	if err := db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
		var saveErr error
		outcome, saveErr = publications.SaveSourceDataAmendmentAllowance(txCtx, content)
		return saveErr
	}); err != nil {
		t.Fatalf("登记资料修订允许声明：%v", err)
	}
	if outcome != pcports.DeclarationSaved {
		t.Fatalf("声明 save outcome = %s, want 已写入", outcome)
	}
}

func amendmentCell(
	t *testing.T,
	group string,
	stage pcdomain.DeclaredAmendmentStage,
	intent pcdomain.DeclaredAmendmentIntent,
	allowance pcdomain.AmendmentAllowance,
) pcdomain.SourceDataAmendmentRule {
	t.Helper()
	return pcdomain.SourceDataAmendmentRule{
		DataGroup: mustValue(t, pcdomain.NewSourceDataGroupReference, group),
		Stage:     stage,
		Intent:    intent,
		Allowance: allowance,
	}
}

// authorizedAmendmentAssembly 装一份「授权放行、其余全真」的编排：矩阵读法、阶段事实读面、仓储、意图交接都是
// 生产那几只。证矩阵那一格的用例都从这里起——生产装配停在授权未决，走不到矩阵。
func authorizedAmendmentAssembly(t *testing.T, db *bentopg.DB) shipmenthttp.AmendmentHandler {
	t.Helper()
	customs, consolidation, err := buildAmendmentStageFactViews(db)
	if err != nil {
		t.Fatalf("装配阶段事实读口：%v", err)
	}
	rules, err := buildSourceDataAmendmentAllowance(db)
	if err != nil {
		t.Fatalf("装配矩阵读法：%v", err)
	}
	amendment, err := assembleCustomerAmendmentOrchestration(db, grantingAmendmentAuthorizer{t: t}, rules, customs, consolidation)
	if err != nil {
		t.Fatalf("装配资料修订编排：%v", err)
	}
	return amendment
}

// Covers: 票 ps-port-remainder/02 余段的装配判据——「登了声明则按声明答、未登则 NotDeclared」，在真库上经
// 生产装配用的同一只读法证四格：闭包在而声明未登记 → 待复核（未声明）；登了（收件人资料组 × 已接受尚未
// 收寄 × 补充）= 允许 → 形成版本、入队恰好一封「资料版本已形成」意图（RECORDED）；同格 × 更正 = 不允许
// → 业务拒绝（DISALLOWED，不是未决，复核翻不了案）、版本与意图都不多；未封闭声明里没登的资料组 → 待复核。
// 四次各是一次新的修订请求身份。声明经 PC 自己的写口登记，闭包按 ADR-0062 回指到接受时固定的那一版。
func TestTheAmendmentAssemblyAnswersFromTheRegisteredAllowanceDeclaration(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	acceptedOnRealAssembly(t, db)
	rules := synAdoptedRulePackage(t)
	seedAdoptedClosure(t, db, rules)
	amendment := authorizedAmendmentAssembly(t, db)

	amend := func(key, group string, intent domain.AmendmentIntent) shipmentapp.AmendCustomerSourceDataResult {
		t.Helper()
		result, err := amendment.Handle(t.Context(), amendmentCommandFor(t, key, group, intent))
		if err != nil {
			t.Fatalf("%s：资料修订：%v", key, err)
		}
		return result
	}

	undeclared := amend("SYN-KEY-A-AMENDMENT", "SYN-GROUP-CONSIGNEE", domain.SupplementIntent)
	if got := undeclared.Outcome(); got != shipmentapp.AmendmentAwaitingReview {
		t.Fatalf("闭包在、声明未登记：outcome = %q（未决原因 %q）, want AWAITING_REVIEW", got, undeclared.PendingReason())
	}
	assertNoSourceDataVersionFormed(t, db)

	declareAmendmentAllowance(t, db, rules, false,
		amendmentCell(t, "SYN-GROUP-CONSIGNEE", pcdomain.DeclaredAcceptedNotYetReceived, pcdomain.DeclaredSupplementIntent, pcdomain.AmendmentAllowed),
		amendmentCell(t, "SYN-GROUP-CONSIGNEE", pcdomain.DeclaredAcceptedNotYetReceived, pcdomain.DeclaredCorrectionIntent, pcdomain.AmendmentDisallowed),
	)

	allowed := amend("SYN-KEY-B-AMENDMENT", "SYN-GROUP-CONSIGNEE", domain.SupplementIntent)
	if got := allowed.Outcome(); got != shipmentapp.AmendmentRecorded {
		t.Fatalf("登了允许：outcome = %q（未决原因 %q）, want RECORDED——声明说了能改，编排就该形成版本", got, allowed.PendingReason())
	}
	version, formed := allowed.Version()
	if !formed || version.VersionID().String() == "" {
		t.Fatal("RECORDED 却没带回形成的版本")
	}
	if got := envelopeCountOfType(t, db, sourceDataVersionEnvelopeType); got != 1 {
		t.Fatalf("「资料版本已形成」意图 = %d 封, want 恰好 1", got)
	}
	requests, err := pspostgres.NewShipmentRequests(db)
	if err != nil {
		t.Fatalf("构造委托仓储：%v", err)
	}
	request, found, err := requests.FindBySourceIdentity(t.Context(), submissionCommand(t).Identity)
	if err != nil || !found {
		t.Fatalf("读回委托：err=%v found=%v", err, found)
	}
	if versions := request.CustomerSourceDataVersions(); len(versions) != 1 || versions[0].VersionID() != version.VersionID() {
		t.Fatalf("委托上的资料版本 = %d 份, want 恰好刚形成的那一份", len(versions))
	}

	disallowed := amend("SYN-KEY-C-AMENDMENT", "SYN-GROUP-CONSIGNEE", domain.CorrectionIntent)
	if got := disallowed.Outcome(); got != shipmentapp.AmendmentDisallowed {
		t.Fatalf("登了不允许：outcome = %q（未决原因 %q）, want DISALLOWED——那是确定的业务拒绝，不是等复核", got, disallowed.PendingReason())
	}
	unlisted := amend("SYN-KEY-D-AMENDMENT", "SYN-GROUP-SHIPPER", domain.SupplementIntent)
	if got := unlisted.Outcome(); got != shipmentapp.AmendmentAwaitingReview {
		t.Fatalf("未封闭声明里没登的资料组：outcome = %q（未决原因 %q）, want AWAITING_REVIEW", got, unlisted.PendingReason())
	}
	if got := envelopeCountOfType(t, db, sourceDataVersionEnvelopeType); got != 1 {
		t.Fatalf("拒绝与待复核之后意图 = %d 封, want 仍是 1——两格都不形成版本", got)
	}
	request, _, err = requests.FindBySourceIdentity(t.Context(), submissionCommand(t).Identity)
	if err != nil {
		t.Fatalf("读回委托：%v", err)
	}
	if versions := request.CustomerSourceDataVersions(); len(versions) != 1 {
		t.Fatalf("拒绝与待复核之后委托上的资料版本 = %d 份, want 仍是 1", len(versions))
	}
}

// Covers: ADR-0120 Decision 三在消费端到端的样子——封闭且零格的声明是一句显式的话「这一版什么都不许改」，
// PS 问任何一格都读到不允许（DISALLOWED，业务拒绝），不是未声明（那会转复核）。封闭的读法由 PC 的
// AllowanceFor 算出，本适配器只翻译；这里证的是它确实原样到了编排的结果上。
func TestTheAmendmentAssemblyReadsAClosedDeclarationAsDisallowingEveryUndeclaredCell(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	acceptedOnRealAssembly(t, db)
	rules := synAdoptedRulePackage(t)
	seedAdoptedClosure(t, db, rules)
	declareAmendmentAllowance(t, db, rules, true)
	amendment := authorizedAmendmentAssembly(t, db)

	result, err := amendment.Handle(t.Context(), amendmentCommand(t, "SYN-KEY-1-AMENDMENT"))
	if err != nil {
		t.Fatalf("资料修订：%v", err)
	}
	if got := result.Outcome(); got != shipmentapp.AmendmentDisallowed {
		t.Fatalf("封闭零格声明：outcome = %q（未决原因 %q）, want DISALLOWED——登记方显式说了其余都不许", got, result.PendingReason())
	}
	assertNoSourceDataVersionFormed(t, db)
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
