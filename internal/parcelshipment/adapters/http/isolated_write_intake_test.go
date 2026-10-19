package shipmenthttp_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	shipmenthttp "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/http"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

const (
	isolatedTenant          = "SYN-TENANT-01"
	isolatedCustomerAccount = "SYN-ACCOUNT-01"
	isolatedSource          = "SYN-SOURCE/admin-web"
	isolatedScopeReference  = "SYN-ADM-SCOPE/shipment-intake"
	isolatedScopeDigest     = "sha256:syn-adm-scope-shipment-intake"
)

// isolatedDraft 是管理台提交页会送的那份草案。测试各例只改其中一处，因此每条断言指向的
// 差异是唯一的。
const isolatedDraft = `{
  "customerShipmentReference": "SYN-CUSTREF-0001",
  "requestedServiceProduct": "国际小包标准",
  "senderRelation": "SYN-SENDER-01",
  "senderAddress": "合成寄件地址",
  "recipientRelation": "SYN-RECIPIENT-01",
  "recipientAddress": "合成收件地址",
  "destinationServiceScope": "SYN-DEST/US",
  "parcels": [
    {
      "customerParcelReference": "SYN-PCLREF-0001",
      "declaredWeightValue": "1.50",
      "declaredWeightUnit": "KG"
    }
  ]
}`

// Covers: ADR-0091 决定二与 ADR-0003 —— 来源信封整组来自注入。报文里没有租户的位置，
// 硬塞一个进去连解码都过不去；铸出来的三维恒为注入值。
//
// 这一条比读面的同名用例更要紧：读面铸不出信封，写面会把这三维**落库**，采信一次自报
// 就在库里留下一行分不清来历的事实。
func TestIsolatedSubmissionIntakeMintsTheEnvelopeAndHasNoPlaceForASelfReportedTenant(t *testing.T) {
	intake := newIsolatedIntake(t, &permittingOwnershipDouble{revision: "SYN-REV-1"})

	command, err := intake.IntakeSubmission(context.Background(), draftRequest(isolatedDraft))
	if err != nil {
		t.Fatalf("接入提交：%v", err)
	}
	if got := command.Identity.TenantID().String(); got != isolatedTenant {
		t.Fatalf("租户 = %q, want %q", got, isolatedTenant)
	}
	if got := command.Identity.CustomerAccountID().String(); got != isolatedCustomerAccount {
		t.Fatalf("客户账户 = %q, want %q", got, isolatedCustomerAccount)
	}
	if got := command.Identity.Source().String(); got != isolatedSource {
		t.Fatalf("来源 = %q, want %q", got, isolatedSource)
	}

	// 报文里根本没有租户这一格：塞进去是未知字段，解码即拒。
	reported := strings.Replace(isolatedDraft, `{`, `{"tenantId":"TENANT-PROD-9",`, 1)
	if _, err := intake.IntakeSubmission(context.Background(), draftRequest(reported)); !errors.Is(err, shipmenthttp.ErrMalformedRequest) {
		t.Fatalf("带自报租户的报文 err = %v, want ErrMalformedRequest——未知字段不得被静默丢弃", err)
	}
}

