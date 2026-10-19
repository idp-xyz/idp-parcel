package postgres_test

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	adapter "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件对真实 PostgreSQL 16 证委托聚合仓储的行为：全聚合快照往返不丢字段、作用域
// 隔离由 SQL 条件承担、重复建单由主键拦住、并发保存由乐观版本拦住、重建门开到
// `已提交`、`已接受`与`已拒绝`（ADR-0061、ADR-0188）由读回侧如实透出。

func TestASubmittedRequestRoundTripsWholly(t *testing.T) {
	repository, transactor, _ := newShipmentRequests(t)
	ctx := t.Context()

	original := submittedShipmentRequest(t, "req-key-1", "request-1")
	mustInsert(t, transactor, ctx, repository, original)

	found, exists, err := repository.FindBySourceIdentity(ctx, requestIdentity(t, "req-key-1"))
	if err != nil {
		t.Fatalf("取回委托：%v", err)
	}
	if !exists {
		t.Fatal("已建单的委托读不回来")
	}
	if found.Revision() != 1 {
		t.Fatalf("revision = %d, want 1（Insert 写 1）", found.Revision())
	}
	if found.ShipmentRequestID() != original.ShipmentRequestID() ||
		found.BatchID() != original.BatchID() ||
		found.State() != original.State() ||
		!found.SubmittedAt().Equal(original.SubmittedAt()) {
		t.Fatalf("委托身份/状态往返变形：%+v", found)
	}

	foundVersion, originalVersion := found.CurrentSubmissionVersion(), original.CurrentSubmissionVersion()
	if foundVersion.VersionID() != originalVersion.VersionID() ||
		foundVersion.SourceSubmission() != originalVersion.SourceSubmission() ||
		len(foundVersion.DeclaredParcelIDs()) != 2 {
		t.Fatalf("提交版本往返变形：%+v", foundVersion)
	}
	// 画像（重量+外廓）随版本往返：丢了它，估价装配从此永远停在缺输入。
	profile, present := foundVersion.ProfileFor(mustBuild(t, domain.NewDeclaredParcelID, "parcel-1"))
	if !present {
		t.Fatal("成员声明画像没有读回来")
	}
	if profile.Measurement().Weight().Value().String() != "2.5" {
		t.Fatalf("weight = %q, want 2.5", profile.Measurement().Weight().Value())
	}
	if _, declared := profile.Measurement().Dimensions(); !declared {
		t.Fatal("外廓没有读回来")
	}

	foundTask, originalTask := found.AcceptanceDecisionTask(), original.AcceptanceDecisionTask()
	if foundTask.TaskID() != originalTask.TaskID() ||
		foundTask.SubmissionVersionID() != originalTask.SubmissionVersionID() ||
		foundTask.IsComplete() || foundTask.IsStopped() {
		t.Fatalf("接受判断任务往返变形：%+v", foundTask)
	}
}

// 票 psb/17：提交版本上委托声明的服务产品随快照往返，值原样；不声明的版本读回仍未声明。丢了它，商业依据解析键
// 回读时就收窄不了候选，同一范围两个产品必然冲突。
func TestTheRequestedServiceProductRoundTripsWithItsSubmissionVersion(t *testing.T) {
	repository, transactor, _ := newShipmentRequests(t)
	ctx := t.Context()

	spec := submittedShipmentRequestSpec(t, "req-key-1", "request-1")
	spec.RequestedProduct = domain.RequestedServiceProductOf([]domain.CanonicalContentEntry{
		mustEntry(t, domain.RequestedServiceProductEntryName, " SYN-PROD-CN-SG-EXPRESS "),
	})
	declared, err := domain.SubmitShipmentRequest(spec)
	if err != nil {
		t.Fatalf("建单：%v", err)
	}
	mustInsert(t, transactor, ctx, repository, declared)
	mustInsert(t, transactor, ctx, repository, submittedShipmentRequest(t, "req-key-2", "request-2"))

	found := mustFind(t, repository, ctx, "req-key-1")
	if got := found.CurrentSubmissionVersion().RequestedServiceProduct(); got.String() != " SYN-PROD-CN-SG-EXPRESS " {
		t.Fatalf("读回的声明产品 = %q，值要原样", got.String())
	}
	if mustFind(t, repository, ctx, "req-key-2").CurrentSubmissionVersion().RequestedServiceProduct().Declared() {
		t.Fatal("没声明产品的版本读回成了已声明")
	}
}

func mustEntry(t *testing.T, name, value string) domain.CanonicalContentEntry {
	t.Helper()
	entry, err := domain.NewCanonicalContentEntry(name, value)
	if err != nil {
		t.Fatalf("条目 %q：%v", name, err)
	}
	return entry
}

