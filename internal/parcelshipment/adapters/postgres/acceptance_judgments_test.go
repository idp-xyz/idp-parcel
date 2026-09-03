package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	adapter "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件对真实 PostgreSQL 16 证接受判断任务四张库的行为：判断整行往返后经领域构造门
// 复验、没判过时读回零值而不是一份看起来通过的空结果、重判以新时点顶替旧判断且历史仍在、
// 采用解析后写覆盖、处理尝试逐轮累积、跨租户读不到、形状矩阵由库内 CHECK 钉住、无事务
// 拒、回滚无痕。

var (
	taskAsOfFirst   = time.Date(2026, 10, 12, 9, 0, 0, 0, time.UTC)
	taskAsOfSecond  = time.Date(2026, 10, 12, 11, 30, 0, 0, time.UTC)
	taskAttemptedAt = time.Date(2026, 10, 12, 9, 5, 0, 0, time.UTC)
)

// TestAdoptedJudgmentsRoundTripThroughTheTask 证三样一起往返：带标识的三值判断、
// 不带标识只带依据的`不适用`、执行通过的财务控制，外加所采用的那次商业解析。
func TestAdoptedJudgmentsRoundTripThroughTheTask(t *testing.T) {
	judgments, transactor, _ := newAcceptanceJudgments(t)
	ctx := t.Context()
	tenant, requestID := psTenant(t, "tenant-1"), taskRequestID(t, "REQ-1")

	reachable := reachabilityJudgment(t, "parcel-1", domain.ReachabilityReachable, "NRJ-1", "", taskAsOfFirst)
	notApplicable := reachabilityJudgment(t, "parcel-2", domain.ReachabilityNotApplicable,
		"", "LABEL_ONLY_CHANNEL_SERVICE", taskAsOfFirst)
	held := financialControlResult(t, domain.FinancialControlHeld, "SAC-1", "", taskAsOfFirst)
	resolution := mustBuild(t, domain.NewCommercialResolutionID, "RES-1")

	mustWithinTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		if err := judgments.RecordReachabilityJudgment(txCtx, tenant, requestID, reachable); err != nil {
			return err
		}
		if err := judgments.RecordReachabilityJudgment(txCtx, tenant, requestID, notApplicable); err != nil {
			return err
		}
		if err := judgments.RecordFinancialControlResult(txCtx, tenant, requestID, held); err != nil {
			return err
		}
		return judgments.RecordAdoptedCommercialResolution(txCtx, tenant, requestID, resolution)
	})

	recorded, err := judgments.LoadRecordedJudgments(ctx, tenant, requestID)
	if err != nil {
		t.Fatalf("读回已采用判断：%v", err)
	}
	if len(recorded.Reachability) != 2 {
		t.Fatalf("读回 %d 项可达性判断，want 2", len(recorded.Reachability))
	}
	if recorded.Reachability[0] != reachable {
		t.Fatalf("三值判断往返变形：%+v", recorded.Reachability[0])
	}
	// `不适用`那一支单独断言：它不带判断标识却必须带依据，往返时任何一头凑数都会让一次
	// 本就不该问的判断变成一次问过且通得过的判断。
	if recorded.Reachability[1] != notApplicable {
		t.Fatalf("`不适用`判断往返变形：%+v", recorded.Reachability[1])
	}
	if recorded.Reachability[1].JudgmentID().String() != "" ||
		recorded.Reachability[1].Basis().String() != "LABEL_ONLY_CHANNEL_SERVICE" {
		t.Fatal("`不适用`读回时被补了一个没人签发过的标识，或丢了依据")
	}
	if recorded.FinancialControl != held {
		t.Fatalf("财务控制结果往返变形：%+v", recorded.FinancialControl)
	}
	if recorded.AdoptedCommercialResolution != resolution {
		t.Fatalf("所采用解析往返变形：%q", recorded.AdoptedCommercialResolution.String())
	}
}

