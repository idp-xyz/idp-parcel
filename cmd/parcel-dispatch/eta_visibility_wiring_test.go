package main

import (
	"context"
	"testing"
	"time"

	"go.idp.xyz/idp-bento-go/postgres/inbox"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	veidentity "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/identity"
	veinbox "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/inbox"
	vepostgres "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/postgres"
	veapplication "go.idp.xyz/idp-parcel/internal/visibilityexception/application"
	vedomain "go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
)

// 本文件证 ETA 客户可见性判断的生产链：`visibility-exception.eta-prediction.formed` 被本进程接住，一拍里完成
// 判断落库与客户视图重派生（UC-VE-003 `AT-VE-187`、UC-VE-008 `AT-VE-158`）。账户维经 PS 按包裹反查；规则册与披露
// 策略都是租户取值，这里直接写行，证的是判断与派生读得对。

const (
	etaVisibilityConsumerName = "visibility-exception/judge-eta-visibility-from-prediction"
)

// registerETAShownPolicy 给夹具租户的 SYN-CUSTOMER-01 登一版披露策略：ETA 准许展示，其余三维待确认。
func registerETAShownPolicy(t *testing.T, fixture *synVerticalFixture) {
	t.Helper()
	tenant := fixture.identity.TenantID().String()
	mustWithinTX(t, fixture.transactor, t.Context(), func(txCtx context.Context) error {
		executor, err := fixture.db.RequireExecutor(txCtx)
		if err != nil {
			return err
		}
		if _, err := executor.Exec(txCtx,
			`INSERT INTO visibility_exception.disclosure_policy_version
				(tenant_id, policy_version, effective_from, effective_to, approved_by)
			 VALUES ($1, 'SYN-VE-DISCLOSE-ETA', $2, NULL, 'SYN-OPS-TRACKING-LEAD')`,
			tenant, time.Now().UTC().Add(-24*time.Hour)); err != nil {
			return err
		}
		_, err = executor.Exec(txCtx,
			`INSERT INTO visibility_exception.disclosure_policy_entry
				(tenant_id, policy_version, customer_account_ref,
				 milestones_state, eta_state, final_state, note_state,
				 milestones_content, eta_content, final_content, note_content)
			 VALUES ($1, 'SYN-VE-DISCLOSE-ETA', 'SYN-CUSTOMER-01',
			         'PENDING_CONFIRMATION', 'SHOWN', 'PENDING_CONFIRMATION', 'PENDING_CONFIRMATION',
			         NULL, 'SYN-VE-CONTENT-ETA-V1', NULL, NULL)`,
			tenant)
		return err
	})
}

// registerETARule 给 SYN-CUSTOMER-01 登一条 ETA 对客规则：DELIVERED 的 ETA 是客户要的那一个，可信度依据在 qualifying 里即达标。
func registerETARule(t *testing.T, fixture *synVerticalFixture, qualifying string) {
	t.Helper()
	registerETARuleQualifying(t, fixture, qualifying)
}

// registerETARuleQualifying 同 registerETARule，但把若干可信度依据都登为达标。
func registerETARuleQualifying(t *testing.T, fixture *synVerticalFixture, qualifying ...string) {
	t.Helper()
	tenant := fixture.identity.TenantID().String()
	mustWithinTX(t, fixture.transactor, t.Context(), func(txCtx context.Context) error {
		executor, err := fixture.db.RequireExecutor(txCtx)
		if err != nil {
			return err
		}
		_, err = executor.Exec(txCtx,
			`INSERT INTO visibility_exception.eta_visibility_rule
				(tenant_id, customer_account_ref, service_rule_version, gate_version, target_milestone_ref,
				 qualifying_confidences, expression, stop_boundary_ref, effective_from, effective_to, approved_by)
			 VALUES ($1, 'SYN-CUSTOMER-01', 'SYN-COM-07-V1', 'SYN-VIS-03-V1', 'DELIVERED',
			         $2::text[], 'TIME_RANGE', 'STOP-AFTER-DELIVERED', $3, NULL, 'SYN-CS-LEAD')`,
			tenant, qualifying, time.Now().UTC().Add(-24*time.Hour))
		return err
	})
}

