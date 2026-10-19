package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	nodeopshttp "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/http"
	nopostgres "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/postgres"
	nodeopsapp "go.idp.xyz/idp-parcel/internal/nodeoperations/application"
	nodomain "go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	noports "go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件对真实 PostgreSQL 16 证人工重核命令口经生产装配（ADR-0199 决定三第二个触发，票 no-reception-source/03）：与门铃触发
// 落下同一结果，不重核的几格照票面写定的答法。夹具全为合成（S 级）。

const nodeIntakeEnvelopeType = "node-operations.node-intake.formed"

// unwiredIdentityRecheck 是装配测试里不该被走到的重核编排：未配置 Intake 拒在它前面。
type unwiredIdentityRecheck struct{}

func (unwiredIdentityRecheck) Recheck(context.Context, nodeopsapp.RecheckPendingIdentityCommand) (nodeopsapp.RecheckPendingIdentityResult, error) {
	panic("unwired identity recheck reached")
}

// fixedTenantRecheckIntake 是只认请求体判断版本、租户取合成租户的 Intake：命令口的认证另有用例，这里证的是编排与答复。
type fixedTenantRecheckIntake struct{}

func (fixedTenantRecheckIntake) IntakeIdentityRecheck(_ context.Context, request *http.Request) (nodeopsapp.RecheckPendingIdentityCommand, error) {
	var body struct {
		JudgmentVersion string `json:"judgmentVersion"`
	}
	if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
		return nodeopsapp.RecheckPendingIdentityCommand{}, err
	}
	version, err := nodomain.NewIntakeResultVersion(body.JudgmentVersion)
	if err != nil {
		return nodeopsapp.RecheckPendingIdentityCommand{}, err
	}
	tenant, err := nodomain.NewTenantID("SYN-TENANT-1")
	return nodeopsapp.RecheckPendingIdentityCommand{TenantID: tenant, Version: version}, err
}

func postIdentityRecheck(t *testing.T, endpoint http.Handler, version nodomain.IntakeResultVersion) (int, map[string]any) {
	t.Helper()
	response := httptest.NewRecorder()
	endpoint.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/node-operations/identity-rechecks",
		strings.NewReader(`{"judgmentVersion":"`+version.String()+`"}`)))
	var body map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("响应不是 JSON：%v（%s）", err, response.Body.String())
	}
	return response.Code, body
}

// receivePendingOnRealAssembly 经生产装配的收寄编排收一件带着依据 basis 送来的实物，交回那一条记录；号码 mark 此刻不在关联册里就
// 落成零候选的待识别。客户包裹件号要经重核或收寄唯一识别，送站就得带着可验证交付范围（ADR-0212 决定四、六）。
func receivePendingOnRealAssembly(
	t *testing.T,
	reception nodeopshttp.ReceptionHandler,
	sourceID, unit, mark string,
	basis nodomain.DeliveryScopeBasisReference,
) noports.ReceptionRecord {
	t.Helper()
	command := receptionCommand(t, sourceID, nodeopsapp.ExplicitReception)
	command.Unit = mustValue(t, nodomain.NewHandlingUnitID, unit)
	command.Mark = noports.ExternalMarkObservation{Mark: mark}
	command.ScopeBasis = basis
	result, err := reception.Handle(t.Context(), command)
	if err != nil {
		t.Fatalf("收寄 %s：%v", sourceID, err)
	}
	record, _ := result.Record()
	return record
}

func identityRecheckDB(t *testing.T) (*bentopg.DB, *pgxpool.Pool) {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	return db, pool
}

// recheckByMarkOnRealAssembly 经生产装配的重核依赖走门铃那一个入口（按号重核），在一笔事务里。
func recheckByMarkOnRealAssembly(t *testing.T, db *bentopg.DB, mark string) []nodeopsapp.RecheckPendingIdentityResult {
	t.Helper()
	deps, err := buildIdentityRecheckDeps(db)
	if err != nil {
		t.Fatalf("装配重核依赖：%v", err)
	}
	handler, err := nodeopsapp.NewRecheckPendingIdentityHandler(deps)
	if err != nil {
		t.Fatalf("重核编排：%v", err)
	}
	var results []nodeopsapp.RecheckPendingIdentityResult
	if err := db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
		var recheckErr error
		results, recheckErr = handler.RecheckByMark(txCtx, mustValue(t, nodomain.NewTenantID, "SYN-TENANT-1"), mark)
		return recheckErr
	}); err != nil {
		t.Fatalf("按号重核：%v", err)
	}
	return results
}

