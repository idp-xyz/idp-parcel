package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	adapter "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件对真实 PostgreSQL 16 证六族声明表的**非测试写入路径**（syn-wall-door-audit
// 票 03）：写入经各自的生产读口读回不变形；同拥有版本同正文是重放、异正文是冲突且
// 原正文一行不动；写入拒绝在事务之外运行。
//
// 读口即验证缝：写进去的东西必须能被 resolve/接受链实际消费的那个口读出来，直插 SQL
// 验证只能证明「行在」，证明不了「行能用」。

// newDeclarationFixture 与 newPublications 同形，但把 db 一并交出：读口与写口必须
// 共用同一个池——pgtest.Pool 每次调用都建一个全新的库，各建各的就各看各的。
func newDeclarationFixture(t *testing.T) (*adapter.CommercialPublications, bentoapp.Transactor, *bentopg.DB) {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	repository, err := adapter.NewCommercialPublications(db)
	if err != nil {
		t.Fatalf("构造发布登记册：%v", err)
	}
	return repository, db.Transactor(), db
}

// effectiveDeclarationOwner 造一份不带指名引用的已生效版本，充当声明拥有对象
// （服务产品/授权规则）。合同拥有的声明用 effectiveContract：正文件的规则包引用要与
// 版本壳上的指名引用保持一致（open-decisions F-3），那个夹具的壳上指名 rules-1。
func effectiveDeclarationOwner(
	t *testing.T,
	kind domain.CommercialObjectKind,
	objectID, label string,
) domain.CommercialVersion {
	t.Helper()
	interval, err := domain.NewEffectiveInterval(effectiveAtRow, effectiveAtRow.Add(90*24*time.Hour))
	if err != nil {
		t.Fatalf("有效区间：%v", err)
	}
	approval, err := domain.NewApprovalBasis(
		pcValue(t, domain.NewApprovalReference, "approval-"+objectID),
		pcValue(t, domain.NewCommercialSourceReference, "source-"+objectID),
		approvedAtFixture,
	)
	if err != nil {
		t.Fatalf("批准依据：%v", err)
	}
	version, err := domain.RehydrateCommercialVersion(domain.RehydrateCommercialVersionSpec{
		TenantID:      pcTenant(t, "tenant-1"),
		Kind:          kind,
		ObjectID:      pcValue(t, domain.NewCommercialObjectID, objectID),
		Version:       pcValue(t, domain.NewCommercialVersionLabel, label),
		Scope:         pcScope(t),
		ContentDigest: pcValue(t, domain.NewCommercialContentDigest, "digest-"+objectID+"-"+label),
		Effective:     interval,
		Status:        domain.CommercialVersionEffective,
		Approval:      approval,
		PublishedAt:   publishedAtRow,
		EffectiveAt:   effectiveAtRow,
	})
	if err != nil {
		t.Fatalf("重建 %s 版本：%v", kind, err)
	}
	return version
}

func mustSaveDeclaration(
	t *testing.T,
	transactor bentoapp.Transactor,
	save func(context.Context) (ports.DeclarationSaveOutcome, error),
	want ports.DeclarationSaveOutcome,
) {
	t.Helper()
	mustWithinPublicationTransaction(t, transactor, t.Context(), func(txCtx context.Context) error {
		outcome, err := save(txCtx)
		if err != nil {
			return err
		}
		if outcome != want {
			t.Fatalf("save outcome = %s, want %s", outcome, want)
		}
		return nil
	})
}

func asOfDeclarationOf(t *testing.T, owner domain.CommercialVersion, reachabilityPolicy string) domain.AsOfDeclaration {
	t.Helper()
	policies := []domain.AsOfPolicy{
		pcAsOfPolicy(t, domain.NetworkReachabilityJudgment, "AT_ACCEPTANCE", reachabilityPolicy),
		pcAsOfPolicy(t, domain.PreAcceptanceFinancialControlJudgment, "AT_SUBMISSION", "asof-policy/ctrl-v1"),
	}
	declaration, err := domain.DeclareAsOfPolicies(owner, policies)
	if err != nil {
		t.Fatalf("组时点锚声明：%v", err)
	}
	return declaration
}

func pcAsOfPolicy(t *testing.T, judgment domain.JudgmentType, semantics, version string) domain.AsOfPolicy {
	t.Helper()
	policy, err := domain.NewAsOfPolicy(judgment,
		pcValue(t, domain.NewAsOfSemanticsReference, semantics),
		pcValue(t, domain.NewAsOfPolicyVersion, version))
	if err != nil {
		t.Fatalf("时点锚：%v", err)
	}
	return policy
}