// formETA 经真实的 FormETAHandler 形成一份运营企业派生的内部 ETA：预测落库与预测版本信封同一事务入队。
func formETA(t *testing.T, fixture *synVerticalFixture, inputs, confidence string) vedomain.ETAVersionID {
	t.Helper()
	return formETAFrom(t, fixture, vedomain.OperatorDerivedETA, inputs, confidence)
}

// formETAFrom 同 formETA，来源由调用方给：承运商提供的与运营企业派生的预测各是一条预测线。
func formETAFrom(t *testing.T, fixture *synVerticalFixture, source vedomain.ETASourceKind, inputs, confidence string) vedomain.ETAVersionID {
	t.Helper()
	outboxStore, err := outbox.NewStore(fixture.db)
	if err != nil {
		t.Fatalf("构造 Outbox Store：%v", err)
	}
	etas, err := vepostgres.NewETAs(fixture.db)
	if err != nil {
		t.Fatalf("构造 ETA 库：%v", err)
	}
	gaps, err := vepostgres.NewVisibilityGaps(fixture.db)
	if err != nil {
		t.Fatalf("构造缺口库：%v", err)
	}
	identities, err := veidentity.NewETAVersions()
	if err != nil {
		t.Fatalf("构造 ETA 身份：%v", err)
	}
	etaHandoff, err := vepostgres.NewOutboxETAHandoff(fixture.db, outboxStore, systemClock{})
	if err != nil {
		t.Fatalf("构造 ETA 交接：%v", err)
	}
	gapHandoff, err := vepostgres.NewOutboxVisibilityGapHandoff(fixture.db, outboxStore, systemClock{})
	if err != nil {
		t.Fatalf("构造缺口交接：%v", err)
	}
	handler := veapplication.NewFormETAHandler(veapplication.FormETADeps{
		Predictions: etas,
		Gaps:        gaps,
		Identities:  identities,
		ViewChain:   etaHandoff,
		SignalChain: gapHandoff,
		Clock:       systemClock{},
	})
	now := time.Now().UTC()
	var formed vedomain.ETAPrediction
	mustWithinTX(t, fixture.transactor, t.Context(), func(txCtx context.Context) error {
		result, err := handler.FormETA(txCtx, veapplication.FormETACommand{
			TenantID:   mustVE(t, vedomain.NewTenantID, fixture.identity.TenantID().String()),
			Parcel:     mustVE(t, vedomain.NewTrackedParcelReference, transportHandoverObject),
			Milestone:  mustVE(t, vedomain.NewMilestoneReference, "DELIVERED"),
			Source:     source,
			Inputs:     mustVE(t, vedomain.NewPredictionInputsReference, inputs),
			Model:      mustVE(t, vedomain.NewPredictionModelReference, "SYN-ETA-MODEL-V1"),
			RangeFrom:  now.Add(30 * time.Hour),
			RangeTo:    now.Add(42 * time.Hour),
			Confidence: mustVE(t, vedomain.NewConfidenceReference, confidence),
		})
		if err != nil {
			return err
		}
		eta, present := result.ETA()
		if !present || result.HandoffReference() != "" {
			t.Errorf("ETA 未形成或意图未交出：outcome = %q 续办 = %q", result.Outcome(), result.HandoffReference())
		}
		formed = eta
		return nil
	})
	return formed.Version()
}

func currentETAJudgement(t *testing.T, fixture *synVerticalFixture) vedomain.ETAVisibilityJudgement {
	t.Helper()
	judgements, err := vepostgres.NewETAVisibilityJudgements(fixture.db)
	if err != nil {
		t.Fatalf("构造判断读口：%v", err)
	}
	judgement, found, err := judgements.FindCurrent(t.Context(),
		mustVE(t, vedomain.NewTenantID, fixture.identity.TenantID().String()),
		mustVE(t, vedomain.NewCustomerAccountReference, "SYN-CUSTOMER-01"),
		mustVE(t, vedomain.NewTrackedParcelReference, transportHandoverObject))
	if err != nil || !found {
		t.Fatalf("读当前判断：err=%v found=%v", err, found)
	}
	return judgement
}