// TestAnUnjudgedTaskReadsBackAsNotYetFormed 证「没判过」读回的是零值。
//
// 财务控制的零值按端口约定就是`尚未形成`，翻译函数据它形成`无法判定`；在这里凑一个结果
// 出来，一次从未执行的控制会看起来像通过了。所采用解析同理：零值让形成决定那一步走首次
// 解析，凑一个标识会让它去重校一份不存在的依据。
func TestAnUnjudgedTaskReadsBackAsNotYetFormed(t *testing.T) {
	judgments, _, _ := newAcceptanceJudgments(t)

	recorded, err := judgments.LoadRecordedJudgments(t.Context(),
		psTenant(t, "tenant-1"), taskRequestID(t, "REQ-NEVER-JUDGED"))
	if err != nil {
		t.Fatalf("读回已采用判断：%v", err)
	}
	if len(recorded.Reachability) != 0 {
		t.Fatalf("一份没判过的委托读回了 %d 项可达性判断", len(recorded.Reachability))
	}
	if recorded.FinancialControl.Outcome() != domain.FinancialControlOutcomeInvalid {
		t.Fatalf("控制从未形成却读回了 %q", recorded.FinancialControl.Outcome())
	}
	if recorded.AdoptedCommercialResolution.String() != "" {
		t.Fatalf("没有采用过依据却读回了 %q", recorded.AdoptedCommercialResolution.String())
	}
}

// TestARejudgedMemberDecidesByItsLatestJudgment 证 `AT-PS-037` 的库面：判断被推翻后以新
// 时点重判，读口交回新的那一份，旧判断留在库里但不参与本轮。
//
// 两份一起交回是不行的：形成决定那一步逐条译成校验结果，而任何一项确定性失败都拒绝整份
// 版本——被推翻的`不可达`会拿自己的失败把重判后的`可达`一起否掉。
func TestARejudgedMemberDecidesByItsLatestJudgment(t *testing.T) {
	judgments, transactor, pool := newAcceptanceJudgments(t)
	ctx := t.Context()
	tenant, requestID := psTenant(t, "tenant-1"), taskRequestID(t, "REQ-1")

	superseded := reachabilityJudgment(t, "parcel-1", domain.ReachabilityUnreachable, "NRJ-1", "", taskAsOfFirst)
	rejudged := reachabilityJudgment(t, "parcel-1", domain.ReachabilityReachable, "NRJ-2", "", taskAsOfSecond)
	firstControl := financialControlResult(t, domain.FinancialControlNotApplicable, "", "PC-NO-CONTROL-1", taskAsOfFirst)
	laterControl := financialControlResult(t, domain.FinancialControlHeld, "SAC-2", "", taskAsOfSecond)

	mustWithinTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		if err := judgments.RecordReachabilityJudgment(txCtx, tenant, requestID, superseded); err != nil {
			return err
		}
		if err := judgments.RecordReachabilityJudgment(txCtx, tenant, requestID, rejudged); err != nil {
			return err
		}
		if err := judgments.RecordFinancialControlResult(txCtx, tenant, requestID, firstControl); err != nil {
			return err
		}
		return judgments.RecordFinancialControlResult(txCtx, tenant, requestID, laterControl)
	})

	recorded, err := judgments.LoadRecordedJudgments(ctx, tenant, requestID)
	if err != nil {
		t.Fatalf("读回已采用判断：%v", err)
	}
	if len(recorded.Reachability) != 1 {
		t.Fatalf("同一成员读回 %d 项判断，want 1——被推翻的那一份仍在参与决定", len(recorded.Reachability))
	}
	if recorded.Reachability[0] != rejudged {
		t.Fatalf("读回的不是重判后的那一份：%+v", recorded.Reachability[0])
	}
	if recorded.FinancialControl != laterControl {
		t.Fatalf("读回的不是最新那次控制结果：%+v", recorded.FinancialControl)
	}
	if rows := countTaskRows(t, pool,
		`SELECT count(*) FROM parcel_shipment.acceptance_reachability_judgment
		  WHERE tenant_id = $1 AND shipment_request_id = $2 AND parcel_id = 'parcel-1'`,
		"tenant-1", "REQ-1"); rows != 2 {
		t.Fatalf("库里 %d 行判断，want 2——重判抹掉了原判断的留痕", rows)
	}
}

