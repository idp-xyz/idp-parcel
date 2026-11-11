package postgres_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	adapter "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件对真实 PostgreSQL 16 证受理前补充换平台箱标号的两件事（ADR-0376 决定二，票 declared-goods-lines/10）：歧义信号跟着委托
// 此刻的声明走——补报的号算、换下的号不再算；换下的号同一笔交声明撤除门铃，一个号先被换下、再被报回、最后随拒绝撤下，两次都响。

// supplementLabels 在一份已提交的委托上形成一份补充：带平台货件与 labels（成员 → 箱标号）；labels 为空即这一份补充不带平台段。
func supplementLabels(t *testing.T, fixture externalIdentifierFixture, identity domain.SourceIdentity, version string, at time.Duration, labels map[string]string) {
	t.Helper()
	ctx := t.Context()
	loaded, found, err := fixture.requests.FindBySourceIdentity(ctx, identity)
	if err != nil || !found {
		t.Fatalf("读回：found = %v err = %v", found, err)
	}
	spec := domain.NewSubmissionVersionSpec{
		VersionID:         mustBuild(t, domain.NewSubmissionVersionID, version),
		TaskID:            mustBuild(t, domain.NewAcceptanceDecisionTaskID, "task-"+version),
		SourceSubmission:  requestFingerprint(t, "supplement-"+version, "digest-"+version),
		DeclaredParcelIDs: loaded.CurrentSubmissionVersion().DeclaredParcelIDs(),
		EstablishedAt:     submittedAtFixture.Add(at),
	}
	if len(labels) > 0 {
		shipment, err := domain.DeclarePlatformShipment("AMAZON", mustBuild(t, domain.NewExternalIdentifierNumber, "FBA15-SUPP"), "")
		if err != nil {
			t.Fatalf("平台货件：%v", err)
		}
		spec.PlatformShipment = shipment
		for parcel, number := range labels {
			label, err := domain.DeclarePlatformBoxLabel(mustBuild(t, domain.NewDeclaredParcelID, parcel),
				mustBuild(t, domain.NewExternalIdentifierNumber, number))
			if err != nil {
				t.Fatalf("箱标号：%v", err)
			}
			spec.PlatformBoxLabels = append(spec.PlatformBoxLabels, label)
		}
	}
	superseded, err := loaded.FormNewSubmissionVersion(spec)
	if err != nil {
		t.Fatalf("形成补充版本 %s：%v", version, err)
	}
	mustSave(t, fixture.transactor, ctx, fixture.requests, identity, superseded)
}

