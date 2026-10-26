package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// Covers: ADR-0256 决定一第 2 格「重放须接着办」、UC-PS-002「未决」一行——来源保全一经落下，上一次可能停在
// 版本落库之前（授权、阶段、矩阵未决，读不回，允许声明未声明，标识签发不出，保存失败，并发抢先）。重放同一请求
// 要按首次请求把没落成的那一步做完：形成版本，且只形成一次；再重放才是已有结果，带那份版本。
//
// 每一格都走完「停下、条件恢复、重放形成、再重放读回」整条线：只断言重放不再答已有结果挡不住「重放又形成了
// 第二份」，只断言形成了挡不住「后面的重放不读回」。
func TestAReplayedAmendmentIsCarriedOnAfterEveryRoundThatFormedNoVersion(t *testing.T) {
	cases := []struct {
		name    string
		stop    func(*amendmentFixture)
		outcome application.AmendmentOutcome
		reason  application.JudgmentPendingReason
		resume  func(*amendmentFixture)
	}{
		{
			name:    "委托读不回",
			stop:    func(f *amendmentFixture) { f.requests.err = errors.New("storage unreachable") },
			outcome: application.AmendmentUndecided,
			reason:  application.ShipmentRequestUnavailable,
			resume:  func(f *amendmentFixture) { f.requests.err = nil },
		},
		{
			name:    "授权答不出",
			stop:    func(f *amendmentFixture) { f.authorizer.err = errors.New("authority unreachable") },
			outcome: application.AmendmentUndecided,
			reason:  application.SourceDataAmendmentAuthorityUnavailable,
			resume:  func(f *amendmentFixture) { f.authorizer.err = nil },
		},
		{
			name:    "授权规则未登记",
			stop:    func(f *amendmentFixture) { f.authorizer.rulesNotConfigured = true },
			outcome: application.AmendmentUndecided,
			reason:  application.SourceDataAmendmentAuthorityRulesNotConfigured,
			resume:  func(f *amendmentFixture) { f.authorizer.rulesNotConfigured = false },
		},
		{
			name:    "授权适用冲突",
			stop:    func(f *amendmentFixture) { f.authorizer.applicabilityConflict = true },
			outcome: application.AmendmentUndecided,
			reason:  application.SourceDataAmendmentAuthorityApplicabilityConflict,
			resume:  func(f *amendmentFixture) { f.authorizer.applicabilityConflict = false },
		},
		{
			name:    "阶段事实读不回",
			stop:    func(f *amendmentFixture) { f.stage.err = errors.New("adoption register unreachable") },
			outcome: application.AmendmentUndecided,
			reason:  application.SourceDataAmendmentStageFactUnavailable,
			resume:  func(f *amendmentFixture) { f.stage.err = nil },
		},
		{
			name: "阶段判不出",
			stop: func(f *amendmentFixture) {
				f.stage.customs["parcel-1"] = unconnectedCustoms()
				f.stage.customs["parcel-2"] = unconnectedCustoms()
			},
			outcome: application.AmendmentUndecided,
			reason:  application.SourceDataAmendmentStageUndetermined,
			resume: func(f *amendmentFixture) {
				delete(f.stage.customs, "parcel-1")
				delete(f.stage.customs, "parcel-2")
			},
		},
		{
			name:    "允许声明读不回",
			stop:    func(f *amendmentFixture) { f.rules.err = errors.New("declaration unreachable") },
			outcome: application.AmendmentUndecided,
			reason:  application.SourceDataRuleUnavailable,
			resume:  func(f *amendmentFixture) { f.rules.err = nil },
		},
		{
			name:    "允许声明未声明（等复核）",
			stop:    func(f *amendmentFixture) { f.rules.allowance = ports.SourceDataAmendmentNotDeclared },
			outcome: application.AmendmentAwaitingReview,
			resume:  func(f *amendmentFixture) { f.rules.allowance = ports.SourceDataAmendmentAllowed },
		},
		{
			name:    "版本标识签发不出",
			stop:    func(f *amendmentFixture) { f.identities.err = errors.New("identity factory unreachable") },
			outcome: application.AmendmentUndecided,
			reason:  application.SourceDataVersionIdentityUnavailable,
			resume:  func(f *amendmentFixture) { f.identities.err = nil },
		},
		{
			name:    "保存失败",
			stop:    func(f *amendmentFixture) { f.requests.saveErr = errors.New("storage unreachable") },
			outcome: application.AmendmentUndecided,
			reason:  application.AmendedRequestNotSaved,
			resume:  func(f *amendmentFixture) { f.requests.saveErr = nil },
		},
		{
			name:    "并发写入抢先",
			stop:    func(f *amendmentFixture) { f.requests.saveConflict = true },
			outcome: application.AmendmentUndecided,
			reason:  application.StaleShipmentRequestRevision,
			resume:  func(f *amendmentFixture) { f.requests.saveConflict = false },
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fixture := newAmendmentFixture(t)
			command := fixture.command(t)

			tc.stop(fixture)
			stopped, err := fixture.handler.Handle(context.Background(), command)
			if err != nil {
				t.Fatalf("handle the round that stops: %v", err)
			}
			if stopped.Outcome() != tc.outcome || stopped.PendingReason() != tc.reason {
				t.Fatalf("outcome = %q reason = %q, want %q / %q", stopped.Outcome(), stopped.PendingReason(), tc.outcome, tc.reason)
			}
			if _, present := stopped.Version(); present {
				t.Fatal("a round that stopped reported a formed version")
			}
			if _, preserved := fixture.sources.stored(t, amendmentSourceIdentity(t)); !preserved {
				t.Fatal("the stopped round did not preserve its source, so there is nothing for a replay to recognise")
			}
			if fixture.requests.saved != nil {
				t.Fatal("a round that stopped saved the request")
			}

			tc.resume(fixture)
			replayed, err := fixture.handler.Handle(context.Background(), command)
			if err != nil {
				t.Fatalf("replay the stopped request: %v", err)
			}
			if replayed.Outcome() != application.AmendmentRecorded {
				t.Fatalf("replay outcome = %q reason = %q, want RECORDED——a replay must carry on, not report a request that never formed as handled",
					replayed.Outcome(), replayed.PendingReason())
			}
			formed, present := replayed.Version()
			if !present {
				t.Fatal("the replay that carried on formed no version")
			}
			if fixture.requests.saved == nil || len(fixture.requests.saved.CustomerSourceDataVersions()) != 1 {
				t.Fatal("the replay did not leave exactly one version on the request")
			}
			if preserved, _ := fixture.sources.stored(t, amendmentSourceIdentity(t)); preserved.Digest() != command.PayloadDigest {
				t.Fatal("the replay overwrote the preserved source")
			}
			if fixture.sources.preserveCount != 1 {
				t.Fatalf("preserved %d times; a replay recognises the preserved source and must not preserve a second one", fixture.sources.preserveCount)
			}
			if len(fixture.sources.observations) != 1 {
				t.Fatalf("observations = %d; the replay is appended beside the preserved fact once", len(fixture.sources.observations))
			}

			authorizations := fixture.authorizer.calls
			again, err := fixture.handler.Handle(context.Background(), command)
			if err != nil {
				t.Fatalf("replay the formed request: %v", err)
			}
			if again.Outcome() != application.AmendmentAlreadyHandled {
				t.Fatalf("second replay outcome = %q, want ALREADY_HANDLED", again.Outcome())
			}
			version, present := again.Version()
			if !present || version.VersionID() != formed.VersionID() {
				t.Fatalf("second replay version = %q (present=%v), want the one the first replay formed (%q)",
					version.VersionID(), present, formed.VersionID())
			}
			if fixture.authorizer.calls != authorizations {
				t.Fatal("a replay of a request that already formed its version asked the authorizer again")
			}
			if kept := fixture.requests.saved.CustomerSourceDataVersions(); len(kept) != 1 {
				t.Fatalf("versions on the request = %d, want the one only", len(kept))
			}
		})
	}
}