// Covers: `AT-NO-026` 的人工触发半边与「人工发起重核与信封触发落下同一结果」：同一种局面各在一个空库里——带着那个客户账户的送站
// 预报送站（ADR-0212 决定四：客户包裹件号的唯一识别要有可验证交付范围），零候选待识别落册后委托接受形成这个号的关联——一处由复核页
// 人工指名重核，一处走门铃那一个入口按号重核。重核按存下的预报重算范围（决定六）。两处追加的一版形状相同：来源标识由原
// 判断版本派生、节点收寄已形成、关联那件包裹、业务时间是原收寄时刻、前版是原待识别、意图一封。随后另一个触发都不再追加：人工
// 那一处再响门铃找不到待重核的对象，门铃那一处再人工指名答`EXISTING_RESULT`、指向门铃追加的那一版。
func TestTheWiredManualRecheckLandsTheSameVersionAsTheDoorbell(t *testing.T) {
	type landed struct {
		pending noports.ReceptionRecord
		formed  noports.ReceptionRecord
	}
	arrange := func(t *testing.T) (*bentopg.DB, http.Handler, noports.ReceptionRecord) {
		t.Helper()
		db, _ := identityRecheckDB(t)
		reception, err := buildReceptionOrchestration(db)
		if err != nil {
			t.Fatalf("装配收寄编排：%v", err)
		}
		basis := preAdviseOnRealDatabase(t, db, "SYN-CUSTOMER-1", "SYN-CUSTOMER-PARCEL-NRS03")
		pending := receivePendingOnRealAssembly(t, reception, "SYN-DELIVERY-NRS03", "SYN-UNIT-NRS03", "SYN-CUSTOMER-PARCEL-NRS03", basis)
		if pending.Kind != noports.RecordPendingIdentification || pending.IdentityConflict {
			t.Fatalf("号码入册之前收寄 = %+v, want 零候选的待识别", pending)
		}
		acceptSubmitted(t, db, submitWithCustomerParcelNumber(t, db, "SYN-CUSTOMER-PARCEL-NRS03"))
		recheck, err := buildIdentityRecheckOrchestration(db)
		if err != nil {
			t.Fatalf("装配重核编排：%v", err)
		}
		return db, nodeopshttp.NewRecheckPendingIdentityEndpoint(fixedTenantRecheckIntake{}, recheck), pending
	}
	readDerived := func(t *testing.T, db *bentopg.DB, pending noports.ReceptionRecord) noports.ReceptionRecord {
		t.Helper()
		receptions, err := nopostgres.NewReceptions(db)
		if err != nil {
			t.Fatalf("构造收寄库：%v", err)
		}
		formed, found, err := receptions.FindByKey(t.Context(), noports.ReceptionKey{
			TenantID: pending.Key.TenantID, SourceID: "IDENTITY-RECHECK/" + pending.JudgmentVersion.String(),
		})
		if err != nil || !found {
			t.Fatalf("追加版没落在派生键上：found %v err %v", found, err)
		}
		return formed
	}

	var manual, doorbell landed
	{
		db, endpoint, pending := arrange(t)
		status, body := postIdentityRecheck(t, endpoint, pending.JudgmentVersion)
		if status != http.StatusCreated || body["outcome"] != "INTAKE_FORMED" {
			t.Fatalf("人工重核答 %d %v, want 201 INTAKE_FORMED", status, body)
		}
		manual = landed{pending: pending, formed: readDerived(t, db, pending)}
		if body["judgmentVersion"] != manual.formed.JudgmentVersion.String() {
			t.Fatalf("答复的判断版本 %v 不是落册的那一版 %s", body["judgmentVersion"], manual.formed.JudgmentVersion)
		}
		if again := recheckByMarkOnRealAssembly(t, db, "SYN-CUSTOMER-PARCEL-NRS03"); len(again) != 0 {
			t.Fatalf("人工重核之后门铃又重核了 %d 件", len(again))
		}
		if n := envelopeCountOfType(t, db, nodeIntakeEnvelopeType); n != 1 {
			t.Fatalf("收寄意图 %d 封, want 1", n)
		}
	}
	{
		db, endpoint, pending := arrange(t)
		results := recheckByMarkOnRealAssembly(t, db, "SYN-CUSTOMER-PARCEL-NRS03")
		if len(results) != 1 || results[0].Outcome() != nodeopsapp.RecheckIntakeFormed {
			t.Fatalf("门铃按号重核 = %d 件, want 恰一件 INTAKE_FORMED", len(results))
		}
		doorbell = landed{pending: pending, formed: readDerived(t, db, pending)}
		status, body := postIdentityRecheck(t, endpoint, pending.JudgmentVersion)
		if status != http.StatusOK || body["outcome"] != "EXISTING_RESULT" || body["currentVersion"] != doorbell.formed.JudgmentVersion.String() {
			t.Fatalf("门铃之后人工重核答 %d %v, want 200 EXISTING_RESULT 指向 %s", status, body, doorbell.formed.JudgmentVersion)
		}
		if n := envelopeCountOfType(t, db, nodeIntakeEnvelopeType); n != 1 {
			t.Fatalf("收寄意图 %d 封, want 1", n)
		}
	}

	for name, side := range map[string]landed{"人工": manual, "门铃": doorbell} {
		association, associated := side.formed.Intake.Association()
		if side.formed.Kind != noports.RecordIntakeFormed || !associated || association.String() != "syn-parcel-1" ||
			side.formed.Predecessor != side.pending.JudgmentVersion || !side.formed.Intake.ReceivedAt().Equal(receptionDeliveredAt) ||
			side.formed.Control.Basis() != side.pending.Control.Basis() || side.formed.ServiceResultCheck != noports.ServiceResultsChecked {
			t.Fatalf("%s那一处追加的一版 = %+v", name, side.formed)
		}
	}
}