// Covers: AT-VE-187、AT-VE-158、AT-VE-213、AT-VE-215 与 CONTEXT「ETA 新版本……必须重新派生客户视图并形成追加更正或明确替代关系」的生产链——
// 租户把 ETA 登成展示、登记了对客规则：第一份内部 ETA 达标，一拍之内形成客户可见 ETA 判断，客户视图 ETA 一维由待确认转展示、
// 内容来处是这一版判断；第二份 ETA 的可信度依据不再达标，一拍之内判断换版为 ETA 不可见（原判断留存、指回），客户视图再
// 换代，ETA 一维不展示，前一版指回。投影版本全程不变。同一份预测信封再投一次（新信封 ID）：判断与视图都不再长版本。
func TestAnETAThatMeetsThenMissesTheGateMovesTheCustomerViewThroughShownToWithheld(t *testing.T) {
	fixture := newAcceptedCustomerViewFixture(t)
	registerETAShownPolicy(t, fixture)
	registerETARule(t, fixture, "MEDIUM/history-based")

	// 视图先在没有判断时形成：策略准许展示而无判断，ETA 一维答待确认。
	base := deriveViewFromHandover(t, fixture, transportHandoverScope, transportHandoverVersion)
	if state := base.view.Dimensions().ETA.State(); state != vedomain.DimensionPendingConfirmation {
		t.Fatalf("无判断时 ETA 一维 = %v, want 待确认", state)
	}

	firstETA := formETA(t, fixture, "SYN-ETA-INPUTS-1", "MEDIUM/history-based")
	beat(t, fixture, "第一份 ETA")
	judgement := currentETAJudgement(t, fixture)
	if judgement.Conclusion() != vedomain.CustomerVisibleETA || judgement.CandidateVersion() != firstETA {
		t.Fatalf("判断 = %q 候选 = %q，want 客户可见且引用 %q", judgement.Conclusion(), judgement.CandidateVersion(), firstETA)
	}
	shownView := currentCustomerView(t, fixture)
	content, shown := shownView.Dimensions().ETA.Content()
	if !shown || content.String() != judgement.Version().String() {
		t.Fatalf("ETA 一维 = %v 内容 %q，want 展示且来处为判断 %q", shownView.Dimensions().ETA.State(), content, judgement.Version())
	}
	if prior, superseding := shownView.PriorVersion(); !superseding || prior != base.view.Version() {
		t.Fatalf("前版 = %q，want 指回 %q", prior, base.view.Version())
	}
	if shownView.BasedOn() != base.projection.Version() {
		t.Fatalf("判断变化触发的重派生换了投影锚：%q vs %q", shownView.BasedOn(), base.projection.Version())
	}

	// 同一份预测信封换个 ID 再投：判断按同一候选重放，视图四维没变，都不长版本。
	redeliverETAFormed(t, fixture, firstETA)
	if n := fixture.countSQL(t, `SELECT count(*) FROM visibility_exception.eta_visibility_judgement_version`); n != 1 {
		t.Fatalf("重投后判断版本 = %d, want 仍为 1", n)
	}
	if n := fixture.countSQL(t, `SELECT count(*) FROM visibility_exception.customer_view_version`); n != 2 {
		t.Fatalf("重投后视图版本 = %d, want 仍为 2", n)
	}

	secondETA := formETA(t, fixture, "SYN-ETA-INPUTS-2", "LOW/sparse")
	beat(t, fixture, "第二份 ETA")
	missed := currentETAJudgement(t, fixture)
	if missed.Conclusion() != vedomain.ETANotVisible || missed.CandidateVersion() != secondETA {
		t.Fatalf("判断 = %q 候选 = %q，want ETA 不可见且引用 %q", missed.Conclusion(), missed.CandidateVersion(), secondETA)
	}
	if prior, superseding := missed.PriorVersion(); !superseding || prior != judgement.Version() {
		t.Fatalf("判断前版 = %q，want 指回 %q", prior, judgement.Version())
	}
	withheld := currentCustomerView(t, fixture)
	if withheld.Dimensions().ETA.State() != vedomain.DimensionNotDisclosed {
		t.Fatalf("ETA 一维 = %v, want 不展示", withheld.Dimensions().ETA.State())
	}
	if prior, superseding := withheld.PriorVersion(); !superseding || prior != shownView.Version() {
		t.Fatalf("视图前版 = %q，want 指回 %q", prior, shownView.Version())
	}
	if n := fixture.countSQL(t, `SELECT count(*) FROM visibility_exception.eta_visibility_judgement_version`); n != 2 {
		t.Fatalf("判断版本 = %d, want 2（原判断留存）", n)
	}
	if n := fixture.countSQL(t, `SELECT count(*) FROM visibility_exception.customer_view_version`); n != 3 {
		t.Fatalf("视图版本 = %d, want 3（原发布留存）", n)
	}
	if n := fixture.countInbox(t, etaVisibilityConsumerName, string(firstETA.String())); n != 1 {
		t.Fatalf("判断消费账 = %d, want 1", n)
	}
}

