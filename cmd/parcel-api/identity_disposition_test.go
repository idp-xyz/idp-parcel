package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	aipg "go.idp.xyz/idp-parcel/internal/accessidentity/adapters/postgres"
	nodeopshttp "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/http"
	nopostgres "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/postgres"
	nodeopsapp "go.idp.xyz/idp-parcel/internal/nodeoperations/application"
	nodomain "go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	noports "go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
	pgpostgres "go.idp.xyz/idp-parcel/internal/pilotgovernance/adapters/postgres"
	pgdomain "go.idp.xyz/idp-parcel/internal/pilotgovernance/domain"
)

// 本文件对真实 PostgreSQL 16 证身份冲突处置命令口经生产装配（UC-NO-004；票 no-reception-identity/06）：认证走生产那一只「运营
// 决定」Intake，授予、准入范围对照与生产权威区间照演示种子的登法；编排、收寄库与意图交付照生产装配。夹具全为合成（S 级）。

const dispositionReceptionTenant = "SYN-TENANT-1"

// unwiredIdentityDisposition 是装配测试里不该被走到的处置编排：未配置 Intake 拒在它前面。
type unwiredIdentityDisposition struct{}

func (unwiredIdentityDisposition) Dispose(context.Context, nodeopsapp.DisposeIdentityConflictCommand) (nodeopsapp.DisposeIdentityConflictResult, error) {
	panic("unwired identity disposition reached")
}

// dispositionGrantRegister 是操作者册的替身：合成操作者绑在收寄所在的租户上，持`身份冲突处置`一种运营决定授予——与演示种子
// 给配置员那一笔同形（能力面 OPERATION_DECISION、决定种类 IDENTITY_CONFLICT_DISPOSITION）。
type dispositionGrantRegister struct{}

func (dispositionGrantRegister) FindOperator(_ context.Context, subject accessidentity.OperatorSubject) (accessidentity.OperatorStanding, bool, error) {
	binding, err := accessidentity.NewOperatorBinding(subject, dispositionReceptionTenant, "SYN-BASIS-BINDING")
	if err != nil {
		return accessidentity.OperatorStanding{}, false, err
	}
	interval, err := accessidentity.NewEffectiveInterval(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Time{})
	if err != nil {
		return accessidentity.OperatorStanding{}, false, err
	}
	grant, err := accessidentity.NewOperationDecisionGrant(dispositionReceptionTenant, "SYN-GRANT-DEX-CONFIG-IDENTITY-CONFLICT-DISPOSITION-01",
		subject, accessidentity.DecisionIdentityConflictDisposition, interval, "SYN-GRANT-BASIS")
	if err != nil {
		return accessidentity.OperatorStanding{}, false, err
	}
	recorded, err := accessidentity.NewRecordedGrant(grant, nil)
	if err != nil {
		return accessidentity.OperatorStanding{}, false, err
	}
	standing, err := accessidentity.NewOperatorStanding(binding, []accessidentity.RecordedGrant{recorded})
	return standing, err == nil, err
}

// registerDispositionAdmission 照演示种子的登法给收寄所在的租户登一行准入范围对照，并在对照指向的对象范围上登本产品的生产
// 权威区间：对照住接入身份，区间住治理登记册。
func registerDispositionAdmission(t *testing.T, db *bentopg.DB) {
	t.Helper()
	mappings, err := aipg.NewAdmissionScopeMappings(db)
	if err != nil {
		t.Fatal(err)
	}
	intervals, err := pgpostgres.NewAuthorityIntervals(db)
	if err != nil {
		t.Fatal(err)
	}
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	validity, err := accessidentity.NewEffectiveInterval(from, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	requirement := accessidentity.AdmissionRequirement{
		Capability: accessidentity.CapabilityOperationDecision.String(),
		FactKind:   accessidentity.DecisionIdentityConflictDisposition.String(),
	}
	mapping, err := accessidentity.NewAdmissionScopeMapping(dispositionReceptionTenant, "SYN-MAP-OD-IDENTITY-CONFLICT-DISPOSITION-01", requirement,
		operationDecisionObjectScope, validity, "SYN-MAP-BASIS-01")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
		if _, err := mappings.RegisterAdmissionScopeMapping(txCtx, mapping); err != nil {
			return err
		}
		return intervals.Append(txCtx, pgdomain.AuthorityInterval{
			ObjectScope: operationDecisionObjectScope, Capability: requirement.Capability, FactKind: requirement.FactKind,
			Authority: isolatedSelfAuthority, From: from,
		})
	}); err != nil {
		t.Fatalf("登对照与区间：%v", err)
	}
}