func acceptanceContentOf(t *testing.T, owner domain.CommercialVersion, review domain.ManualReviewDirective) domain.AcceptanceRuleContent {
	t.Helper()
	content, err := domain.DeclareAcceptanceRuleContent(owner, []domain.AcceptanceCheckGroupType{
		domain.CustomerRelationshipCheckGroup,
		domain.RequiredDocumentCheckGroup,
	}, review)
	if err != nil {
		t.Fatalf("组接受内容声明：%v", err)
	}
	return content
}

func intakeQualificationOf(t *testing.T, owner domain.CommercialVersion, sources ...domain.DeclaredIntakeSource) domain.IntakeQualificationContent {
	t.Helper()
	content, err := domain.NewIntakeQualificationContent(owner, sources,
		[]domain.RuleReference{pcValue(t, domain.NewRuleReference, "INTAKE-QUAL/customs-precheck")})
	if err != nil {
		t.Fatalf("组收寄资格声明：%v", err)
	}
	return content
}

func finalRuleOf(t *testing.T, owner domain.CommercialVersion, deliveryKind string) domain.FinalRuleContent {
	t.Helper()
	content, err := domain.NewFinalRuleContent(owner, []domain.FinalizationDeclaration{
		{Outcome: domain.DeclaredEffectiveDelivery, FinalKind: pcValue(t, domain.NewRuleReference, deliveryKind)},
		{Outcome: domain.DeclaredReturnCompleted, FinalKind: pcValue(t, domain.NewRuleReference, "FINAL/return")},
	})
	if err != nil {
		t.Fatalf("组终局规则声明：%v", err)
	}
	return content
}

func cancellationAuthorityOf(t *testing.T, owner domain.CommercialVersion, customerRule string) domain.CancellationAuthorityContent {
	t.Helper()
	content, err := domain.NewCancellationAuthorityContent(owner, []domain.CancellationAuthorityDeclaration{
		{Party: domain.DeclaredCustomerCancellation, Rule: pcValue(t, domain.NewRuleReference, customerRule)},
	})
	if err != nil {
		t.Fatalf("组取消授权目录：%v", err)
	}
	return content
}

func rulePackageBodyOf(t *testing.T, owner domain.CommercialVersion, rules ...domain.AssembledRule) domain.AcceptanceRulePackage {
	t.Helper()
	interval, err := domain.NewEffectiveInterval(effectiveAtRow, effectiveAtRow.Add(90*24*time.Hour))
	if err != nil {
		t.Fatalf("适用期间：%v", err)
	}
	applicability, err := domain.NewRulePackageApplicability(
		pcValue(t, domain.NewCommercialObjectID, "product-1"),
		pcValue(t, domain.NewCommercialObjectID, "contract-1"),
		pcValue(t, domain.NewLegalEntityReference, "legal-1"),
		pcScope(t),
		interval,
	)
	if err != nil {
		t.Fatalf("五维适用性：%v", err)
	}
	pack, err := domain.NewAcceptanceRulePackage(owner, applicability, rules)
	if err != nil {
		t.Fatalf("组规则包正文：%v", err)
	}
	return pack
}

func pcAssembledRule(t *testing.T, category domain.RuleCategory, reference string) domain.AssembledRule {
	t.Helper()
	rule, err := domain.NewAssembledRule(category, pcValue(t, domain.NewRuleReference, reference))
	if err != nil {
		t.Fatalf("装配规则：%v", err)
	}
	return rule
}

func contractContentOf(t *testing.T, contract domain.CommercialVersion, controlPolicy string) domain.CustomerContract {
	t.Helper()
	applied, err := domain.NewAppliedFinancialControl(
		pcValue(t, domain.NewChargeScopeReference, "charge-scope-1"),
		pcValue(t, domain.NewCommercialObjectID, controlPolicy))
	if err != nil {
		t.Fatalf("适用控制约定：%v", err)
	}
	inapplicable, err := domain.NewInapplicableFinancialControl(
		pcValue(t, domain.NewChargeScopeReference, "charge-scope-2"),
		pcValue(t, domain.NewInapplicabilityBasis, "CONTRACT-CLAUSE/NO-CONTROL"))
	if err != nil {
		t.Fatalf("不适用控制约定：%v", err)
	}
	content, err := domain.NewCustomerContract(contract,
		pcValue(t, domain.NewCommercialObjectID, "rules-1"),
		[]domain.FinancialControlBinding{applied, inapplicable})
	if err != nil {
		t.Fatalf("组合同正文：%v", err)
	}
	return content
}

