package postgres_test

import (
	"context"
	"testing"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	adapter "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
)

// 本文件对真实 PostgreSQL 16 证资料修订允许声明（0027，ADR-0120）：写入经生产读口 LoadSourceDataAmendmentAllowance
// 读回不变形，三值（含父行「封闭」标记对缺格的读法）由读回的内容算出；重放 / 冲突判据含封闭标记与整份格集合且
// 原正文一行不动；库上 CHECK 镜像领域封闭集与构造门。
//
// 读口即验证缝：PS 的消费适配器（票 ps-port-remainder/02 余下一段）读的正是它交回的 AllowanceFor，直插 SQL 只证明
// 「行在」，证明不了「消费方读到的那一格是对的」。

func amendmentCell(t *testing.T, group string, stage domain.DeclaredAmendmentStage, intent domain.DeclaredAmendmentIntent, allowance domain.AmendmentAllowance) domain.SourceDataAmendmentRule {
	t.Helper()
	return domain.SourceDataAmendmentRule{
		DataGroup: pcValue(t, domain.NewSourceDataGroupReference, group),
		Stage:     stage,
		Intent:    intent,
		Allowance: allowance,
	}
}

func amendmentAllowanceOf(t *testing.T, owner domain.CommercialVersion, closed bool, rules ...domain.SourceDataAmendmentRule) domain.SourceDataAmendmentAllowanceContent {
	t.Helper()
	content, err := domain.NewSourceDataAmendmentAllowanceContent(owner, closed, rules)
	if err != nil {
		t.Fatalf("组资料修订允许声明：%v", err)
	}
	return content
}

