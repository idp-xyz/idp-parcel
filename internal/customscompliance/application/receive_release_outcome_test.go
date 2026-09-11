package application_test

import (
	"context"
	"testing"

	"go.idp.xyz/idp-parcel/internal/customscompliance/application"
	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
)

// UC-CC-006 步 5 的放行层（票 mechanism-executor-triage/07 CC-b）：放行层的来源响应在
// 分层事实之外还要落成 CustomsReleaseOutcome——种类、监管机构、范围、条件四件由接入侧
// 按来源权威语义拆出交进命令（与 VerifyDispositionCommand 交入已成型的 RegulatoryDecision
// 同形）；拆不出来的放行层响应是`外部结果解释未决`，不是「先记一条再说」。

func releaseCommand(t *testing.T, sourceID string, kind domain.ReleaseKind, condition string) application.ReceiveExternalResultCommand {
	t.Helper()
	command := resultCommand(t, sourceID)
	command.Layer = domain.ReleaseResultLayer
	command.RawSemantics = "RELEASE"
	command.Release = &application.ReleaseContent{
		Kind:      kind,
		Authority: mustValue(t, domain.NewRegulatoryAuthorityReference, "customs-authority-1"),
		Condition: condition,
	}
	return command
}

// 放行层带齐放行三件：分层事实照常形成，同一记录上落成放行结果——范围取结果范围、接收
// 时间取来源接收时间，附条件那格连条件一起回来。
func TestAReleaseLayerResultAlsoFormsTheReleaseOutcome(t *testing.T) {
	fixture := newResultFixture(t)

	result, err := fixture.handler.Handle(context.Background(),
		releaseCommand(t, "source-release-1", domain.ConditionalRelease, "re-export within 30 days"))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result.Outcome() != application.ResultRecorded {
		t.Fatalf("outcome = %q, want RESULT_RECORDED", result.Outcome())
	}
	record, present := result.Record()
	if !present || record.Release == nil {
		t.Fatalf("放行层记录没落成放行结果：%+v", record)
	}
	release := *record.Release
	condition, conditional := release.Condition()
	if release.Kind() != domain.ConditionalRelease || !conditional || condition != "re-export within 30 days" ||
		release.Authority().String() != "customs-authority-1" ||
		release.Scope().String() != "scope-unit-1" ||
		!release.ReceivedAt().Equal(record.Result.ReceivedAt()) {
		t.Fatalf("放行结果走样：kind=%s authority=%s scope=%s condition=%q receivedAt=%v",
			release.Kind(), release.Authority(), release.Scope(), condition, release.ReceivedAt())
	}
	if len(fixture.handoff.intents) != 1 {
		t.Fatalf("intents = %d, want 1", len(fixture.handoff.intents))
	}
}

// 放行层的来源语义没被拆成放行三件：这就是 UC-CC-006「外部结果解释未决」那一格——业务
// 结果目标已关联，但层次语义无权威解释。不落记录、不交意图、不按文字猜种类。
func TestAReleaseLayerResultWithoutReleaseContentIsUndecided(t *testing.T) {
	fixture := newResultFixture(t)
	command := releaseCommand(t, "source-release-2", domain.FullRelease, "")
	command.Release = nil

	result, err := fixture.handler.Handle(context.Background(), command)
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result.Outcome() != application.ResultUndecided ||
		result.UndecidedReason() != application.ReleaseSemanticsUninterpreted {
		t.Fatalf("outcome = %q reason = %q, want RESULT_UNDECIDED / RELEASE_SEMANTICS_UNINTERPRETED",
			result.Outcome(), result.UndecidedReason())
	}
	if result.ContinuationReference() == "" {
		t.Fatal("未决没有留续办引用")
	}
	if fixture.store.saves != 0 || len(fixture.handoff.intents) != 0 {
		t.Fatalf("未决却落了记录或交了意图：saves=%d intents=%d", fixture.store.saves, len(fixture.handoff.intents))
	}
}