// Covers: 同一场景的交接半边——停在授权之后重放形成的版本，下游收到的意图指着这一份版本；再重放读回它并重发
// 同一份意图，不是第二份。
func TestAReplayThatFormsTheVersionHandsOffThatVersion(t *testing.T) {
	fixture := newAmendmentFixture(t)
	command := fixture.command(t)
	fixture.authorizer.err = errors.New("authority unreachable")
	if _, err := fixture.handler.Handle(context.Background(), command); err != nil {
		t.Fatalf("handle the round that stops: %v", err)
	}
	if len(fixture.downstream.intents) != 0 {
		t.Fatalf("handed off %d intents for a round that formed nothing", len(fixture.downstream.intents))
	}

	fixture.authorizer.err = nil
	replayed, err := fixture.handler.Handle(context.Background(), command)
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	formed, _ := replayed.Version()
	if len(fixture.downstream.intents) != 1 || fixture.downstream.intents[0].Version != formed.VersionID() {
		t.Fatalf("intents = %+v, want one pointing at the version the replay formed (%q)", fixture.downstream.intents, formed.VersionID())
	}
	if _, err := fixture.handler.Handle(context.Background(), command); err != nil {
		t.Fatalf("replay again: %v", err)
	}
	if len(fixture.downstream.intents) != 2 || fixture.downstream.intents[1].Version != formed.VersionID() {
		t.Fatalf("intents = %+v, want the same version re-sent and no second version", fixture.downstream.intents)
	}
}