func TestSourceDataAmendmentAllowanceRoundTripsThroughTheProductionReadPort(t *testing.T) {
	repository, transactor, db := newDeclarationFixture(t)
	ctx := t.Context()
	tenant := pcTenant(t, "tenant-1")
	reader, err := adapter.NewStageContentDeclarations(db)
	if err != nil {
		t.Fatalf("构造读口：%v", err)
	}
	address := pcValue(t, domain.NewSourceDataGroupReference, "consignee.address")
	weight := pcValue(t, domain.NewSourceDataGroupReference, "parcel.weight")

	t.Run("未封闭的声明逐格往返，缺格读未声明", func(t *testing.T) {
		rules := effectiveDeclarationOwner(t, domain.AcceptanceRulePackageObject, "rules-open", "v1")
		mustSaveVersion(t, transactor, ctx, repository, rules)
		mustSaveDeclaration(t, transactor, func(txCtx context.Context) (ports.DeclarationSaveOutcome, error) {
			return repository.SaveSourceDataAmendmentAllowance(txCtx, amendmentAllowanceOf(t, rules, false,
				amendmentCell(t, "consignee.address", domain.DeclaredAcceptedNotYetReceived, domain.DeclaredCorrectionIntent, domain.AmendmentAllowed),
				amendmentCell(t, "consignee.address", domain.DeclaredAcceptedNotYetReceived, domain.DeclaredExplicitClearIntent, domain.AmendmentDisallowed),
				amendmentCell(t, "consignee.address", domain.DeclaredCustomsSubmitted, domain.DeclaredCorrectionIntent, domain.AmendmentDisallowed),
			))
		}, ports.DeclarationSaved)

		loaded, found, err := reader.LoadSourceDataAmendmentAllowance(ctx, tenant, rules)
		if err != nil || !found {
			t.Fatalf("读回 found=%v err=%v", found, err)
		}
		if loaded.Closed() || len(loaded.Rules()) != 3 || !loaded.Owner().SameVersionAs(rules) {
			t.Fatalf("声明变形：closed=%v rules=%d", loaded.Closed(), len(loaded.Rules()))
		}
		for name, tc := range map[string]struct {
			group  domain.SourceDataGroupReference
			stage  domain.DeclaredAmendmentStage
			intent domain.DeclaredAmendmentIntent
			want   domain.AmendmentAllowance
		}{
			"允许的格":      {address, domain.DeclaredAcceptedNotYetReceived, domain.DeclaredCorrectionIntent, domain.AmendmentAllowed},
			"同阶段清空不允许":  {address, domain.DeclaredAcceptedNotYetReceived, domain.DeclaredExplicitClearIntent, domain.AmendmentDisallowed},
			"提交关务后不允许":  {address, domain.DeclaredCustomsSubmitted, domain.DeclaredCorrectionIntent, domain.AmendmentDisallowed},
			"没登的资料组未声明": {weight, domain.DeclaredAcceptedNotYetReceived, domain.DeclaredCorrectionIntent, domain.AmendmentAllowanceNotDeclared},
			"没登的阶段未声明":  {address, domain.DeclaredReceivedOrMeasured, domain.DeclaredCorrectionIntent, domain.AmendmentAllowanceNotDeclared},
		} {
			if got := loaded.AllowanceFor(tc.group, tc.stage, tc.intent); got != tc.want {
				t.Errorf("%s：AllowanceFor = %v, want %v", name, got, tc.want)
			}
		}
	})

	t.Run("封闭且零格是一句显式的话，读回缺格一律不允许", func(t *testing.T) {
		rules := effectiveDeclarationOwner(t, domain.AcceptanceRulePackageObject, "rules-closed-empty", "v1")
		mustSaveVersion(t, transactor, ctx, repository, rules)
		mustSaveDeclaration(t, transactor, func(txCtx context.Context) (ports.DeclarationSaveOutcome, error) {
			return repository.SaveSourceDataAmendmentAllowance(txCtx, amendmentAllowanceOf(t, rules, true))
		}, ports.DeclarationSaved)

		loaded, found, err := reader.LoadSourceDataAmendmentAllowance(ctx, tenant, rules)
		if err != nil || !found {
			t.Fatalf("读回 found=%v err=%v", found, err)
		}
		if !loaded.Closed() || len(loaded.Rules()) != 0 {
			t.Fatalf("声明变形：closed=%v rules=%d", loaded.Closed(), len(loaded.Rules()))
		}
		if got := loaded.AllowanceFor(address, domain.DeclaredAcceptedNotYetReceived, domain.DeclaredSupplementIntent); got != domain.AmendmentDisallowed {
			t.Fatalf("封闭零格读回缺格 = %v, want DISALLOWED", got)
		}
	})

	t.Run("封闭带格：登了的按格答，没登的不允许", func(t *testing.T) {
		rules := effectiveDeclarationOwner(t, domain.AcceptanceRulePackageObject, "rules-closed", "v1")
		mustSaveVersion(t, transactor, ctx, repository, rules)
		mustSaveDeclaration(t, transactor, func(txCtx context.Context) (ports.DeclarationSaveOutcome, error) {
			return repository.SaveSourceDataAmendmentAllowance(txCtx, amendmentAllowanceOf(t, rules, true,
				amendmentCell(t, "parcel.weight", domain.DeclaredReceivedOrMeasured, domain.DeclaredCorrectionIntent, domain.AmendmentAllowed),
			))
		}, ports.DeclarationSaved)

		loaded, found, err := reader.LoadSourceDataAmendmentAllowance(ctx, tenant, rules)
		if err != nil || !found {
			t.Fatalf("读回 found=%v err=%v", found, err)
		}
		if got := loaded.AllowanceFor(weight, domain.DeclaredReceivedOrMeasured, domain.DeclaredCorrectionIntent); got != domain.AmendmentAllowed {
			t.Fatalf("登了的格 = %v, want ALLOWED", got)
		}
		if got := loaded.AllowanceFor(weight, domain.DeclaredCustomsSubmitted, domain.DeclaredCorrectionIntent); got != domain.AmendmentDisallowed {
			t.Fatalf("封闭下没登的格 = %v, want DISALLOWED", got)
		}
	})

	t.Run("没登记就是未配置，不是错", func(t *testing.T) {
		rules := effectiveDeclarationOwner(t, domain.AcceptanceRulePackageObject, "rules-none", "v1")
		mustSaveVersion(t, transactor, ctx, repository, rules)
		if _, found, err := reader.LoadSourceDataAmendmentAllowance(ctx, tenant, rules); err != nil || found {
			t.Fatalf("未登记读回 found=%v err=%v", found, err)
		}
	})

	t.Run("显式租户与拥有版本不是同一身份即拒", func(t *testing.T) {
		rules := effectiveDeclarationOwner(t, domain.AcceptanceRulePackageObject, "rules-open", "v1")
		if _, _, err := reader.LoadSourceDataAmendmentAllowance(ctx, pcTenant(t, "tenant-2"), rules); err == nil {
			t.Fatal("别的租户拿着这份版本读到了声明")
		}
	})
}

