package main

import (
	"context"
	"fmt"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	pspartycommercial "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/partycommercial"
	pspostgres "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres"
	shipmentapp "go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	psports "go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	pcpostgres "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/postgres"
	pcapplication "go.idp.xyz/idp-parcel/internal/partycommercial/application"
	pcdomain "go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	pcports "go.idp.xyz/idp-parcel/internal/partycommercial/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	vepostgres "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/postgres"
	visibilityapp "go.idp.xyz/idp-parcel/internal/visibilityexception/application"
	visibilitydomain "go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
	veports "go.idp.xyz/idp-parcel/internal/visibilityexception/ports"
)

// Covers: `/claims` 的第二参是真编排——索赔库、追偿库、标识签发与责任结论 Outbox
// 在真实 PostgreSQL 上装得起来；受理真实落库且重放走已有项（证首笔事务真的提交了）；
// 资格缝接真后（票 ve-claims-read-seams/01）审核入口按租户的册作答且两态分明——
// 未登记租户停在「声明待登记」的业务格（不是 error，那会被端点折成 5xx；也不是
// `不予受理`或`不受理`，那是业务否定），别的租户登了册也不改这一答；本租户登册后
// 审核走进逐维核对、停在册上真实缺的那一维。证据缝接真后（票 ve-claims-read-seams/02）
// 零收讫答 known=true 的零件、登记后按行作答。索赔项全程一字不动。测试输入是隔离
// 合成，只记 `S`，不进生产装配。
func TestTheWiredClaimsAnswerHonestlyAgainstARealDatabase(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}

	claims, err := buildClaimsOrchestration(db)
	if err != nil {
		t.Fatalf("装配索赔编排：%v", err)
	}

	command := visibilityapp.ReceiveClaimCommand{
		TenantID:    mustValue(t, visibilitydomain.NewTenantID, "SYN-TENANT-1"),
		Batch:       mustValue(t, visibilitydomain.NewClaimBatchReference, "SYN-CLAIM-BATCH-1"),
		Item:        mustValue(t, visibilitydomain.NewClaimItemID, "SYN-CLAIM-ITEM-1"),
		Customer:    mustValue(t, visibilitydomain.NewCustomerAccountReference, "SYN-CUSTOMER-1"),
		Applicant:   mustValue(t, visibilitydomain.NewApplicantReference, "SYN-APPLICANT-1"),
		Contract:    mustValue(t, visibilitydomain.NewContractScopeReference, "SYN-CONTRACT-1"),
		Target:      mustValue(t, visibilitydomain.NewRequestScopeReference, "SYN-PARCEL-1"),
		Kind:        mustValue(t, visibilitydomain.NewClaimKindReference, "SYN-KIND-LOSS"),
		SubmittedAt: time.Date(2026, 8, 22, 10, 0, 0, 0, time.UTC),
	}

	received, err := claims.ReceiveClaim(t.Context(), command)
	if err != nil {
		t.Fatalf("受理索赔：%v", err)
	}
	if got := received.Outcome(); got != visibilityapp.ClaimReceived {
		t.Fatalf("outcome = %v, want CLAIM_RECEIVED", got)
	}
	if _, has := received.Claim(); !has {
		t.Fatal("受理成功却没带回索赔项")
	}

	replay, err := claims.ReceiveClaim(t.Context(), command)
	if err != nil {
		t.Fatalf("重放同一份受理：%v", err)
	}
	if got := replay.Outcome(); got != visibilityapp.ClaimExistingResult {
		t.Fatalf("outcome = %v, want CLAIM_EXISTING_RESULT——重放没走已有项，首笔事务的落库没有提交", got)
	}

	// 票 ve-claims-read-seams/01 的验收钉（换下接线票 04 的 UNAVAILABLE 钉）：资格缝
	// 已接真，未登记租户的审核停在「声明待登记」的业务格。三个都不是它的答案——error
	// （端点会折成 5xx NO_ANSWER_FORMED）、`不予受理`（ADR-0051 的永久格，默认拒赔）、
	// `已过审`（默认放行）。
	screenCommand := visibilityapp.ScreenClaimCommand{
		TenantID: command.TenantID,
		Batch:    command.Batch,
		Item:     command.Item,
	}
	screened, err := claims.ScreenClaim(t.Context(), screenCommand)
	if err != nil {
		t.Fatalf("未登记租户的审核不该以 error 交回（那是 5xx，不是未决）：%v", err)
	}
	if got := screened.Outcome(); got != visibilityapp.HandleClaimUndecided {
		t.Fatalf("outcome = %v, want UNDECIDED——声明待登记既不是业务否定也不是放行", got)
	}
	if got := screened.UndecidedReason(); got != visibilityapp.EligibilityCatalogueNotConfigured {
		t.Fatalf("reason = %v, want ELIGIBILITY_CATALOGUE_NOT_CONFIGURED——恢复动作是登记声明，不再是接缝", got)
	}

	// 别的租户登了册也不改这一答：读的若不是查询租户自己的册，这一格就会串。
	registrar, err := vepostgres.NewCatalogRegistrar(db)
	if err != nil {
		t.Fatalf("构造目录写入方：%v", err)
	}
	registerEligibility := func(tenant visibilitydomain.TenantID) {
		t.Helper()
		var outcome veports.CatalogRegistrationOutcome
		if err := db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
			var registerErr error
			outcome, registerErr = registrar.RegisterClaimEligibility(txCtx, tenant,
				veports.ClaimEligibilityRegistration{
					Header:       veports.CatalogApprovalHeader{Version: "SYN-CLAIM-RULES-1", ApprovedBy: "SYN-OPERATOR-1"},
					Contract:     command.Contract,
					CoveredKinds: []visibilitydomain.ClaimKindReference{command.Kind},
				})
			return registerErr
		}); err != nil {
			t.Fatalf("事务内登记失败：%v", err)
		}
		if outcome != veports.CatalogVersionRegistered {
			t.Fatalf("登记索赔声明应成功，实得 %s", outcome)
		}
	}
	registerEligibility(mustValue(t, visibilitydomain.NewTenantID, "SYN-TENANT-2"))
	crossTenant, err := claims.ScreenClaim(t.Context(), screenCommand)
	if err != nil {
		t.Fatalf("跨租户审核：%v", err)
	}
	if got := crossTenant.UndecidedReason(); got != visibilityapp.EligibilityCatalogueNotConfigured {
		t.Fatalf("reason = %v——别的租户登册改了本租户的答案，租户维没真的进查询", got)
	}

	// 本租户登册后按册作答：声明在场、类型在保，审核走进逐维核对，停在册上真实缺的
	// 第一维（首次索赔期限属 `PAR-VIS-08` 待登记实例参数）——证明答案确实来自这租户
	// 的册，而不是任何一格顶位。
	registerEligibility(command.TenantID)
	registered, err := claims.ScreenClaim(t.Context(), screenCommand)
	if err != nil {
		t.Fatalf("登册后的审核：%v", err)
	}
	if got := registered.Outcome(); got != visibilityapp.HandleClaimUndecided {
		t.Fatalf("outcome = %v, want UNDECIDED——期限维还没登记", got)
	}
	if got := registered.UndecidedReason(); got != visibilityapp.EligibilityFilingDeadlineNotRegistered {
		t.Fatalf("reason = %v, want ELIGIBILITY_FILING_DEADLINE_NOT_REGISTERED——登册后要按册答到真实缺的那一维", got)
	}

	// 证据缝经公开入口仍走不到：材料清单维未登记时，逐维核对在读证据之前就停下
	// （judgeMinimumMaterials 先看 Registered）。直接钉装配所接的真读适配器（票
	// ve-claims-read-seams/02 换下显式未配置桩）：没有收讫行答 known=true 的零件
	// ——「查过了，一件都没收到」的有效事实，与退役桩的「无从查起」（known=false）
	// 语义相反；经受控写入口登记一笔后按行作答。
	evidence, err := vepostgres.NewClaimMaterialReceipts(db)
	if err != nil {
		t.Fatalf("构造证据视图：%v", err)
	}
	materials, known, err := evidence.ReceivedMaterials(
		t.Context(), command.TenantID, command.Batch, command.Item)
	if err != nil {
		t.Fatalf("读空归集不该以 error 交回：%v", err)
	}
	if !known {
		t.Fatal("归集面已接真却答 known=false——「无从查起」那格已随桩退役")
	}
	if len(materials) != 0 {
		t.Fatalf("零收讫却交回材料清单：%v", materials)
	}

	receipts, err := vepostgres.NewMaterialReceiptRegistrar(db)
	if err != nil {
		t.Fatalf("构造归集面写入方：%v", err)
	}
	photo := mustValue(t, visibilitydomain.NewMaterialRequirementReference, "SYN-MAT-PHOTO")
	var receiptOutcome veports.MaterialReceiptWriteOutcome
	if err := db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
		var registerErr error
		receiptOutcome, registerErr = receipts.RegisterReceipt(txCtx, command.TenantID, veports.MaterialReceipt{
			Batch:      command.Batch,
			Item:       command.Item,
			Material:   photo,
			ReceivedAt: time.Date(2026, 8, 23, 9, 0, 0, 0, time.UTC),
			ReceivedBy: "SYN-OPERATOR-1",
		})
		return registerErr
	}); err != nil {
		t.Fatalf("事务内登记收讫失败：%v", err)
	}
	if receiptOutcome != veports.MaterialReceiptRecorded {
		t.Fatalf("登记收讫应成功，实得 %d", receiptOutcome)
	}
	materials, known, err = evidence.ReceivedMaterials(
		t.Context(), command.TenantID, command.Batch, command.Item)
	if err != nil || !known {
		t.Fatalf("登记后读现存集：err=%v known=%v", err, known)
	}
	if len(materials) != 1 || materials[0].String() != "SYN-MAT-PHOTO" {
		t.Fatalf("现存集 = %v，要登记的那一件", materials)
	}
}