// TestRecordingTheSameJudgmentTwiceKeepsTheFirst 证同成员同时点重复到达是重放：保留先到
// 者，且撞键后事务仍可用（编排还要在同一事务里继续办事）。
func TestRecordingTheSameJudgmentTwiceKeepsTheFirst(t *testing.T) {
	judgments, transactor, pool := newAcceptanceJudgments(t)
	ctx := t.Context()
	tenant, requestID := psTenant(t, "tenant-1"), taskRequestID(t, "REQ-1")

	first := reachabilityJudgment(t, "parcel-1", domain.ReachabilityReachable, "NRJ-1", "", taskAsOfFirst)
	// 同一时点上的另一份答复：权威对同一业务时刻只该有一个答案，第二份是重放而不是新判断。
	second := reachabilityJudgment(t, "parcel-1", domain.ReachabilityUnreachable, "NRJ-2", "", taskAsOfFirst)
	mustWithinTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		return judgments.RecordReachabilityJudgment(txCtx, tenant, requestID, first)
	})

	// 闭包只做 IO 并把结果带出来：t.Fatal 系走 runtime.Goexit，回调因而永不返回，
	// WithinTransaction 的提交与回滚两条分支都会被跳过。
	var replayed []domain.ReachabilityJudgment
	mustWithinTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		if err := judgments.RecordReachabilityJudgment(txCtx, tenant, requestID, second); err != nil {
			return err
		}
		recorded, err := judgments.LoadRecordedJudgments(txCtx, tenant, requestID)
		replayed = recorded.Reachability
		return err
	})

	if len(replayed) != 1 || replayed[0] != first {
		t.Fatalf("重放没有保留先到者：%+v", replayed)
	}
	if rows := countTaskRows(t, pool,
		`SELECT count(*) FROM parcel_shipment.acceptance_reachability_judgment
		  WHERE tenant_id = $1 AND shipment_request_id = $2`,
		"tenant-1", "REQ-1"); rows != 1 {
		t.Fatalf("库里 %d 行判断，want 1——重放建了第二份", rows)
	}
}

// TestTheAdoptedResolutionIsReplacedByTheLaterOne 证所采用解析后写覆盖（端口原文），并且
// 同标识重复记录是幂等的——一份委托的多轮判断本就该采用同一次解析。
func TestTheAdoptedResolutionIsReplacedByTheLaterOne(t *testing.T) {
	judgments, transactor, pool := newAcceptanceJudgments(t)
	ctx := t.Context()
	tenant, requestID := psTenant(t, "tenant-1"), taskRequestID(t, "REQ-1")

	first := mustBuild(t, domain.NewCommercialResolutionID, "RES-1")
	revalidated := mustBuild(t, domain.NewCommercialResolutionID, "RES-2")
	mustWithinTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		if err := judgments.RecordAdoptedCommercialResolution(txCtx, tenant, requestID, first); err != nil {
			return err
		}
		if err := judgments.RecordAdoptedCommercialResolution(txCtx, tenant, requestID, first); err != nil {
			return err
		}
		return judgments.RecordAdoptedCommercialResolution(txCtx, tenant, requestID, revalidated)
	})

	recorded, err := judgments.LoadRecordedJudgments(ctx, tenant, requestID)
	if err != nil {
		t.Fatalf("读回已采用判断：%v", err)
	}
	if recorded.AdoptedCommercialResolution != revalidated {
		t.Fatalf("读回 %q，want RES-2——重解后仍拿着已被取代的那次依据",
			recorded.AdoptedCommercialResolution.String())
	}
	if rows := countTaskRows(t, pool,
		`SELECT count(*) FROM parcel_shipment.acceptance_adopted_resolution
		  WHERE tenant_id = $1 AND shipment_request_id = $2`,
		"tenant-1", "REQ-1"); rows != 1 {
		t.Fatalf("库里 %d 行采用解析，want 1", rows)
	}
}

