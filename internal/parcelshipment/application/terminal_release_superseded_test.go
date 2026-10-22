package application_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

// 本文件证终局路径放的是这份委托已成立的占用，不只是当前版本那一笔（UC-PS-005 步骤 6；PS CONTEXT 生命周期「已提交
// 期间形成新提交版本」）：客户补件之后、新版本那一轮链提交之前委托就走到终局时，被取代那一版的占用再没有轮次去放。
// 委托摆在补件之后——当前版本 version-2、version-1 被取代，两版各占下一笔（controlByVersion）。

// releasedVersions 把释放替身收下的版本译成字面，便于与期望整列比对。
func releasedVersions(release *controlReleaseDouble) []string {
	got := make([]string, 0, len(release.versions))
	for _, version := range release.versions {
		got = append(got, version.String())
	}
	return got
}

func bothVersionsOccupied() map[string]domain.FinancialControlOutcome {
	return map[string]domain.FinancialControlOutcome{
		"version-1": domain.FinancialControlHeld,
		"version-2": domain.FinancialControlHeld,
	}
}

func supersededWithdrawal(t *testing.T) (*withdrawalFixture, application.WithdrawShipmentRequestCommand) {
	t.Helper()
	fixture := newWithdrawalFixture(t)
	superseded := supersededRequest(t)
	fixture.requests.stored = &superseded
	fixture.judgments.controlByVersion = bothVersionsOccupied()
	command := fixture.command(t)
	command.SubmissionVersion = mustValue(t, domain.NewSubmissionVersionID, "version-2")
	return fixture, command
}

// Covers: UC-PS-005 步骤 6「按原业务关联幂等释放已成立各项的占用」— 补件之后、新版本那一轮链提交之前撤回，两版的
// 占用都按各自的原关联释放：本版在前，被取代那一版随后。
func TestAWithdrawalReleasesEverySupersededVersionsOccupation(t *testing.T) {
	fixture, command := supersededWithdrawal(t)

	result, err := fixture.handler.Handle(context.Background(), command)
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result.Outcome() != application.WithdrawalFormed {
		t.Fatalf("outcome = %q, want FORMED", result.Outcome())
	}
	if got := releasedVersions(fixture.release); !slices.Equal(got, []string{"version-2", "version-1"}) {
		t.Fatalf("释放指名的版本 = %v, want [version-2 version-1]——被取代那一版那一笔成了孤儿", got)
	}
	if result.CompensationReference().String() != "" {
		t.Fatalf("各版都放掉了却留了补偿引用 %q", result.CompensationReference())
	}
}

// Covers: UC-PS-005 AT-PS-073、「撤回提交成功后不等待财务释放才生效」与步骤 6「失败时续办原补偿，不回滚撤回」— 被取代那一版放
// 不掉时撤回照样成立，补偿引用指名那一版那一笔；一版放不掉不挡住其余各版。
func TestAFailedSupersededReleaseKeepsTheWithdrawalAndLeavesCompensationPending(t *testing.T) {
	fixture, command := supersededWithdrawal(t)
	fixture.release.errFor = map[string]error{"version-1": errors.New("settlement-accounting unavailable")}

	result, err := fixture.handler.Handle(context.Background(), command)
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result.Outcome() != application.WithdrawalFormed || result.State() != domain.ShipmentRequestWithdrawn {
		t.Fatalf("outcome = %q state = %q——一次补偿失败改写了撤回", result.Outcome(), result.State())
	}
	if got := releasedVersions(fixture.release); !slices.Equal(got, []string{"version-2", "version-1"}) {
		t.Fatalf("释放指名的版本 = %v, want 两版都问过", got)
	}
	if result.CompensationReference().String() == "" {
		t.Fatal("被取代那一版没放掉却没有补偿引用")
	}

	// 当前版本那一笔先放不掉时，引用指名的是它——本版那一笔正是这次撤回在办的那一份；被取代那一版照样去放。
	current, command := supersededWithdrawal(t)
	current.release.errFor = map[string]error{"version-2": errors.New("settlement-accounting unavailable")}
	byCurrent, err := current.handler.Handle(context.Background(), command)
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if byCurrent.CompensationReference().String() == result.CompensationReference().String() {
		t.Fatal("两版各自放不掉派生出了同一个补偿引用——续办方分不出该重放哪一笔")
	}
	if got := releasedVersions(current.release); !slices.Equal(got, []string{"version-2", "version-1"}) {
		t.Fatalf("本版放不掉之后释放指名的版本 = %v, want 被取代那一版照样去放", got)
	}
}

// Covers: 撤回与拒绝不核对命令指名的版本——指名一版旧版本时，当前版本那一笔同样要放，否则它成了孤儿。
func TestAWithdrawalNamingASupersededVersionStillReleasesTheCurrentOne(t *testing.T) {
	fixture, command := supersededWithdrawal(t)
	command.SubmissionVersion = mustValue(t, domain.NewSubmissionVersionID, "version-1")

	if _, err := fixture.handler.Handle(context.Background(), command); err != nil {
		t.Fatalf("handle: %v", err)
	}
	if got := releasedVersions(fixture.release); !slices.Equal(got, []string{"version-1", "version-2"}) {
		t.Fatalf("释放指名的版本 = %v, want [version-1 version-2]，每版恰好一次", got)
	}
}