// Covers: ADR-0376 决定二——补充把 parcel-b1 的箱标号由 OLD 换成 NEW、保留 B2：按 NEW 核对带歧义信号，按 OLD 不再带；换下 OLD
// 那一笔交一封撤除门铃，信封 ID 带补充版本。再一份补充不带平台段：NEW 与 B2 都撤下、各响一封。第三份补充把 OLD 报回来，委托随后被
// 拒绝：OLD 再响一封——与补充换下那一封 ID 不同，不被当成重放吞掉。
func TestASupplementThatSwapsBoxLabelsMovesTheSignalAndRingsTheRetiredDoorbell(t *testing.T) {
	fixture := newExternalIdentifierFixture(t)
	ctx := t.Context()
	identity := requestScopedIdentity(t, "tenant-1", "customer-2", "req-key-b")
	mustInsert(t, fixture.transactor, ctx, fixture.requests, labelledRequest(t, "tenant-1", "customer-2", "req-key-b", "request-b",
		map[string]string{"parcel-b1": "FBA15SYN-OLD", "parcel-b2": "FBA15SYN-B2"}))
	if _, signalled := fixture.answerOf(t, "tenant-1", "FBA15SYN-OLD"); !signalled {
		t.Fatal("首次提交声明的箱标号不算歧义信号")
	}

	supplementLabels(t, fixture, identity, "version-2", 10*time.Minute, map[string]string{"parcel-b1": "FBA15SYN-NEW", "parcel-b2": "FBA15SYN-B2"})
	if _, signalled := fixture.answerOf(t, "tenant-1", "FBA15SYN-NEW"); !signalled {
		t.Fatal("补充补报的箱标号不算歧义信号")
	}
	if _, signalled := fixture.answerOf(t, "tenant-1", "FBA15SYN-OLD"); signalled {
		t.Fatal("补充换下的箱标号仍算歧义信号")
	}
	bells := retiredDoorbellsOf(t, fixture.pool)
	if got := retiredNumbers(bells); !slices.Equal(got, []string{"FBA15SYN-OLD"}) {
		t.Fatalf("换下一个号后门铃 = %v, want 只有 FBA15SYN-OLD", got)
	}
	if !strings.HasPrefix(bells[0].eventID, "customer-parcel-number-declaration-retired/") {
		t.Fatalf("信封 ID = %q, want 同一种门铃的口名前缀", bells[0].eventID)
	}

	supplementLabels(t, fixture, identity, "version-3", 20*time.Minute, nil)
	if got := retiredNumbers(retiredDoorbellsOf(t, fixture.pool)); !slices.Equal(got, []string{"FBA15SYN-B2", "FBA15SYN-NEW", "FBA15SYN-OLD"}) {
		t.Fatalf("不带平台段的补充之后门铃 = %v, want 再多 B2 与 NEW", got)
	}
	if _, signalled := fixture.answerOf(t, "tenant-1", "FBA15SYN-B2"); signalled {
		t.Fatal("补充撤下全部平台声明后 B2 仍算歧义信号")
	}

	supplementLabels(t, fixture, identity, "version-4", 30*time.Minute, map[string]string{"parcel-b1": "FBA15SYN-OLD"})
	loaded, _, err := fixture.requests.FindBySourceIdentity(ctx, identity)
	if err != nil {
		t.Fatal(err)
	}
	rejected, err := rejectByContract(t)(loaded)
	if err != nil {
		t.Fatalf("拒绝：%v", err)
	}
	mustSave(t, fixture.transactor, ctx, fixture.requests, identity, rejected)
	old := 0
	for _, number := range retiredNumbers(retiredDoorbellsOf(t, fixture.pool)) {
		if number == "FBA15SYN-OLD" {
			old++
		}
	}
	if old != 2 {
		t.Fatalf("FBA15SYN-OLD 的门铃 %d 封, want 2：补充换下一次、报回后随拒绝撤下一次", old)
	}
}

// Covers: 同一份补充保存不改箱标号时不响——补充只换了别的内容、平台声明照旧整份重报，不是撤下。
func TestASupplementThatKeepsTheBoxLabelsRingsNothing(t *testing.T) {
	fixture := newExternalIdentifierFixture(t)
	identity := requestScopedIdentity(t, "tenant-1", "customer-2", "req-key-k")
	mustInsert(t, fixture.transactor, t.Context(), fixture.requests, labelledRequest(t, "tenant-1", "customer-2", "req-key-k", "request-k",
		map[string]string{"parcel-k1": "FBA15SYN-K1"}))
	supplementLabels(t, fixture, identity, "version-2", 10*time.Minute, map[string]string{"parcel-k1": "FBA15SYN-K1"})
	if bells := retiredDoorbellsOf(t, fixture.pool); len(bells) != 0 {
		t.Fatalf("箱标号没变的补充交了 %d 封门铃", len(bells))
	}
}

// Covers: PBC-08 负向证据——补充换下箱标号的那一封门铃与补充同生共死，没有事务就拒，不在自动提交下单独入队。
func TestTheWithdrawnBoxLabelDoorbellRefusesToRingOutsideATransaction(t *testing.T) {
	db, err := bentopg.NewDB(pgtest.Pool(t), bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	doorbell, err := adapter.NewExternalIdentifierDoorbell(db)
	if err != nil {
		t.Fatalf("构造门铃：%v", err)
	}
	request := labelledRequest(t, "tenant-1", "customer-1", "req-key-1", "request-1", map[string]string{"parcel-1": "FBA15SYN-TX"})
	numbers := []domain.ExternalIdentifierNumber{request.PlatformBoxLabels()[0].Number()}
	err = doorbell.RingBoxLabelsWithdrawn(context.Background(), mustBuild(t, domain.NewTenantID, "tenant-1"), request, numbers, time.Now())
	if !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("无事务响门铃 err = %v, want ErrTransactionRequired", err)
	}
}
