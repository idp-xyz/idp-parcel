package postgres_test

import (
	"context"
	"testing"
	"time"

	adapter "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
)

// 本文件对真实 PostgreSQL 16 证终局规则声明父行上那一格面单有效期（0026，ADR-0119）：写入经生产读口
// LoadFinalRule 读回不变形、缺席可分辨；重放 / 冲突判据把有效期算进去且原正文一行不动；库上 CHECK 镜像
// 领域构造门（同在同缺、时长为正、种类封闭）。
//
// 读口即验证缝：PS 的面单失效判断（票 ps-port-remainder/01）读的正是 LoadFinalRule 交回的 Validity()，
// 直插 SQL 只证明「列有值」，证明不了「消费方读得到那个时长」。

func finalRuleWithValidityOf(t *testing.T, owner domain.CommercialVersion, deliveryKind string, duration time.Duration) domain.FinalRuleContent {
	t.Helper()
	validity, err := domain.NewLabelValidityDeclaration(domain.ChannelResultObservedAnchor, duration)
	if err != nil {
		t.Fatalf("面单有效期：%v", err)
	}
	content, err := domain.NewFinalRuleContentWithValidity(owner, []domain.FinalizationDeclaration{
		{Outcome: domain.DeclaredEffectiveDelivery, FinalKind: pcValue(t, domain.NewRuleReference, deliveryKind)},
	}, validity)
	if err != nil {
		t.Fatalf("组带有效期的终局规则声明：%v", err)
	}
	return content
}

func TestFinalRuleValidityRoundTripsAndAbsenceStaysDistinguishable(t *testing.T) {
	repository, transactor, db := newDeclarationFixture(t)
	ctx := t.Context()
	tenant := pcTenant(t, "tenant-1")
	reader, err := adapter.NewStageContentDeclarations(db)
	if err != nil {
		t.Fatalf("构造读口：%v", err)
	}

	t.Run("带有效期的声明按时刻精度往返", func(t *testing.T) {
		rules := effectiveDeclarationOwner(t, domain.AcceptanceRulePackageObject, "rules-validity", "v1")
		mustSaveVersion(t, transactor, ctx, repository, rules)
		mustSaveDeclaration(t, transactor, func(txCtx context.Context) (ports.DeclarationSaveOutcome, error) {
			return repository.SaveFinalRule(txCtx, finalRuleWithValidityOf(t, rules, "FINAL/effective-delivery", 84*time.Hour+30*time.Minute))
		}, ports.DeclarationSaved)

		loaded, found, err := reader.LoadFinalRule(ctx, tenant, rules)
		if err != nil || !found {
			t.Fatalf("读回 found=%v err=%v", found, err)
		}
		validity, declared := loaded.Validity()
		if !declared || validity.Anchor() != domain.ChannelResultObservedAnchor || validity.Duration() != 84*time.Hour+30*time.Minute {
			t.Fatalf("有效期往返变形：%+v declared=%v", validity, declared)
		}
		if _, ok := loaded.FinalKindFor(domain.DeclaredEffectiveDelivery); !ok {
			t.Fatal("加了有效期之后终局规则行丢了")
		}
	})

	t.Run("没声明有效期读回就是没有", func(t *testing.T) {
		rules := effectiveDeclarationOwner(t, domain.AcceptanceRulePackageObject, "rules-bare", "v1")
		mustSaveVersion(t, transactor, ctx, repository, rules)
		mustSaveDeclaration(t, transactor, func(txCtx context.Context) (ports.DeclarationSaveOutcome, error) {
			return repository.SaveFinalRule(txCtx, finalRuleOf(t, rules, "FINAL/effective-delivery"))
		}, ports.DeclarationSaved)

		loaded, found, err := reader.LoadFinalRule(ctx, tenant, rules)
		if err != nil || !found {
			t.Fatalf("读回 found=%v err=%v", found, err)
		}
		if _, declared := loaded.Validity(); declared {
			t.Fatal("未声明的有效期被读成了已声明——终局规则其它行在场不等于有效期已配置")
		}
	})
}