// Covers: 票 ve-claims-read-seams/04「要做什么」4 与完成判据 3（ADR-0136 决定二 / 四）——资格缝的两维按目标
// 包裹所属委托**接受时固定的商业解析回指**选规则版本：PS 真委托仓储按（租户，包裹身份）答回指，PC 真解析库按
// （租户，回指）读回快照里已采用的客户服务规则版本（判据 5 读写对称的真库实证），再点读正文。对真库钉三态：
// 态零「PS 没有」——目标不属任何已接受委托，两维未登记、编排照旧停在 ELIGIBILITY_FILING_DEADLINE_NOT_REGISTERED，
// 这就是生产装配今天对每一封索赔的可观察行为（PS 登记面尚未让闭包形成，ADR-0136 Consequences），与本票之前
// 键来源留 nil 一字不变；态一「闭包未采用客户服务规则」——委托已接受、闭包只采用了客户合同，两维未登记（恢复
// 动作是去 PS 解析键登记面列进必需依据，不是登正文）；态二「已登记」——闭包采用了客户合同与客户服务规则、PC
// 登了正文，两维 Registered、RuleVersion 是 PC 三段版本引用、Required 与 PC 条目逐项相等、留格三样仍零值；编排
// 侧照票 05 钉差材料 → SUPPLEMENT_DEADLINE_UNDERIVABLE、材料齐 → FILING_DEADLINE_UNDERIVABLE。
//
// **闭包怎么来**：ADR-0136 决定四让实例半边落在 PS 的解析键登记面。两态都真走它：两个货主客户账户各登一行
// ——客户一的必需依据含客户合同 + 客户服务规则（PS 登记面自票 `.scratch/ps-port-remainder/issues/09-resolution-key-face-does-not-accept-customer-service-rule.md`
// 起收这一类：迁移 0022 重加的 CHECK `commercial_resolution_key_registration_bases_closed` 与 PS `commercialKindFrom`
// 各加一格），客户二只含客户合同——各按登记行 `FormResolutionKey` 成键 → PC 真 `ResolveCommercialBasisHandler` 对真
// 发布登记册解出、真解析库固定闭包 → PS 真 `ShipmentRequests` 照包内夹具同形 Decide → Save 接受委托，决定上的回指
// 指向它。VE 这一头读的每一段（PS 回指读口、PC 快照读回、正文点读）都是真件，与生产路径同一条。测试输入是隔离
// 合成，只记 `S`——不为任何租户预填含客户服务规则的键。
func TestTheWiredClaimsReadTheRuleAdoptedAtAcceptanceThroughParcelShipment(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	ctx := t.Context()

	tenant := mustValue(t, pcdomain.NewTenantID, "SYN-TENANT-1")
	scope := mustValue(t, pcdomain.NewCommercialScopeReference, "SYN-SCOPE-1")

	// 生产装配本身就是被测对象：`buildClaimsOrchestration` 接的读面与 `buildClaimEligibilityRules` 同一份。
	claims, err := buildClaimsOrchestration(db)
	if err != nil {
		t.Fatalf("装配索赔编排：%v", err)
	}
	rules, err := buildClaimEligibilityRules(db)
	if err != nil {
		t.Fatalf("装配资格规则读面：%v", err)
	}

	// 两项索赔同租户同合同、各属一个货主客户账户：客户一那项指向随后接受的委托一（登记行含客户合同 + 客户服务
	// 规则），客户二那项指向委托二（登记行只含客户合同）。解析键登记面按（租户，客户账户）一行，两种登记行因此
	// 要两个客户。目标范围引用原样是 PS 的声明包裹身份。
	submission, secondSubmission := submissionCommand(t), secondSubmissionCommand(t)
	receive := func(item, target string, identity psdomain.SourceIdentity) visibilityapp.ReceiveClaimCommand {
		t.Helper()
		command := visibilityapp.ReceiveClaimCommand{
			TenantID:    mustValue(t, visibilitydomain.NewTenantID, identity.TenantID().String()),
			Batch:       mustValue(t, visibilitydomain.NewClaimBatchReference, "SYN-CLAIM-BATCH-1"),
			Item:        mustValue(t, visibilitydomain.NewClaimItemID, item),
			Customer:    mustValue(t, visibilitydomain.NewCustomerAccountReference, identity.CustomerAccountID().String()),
			Applicant:   mustValue(t, visibilitydomain.NewApplicantReference, "SYN-APPLICANT-1"),
			Contract:    mustValue(t, visibilitydomain.NewContractScopeReference, "SYN-CONTRACT-1"),
			Target:      mustValue(t, visibilitydomain.NewRequestScopeReference, target),
			Kind:        mustValue(t, visibilitydomain.NewClaimKindReference, "SYN-KIND-LOSS"),
			SubmittedAt: time.Date(2026, 8, 22, 10, 0, 0, 0, time.UTC),
		}
		if received, err := claims.ReceiveClaim(ctx, command); err != nil || received.Outcome() != visibilityapp.ClaimReceived {
			t.Fatalf("受理索赔 %s：outcome=%v err=%v", item, received.Outcome(), err)
		}
		return command
	}
	first := receive("SYN-CLAIM-ITEM-1", submission.DeclaredParcelIDs[0].String(), submission.Identity)
	second := receive("SYN-CLAIM-ITEM-2", secondSubmission.DeclaredParcelIDs[0].String(), secondSubmission.Identity)

	registrar, err := vepostgres.NewCatalogRegistrar(db)
	if err != nil {
		t.Fatalf("构造目录写入方：%v", err)
	}
	var eligibilityOutcome veports.CatalogRegistrationOutcome
	if err := db.Transactor().WithinTransaction(ctx, func(txCtx context.Context) error {
		var registerErr error
		eligibilityOutcome, registerErr = registrar.RegisterClaimEligibility(txCtx, first.TenantID,
			veports.ClaimEligibilityRegistration{
				Header:       veports.CatalogApprovalHeader{Version: "SYN-CLAIM-RULES-1", ApprovedBy: "SYN-OPERATOR-1"},
				Contract:     first.Contract,
				CoveredKinds: []visibilitydomain.ClaimKindReference{first.Kind},
			})
		return registerErr
	}); err != nil {
		t.Fatalf("事务内登记失败：%v", err)
	}
	if eligibilityOutcome != veports.CatalogVersionRegistered {
		t.Fatalf("登记索赔声明应成功，实得 %s", eligibilityOutcome)
	}

	queryFor := func(command visibilityapp.ReceiveClaimCommand) veports.EligibilityQuery {
		return veports.EligibilityQuery{
			Tenant: command.TenantID, Batch: command.Batch, Item: command.Item, Customer: command.Customer,
			Contract: command.Contract, Target: command.Target, Kind: command.Kind, Applicant: command.Applicant,
		}
	}
	// 读面走 RequireExecutor，只在事务内可用——生产路径上索赔编排本来就包在一笔事务里（transactionalClaims）。
	readRules := func(query veports.EligibilityQuery) veports.EligibilityRules {
		t.Helper()
		var answer veports.EligibilityRules
		var declared bool
		if err := db.Transactor().WithinTransaction(ctx, func(txCtx context.Context) error {
			var readErr error
			answer, declared, readErr = rules.RulesForClaim(txCtx, query)
			return readErr
		}); err != nil {
			t.Fatalf("读资格规则：%v", err)
		}
		if !declared {
			t.Fatal("声明已登记却答不在场")
		}
		return answer
	}
	screenFor := func(command visibilityapp.ReceiveClaimCommand) visibilityapp.ScreenClaimCommand {
		return visibilityapp.ScreenClaimCommand{TenantID: command.TenantID, Batch: command.Batch, Item: command.Item}
	}

	// 态零：目标不属任何已接受委托——PS 答没有，不进 PC；两维未登记，编排照旧停在 FILING_DEADLINE_NOT_REGISTERED。
	// 这一格就是生产装配今天的行为，与本票之前键来源留 nil 一字不变。
	before := readRules(queryFor(first))
	if before.FilingDeadline.Registered || before.Materials.Registered {
		t.Fatalf("PS 没有回指却答了登记：%#v", before)
	}
	if !before.KindCovered {
		t.Fatal("VE 自己的册说类型在保，叠两维之后丢了")
	}
	screened, err := claims.ScreenClaim(ctx, screenFor(first))
	if err != nil || screened.Outcome() != visibilityapp.HandleClaimUndecided ||
		screened.UndecidedReason() != visibilityapp.EligibilityFilingDeadlineNotRegistered {
		t.Fatalf("PS 没有回指时的审核：outcome=%v reason=%v err=%v，要照旧停在 ELIGIBILITY_FILING_DEADLINE_NOT_REGISTERED",
			screened.Outcome(), screened.UndecidedReason(), err)
	}

	// PC 先有两版壳可选：客户合同与客户服务规则各一版生效于同一范围（正文到态二才登）。
	publications, err := pcpostgres.NewCommercialPublications(db)
	if err != nil {
		t.Fatalf("构造商业发布登记册：%v", err)
	}
	contract := effectiveCommercialShell(t, tenant, pcdomain.CustomerContractObject, "SYN-CONTRACT-1", scope)
	version := effectiveCustomerServiceRule(t, tenant, scope)
	for _, shell := range []pcdomain.CommercialVersion{contract, version} {
		var shellOutcome pcports.PublicationSaveOutcome
		if err := db.Transactor().WithinTransaction(ctx, func(txCtx context.Context) error {
			var saveErr error
			shellOutcome, saveErr = publications.SaveVersion(txCtx, shell)
			return saveErr
		}); err != nil || shellOutcome != pcports.PublicationSaved {
			t.Fatalf("发布壳 %s：outcome=%s err=%v", shell.ObjectID(), shellOutcome, err)
		}
	}

	// PS 解析键登记面：两个客户各一行——客户一的必需依据含客户合同 + 客户服务规则（ps-port-remainder/09 放行的
	// 那一格），客户二只含客户合同。两行都是登记进去的实例参数，不带任何默认。
	keyStore, err := pspostgres.NewCommercialResolutionKeyStore(db)
	if err != nil {
		t.Fatalf("构造解析键登记面：%v", err)
	}
	keys, err := pspartycommercial.NewCommercialResolutionKeys(keyStore)
	if err != nil {
		t.Fatalf("构造解析键适配器：%v", err)
	}
	registerKey := func(identity psdomain.SourceIdentity, bases ...pcdomain.CommercialObjectKind) {
		t.Helper()
		var outcome pspartycommercial.ResolutionKeySaveOutcome
		if err := db.Transactor().WithinTransaction(ctx, func(txCtx context.Context) error {
			var registerErr error
			outcome, registerErr = keys.Register(txCtx, pspartycommercial.ResolutionKeyRegistration{
				TenantID:          identity.TenantID(),
				CustomerAccountID: identity.CustomerAccountID(),
				Scope:             scope,
				LegalEntity:       mustValue(t, pcdomain.NewLegalEntityReference, "SYN-LEGAL-1"),
				AnchorPolicy:      mustValue(t, pcdomain.NewAnchorPolicyVersion, "SYN-ANCHOR-POLICY-1"),
				AnchorAt:          time.Date(2026, 8, 21, 10, 0, 0, 0, time.UTC),
				RequiredBases:     bases,
			})
			return registerErr
		}); err != nil || outcome != pspartycommercial.ResolutionKeySaved {
			t.Fatalf("解析键行 %s / %v 登不进 PS 登记面：outcome=%s err=%v", identity.CustomerAccountID(), bases, outcome, err)
		}
	}
	registerKey(submission.Identity, pcdomain.CustomerContractObject, pcdomain.CustomerServiceRuleObject)
	registerKey(secondSubmission.Identity, pcdomain.CustomerContractObject)
	formKey := func(identity psdomain.SourceIdentity) pcdomain.ClosureResolutionKey {
		t.Helper()
		key, formed, err := keys.FormResolutionKey(ctx, psports.CommercialBasisQuery{Identity: identity})
		if err != nil || !formed {
			t.Fatalf("按登记行成键 %s：formed=%v err=%v", identity.CustomerAccountID(), formed, err)
		}
		return key
	}

	// 态一：客户二的登记行经 FormResolutionKey 成键（必需依据只有客户合同），PC 真解析器解出并固定闭包，委托二
	// 据此接受。闭包只采用了客户合同，两维未登记：恢复动作是去 PS 解析键登记面列进客户服务规则那一类，不是去
	// PC 登正文。
	contractOnlyClosure := resolveClosureOnRealAssembly(t, db, publications, formKey(secondSubmission.Identity))
	if _, adopted := contractOnlyClosure.AdoptedFor(pcdomain.CustomerServiceRuleObject); adopted {
		t.Fatal("键不含客户服务规则，闭包却采用了它")
	}
	acceptOnRealAssemblyWith(t, db, secondSubmission, contractOnlyClosure.ResolutionID().String(), "SYN-DECISION-2")
	contractOnly := readRules(queryFor(second))
	if contractOnly.FilingDeadline.Registered || contractOnly.Materials.Registered {
		t.Fatalf("闭包没采用客户服务规则却答了登记：%#v", contractOnly)
	}
	screened, err = claims.ScreenClaim(ctx, screenFor(second))
	if err != nil || screened.UndecidedReason() != visibilityapp.EligibilityFilingDeadlineNotRegistered {
		t.Fatalf("闭包未采用客户服务规则时的审核：reason=%v err=%v，要停在 ELIGIBILITY_FILING_DEADLINE_NOT_REGISTERED",
			screened.UndecidedReason(), err)
	}

	// 态二：客户一的登记行经 FormResolutionKey 成键（必需依据含客户合同 + 客户服务规则），PC 真解析器解出并固定
	// 闭包，委托一据此接受；PC 再登那一版客户服务规则的正文（首次索赔期限一行 + 本索赔类型一份材料清单）。
	fullClosure := resolveClosureOnRealAssembly(t, db, publications, formKey(submission.Identity))
	if adopted, ok := fullClosure.AdoptedFor(pcdomain.CustomerServiceRuleObject); !ok ||
		adopted.Version().ObjectID().String() != "SYN-CSR-1" {
		t.Fatalf("闭包没采用那一版客户服务规则：%v", ok)
	}
	if fullClosure.ResolutionID() == contractOnlyClosure.ResolutionID() {
		t.Fatal("两份采用集不同的闭包算出同一个解析标识")
	}
	acceptOnRealAssemblyWith(t, db, submission, fullClosure.ResolutionID().String(), "SYN-DECISION-1")
	content, err := pcdomain.NewCustomerServiceRuleVersion(
		version,
		pcdomain.CustomerServiceRuleAppliesToCustomerContract(mustValue(t, pcdomain.NewCommercialObjectID, "SYN-CONTRACT-1")),
		mustValue(t, pcdomain.NewPartyID, "SYN-OPERATOR-1"),
		scope,
		[]pcdomain.ClaimDeadlineRule{mustDeadlineRule(t, pcdomain.FirstClaimDeadline, "SYN-EVENT-DELIVERED", 30, "SYN-CALENDAR-1")},
		[]pcdomain.MinimumMaterialsRule{mustMaterialsRule(t, "SYN-KIND-LOSS", "SYN-MAT-PHOTO", "SYN-MAT-INVOICE")},
	)
	if err != nil {
		t.Fatalf("客户服务规则正文：%v", err)
	}
	var contentOutcome pcports.CustomerServiceRuleSaveOutcome
	if err := db.Transactor().WithinTransaction(ctx, func(txCtx context.Context) error {
		var saveErr error
		contentOutcome, saveErr = publications.SaveCustomerServiceRule(txCtx, content)
		return saveErr
	}); err != nil {
		t.Fatalf("事务内登记客户服务规则正文失败：%v", err)
	}
	if contentOutcome != pcports.CustomerServiceRuleSaved {
		t.Fatalf("登记正文 = %s，应成功", contentOutcome)
	}

	command, screenCommand := first, screenFor(first)
	after := readRules(queryFor(first))
	deadline := after.FilingDeadline
	if !deadline.Registered || deadline.RuleVersion != "SYN-TENANT-1/SYN-CSR-1/v1" ||
		deadline.StartEvent != "SYN-EVENT-DELIVERED" || deadline.Calendar != "SYN-CALENDAR-1" ||
		deadline.Scope != first.Target.String() {
		t.Fatalf("首次索赔期限维 = %#v，要 Registered、PC 三段版本引用、照引用转写的起算事件与日历、索赔目标范围", deadline)
	}
	if !deadline.Deadline.IsZero() {
		t.Fatalf("Deadline = %v；起算事实源与业务日历今天都没有，截止时刻只能留格", deadline.Deadline)
	}
	materials := after.Materials
	if !materials.Registered || materials.RuleVersion != "SYN-TENANT-1/SYN-CSR-1/v1" ||
		len(materials.Required) != 2 ||
		materials.Required[0].String() != "SYN-MAT-INVOICE" || materials.Required[1].String() != "SYN-MAT-PHOTO" {
		t.Fatalf("最低材料维 = %#v，要 Registered、同一版本引用、与 PC 材料条目逐项相等", materials)
	}
	if materials.Notice.String() != "" || !materials.SupplementDeadline.IsZero() {
		t.Fatalf("Notice=%q SupplementDeadline=%v；两样今天都没有来源，只能留格", materials.Notice, materials.SupplementDeadline)
	}

	// 材料维接通之后，零收讫的索赔走进「差材料」那一支；补充截止是留格的零值，编排停在
	// ELIGIBILITY_SUPPLEMENT_DEADLINE_UNDERIVABLE（票 ve-claims-read-seams/05）——停在未决、不记第三态、
	// 不拒赔，名字说的是「截止算不出」而不是「窗口已关」。
	screened, err = claims.ScreenClaim(ctx, screenCommand)
	if err != nil || screened.Outcome() != visibilityapp.HandleClaimUndecided {
		t.Fatalf("PC 登了正文后的审核：outcome=%v err=%v，要停在未决而不是拒赔或放行", screened.Outcome(), err)
	}
	if got := screened.UndecidedReason(); got != visibilityapp.EligibilitySupplementDeadlineUnderivable {
		t.Fatalf("reason = %v, want ELIGIBILITY_SUPPLEMENT_DEADLINE_UNDERIVABLE——材料维已按 PC 清单核出缺口、补充截止留格", got)
	}
	if _, has := screened.Claim(); has {
		t.Fatal("停在未决却交回了索赔项——未决时索赔项该一字不动、不随答案交出")
	}

	// 材料按 PC 清单收齐之后，差材料那一支不再走到，停下的换成首次索赔期限维：规则登了而截止算不出
	// ——ELIGIBILITY_FILING_DEADLINE_UNDERIVABLE，不是「未登记」（规则明明在 PC 里）。
	receipts, err := vepostgres.NewMaterialReceiptRegistrar(db)
	if err != nil {
		t.Fatalf("构造归集面写入方：%v", err)
	}
	for _, material := range []string{"SYN-MAT-PHOTO", "SYN-MAT-INVOICE"} {
		var receiptOutcome veports.MaterialReceiptWriteOutcome
		if err := db.Transactor().WithinTransaction(ctx, func(txCtx context.Context) error {
			var registerErr error
			receiptOutcome, registerErr = receipts.RegisterReceipt(txCtx, command.TenantID, veports.MaterialReceipt{
				Batch:      command.Batch,
				Item:       command.Item,
				Material:   mustValue(t, visibilitydomain.NewMaterialRequirementReference, material),
				ReceivedAt: time.Date(2026, 8, 23, 9, 0, 0, 0, time.UTC),
				ReceivedBy: "SYN-OPERATOR-1",
			})
			return registerErr
		}); err != nil {
			t.Fatalf("事务内登记收讫 %s 失败：%v", material, err)
		}
		if receiptOutcome != veports.MaterialReceiptRecorded {
			t.Fatalf("登记收讫 %s 应成功，实得 %d", material, receiptOutcome)
		}
	}
	screened, err = claims.ScreenClaim(ctx, screenCommand)
	if err != nil || screened.Outcome() != visibilityapp.HandleClaimUndecided {
		t.Fatalf("材料齐后的审核：outcome=%v err=%v，要停在未决", screened.Outcome(), err)
	}
	if got := screened.UndecidedReason(); got != visibilityapp.EligibilityFilingDeadlineUnderivable {
		t.Fatalf("reason = %v, want ELIGIBILITY_FILING_DEADLINE_UNDERIVABLE——规则在 PC 里，缺的是起算事实与日历能力", got)
	}

	// 委托二那项索赔不因委托一的闭包改口：两维按各自目标包裹的回指各选各的，闭包只采用了客户合同的仍未登记。
	if again := readRules(queryFor(second)); again.FilingDeadline.Registered || again.Materials.Registered {
		t.Fatalf("委托二的索赔借了委托一的规则：%#v", again)
	}
}