// TestHistoryAndAttemptsSurviveTheRoundTrip 证受控补充历史（ADR-0045）与处理记录随
// 快照往返：丢历史，一份补充过的委托重建后看起来像从未补充过；丢处理记录，「卡过几轮」
// 从头数起。
func TestHistoryAndAttemptsSurviveTheRoundTrip(t *testing.T) {
	repository, transactor, _ := newShipmentRequests(t)
	ctx := t.Context()

	original := submittedShipmentRequest(t, "req-key-1", "request-1")
	mustInsert(t, transactor, ctx, repository, original)

	loaded, _, err := repository.FindBySourceIdentity(ctx, requestIdentity(t, "req-key-1"))
	if err != nil {
		t.Fatalf("读回：%v", err)
	}
	withAttempt, err := loaded.RecordProcessingAttempt(processingAttempt(t))
	if err != nil {
		t.Fatalf("记处理尝试：%v", err)
	}
	// 新版本必须以自己的来源身份到达（领域守卫）：换一个来源请求键。
	superseded, err := withAttempt.FormNewSubmissionVersion(domain.NewSubmissionVersionSpec{
		VersionID:        mustBuild(t, domain.NewSubmissionVersionID, "version-2"),
		TaskID:           mustBuild(t, domain.NewAcceptanceDecisionTaskID, "task-2"),
		SourceSubmission: requestFingerprint(t, "req-key-1-supplement", "digest-2"),
		// 受控补充不改成员集合（改了要走关联新委托）：与当前版本等值的两个成员。
		DeclaredParcelIDs: []domain.DeclaredParcelID{
			mustBuild(t, domain.NewDeclaredParcelID, "parcel-1"),
			mustBuild(t, domain.NewDeclaredParcelID, "parcel-2"),
		},
		EstablishedAt: submittedAtFixture.Add(2 * time.Hour),
	})
	if err != nil {
		t.Fatalf("形成新提交版本：%v", err)
	}

	var saved ports.ShipmentRequestSaveOutcome
	mustWithinTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		var err error
		saved, err = repository.Save(txCtx, requestIdentity(t, "req-key-1"), superseded)
		return err
	})
	if saved != ports.ShipmentRequestSaved {
		t.Fatalf("save outcome = %s, want SAVED", saved)
	}

	found, _, err := repository.FindBySourceIdentity(ctx, requestIdentity(t, "req-key-1"))
	if err != nil {
		t.Fatalf("读回换代后的委托：%v", err)
	}
	if found.Revision() != 2 {
		t.Fatalf("revision = %d, want 2（一次保存推进一格）", found.Revision())
	}
	if found.CurrentSubmissionVersion().VersionID() != mustBuild(t, domain.NewSubmissionVersionID, "version-2") {
		t.Fatalf("当前版本 = %+v", found.CurrentSubmissionVersion().VersionID())
	}
	priors := found.PriorSubmissionVersions()
	if len(priors) != 1 || priors[0].VersionID() != mustBuild(t, domain.NewSubmissionVersionID, "version-1") {
		t.Fatalf("历史版本 = %+v；补充过的委托读回后像从未补充过", priors)
	}
	priorTasks := found.PriorAcceptanceTasks()
	if len(priorTasks) != 1 || len(priorTasks[0].ProcessingAttempts()) != 1 {
		t.Fatalf("历史任务或处理记录丢失：%+v", priorTasks)
	}
}

// TestOtherScopesAreInvisibleForRequests 证否定结果不泄露其他作用域是否存在该委托。
func TestOtherScopesAreInvisibleForRequests(t *testing.T) {
	repository, transactor, _ := newShipmentRequests(t)
	ctx := t.Context()

	mustInsert(t, transactor, ctx, repository, submittedShipmentRequest(t, "shared-key", "request-1"))

	elsewhere := map[string]domain.SourceIdentity{
		"另一个租户":   requestScopedIdentity(t, "tenant-b", "customer-1", "shared-key"),
		"另一个客户账户": requestScopedIdentity(t, "tenant-1", "customer-b", "shared-key"),
	}
	for name, scope := range elsewhere {
		_, exists, err := repository.FindBySourceIdentity(ctx, scope)
		if err != nil {
			t.Fatalf("%s：查询出错 %v", name, err)
		}
		if exists {
			t.Errorf("%s：读到了不属于该作用域的委托", name)
		}
	}
}

// TestInsertingTwiceReportsAlreadyExists 证并发重复建单由主键拦住并译成业务答案
// （ADR-0031：`已存在`不是 error），先到者的快照原样保留。
func TestInsertingTwiceReportsAlreadyExists(t *testing.T) {
	repository, transactor, _ := newShipmentRequests(t)
	ctx := t.Context()

	first := submittedShipmentRequest(t, "req-key-1", "request-1")
	mustInsert(t, transactor, ctx, repository, first)

	second := submittedShipmentRequest(t, "req-key-1", "request-2")
	var outcome ports.ShipmentRequestInsertOutcome
	mustWithinTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		var err error
		outcome, err = repository.Insert(txCtx, requestIdentity(t, "req-key-1"), second)
		return err
	})
	if outcome != ports.ShipmentRequestAlreadyExists {
		t.Fatalf("outcome = %s, want ALREADY_EXISTS", outcome)
	}

	found, _, err := repository.FindBySourceIdentity(ctx, requestIdentity(t, "req-key-1"))
	if err != nil {
		t.Fatalf("读回：%v", err)
	}
	if found.ShipmentRequestID() != first.ShipmentRequestID() {
		t.Fatal("后到者覆盖了先到者的委托")
	}
}