// 非放行层带着放行三件进来是矛盾输入——业务受理携带放行内容就是把放行夹带进低层结果
// （CONTEXT「任何前一层成功都不能自动生成后一层结果」：任何前一层成功都不能自动生成后一层结果）。不受理，不落。
func TestReleaseContentOnANonReleaseLayerIsNotAccepted(t *testing.T) {
	fixture := newResultFixture(t)
	command := releaseCommand(t, "source-release-3", domain.FullRelease, "")
	command.Layer = domain.BusinessAcceptanceLayer

	result, err := fixture.handler.Handle(context.Background(), command)
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result.Outcome() != application.ResultNotAccepted {
		t.Fatalf("outcome = %q, want SOURCE_NOT_ACCEPTED", result.Outcome())
	}
	if fixture.store.saves != 0 {
		t.Fatal("被拒的响应落了记录")
	}
}

// 放行三件立不起领域对象的每一格都不受理：附条件放行说不出条件、全部放行却带条件、
// 监管机构缺席、种类集外。领域构造把门，这里只钉编排把拒绝翻成`未受理`且不落。
func TestMalformedReleaseContentIsNotAccepted(t *testing.T) {
	fixture := newResultFixture(t)

	conditionalWithoutCondition := releaseCommand(t, "source-release-4", domain.ConditionalRelease, "")
	fullWithCondition := releaseCommand(t, "source-release-5", domain.FullRelease, "unexpected condition")
	blankAuthority := releaseCommand(t, "source-release-6", domain.FullRelease, "")
	blankAuthority.Release.Authority = domain.RegulatoryAuthorityReference{}
	unknownKind := releaseCommand(t, "source-release-7", domain.ReleaseKindInvalid, "")

	for name, command := range map[string]application.ReceiveExternalResultCommand{
		"附条件无条件":  conditionalWithoutCondition,
		"全部放行带条件": fullWithCondition,
		"机构缺席":    blankAuthority,
		"种类集外":    unknownKind,
	} {
		result, err := fixture.handler.Handle(context.Background(), command)
		if err != nil {
			t.Fatalf("%s：handle: %v", name, err)
		}
		if result.Outcome() != application.ResultNotAccepted {
			t.Fatalf("%s该不受理：outcome = %q", name, result.Outcome())
		}
	}
	if fixture.store.saves != 0 {
		t.Fatal("被拒的放行响应落了记录")
	}
}

// 放行三件参与同一来源身份的内容指纹：同来源身份先说全部放行、再说部分放行是来源响应
// 冲突（保留原结果，不按最后到达覆盖）；重放同一份放行是`已有来源处理结果`。
func TestReleaseContentTakesPartInTheSourceContentDigest(t *testing.T) {
	fixture := newResultFixture(t)

	if result, err := fixture.handler.Handle(context.Background(),
		releaseCommand(t, "source-release-8", domain.FullRelease, "")); err != nil ||
		result.Outcome() != application.ResultRecorded {
		t.Fatalf("首次：err=%v outcome=%q", err, result.Outcome())
	}
	replay, err := fixture.handler.Handle(context.Background(),
		releaseCommand(t, "source-release-8", domain.FullRelease, ""))
	if err != nil || replay.Outcome() != application.ResultExistingResult {
		t.Fatalf("重放该是已有结果：err=%v outcome=%q", err, replay.Outcome())
	}
	changed, err := fixture.handler.Handle(context.Background(),
		releaseCommand(t, "source-release-8", domain.PartialRelease, ""))
	if err != nil || changed.Outcome() != application.ResultSourceConflict {
		t.Fatalf("同身份换放行种类该是来源响应冲突：err=%v outcome=%q", err, changed.Outcome())
	}
	replayed, _ := replay.Record()
	stored, present := fixture.store.records[resultKey(replayed.Key)]
	if !present || stored.Release == nil || stored.Release.Kind() != domain.FullRelease {
		t.Fatalf("冲突顶掉了原放行结果：%+v", stored)
	}
}

// 非放行层的响应不受本笔影响：不带放行三件照常形成分层事实，记录上无放行结果。
func TestANonReleaseLayerResultCarriesNoReleaseOutcome(t *testing.T) {
	fixture := newResultFixture(t)

	result, err := fixture.handler.Handle(context.Background(), resultCommand(t, "source-receipt-1"))
	if err != nil || result.Outcome() != application.ResultRecorded {
		t.Fatalf("监管接收：err=%v outcome=%q", err, result.Outcome())
	}
	if record, _ := result.Record(); record.Release != nil {
		t.Fatalf("监管接收层凭空多出放行结果：%+v", record.Release)
	}
}