// Covers: 类型注释第二条 —— 摘要只吃客户声明的引用，不吃本包现签的内部标识。同一份内容
// 重发两次必须得到同一个摘要，否则重放会被判成`接入冲突`；换一处客户声明的内容则必须变。
//
// 这条同时是「内部标识现签」那个取舍的安全网：现签让两次命令的标识不同，若摘要跟着标识
// 走，重放语义就坏了而没有任何别的东西会红。
func TestIsolatedSubmissionIntakeDigestsCustomerContentNotMintedIdentities(t *testing.T) {
	intake := newIsolatedIntake(t, &permittingOwnershipDouble{revision: "SYN-REV-1"})

	first, err := intake.IntakeSubmission(context.Background(), draftRequest(isolatedDraft))
	if err != nil {
		t.Fatalf("首次接入：%v", err)
	}
	replay, err := intake.IntakeSubmission(context.Background(), draftRequest(isolatedDraft))
	if err != nil {
		t.Fatalf("重发同一份草案：%v", err)
	}

	if first.PayloadDigest != replay.PayloadDigest {
		t.Fatalf("同一份内容两次摘要不同（%q / %q）——重放会被判成接入冲突",
			first.PayloadDigest.String(), replay.PayloadDigest.String())
	}
	if first.ShipmentRequestID == replay.ShipmentRequestID {
		t.Fatal("两次现签出同一个内部标识——那说明它是从客户参考派生的，端口注释明禁")
	}
	if first.Identity != replay.Identity {
		t.Fatal("同一份委托参考铸出了不同的来源身份——重放认不出来")
	}

	changed := strings.Replace(isolatedDraft, "合成收件地址", "另一个收件地址", 1)
	other, err := intake.IntakeSubmission(context.Background(), draftRequest(changed))
	if err != nil {
		t.Fatalf("接入改过收件地址的草案：%v", err)
	}
	if other.PayloadDigest == first.PayloadDigest {
		t.Fatal("改了收件地址摘要没变——内容差异进不了摘要，同键异容就检不出冲突")
	}
}

// Covers: 类型注释第三条与 `UC-PS-001` 步骤 3B —— 期望规则修订向归属权威预取，原样带进
// 命令；准入范围取注入的合成值。修订若不取自权威，门禁会在提交时点判`决定已过期`，
// 而那一格与「归属真的未确定」在结果上都是 OWNERSHIP_UNRESOLVED，分不出来。
func TestIsolatedSubmissionIntakeTakesTheExpectedRevisionFromTheOwnershipAuthority(t *testing.T) {
	authority := &permittingOwnershipDouble{revision: "SYN-REV-FROM-AUTHORITY"}
	intake := newIsolatedIntake(t, authority)

	command, err := intake.IntakeSubmission(context.Background(), draftRequest(isolatedDraft))
	if err != nil {
		t.Fatalf("接入提交：%v", err)
	}
	if got := command.ExpectedRevision.String(); got != "SYN-REV-FROM-AUTHORITY" {
		t.Fatalf("期望修订 = %q, want 来自归属权威的那个", got)
	}
	if got := command.AdmissionScope.Digest().String(); got != isolatedScopeDigest {
		t.Fatalf("准入范围摘要 = %q, want %q", got, isolatedScopeDigest)
	}
	if authority.asked != 1 {
		t.Fatalf("问归属 %d 次, want 1", authority.asked)
	}
	if authority.scope.Digest().String() != isolatedScopeDigest {
		t.Fatal("问归属用的范围与带进命令的不是同一个——门禁会判范围不符")
	}
}

// Covers: ADR-0029 的恢复动作分格 —— 归属权威调不通是依赖故障，不得包成
// ErrMalformedRequest。包错了，离线客户端会把一件重试就能好的事出队交给人。
func TestIsolatedSubmissionIntakeDoesNotDressADependencyFailureAsAMalformedRequest(t *testing.T) {
	intake := newIsolatedIntake(t, &permittingOwnershipDouble{err: errors.New("governance register unreachable")})

	_, err := intake.IntakeSubmission(context.Background(), draftRequest(isolatedDraft))
	if err == nil {
		t.Fatal("归属调不通却接入成功了")
	}
	if errors.Is(err, shipmenthttp.ErrMalformedRequest) {
		t.Fatalf("依赖故障被包成了形状错：%v", err)
	}
}

// Covers: 票 psb/17——草案的 requestedServiceProduct 以封闭条目名进服务要求段，随同一次 CanonicalizeSubmission 既进
// 摘要又挑成声明的产品进命令；条目名取既有的字面，所以既有草案的摘要一字不变。
func TestIsolatedSubmissionIntakeCarriesTheRequestedServiceProductFromOneCanonicalization(t *testing.T) {
	intake := newIsolatedIntake(t, &permittingOwnershipDouble{revision: "SYN-REV-1"})

	command, err := intake.IntakeSubmission(context.Background(), draftRequest(isolatedDraft))
	if err != nil {
		t.Fatalf("接入草案：%v", err)
	}
	if got := command.RequestedServiceProduct; got.String() != "国际小包标准" {
		t.Fatalf("命令里的声明产品 = %q, want 国际小包标准", got.String())
	}

	undeclared := strings.Replace(isolatedDraft, `"requestedServiceProduct": "国际小包标准",`, ``, 1)
	plain, err := intake.IntakeSubmission(context.Background(), draftRequest(undeclared))
	if err != nil {
		t.Fatalf("接入不带产品的草案：%v", err)
	}
	if plain.RequestedServiceProduct.Declared() {
		t.Fatalf("没报产品的草案带出了 %q", plain.RequestedServiceProduct.String())
	}
	if plain.PayloadDigest == command.PayloadDigest {
		t.Fatal("报没报产品摘要一样——声明的产品是内容，不进摘要就检不出同键异容")
	}
}