// TestSaveDetectsTheRevisionConflict 证并发保存由乐观版本拦住：拿着旧版本的一方零行
// 命中，译成`版本冲突`而不是覆盖赢家。
func TestSaveDetectsTheRevisionConflict(t *testing.T) {
	repository, transactor, _ := newShipmentRequests(t)
	ctx := t.Context()

	mustInsert(t, transactor, ctx, repository, submittedShipmentRequest(t, "req-key-1", "request-1"))
	stale, _, err := repository.FindBySourceIdentity(ctx, requestIdentity(t, "req-key-1"))
	if err != nil {
		t.Fatalf("读回：%v", err)
	}

	// 赢家先保存一次（revision 1→2）。
	var winner ports.ShipmentRequestSaveOutcome
	mustWithinTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		var err error
		winner, err = repository.Save(txCtx, requestIdentity(t, "req-key-1"), stale)
		return err
	})
	if winner != ports.ShipmentRequestSaved {
		t.Fatalf("winner outcome = %s", winner)
	}

	// 落败方拿着同一份 revision 1 的聚合再保存：零行命中。
	var loser ports.ShipmentRequestSaveOutcome
	mustWithinTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		var err error
		loser, err = repository.Save(txCtx, requestIdentity(t, "req-key-1"), stale)
		return err
	})
	if loser != ports.ShipmentRequestRevisionConflict {
		t.Fatalf("loser outcome = %s, want REVISION_CONFLICT", loser)
	}
}

// TestRequestWritesRefuseToRunOutsideATransaction 证写入不会在缺少事务时改用连接池
// （`PBC-08`「不绕过 DB.RequireExecutor」的行为面）。
func TestRequestWritesRefuseToRunOutsideATransaction(t *testing.T) {
	repository, _, _ := newShipmentRequests(t)
	ctx := t.Context()

	request := submittedShipmentRequest(t, "req-key-1", "request-1")
	if _, err := repository.Insert(ctx, requestIdentity(t, "req-key-1"), request); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务建单应返回 ErrTransactionRequired，实得：%v", err)
	}
	if _, err := repository.Save(ctx, requestIdentity(t, "req-key-1"), request); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务保存应返回 ErrTransactionRequired，实得：%v", err)
	}
}

// TestUnsupportedStatesSurfaceOnRead 证门外状态与坏快照分格（ADR-0061、ADR-0188）：只把状态列推到
// `已接受`或`已拒绝`、快照仍是已提交形状，读回是坏数据而不是「本期不支持」——门已经开了，缺产物就是
// 这一行立不起来。`已撤回`仍走不支持哨兵。
func TestUnsupportedStatesSurfaceOnRead(t *testing.T) {
	repository, transactor, pool := newShipmentRequests(t)
	ctx := t.Context()

	// 状态列推进到已开门的状态、文档仍是已提交形状：门开着，缺产物就是这一行立不起来（ADR-0061、ADR-0188）。
	// `已拒绝`不再算门外——一份没有决定的已拒绝快照是坏数据，不是「等门开」。
	opened := map[string]domain.ShipmentRequestState{
		"req-key-accepted": domain.ShipmentRequestAccepted,
		"req-key-rejected": domain.ShipmentRequestRejected,
	}
	for key, state := range opened {
		mustInsert(t, transactor, ctx, repository, submittedShipmentRequest(t, key, "request-"+key))
		if _, err := pool.Exec(ctx,
			`UPDATE parcel_shipment.shipment_request SET state = $1 WHERE source_request_key = $2`,
			uint8(state), key); err != nil {
			t.Fatalf("推进状态列到 %s：%v", state, err)
		}
		_, _, err := repository.FindBySourceIdentity(ctx, requestIdentity(t, key))
		if !errors.Is(err, domain.ErrInvalidRehydratedShipmentRequest) {
			t.Fatalf("%s 缺产物：err = %v, want ErrInvalidRehydratedShipmentRequest", state, err)
		}
	}

	mustInsert(t, transactor, ctx, repository, submittedShipmentRequest(t, "req-key-withdrawn", "request-withdrawn"))
	if _, err := pool.Exec(ctx,
		`UPDATE parcel_shipment.shipment_request SET state = $1 WHERE source_request_key = $2`,
		uint8(domain.ShipmentRequestWithdrawn), "req-key-withdrawn"); err != nil {
		t.Fatalf("推进状态列到已撤回：%v", err)
	}
	_, _, err := repository.FindBySourceIdentity(ctx, requestIdentity(t, "req-key-withdrawn"))
	if !errors.Is(err, domain.ErrRehydrationStateNotSupported) {
		t.Fatalf("已撤回：err = %v, want ErrRehydrationStateNotSupported", err)
	}
}