// secondSubmissionCommand 是 submissionCommand 那份委托的姊妹：同租户、另一个货主客户账户、另一把来源键、另一件
// 包裹——解析键登记面按（租户，客户账户）一行，要两种登记行就要两个客户。
func secondSubmissionCommand(t *testing.T) shipmentapp.SubmitShipmentRequestCommand {
	t.Helper()
	base := submissionCommand(t)
	identity, err := psdomain.NewSourceIdentity(
		base.Identity.TenantID(), mustValue(t, psdomain.NewCustomerAccountID, "SYN-CUSTOMER-2"), base.Identity.Source(),
		mustValue(t, psdomain.NewSourceRequestKey, "SYN-KEY-2"))
	if err != nil {
		t.Fatalf("第二份来源身份：%v", err)
	}
	command := base
	command.Identity = identity
	command.PayloadDigest = mustValue(t, psdomain.NewPayloadDigest, "syn-digest-2")
	command.BatchID = mustValue(t, psdomain.NewSubmissionBatchID, "syn-batch-2")
	command.ShipmentRequestID = mustValue(t, psdomain.NewShipmentRequestID, "syn-request-2")
	command.DeclaredParcelIDs = []psdomain.DeclaredParcelID{mustValue(t, psdomain.NewDeclaredParcelID, "syn-parcel-2")}
	return command
}