// TestProcessingAttemptsAccumulateAcrossRounds 证卡了两轮就留两条，同一轮重放不留第二条：
// 只留最近一条就说不出这份委托卡过几轮、各卡在哪里。
func TestProcessingAttemptsAccumulateAcrossRounds(t *testing.T) {
	judgments, transactor, pool := newAcceptanceJudgments(t)
	ctx := t.Context()
	tenant, requestID := psTenant(t, "tenant-1"), taskRequestID(t, "REQ-1")

	stalled := taskAttempt(t, "REACHABILITY_AUTHORITY_UNAVAILABLE",
		domain.ResumeByInternalRetry, "CONT-a1b2", taskAttemptedAt)
	waitingOnCustomer := taskAttempt(t, "CUSTOMER_SUPPLEMENT_PENDING",
		domain.ResumeByCustomerSupplement, "CONT-c3d4", taskAttemptedAt.Add(time.Hour))
	mustWithinTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		if err := judgments.RecordProcessingAttempt(txCtx, tenant, requestID, stalled); err != nil {
			return err
		}
		if err := judgments.RecordProcessingAttempt(txCtx, tenant, requestID, stalled); err != nil {
			return err
		}
		return judgments.RecordProcessingAttempt(txCtx, tenant, requestID, waitingOnCustomer)
	})

	if rows := countTaskRows(t, pool,
		`SELECT count(*) FROM parcel_shipment.acceptance_processing_attempt
		  WHERE tenant_id = $1 AND shipment_request_id = $2`,
		"tenant-1", "REQ-1"); rows != 2 {
		t.Fatalf("库里 %d 条处理尝试，want 2", rows)
	}
	if rows := countTaskRows(t, pool,
		`SELECT count(*) FROM parcel_shipment.acceptance_processing_attempt
		  WHERE tenant_id = $1 AND shipment_request_id = $2
		    AND resume_path = 'CUSTOMER_SUPPLEMENT'`,
		"tenant-1", "REQ-1"); rows != 1 {
		t.Fatalf("等待客户补充那一条没有按自己的续办路径落库（实得 %d 条）", rows)
	}
}

// TestAnotherTenantReadsNoneOfTheseJudgments 证跨租户读不到。
//
// 委托标识在租户内唯一，两个租户各有一份同号委托是隔离的正常形态；读口漏掉租户这一维，
// 它们就会读到彼此的判断，而 ADR-0003 把租户定为最高数据隔离边界。
func TestAnotherTenantReadsNoneOfTheseJudgments(t *testing.T) {
	judgments, transactor, _ := newAcceptanceJudgments(t)
	ctx := t.Context()
	owner, requestID := psTenant(t, "tenant-1"), taskRequestID(t, "REQ-1")

	judgment := reachabilityJudgment(t, "parcel-1", domain.ReachabilityReachable, "NRJ-1", "", taskAsOfFirst)
	control := financialControlResult(t, domain.FinancialControlHeld, "SAC-1", "", taskAsOfFirst)
	resolution := mustBuild(t, domain.NewCommercialResolutionID, "RES-1")
	mustWithinTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		if err := judgments.RecordReachabilityJudgment(txCtx, owner, requestID, judgment); err != nil {
			return err
		}
		if err := judgments.RecordFinancialControlResult(txCtx, owner, requestID, control); err != nil {
			return err
		}
		return judgments.RecordAdoptedCommercialResolution(txCtx, owner, requestID, resolution)
	})

	// 同一个委托号，另一个租户：租户漏传时这一支会整份读到本租户的判断。
	elsewhere, err := judgments.LoadRecordedJudgments(ctx, psTenant(t, "tenant-b"), requestID)
	if err != nil {
		t.Fatalf("他租户查询出错：%v", err)
	}
	if len(elsewhere.Reachability) != 0 {
		t.Errorf("他租户读到了本租户的 %d 项可达性判断", len(elsewhere.Reachability))
	}
	if elsewhere.FinancialControl.Outcome() != domain.FinancialControlOutcomeInvalid {
		t.Error("他租户读到了本租户的财务控制结果")
	}
	if elsewhere.AdoptedCommercialResolution.String() != "" {
		t.Error("他租户读到了本租户所采用的商业解析")
	}
}