func preAcceptanceControlOf(t *testing.T, contract domain.CommercialVersion) domain.PreAcceptanceControlDeclaration {
	t.Helper()
	declaration, err := domain.DeclarePreAcceptanceControl(contract,
		domain.PreAcceptanceControlRequired, domain.ControlNotApplicableBasis{})
	if err != nil {
		t.Fatalf("组接受前控制声明：%v", err)
	}
	return declaration
}

func pendingRoutingOf(t *testing.T, product domain.CommercialVersion, basis string) domain.PendingRoutingPermission {
	t.Helper()
	permission, err := domain.DeclarePendingRoutingPermission(product,
		pcValue(t, domain.NewPendingRoutingBasisReference, basis))
	if err != nil {
		t.Fatalf("组待路由许可：%v", err)
	}
	return permission
}

// TestPublishedDeclarationsRoundTripThroughTheirReadPorts 证九个写入口各自把声明送进
// 对应生产读口的视野：save 后按同一拥有版本读回，内容逐项不变形。
func TestPublishedDeclarationsRoundTripThroughTheirReadPorts(t *testing.T) {
	repository, transactor, db := newDeclarationFixture(t)
	ctx := t.Context()
	tenant := pcTenant(t, "tenant-1")

	rules := effectiveRulePackage(t, "rules-1", "v1")
	product := effectiveDeclarationOwner(t, domain.ServiceProductObject, "product-1", "v1")
	contract := effectiveContract(t, "contract-1", "v1", "digest-c1")
	authorization := effectiveDeclarationOwner(t, domain.AuthorizationRuleObject, "authz-1", "v1")

	t.Run("时点锚声明", func(t *testing.T) {
		declaration := asOfDeclarationOf(t, rules, "asof-policy/reach-v1")
		mustSaveDeclaration(t, transactor, func(txCtx context.Context) (ports.DeclarationSaveOutcome, error) {
			return repository.SaveAsOfPolicies(txCtx, declaration)
		}, ports.DeclarationSaved)

		reader, err := adapter.NewAsOfPolicyDeclarations(db)
		if err != nil {
			t.Fatalf("构造读口：%v", err)
		}
		policies, err := reader.LoadAsOfPolicies(ctx, tenant, rules)
		if err != nil {
			t.Fatalf("读回：%v", err)
		}
		loaded, err := domain.DeclareAsOfPolicies(rules, policies)
		if err != nil {
			t.Fatalf("读回的声明领域收不下：%v", err)
		}
		policy, found := loaded.PolicyFor(domain.NetworkReachabilityJudgment)
		if !found || policy.PolicyVersion().String() != "asof-policy/reach-v1" {
			t.Fatalf("读回的时点锚变形：%#v found=%v", policy, found)
		}
	})

	t.Run("接受内容声明", func(t *testing.T) {
		content := acceptanceContentOf(t, rules, domain.ManualReviewRequired)
		mustSaveDeclaration(t, transactor, func(txCtx context.Context) (ports.DeclarationSaveOutcome, error) {
			return repository.SaveAcceptanceRuleContent(txCtx, content)
		}, ports.DeclarationSaved)

		reader, err := adapter.NewAcceptanceContentDeclarations(db)
		if err != nil {
			t.Fatalf("构造读口：%v", err)
		}
		loaded, found, err := reader.LoadAcceptanceRuleContent(ctx, tenant, rules)
		if err != nil || !found {
			t.Fatalf("读回 found=%v err=%v", found, err)
		}
		if !loaded.Applies(domain.RequiredDocumentCheckGroup) || loaded.ManualReview() != domain.ManualReviewRequired {
			t.Fatalf("读回的接受内容变形：%#v", loaded)
		}
	})

	t.Run("待路由许可", func(t *testing.T) {
		permission := pendingRoutingOf(t, product, "PRODUCT-CLAUSE/PENDING-OK")
		mustSaveDeclaration(t, transactor, func(txCtx context.Context) (ports.DeclarationSaveOutcome, error) {
			return repository.SavePendingRoutingPermission(txCtx, permission)
		}, ports.DeclarationSaved)

		reader, err := adapter.NewAcceptanceContentDeclarations(db)
		if err != nil {
			t.Fatalf("构造读口：%v", err)
		}
		loaded, found, err := reader.LoadPendingRoutingPermission(ctx, tenant, product)
		if err != nil || !found {
			t.Fatalf("读回 found=%v err=%v", found, err)
		}
		if !loaded.Allowed() || loaded.Basis().String() != "PRODUCT-CLAUSE/PENDING-OK" {
			t.Fatalf("读回的许可变形：%#v", loaded)
		}
	})

	t.Run("接受前控制声明", func(t *testing.T) {
		declaration := preAcceptanceControlOf(t, contract)
		mustSaveDeclaration(t, transactor, func(txCtx context.Context) (ports.DeclarationSaveOutcome, error) {
			return repository.SavePreAcceptanceControl(txCtx, declaration)
		}, ports.DeclarationSaved)

		reader, err := adapter.NewPreAcceptanceControlDeclarations(db)
		if err != nil {
			t.Fatalf("构造读口：%v", err)
		}
		loaded, found, err := reader.LoadPreAcceptanceControl(ctx, tenant, contract)
		if err != nil || !found {
			t.Fatalf("读回 found=%v err=%v", found, err)
		}
		if loaded.Requirement() != domain.PreAcceptanceControlRequired {
			t.Fatalf("读回的控制要求变形：%q", loaded.Requirement())
		}
	})

	t.Run("合同正文", func(t *testing.T) {
		content := contractContentOf(t, contract, "control-policy-1")
		mustSaveDeclaration(t, transactor, func(txCtx context.Context) (ports.DeclarationSaveOutcome, error) {
			return repository.SaveCustomerContractContent(txCtx, content)
		}, ports.DeclarationSaved)

		reader, err := adapter.NewCustomerContractContents(db)
		if err != nil {
			t.Fatalf("构造读口：%v", err)
		}
		loaded, found, err := reader.LoadCustomerContract(ctx, tenant, contract)
		if err != nil || !found {
			t.Fatalf("读回 found=%v err=%v", found, err)
		}
		if loaded.AcceptanceRulePackage().String() != "rules-1" {
			t.Fatalf("读回的规则包引用变形：%q", loaded.AcceptanceRulePackage())
		}
		binding, bound := loaded.FinancialControlFor(pcValue(t, domain.NewChargeScopeReference, "charge-scope-2"))
		if !bound || !binding.ExplicitlyInapplicable() {
			t.Fatalf("显式不适用的约定没有随正文往返：%#v bound=%v", binding, bound)
		}
	})

	t.Run("收寄资格声明", func(t *testing.T) {
		content := intakeQualificationOf(t, rules, domain.DeclaredNodeIntake, domain.DeclaredOffsitePickup)
		mustSaveDeclaration(t, transactor, func(txCtx context.Context) (ports.DeclarationSaveOutcome, error) {
			return repository.SaveIntakeQualification(txCtx, content)
		}, ports.DeclarationSaved)

		reader, err := adapter.NewStageContentDeclarations(db)
		if err != nil {
			t.Fatalf("构造读口：%v", err)
		}
		loaded, found, err := reader.LoadIntakeQualification(ctx, tenant, rules)
		if err != nil || !found {
			t.Fatalf("读回 found=%v err=%v", found, err)
		}
		if !loaded.Allows(domain.DeclaredOffsitePickup) || len(loaded.Qualifications()) != 1 {
			t.Fatalf("读回的收寄资格变形：%#v", loaded)
		}
	})

	t.Run("终局规则声明", func(t *testing.T) {
		content := finalRuleOf(t, rules, "FINAL/effective-delivery")
		mustSaveDeclaration(t, transactor, func(txCtx context.Context) (ports.DeclarationSaveOutcome, error) {
			return repository.SaveFinalRule(txCtx, content)
		}, ports.DeclarationSaved)

		reader, err := adapter.NewStageContentDeclarations(db)
		if err != nil {
			t.Fatalf("构造读口：%v", err)
		}
		loaded, found, err := reader.LoadFinalRule(ctx, tenant, rules)
		if err != nil || !found {
			t.Fatalf("读回 found=%v err=%v", found, err)
		}
		kind, declared := loaded.FinalKindFor(domain.DeclaredEffectiveDelivery)
		if !declared || kind.String() != "FINAL/effective-delivery" {
			t.Fatalf("读回的终局声明变形：%q declared=%v", kind, declared)
		}
	})

	t.Run("取消授权目录", func(t *testing.T) {
		content := cancellationAuthorityOf(t, authorization, "CANCEL/customer-before-intake")
		mustSaveDeclaration(t, transactor, func(txCtx context.Context) (ports.DeclarationSaveOutcome, error) {
			return repository.SaveCancellationAuthority(txCtx, content)
		}, ports.DeclarationSaved)

		reader, err := adapter.NewStageContentDeclarations(db)
		if err != nil {
			t.Fatalf("构造读口：%v", err)
		}
		loaded, found, err := reader.LoadCancellationAuthority(ctx, tenant, authorization)
		if err != nil || !found {
			t.Fatalf("读回 found=%v err=%v", found, err)
		}
		rule, declared := loaded.RuleFor(domain.DeclaredCustomerCancellation)
		if !declared || rule.String() != "CANCEL/customer-before-intake" {
			t.Fatalf("读回的取消授权变形：%q declared=%v", rule, declared)
		}
		if _, declared := loaded.RuleFor(domain.DeclaredOperationsCancellation); declared {
			t.Fatal("没登记的请求方格读出了授权——缺行是真话，不是默认放行")
		}
	})

	t.Run("规则包正文", func(t *testing.T) {
		pack := rulePackageBodyOf(t, rules,
			pcAssembledRule(t, domain.MinimumIngressIdentityRules, "RULE/ingress-identity"),
			pcAssembledRule(t, domain.ShipmentInvariantRules, "RULE/shipment-invariant"))
		mustSaveDeclaration(t, transactor, func(txCtx context.Context) (ports.DeclarationSaveOutcome, error) {
			return repository.SaveAcceptanceRulePackage(txCtx, pack)
		}, ports.DeclarationSaved)

		reader, err := adapter.NewAcceptanceRulePackages(db)
		if err != nil {
			t.Fatalf("构造读口：%v", err)
		}
		loaded, found, err := reader.LoadAcceptanceRulePackage(ctx, tenant, rules)
		if err != nil || !found {
			t.Fatalf("读回 found=%v err=%v", found, err)
		}
		if rules := loaded.RulesIn(domain.MinimumIngressIdentityRules); len(rules) != 1 ||
			rules[0].Reference().String() != "RULE/ingress-identity" {
			t.Fatalf("读回的规则包正文变形：%#v", rules)
		}
		if loaded.Applicability().LegalEntity().String() != "legal-1" {
			t.Fatalf("读回的五维适用性变形：%q", loaded.Applicability().LegalEntity())
		}
	})
}