// Covers: ADR-0188——`已拒绝`三条来路（规则拒绝分带依据与不带依据两格）都经真实写路径落库再读回，决定与
// 主动拒绝留痕逐格相等；处置`拒绝`的处置记录随任务一起回来。写侧丢留痕或读侧在依据解码处报裸错，
// 凡按来源身份读委托的编排（提交重放、`AT-PS-036` 关联、两口重放、续办信封）都会读不回已拒绝委托。
func TestRejectionsRoundTripWithTheirTrail(t *testing.T) {
	repository, transactor, pool := newShipmentRequests(t)
	ctx := t.Context()

	for _, path := range rejectionPaths(t) {
		t.Run(path.name, func(t *testing.T) {
			key := "req-key-" + path.name
			mustInsert(t, transactor, ctx, repository, path.submitted(t, key, "request-"+path.name))
			rejected, err := path.reject(t, mustFind(t, repository, ctx, key))
			if err != nil || rejected.State() != domain.ShipmentRequestRejected {
				t.Fatalf("形成拒绝：state = %q, err = %v", rejected.State(), err)
			}
			mustSave(t, transactor, ctx, repository, requestIdentity(t, key), rejected)

			var authority, decider, reason, evidence *string
			if err := pool.QueryRow(ctx,
				`SELECT snapshot->'decision'->'activeRejection'->>'authority',
				        snapshot->'decision'->'activeRejection'->>'decider',
				        snapshot->'decision'->'activeRejection'->>'reason',
				        snapshot->'decision'->'activeRejection'->>'evidence'
				   FROM parcel_shipment.shipment_request WHERE source_request_key = $1`, key,
			).Scan(&authority, &decider, &reason, &evidence); err != nil {
				t.Fatalf("读快照留痕：%v", err)
			}
			want, _ := rejected.AcceptanceDecision()
			if trail, active := want.ActiveRejection(); active {
				if authority == nil || *authority != trail.Authority().String() ||
					decider == nil || *decider != trail.Decider().String() ||
					reason == nil || *reason != trail.Reason().String() ||
					evidence == nil || *evidence != trail.Evidence().String() {
					t.Fatalf("快照留痕 = %v/%v/%v/%v, want %+v——留痕在落库那一刻丢了", authority, decider, reason, evidence, trail)
				}
			} else if authority != nil || decider != nil || reason != nil || evidence != nil {
				t.Fatal("规则拒绝的快照凭空长出了主动拒绝留痕")
			}

			found := mustFind(t, repository, ctx, key)
			if found.State() != domain.ShipmentRequestRejected || found.Revision() != rejected.Revision()+1 {
				t.Fatalf("state = %q revision = %d", found.State(), found.Revision())
			}
			got, present := found.AcceptanceDecision()
			if !present || !reflect.DeepEqual(got, want) {
				t.Fatalf("决定往返不等：\n got = %#v\nwant = %#v", got, want)
			}
			if !reflect.DeepEqual(found.AcceptanceDecisionTask(), rejected.AcceptanceDecisionTask()) {
				t.Fatalf("判断任务往返不等：\n got = %#v\nwant = %#v", found.AcceptanceDecisionTask(), rejected.AcceptanceDecisionTask())
			}
			if _, present := found.AcceptanceBaseline(); present {
				t.Fatal("已拒绝委托读回长出了接受基线")
			}
			if _, present := found.ExpectedCommitment(); present {
				t.Fatal("已拒绝委托读回长出了预计承诺")
			}
		})
	}
}

// Covers: ADR-0188 决定三之 1、6——补表达之前写下的主动拒绝行（决定上没有留痕、也没有失败项）与两样都有的
// 行读回都是坏数据，不折成「本期不支持」，也不从任务处置记录推回留痕。
func TestRejectedRowsWithoutOrWithBothExplanationsAreInvalidOnRead(t *testing.T) {
	repository, transactor, pool := newShipmentRequests(t)
	ctx := t.Context()
	paths := map[string]rejectionPath{}
	for _, path := range rejectionPaths(t) {
		paths[path.name] = path
	}
	cases := map[string]struct {
		path    string
		rewrite string
	}{
		"补表达前的主动拒绝行": {path: "active", rewrite: `snapshot #- '{decision,activeRejection}'`},
		"补表达前的处置拒绝行": {path: "disposition", rewrite: `snapshot #- '{decision,activeRejection}'`},
		"规则拒绝又带留痕": {path: "rule-without-basis", rewrite: `jsonb_set(snapshot, '{decision,activeRejection}',
			'{"authority":"SYN-REJECT-RULE-1/v1","decider":"SYN-OPERATOR-01","reason":"SYN-REJECT-REASON-1","evidence":"SYN-REJECT-EVIDENCE-1"}')`},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			path := paths[c.path]
			key := "req-key-old-" + c.path
			mustInsert(t, transactor, ctx, repository, path.submitted(t, key, "request-old-"+c.path))
			rejected, err := path.reject(t, mustFind(t, repository, ctx, key))
			if err != nil {
				t.Fatalf("形成拒绝：%v", err)
			}
			mustSave(t, transactor, ctx, repository, requestIdentity(t, key), rejected)
			if _, err := pool.Exec(ctx,
				`UPDATE parcel_shipment.shipment_request SET snapshot = `+c.rewrite+` WHERE source_request_key = $1`, key,
			); err != nil {
				t.Fatalf("改写快照：%v", err)
			}

			_, _, err = repository.FindBySourceIdentity(ctx, requestIdentity(t, key))
			if !errors.Is(err, domain.ErrInvalidRehydratedShipmentRequest) {
				t.Fatalf("读回 err = %v, want ErrInvalidRehydratedShipmentRequest", err)
			}
			if errors.Is(err, domain.ErrRehydrationStateNotSupported) {
				t.Fatalf("读回 err = %v；门已开到`已拒绝`，坏数据不得折成未开门", err)
			}
		})
	}
}

// rejectionPath 是`已拒绝`的一条来路：从哪一份`已提交`委托出发、经哪个转移形成拒绝。
type rejectionPath struct {
	name      string
	submitted func(t *testing.T, key, requestID string) domain.ShipmentRequest
	reject    func(t *testing.T, loaded domain.ShipmentRequest) (domain.ShipmentRequest, error)
}