// TestAcceptanceJudgmentShapesArePinnedInTheDatabase 证形状矩阵的库面。四支裸写探针绕过
// 领域构造门直接写，全部走 NULL 缝（f822e1d 入册的三值逻辑纪律）。
func TestAcceptanceJudgmentShapesArePinnedInTheDatabase(t *testing.T) {
	_, _, pool := newAcceptanceJudgments(t)
	ctx := t.Context()

	if _, err := pool.Exec(ctx,
		`INSERT INTO parcel_shipment.acceptance_reachability_judgment
			(tenant_id, shipment_request_id, parcel_id, as_of_at,
			 judgment_id, judgment_value, basis_ref,
			 as_of_kind, as_of_semantics, as_of_policy)
		 VALUES ('tenant-1', 'REQ-x', 'parcel-x', now(),
		         NULL, 'NOT_APPLICABLE', NULL,
		         'REACHABILITY', 'sem-1', 'policy-1')`); err == nil {
		t.Error("一次没有依据的`不适用`按 NULL 溜进了判断库——它与一次悄悄放行分不开")
	}

	if _, err := pool.Exec(ctx,
		`INSERT INTO parcel_shipment.acceptance_reachability_judgment
			(tenant_id, shipment_request_id, parcel_id, as_of_at,
			 judgment_id, judgment_value, basis_ref,
			 as_of_kind, as_of_semantics, as_of_policy)
		 VALUES ('tenant-1', 'REQ-x', 'parcel-y', now(),
		         NULL, 'REACHABLE', NULL,
		         'REACHABILITY', 'sem-1', 'policy-1')`); err == nil {
		t.Error("一份没有权威标识的`可达`按 NULL 溜进了判断库")
	}

	if _, err := pool.Exec(ctx,
		`INSERT INTO parcel_shipment.acceptance_financial_control
			(tenant_id, shipment_request_id, as_of_at,
			 result_id, control_outcome, basis_ref,
			 as_of_kind, as_of_semantics, as_of_policy)
		 VALUES ('tenant-1', 'REQ-x', now(),
		         'SAC-x', 'NOT_APPLICABLE', NULL,
		         'FINANCIAL_CONTROL', 'sem-1', 'policy-1')`); err == nil {
		t.Error("一次没有依据的`明确无控制`溜进了控制库——它与「默认信用通过」分不开")
	}

	if _, err := pool.Exec(ctx,
		`INSERT INTO parcel_shipment.acceptance_processing_attempt
			(tenant_id, shipment_request_id, continuation_ref, attempted_at,
			 reason_ref, resume_path)
		 VALUES ('tenant-1', 'REQ-x', 'CONT-x', now(), 'SOME_REASON', 'ASK_SOMEONE')`); err == nil {
		t.Error("一条封闭集合以外的续办路径溜进了处理尝试库——催办会催错人")
	}
}