// Covers: 票面「对冲突行与无来源旧行的答法」——人工指名一件记身份冲突的待识别（一码两物里后到的那一件；两件都带着那个客户账户的
// 送站预报送站，号码才唯一识别得出包裹，ADR-0212 决定四），答 200
// `IDENTITY_CONFLICT_RECORDED`；指名一行来源保全之前落下的待识别，答 200 `SOURCE_NOT_PRESERVED`：服务端形成了答案、知道为什么
// 不重核（ADR-0022：形成答案即 2xx；ADR-0186：不是命令缺陷，也不是没形成答案）。两格都不追加、不交意图。指名册上没有的版本
// 答 404，不带 outcome。
func TestTheWiredManualRecheckLeavesConflictsAndSourcelessRowsAlone(t *testing.T) {
	db, pool := identityRecheckDB(t)
	reception, err := buildReceptionOrchestration(db)
	if err != nil {
		t.Fatalf("装配收寄编排：%v", err)
	}
	acceptSubmitted(t, db, submitWithCustomerParcelNumber(t, db, "SYN-CUSTOMER-PARCEL-NRS03"))
	basis := preAdviseOnRealDatabase(t, db, "SYN-CUSTOMER-1", "SYN-CUSTOMER-PARCEL-NRS03")
	receivePendingOnRealAssembly(t, reception, "SYN-DELIVERY-NRS03-A", "SYN-UNIT-NRS03-A", "SYN-CUSTOMER-PARCEL-NRS03", basis)
	late := receivePendingOnRealAssembly(t, reception, "SYN-DELIVERY-NRS03-B", "SYN-UNIT-NRS03-B", "SYN-CUSTOMER-PARCEL-NRS03", basis)
	if late.Kind != noports.RecordPendingIdentification || !late.IdentityConflict {
		t.Fatalf("后到那件 = %+v, want 记身份冲突的待识别", late)
	}

	sourceless := receivePendingOnRealAssembly(t, reception, "SYN-DELIVERY-NRS03-OLD", "SYN-UNIT-NRS03-OLD", "SYN-BARCODE-NRS03-UNKNOWN",
		nodomain.DeliveryScopeBasisReference{})
	if _, err := pool.Exec(t.Context(),
		`UPDATE node_operations.reception SET source_document = NULL WHERE tenant_id = $1 AND source_id = $2`,
		sourceless.Key.TenantID.String(), sourceless.Key.SourceID); err != nil {
		t.Fatalf("把那一行改回来源保全之前的形状：%v", err)
	}

	recheck, err := buildIdentityRecheckOrchestration(db)
	if err != nil {
		t.Fatalf("装配重核编排：%v", err)
	}
	endpoint := nodeopshttp.NewRecheckPendingIdentityEndpoint(fixedTenantRecheckIntake{}, recheck)
	rows := countReceptionRows(t, pool)
	intents := envelopeCountOfType(t, db, nodeIntakeEnvelopeType)
	for name, testCase := range map[string]struct {
		version nodomain.IntakeResultVersion
		outcome string
	}{
		"记身份冲突的待识别": {late.JudgmentVersion, "IDENTITY_CONFLICT_RECORDED"},
		"来源未保全的旧行":  {sourceless.JudgmentVersion, "SOURCE_NOT_PRESERVED"},
	} {
		status, body := postIdentityRecheck(t, endpoint, testCase.version)
		if status != http.StatusOK || body["outcome"] != testCase.outcome {
			t.Fatalf("%s：答 %d %v, want 200 %s", name, status, body, testCase.outcome)
		}
	}
	status, body := postIdentityRecheck(t, endpoint, mustValue(t, nodomain.NewIntakeResultVersion, "SYN-INTAKEV-NRS03-ABSENT"))
	if _, carries := body["outcome"]; status != http.StatusNotFound || carries {
		t.Fatalf("指名册上没有的版本答 %d %v, want 404 不带 outcome", status, body)
	}
	if countReceptionRows(t, pool) != rows || envelopeCountOfType(t, db, nodeIntakeEnvelopeType) != intents {
		t.Fatal("不重核的几格追加了判断或交了意图")
	}
}