// rejectionPaths 列`已拒绝`的三条来路（规则拒绝按带不带商业依据分两格）。值一律合成。
func rejectionPaths(t *testing.T) []rejectionPath {
	t.Helper()
	rejectedAt := submittedAtFixture.Add(time.Hour)
	failing := func(t *testing.T) []domain.AcceptanceCheck {
		return []domain.AcceptanceCheck{
			versionCheck(t, domain.CustomerRelationshipCheck, domain.CheckFailed, "NO_APPLICABLE_BASIS"),
			versionCheck(t, domain.LegalEntityAndContractCheck, domain.CheckFailed, "NO_APPLICABLE_BASIS"),
		}
	}
	return []rejectionPath{
		{
			name:      "rule-with-basis",
			submitted: submittedShipmentRequest,
			reject: func(t *testing.T, loaded domain.ShipmentRequest) (domain.ShipmentRequest, error) {
				return loaded.Decide(domain.AcceptanceDecisionSpec{
					DecisionID: mustBuild(t, domain.NewAcceptanceDecisionID, "SYN-DECISION-RULE-BASIS"),
					Checks:     failing(t),
					Basis:      acceptanceBasis(t),
					DecidedAt:  rejectedAt,
				})
			},
		},
		{
			// `没有适用合同`：依据带不出来，决定照样形成。
			name:      "rule-without-basis",
			submitted: submittedShipmentRequest,
			reject: func(t *testing.T, loaded domain.ShipmentRequest) (domain.ShipmentRequest, error) {
				return loaded.Decide(domain.AcceptanceDecisionSpec{
					DecisionID: mustBuild(t, domain.NewAcceptanceDecisionID, "SYN-DECISION-RULE-NOBASIS"),
					Checks:     failing(t),
					DecidedAt:  rejectedAt,
				})
			},
		},
		{
			name:      "active",
			submitted: submittedShipmentRequest,
			reject: func(t *testing.T, loaded domain.ShipmentRequest) (domain.ShipmentRequest, error) {
				return loaded.RejectByAuthority(domain.ActiveRejectionSpec{
					DecisionID: mustBuild(t, domain.NewAcceptanceDecisionID, "SYN-DECISION-ACTIVE"),
					Authority:  mustBuild(t, domain.NewRejectionAuthorityReference, "SYN-REJECT-RULE-1/v1"),
					Decider:    mustBuild(t, domain.NewDeciderReference, "SYN-OPERATOR-01"),
					Reason:     mustBuild(t, domain.NewRejectionReasonReference, "SYN-REJECT-REASON-1"),
					Evidence:   mustBuild(t, domain.NewRejectionEvidenceReference, "SYN-REJECT-EVIDENCE-1"),
					DecidedAt:  rejectedAt,
				})
			},
		},
		{
			name:      "disposition",
			submitted: awaitingDispositionShipmentRequest,
			reject: func(t *testing.T, loaded domain.ShipmentRequest) (domain.ShipmentRequest, error) {
				return loaded.DisposeUnderAuthority(domain.DisposeUnderAuthoritySpec{
					Disposition: authorizedDispositionFixture(t, domain.DisposeByRejection),
					DecisionID:  mustBuild(t, domain.NewAcceptanceDecisionID, "SYN-DECISION-DISPOSE"),
				})
			},
		},
	}
}

// TestAnAcceptedRequestRoundTripsWholly 证 Insert→Decide→Save→Find 把已接受产物整份带回：
// 决定、基线、承诺都在，且不重算 Decide。丢基线会让后续资料更正全部落到基线外。
func TestAnAcceptedRequestRoundTripsWholly(t *testing.T) {
	repository, transactor, _ := newShipmentRequests(t)
	ctx := t.Context()

	mustInsert(t, transactor, ctx, repository, submittedShipmentRequest(t, "req-key-1", "request-1"))
	loaded := mustFind(t, repository, ctx, "req-key-1")
	accepted, err := loaded.Decide(acceptanceDecisionSpec(t))
	if err != nil {
		t.Fatalf("形成接受：%v", err)
	}
	if accepted.State() != domain.ShipmentRequestAccepted {
		t.Fatalf("state = %q, want ACCEPTED", accepted.State())
	}
	mustSave(t, transactor, ctx, repository, requestIdentity(t, "req-key-1"), accepted)

	found := mustFind(t, repository, ctx, "req-key-1")
	if found.Revision() != 2 {
		t.Fatalf("revision = %d, want 2", found.Revision())
	}
	if found.State() != domain.ShipmentRequestAccepted {
		t.Fatalf("state = %q, want ACCEPTED", found.State())
	}
	decision, present := found.AcceptanceDecision()
	if !present || !decision.Accepted() || decision.DecisionID().String() != "accept-1" {
		t.Fatalf("decision = %#v present = %v", decision, present)
	}
	if decision.ManualReview() != domain.ManualReviewNotRequired {
		t.Fatalf("manual review = %d, want NOT_REQUIRED", decision.ManualReview())
	}
	if len(decision.Checks()) != 8 {
		t.Fatalf("checks = %d, want 8", len(decision.Checks()))
	}
	baseline, present := found.AcceptanceBaseline()
	if !present || baseline.SubmissionVersionID().String() != "version-1" {
		t.Fatalf("baseline = %#v present = %v", baseline, present)
	}
	if len(baseline.DeclaredParcelIDs()) != 2 {
		t.Fatalf("baseline members = %d, want 2", len(baseline.DeclaredParcelIDs()))
	}
	commitment, present := found.ExpectedCommitment()
	if !present || commitment.Basis().ResolutionID().String() != "RES-1" {
		t.Fatalf("commitment = %#v present = %v", commitment, present)
	}
	if commitment.Basis().ViewRevision() != decision.Basis().ViewRevision() {
		t.Fatal("承诺依据与决定依据往返后对不上")
	}
	if !commitment.FormedAt().Equal(decision.DecidedAt()) || !baseline.FixedAt().Equal(decision.DecidedAt()) {
		t.Fatal("基线或承诺时刻与决定时刻往返后对不上")
	}
	if len(found.CustomerSourceDataVersions()) != 0 {
		t.Fatal("未修订的已接受委托凭空长出了客户资料版本")
	}
}