// Covers: pp-seams/05 裁决 3 与完成判据 (3)「接单入口用例断言摘要与内容出自同一次调用」——草案的寄 / 收邮编与国家 / 地区码
// 字段译成按封闭要素名命名的范围条目，随同一次 CanonicalizeSubmission 既进摘要又挑成要素子段进命令：带了邮编摘要就变
// （它是内容），要素子段里就有同一个值（它是内容的那一半）；不带则子段缺席、摘要与从前一字不差——新字段留空不改任何
// 既有草案的摘要。
func TestIsolatedSubmissionIntakeCarriesTheClosedElementsWithTheDigestFromOneCanonicalization(t *testing.T) {
	intake := newIsolatedIntake(t, &permittingOwnershipDouble{revision: "SYN-REV-1"})

	plain, err := intake.IntakeSubmission(context.Background(), draftRequest(isolatedDraft))
	if err != nil {
		t.Fatalf("接入不带要素的草案：%v", err)
	}
	if !plain.DeclaredElements.Empty() {
		t.Fatalf("没报邮编的草案带出了要素 %#v", plain.DeclaredElements)
	}
	blank := strings.Replace(isolatedDraft, `"recipientAddress"`,
		`"recipientPostalCode": "", "senderPostalCode": "   ", "recipientAddress"`, 1)
	blanked, err := intake.IntakeSubmission(context.Background(), draftRequest(blank))
	if err != nil {
		t.Fatalf("接入新字段留空的草案：%v", err)
	}
	if blanked.PayloadDigest != plain.PayloadDigest || !blanked.DeclaredElements.Empty() {
		t.Fatal("新字段留空改了摘要或长出了要素——缺席的字段整条不进（canonicalEntries 头注）")
	}

	declared := strings.Replace(isolatedDraft, `"recipientAddress"`,
		`"recipientPostalCode": "SYN-100115", "recipientCountryCode": "SYN-CC", "senderPostalCode": "SYN-200000", "recipientAddress"`, 1)
	command, err := intake.IntakeSubmission(context.Background(), draftRequest(declared))
	if err != nil {
		t.Fatalf("接入带要素的草案：%v", err)
	}
	if command.PayloadDigest == plain.PayloadDigest {
		t.Fatal("报了邮编摘要没变——要素是内容，不进摘要就检不出同键异容")
	}
	destination := command.DeclaredElements.InGroup(domain.DeliveryPlaceDataGroup())
	if postal, present := destination.PostalCode(); !present || postal != "SYN-100115" {
		t.Fatalf("收件邮编 = %q present = %v, want SYN-100115", postal, present)
	}
	if country, present := destination.CountryCode(); !present || country != "SYN-CC" {
		t.Fatalf("收件国家 / 地区码 = %q present = %v, want SYN-CC", country, present)
	}
	origin := command.DeclaredElements.InGroup(domain.SenderPlaceDataGroup())
	if postal, present := origin.PostalCode(); !present || postal != "SYN-200000" {
		t.Fatalf("寄件邮编 = %q present = %v", postal, present)
	}
	if _, present := origin.CountryCode(); present {
		t.Fatal("寄件范围没报国家 / 地区码却在场")
	}

	// 同一份草案重发，摘要与要素都一样：两样出自同一次规范化，不因现签标识而变。
	replay, err := intake.IntakeSubmission(context.Background(), draftRequest(declared))
	if err != nil {
		t.Fatalf("重发带要素的草案：%v", err)
	}
	if replay.PayloadDigest != command.PayloadDigest || replay.DeclaredElements != command.DeclaredElements {
		t.Fatal("同一份草案两次接入得到的摘要或要素不同")
	}
}

