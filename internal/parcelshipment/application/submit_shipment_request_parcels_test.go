package application_test

import (
	"context"
	"slices"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

// twoParcelCommand 是两件、各带客户包裹件号的一份提交；minted 是接单入口这一次现签的两枚内部标识。
func twoParcelCommand(t *testing.T, value *fixture, minted ...string) application.SubmitShipmentRequestCommand {
	t.Helper()
	command := value.command(t)
	command.DeclaredParcelIDs = nil
	command.CustomerParcelNumbers = nil
	for index, parcel := range minted {
		member := mustValue(t, domain.NewDeclaredParcelID, parcel)
		command.DeclaredParcelIDs = append(command.DeclaredParcelIDs, member)
		declaration, err := domain.DeclareCustomerParcelNumber(
			member, mustValue(t, domain.NewExternalIdentifierNumber, []string{"CUST-0001", "CUST-0002"}[index]))
		if err != nil {
			t.Fatalf("配对：%v", err)
		}
		command.CustomerParcelNumbers = append(command.CustomerParcelNumbers, declaration)
	}
	return command
}

// parcelPairs 把结果里的逐件摊成「内部标识=客户件号」，没声明件号的那一件只写内部标识。
func parcelPairs(result application.SubmitShipmentRequestResult) []string {
	pairs := make([]string, 0, len(result.Parcels()))
	for _, parcel := range result.Parcels() {
		pair := parcel.Parcel().String()
		if number, declared := parcel.CustomerParcelNumber(); declared {
			pair += "=" + number.String()
		}
		pairs = append(pairs, pair)
	}
	return pairs
}

// Covers: `AT-PS-101`（ADR-0223 决定一）——建成委托的结果逐件指名委托上的声明包裹，配客户为它声明的件号，次序同声明；
// 指名的就是落库那份委托的成员，不是别处另算的一组。
func TestASubmittedResultNamesEachDeclaredParcelBesideTheCustomersReference(t *testing.T) {
	fixture := newFixture(t)

	result, err := fixture.handler.Handle(context.Background(), twoParcelCommand(t, fixture, "parcel-1", "parcel-2"))
	if err != nil || result.Outcome() != application.OutcomeSubmitted {
		t.Fatalf("outcome = %q, err = %v, want SUBMITTED", result.Outcome(), err)
	}
	if got, want := parcelPairs(result), []string{"parcel-1=CUST-0001", "parcel-2=CUST-0002"}; !slices.Equal(got, want) {
		t.Fatalf("parcels = %v, want %v", got, want)
	}
	stored, _ := fixture.requests.stored(t, fixture.identity(t))
	members := make([]string, 0, 2)
	for _, member := range stored.CurrentSubmissionVersion().DeclaredParcelIDs() {
		members = append(members, member.String())
	}
	if !slices.Equal(members, []string{"parcel-1", "parcel-2"}) {
		t.Fatalf("落库成员 = %v，与结果指名的不是同一组", members)
	}
}

// Covers: `AT-PS-102`（ADR-0223 决定二）——同身份同内容重放答`已有结果`，带回原委托已签的那组号，与首答逐件相同；接单入口
// 这一次现签的号不出现，也不重签。
func TestAReplayHandsBackTheParcelsTheRequestWasBuiltWithNotTheFreshlyMintedOnes(t *testing.T) {
	fixture := newFixture(t)
	ctx := context.Background()

	first, err := fixture.handler.Handle(ctx, twoParcelCommand(t, fixture, "parcel-1", "parcel-2"))
	if err != nil {
		t.Fatalf("first handle: %v", err)
	}
	replay, err := fixture.handler.Handle(ctx, twoParcelCommand(t, fixture, "parcel-8", "parcel-9"))
	if err != nil || replay.Outcome() != application.OutcomeExistingResult {
		t.Fatalf("outcome = %q, err = %v, want EXISTING_RESULT", replay.Outcome(), err)
	}
	if got, want := parcelPairs(replay), parcelPairs(first); !slices.Equal(got, want) {
		t.Fatalf("重放带回 %v，want 首答那组 %v", got, want)
	}
}

// Covers: `AT-PS-103`（ADR-0223 决定二）——并发下建单撞「已存在」，答复带先建成那一份的号，不带本次命令现签、没落库的号。
func TestAConcurrentInsertConflictHandsBackTheWinnersParcels(t *testing.T) {
	fixture := newFixture(t)
	ctx := context.Background()

	if _, err := fixture.handler.Handle(ctx, twoParcelCommand(t, fixture, "parcel-1", "parcel-2")); err != nil {
		t.Fatalf("first handle: %v", err)
	}
	fixture.sources.missPreservedOnce = true

	concurrent, err := fixture.handler.Handle(ctx, twoParcelCommand(t, fixture, "parcel-8", "parcel-9"))
	if err != nil || concurrent.Outcome() != application.OutcomeExistingResult {
		t.Fatalf("outcome = %q, err = %v, want EXISTING_RESULT", concurrent.Outcome(), err)
	}
	if got, want := parcelPairs(concurrent), []string{"parcel-1=CUST-0001", "parcel-2=CUST-0002"}; !slices.Equal(got, want) {
		t.Fatalf("parcels = %v, want 先建那一份的 %v", got, want)
	}
}

// Covers: `AT-PS-104`（ADR-0223 决定一）——没有建成委托的结果不指名任何包裹：接入冲突、输入未受理、归属挡下，以及重放一份
// 从未建过单的输入。命令里现签的号从没落库，交出去客户就记下一组查不到的号。
func TestAResultThatNamesNoRequestNamesNoParcels(t *testing.T) {
	ctx := context.Background()
	cases := map[string]func(*testing.T) application.SubmitShipmentRequestResult{
		"ingress conflict": func(t *testing.T) application.SubmitShipmentRequestResult {
			fixture := newFixture(t)
			if _, err := fixture.handler.Handle(ctx, twoParcelCommand(t, fixture, "parcel-1", "parcel-2")); err != nil {
				t.Fatalf("first handle: %v", err)
			}
			changed := twoParcelCommand(t, fixture, "parcel-8", "parcel-9")
			changed.PayloadDigest = mustValue(t, domain.NewPayloadDigest, "digest-2")
			result, err := fixture.handler.Handle(ctx, changed)
			if err != nil || result.Outcome() != application.OutcomeIngressConflict {
				t.Fatalf("outcome = %q, err = %v, want INGRESS_CONFLICT", result.Outcome(), err)
			}
			return result
		},
		"input not accepted": func(t *testing.T) application.SubmitShipmentRequestResult {
			fixture := newFixture(t)
			command := twoParcelCommand(t, fixture, "parcel-1", "parcel-2")
			command.CustomerParcelNumbers = command.CustomerParcelNumbers[:1]
			command.CustomerParcelNumbers = append(command.CustomerParcelNumbers, command.CustomerParcelNumbers[0])
			result, err := fixture.handler.Handle(ctx, command)
			if err != nil || result.Outcome() != application.OutcomeInputNotAccepted {
				t.Fatalf("outcome = %q, err = %v, want INPUT_NOT_ACCEPTED", result.Outcome(), err)
			}
			return result
		},
		"ownership unresolved": func(t *testing.T) application.SubmitShipmentRequestResult {
			fixture := newFixture(t)
			fixture.ownership.authority = domain.ProductionAuthorityUnresolved
			result, err := fixture.handler.Handle(ctx, twoParcelCommand(t, fixture, "parcel-1", "parcel-2"))
			if err != nil || result.Outcome() != application.OutcomeOwnershipUnresolved {
				t.Fatalf("outcome = %q, err = %v, want OWNERSHIP_UNRESOLVED", result.Outcome(), err)
			}
			return result
		},
		"replay of an input that never built a request": func(t *testing.T) application.SubmitShipmentRequestResult {
			fixture := newFixture(t)
			fixture.ownership.control = domain.AdmissionControlPaused
			if _, err := fixture.handler.Handle(ctx, twoParcelCommand(t, fixture, "parcel-1", "parcel-2")); err != nil {
				t.Fatalf("first handle: %v", err)
			}
			result, err := fixture.handler.Handle(ctx, twoParcelCommand(t, fixture, "parcel-8", "parcel-9"))
			if err != nil || result.Outcome() != application.OutcomeExistingResult {
				t.Fatalf("outcome = %q, err = %v, want EXISTING_RESULT", result.Outcome(), err)
			}
			return result
		},
	}
	for name, run := range cases {
		t.Run(name, func(t *testing.T) {
			result := run(t)
			if _, named := result.ShipmentRequestID(); named {
				t.Fatal("没有建成委托的结果指名了委托")
			}
			if parcels := result.Parcels(); len(parcels) != 0 {
				t.Fatalf("没有建成委托的结果指名了 %d 件包裹", len(parcels))
			}
		})
	}
}

// Covers: `AT-PS-105`（ADR-0223 决定一）——没声明件号的那一件只带内部标识，不拿空值或内部标识顶替客户参考；受控补充形成
// 新提交版本、成员次序换了之后，重放仍按首个提交版本的次序交回。
func TestAParcelWithoutACustomerNumberStandsAloneAndTheOrderOutlivesASupplement(t *testing.T) {
	fixture := newFixture(t)
	ctx := context.Background()
	command := twoParcelCommand(t, fixture, "parcel-1", "parcel-2")
	command.CustomerParcelNumbers = command.CustomerParcelNumbers[1:]

	first, err := fixture.handler.Handle(ctx, command)
	if err != nil || first.Outcome() != application.OutcomeSubmitted {
		t.Fatalf("outcome = %q, err = %v, want SUBMITTED", first.Outcome(), err)
	}
	if got, want := parcelPairs(first), []string{"parcel-1", "parcel-2=CUST-0002"}; !slices.Equal(got, want) {
		t.Fatalf("parcels = %v, want %v", got, want)
	}

	supplementedAt := time.Date(2026, 8, 7, 13, 0, 0, 0, time.UTC)
	supplement, err := domain.NewSourceSubmissionFingerprint(
		sourceIdentity(t, "tenant-1", "customer-1", "source-a", "supplement-key-1"),
		mustValue(t, domain.NewPayloadDigest, "digest-3"),
		supplementedAt,
		supplementedAt.Add(time.Second),
	)
	if err != nil {
		t.Fatalf("new supplement fingerprint: %v", err)
	}
	stored, _ := fixture.requests.stored(t, fixture.identity(t))
	superseded, err := stored.FormNewSubmissionVersion(domain.NewSubmissionVersionSpec{
		VersionID:        mustValue(t, domain.NewSubmissionVersionID, "version-supplement"),
		TaskID:           mustValue(t, domain.NewAcceptanceDecisionTaskID, "task-supplement"),
		SourceSubmission: supplement,
		DeclaredParcelIDs: []domain.DeclaredParcelID{
			mustValue(t, domain.NewDeclaredParcelID, "parcel-2"),
			mustValue(t, domain.NewDeclaredParcelID, "parcel-1"),
		},
		EstablishedAt: supplementedAt,
	})
	if err != nil {
		t.Fatalf("受控补充：%v", err)
	}
	fixture.requests.records[fixture.identity(t)] = superseded

	replay, err := fixture.handler.Handle(ctx, command)
	if err != nil || replay.Outcome() != application.OutcomeExistingResult {
		t.Fatalf("outcome = %q, err = %v, want EXISTING_RESULT", replay.Outcome(), err)
	}
	if got, want := parcelPairs(replay), parcelPairs(first); !slices.Equal(got, want) {
		t.Fatalf("补充之后重放 = %v, want 首答次序 %v", got, want)
	}
}