func countReceptionRows(t *testing.T, pool *pgxpool.Pool) int {
	t.Helper()
	var count int
	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM node_operations.reception`).Scan(&count); err != nil {
		t.Fatalf("数收寄册：%v", err)
	}
	return count
}

// Covers: 票面并发判据「重核与同一包裹另一件实物的收寄并发，经（租户、包裹）锁串行，后到者答身份冲突，不出现两件都关联成立」
// （ADR-0199 决定三：重核走与收寄步骤 4 同一份身份判断，含票 no-reception-identity/03 的包裹锁）。A 零候选落册，委托接受形成号的
// 关联；A 的重核读完别的实物关联（空）之后挂住，B 贴着同一个号经生产装配送站，等在那件包裹的锁上；重核提交后 B 读到 A 的追加
// 版，答记身份冲突的待识别，A 那条链再追加一版身份冲突。两件都带着那个客户账户的送站预报（ADR-0212 决定四）。方向依据的身份一格只读各链链头（ADR-0210 决定五）：A 那条链是
// 原待识别 → 重核版 → 冲突版，链头是冲突版；B 只有自己那一版。两件都落成缺少·{待识别, 身份冲突}。
func TestARecheckAndAnotherUnitsReceptionSerializeOnTheParcel(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	reception, err := buildReceptionOrchestration(db)
	if err != nil {
		t.Fatalf("装配收寄编排：%v", err)
	}
	basis := preAdviseOnRealDatabase(t, db, "SYN-CUSTOMER-1", "SYN-CUSTOMER-PARCEL-NRS03")
	pending := receivePendingOnRealAssembly(t, reception, "SYN-DELIVERY-NRS03-A", "SYN-UNIT-NRS03-A", "SYN-CUSTOMER-PARCEL-NRS03", basis)
	acceptSubmitted(t, db, submitWithCustomerParcelNumber(t, db, "SYN-CUSTOMER-PARCEL-NRS03"))
	second := receptionCommand(t, "SYN-DELIVERY-NRS03-B", nodeopsapp.ExplicitReception)
	second.Unit = mustValue(t, nodomain.NewHandlingUnitID, "SYN-UNIT-NRS03-B")
	second.Mark = noports.ExternalMarkObservation{Mark: "SYN-CUSTOMER-PARCEL-NRS03"}
	second.ScopeBasis = basis

	var (
		secondResult nodeopsapp.ReceiveDeliveredUnitResult
		secondErr    error
		secondDone   chan struct{}
		waited       bool
	)
	deps, err := buildIdentityRecheckDeps(db)
	if err != nil {
		t.Fatalf("装配重核依赖：%v", err)
	}
	deps.Receptions = pausingReceptions{ReceptionStore: deps.Receptions, afterFindOthers: func() {
		if secondDone != nil {
			return
		}
		secondDone = make(chan struct{})
		go func(done chan struct{}) {
			defer close(done)
			secondResult, secondErr = reception.Handle(t.Context(), second)
		}(secondDone)
		waited = awaitAdvisoryLockWaitOrDone(t, pool, secondDone)
	}}
	paused, err := identityRecheckWithin(db, deps)
	if err != nil {
		t.Fatalf("装配挂住的重核编排：%v", err)
	}

	rechecked, recheckErr := paused.Recheck(t.Context(), nodeopsapp.RecheckPendingIdentityCommand{
		TenantID: pending.Key.TenantID, Version: pending.JudgmentVersion,
	})
	if secondDone == nil {
		t.Fatal("重核没走到按包裹读别的实物关联那一步——号码没唯一识别出包裹")
	}
	awaitDone(t, secondDone)
	if recheckErr != nil || secondErr != nil {
		t.Fatalf("重核 err = %v，B err = %v", recheckErr, secondErr)
	}
	if rechecked.Outcome() != nodeopsapp.RecheckIntakeFormed || secondResult.Outcome() != nodeopsapp.UnitPendingIdentification {
		t.Fatalf("重核 %v / B %v, want INTAKE_FORMED / PENDING_IDENTIFICATION", rechecked.Outcome(), secondResult.Outcome())
	}
	if !waited {
		t.Fatal("B 没在包裹锁上等就走完了")
	}
	late, _ := secondResult.Record()
	if !late.IdentityConflict || !slices.Equal(candidatesOf(late), []string{"syn-parcel-1"}) {
		t.Fatalf("B = %+v, want 记身份冲突、候选是那件包裹", late)
	}
	var associatedUnits int
	if err := pool.QueryRow(t.Context(),
		`SELECT count(DISTINCT intake ->> 'unit') FROM node_operations.reception WHERE intake ->> 'association' = 'syn-parcel-1'`,
	).Scan(&associatedUnits); err != nil {
		t.Fatalf("数关联到那件包裹的实物：%v", err)
	}
	if associatedUnits != 1 {
		t.Fatalf("%d 件实物关联到同一件包裹, want 1", associatedUnits)
	}

	identities, err := nopostgres.NewIntakeIdentities(db)
	if err != nil {
		t.Fatalf("构造身份读口：%v", err)
	}
	for _, unit := range []string{"SYN-UNIT-NRS03-A", "SYN-UNIT-NRS03-B"} {
		observations, err := identities.FindIntakeIdentities(t.Context(), pending.Key.TenantID, mustValue(t, nodomain.NewHandlingUnitID, unit))
		if err != nil {
			t.Fatalf("读 %s 的收寄身份：%v", unit, err)
		}
		basis, err := nodomain.JudgeIdentityBasis(observations)
		if err != nil {
			t.Fatalf("判 %s 的身份：%v", unit, err)
		}
		want := []nodomain.DirectionalBasisReason{nodomain.ReasonPendingIdentification, nodomain.ReasonIdentityConflict}
		if reasons := basis.Cell().Reasons(); basis.Cell().Answer() != nodomain.BasisMissing || !slices.Equal(reasons, want) {
			t.Fatalf("%s 的身份一格 = %s %v（%d 行）, want 缺少·%v", unit, basis.Cell().Answer(), reasons, len(observations), want)
		}
	}
}
