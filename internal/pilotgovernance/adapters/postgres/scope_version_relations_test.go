package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	adapter "go.idp.xyz/idp-parcel/internal/pilotgovernance/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/pilotgovernance/domain"
	"go.idp.xyz/idp-parcel/internal/pilotgovernance/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件对真实 PostgreSQL 16 证覆盖关系边库：往返重建、同有序对不可顶替、外键钉死
// 「无 Go/No-Go 决定就无关系登记」、种类封闭与自指边由 CHECK 拦下、写入必须在事务内。

var relationRegisteredAt = time.Date(2026, 8, 21, 14, 0, 0, 0, time.UTC)

type relationFixture struct {
	pool       *pgxpool.Pool
	sets       *adapter.CandidateSets
	reviews    *adapter.ReviewDecisions
	relations  *adapter.ScopeVersionRelations
	transactor bentoapp.Transactor
}

func newRelationFixture(t *testing.T) *relationFixture {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	sets, err := adapter.NewCandidateSets(db)
	if err != nil {
		t.Fatalf("构造候选组库：%v", err)
	}
	reviews, err := adapter.NewReviewDecisions(db)
	if err != nil {
		t.Fatalf("构造评审库：%v", err)
	}
	relations, err := adapter.NewScopeVersionRelations(db)
	if err != nil {
		t.Fatalf("构造关系边库：%v", err)
	}
	return &relationFixture{
		pool:       pool,
		sets:       sets,
		reviews:    reviews,
		relations:  relations,
		transactor: db.Transactor(),
	}
}

// seedOwningDecision 落一份候选组与评审决定，充当关系边的所属决定。
func (fixture *relationFixture) seedOwningDecision(t *testing.T, setID string) ports.ReviewKey {
	t.Helper()
	key := ports.ReviewKey{
		Objective:  "enter-limited-production",
		Candidates: govRef(t, domain.NewCandidateVersionSetID, setID),
	}
	if err := fixture.transactor.WithinTransaction(t.Context(), func(txCtx context.Context) error {
		if err := fixture.sets.Save(txCtx, candidateSet(t, setID)); err != nil {
			return err
		}
		_, err := fixture.reviews.Save(txCtx, key, goDecision(t, setID))
		return err
	}); err != nil {
		t.Fatalf("铺垫所属决定：%v", err)
	}
	return key
}

func ownedRelation(t *testing.T, key ports.ReviewKey, successor, predecessor string, kind domain.ScopeVersionRelationKind) domain.ScopeVersionRelation {
	t.Helper()
	relation, err := domain.RegisterScopeVersionRelation(domain.ScopeVersionRelationSpec{
		Successor:    govRef(t, domain.NewScopeVersionReference, successor),
		Predecessor:  govRef(t, domain.NewScopeVersionReference, predecessor),
		Kind:         kind,
		Objective:    key.Objective,
		Candidates:   key.Candidates,
		RegisteredAt: relationRegisteredAt,
	})
	if err != nil {
		t.Fatalf("形成关系边：%v", err)
	}
	return relation
}

func (fixture *relationFixture) save(t *testing.T, relation domain.ScopeVersionRelation) (ports.GovernanceSaveOutcome, error) {
	t.Helper()
	var outcome ports.GovernanceSaveOutcome
	err := fixture.transactor.WithinTransaction(t.Context(), func(txCtx context.Context) error {
		saved, err := fixture.relations.Save(txCtx, relation)
		outcome = saved
		return err
	})
	return outcome, err
}