// TestAcceptedSourceDataVersionsSurviveSaveAndFind 证资料版本随已接受聚合往返：登记册
// 另有一份只追加表，但聚合快照仍是重建入口的权威；丢了它，一份已经补过资料的委托读回来
// 像从未补过。
func TestAcceptedSourceDataVersionsSurviveSaveAndFind(t *testing.T) {
	repository, transactor, _ := newShipmentRequests(t)
	ctx := t.Context()

	mustInsert(t, transactor, ctx, repository, submittedShipmentRequest(t, "req-key-1", "request-1"))
	loaded := mustFind(t, repository, ctx, "req-key-1")
	accepted, err := loaded.Decide(acceptanceDecisionSpec(t))
	if err != nil {
		t.Fatalf("形成接受：%v", err)
	}
	mustSave(t, transactor, ctx, repository, requestIdentity(t, "req-key-1"), accepted)

	accepted = mustFind(t, repository, ctx, "req-key-1")
	scope, err := domain.NewShipmentScopedSourceData(
		accepted.ShipmentRequestID(),
		mustBuild(t, domain.NewSourceDataGroupReference, "SHIPPER_NAME"),
	)
	if err != nil {
		t.Fatalf("资料范围：%v", err)
	}
	version, err := domain.FormCustomerSourceDataVersion(domain.CustomerSourceDataVersionSpec{
		VersionID: mustBuild(t, domain.NewSourceDataVersionID, "src-ver-1"),
		Scope:     scope,
		Basis:     domain.NewSupplementOnAcceptanceBaseline(),
		Intent:    domain.SupplementIntent,
		Request:   requestFingerprint(t, "req-key-1-amend", "digest-amend"),
		Reason:    mustBuild(t, domain.NewAmendmentReasonReference, "MISSING_SHIPPER"),
		Requester: mustBuild(t, domain.NewRequesterReference, "CUSTOMER-1"),
		Decider:   mustBuild(t, domain.NewDeciderReference, "OPERATOR-1"),
		Authority: mustBuild(t, domain.NewAmendmentAuthoritySnapshot, "AUTH-SNAP-1"),
		FormedAt:  submittedAtFixture.Add(2 * time.Hour),
	})
	if err != nil {
		t.Fatalf("形成资料版本：%v", err)
	}
	amended, err := accepted.AmendCustomerSourceData(version)
	if err != nil {
		t.Fatalf("追加资料版本：%v", err)
	}
	mustSave(t, transactor, ctx, repository, requestIdentity(t, "req-key-1"), amended)

	found := mustFind(t, repository, ctx, "req-key-1")
	kept := found.CustomerSourceDataVersions()
	if len(kept) != 1 || kept[0].VersionID().String() != "src-ver-1" {
		t.Fatalf("versions = %#v", kept)
	}
	if found.SourceDataScopeOutsideAcceptanceBaseline(kept[0].Scope()) {
		t.Fatal("往返后资料范围被当成基线外")
	}
	if kept[0].Intent() != domain.SupplementIntent || !kept[0].Basis().OnAcceptanceBaseline() {
		t.Fatalf("资料版本意图或基准往返变形：%#v", kept[0])
	}
}

// ---- 夹具 ----

var submittedAtFixture = time.Date(2026, 9, 6, 11, 0, 0, 0, time.UTC)

func newShipmentRequests(t *testing.T) (*adapter.ShipmentRequests, bentoapp.Transactor, *pgxpool.Pool) {
	t.Helper()

	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	repository, err := adapter.NewShipmentRequests(db)
	if err != nil {
		t.Fatalf("构造委托仓储：%v", err)
	}
	return repository, db.Transactor(), pool
}

func mustInsert(
	t *testing.T,
	transactor bentoapp.Transactor,
	ctx context.Context,
	repository *adapter.ShipmentRequests,
	request domain.ShipmentRequest,
) {
	t.Helper()
	var outcome ports.ShipmentRequestInsertOutcome
	mustWithinTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		var err error
		outcome, err = repository.Insert(txCtx,
			request.CurrentSubmissionVersion().SourceSubmission().Identity(), request)
		return err
	})
	if outcome != ports.ShipmentRequestInserted {
		t.Fatalf("insert outcome = %s", outcome)
	}
}

func mustSave(
	t *testing.T,
	transactor bentoapp.Transactor,
	ctx context.Context,
	repository *adapter.ShipmentRequests,
	identity domain.SourceIdentity,
	request domain.ShipmentRequest,
) {
	t.Helper()
	var outcome ports.ShipmentRequestSaveOutcome
	mustWithinTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		var err error
		outcome, err = repository.Save(txCtx, identity, request)
		return err
	})
	if outcome != ports.ShipmentRequestSaved {
		t.Fatalf("save outcome = %s", outcome)
	}
}

func mustFind(
	t *testing.T,
	repository *adapter.ShipmentRequests,
	ctx context.Context,
	key string,
) domain.ShipmentRequest {
	t.Helper()
	found, exists, err := repository.FindBySourceIdentity(ctx, requestIdentity(t, key))
	if err != nil {
		t.Fatalf("取回委托：%v", err)
	}
	if !exists {
		t.Fatal("委托读不回来")
	}
	return found
}

func acceptanceDecisionSpec(t *testing.T) domain.AcceptanceDecisionSpec {
	t.Helper()
	return domain.AcceptanceDecisionSpec{
		DecisionID: mustBuild(t, domain.NewAcceptanceDecisionID, "accept-1"),
		Checks:     allGroupsPassing(t),
		Basis:      acceptanceBasis(t),
		DecidedAt:  submittedAtFixture.Add(time.Hour),
	}
}