// Covers: 确定的业务答案重放不被改写成已有结果——未获授权与登记为不允许在重放时重新判，条件没变就还是同一个答案，
// 不是 ALREADY_HANDLED 却不带版本；条件变了（授权补上了）才接着办形成版本。
func TestAReplayedAmendmentThatWasRefusedIsAnsweredAgainNotReportedAsHandled(t *testing.T) {
	t.Run("未获授权", func(t *testing.T) {
		fixture := newAmendmentFixture(t)
		command := fixture.command(t)
		fixture.authorizer.granted = false

		first, err := fixture.handler.Handle(context.Background(), command)
		if err != nil || first.Outcome() != application.AmendmentNotAuthorized {
			t.Fatalf("first: outcome = %q err = %v, want NOT_AUTHORIZED", first.Outcome(), err)
		}
		again, err := fixture.handler.Handle(context.Background(), command)
		if err != nil || again.Outcome() != application.AmendmentNotAuthorized {
			t.Fatalf("replay: outcome = %q err = %v, want NOT_AUTHORIZED again", again.Outcome(), err)
		}

		fixture.authorizer.granted = true
		carried, err := fixture.handler.Handle(context.Background(), command)
		if err != nil || carried.Outcome() != application.AmendmentRecorded {
			t.Fatalf("replay after the grant: outcome = %q err = %v, want RECORDED", carried.Outcome(), err)
		}
	})

	t.Run("登记为不允许", func(t *testing.T) {
		fixture := newAmendmentFixture(t)
		command := fixture.command(t)
		fixture.rules.allowance = ports.SourceDataAmendmentDisallowed

		first, err := fixture.handler.Handle(context.Background(), command)
		if err != nil || first.Outcome() != application.AmendmentDisallowed {
			t.Fatalf("first: outcome = %q err = %v, want DISALLOWED", first.Outcome(), err)
		}
		again, err := fixture.handler.Handle(context.Background(), command)
		if err != nil || again.Outcome() != application.AmendmentDisallowed {
			t.Fatalf("replay: outcome = %q err = %v, want DISALLOWED again", again.Outcome(), err)
		}
		if fixture.requests.saved != nil || fixture.identities.issued != 0 {
			t.Fatal("a refused amendment formed a version or consumed an identity")
		}
	})
}

// Covers: 受理阶段调用方的错重放仍是调用方的错——修订意图没声明的请求，首次答 ErrAmendmentIntentNotDeclared，
// 来源已保全；重放不能因此变成不带版本的已有结果。
func TestAReplayedAmendmentWithoutADeclaredIntentIsStillACallerError(t *testing.T) {
	fixture := newAmendmentFixture(t)
	command := fixture.command(t)
	command.Intent = domain.AmendmentIntentInvalid

	for _, round := range []string{"first", "replay"} {
		_, err := fixture.handler.Handle(context.Background(), command)
		if !errors.Is(err, application.ErrAmendmentIntentNotDeclared) {
			t.Fatalf("%s: err = %v, want ErrAmendmentIntentNotDeclared", round, err)
		}
	}
	if fixture.requests.saved != nil || fixture.authorizer.calls != 0 {
		t.Fatal("a request without a declared intent reached authorization or formed a version")
	}
}