func TestSourceDataAmendmentAllowanceEntersTheReplayAndConflictJudgement(t *testing.T) {
	repository, transactor, db := newDeclarationFixture(t)
	ctx := t.Context()
	tenant := pcTenant(t, "tenant-1")
	rules := effectiveDeclarationOwner(t, domain.AcceptanceRulePackageObject, "rules-1", "v1")
	mustSaveVersion(t, transactor, ctx, repository, rules)

	allowed := amendmentCell(t, "consignee.address", domain.DeclaredAcceptedNotYetReceived, domain.DeclaredCorrectionIntent, domain.AmendmentAllowed)
	disallowed := amendmentCell(t, "consignee.address", domain.DeclaredCustomsSubmitted, domain.DeclaredCorrectionIntent, domain.AmendmentDisallowed)
	mustSaveDeclaration(t, transactor, func(txCtx context.Context) (ports.DeclarationSaveOutcome, error) {
		return repository.SaveSourceDataAmendmentAllowance(txCtx, amendmentAllowanceOf(t, rules, false, allowed, disallowed))
	}, ports.DeclarationSaved)

	// 同封闭标记、同一份格：重放。格的给出顺序不算正文的一部分。
	mustSaveDeclaration(t, transactor, func(txCtx context.Context) (ports.DeclarationSaveOutcome, error) {
		return repository.SaveSourceDataAmendmentAllowance(txCtx, amendmentAllowanceOf(t, rules, false, disallowed, allowed))
	}, ports.DeclarationAlreadyRegistered)
	// 同一份格、翻封闭标记：冲突——缺格的读法变了，是另一份正文。
	mustSaveDeclaration(t, transactor, func(txCtx context.Context) (ports.DeclarationSaveOutcome, error) {
		return repository.SaveSourceDataAmendmentAllowance(txCtx, amendmentAllowanceOf(t, rules, true, allowed, disallowed))
	}, ports.DeclarationContentConflict)
	// 多一格：冲突。
	mustSaveDeclaration(t, transactor, func(txCtx context.Context) (ports.DeclarationSaveOutcome, error) {
		return repository.SaveSourceDataAmendmentAllowance(txCtx, amendmentAllowanceOf(t, rules, false, allowed, disallowed,
			amendmentCell(t, "parcel.weight", domain.DeclaredReceivedOrMeasured, domain.DeclaredSupplementIntent, domain.AmendmentAllowed)))
	}, ports.DeclarationContentConflict)
	// 少一格：冲突——不是「少给一格无妨」。
	mustSaveDeclaration(t, transactor, func(txCtx context.Context) (ports.DeclarationSaveOutcome, error) {
		return repository.SaveSourceDataAmendmentAllowance(txCtx, amendmentAllowanceOf(t, rules, false, allowed))
	}, ports.DeclarationContentConflict)
	// 同格换值：冲突。
	mustSaveDeclaration(t, transactor, func(txCtx context.Context) (ports.DeclarationSaveOutcome, error) {
		flipped := amendmentCell(t, "consignee.address", domain.DeclaredCustomsSubmitted, domain.DeclaredCorrectionIntent, domain.AmendmentAllowed)
		return repository.SaveSourceDataAmendmentAllowance(txCtx, amendmentAllowanceOf(t, rules, false, allowed, flipped))
	}, ports.DeclarationContentConflict)

	reader, err := adapter.NewStageContentDeclarations(db)
	if err != nil {
		t.Fatalf("构造读口：%v", err)
	}
	loaded, found, err := reader.LoadSourceDataAmendmentAllowance(ctx, tenant, rules)
	if err != nil || !found {
		t.Fatalf("读回 found=%v err=%v", found, err)
	}
	if loaded.Closed() || len(loaded.Rules()) != 2 {
		t.Fatalf("冲突写入改写了原正文：closed=%v rules=%d", loaded.Closed(), len(loaded.Rules()))
	}
	if got := loaded.AllowanceFor(disallowed.DataGroup, disallowed.Stage, disallowed.Intent); got != domain.AmendmentDisallowed {
		t.Fatalf("冲突写入改写了原格：%v", got)
	}
}

