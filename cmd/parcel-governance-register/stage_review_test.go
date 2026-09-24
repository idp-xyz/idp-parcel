package main

import (
	"strings"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	pgadapter "go.idp.xyz/idp-parcel/internal/pilotgovernance/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/pilotgovernance/application"
	"go.idp.xyz/idp-parcel/internal/pilotgovernance/domain"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件证 stage-review 子命令（票 demo-intake-admission-paused/01）：翻译严格零默认、评审的范围与
// 候选组引用取自同一份候选组；两段答案译成退出码；真库上登了「互不相干」之后别处的暂停不再拦
// 所问范围版本，评审没落成时候选组随整笔事务撤回。

func stageReviewJSON(setID, parameters, objective, verdict, disposition, coverage string) []byte {
	dispositionField := ""
	if disposition != "" {
		dispositionField = `"disposition": "` + disposition + `",`
	}
	return []byte(`{
		"candidateSet": {
			"setId": "` + setID + `",
			"scope": "pilot-scope/intake@v1",
			"parameters": "` + parameters + `",
			"rules": "rules/intake-v1",
			"formedAt": "2026-08-20T00:00:00Z"
		},
		"review": {
			"stage": "SHADOW_RUN",
			"objective": "` + objective + `",
			"evidencePack": "evidence-pack/shadow-run-intake",
			"verdict": "` + verdict + `",
			` + dispositionField + `
			"decidedBy": "pilot-business-owner",
			"decidedAt": "2026-08-22T00:00:00Z",
			"effectiveAt": "2026-08-23T00:00:00Z"
		},
		"coverage": ` + coverage + `
	}`)
}

const unrelatedToPricing = `[{"predecessor": "pilot-scope/pricing@v1", "kind": "UNRELATED"}]`

// 评审的范围版本与候选组引用取自候选组本身；覆盖关系声明逐条译成领域声明；没给授予区间就是
// 没有，不替它造一条。
func TestStageReviewTranslatesTheFixedSetTheDecisionAndTheCoverage(t *testing.T) {
	input, err := stageReviewFromJSON(stageReviewJSON("candidate-set-1", "params/intake-v1", "objective/intake-lp", "GO", "",
		`[{"predecessor": "pilot-scope/pricing@v1", "kind": "UNRELATED"},
		  {"predecessor": "pilot-scope/intake@v0", "kind": "INHERITS_SUSPENSIONS"}]`))
	if err != nil {
		t.Fatalf("翻译：%v", err)
	}
	set := input.candidateSet
	if set.ID().String() != "candidate-set-1" || set.Scope().String() != "pilot-scope/intake@v1" ||
		set.Parameters().String() != "params/intake-v1" || set.Rules().String() != "rules/intake-v1" ||
		!set.FormedAt().Equal(time.Date(2026, 8, 20, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("候选组 = %+v", set)
	}
	review := input.command.Review
	if review.Scope != set.Scope() || review.Candidates != set.ID() {
		t.Fatalf("评审范围 / 候选组 = %s / %s，要取自候选组", review.Scope, review.Candidates)
	}
	if review.Stage != domain.ShadowRun || review.Verdict != domain.StageGo ||
		review.Disposition != domain.NoGoDispositionInvalid || review.Objective != "objective/intake-lp" ||
		review.DecidedBy != "pilot-business-owner" || len(review.Deviations) != 0 {
		t.Fatalf("评审 = %+v", review)
	}
	if input.command.GrantedInterval != nil {
		t.Fatalf("没给授予区间却译出一条：%+v", *input.command.GrantedInterval)
	}
	coverage := input.command.Coverage
	if len(coverage) != 2 ||
		coverage[0].Predecessor.String() != "pilot-scope/pricing@v1" || coverage[0].Kind != domain.ScopeUnrelated ||
		coverage[1].Predecessor.String() != "pilot-scope/intake@v0" || coverage[1].Kind != domain.ScopeInheritsSuspensions {
		t.Fatalf("覆盖关系 = %+v", coverage)
	}
}

// 集合外的阶段、结论、处置与关系种类一律拒，不猜近似；未知字段与候选组缺件在触库前拒。
func TestStageReviewTranslationRefusesOutsideWordsAndMissingParts(t *testing.T) {
	valid := string(stageReviewJSON("candidate-set-1", "params/intake-v1", "objective/intake-lp", "GO", "", unrelatedToPricing))
	cases := map[string]string{
		"集合外阶段":    strings.Replace(valid, `"SHADOW_RUN"`, `"SHADOW"`, 1),
		"集合外结论":    strings.Replace(valid, `"verdict": "GO"`, `"verdict": "PASS"`, 1),
		"集合外处置":    string(stageReviewJSON("candidate-set-1", "params/intake-v1", "objective/intake-lp", "NO_GO", "ROLLBACK", unrelatedToPricing)),
		"集合外关系种类":  strings.Replace(valid, `"UNRELATED"`, `"DISJOINT"`, 1),
		"未知字段":     strings.Replace(valid, `"rules": "rules/intake-v1",`, `"rules": "rules/intake-v1", "extra": 1,`, 1),
		"候选组缺形成时点": strings.Replace(valid, `"formedAt": "2026-08-20T00:00:00Z"`, `"formedAt": "0001-01-01T00:00:00Z"`, 1),
		"空前代":      strings.Replace(valid, `"pilot-scope/pricing@v1"`, `""`, 1),
	}
	for name, raw := range cases {
		if _, err := stageReviewFromJSON([]byte(raw)); err == nil {
			t.Fatalf("%s：翻译应拒收", name)
		}
	}
}

func TestStageReviewAnswerCoversEveryOutcome(t *testing.T) {
	landed := []application.FixCandidateSetOutcome{application.CandidateSetFixed, application.CandidateSetAlreadyFixed}
	for _, fixed := range landed {
		for outcome, want := range map[application.StageReviewOutcome]int{
			application.ReviewRecorded:            exitRegistered,
			application.ReviewExistingDecision:    exitRegistered,
			application.AuthorityConflictBlocked:  exitGovernance,
			application.CoverageConflictBlocked:   exitGovernance,
			application.ReviewNotAccepted:         exitUsage,
			application.ReviewUndecided:           exitUndecided,
			application.StageReviewOutcomeInvalid: exitUndecided,
		} {
			if _, code := stageReviewAnswer(fixed, outcome, stageReviewNote{}); code != want {
				t.Fatalf("%s + %s 退出码 = %d，要 %d", fixed, outcome, code, want)
			}
		}
		pending := stageReviewNote{continuations: []string{"CONT-COVERAGE/a/b"}}
		if message, code := stageReviewAnswer(fixed, application.ReviewRecorded, pending); code != exitGovernance ||
			!strings.Contains(message, "CONT-COVERAGE/a/b") {
			t.Fatalf("决定已落而边没登上 = %d（%s），要治理退出码并带续办引用", code, message)
		}
	}
	for fixed, want := range map[application.FixCandidateSetOutcome]int{
		application.CandidateSetContentConflict:   exitGovernance,
		application.CandidateSetNotAccepted:       exitUsage,
		application.CandidateSetUndecided:         exitUndecided,
		application.FixCandidateSetOutcomeInvalid: exitUndecided,
	} {
		if _, code := stageReviewAnswer(fixed, application.ReviewRecorded, stageReviewNote{}); code != want {
			t.Fatalf("候选组 %s 退出码 = %d，要 %d（候选组没进册就没有评审可言）", fixed, code, want)
		}
	}
}

// 真库贯通：别处范围的暂停在未登关系时保守拦所问版本；stage-review 随 Go 登下「互不相干」之后
// 放行，而那条暂停对它自己的范围照旧立着。重放答已有决定、留第二痕；同标识异内容的候选组答
// 治理冲突、不留痕；评审未受理时候选组随整笔事务撤回，不在册。
func TestStageReviewOnRealPostgresUnblocksAdmissionAndRollsBackUnlandedReviews(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	regs, err := buildRegistrars(db)
	if err != nil {
		t.Fatalf("装配登记口：%v", err)
	}
	ctx := t.Context()
	identity := channelIdentity{osUser: "OPSHOST\\operator-a", hostname: "ops-host-01"}
	countTraces := func() int {
		t.Helper()
		querier, err := db.ReadExecutor(ctx)
		if err != nil {
			t.Fatalf("取读执行器：%v", err)
		}
		var count int
		if err := querier.QueryRow(ctx,
			`SELECT count(*) FROM pilot_governance.channel_execution WHERE command = $1`,
			commandStageReview,
		).Scan(&count); err != nil {
			t.Fatalf("数留痕行：%v", err)
		}
		return count
	}

	suspendPricing := []byte(`{
		"suspensionId": "suspension-pricing-1",
		"triggerSource": "PILOT-RULE/series-gap",
		"basis": "fuel series stale",
		"evidence": "evidence-pack/incident-p1",
		"scope": "pilot-scope/pricing@v1",
		"executedBy": "declared-duty-officer",
		"occurredAt": "2026-08-24T01:00:00Z",
		"effectiveAt": "2026-08-24T01:05:00Z",
		"inTransitNote": "in-transit evaluations keep their locked versions"
	}`)
	if message, code := execute(ctx, commandSuspend, suspendPricing, identity, regs); code != exitRegistered {
		t.Fatalf("暂停退出码 = %d（%s）", code, message)
	}
	suspensions, err := pgadapter.NewSuspensions(db)
	if err != nil {
		t.Fatalf("构造暂停库：%v", err)
	}
	intake, err := domain.NewScopeVersionReference("pilot-scope/intake@v1")
	if err != nil {
		t.Fatalf("构造范围版本：%v", err)
	}
	askAt := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	if _, ground, err := suspensions.FindUnresumedSuspension(ctx, intake, askAt); err != nil ||
		ground != domain.AdmissionSuspendedByUnreadableScopeRelation {
		t.Fatalf("登关系前 = (%s, %v)，要保守拦 SUSPENDED_BY_UNREADABLE_SCOPE_RELATION", ground, err)
	}

	review := stageReviewJSON("candidate-set-1", "params/intake-v1", "objective/intake-lp", "GO", "", unrelatedToPricing)
	if message, code := execute(ctx, commandStageReview, review, identity, regs); code != exitRegistered ||
		!strings.Contains(message, "RECORDED") {
		t.Fatalf("阶段评审 = %d（%s）", code, message)
	}
	if _, ground, err := suspensions.FindUnresumedSuspension(ctx, intake, askAt); err != nil ||
		ground != domain.AdmissionNotSuspended {
		t.Fatalf("登关系后 = (%s, %v)，要 NOT_SUSPENDED", ground, err)
	}
	pricing, err := domain.NewScopeVersionReference("pilot-scope/pricing@v1")
	if err != nil {
		t.Fatalf("构造范围版本：%v", err)
	}
	if _, ground, err := suspensions.FindUnresumedSuspension(ctx, pricing, askAt); err != nil ||
		ground != domain.AdmissionSuspendedByNamedScope {
		t.Fatalf("暂停写明的范围 = (%s, %v)，要照旧拦 SUSPENDED_BY_NAMED_SCOPE", ground, err)
	}

	if message, code := execute(ctx, commandStageReview, review, identity, regs); code != exitRegistered ||
		!strings.Contains(message, "EXISTING_DECISION") {
		t.Fatalf("重放 = %d（%s）", code, message)
	}
	if count := countTraces(); count != 2 {
		t.Fatalf("stage-review 留痕 = %d，要 2", count)
	}

	widened := stageReviewJSON("candidate-set-1", "params/intake-v2", "objective/intake-lp-2", "GO", "", unrelatedToPricing)
	if message, code := execute(ctx, commandStageReview, widened, identity, regs); code != exitGovernance ||
		!strings.Contains(message, "CANDIDATE_SET_CONTENT_CONFLICT") {
		t.Fatalf("同标识异内容的候选组 = %d（%s），要治理冲突", code, message)
	}

	unaccepted := stageReviewJSON("candidate-set-nogo", "params/intake-v1", "objective/intake-nogo", "NO_GO", "", unrelatedToPricing)
	if message, code := execute(ctx, commandStageReview, unaccepted, identity, regs); code != exitUsage ||
		!strings.Contains(message, "NOT_ACCEPTED") {
		t.Fatalf("No-Go 缺处理方式 = %d（%s），要未受理", code, message)
	}
	candidateSets, err := pgadapter.NewCandidateSets(db)
	if err != nil {
		t.Fatalf("构造候选组库：%v", err)
	}
	nogo, err := domain.NewCandidateVersionSetID("candidate-set-nogo")
	if err != nil {
		t.Fatalf("构造候选组标识：%v", err)
	}
	if _, found, err := candidateSets.FindByID(ctx, nogo); err != nil || found {
		t.Fatalf("评审未落时候选组在册 = (%v, %v)，要随整笔事务撤回", found, err)
	}
	if count := countTraces(); count != 2 {
		t.Fatalf("没落成的执行留了痕：stage-review 留痕 = %d，要 2", count)
	}
}