// Covers: 受理阶段调用方的错重放仍是调用方的错——内容与范围、意图不配的请求，首次答 ErrAmendmentContentUnfit（接
// HTTP 答 400，改请求才会好），来源已保全；重放同样要在授权之前拒，仍答 ErrAmendmentContentUnfit。
//
// 这一格要单独钉住：重放若漏了这道受理，请求会跑完授权、阶段、矩阵与版本号签发，到 FormCustomerSourceDataVersion
// 的领域门才被拒，调用方的错于是被当成服务端的错（5xx），客户拿同一份请求重试永远得不到「改请求」的提示。两轮
// 都不许碰授权、不签版本号、不交接下游。
func TestAReplayedAmendmentWhoseContentDoesNotFitIsStillACallerError(t *testing.T) {
	cases := map[string]func(*testing.T, *application.AmendCustomerSourceDataCommand){
		"收件范围带测量内容": func(t *testing.T, command *application.AmendCustomerSourceDataCommand) {
			command.Scope = deliveryPlaceDataScope(t)
			command.Content = weightOnlyContent(t, "1.00")
		},
		"开放范围带要素内容": func(t *testing.T, command *application.AmendCustomerSourceDataCommand) {
			command.Content = deliveryPostalCodeContent(t, "SYN-200095")
		},
		"显式清空带要素内容": func(t *testing.T, command *application.AmendCustomerSourceDataCommand) {
			command.Scope = deliveryPlaceDataScope(t)
			command.Intent = domain.ExplicitClearIntent
			command.Content = deliveryPostalCodeContent(t, "SYN-200095")
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			fixture := newAmendmentFixture(t)
			command := fixture.command(t)
			mutate(t, &command)

			for _, round := range []string{"first", "replay"} {
				_, err := fixture.handler.Handle(context.Background(), command)
				if !errors.Is(err, application.ErrAmendmentContentUnfit) {
					t.Fatalf("%s: err = %v, want ErrAmendmentContentUnfit", round, err)
				}
			}
			if fixture.authorizer.calls != 0 {
				t.Fatalf("authorized %d times; a request that cannot stand is refused before asking about authority, replayed or not", fixture.authorizer.calls)
			}
			if fixture.identities.issued != 0 {
				t.Fatalf("issued %d version IDs for a request that cannot stand", fixture.identities.issued)
			}
			if fixture.requests.saved != nil || len(fixture.downstream.intents) != 0 {
				t.Fatal("a request that cannot stand formed a version or was handed off")
			}
			if _, preserved := fixture.sources.stored(t, amendmentSourceIdentity(t)); !preserved {
				t.Fatal("the refused request still arrived; its source fact must be preserved")
			}
		})
	}
}

// Covers: AT-PS-017「同一请求身份携带不同内容 → 请求冲突，原请求和原版本不被覆盖」——停在未决之后，同身份不同内容
// 仍答冲突，不被重放逻辑吞成接着办；原请求的内容摘要不变，也不形成版本。
func TestAnAmendmentWithDifferentContentAfterAStoppedRoundStillConflicts(t *testing.T) {
	fixture := newAmendmentFixture(t)
	fixture.authorizer.err = errors.New("authority unreachable")
	if _, err := fixture.handler.Handle(context.Background(), fixture.command(t)); err != nil {
		t.Fatalf("handle the round that stops: %v", err)
	}

	fixture.authorizer.err = nil
	conflicting := fixture.command(t)
	conflicting.PayloadDigest = mustValue(t, domain.NewPayloadDigest, "amend-digest-2")
	authorizations := fixture.authorizer.calls

	result, err := fixture.handler.Handle(context.Background(), conflicting)
	if err != nil {
		t.Fatalf("handle the conflicting request: %v", err)
	}
	if result.Outcome() != application.AmendmentSourceConflict {
		t.Fatalf("outcome = %q, want SOURCE_CONFLICT", result.Outcome())
	}
	if fixture.authorizer.calls != authorizations || fixture.requests.saved != nil {
		t.Fatal("a conflicting request was carried on instead of refused")
	}
	if preserved, _ := fixture.sources.stored(t, amendmentSourceIdentity(t)); preserved.Digest().String() != "amend-digest-1" {
		t.Fatalf("preserved digest = %q; the conflicting request overwrote the original source", preserved.Digest())
	}
	if len(fixture.sources.observations) != 0 {
		t.Fatal("a conflicting request was appended as an observation of the original")
	}
}