func TestSourceDataAmendmentAllowanceInvariantsAreMirroredInTheDatabase(t *testing.T) {
	repository, transactor, pool := newPublications(t)
	ctx := t.Context()
	rules := effectiveDeclarationOwner(t, domain.AcceptanceRulePackageObject, "rules-1", "v1")
	mustSaveVersion(t, transactor, ctx, repository, rules)

	if _, err := pool.Exec(ctx,
		`INSERT INTO party_commercial.source_data_amendment_content (tenant_id, object_kind, object_id, version_label, closed)
		 VALUES ('tenant-1', 4, 'rules-1', 'v1', false)`,
	); err != nil {
		t.Fatalf("父行进不去：%v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO party_commercial.source_data_amendment_allowance
			(tenant_id, object_kind, object_id, version_label, data_group_ref, stage, intent, allowance)
		 VALUES ('tenant-1', 4, 'rules-1', 'v1', 'consignee.address', 'ACCEPTED_NOT_YET_RECEIVED', 'CORRECTION', 'ALLOWED')`,
	); err != nil {
		t.Fatalf("合法的一格进不去：%v", err)
	}

	rejected := map[string]string{
		"父行挂授权规则": `INSERT INTO party_commercial.source_data_amendment_content
			(tenant_id, object_kind, object_id, version_label, closed)
			VALUES ('tenant-1', 9, 'auth-1', 'v1', false)`,
		"阶段集外": `INSERT INTO party_commercial.source_data_amendment_allowance
			(tenant_id, object_kind, object_id, version_label, data_group_ref, stage, intent, allowance)
			VALUES ('tenant-1', 4, 'rules-1', 'v1', 'consignee.address', 'IN_TRANSIT', 'CORRECTION', 'ALLOWED')`,
		"意图集外": `INSERT INTO party_commercial.source_data_amendment_allowance
			(tenant_id, object_kind, object_id, version_label, data_group_ref, stage, intent, allowance)
			VALUES ('tenant-1', 4, 'rules-1', 'v1', 'consignee.address', 'ACCEPTED_NOT_YET_RECEIVED', 'REVOKE', 'ALLOWED')`,
		"未声明登成一格": `INSERT INTO party_commercial.source_data_amendment_allowance
			(tenant_id, object_kind, object_id, version_label, data_group_ref, stage, intent, allowance)
			VALUES ('tenant-1', 4, 'rules-1', 'v1', 'consignee.address', 'RECEIVED_OR_MEASURED', 'CORRECTION', 'NOT_DECLARED')`,
		"资料组为空": `INSERT INTO party_commercial.source_data_amendment_allowance
			(tenant_id, object_kind, object_id, version_label, data_group_ref, stage, intent, allowance)
			VALUES ('tenant-1', 4, 'rules-1', 'v1', '  ', 'RECEIVED_OR_MEASURED', 'CORRECTION', 'ALLOWED')`,
		"同一格两行": `INSERT INTO party_commercial.source_data_amendment_allowance
			(tenant_id, object_kind, object_id, version_label, data_group_ref, stage, intent, allowance)
			VALUES ('tenant-1', 4, 'rules-1', 'v1', 'consignee.address', 'ACCEPTED_NOT_YET_RECEIVED', 'CORRECTION', 'DISALLOWED')`,
		"没有父行的格": `INSERT INTO party_commercial.source_data_amendment_allowance
			(tenant_id, object_kind, object_id, version_label, data_group_ref, stage, intent, allowance)
			VALUES ('tenant-1', 4, 'rules-orphan', 'v1', 'consignee.address', 'ACCEPTED_NOT_YET_RECEIVED', 'CORRECTION', 'ALLOWED')`,
	}
	for name, sql := range rejected {
		t.Run(name, func(t *testing.T) {
			if _, err := pool.Exec(ctx, sql); err == nil {
				t.Fatal("库放行了领域构造门会拒的一行")
			}
		})
	}
}

// Covers: 0027 头注「SQL 表达不了未封闭至少一格」——直插一个未封闭的零格父行，库放行，但生产读口必须报坏声明
// 而不是折成未配置：折成未配置会让消费方去催一份其实已经写坏的配置。
func TestAnOpenParentRowWithoutCellsIsABrokenDeclarationNotAnAbsentOne(t *testing.T) {
	// 读口与直插必须共用同一个池：pgtest.Pool 每次调用都建一个全新的库。
	repository, transactor, pool := newPublications(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	ctx := t.Context()
	rules := effectiveDeclarationOwner(t, domain.AcceptanceRulePackageObject, "rules-1", "v1")
	mustSaveVersion(t, transactor, ctx, repository, rules)
	if _, err := pool.Exec(ctx,
		`INSERT INTO party_commercial.source_data_amendment_content (tenant_id, object_kind, object_id, version_label, closed)
		 VALUES ('tenant-1', 4, 'rules-1', 'v1', false)`,
	); err != nil {
		t.Fatalf("父行进不去：%v", err)
	}
	reader, err := adapter.NewStageContentDeclarations(db)
	if err != nil {
		t.Fatalf("构造读口：%v", err)
	}
	_, found, err := reader.LoadSourceDataAmendmentAllowance(ctx, pcTenant(t, "tenant-1"), rules)
	if err == nil || found {
		t.Fatalf("未封闭零格父行读回 found=%v err=%v，want 坏声明 error", found, err)
	}
}