// Covers: 形状级失败一律 4xx。三种缺失各测一次——它们缺的是不同的东西，而每一种都不该
// 让编排收到一条立不起来的命令。
func TestIsolatedSubmissionIntakeRefusesDraftsThatCannotFormACommand(t *testing.T) {
	for name, body := range map[string]string{
		"委托参考为空": strings.Replace(isolatedDraft, `"SYN-CUSTREF-0001"`, `""`, 1),
		"一件包裹都没有": strings.Replace(isolatedDraft,
			`"parcels": [`, `"parcels": [], "ignored": [`, 1),
		"生效时间不是时间": strings.Replace(isolatedDraft,
			`"requestedServiceProduct"`, `"requestEffectiveAt": "下周三", "requestedServiceProduct"`, 1),
		"报文不是 JSON": `{`,
	} {
		t.Run(name, func(t *testing.T) {
			intake := newIsolatedIntake(t, &permittingOwnershipDouble{revision: "SYN-REV-1"})
			if _, err := intake.IntakeSubmission(context.Background(), draftRequest(body)); !errors.Is(err, shipmenthttp.ErrMalformedRequest) {
				t.Fatalf("err = %v, want ErrMalformedRequest", err)
			}
		})
	}
}

// Covers: 票 product-strategy-boundary/26——草案上已声明的重量编成成员画像，成员是同一下标现签的内部包裹标识。
// 重量缺席不立画像、不补默认测量。尺寸在场才带外廓。服务条目仍进摘要：拿掉重量摘要要变。
func TestIsolatedSubmissionIntakeCarriesAProfileOnlyWhenWeightIsDeclared(t *testing.T) {
	intake := newIsolatedIntake(t, &permittingOwnershipDouble{revision: "SYN-REV-1"})

	command, err := intake.IntakeSubmission(context.Background(), draftRequest(isolatedDraft))
	if err != nil {
		t.Fatalf("接入带重量的草案：%v", err)
	}
	if len(command.DeclaredProfiles) != 1 {
		t.Fatalf("画像 %d 份, want 1", len(command.DeclaredProfiles))
	}
	profile := command.DeclaredProfiles[0]
	if len(command.DeclaredParcelIDs) != 1 || profile.Parcel() != command.DeclaredParcelIDs[0] {
		t.Fatalf("画像成员 = %s, 现签成员 = %v——画像必须挂在同一下标的内部标识上",
			profile.Parcel(), command.DeclaredParcelIDs)
	}
	weight := profile.Measurement().Weight()
	if weight.Value().String() != "1.50" || weight.Unit().String() != "KG" {
		t.Fatalf("重量 = %s %s, want 1.50 KG", weight.Value(), weight.Unit())
	}
	if _, present := profile.Measurement().Dimensions(); present {
		t.Fatal("没报尺寸的草案带出了外廓")
	}

	sized := strings.Replace(isolatedDraft, `"declaredWeightUnit": "KG"`,
		`"declaredWeightUnit": "KG", "declaredLength": "20", "declaredWidth": "15", "declaredHeight": "10", "declaredDimensionsUnit": "CM"`, 1)
	withSize, err := intake.IntakeSubmission(context.Background(), draftRequest(sized))
	if err != nil {
		t.Fatalf("接入带尺寸的草案：%v", err)
	}
	if len(withSize.DeclaredProfiles) != 1 {
		t.Fatalf("带尺寸的画像 %d 份, want 1", len(withSize.DeclaredProfiles))
	}
	dimensions, present := withSize.DeclaredProfiles[0].Measurement().Dimensions()
	if !present || dimensions.Length().String() != "20" || dimensions.Width().String() != "15" ||
		dimensions.Height().String() != "10" || dimensions.Unit().String() != "CM" {
		t.Fatalf("外廓 = %s×%s×%s %s present=%v, want 20×15×10 CM",
			dimensions.Length(), dimensions.Width(), dimensions.Height(), dimensions.Unit(), present)
	}
	if withSize.PayloadDigest == command.PayloadDigest {
		t.Fatal("报了尺寸摘要没变——尺寸仍要留在服务条目里")
	}

	unweighed := strings.Replace(isolatedDraft,
		`,`+"\n"+`      "declaredWeightValue": "1.50",`+"\n"+`      "declaredWeightUnit": "KG"`, ``, 1)
	plain, err := intake.IntakeSubmission(context.Background(), draftRequest(unweighed))
	if err != nil {
		t.Fatalf("接入没报重量的草案：%v", err)
	}
	if len(plain.DeclaredProfiles) != 0 {
		t.Fatalf("没报重量却交出 %d 份画像", len(plain.DeclaredProfiles))
	}
	if plain.PayloadDigest == command.PayloadDigest {
		t.Fatal("拿掉重量摘要没变——服务条目不该跟着画像一起消失")
	}
}