// wiredDispositionEndpoint 装生产那一只运营决定 Intake（核验方换成认任何令牌的替身，其余照生产）与生产处置编排。
func wiredDispositionEndpoint(t *testing.T, db *bentopg.DB, handler nodeopshttp.IdentityDispositionHandler) http.Handler {
	t.Helper()
	admission, err := buildAdmissionScope(db, isolatedSelfAuthority)
	if err != nil {
		t.Fatal(err)
	}
	minter, err := buildOperatorMinter(acceptingVerifier{}, dispositionGrantRegister{}, admission)
	if err != nil {
		t.Fatal(err)
	}
	decisions, err := buildOperatorDecisionIntakes(minter, unwiredDecisionTargets{}, unwiredDecisionTargets{})
	if err != nil {
		t.Fatal(err)
	}
	return nodeopshttp.NewDisposeIdentityConflictEndpoint(decisions.nodeOperations, handler)
}

func postWiredDisposition(t *testing.T, endpoint http.Handler, body string) (int, map[string]any) {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, "/node-operations/identity-dispositions", strings.NewReader(body))
	request.Header.Set("Authorization", "Bearer presented.operator.token")
	response := httptest.NewRecorder()
	endpoint.ServeHTTP(response, request)
	var decoded map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &decoded); err != nil {
		t.Fatalf("响应不是 JSON：%v（%s）", err, response.Body.String())
	}
	return response.Code, decoded
}

// oneMarkOnRealAssembly 经生产收寄编排落一码两物：委托接受形成号的关联，A 先到关联成立，B 后到记身份冲突。两件都带着那个客户账户
// 的送站预报送站——客户包裹件号要有可验证交付范围才唯一识别（ADR-0212 决定四），一码两物只从唯一命中那一支来。交回 A 那条链此刻
// 的链头（冲突版）与 B。
func oneMarkOnRealAssembly(t *testing.T, db *bentopg.DB) (noports.ReceptionRecord, noports.ReceptionRecord) {
	t.Helper()
	reception, err := buildReceptionOrchestration(db)
	if err != nil {
		t.Fatalf("装配收寄编排：%v", err)
	}
	acceptSubmitted(t, db, submitWithCustomerParcelNumber(t, db, "SYN-CUSTOMER-PARCEL-NRI06"))
	basis := preAdviseOnRealDatabase(t, db, "SYN-CUSTOMER-1", "SYN-CUSTOMER-PARCEL-NRI06")
	early := receivePendingOnRealAssembly(t, reception, "SYN-DELIVERY-NRI06-A", "SYN-UNIT-NRI06-A", "SYN-CUSTOMER-PARCEL-NRI06", basis)
	late := receivePendingOnRealAssembly(t, reception, "SYN-DELIVERY-NRI06-B", "SYN-UNIT-NRI06-B", "SYN-CUSTOMER-PARCEL-NRI06", basis)
	if early.Kind != noports.RecordIntakeFormed || !late.IdentityConflict {
		t.Fatalf("一码两物没落成：A %v / B 冲突 %v", early.Kind, late.IdentityConflict)
	}
	receptions, err := nopostgres.NewReceptions(db)
	if err != nil {
		t.Fatal(err)
	}
	head, err := receptions.CurrentVersion(t.Context(), early.Key.TenantID, early.JudgmentVersion)
	if err != nil || head == early.JudgmentVersion {
		t.Fatalf("A 那条链没被冲突版接续：%s err %v", head, err)
	}
	conflict, found, err := receptions.FindByJudgmentVersion(t.Context(), early.Key.TenantID, head)
	if err != nil || !found {
		t.Fatalf("读 A 的冲突版：found %v err %v", found, err)
	}
	return conflict, late
}

func identityReasonsOnRealAssembly(t *testing.T, db *bentopg.DB, unit string) []nodomain.DirectionalBasisReason {
	t.Helper()
	identities, err := nopostgres.NewIntakeIdentities(db)
	if err != nil {
		t.Fatal(err)
	}
	observations, err := identities.FindIntakeIdentities(t.Context(), mustValue(t, nodomain.NewTenantID, dispositionReceptionTenant), mustValue(t, nodomain.NewHandlingUnitID, unit))
	if err != nil {
		t.Fatalf("读 %s 的收寄身份：%v", unit, err)
	}
	basis, err := nodomain.JudgeIdentityBasis(observations)
	if err != nil {
		t.Fatalf("判 %s 的身份：%v", unit, err)
	}
	return basis.Cell().Reasons()
}