func allGroupsPassing(t *testing.T) []domain.AcceptanceCheck {
	t.Helper()
	groups := []domain.AcceptanceCheckGroup{
		domain.CustomerRelationshipCheck,
		domain.LegalEntityAndContractCheck,
		domain.ProductAndServiceCheck,
		domain.MemberBaselineCheck,
		domain.RequiredDocumentCheck,
		domain.PreAcceptanceFinancialControlCheck,
	}
	checks := make([]domain.AcceptanceCheck, 0, len(groups)+2)
	for _, group := range groups {
		checks = append(checks, versionCheck(t, group, domain.CheckPassed, ""))
	}
	for _, parcel := range []string{"parcel-1", "parcel-2"} {
		checks = append(checks, parcelCheck(t, domain.NetworkReachabilityCheck, parcel, domain.CheckPassed, ""))
	}
	return checks
}

func versionCheck(t *testing.T, group domain.AcceptanceCheckGroup, outcome domain.CheckOutcome, reason string) domain.AcceptanceCheck {
	t.Helper()
	check, err := domain.NewAcceptanceCheck(group, domain.DeclaredParcelID{}, outcome, checkReason(t, reason))
	if err != nil {
		t.Fatalf("version check: %v", err)
	}
	return check
}

func parcelCheck(t *testing.T, group domain.AcceptanceCheckGroup, parcel string, outcome domain.CheckOutcome, reason string) domain.AcceptanceCheck {
	t.Helper()
	check, err := domain.NewAcceptanceCheck(group, mustBuild(t, domain.NewDeclaredParcelID, parcel), outcome, checkReason(t, reason))
	if err != nil {
		t.Fatalf("parcel check: %v", err)
	}
	return check
}

func checkReason(t *testing.T, reason string) domain.CheckReason {
	t.Helper()
	if reason == "" {
		return domain.CheckReason{}
	}
	return mustBuild(t, domain.NewCheckReason, reason)
}

func acceptanceBasis(t *testing.T) domain.CommercialBasisSnapshot {
	t.Helper()
	return basisWithReviewPolicy(t, domain.ManualReviewNotRequiredByRules)
}

// basisWithReviewPolicy 是 acceptanceBasis 的参数化形：复核队列读面的夹具（票
// acceptance-review-read-face/01）要一份「规则要求复核」的依据，其余组件同一配方。
func basisWithReviewPolicy(t *testing.T, policy domain.ManualReviewPolicy) domain.CommercialBasisSnapshot {
	t.Helper()
	applicable, err := domain.NewApplicableCheckGroups(
		domain.CustomerRelationshipCheck,
		domain.LegalEntityAndContractCheck,
		domain.ProductAndServiceCheck,
		domain.MemberBaselineCheck,
		domain.RequiredDocumentCheck,
		domain.PreAcceptanceFinancialControlCheck,
		domain.NetworkReachabilityCheck,
	)
	if err != nil {
		t.Fatalf("适用校验组：%v", err)
	}
	reachability, err := domain.NewDeclaredAsOf(
		domain.ReachabilityJudgmentKind,
		mustBuild(t, domain.NewAsOfSemanticsReference, "ACCEPTANCE"),
		mustBuild(t, domain.NewAsOfPolicyVersion, "asof-1"),
	)
	if err != nil {
		t.Fatalf("可达性时点：%v", err)
	}
	control, err := domain.NewDeclaredAsOf(
		domain.FinancialControlJudgmentKind,
		mustBuild(t, domain.NewAsOfSemanticsReference, "ACCEPTANCE"),
		mustBuild(t, domain.NewAsOfPolicyVersion, "asof-1"),
	)
	if err != nil {
		t.Fatalf("财务控制时点：%v", err)
	}
	snapshot, err := domain.NewCommercialBasisSnapshot(domain.CommercialBasisSnapshotSpec{
		ResolutionID: mustBuild(t, domain.NewCommercialResolutionID, "RES-1"),
		RulePackage:  mustBuild(t, domain.NewRulePackageReference, "rules-1/v1"),
		ViewRevision: mustBuild(t, domain.NewCommercialViewRevision, "VIEW-1"),
		DeclaredAsOf: []domain.DeclaredAsOf{reachability, control},
		Applicable:   applicable,
		ManualReview: policy,
	})
	if err != nil {
		t.Fatalf("商业依据：%v", err)
	}
	return snapshot
}

func mustBuild[T any](t *testing.T, construct func(string) (T, error), raw string) T {
	t.Helper()
	value, err := construct(raw)
	if err != nil {
		t.Fatalf("construct %q: %v", raw, err)
	}
	return value
}

func requestScopedIdentity(t *testing.T, tenant, customer, key string) domain.SourceIdentity {
	t.Helper()
	built, err := domain.NewSourceIdentity(
		mustBuild(t, domain.NewTenantID, tenant),
		mustBuild(t, domain.NewCustomerAccountID, customer),
		mustBuild(t, domain.NewSource, "portal"),
		mustBuild(t, domain.NewSourceRequestKey, key),
	)
	if err != nil {
		t.Fatalf("来源身份：%v", err)
	}
	return built
}

func requestIdentity(t *testing.T, key string) domain.SourceIdentity {
	t.Helper()
	return requestScopedIdentity(t, "tenant-1", "customer-1", key)
}