func newIsolatedIntake(t *testing.T, authority *permittingOwnershipDouble) *shipmenthttp.IsolatedSubmissionIntake {
	t.Helper()
	intake, err := shipmenthttp.NewIsolatedSubmissionIntake(shipmenthttp.IsolatedSubmissionIntakeDeps{
		Tenant:          isolatedTenant,
		CustomerAccount: isolatedCustomerAccount,
		Source:          isolatedSource,
		ScopeReference:  isolatedScopeReference,
		ScopeDigest:     isolatedScopeDigest,
		Ownership:       authority,
		Clock:           frozenClock{at: time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC)},
	})
	if err != nil {
		t.Fatalf("构造隔离提交 Intake：%v", err)
	}
	return intake
}

func draftRequest(body string) *http.Request {
	return httptest.NewRequest(http.MethodPost, "/shipment-requests", strings.NewReader(body))
}

type frozenClock struct{ at time.Time }

func (clock frozenClock) Now() time.Time { return clock.at }

// permittingOwnershipDouble 交回一个带指定修订的归属决定。本包的用例只关心「修订从哪来」
// 与「问了几次」，归属判定本身归 pilotgovernance 适配器的用例。
type permittingOwnershipDouble struct {
	revision string
	err      error
	asked    int
	scope    domain.AdmissionScope
}

func (double *permittingOwnershipDouble) DecideProductionOwnership(
	_ context.Context,
	scope domain.AdmissionScope,
) (domain.ProductionOwnershipDecision, error) {
	double.asked++
	double.scope = scope
	if double.err != nil {
		return domain.ProductionOwnershipDecision{}, double.err
	}
	at := time.Date(2026, 9, 2, 11, 0, 0, 0, time.UTC)
	validity, err := domain.NewOwnershipValidityInterval(at.Add(-time.Hour), at.Add(24*time.Hour))
	if err != nil {
		return domain.ProductionOwnershipDecision{}, err
	}
	decisionID, err := domain.NewProductionOwnershipDecisionID("SYN-OWN-DEC-1")
	if err != nil {
		return domain.ProductionOwnershipDecision{}, err
	}
	ruleVersion, err := domain.NewProductionOwnershipRuleVersion("SYN-OWN-RULE-1")
	if err != nil {
		return domain.ProductionOwnershipDecision{}, err
	}
	revision, err := domain.NewProductionOwnershipRevision(double.revision)
	if err != nil {
		return domain.ProductionOwnershipDecision{}, err
	}
	return domain.NewProductionOwnershipDecision(domain.ProductionOwnershipDecisionSpec{
		DecisionID:       decisionID,
		Scope:            scope,
		Authority:        domain.ProductionAuthorityIDPParcel,
		AdmissionControl: domain.AdmissionControlOpen,
		RuleVersion:      ruleVersion,
		AsOf:             at,
		Validity:         validity,
		Revision:         revision,
		DecisionAt:       at,
	})
}

