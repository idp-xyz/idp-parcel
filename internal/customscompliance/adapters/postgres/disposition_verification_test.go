package postgres_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	adapter "go.idp.xyz/idp-parcel/internal/customscompliance/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

var verificationBaseAt = time.Date(2026, 8, 14, 17, 0, 0, 0, time.UTC)

type verificationFixture struct {
	store      *adapter.DispositionVerifications
	transactor bentoapp.Transactor
	pool       *pgxpool.Pool
}

func newVerificationFixture(t *testing.T) *verificationFixture {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	store, err := adapter.NewDispositionVerifications(db)
	if err != nil {
		t.Fatalf("构造核对库：%v", err)
	}
	return &verificationFixture{store: store, transactor: db.Transactor(), pool: pool}
}

func (fixture *verificationFixture) inTx(t *testing.T, ctx context.Context, fn func(context.Context) error) {
	t.Helper()
	if err := fixture.transactor.WithinTransaction(ctx, fn); err != nil {
		t.Fatalf("事务内写入失败：%v", err)
	}
}

func verificationValue[T any](t *testing.T, construct func(string) (T, error), raw string) T {
	t.Helper()
	built, err := construct(raw)
	if err != nil {
		t.Fatalf("构造 %q：%v", raw, err)
	}
	return built
}

func verificationDecision(t *testing.T) domain.RegulatoryDecision {
	t.Helper()
	decision, err := domain.NewRegulatoryDecision(domain.RegulatoryDecisionSpec{
		ID:         verificationValue(t, domain.NewRegulatoryDecisionID, "decision-1"),
		Authority:  verificationValue(t, domain.NewRegulatoryAuthorityReference, "CUSTOMS/US-CBP"),
		Action:     verificationValue(t, domain.NewLegalActionReference, "DESTRUCTION"),
		Scope:      verificationValue(t, domain.NewDecisionScopeReference, "parcel-1"),
		Quantity:   domain.RequiredQuantity{Provided: true, Units: 2},
		ReceivedAt: verificationBaseAt,
	})
	if err != nil {
		t.Fatalf("构造决定：%v", err)
	}
	return decision
}

func verificationFact(t *testing.T, reference string, units int) domain.ExecutionFact {
	t.Helper()
	fact, err := domain.NewExecutionFact(domain.ExecutionFactSpec{
		Executor:   verificationValue(t, domain.NewExecutorReference, "node-origin"),
		Fact:       verificationValue(t, domain.NewExecutionFactReference, reference),
		Scope:      verificationValue(t, domain.NewDecisionScopeReference, "parcel-1"),
		Units:      units,
		OccurredAt: verificationBaseAt.Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("构造执行事实：%v", err)
	}
	return fact
}

func verificationKey(t *testing.T, tenant string, facts []domain.ExecutionFact) ports.VerificationKey {
	t.Helper()
	references := make([]string, 0, len(facts))
	for _, fact := range facts {
		references = append(references, fact.Fact().String())
	}
	sort.Strings(references)
	digest := sha256.Sum256([]byte(strings.Join(references, "\x00")))
	return ports.VerificationKey{
		TenantID: verificationValue(t, domain.NewTenantID, tenant),
		Decision: verificationValue(t, domain.NewRegulatoryDecisionID, "decision-1"),
		Digest:   hex.EncodeToString(digest[:]),
	}
}

func TestDispositionVerificationRoundTripsAndSameDigestIsAlreadyRecorded(t *testing.T) {
	fixture := newVerificationFixture(t)
	ctx := t.Context()
	facts := []domain.ExecutionFact{
		verificationFact(t, "DESTRUCTION-EXEC/1", 1),
		verificationFact(t, "DESTRUCTION-EXEC/2", 1),
	}
	verification, err := domain.VerifyDispositionExecution(verificationDecision(t), facts, verificationBaseAt.Add(2*time.Hour))
	if err != nil {
		t.Fatalf("核对：%v", err)
	}
	key := verificationKey(t, "tenant-a", facts)

	var first, second ports.VerificationSaveOutcome
	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		var err error
		first, err = fixture.store.Save(txCtx, key, verification)
		if err != nil {
			return err
		}
		second, err = fixture.store.Save(txCtx, key, verification)
		return err
	})
	if first != ports.VerificationSaved || second != ports.VerificationAlreadyRecorded {
		t.Fatalf("outcome 首次=%d 二次=%d", first, second)
	}

	loaded, found, err := fixture.store.FindByKey(ctx, key)
	if err != nil || !found || loaded.Conclusion() != domain.ExecutionCovered || loaded.ExecutedUnits() != 2 {
		t.Fatalf("往返：err=%v found=%v conclusion=%s units=%d",
			err, found, loaded.Conclusion(), loaded.ExecutedUnits())
	}
}