// resolveClosureOnRealAssembly 让 PC 真解析器对着真发布登记册解一把键并固定闭包（真解析库）：闭包的形状——采用了哪些
// 类别、回指怎么算——由提供方定义，测试不手搓。读面走 RequireExecutor，只在事务内可用。
func resolveClosureOnRealAssembly(
	t *testing.T, db *bentopg.DB, publications *pcpostgres.CommercialPublications, key pcdomain.ClosureResolutionKey,
) pcdomain.CommercialClosure {
	t.Helper()
	authority, err := pcpostgres.NewCommercialAuthority(publications)
	if err != nil {
		t.Fatalf("构造商业权威读口：%v", err)
	}
	resolutions, err := pcpostgres.NewCommercialResolutions(db)
	if err != nil {
		t.Fatalf("构造解析库：%v", err)
	}
	resolver := pcapplication.NewResolveCommercialBasisHandler(authority, resolutions, fixedClock{at: key.Anchor.At()})
	var result pcapplication.ResolveCommercialBasisResult
	if err := db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
		var resolveErr error
		result, resolveErr = resolver.Handle(txCtx, pcapplication.ResolveCommercialBasisCommand{Key: key})
		return resolveErr
	}); err != nil {
		t.Fatalf("解析商业依据：%v", err)
	}
	closure := result.Closure()
	if closure.Outcome() != pcdomain.UniquelyResolved || result.Fixed() != pcports.ResolutionSaved {
		t.Fatalf("闭包 outcome=%s fixed=%s，要唯一已解析并首次固定进解析库", closure.Outcome(), result.Fixed())
	}
	return closure
}