// TestEveryResumePathLandsInTheAttemptTable 证库面镜像与领域封闭集合**逐格**对得上：ResumePath
// 的每一个取值都写得进处理尝试库。上一条只钉「集合外被拒」，钉不住「集合内也被拒」——第四格
// 加进领域而 0005 的 CHECK 没跟时，全仓照样绿，真进程上却把一次如实的未决落成
// dispatch.publish_failed（票 first-tenant-runway/07 在真库上量到过；迁移 0011 对齐后本条才绿）。
//
// 集合以 String() 非空为界，与领域那条遍历门禁同一口径；不写死条数——日后再加一格，这里自动
// 跟着走，而库面镜像没跟时它就红。单独钉一句 OPERATOR_REGISTRATION 在遍历里，是防遍历口径
// 变了之后本条空转成绿。
func TestEveryResumePathLandsInTheAttemptTable(t *testing.T) {
	judgments, transactor, _ := newAcceptanceJudgments(t)
	ctx := t.Context()
	tenant, requestID := psTenant(t, "tenant-1"), taskRequestID(t, "REQ-1")

	sawOperatorRegistration := false
	for path := domain.ResumePath(1); path.String() != ""; path++ {
		attempt := taskAttempt(t, "SOME_REASON", path, "CONT-"+path.String(), taskAttemptedAt)
		if err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
			return judgments.RecordProcessingAttempt(txCtx, tenant, requestID, attempt)
		}); err != nil {
			t.Errorf("续办路径 %s 写不进处理尝试库——库面镜像落后于领域封闭集合：%v", path, err)
		}
		sawOperatorRegistration = sawOperatorRegistration || path == domain.ResumeByOperatorRegistration
	}
	if !sawOperatorRegistration {
		t.Fatal("遍历没走到 OPERATOR_REGISTRATION——String() 的遍历口径变了，本条要跟着改")
	}
}

// TestTaskWaitingOnProjectionMirrorsEveryResumePath 证等待态投影列 task_waiting_on 的 CHECK
// 与领域封闭集合逐格对得上、上界外仍被拒（0009 立的约束，0011 对齐到第四格）。
//
// 用裸写而不走 Save：领域今天没有一条路径把等待态写成`等待运营登记`——Decide 把未决一律折成
// 内部续办或客户补充，第四格的等待态要等 ADR-0094 Decision 五那一片。所以这里钉的只是库面镜像：
// 那一片落地那天 Save 写下 4，不该在这条约束上撞死；而它撞死的样子与快照一起整份落不了库，
// 因为投影列与快照是同一条 SQL。
func TestTaskWaitingOnProjectionMirrorsEveryResumePath(t *testing.T) {
	repository, transactor, pool := newShipmentRequests(t)
	ctx := t.Context()
	mustInsert(t, transactor, ctx, repository, submittedShipmentRequest(t, "arq-w", "REQ-WAIT-1"))

	last := domain.ResumePathInvalid
	for path := domain.ResumePath(1); path.String() != ""; path++ {
		if _, err := pool.Exec(ctx,
			`UPDATE parcel_shipment.shipment_request SET task_waiting_on = $1
			  WHERE shipment_request_id = 'REQ-WAIT-1'`, uint8(path)); err != nil {
			t.Errorf("等待态 %s 写不进投影列——库面镜像落后于领域封闭集合：%v", path, err)
		}
		last = path
	}
	if last != domain.ResumeByOperatorRegistration {
		t.Fatalf("遍历止于 %s，want OPERATOR_REGISTRATION——String() 的遍历口径变了，本条要跟着改", last)
	}
	if _, err := pool.Exec(ctx,
		`UPDATE parcel_shipment.shipment_request SET task_waiting_on = $1
		  WHERE shipment_request_id = 'REQ-WAIT-1'`, uint8(last)+1); err == nil {
		t.Error("一个领域集合外的等待态溜进了投影列——队列过滤会按一个不存在的续办方列出委托")
	}
}