// Covers: 被取代那一版没占下什么（从未形成控制）就不发它那一版的释放。
func TestAWithdrawalSkipsASupersededVersionWithoutOccupation(t *testing.T) {
	fixture, command := supersededWithdrawal(t)
	fixture.judgments.controlByVersion["version-1"] = domain.FinancialControlOutcomeInvalid

	if _, err := fixture.handler.Handle(context.Background(), command); err != nil {
		t.Fatalf("handle: %v", err)
	}
	if got := releasedVersions(fixture.release); !slices.Equal(got, []string{"version-2"}) {
		t.Fatalf("释放指名的版本 = %v, want 只有 version-2", got)
	}
}

// Covers: AT-PS-068「重复撤回返回原撤回及原补偿关联」— 重放同一撤回时逐版再请求一次释放，提供方按幂等答原释放；
// 补偿引用与首次一致。
func TestARepeatedWithdrawalAsksForEverySupersededReleaseAgain(t *testing.T) {
	fixture, command := supersededWithdrawal(t)
	fixture.release.errFor = map[string]error{"version-1": errors.New("settlement-accounting unavailable")}
	original, err := fixture.handler.Handle(context.Background(), command)
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	fixture.requests.stored = fixture.requests.saved

	again, err := fixture.handler.Handle(context.Background(), command)
	if err != nil {
		t.Fatalf("again: %v", err)
	}
	if got := releasedVersions(fixture.release); !slices.Equal(got, []string{"version-2", "version-1", "version-2", "version-1"}) {
		t.Fatalf("释放指名的版本 = %v, want 两轮各放两版", got)
	}
	if again.CompensationReference().String() != original.CompensationReference().String() {
		t.Fatalf("重放 %q, 首次 %q——重复请求必须续办原补偿关联", again.CompensationReference(), original.CompensationReference())
	}
}

// Covers: PS CONTEXT「运营企业主动拒绝尚未接受的委托」与 AT-PS-035「接受提交未成立时按原关联释放已成立各项的占用」——
// 主动拒绝同此：两版的占用都放。
func TestAnActiveRejectionReleasesEverySupersededVersionsOccupation(t *testing.T) {
	fixture := newRejectionFixture(t)
	superseded := supersededRequest(t)
	fixture.requests.stored = &superseded
	fixture.judgments.controlByVersion = bothVersionsOccupied()
	command := fixture.command(t)
	command.SubmissionVersion = mustValue(t, domain.NewSubmissionVersionID, "version-2")
	fixture.release.errFor = map[string]error{"version-1": errors.New("settlement-accounting unavailable")}

	result, err := fixture.handler.Handle(context.Background(), command)
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result.Outcome() != application.ActiveRejectionFormed {
		t.Fatalf("outcome = %q, want FORMED", result.Outcome())
	}
	if got := releasedVersions(fixture.release); !slices.Equal(got, []string{"version-2", "version-1"}) {
		t.Fatalf("释放指名的版本 = %v, want [version-2 version-1]", got)
	}
	if result.CompensationReference().String() == "" {
		t.Fatal("被取代那一版没放掉却没有补偿引用")
	}
}

// Covers: AT-PS-035 与 UC-PS-001 步骤 8 的自动拒绝——规则形成的拒绝同样放这份委托各版的占用。
func TestAnAutomaticRejectionReleasesEverySupersededVersionsOccupation(t *testing.T) {
	fixture := newDecisionFixture(t)
	superseded := supersededRequest(t)
	fixture.requests.saved, fixture.requests.sticky = &superseded, true
	fixture.judgments.controlByVersion = bothVersionsOccupied()
	fixture.judgments.reachability["parcel-2"] = domain.ReachabilityUnreachable
	command := fixture.command(t)
	command.SubmissionVersion = mustValue(t, domain.NewSubmissionVersionID, "version-2")

	result, err := fixture.handler.Handle(context.Background(), command)
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result.State() != domain.ShipmentRequestRejected {
		t.Fatalf("state = %q, want REJECTED", result.State())
	}
	if got := releasedVersions(fixture.release); !slices.Equal(got, []string{"version-2", "version-1"}) {
		t.Fatalf("释放指名的版本 = %v, want [version-2 version-1]", got)
	}
}

// Covers: ADR-0132 决定一「两个去向都按原业务关联释放本版本已成立各项的占用」扩到这份委托的各版——处置面对补件之后
// 那一版时，被取代那一版若还占着也一并放；两个去向同此。
func TestAnAuthorizedDispositionReleasesEverySupersededVersionsOccupation(t *testing.T) {
	for name, choice := range map[string]domain.AuthorizedDispositionChoice{
		"拒绝":    domain.DisposeByRejection,
		"交客户补充": domain.DisposeByCustomerSupplement,
	} {
		t.Run(name, func(t *testing.T) {
			fixture := newDispositionFixture(t)
			fixture.requests.request = awaitingDispositionRequestOn(t, supersededRequest(t))
			// version-2 不列：落回夹具的「第一项冻结成立、第二项受限」那份（SAC-1）。
			fixture.judgments.controlByVersion = map[string]domain.FinancialControlOutcome{
				"version-1": domain.FinancialControlHeld,
			}
			command := fixture.command(t, choice)
			command.SubmissionVersion = mustValue(t, domain.NewSubmissionVersionID, "version-2")

			result, err := fixture.handler.Handle(context.Background(), command)
			if err != nil {
				t.Fatalf("handle: %v", err)
			}
			if result.Outcome() != application.AuthorizedDispositionRecorded {
				t.Fatalf("outcome = %q, want RECORDED", result.Outcome())
			}
			if got := releasedVersions(fixture.release); !slices.Equal(got, []string{"version-2", "version-1"}) {
				t.Fatalf("释放指名的版本 = %v, want [version-2 version-1]", got)
			}
		})
	}
}