// acceptOnRealAssemblyWith 走生产提交装配提交 command，再照包内夹具同形经领域 Decide + 真仓储 Save 把它推到`已接受`，
// 接受决定上的回指指向 resolution（形照 acceptedOnRealAssembly，只把回指与决定标识开成参数）。
func acceptOnRealAssemblyWith(
	t *testing.T, db *bentopg.DB, command shipmentapp.SubmitShipmentRequestCommand, resolution, decisionID string,
) {
	t.Helper()
	submission, _ := envelopeMintingSubmission(t, db)
	if submitted, err := submission.Handle(t.Context(), command); err != nil || submitted.Outcome() != shipmentapp.OutcomeSubmitted {
		t.Fatalf("提交委托 %s：outcome=%v err=%v", command.Identity.RequestKey(), submitted.Outcome(), err)
	}
	requests, err := pspostgres.NewShipmentRequests(db)
	if err != nil {
		t.Fatalf("构造委托仓储：%v", err)
	}
	identity := command.Identity
	request, found, err := requests.FindBySourceIdentity(t.Context(), identity)
	if err != nil || !found {
		t.Fatalf("取回委托 %s：err=%v found=%v", identity.RequestKey(), err, found)
	}
	applicable, err := psdomain.NewApplicableCheckGroups(psdomain.NetworkReachabilityCheck)
	if err != nil {
		t.Fatalf("适用校验组：%v", err)
	}
	basis, err := psdomain.NewCommercialBasisSnapshot(psdomain.CommercialBasisSnapshotSpec{
		ResolutionID: mustValue(t, psdomain.NewCommercialResolutionID, resolution),
		RulePackage:  mustValue(t, psdomain.NewRulePackageReference, "SYN-RULES-1/v1"),
		ViewRevision: mustValue(t, psdomain.NewCommercialViewRevision, "SYN-VIEW-1"),
		Applicable:   applicable,
		ManualReview: psdomain.ManualReviewNotRequiredByRules,
	})
	if err != nil {
		t.Fatalf("商业依据快照：%v", err)
	}
	checks := make([]psdomain.AcceptanceCheck, 0, len(command.DeclaredParcelIDs))
	for _, parcel := range command.DeclaredParcelIDs {
		check, err := psdomain.NewAcceptanceCheck(psdomain.NetworkReachabilityCheck, parcel, psdomain.CheckPassed, psdomain.CheckReason{})
		if err != nil {
			t.Fatalf("接受校验：%v", err)
		}
		checks = append(checks, check)
	}
	accepted, err := request.Decide(psdomain.AcceptanceDecisionSpec{
		DecisionID: mustValue(t, psdomain.NewAcceptanceDecisionID, decisionID),
		Checks:     checks,
		Basis:      basis,
		DecidedAt:  time.Date(2026, 8, 21, 10, 30, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("形成接受：%v", err)
	}
	if err := db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
		saved, err := requests.Save(txCtx, identity, accepted)
		if err != nil {
			return err
		}
		if saved != psports.ShipmentRequestSaved {
			return fmt.Errorf("save outcome = %v", saved)
		}
		return nil
	}); err != nil {
		t.Fatalf("存已接受的委托 %s：%v", identity.RequestKey(), err)
	}
}