// TestDeclarationReplayAndConflictSplitByContent 证声明写入的落点代数：同拥有版本同
// 正文是重放、异正文是冲突，冲突路径一行不写、原正文完好——正文随发布固定，改声明
// 必须发新版本（ADR-0031 的落点纪律用在声明通道上）。
func TestDeclarationReplayAndConflictSplitByContent(t *testing.T) {
	repository, transactor, db := newDeclarationFixture(t)
	ctx := t.Context()
	tenant := pcTenant(t, "tenant-1")

	rules := effectiveRulePackage(t, "rules-1", "v1")
	product := effectiveDeclarationOwner(t, domain.ServiceProductObject, "product-1", "v1")
	contract := effectiveContract(t, "contract-1", "v1", "digest-c1")
	authorization := effectiveDeclarationOwner(t, domain.AuthorizationRuleObject, "authz-1", "v1")

	cases := []struct {
		name     string
		original func(context.Context) (ports.DeclarationSaveOutcome, error)
		changed  func(context.Context) (ports.DeclarationSaveOutcome, error)
	}{
		{
			name: "时点锚声明",
			original: func(txCtx context.Context) (ports.DeclarationSaveOutcome, error) {
				return repository.SaveAsOfPolicies(txCtx, asOfDeclarationOf(t, rules, "asof-policy/reach-v1"))
			},
			changed: func(txCtx context.Context) (ports.DeclarationSaveOutcome, error) {
				return repository.SaveAsOfPolicies(txCtx, asOfDeclarationOf(t, rules, "asof-policy/reach-v9"))
			},
		},
		{
			name: "接受内容声明",
			original: func(txCtx context.Context) (ports.DeclarationSaveOutcome, error) {
				return repository.SaveAcceptanceRuleContent(txCtx, acceptanceContentOf(t, rules, domain.ManualReviewRequired))
			},
			changed: func(txCtx context.Context) (ports.DeclarationSaveOutcome, error) {
				return repository.SaveAcceptanceRuleContent(txCtx, acceptanceContentOf(t, rules, domain.ManualReviewNotRequired))
			},
		},
		{
			name: "待路由许可",
			original: func(txCtx context.Context) (ports.DeclarationSaveOutcome, error) {
				return repository.SavePendingRoutingPermission(txCtx, pendingRoutingOf(t, product, "PRODUCT-CLAUSE/PENDING-OK"))
			},
			changed: func(txCtx context.Context) (ports.DeclarationSaveOutcome, error) {
				return repository.SavePendingRoutingPermission(txCtx, pendingRoutingOf(t, product, "PRODUCT-CLAUSE/OTHER"))
			},
		},
		{
			name: "接受前控制声明",
			original: func(txCtx context.Context) (ports.DeclarationSaveOutcome, error) {
				return repository.SavePreAcceptanceControl(txCtx, preAcceptanceControlOf(t, contract))
			},
			changed: func(txCtx context.Context) (ports.DeclarationSaveOutcome, error) {
				notApplicable, err := domain.DeclarePreAcceptanceControl(contract,
					domain.PreAcceptanceControlNotApplicable,
					pcValue(t, domain.NewControlNotApplicableBasis, "CONTRACT-CLAUSE/NO-CONTROL"))
				if err != nil {
					t.Fatalf("组不适用声明：%v", err)
				}
				return repository.SavePreAcceptanceControl(txCtx, notApplicable)
			},
		},
		{
			name: "合同正文",
			original: func(txCtx context.Context) (ports.DeclarationSaveOutcome, error) {
				return repository.SaveCustomerContractContent(txCtx, contractContentOf(t, contract, "control-policy-1"))
			},
			changed: func(txCtx context.Context) (ports.DeclarationSaveOutcome, error) {
				return repository.SaveCustomerContractContent(txCtx, contractContentOf(t, contract, "control-policy-9"))
			},
		},
		{
			name: "收寄资格声明",
			original: func(txCtx context.Context) (ports.DeclarationSaveOutcome, error) {
				return repository.SaveIntakeQualification(txCtx,
					intakeQualificationOf(t, rules, domain.DeclaredNodeIntake, domain.DeclaredOffsitePickup))
			},
			changed: func(txCtx context.Context) (ports.DeclarationSaveOutcome, error) {
				return repository.SaveIntakeQualification(txCtx,
					intakeQualificationOf(t, rules, domain.DeclaredNodeIntake))
			},
		},
		{
			name: "终局规则声明",
			original: func(txCtx context.Context) (ports.DeclarationSaveOutcome, error) {
				return repository.SaveFinalRule(txCtx, finalRuleOf(t, rules, "FINAL/effective-delivery"))
			},
			changed: func(txCtx context.Context) (ports.DeclarationSaveOutcome, error) {
				return repository.SaveFinalRule(txCtx, finalRuleOf(t, rules, "FINAL/another-kind"))
			},
		},
		{
			name: "取消授权目录",
			original: func(txCtx context.Context) (ports.DeclarationSaveOutcome, error) {
				return repository.SaveCancellationAuthority(txCtx,
					cancellationAuthorityOf(t, authorization, "CANCEL/customer-before-intake"))
			},
			changed: func(txCtx context.Context) (ports.DeclarationSaveOutcome, error) {
				return repository.SaveCancellationAuthority(txCtx,
					cancellationAuthorityOf(t, authorization, "CANCEL/looser-rule"))
			},
		},
		{
			name: "规则包正文",
			original: func(txCtx context.Context) (ports.DeclarationSaveOutcome, error) {
				return repository.SaveAcceptanceRulePackage(txCtx, rulePackageBodyOf(t, rules,
					pcAssembledRule(t, domain.MinimumIngressIdentityRules, "RULE/ingress-identity")))
			},
			changed: func(txCtx context.Context) (ports.DeclarationSaveOutcome, error) {
				return repository.SaveAcceptanceRulePackage(txCtx, rulePackageBodyOf(t, rules,
					pcAssembledRule(t, domain.MinimumIngressIdentityRules, "RULE/ingress-identity"),
					pcAssembledRule(t, domain.CrossFieldConditionRules, "RULE/cross-field")))
			},
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			mustSaveDeclaration(t, transactor, testCase.original, ports.DeclarationSaved)
			mustSaveDeclaration(t, transactor, testCase.original, ports.DeclarationAlreadyRegistered)
			mustSaveDeclaration(t, transactor, testCase.changed, ports.DeclarationContentConflict)
			// 冲突之后重放原正文仍答已登记：冲突路径一行没写，原正文没有被并进新内容。
			mustSaveDeclaration(t, transactor, testCase.original, ports.DeclarationAlreadyRegistered)
		})
	}

	// 抽一族经读口证原正文完好：冲突路径写了任何东西，这里都会读出混种正文。
	reader, err := adapter.NewAsOfPolicyDeclarations(db)
	if err != nil {
		t.Fatalf("构造读口：%v", err)
	}
	policies, err := reader.LoadAsOfPolicies(ctx, tenant, rules)
	if err != nil {
		t.Fatalf("读回：%v", err)
	}
	for _, policy := range policies {
		if policy.Judgment() == domain.NetworkReachabilityJudgment &&
			policy.PolicyVersion().String() != "asof-policy/reach-v1" {
			t.Fatalf("冲突写入改动了原正文：%q", policy.PolicyVersion())
		}
	}
}