func designateBody(late, conflict noports.ReceptionRecord) string {
	return `{"unit":"SYN-UNIT-NRI06-B","disposedVersion":"` + late.JudgmentVersion.String() +
		`","result":"DESIGNATED","parcel":"syn-parcel-1","coDisposedUnits":[{"unit":"SYN-UNIT-NRI06-A","version":"` +
		conflict.JudgmentVersion.String() + `"}],"basis":"SYN-DISPOSITION-BASIS-NRI06"}`
}

// Covers: 票 06 完成判据「真库用例」与 `AT-NO-053`、`AT-NO-056` 的真库面：一码两物经生产收寄编排落册；持`身份冲突处置`授予、准入
// 范围与生产权威区间照演示种子登好的操作者，经生产 Intake 与生产编排指定 B 是那件包裹、A 连带——答 201；B 身份一格成立、A 缺少·
// 待识别；收寄意图多入队一封（A 早先那一封照留）；原样重投答 200 `EXISTING_RESULT`、册上不多一行、不多一封。指名查无此版答 404。
func TestTheWiredDispositionDisposesOneMarkOnTwoUnits(t *testing.T) {
	db, pool := identityRecheckDB(t)
	registerDispositionAdmission(t, db)
	conflict, late := oneMarkOnRealAssembly(t, db)
	disposition, err := buildIdentityDispositionOrchestration(db)
	if err != nil {
		t.Fatalf("装配处置编排：%v", err)
	}
	endpoint := wiredDispositionEndpoint(t, db, disposition)
	intents := envelopeCountOfType(t, db, nodeIntakeEnvelopeType)

	status, body := postWiredDisposition(t, endpoint, designateBody(late, conflict))
	if status != http.StatusCreated || body["outcome"] != "IDENTITY_CONFLICT_DISPOSED" {
		t.Fatalf("处置答 %d %v, want 201 IDENTITY_CONFLICT_DISPOSED", status, body)
	}
	document, _ := body["disposition"].(map[string]any)
	if document["operator"] != "OPERATOR/https://id.syn.example/dex#SYN-OPERATOR-01" || document["basis"] != "SYN-DISPOSITION-BASIS-NRI06" {
		t.Fatalf("处置文档 = %v, want 操作者取自信封、依据原样", document)
	}
	if reasons := identityReasonsOnRealAssembly(t, db, "SYN-UNIT-NRI06-B"); len(reasons) != 0 {
		t.Fatalf("B 缺少 %v, want 成立", reasons)
	}
	if reasons := identityReasonsOnRealAssembly(t, db, "SYN-UNIT-NRI06-A"); !slices.Equal(reasons, []nodomain.DirectionalBasisReason{nodomain.ReasonPendingIdentification}) {
		t.Fatalf("A 缺少 %v, want 只缺·待识别", reasons)
	}
	if n := envelopeCountOfType(t, db, nodeIntakeEnvelopeType); n != intents+1 {
		t.Fatalf("收寄意图 %d 封, want %d（A 早先那一封加 B 的处置版一封）", n, intents+1)
	}

	rows := countReceptionRows(t, pool)
	status, body = postWiredDisposition(t, endpoint, designateBody(late, conflict))
	if status != http.StatusOK || body["outcome"] != "EXISTING_RESULT" {
		t.Fatalf("原样重投答 %d %v, want 200 EXISTING_RESULT", status, body)
	}
	if countReceptionRows(t, pool) != rows || envelopeCountOfType(t, db, nodeIntakeEnvelopeType) != intents+1 {
		t.Fatal("原样重投多落了一行或多交了一封")
	}
	status, body = postWiredDisposition(t, endpoint, `{"unit":"SYN-UNIT-NRI06-B","disposedVersion":"SYN-V-ABSENT","result":"NONE_OF_CANDIDATES","basis":"SYN-BASIS"}`)
	if _, carries := body["outcome"]; status != http.StatusNotFound || carries {
		t.Fatalf("查无此版答 %d %v, want 404 不带 outcome", status, body)
	}
}