// effectiveCommercialShell 用导出 API 造一版已生效的商业版本壳（客户合同一类在这里只当闭包的采用依据，不登正文）。
func effectiveCommercialShell(t *testing.T, tenant pcdomain.TenantID, kind pcdomain.CommercialObjectKind, objectID string, scope pcdomain.CommercialScopeReference) pcdomain.CommercialVersion {
	t.Helper()
	interval, err := pcdomain.NewEffectiveInterval(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Time{})
	if err != nil {
		t.Fatalf("有效区间：%v", err)
	}
	draft, err := pcdomain.NewCommercialDraft(pcdomain.CommercialVersionSpec{
		TenantID:      tenant,
		Kind:          kind,
		ObjectID:      mustValue(t, pcdomain.NewCommercialObjectID, objectID),
		Version:       mustValue(t, pcdomain.NewCommercialVersionLabel, "v1"),
		Scope:         scope,
		ContentDigest: mustValue(t, pcdomain.NewCommercialContentDigest, "sha256:"+objectID),
		Effective:     interval,
	})
	if err != nil {
		t.Fatalf("商业草稿：%v", err)
	}
	approvedAt := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	basis, err := pcdomain.NewApprovalBasis(
		mustValue(t, pcdomain.NewApprovalReference, "SYN-APPROVAL-"+objectID),
		mustValue(t, pcdomain.NewCommercialSourceReference, "SYN-SOURCE-"+objectID),
		approvedAt,
	)
	if err != nil {
		t.Fatalf("批准依据：%v", err)
	}
	published, err := draft.Publish(basis, pcdomain.ApprovalRoleConfirmed, approvedAt, nil)
	if err != nil {
		t.Fatalf("发布：%v", err)
	}
	live, err := published.TakeEffect(approvedAt)
	if err != nil {
		t.Fatalf("生效：%v", err)
	}
	return live
}