// Covers: ADR-0184 决定四——逐件的客户包裹参考按次序与现签的内部成员配成客户包裹件号带进命令：号码是客户给的
// 那个，包裹是本包现签的那个，不是反过来（客户参考号不得变成内部标识）。
func TestIsolatedSubmissionIntakePairsCustomerParcelReferencesWithMintedMembers(t *testing.T) {
	intake := newIsolatedIntake(t, &permittingOwnershipDouble{revision: "SYN-REV-1"})
	twoParcels := strings.Replace(isolatedDraft, `"parcels": [`, `"parcels": [
    {"customerParcelReference": "SYN-PCLREF-0000"},`, 1)

	command, err := intake.IntakeSubmission(context.Background(), draftRequest(twoParcels))
	if err != nil {
		t.Fatalf("接入草案：%v", err)
	}
	if len(command.CustomerParcelNumbers) != 2 || len(command.DeclaredParcelIDs) != 2 {
		t.Fatalf("配对 %d 组、成员 %d 件，want 各 2", len(command.CustomerParcelNumbers), len(command.DeclaredParcelIDs))
	}
	for index, want := range []string{"SYN-PCLREF-0000", "SYN-PCLREF-0001"} {
		declaration := command.CustomerParcelNumbers[index]
		if declaration.Number().String() != want || declaration.Parcel() != command.DeclaredParcelIDs[index] {
			t.Fatalf("第 %d 组 = (%s, %s)，want (%s, %s)", index,
				declaration.Parcel(), declaration.Number(), command.DeclaredParcelIDs[index], want)
		}
		if declaration.Parcel().String() == want {
			t.Fatalf("第 %d 组的包裹就是客户参考——内部标识被客户参考顶替了", index)
		}
	}
}

// Covers: ADR-0184 复核记录「越权风险点 2 的事实更正」——入口去两端空白，定的是委托记下的声明值：客户包裹件号取
// 去过空白的值，载荷摘要与来源身份都与不带空白的同一份草案相同，委托没有第二个声明值。入口只去单元格两端：号码
// 本身的字符一个不动，大小写与内部空格原样——改号码字符是条码规则，属租户取值，入口同样不许做。
func TestIsolatedSubmissionIntakeTrimsOnlyTheEndsOfCustomerReferences(t *testing.T) {
	intake := newIsolatedIntake(t, &permittingOwnershipDouble{revision: "SYN-REV-1"})
	plain, err := intake.IntakeSubmission(context.Background(), draftRequest(isolatedDraft))
	if err != nil {
		t.Fatalf("接入不带空白的草案：%v", err)
	}

	padded := strings.NewReplacer(
		`"SYN-CUSTREF-0001"`, `" \tSYN-CUSTREF-0001\t "`,
		`"SYN-PCLREF-0001"`, `"  \tSYN-PCLREF-0001\t "`,
	).Replace(isolatedDraft)
	command, err := intake.IntakeSubmission(context.Background(), draftRequest(padded))
	if err != nil {
		t.Fatalf("接入两端带空白的草案：%v", err)
	}
	if got := customerNumbersOf(command); !slices.Equal(got, []string{"SYN-PCLREF-0001"}) {
		t.Fatalf("客户包裹件号 = %q, want [SYN-PCLREF-0001]——两端空白没去", got)
	}
	if command.PayloadDigest != plain.PayloadDigest {
		t.Fatal("两端带空白的草案摘要变了——摘要吃到的不是去过空白的那份声明值，与册里的号码各说各话")
	}
	if command.Identity != plain.Identity {
		t.Fatal("两端带空白的委托参考铸出了另一个来源身份")
	}

	verbatim := strings.Replace(isolatedDraft, `"SYN-PCLREF-0001"`, `" \tsyn-pclref 0001 \t"`, 1)
	cased, err := intake.IntakeSubmission(context.Background(), draftRequest(verbatim))
	if err != nil {
		t.Fatalf("接入小写带内部空格的草案：%v", err)
	}
	if got := customerNumbersOf(cased); !slices.Equal(got, []string{"syn-pclref 0001"}) {
		t.Fatalf("客户包裹件号 = %q, want [syn-pclref 0001]——入口只去两端，号码本身不许折算", got)
	}
}

func customerNumbersOf(command application.SubmitShipmentRequestCommand) []string {
	numbers := make([]string, len(command.CustomerParcelNumbers))
	for index, declaration := range command.CustomerParcelNumbers {
		numbers[index] = declaration.Number().String()
	}
	return numbers
}