func TestAcceptanceJudgmentWritesRefuseToRunOutsideATransaction(t *testing.T) {
	judgments, _, _ := newAcceptanceJudgments(t)
	ctx := t.Context()
	tenant, requestID := psTenant(t, "tenant-1"), taskRequestID(t, "REQ-1")

	if err := judgments.RecordReachabilityJudgment(ctx, tenant, requestID,
		reachabilityJudgment(t, "parcel-1", domain.ReachabilityReachable, "NRJ-1", "", taskAsOfFirst),
	); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务记录可达性判断应返回 ErrTransactionRequired，实得：%v", err)
	}
	if err := judgments.RecordFinancialControlResult(ctx, tenant, requestID,
		financialControlResult(t, domain.FinancialControlHeld, "SAC-1", "", taskAsOfFirst),
	); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务记录控制结果应返回 ErrTransactionRequired，实得：%v", err)
	}
	if err := judgments.RecordProcessingAttempt(ctx, tenant, requestID,
		taskAttempt(t, "JUDGMENT_NOT_RECORDED", domain.ResumeByInternalRetry, "CONT-a1b2", taskAttemptedAt),
	); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务记录处理尝试应返回 ErrTransactionRequired，实得：%v", err)
	}
	if err := judgments.RecordAdoptedCommercialResolution(ctx, tenant, requestID,
		mustBuild(t, domain.NewCommercialResolutionID, "RES-1"),
	); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务记录所采用解析应返回 ErrTransactionRequired，实得：%v", err)
	}
}

func TestAcceptanceJudgmentRollbackLeavesNothingBehind(t *testing.T) {
	judgments, transactor, _ := newAcceptanceJudgments(t)
	ctx := t.Context()
	tenant, requestID := psTenant(t, "tenant-1"), taskRequestID(t, "REQ-1")
	rollback := errors.New("回滚")

	judgment := reachabilityJudgment(t, "parcel-1", domain.ReachabilityReachable, "NRJ-1", "", taskAsOfFirst)
	control := financialControlResult(t, domain.FinancialControlHeld, "SAC-1", "", taskAsOfFirst)
	resolution := mustBuild(t, domain.NewCommercialResolutionID, "RES-1")
	attempt := taskAttempt(t, "JUDGMENT_NOT_RECORDED", domain.ResumeByInternalRetry, "CONT-a1b2", taskAttemptedAt)
	if err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		if err := judgments.RecordReachabilityJudgment(txCtx, tenant, requestID, judgment); err != nil {
			return err
		}
		if err := judgments.RecordFinancialControlResult(txCtx, tenant, requestID, control); err != nil {
			return err
		}
		if err := judgments.RecordAdoptedCommercialResolution(txCtx, tenant, requestID, resolution); err != nil {
			return err
		}
		if err := judgments.RecordProcessingAttempt(txCtx, tenant, requestID, attempt); err != nil {
			return err
		}
		return rollback
	}); !errors.Is(err, rollback) {
		t.Fatalf("事务应以回滚错误结束，实得：%v", err)
	}

	recorded, err := judgments.LoadRecordedJudgments(ctx, tenant, requestID)
	if err != nil {
		t.Fatalf("读回已采用判断：%v", err)
	}
	if len(recorded.Reachability) != 0 ||
		recorded.FinancialControl.Outcome() != domain.FinancialControlOutcomeInvalid ||
		recorded.AdoptedCommercialResolution.String() != "" {
		t.Fatalf("回滚后判断仍在：%+v", recorded)
	}
}

// ---- 夹具 ----

func newAcceptanceJudgments(t *testing.T) (*adapter.AcceptanceJudgments, bentoapp.Transactor, *pgxpool.Pool) {
	t.Helper()

	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	judgments, err := adapter.NewAcceptanceJudgments(db)
	if err != nil {
		t.Fatalf("构造判断库：%v", err)
	}
	return judgments, db.Transactor(), pool
}

func taskRequestID(t *testing.T, raw string) domain.ShipmentRequestID {
	t.Helper()
	return mustBuild(t, domain.NewShipmentRequestID, raw)
}