// Covers: AT-VE-051、AT-VE-222、AT-VE-224、AT-VE-225、AT-VE-213、AT-VE-215 与 CONTEXT「承运商提供的预测与运营企业派生的预测分别保留来源和口径。新的 ETA 形成新版本，
// 不覆盖历史预测」的生产链——承运商与运营企业两路预测并存：只有承运商一路时它是候选；运营企业一路到了候选换手，判断替代指回、视图
// 换代；承运商再出新版本（哪怕质量已不达标）候选没变，判断与视图都不长版本；运营企业出新版本才再换代。四个预测版本、三版判断
// 都留在库里，判断的候选引用（包裹、里程碑、来源、版本）在预测库里按版本读得回，各线当前版是各自最新的一版。
func TestTwoETASourcesKeepTheirOwnLinesAndTheJudgementFollowsTheOperatorDerivedOne(t *testing.T) {
	fixture := newAcceptedCustomerViewFixture(t)
	registerETAShownPolicy(t, fixture)
	registerETARuleQualifying(t, fixture, "MEDIUM/history-based", "HIGH/carrier-confirmed")
	deriveViewFromHandover(t, fixture, transportHandoverScope, transportHandoverVersion)

	carrierV1 := formETAFrom(t, fixture, vedomain.CarrierProvidedETA, "SYN-CARRIER-FEED-1", "HIGH/carrier-confirmed")
	beat(t, fixture, "承运商第一份")
	first := currentETAJudgement(t, fixture)
	if first.Conclusion() != vedomain.CustomerVisibleETA || first.CandidateVersion() != carrierV1 ||
		first.CandidateSource() != vedomain.CarrierProvidedETA {
		t.Fatalf("判断 = %q 候选 = %q（%v），want 客户可见且候选是承运商 %q", first.Conclusion(), first.CandidateVersion(),
			first.CandidateSource(), carrierV1)
	}

	operatorV1 := formETAFrom(t, fixture, vedomain.OperatorDerivedETA, "SYN-ETA-INPUTS-1", "MEDIUM/history-based")
	beat(t, fixture, "运营企业第一份")
	second := currentETAJudgement(t, fixture)
	if second.CandidateVersion() != operatorV1 || second.CandidateSource() != vedomain.OperatorDerivedETA {
		t.Fatalf("候选 = %q（%v），want 换手到运营企业 %q", second.CandidateVersion(), second.CandidateSource(), operatorV1)
	}
	if prior, superseding := second.PriorVersion(); !superseding || prior != first.Version() {
		t.Fatalf("判断前版 = %q，want 指回 %q", prior, first.Version())
	}
	handedOver := currentCustomerView(t, fixture)
	if content, shown := handedOver.Dimensions().ETA.Content(); !shown || content.String() != second.Version().String() {
		t.Fatalf("候选换手后视图 ETA 一维 = %v 内容 %q，want 展示且来处为判断 %q",
			handedOver.Dimensions().ETA.State(), content, second.Version())
	}
	viewsAfterHandover := fixture.countSQL(t, `SELECT count(*) FROM visibility_exception.customer_view_version`)

	// 承运商一路再出新版本，且质量已不达标：候选是运营企业那一版，没变，判断与视图都不动。
	carrierV2 := formETAFrom(t, fixture, vedomain.CarrierProvidedETA, "SYN-CARRIER-FEED-2", "LOW/stale")
	beat(t, fixture, "承运商第二份")
	if again := currentETAJudgement(t, fixture); again.Version() != second.Version() {
		t.Fatalf("非候选来源出新版本换了判断：%q → %q", second.Version(), again.Version())
	}
	if n := fixture.countSQL(t, `SELECT count(*) FROM visibility_exception.eta_visibility_judgement_version`); n != 2 {
		t.Fatalf("判断版本 = %d, want 2", n)
	}
	if n := fixture.countSQL(t, `SELECT count(*) FROM visibility_exception.customer_view_version`); n != viewsAfterHandover {
		t.Fatalf("视图版本 = %d, want 仍为 %d", n, viewsAfterHandover)
	}

	// 运营企业一路出新版本且质量不达标：候选版本变了，判断换版为 ETA 不可见，视图随之不展示。
	operatorV2 := formETAFrom(t, fixture, vedomain.OperatorDerivedETA, "SYN-ETA-INPUTS-2", "LOW/sparse")
	beat(t, fixture, "运营企业第二份")
	third := currentETAJudgement(t, fixture)
	if third.Conclusion() != vedomain.ETANotVisible || third.CandidateVersion() != operatorV2 {
		t.Fatalf("判断 = %q 候选 = %q，want ETA 不可见且引用运营企业 %q", third.Conclusion(), third.CandidateVersion(), operatorV2)
	}
	if view := currentCustomerView(t, fixture); view.Dimensions().ETA.State() != vedomain.DimensionNotDisclosed {
		t.Fatalf("ETA 一维 = %v, want 不展示", view.Dimensions().ETA.State())
	}

	// 历史不丢：四个预测版本都在、各线当前版是各自最新的一版，判断留下的候选引用按版本读得回。
	etas, err := vepostgres.NewETAs(fixture.db)
	if err != nil {
		t.Fatalf("构造 ETA 库：%v", err)
	}
	tenant := mustVE(t, vedomain.NewTenantID, fixture.identity.TenantID().String())
	parcel := mustVE(t, vedomain.NewTrackedParcelReference, transportHandoverObject)
	milestone := mustVE(t, vedomain.NewMilestoneReference, "DELIVERED")
	if n := fixture.countSQL(t, `SELECT count(*) FROM visibility_exception.eta_prediction_version`); n != 4 {
		t.Fatalf("预测版本行 = %d, want 4：任何一路的刷新都不该覆盖旧版", n)
	}
	for _, kept := range []struct {
		source  vedomain.ETASourceKind
		version vedomain.ETAVersionID
		inputs  string
	}{
		{vedomain.CarrierProvidedETA, carrierV1, "SYN-CARRIER-FEED-1"},
		{vedomain.CarrierProvidedETA, carrierV2, "SYN-CARRIER-FEED-2"},
		{vedomain.OperatorDerivedETA, operatorV1, "SYN-ETA-INPUTS-1"},
		{vedomain.OperatorDerivedETA, operatorV2, "SYN-ETA-INPUTS-2"},
	} {
		eta, found, err := etas.FindVersion(t.Context(), tenant, parcel, milestone, kept.source, kept.version)
		if err != nil || !found || eta.Inputs().String() != kept.inputs {
			t.Fatalf("预测版本 %q（%v）读不回或输入被改写：found=%v err=%v", kept.version, kept.source, found, err)
		}
	}
	currents, err := etas.ListCurrent(t.Context(), tenant, parcel, milestone)
	if err != nil || len(currents) != 2 || currents[0].Version() != carrierV2 || currents[1].Version() != operatorV2 {
		t.Fatalf("各线当前版 = %v err=%v，want 承运商 %q 与运营企业 %q", currents, err, carrierV2, operatorV2)
	}
	judgements, err := vepostgres.NewETAVisibilityJudgements(fixture.db)
	if err != nil {
		t.Fatalf("构造判断库：%v", err)
	}
	customer := mustVE(t, vedomain.NewCustomerAccountReference, "SYN-CUSTOMER-01")
	oldest, found, err := judgements.FindVersion(t.Context(), tenant, customer, parcel, first.Version())
	if err != nil || !found {
		t.Fatalf("首版判断读不回：found=%v err=%v", found, err)
	}
	candidate, found, err := etas.FindVersion(t.Context(), tenant, parcel, oldest.Milestone(), oldest.CandidateSource(), oldest.CandidateVersion())
	if err != nil || !found || candidate.Inputs().String() != "SYN-CARRIER-FEED-1" {
		t.Fatalf("首版判断的候选引用在预测库里读不回：found=%v err=%v", found, err)
	}
}