// effectiveCustomerServiceRule 用导出 API 造一版已生效的客户服务规则壳，范围与锚点落在同一处。
func effectiveCustomerServiceRule(t *testing.T, tenant pcdomain.TenantID, scope pcdomain.CommercialScopeReference) pcdomain.CommercialVersion {
	t.Helper()
	interval, err := pcdomain.NewEffectiveInterval(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Time{})
	if err != nil {
		t.Fatalf("有效区间：%v", err)
	}
	draft, err := pcdomain.NewCommercialDraft(pcdomain.CommercialVersionSpec{
		TenantID:      tenant,
		Kind:          pcdomain.CustomerServiceRuleObject,
		ObjectID:      mustValue(t, pcdomain.NewCommercialObjectID, "SYN-CSR-1"),
		Version:       mustValue(t, pcdomain.NewCommercialVersionLabel, "v1"),
		Scope:         scope,
		ContentDigest: mustValue(t, pcdomain.NewCommercialContentDigest, "sha256:syn-csr-1"),
		Effective:     interval,
	})
	if err != nil {
		t.Fatalf("商业草稿：%v", err)
	}
	approvedAt := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	basis, err := pcdomain.NewApprovalBasis(
		mustValue(t, pcdomain.NewApprovalReference, "SYN-APPROVAL-CSR-1"),
		mustValue(t, pcdomain.NewCommercialSourceReference, "SYN-SOURCE-CSR-1"),
		approvedAt,
	)
	if err != nil {
		t.Fatalf("批准依据：%v", err)
	}
	published, err := draft.Publish(basis, pcdomain.ApprovalRoleConfirmed, approvedAt, nil)
	if err != nil {
		t.Fatalf("发布：%v", err)
	}
	live, err := published.TakeEffect(approvedAt)
	if err != nil {
		t.Fatalf("生效：%v", err)
	}
	return live
}

func mustDeadlineRule(t *testing.T, kind pcdomain.ClaimDeadlineKind, event string, days int, calendar string) pcdomain.ClaimDeadlineRule {
	t.Helper()
	rule, err := pcdomain.NewClaimDeadlineRule(kind,
		mustValue(t, pcdomain.NewDeadlineStartEventReference, event), days,
		mustValue(t, pcdomain.NewBusinessCalendarReference, calendar))
	if err != nil {
		t.Fatalf("索赔期限规则：%v", err)
	}
	return rule
}

func mustMaterialsRule(t *testing.T, claimKind string, materials ...string) pcdomain.MinimumMaterialsRule {
	t.Helper()
	references := make([]pcdomain.MaterialRequirementReference, 0, len(materials))
	for _, material := range materials {
		references = append(references, mustValue(t, pcdomain.NewMaterialRequirementReference, material))
	}
	rule, err := pcdomain.NewMinimumMaterialsRule(mustValue(t, pcdomain.NewClaimKindReference, claimKind), references)
	if err != nil {
		t.Fatalf("最低材料规则：%v", err)
	}
	return rule
}