func TestFinalRuleValidityEntersTheReplayAndConflictJudgement(t *testing.T) {
	repository, transactor, db := newDeclarationFixture(t)
	ctx := t.Context()
	tenant := pcTenant(t, "tenant-1")
	rules := effectiveDeclarationOwner(t, domain.AcceptanceRulePackageObject, "rules-1", "v1")
	mustSaveVersion(t, transactor, ctx, repository, rules)
	mustSaveDeclaration(t, transactor, func(txCtx context.Context) (ports.DeclarationSaveOutcome, error) {
		return repository.SaveFinalRule(txCtx, finalRuleWithValidityOf(t, rules, "FINAL/effective-delivery", 72*time.Hour))
	}, ports.DeclarationSaved)

	// 同行、同有效期：重放。
	mustSaveDeclaration(t, transactor, func(txCtx context.Context) (ports.DeclarationSaveOutcome, error) {
		return repository.SaveFinalRule(txCtx, finalRuleWithValidityOf(t, rules, "FINAL/effective-delivery", 72*time.Hour))
	}, ports.DeclarationAlreadyRegistered)
	// 同行、换时长：冲突。
	mustSaveDeclaration(t, transactor, func(txCtx context.Context) (ports.DeclarationSaveOutcome, error) {
		return repository.SaveFinalRule(txCtx, finalRuleWithValidityOf(t, rules, "FINAL/effective-delivery", 96*time.Hour))
	}, ports.DeclarationContentConflict)
	// 同行、去掉有效期：也是冲突——缺席是另一份正文，不是「少给一格无妨」。
	mustSaveDeclaration(t, transactor, func(txCtx context.Context) (ports.DeclarationSaveOutcome, error) {
		return repository.SaveFinalRule(txCtx, finalRuleOf(t, rules, "FINAL/effective-delivery"))
	}, ports.DeclarationContentConflict)

	reader, err := adapter.NewStageContentDeclarations(db)
	if err != nil {
		t.Fatalf("构造读口：%v", err)
	}
	loaded, found, err := reader.LoadFinalRule(ctx, tenant, rules)
	if err != nil || !found {
		t.Fatalf("读回 found=%v err=%v", found, err)
	}
	validity, declared := loaded.Validity()
	if !declared || validity.Duration() != 72*time.Hour {
		t.Fatalf("冲突写入改写了原有效期：%+v declared=%v", validity, declared)
	}
}

func TestFinalRuleValidityInvariantsAreMirroredInTheDatabase(t *testing.T) {
	repository, transactor, pool := newPublications(t)
	ctx := t.Context()
	rules := effectiveDeclarationOwner(t, domain.AcceptanceRulePackageObject, "rules-1", "v1")
	mustSaveVersion(t, transactor, ctx, repository, rules)

	rejected := map[string]string{
		"只有种类没有时长": `INSERT INTO party_commercial.final_rule_content
			(tenant_id, object_kind, object_id, version_label, validity_anchor)
			VALUES ('tenant-1', 4, 'rules-1', 'v1', 'CHANNEL_RESULT_OBSERVED')`,
		"只有时长没有种类": `INSERT INTO party_commercial.final_rule_content
			(tenant_id, object_kind, object_id, version_label, validity_duration)
			VALUES ('tenant-1', 4, 'rules-1', 'v1', interval '72 hours')`,
		"零时长": `INSERT INTO party_commercial.final_rule_content
			(tenant_id, object_kind, object_id, version_label, validity_anchor, validity_duration)
			VALUES ('tenant-1', 4, 'rules-1', 'v1', 'CHANNEL_RESULT_OBSERVED', interval '0')`,
		"负时长": `INSERT INTO party_commercial.final_rule_content
			(tenant_id, object_kind, object_id, version_label, validity_anchor, validity_duration)
			VALUES ('tenant-1', 4, 'rules-1', 'v1', 'CHANNEL_RESULT_OBSERVED', interval '-1 day')`,
		"起算时刻种类集外": `INSERT INTO party_commercial.final_rule_content
			(tenant_id, object_kind, object_id, version_label, validity_anchor, validity_duration)
			VALUES ('tenant-1', 4, 'rules-1', 'v1', 'LABEL_ISSUED', interval '72 hours')`,
	}
	for name, sql := range rejected {
		t.Run(name, func(t *testing.T) {
			if _, err := pool.Exec(ctx, sql); err == nil {
				t.Fatal("库放行了领域构造门会拒的一行")
			}
		})
	}

	// 两列皆空仍然合法——那是 0013 既有父行的形状，0026 不改它的语义。
	if _, err := pool.Exec(ctx,
		`INSERT INTO party_commercial.final_rule_content (tenant_id, object_kind, object_id, version_label)
		 VALUES ('tenant-1', 4, 'rules-1', 'v1')`,
	); err != nil {
		t.Fatalf("没有有效期的父行进不去了：%v", err)
	}
}