// Covers: AT-VE-170 的生产链——租户没有登记 ETA 对客规则：ETA 形成之后判断落「ETA 可见性待判断」，客户视图 ETA 一维仍待确认，
// 不因为策略准许展示就按默认值判为可见，也不形成多余的视图版本。
func TestAnETAWithoutARegisteredRuleIsJudgedPendingAndTheViewStaysPending(t *testing.T) {
	fixture := newAcceptedCustomerViewFixture(t)
	registerETAShownPolicy(t, fixture)
	base := deriveViewFromHandover(t, fixture, transportHandoverScope, transportHandoverVersion)

	formETA(t, fixture, "SYN-ETA-INPUTS-1", "HIGH/carrier-confirmed")
	beat(t, fixture, "无规则的 ETA")

	judgement := currentETAJudgement(t, fixture)
	if judgement.Conclusion() != vedomain.ETAVisibilityPendingJudgement || judgement.Reason() != vedomain.CustomerRuleNotConfigured {
		t.Fatalf("判断 = %q 理由 = %q，want 待判断/规则未配置", judgement.Conclusion(), judgement.Reason())
	}
	view := currentCustomerView(t, fixture)
	if view.Version() != base.view.Version() || view.Dimensions().ETA.State() != vedomain.DimensionPendingConfirmation {
		t.Fatalf("视图 = %s ETA = %v，want 仍是 %s 且待确认", view.Version(), view.Dimensions().ETA.State(), base.view.Version())
	}
}