// Covers: 票 06 完成判据「并发：处置与同一包裹上又一件实物的收寄并发，经包裹锁串行，不出现一版两个后继」与 `AT-NO-059` 的真库面。
// 处置取完那件包裹的锁之后挂住，C 贴同一个号经生产装配送站，等在那件包裹的锁上；处置提交后 C 只与此刻关联到那件包裹的 B 形成一码
// 两物——冲突版接在 B 的处置版后面，A 的链头仍是它的「不是它」那一版，没有一版有两个后继。C 也带着那个客户账户的送站预报（ADR-0212
// 决定四），号码才唯一识别得出包裹、走到包裹锁。
func TestADispositionAndAnotherUnitsReceptionSerializeOnTheParcel(t *testing.T) {
	db, pool := identityRecheckDB(t)
	conflict, late := oneMarkOnRealAssembly(t, db)
	reception, err := buildReceptionOrchestration(db)
	if err != nil {
		t.Fatal(err)
	}
	third := receptionCommand(t, "SYN-DELIVERY-NRI06-C", nodeopsapp.ExplicitReception)
	third.Unit = mustValue(t, nodomain.NewHandlingUnitID, "SYN-UNIT-NRI06-C")
	third.Mark = noports.ExternalMarkObservation{Mark: "SYN-CUSTOMER-PARCEL-NRI06"}
	third.ScopeBasis = preAdviseOnRealDatabase(t, db, "SYN-CUSTOMER-1", "SYN-CUSTOMER-PARCEL-NRI06")

	var (
		thirdResult nodeopsapp.ReceiveDeliveredUnitResult
		thirdErr    error
		thirdDone   chan struct{}
		waited      bool
	)
	deps, err := buildIdentityDispositionDeps(db)
	if err != nil {
		t.Fatalf("装配处置依赖：%v", err)
	}
	deps.Receptions = pausingReceptions{ReceptionStore: deps.Receptions, afterFindOthers: func() {
		if thirdDone != nil {
			return
		}
		thirdDone = make(chan struct{})
		go func(done chan struct{}) {
			defer close(done)
			thirdResult, thirdErr = reception.Handle(t.Context(), third)
		}(thirdDone)
		waited = awaitAdvisoryLockWaitOrDone(t, pool, thirdDone)
	}}
	paused, err := identityDispositionWithin(db, deps)
	if err != nil {
		t.Fatalf("装配挂住的处置编排：%v", err)
	}
	result, disposeErr := paused.Dispose(t.Context(), nodeopsapp.DisposeIdentityConflictCommand{
		TenantID: late.Key.TenantID, Operator: "OPERATOR/https://id.syn.example/dex#SYN-OPERATOR-01",
		Unit: late.Intake.Unit(), Disposed: late.JudgmentVersion, Result: nodomain.DispositionDesignatesParcel,
		Parcel:     mustValue(t, nodomain.NewParcelAssociationReference, "syn-parcel-1"),
		CoDisposed: []nodomain.CoDisposedUnit{{Unit: conflict.Intake.Unit(), Version: conflict.JudgmentVersion}},
		Basis:      "SYN-DISPOSITION-BASIS-NRI06",
	})
	if thirdDone == nil {
		t.Fatal("处置没走到按包裹读别的实物关联那一步")
	}
	awaitDone(t, thirdDone)
	if disposeErr != nil || thirdErr != nil {
		t.Fatalf("处置 err = %v，C err = %v", disposeErr, thirdErr)
	}
	if result.Outcome() != nodeopsapp.IdentityConflictDisposed || thirdResult.Outcome() != nodeopsapp.UnitPendingIdentification {
		t.Fatalf("处置 %v / C %v, want IDENTITY_CONFLICT_DISPOSED / PENDING_IDENTIFICATION", result.Outcome(), thirdResult.Outcome())
	}
	if !waited {
		t.Fatal("C 没在包裹锁上等就走完了")
	}
	var forked int
	if err := pool.QueryRow(t.Context(),
		`SELECT count(*) FROM (SELECT predecessor_version FROM node_operations.reception
		  WHERE predecessor_version IS NOT NULL GROUP BY tenant_id, predecessor_version HAVING count(*) > 1) AS forks`,
	).Scan(&forked); err != nil || forked != 0 {
		t.Fatalf("%d 版有两个后继（err %v）", forked, err)
	}
	receptions, err := nopostgres.NewReceptions(db)
	if err != nil {
		t.Fatal(err)
	}
	designated, found, err := receptions.FindByKey(t.Context(), noports.ReceptionKey{TenantID: late.Key.TenantID, SourceID: "IDENTITY-DISPOSITION/" + late.JudgmentVersion.String()})
	if err != nil || !found {
		t.Fatalf("读 B 的处置版：found %v err %v", found, err)
	}
	if head, err := receptions.CurrentVersion(t.Context(), late.Key.TenantID, designated.JudgmentVersion); err != nil || head == designated.JudgmentVersion {
		t.Fatalf("C 的冲突版没接在 B 的处置版后面：链头 %s err %v", head, err)
	}
	notIt, found, err := receptions.FindByKey(t.Context(), noports.ReceptionKey{TenantID: late.Key.TenantID, SourceID: "IDENTITY-DISPOSITION/" + conflict.JudgmentVersion.String()})
	if err != nil || !found {
		t.Fatalf("读 A 的处置版：found %v err %v", found, err)
	}
	if head, err := receptions.CurrentVersion(t.Context(), late.Key.TenantID, notIt.JudgmentVersion); err != nil || head != notIt.JudgmentVersion {
		t.Fatalf("A 又被追加了冲突版：链头 %s err %v", head, err)
	}
}