func TestScopeRelationRoundTripsAndNeverOverwrites(t *testing.T) {
	fixture := newRelationFixture(t)
	ctx := t.Context()
	key := fixture.seedOwningDecision(t, "set-rel-1")

	first := ownedRelation(t, key, "pilot-scope/v4", "pilot-scope/v3", domain.ScopeInheritsSuspensions)
	if outcome, err := fixture.save(t, first); err != nil || outcome != ports.GovernanceSaved {
		t.Fatalf("首登：outcome=%d err=%v", outcome, err)
	}

	found, ok, err := fixture.relations.FindByPair(ctx,
		govRef(t, domain.NewScopeVersionReference, "pilot-scope/v4"),
		govRef(t, domain.NewScopeVersionReference, "pilot-scope/v3"))
	if err != nil || !ok {
		t.Fatalf("读回：ok=%v err=%v", ok, err)
	}
	if found.Kind() != domain.ScopeInheritsSuspensions ||
		found.Objective() != key.Objective ||
		found.Candidates() != key.Candidates ||
		!found.RegisteredAt().Equal(relationRegisteredAt) {
		t.Fatalf("往返失真：%+v", found)
	}

	// 同有序对换种类再登：原行不被顶替，答已有记录。
	flipped := ownedRelation(t, key, "pilot-scope/v4", "pilot-scope/v3", domain.ScopeUnrelated)
	if outcome, err := fixture.save(t, flipped); err != nil || outcome != ports.GovernanceAlreadyRecorded {
		t.Fatalf("重登：outcome=%d err=%v, 想要已有记录", outcome, err)
	}
	found, _, err = fixture.relations.FindByPair(ctx,
		govRef(t, domain.NewScopeVersionReference, "pilot-scope/v4"),
		govRef(t, domain.NewScopeVersionReference, "pilot-scope/v3"))
	if err != nil || found.Kind() != domain.ScopeInheritsSuspensions {
		t.Fatalf("原行被顶替了：kind=%s err=%v", found.Kind(), err)
	}

	// 反向对是另一行：有向身份，不与正向互斥。
	reverse := ownedRelation(t, key, "pilot-scope/v3", "pilot-scope/v4", domain.ScopeInheritsSuspensions)
	if outcome, err := fixture.save(t, reverse); err != nil || outcome != ports.GovernanceSaved {
		t.Fatalf("反向登：outcome=%d err=%v", outcome, err)
	}
}

func TestScopeRelationRequiresAnOwningDecision(t *testing.T) {
	fixture := newRelationFixture(t)

	orphan, err := domain.RegisterScopeVersionRelation(domain.ScopeVersionRelationSpec{
		Successor:    govRef(t, domain.NewScopeVersionReference, "pilot-scope/v4"),
		Predecessor:  govRef(t, domain.NewScopeVersionReference, "pilot-scope/v3"),
		Kind:         domain.ScopeInheritsSuspensions,
		Objective:    "decision-that-never-happened",
		Candidates:   govRef(t, domain.NewCandidateVersionSetID, "set-ghost"),
		RegisteredAt: relationRegisteredAt,
	})
	if err != nil {
		t.Fatalf("形成孤边：%v", err)
	}
	if _, err := fixture.save(t, orphan); err == nil {
		t.Fatal("没有所属决定的关系边被收下了——外键没起作用")
	}
}

func TestScopeRelationShapeChecksRejectForeignKindAndSelfEdge(t *testing.T) {
	fixture := newRelationFixture(t)
	ctx := t.Context()
	fixture.seedOwningDecision(t, "set-rel-2")

	if _, err := fixture.pool.Exec(ctx,
		`INSERT INTO pilot_governance.scope_version_relation
			(successor_scope, predecessor_scope, relation_kind, objective, candidate_set_id, registered_at)
		 VALUES ('pilot-scope/v4', 'pilot-scope/v3', 'SUPERSEDES', 'enter-limited-production', 'set-rel-2', now())`); err == nil {
		t.Fatal("集合外的种类被库接受了")
	}
	if _, err := fixture.pool.Exec(ctx,
		`INSERT INTO pilot_governance.scope_version_relation
			(successor_scope, predecessor_scope, relation_kind, objective, candidate_set_id, registered_at)
		 VALUES ('pilot-scope/v4', 'pilot-scope/v4', 'UNRELATED', 'enter-limited-production', 'set-rel-2', now())`); err == nil {
		t.Fatal("自指边被库接受了")
	}
}

func TestScopeRelationRequiresTransaction(t *testing.T) {
	fixture := newRelationFixture(t)
	key := fixture.seedOwningDecision(t, "set-rel-3")

	relation := ownedRelation(t, key, "pilot-scope/v4", "pilot-scope/v3", domain.ScopeUnrelated)
	if _, err := fixture.relations.Save(t.Context(), relation); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("无事务登记应拒，实得：%v", err)
	}
}
