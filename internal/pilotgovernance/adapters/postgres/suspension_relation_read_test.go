package postgres_test

import (
	"context"
	"testing"
	"time"

	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	adapter "go.idp.xyz/idp-parcel/internal/pilotgovernance/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/pilotgovernance/domain"
	"go.idp.xyz/idp-parcel/internal/pilotgovernance/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件对真实 PostgreSQL 16 证准入查询消费覆盖关系边之后的答复进化：登了承继边按
// 承继拦（命中仍优先）、登了互不相干（任一方向）那条暂停不及于所问版本但对它写明的
// 范围照旧拦着（不是静默恢复）、什么关系都没登第三态保守作答照旧、关系不外溢到
// 未被声明的版本。

var relationReadAt = time.Date(2026, 8, 21, 16, 0, 0, 0, time.UTC)

type relationReadFixture struct {
	suspensions *adapter.Suspensions
	resumptions *adapter.Resumptions
	sets        *adapter.CandidateSets
	reviews     *adapter.ReviewDecisions
	relations   *adapter.ScopeVersionRelations
	transactor  bentoapp.Transactor
	decision    ports.ReviewKey
}

func newRelationReadFixture(t *testing.T) *relationReadFixture {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	fixture := &relationReadFixture{transactor: db.Transactor()}
	if fixture.suspensions, err = adapter.NewSuspensions(db); err != nil {
		t.Fatalf("构造暂停库：%v", err)
	}
	if fixture.resumptions, err = adapter.NewResumptions(db); err != nil {
		t.Fatalf("构造恢复库：%v", err)
	}
	if fixture.sets, err = adapter.NewCandidateSets(db); err != nil {
		t.Fatalf("构造候选组库：%v", err)
	}
	if fixture.reviews, err = adapter.NewReviewDecisions(db); err != nil {
		t.Fatalf("构造评审库：%v", err)
	}
	if fixture.relations, err = adapter.NewScopeVersionRelations(db); err != nil {
		t.Fatalf("构造关系边库：%v", err)
	}

	// 关系边的所属决定：一份就够，全部边都挂它名下。
	fixture.decision = ports.ReviewKey{
		Objective:  "enter-limited-production",
		Candidates: govRef(t, domain.NewCandidateVersionSetID, "set-read-1"),
	}
	fixture.inTx(t, func(txCtx context.Context) error {
		if err := fixture.sets.Save(txCtx, candidateSet(t, "set-read-1")); err != nil {
			return err
		}
		_, err := fixture.reviews.Save(txCtx, fixture.decision, goDecision(t, "set-read-1"))
		return err
	})
	return fixture
}

func (fixture *relationReadFixture) inTx(t *testing.T, fn func(context.Context) error) {
	t.Helper()
	if err := fixture.transactor.WithinTransaction(t.Context(), fn); err != nil {
		t.Fatalf("事务内写入失败：%v", err)
	}
}

func (fixture *relationReadFixture) suspend(t *testing.T, id, scope string, effectiveAt time.Time) {
	t.Helper()
	fixture.inTx(t, func(txCtx context.Context) error {
		_, err := fixture.suspensions.Save(txCtx, scopedSuspension(t, id, scope, effectiveAt))
		return err
	})
}

func (fixture *relationReadFixture) relate(t *testing.T, successor, predecessor string, kind domain.ScopeVersionRelationKind) {
	t.Helper()
	relation, err := domain.RegisterScopeVersionRelation(domain.ScopeVersionRelationSpec{
		Successor:    govRef(t, domain.NewScopeVersionReference, successor),
		Predecessor:  govRef(t, domain.NewScopeVersionReference, predecessor),
		Kind:         kind,
		Objective:    fixture.decision.Objective,
		Candidates:   fixture.decision.Candidates,
		RegisteredAt: relationReadAt,
	})
	if err != nil {
		t.Fatalf("形成关系边：%v", err)
	}
	fixture.inTx(t, func(txCtx context.Context) error {
		_, err := fixture.relations.Save(txCtx, relation)
		return err
	})
}

func (fixture *relationReadFixture) ask(t *testing.T, scope string, at time.Time) (domain.SuspensionDecision, domain.AdmissionSuspensionGround) {
	t.Helper()
	decision, ground, err := fixture.suspensions.FindUnresumedSuspension(
		t.Context(), govRef(t, domain.NewScopeVersionReference, scope), at)
	if err != nil {
		t.Fatalf("取尚未恢复的暂停：%v", err)
	}
	return decision, ground
}

// Covers: 登了「后继承继前代」的边之后，前代那条未恢复的暂停按承继格拦住后继；写明
// 后继自身的暂停仍优先交回（命中 > 承继）；承继不外溢到没登边的版本（照旧保守）。
func TestAnInheritedSuspensionBlocksTheSuccessorScope(t *testing.T) {
	fixture := newRelationReadFixture(t)
	suspendedAt := relationReadAt.Add(time.Hour)
	askedAt := relationReadAt.Add(6 * time.Hour)

	fixture.suspend(t, "suspend-v1", "pilot-scope/v1", suspendedAt)
	fixture.relate(t, "pilot-scope/v2", "pilot-scope/v1", domain.ScopeInheritsSuspensions)

	found, ground := fixture.ask(t, "pilot-scope/v2", askedAt)
	if ground != domain.AdmissionSuspendedByInheritedScope {
		t.Fatalf("ground = %s, want %s", ground, domain.AdmissionSuspendedByInheritedScope)
	}
	if found.ID().String() != "suspend-v1" {
		t.Fatalf("交回 = %q, want suspend-v1（被承继的那条才是该引的证据）", found.ID().String())
	}

	// 没登边的版本照旧保守：承继是被登记的事实，不是可推广的猜测。
	if _, ground := fixture.ask(t, "pilot-scope/v9", askedAt); ground != domain.AdmissionSuspendedByUnreadableScopeRelation {
		t.Fatalf("未声明版本 ground = %s, want %s", ground, domain.AdmissionSuspendedByUnreadableScopeRelation)
	}

	// 写明后继自身的暂停压过承继来的。
	fixture.suspend(t, "suspend-v2", "pilot-scope/v2", suspendedAt.Add(time.Hour))
	found, ground = fixture.ask(t, "pilot-scope/v2", askedAt)
	if ground != domain.AdmissionSuspendedByNamedScope || found.ID().String() != "suspend-v2" {
		t.Fatalf("命中未压过承继：ground=%s id=%s", ground, found.ID().String())
	}
}

// Covers: 登了互不相干（无论从哪个方向登）之后，前代的暂停不再以保守格拦住所问版本
// ——第三态从「恒成立」变成「关系确实登不出来时才成立」；但暂停对它写明的范围照旧
// 拦着，解除它仍只走恢复决定，这不是静默恢复。
func TestAnUnrelatedEdgeLiftsOnlyTheConservativeAnswer(t *testing.T) {
	fixture := newRelationReadFixture(t)
	suspendedAt := relationReadAt.Add(time.Hour)
	askedAt := relationReadAt.Add(6 * time.Hour)

	fixture.suspend(t, "suspend-v1", "pilot-scope/v1", suspendedAt)
	fixture.relate(t, "pilot-scope/v2", "pilot-scope/v1", domain.ScopeUnrelated)

	if _, ground := fixture.ask(t, "pilot-scope/v2", askedAt); ground != domain.AdmissionNotSuspended {
		t.Fatalf("正向互不相干后 ground = %s, want %s", ground, domain.AdmissionNotSuspended)
	}

	// 暂停对写明的范围照旧拦着——互不相干隔开的是别的版本，不解除暂停本身。
	if found, ground := fixture.ask(t, "pilot-scope/v1", askedAt); ground != domain.AdmissionSuspendedByNamedScope ||
		found.ID().String() != "suspend-v1" {
		t.Fatalf("前代自身被误开放：ground=%s id=%s", ground, found.ID().String())
	}

	// 对称事实：从暂停一侧登的边同样隔开反方向的提问。独立夹具——上面那条 v1 的暂停
	// 与 v6 的关系没登过，留在同一册里会以保守格正确地拦住 v6，考不出对称这一格。
	symmetric := newRelationReadFixture(t)
	symmetric.suspend(t, "suspend-v5", "pilot-scope/v5", suspendedAt)
	symmetric.relate(t, "pilot-scope/v5", "pilot-scope/v6", domain.ScopeUnrelated)
	if _, ground := symmetric.ask(t, "pilot-scope/v6", askedAt); ground != domain.AdmissionNotSuspended {
		t.Fatalf("反向互不相干后 ground = %s, want %s", ground, domain.AdmissionNotSuspended)
	}
}

// Covers: 关系按暂停逐条起作用——同时立着两条暂停时，隔开其中一条不影响另一条照旧
// 保守拦住；把另一条也用承继边接上后按承继拦。
func TestRelationsApplyPerSuspensionNotPerRegister(t *testing.T) {
	fixture := newRelationReadFixture(t)
	suspendedAt := relationReadAt.Add(time.Hour)
	askedAt := relationReadAt.Add(6 * time.Hour)

	fixture.suspend(t, "suspend-v1", "pilot-scope/v1", suspendedAt)
	fixture.suspend(t, "suspend-v7", "pilot-scope/v7", suspendedAt.Add(time.Minute))
	fixture.relate(t, "pilot-scope/v2", "pilot-scope/v1", domain.ScopeUnrelated)

	// v1 被隔开了，但 v7 的暂停与 v2 的关系仍读不出来——保守格交回 v7 那条。
	found, ground := fixture.ask(t, "pilot-scope/v2", askedAt)
	if ground != domain.AdmissionSuspendedByUnreadableScopeRelation || found.ID().String() != "suspend-v7" {
		t.Fatalf("逐条语义失守：ground=%s id=%s, want 保守格交回 suspend-v7", ground, found.ID().String())
	}

	fixture.relate(t, "pilot-scope/v2", "pilot-scope/v7", domain.ScopeInheritsSuspensions)
	found, ground = fixture.ask(t, "pilot-scope/v2", askedAt)
	if ground != domain.AdmissionSuspendedByInheritedScope || found.ID().String() != "suspend-v7" {
		t.Fatalf("承继边未生效：ground=%s id=%s", ground, found.ID().String())
	}
}