func requestFingerprint(t *testing.T, key, digest string) domain.SourceSubmissionFingerprint {
	t.Helper()
	built, err := domain.NewSourceSubmissionFingerprint(
		requestIdentity(t, key),
		mustBuild(t, domain.NewPayloadDigest, digest),
		submittedAtFixture.Add(-time.Hour),
		submittedAtFixture.Add(-time.Hour+time.Second),
	)
	if err != nil {
		t.Fatalf("来源指纹：%v", err)
	}
	return built
}

func processingAttempt(t *testing.T) domain.ProcessingAttempt {
	t.Helper()
	attempt, err := domain.NewProcessingAttempt(domain.ProcessingAttemptSpec{
		Reason:       mustBuild(t, domain.NewProcessingAttemptReason, "COMMERCIAL_BASIS_UNAVAILABLE"),
		ResumePath:   domain.ResumeByInternalRetry,
		Continuation: mustBuild(t, domain.NewOwnershipContinuationReference, "CONT-1"),
		AttemptedAt:  submittedAtFixture.Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("处理记录：%v", err)
	}
	return attempt
}

// submittedShipmentRequest 经领域构造一份带两个成员、一张画像的`已提交`委托——与应用
// 测试的同名夹具同款配方，画像多带一张以证测量往返。
func submittedShipmentRequest(t *testing.T, key, requestID string) domain.ShipmentRequest {
	t.Helper()
	request, err := domain.SubmitShipmentRequest(submittedShipmentRequestSpec(t, key, requestID))
	if err != nil {
		t.Fatalf("建单：%v", err)
	}
	return request
}

// submittedShipmentRequestSpec 是上面那份委托的建单输入，单独交出让别的夹具在同一配方上多带一段（如地址要素子段）。
func submittedShipmentRequestSpec(t *testing.T, key, requestID string) domain.SubmitShipmentRequestSpec {
	t.Helper()

	scope, err := domain.NewAdmissionScope(
		mustBuild(t, domain.NewAdmissionScopeReference, "scope-ref-1"),
		mustBuild(t, domain.NewAdmissionScopeDigest, "scope-1"),
	)
	if err != nil {
		t.Fatalf("准入范围：%v", err)
	}
	validity, err := domain.NewOwnershipValidityInterval(
		submittedAtFixture.Add(-24*time.Hour),
		submittedAtFixture.Add(24*time.Hour),
	)
	if err != nil {
		t.Fatalf("有效区间：%v", err)
	}
	decision, err := domain.NewProductionOwnershipDecision(domain.ProductionOwnershipDecisionSpec{
		DecisionID:       mustBuild(t, domain.NewProductionOwnershipDecisionID, "decision-1"),
		Scope:            scope,
		Authority:        domain.ProductionAuthorityIDPParcel,
		AdmissionControl: domain.AdmissionControlOpen,
		RuleVersion:      mustBuild(t, domain.NewProductionOwnershipRuleVersion, "rule-1"),
		AsOf:             submittedAtFixture,
		Validity:         validity,
		Revision:         mustBuild(t, domain.NewProductionOwnershipRevision, "rev-1"),
		DecisionAt:       submittedAtFixture,
	})
	if err != nil {
		t.Fatalf("归属决定：%v", err)
	}
	gate, err := domain.EvaluateFutureSubmissionGate(
		decision, scope.Digest(),
		mustBuild(t, domain.NewProductionOwnershipRevision, "rev-1"), submittedAtFixture)
	if err != nil {
		t.Fatalf("建单门禁：%v", err)
	}
	candidate, err := domain.NewSubmissionCandidate(
		requestFingerprint(t, key, "digest-1"),
		mustBuild(t, domain.NewSubmissionBatchID, "batch-1"),
		mustBuild(t, domain.NewShipmentRequestID, requestID),
		[]domain.DeclaredParcelID{
			mustBuild(t, domain.NewDeclaredParcelID, "parcel-1"),
			mustBuild(t, domain.NewDeclaredParcelID, "parcel-2"),
		},
	)
	if err != nil {
		t.Fatalf("提交候选：%v", err)
	}

	weight, err := domain.NewDeclaredWeight(
		mustBuild(t, domain.NewMeasurementValue, "2.5"),
		mustBuild(t, domain.NewMeasurementUnitReference, "kg"))
	if err != nil {
		t.Fatalf("申报重量：%v", err)
	}
	dimensions, err := domain.NewDeclaredDimensions(
		mustBuild(t, domain.NewMeasurementValue, "30"),
		mustBuild(t, domain.NewMeasurementValue, "20"),
		mustBuild(t, domain.NewMeasurementValue, "10"),
		mustBuild(t, domain.NewMeasurementUnitReference, "cm"))
	if err != nil {
		t.Fatalf("申报外廓：%v", err)
	}
	measurement, err := domain.NewDeclaredMeasurement(weight, dimensions)
	if err != nil {
		t.Fatalf("申报测量：%v", err)
	}
	profile, err := domain.NewDeclaredParcelProfile(
		mustBuild(t, domain.NewDeclaredParcelID, "parcel-1"), measurement)
	if err != nil {
		t.Fatalf("成员画像：%v", err)
	}

	return domain.SubmitShipmentRequestSpec{
		Candidate:   candidate,
		Gate:        gate,
		VersionID:   mustBuild(t, domain.NewSubmissionVersionID, "version-1"),
		TaskID:      mustBuild(t, domain.NewAcceptanceDecisionTaskID, "task-1"),
		SubmittedAt: submittedAtFixture,
		Profiles:    []domain.DeclaredParcelProfile{profile},
	}
}