// Covers: 重放形成的版本记的请求是保全下来的那一份——来源保全只认第一次到达的指纹，版本上的请求留痕与它一致，
// 不是重放那一次到达的接收时间。
func TestAVersionFormedByAReplayCarriesTheFirstArrivalsSourceRecord(t *testing.T) {
	fixture := newAmendmentFixture(t)
	command := fixture.command(t)
	fixture.authorizer.err = errors.New("authority unreachable")
	if _, err := fixture.handler.Handle(context.Background(), command); err != nil {
		t.Fatalf("handle the round that stops: %v", err)
	}

	fixture.authorizer.err = nil
	replay := command
	replay.ReceivedAt = command.ReceivedAt.Add(time.Hour)
	replay.OccurredAt = command.OccurredAt.Add(time.Hour)
	result, err := fixture.handler.Handle(context.Background(), replay)
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	version, present := result.Version()
	if !present {
		t.Fatalf("replay outcome = %q reason = %q, want a formed version", result.Outcome(), result.PendingReason())
	}
	if !version.Request().ReceivedAt().Equal(command.ReceivedAt) || !version.Request().OccurredAt().Equal(command.OccurredAt) {
		t.Fatalf("version request received %v occurred %v, want the first arrival's %v / %v",
			version.Request().ReceivedAt(), version.Request().OccurredAt(), command.ReceivedAt, command.OccurredAt)
	}
}

// Covers: 重放在委托的资料版本里按这次请求的身份找版本，不是见到任何一份修订版本就当办过——同一份委托上别的修订请求
// 已经形成了版本，这次停在授权上的请求重放时仍要形成自己的版本；再重放读回的是它自己的那一份。
func TestAReplayIsNotAnsweredWithAnotherRequestsVersion(t *testing.T) {
	fixture := newAmendmentFixture(t)
	earlier := fixture.command(t)
	if result, err := fixture.handler.Handle(context.Background(), earlier); err != nil || result.Outcome() != application.AmendmentRecorded {
		t.Fatalf("the earlier request: outcome = %q err = %v, want RECORDED", result.Outcome(), err)
	}

	later := fixture.command(t)
	later.AmendmentIdentity = sourceIdentity(t, "tenant-1", "customer-1", "source-a", "amend-key-2")
	later.PayloadDigest = mustValue(t, domain.NewPayloadDigest, "amend-digest-2")
	fixture.authorizer.err = errors.New("authority unreachable")
	if stopped, err := fixture.handler.Handle(context.Background(), later); err != nil || stopped.Outcome() != application.AmendmentUndecided {
		t.Fatalf("the later request: outcome = %q err = %v, want UNDECIDED", stopped.Outcome(), err)
	}

	fixture.authorizer.err = nil
	for _, want := range []application.AmendmentOutcome{application.AmendmentRecorded, application.AmendmentAlreadyHandled} {
		result, err := fixture.handler.Handle(context.Background(), later)
		if err != nil {
			t.Fatalf("replay the later request: %v", err)
		}
		if result.Outcome() != want {
			t.Fatalf("outcome = %q, want %q", result.Outcome(), want)
		}
		version, present := result.Version()
		if !present || version.Request().Identity() != later.AmendmentIdentity {
			t.Fatalf("version of %v (present=%v), want the version this request formed", version.Request().Identity(), present)
		}
	}
	if kept := fixture.requests.saved.CustomerSourceDataVersions(); len(kept) != 2 {
		t.Fatalf("versions on the request = %d, want one per request", len(kept))
	}
}