// TestDeclarationWritesRefuseToRunOutsideATransaction 证九个写入口都不会在缺少事务时
// 改用连接池：声明与其拥有版本同一事务落库，是「原子保存单一对象版本与发布意图」
// （UC-PC-001 步骤 6）的持久化半边。
func TestDeclarationWritesRefuseToRunOutsideATransaction(t *testing.T) {
	repository, _, _ := newDeclarationFixture(t)
	ctx := t.Context()

	rules := effectiveRulePackage(t, "rules-1", "v1")
	product := effectiveDeclarationOwner(t, domain.ServiceProductObject, "product-1", "v1")
	contract := effectiveContract(t, "contract-1", "v1", "digest-c1")
	authorization := effectiveDeclarationOwner(t, domain.AuthorizationRuleObject, "authz-1", "v1")

	saves := map[string]func() (ports.DeclarationSaveOutcome, error){
		"时点锚声明": func() (ports.DeclarationSaveOutcome, error) {
			return repository.SaveAsOfPolicies(ctx, asOfDeclarationOf(t, rules, "asof-policy/reach-v1"))
		},
		"接受内容声明": func() (ports.DeclarationSaveOutcome, error) {
			return repository.SaveAcceptanceRuleContent(ctx, acceptanceContentOf(t, rules, domain.ManualReviewRequired))
		},
		"待路由许可": func() (ports.DeclarationSaveOutcome, error) {
			return repository.SavePendingRoutingPermission(ctx, pendingRoutingOf(t, product, "PRODUCT-CLAUSE/PENDING-OK"))
		},
		"接受前控制声明": func() (ports.DeclarationSaveOutcome, error) {
			return repository.SavePreAcceptanceControl(ctx, preAcceptanceControlOf(t, contract))
		},
		"合同正文": func() (ports.DeclarationSaveOutcome, error) {
			return repository.SaveCustomerContractContent(ctx, contractContentOf(t, contract, "control-policy-1"))
		},
		"收寄资格声明": func() (ports.DeclarationSaveOutcome, error) {
			return repository.SaveIntakeQualification(ctx, intakeQualificationOf(t, rules, domain.DeclaredNodeIntake))
		},
		"终局规则声明": func() (ports.DeclarationSaveOutcome, error) {
			return repository.SaveFinalRule(ctx, finalRuleOf(t, rules, "FINAL/effective-delivery"))
		},
		"取消授权目录": func() (ports.DeclarationSaveOutcome, error) {
			return repository.SaveCancellationAuthority(ctx, cancellationAuthorityOf(t, authorization, "CANCEL/x"))
		},
		"规则包正文": func() (ports.DeclarationSaveOutcome, error) {
			return repository.SaveAcceptanceRulePackage(ctx, rulePackageBodyOf(t, rules,
				pcAssembledRule(t, domain.MinimumIngressIdentityRules, "RULE/ingress-identity")))
		},
	}
	for name, save := range saves {
		t.Run(name, func(t *testing.T) {
			if _, err := save(); !errors.Is(err, bentopg.ErrTransactionRequired) {
				t.Errorf("无事务写入应返回 ErrTransactionRequired，实得：%v", err)
			}
		})
	}
}