func beat(t *testing.T, fixture *synVerticalFixture, label string) {
	t.Helper()
	published, err := fixture.beat.DispatchOnce(t.Context())
	if err != nil {
		t.Fatalf("%s 的一拍：%v", label, err)
	}
	if published != 1 {
		t.Fatalf("%s 应恰好定稿一封预测信封：published = %d", label, published)
	}
}

// redeliverETAFormed 以新 ID 把同一版预测信封交给生产的判断那一路。
func redeliverETAFormed(t *testing.T, fixture *synVerticalFixture, version vedomain.ETAVersionID) {
	t.Helper()
	outboxStore, err := outbox.NewStore(fixture.db)
	if err != nil {
		t.Fatalf("构造 Outbox Store：%v", err)
	}
	inboxStore, err := inbox.NewStore(fixture.db)
	if err != nil {
		t.Fatalf("构造 Inbox Store：%v", err)
	}
	consumer, err := judgeETAVisibilityOnPredictionConsumer(fixture.db, outboxStore, inboxStore, systemClock{})
	if err != nil {
		t.Fatalf("装配判断那一路：%v", err)
	}
	tenant := fixture.identity.TenantID().String()
	envelope := redeliveryEnvelope(tenant, "redeliver-eta-"+version.String(), veinbox.ETAPredictionFormedEventType,
		version.String(),
		`{"tenantId":"`+tenant+`","parcel":"`+transportHandoverObject+`","milestone":"DELIVERED","versionId":"`+version.String()+`"}`)
	envelope.Source = "idp-parcel/visibility-exception"
	if err := consumer.Consume(t.Context(), envelope); err != nil {
		t.Fatalf("重投预测信封：%v", err)
	}
}