// taskJudgmentAsOf 造一份已由 party-commercial 校验回显的时点。回显政策必须另行构造——把
// 第一阶段的声明直接塞进 NewJudgmentAsOf 已经编译不过。
func taskJudgmentAsOf(t *testing.T, kind domain.JudgmentKind, at time.Time) domain.JudgmentAsOf {
	t.Helper()
	echoed, err := domain.NewEchoedAsOfPolicy(
		kind,
		mustBuild(t, domain.NewAsOfSemanticsReference, "ASOF-SEMANTICS-"+kind.String()),
		mustBuild(t, domain.NewAsOfPolicyVersion, "asof-policy-v1"),
	)
	if err != nil {
		t.Fatalf("回显时点政策：%v", err)
	}
	asOf, err := domain.NewJudgmentAsOf(at, echoed)
	if err != nil {
		t.Fatalf("判断时点：%v", err)
	}
	return asOf
}

// reachabilityJudgment 造一份可达性判断。judgmentID 与 basis 传空串即缺席，这样`不适用`
// 那一支（无标识、有依据）与其余三值（有标识）能在同一个夹具里表达。
func reachabilityJudgment(
	t *testing.T,
	parcel string,
	value domain.ReachabilityValue,
	judgmentID, basis string,
	at time.Time,
) domain.ReachabilityJudgment {
	t.Helper()
	spec := domain.ReachabilityJudgmentSpec{
		ParcelID: mustBuild(t, domain.NewDeclaredParcelID, parcel),
		Value:    value,
		AsOf:     taskJudgmentAsOf(t, domain.ReachabilityJudgmentKind, at),
	}
	if judgmentID != "" {
		spec.JudgmentID = mustBuild(t, domain.NewReachabilityJudgmentID, judgmentID)
	}
	if basis != "" {
		spec.Basis = mustBuild(t, domain.NewReachabilityBasisReference, basis)
	}
	judgment, err := domain.NewReachabilityJudgment(spec)
	if err != nil {
		t.Fatalf("形成可达性判断：%v", err)
	}
	return judgment
}

func financialControlResult(
	t *testing.T,
	outcome domain.FinancialControlOutcome,
	resultID, basis string,
	at time.Time,
) domain.FinancialControlResult {
	t.Helper()
	var identifier domain.FinancialControlResultID
	if resultID != "" {
		identifier = mustBuild(t, domain.NewFinancialControlResultID, resultID)
	}
	var reference domain.ControlBasisReference
	if basis != "" {
		reference = mustBuild(t, domain.NewControlBasisReference, basis)
	}
	result, err := domain.NewFinancialControlResult(identifier, outcome, reference,
		taskJudgmentAsOf(t, domain.FinancialControlJudgmentKind, at))
	if err != nil {
		t.Fatalf("形成财务控制结果：%v", err)
	}
	return result
}

func taskAttempt(
	t *testing.T,
	reason string,
	resumePath domain.ResumePath,
	continuation string,
	at time.Time,
) domain.ProcessingAttempt {
	t.Helper()
	attempt, err := domain.NewProcessingAttempt(domain.ProcessingAttemptSpec{
		Reason:       mustBuild(t, domain.NewProcessingAttemptReason, reason),
		ResumePath:   resumePath,
		Continuation: mustBuild(t, domain.NewOwnershipContinuationReference, continuation),
		AttemptedAt:  at,
	})
	if err != nil {
		t.Fatalf("形成处理尝试：%v", err)
	}
	return attempt
}

func countTaskRows(t *testing.T, pool *pgxpool.Pool, query, tenant, requestID string) int {
	t.Helper()

	var count int
	if err := pool.QueryRow(t.Context(), query, tenant, requestID).Scan(&count); err != nil {
		t.Fatalf("计数出错：%v", err)
	}
	return count
}
