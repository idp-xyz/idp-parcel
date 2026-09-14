package postgres_test

import (
	"context"
	"testing"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
)

// 本文件对真实 PostgreSQL 16 证评价的回指一格（票 sa-cc/11 裁决 2）：带回指的评价按（租户、回指）读得回、
// 快照与列交叉核过；同租户同回指第二份撞迁移 0010 的部分唯一索引答`已有记录`、先到者不被顶替；别的租户按
// 同一回指读不到（ADR-0003）。

// referencedFixture 真算一份带回指的评价：夹具不手搓评价，回指经领域门带上。
func referencedFixture(t *testing.T, id, tenant, zone, reference string) domain.PricingEvaluation {
	t.Helper()
	request, err := domain.NewEvaluationRequest(evaluationValue(t, domain.NewEvaluationID, id), syntheticPlan(t), syntheticInput(t, tenant, zone), domain.EvidenceSynthetic)
	if err != nil {
		t.Fatalf("评价请求：%v", err)
	}
	referenced, err := request.WithRequestReference(evaluationValue(t, domain.NewEvaluationRequestReference, reference))
	if err != nil {
		t.Fatalf("带回指：%v", err)
	}
	return domain.EvaluatePricing(referenced)
}

func TestEvaluationIsReadBackByItsRequestReference(t *testing.T) {
	repository, transactor := newEvaluations(t)
	ctx := t.Context()

	evaluation := referencedFixture(t, "eval-by-request", "tenant-a", "Z1", "EVREQ-SYN-1")
	inEvaluationTx(t, transactor, ctx, func(txCtx context.Context) error {
		_, err := repository.Save(txCtx, evaluation)
		return err
	})

	tenant := evaluationValue(t, domain.NewTenantID, "tenant-a")
	reference := evaluationValue(t, domain.NewEvaluationRequestReference, "EVREQ-SYN-1")
	found, exists, err := repository.FindByRequestReference(ctx, tenant, reference)
	if err != nil || !exists {
		t.Fatalf("按回指读回：%v exists=%v", err, exists)
	}
	if snapshotOf(t, found) != snapshotOf(t, evaluation) {
		t.Fatalf("按回指读回的快照与写入不同答")
	}
	if got, has := found.RequestReference(); !has || got != reference {
		t.Fatalf("读回的回指 = %q present=%v", got, has)
	}

	// 按标识那条读法读同一行，回指同样在——两条读法共用一套比对列。
	byID, exists, err := repository.FindByID(ctx, evaluation.ID())
	if err != nil || !exists {
		t.Fatalf("按标识读回：%v exists=%v", err, exists)
	}
	if got, has := byID.RequestReference(); !has || got != reference {
		t.Fatalf("按标识读回的回指 = %q present=%v", got, has)
	}

	// 别的租户按同一回指读不到；没带回指的评价按任何回指都读不到。
	other := evaluationValue(t, domain.NewTenantID, "tenant-b")
	if _, exists, err := repository.FindByRequestReference(ctx, other, reference); err != nil || exists {
		t.Fatalf("别的租户读到了：err=%v exists=%v", err, exists)
	}
	if _, exists, err := repository.FindByRequestReference(ctx, tenant, evaluationValue(t, domain.NewEvaluationRequestReference, "EVREQ-SYN-none")); err != nil || exists {
		t.Fatalf("不存在的回指读到了：err=%v exists=%v", err, exists)
	}
}

// TestSecondEvaluationForTheSameRequestIsAlreadyRecorded 证「同一请求不形成第二份评价」在库上成立：另一个评价
// 标识带同一回指，Save 答`已有记录`，先到者原样。
func TestSecondEvaluationForTheSameRequestIsAlreadyRecorded(t *testing.T) {
	repository, transactor := newEvaluations(t)
	ctx := t.Context()

	first := referencedFixture(t, "eval-req-first", "tenant-a", "Z1", "EVREQ-SYN-2")
	inEvaluationTx(t, transactor, ctx, func(txCtx context.Context) error {
		_, err := repository.Save(txCtx, first)
		return err
	})

	second := referencedFixture(t, "eval-req-second", "tenant-a", "Z9", "EVREQ-SYN-2")
	var outcome ports.EvaluationSaveOutcome
	inEvaluationTx(t, transactor, ctx, func(txCtx context.Context) error {
		saved, err := repository.Save(txCtx, second)
		outcome = saved
		return err
	})
	if outcome != ports.EvaluationAlreadyRecorded {
		t.Fatalf("第二份 outcome = %d, 想要 EvaluationAlreadyRecorded", outcome)
	}

	winner, exists, err := repository.FindByRequestReference(ctx,
		evaluationValue(t, domain.NewTenantID, "tenant-a"),
		evaluationValue(t, domain.NewEvaluationRequestReference, "EVREQ-SYN-2"))
	if err != nil || !exists {
		t.Fatalf("读回先到者：%v exists=%v", err, exists)
	}
	if winner.ID() != first.ID() {
		t.Fatalf("先到者被顶替：读回 %s", winner.ID())
	}
	if _, exists, err := repository.FindByID(ctx, second.ID()); err != nil || exists {
		t.Fatalf("第二份不该落库：err=%v exists=%v", err, exists)
	}

	// 别的租户带同一回指是另一份请求（回指只在租户内唯一），照常落库。
	elsewhere := referencedFixture(t, "eval-req-elsewhere", "tenant-b", "Z1", "EVREQ-SYN-2")
	inEvaluationTx(t, transactor, ctx, func(txCtx context.Context) error {
		saved, err := repository.Save(txCtx, elsewhere)
		outcome = saved
		return err
	})
	if outcome != ports.EvaluationSaved {
		t.Fatalf("别的租户 outcome = %d, 想要 EvaluationSaved", outcome)
	}
}

// TestEvaluationWithoutRequestReferenceLeavesTheColumnNull 证没带回指的评价照旧：列为空、按标识读回回指缺席——
// 迁移 0010 之前的读法一格不变。
func TestEvaluationWithoutRequestReferenceLeavesTheColumnNull(t *testing.T) {
	repository, transactor := newEvaluations(t)
	ctx := t.Context()

	plain := evaluatedFixture(t, "eval-plain", "tenant-a", "Z1")
	inEvaluationTx(t, transactor, ctx, func(txCtx context.Context) error {
		_, err := repository.Save(txCtx, plain)
		return err
	})
	found, exists, err := repository.FindByID(ctx, plain.ID())
	if err != nil || !exists {
		t.Fatalf("读回：%v exists=%v", err, exists)
	}
	if _, has := found.RequestReference(); has {
		t.Fatalf("没带回指的评价读回竟有回指")
	}
}