// Covers: `AT-NO-058` 的装配面：处置口挂的是生产那一只运营决定 Intake，信封一层各答其格、都拒在编排之前（编排是一碰就 panic 的
// 占位）——发行方未配置 403、没带令牌 401、只持别的决定种类 403 无授予、授予齐备而准入对照没登 403 不在准入范围；授予与准入都
// 齐备时，请求体自带租户或操作者答 400。
func TestTheWiredDispositionAnswersTheChannelGrades(t *testing.T) {
	db, _ := identityRecheckDB(t)
	registerDispositionAdmission(t, db)
	admitted, err := buildAdmissionScope(db, isolatedSelfAuthority)
	if err != nil {
		t.Fatal(err)
	}
	endpointWith := func(register accessidentity.OperatorRegistry, admission accessidentity.AdmissionScope) http.Handler {
		minter, err := buildOperatorMinter(acceptingVerifier{}, register, admission)
		if err != nil {
			t.Fatal(err)
		}
		decisions, err := buildOperatorDecisionIntakes(minter, unwiredDecisionTargets{}, unwiredDecisionTargets{})
		if err != nil {
			t.Fatal(err)
		}
		return nodeopshttp.NewDisposeIdentityConflictEndpoint(decisions.nodeOperations, unwiredIdentityDisposition{})
	}
	const valid = `{"unit":"SYN-UNIT-NRI06-B","disposedVersion":"SYN-V-B1","result":"NONE_OF_CANDIDATES","basis":"SYN-BASIS"}`
	cases := map[string]struct {
		endpoint http.Handler
		token    string
		body     string
		status   int
		code     string
	}{
		"发行方未配置": {nodeopshttp.NewDisposeIdentityConflictEndpoint(unconfiguredOperatorDecisions().nodeOperations, unwiredIdentityDisposition{}),
			"presented.operator.token", valid, http.StatusForbidden, "ACCESS_CHANNEL_NOT_CONFIGURED"},
		"没带令牌": {endpointWith(dispositionGrantRegister{}, admitted), "", valid, http.StatusUnauthorized, "OPERATOR_CREDENTIAL_REJECTED"},
		"只持别的决定种类": {endpointWith(decisionRegister{kinds: []accessidentity.DecisionKind{accessidentity.DecisionPendingIdentityRecheck}}, admitted),
			"presented.operator.token", valid, http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
		"准入对照没登": {endpointWith(dispositionGrantRegister{}, admissionScopeUnset()), "presented.operator.token", valid, http.StatusForbidden, "OUTSIDE_ADMISSION_SCOPE"},
		"自带租户": {endpointWith(dispositionGrantRegister{}, admitted), "presented.operator.token",
			`{"unit":"SYN-UNIT-NRI06-B","disposedVersion":"SYN-V-B1","result":"NONE_OF_CANDIDATES","basis":"SYN-BASIS","tenantId":"SYN-TENANT-02"}`,
			http.StatusBadRequest, "MALFORMED_REQUEST"},
		"自带操作者": {endpointWith(dispositionGrantRegister{}, admitted), "presented.operator.token",
			`{"unit":"SYN-UNIT-NRI06-B","disposedVersion":"SYN-V-B1","result":"NONE_OF_CANDIDATES","basis":"SYN-BASIS","operator":"SYN-OPERATOR-02"}`,
			http.StatusBadRequest, "MALFORMED_REQUEST"},
	}
	for name, testCase := range cases {
		request := httptest.NewRequest(http.MethodPost, "/node-operations/identity-dispositions", strings.NewReader(testCase.body))
		if testCase.token != "" {
			request.Header.Set("Authorization", "Bearer "+testCase.token)
		}
		response := httptest.NewRecorder()
		testCase.endpoint.ServeHTTP(response, request)
		if response.Code != testCase.status || problemCode(t, response) != testCase.code {
			t.Fatalf("%s：答 %d %q, want %d %q", name, response.Code, problemCode(t, response), testCase.status, testCase.code)
		}
	}
}