func TestNewFactSetDigestCreatesANewVerificationVersion(t *testing.T) {
	fixture := newVerificationFixture(t)
	ctx := t.Context()
	coveredFacts := []domain.ExecutionFact{verificationFact(t, "DESTRUCTION-EXEC/1", 1)}
	covered, err := domain.VerifyDispositionExecution(verificationDecision(t), coveredFacts, verificationBaseAt.Add(2*time.Hour))
	if err != nil {
		t.Fatalf("部分覆盖：%v", err)
	}
	allFacts := []domain.ExecutionFact{
		verificationFact(t, "DESTRUCTION-EXEC/1", 1),
		verificationFact(t, "DESTRUCTION-EXEC/2", 1),
	}
	full, err := domain.VerifyDispositionExecution(verificationDecision(t), allFacts, verificationBaseAt.Add(3*time.Hour))
	if err != nil {
		t.Fatalf("已覆盖：%v", err)
	}

	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		if _, err := fixture.store.Save(txCtx, verificationKey(t, "tenant-a", coveredFacts), covered); err != nil {
			return err
		}
		_, err := fixture.store.Save(txCtx, verificationKey(t, "tenant-a", allFacts), full)
		return err
	})

	partial, found, err := fixture.store.FindByKey(ctx, verificationKey(t, "tenant-a", coveredFacts))
	if err != nil || !found || partial.Conclusion() != domain.ExecutionPartiallyCovered {
		t.Fatalf("原判断被覆盖：err=%v found=%v conclusion=%s", err, found, partial.Conclusion())
	}
	complete, found, err := fixture.store.FindByKey(ctx, verificationKey(t, "tenant-a", allFacts))
	if err != nil || !found || complete.Conclusion() != domain.ExecutionCovered {
		t.Fatalf("新指纹没立住：err=%v found=%v conclusion=%s", err, found, complete.Conclusion())
	}
}

func TestDispositionVerificationChecksRejectUnknownConclusion(t *testing.T) {
	fixture := newVerificationFixture(t)
	if _, err := fixture.pool.Exec(t.Context(),
		`INSERT INTO customs_compliance.disposition_verification
			(tenant_id, decision_id, facts_digest, authority_ref, action_ref, scope_ref,
			 quantity_provided, quantity_units, received_at, facts, conclusion,
			 executed_units, verified_at)
		 VALUES ('t', 'd', 'dig', 'a', 'act', 's', false, 0, now(), '[]', 'OBLIGATION_ENDED', 0, now())`); err == nil {
		t.Fatal("封闭集合外的核对结论被库接受了")
	}
}

func TestDispositionVerificationsOfAnotherTenantAreInvisible(t *testing.T) {
	fixture := newVerificationFixture(t)
	ctx := t.Context()
	facts := []domain.ExecutionFact{verificationFact(t, "DESTRUCTION-EXEC/1", 2)}
	verification, err := domain.VerifyDispositionExecution(verificationDecision(t), facts, verificationBaseAt.Add(2*time.Hour))
	if err != nil {
		t.Fatalf("核对：%v", err)
	}
	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		_, err := fixture.store.Save(txCtx, verificationKey(t, "tenant-a", facts), verification)
		return err
	})
	if _, found, err := fixture.store.FindByKey(ctx, verificationKey(t, "tenant-b", facts)); err != nil || found {
		t.Fatalf("跨租户可见：err=%v found=%v", err, found)
	}
}

func TestDispositionVerificationWritesRequireTransaction(t *testing.T) {
	fixture := newVerificationFixture(t)
	facts := []domain.ExecutionFact{verificationFact(t, "DESTRUCTION-EXEC/1", 2)}
	verification, err := domain.VerifyDispositionExecution(verificationDecision(t), facts, verificationBaseAt.Add(2*time.Hour))
	if err != nil {
		t.Fatalf("核对：%v", err)
	}
	if _, err := fixture.store.Save(t.Context(), verificationKey(t, "tenant-a", facts), verification); err == nil {
		t.Fatal("无事务 Save 被接受了")
	}
}
