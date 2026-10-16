package domain_test

import (
	"errors"
	"testing"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

// 本文件证外部标识关联的领域面（ADR-0184）：客户包裹件号挂在委托上、首次提交时固定，随委托接受
// 形成关联；接受前与拒绝都不形成。夹具全为合成引用（S 级）。

func customerNumber(t *testing.T, parcel, number string) domain.CustomerParcelNumberDeclaration {
	t.Helper()
	declaration, err := domain.DeclareCustomerParcelNumber(
		mustValue(t, domain.NewDeclaredParcelID, parcel),
		mustValue(t, domain.NewExternalIdentifierNumber, number),
	)
	if err != nil {
		t.Fatalf("客户包裹件号 %s→%s：%v", parcel, number, err)
	}
	return declaration
}

func submittedWithCustomerNumbers(t *testing.T, declarations ...domain.CustomerParcelNumberDeclaration) domain.ShipmentRequest {
	t.Helper()
	spec := submitSpec(t, "parcel-1", "parcel-2")
	spec.CustomerParcelNumbers = declarations
	request, err := domain.SubmitShipmentRequest(spec)
	if err != nil {
		t.Fatalf("建单：%v", err)
	}
	return request
}

func identityOfFixture(t *testing.T) domain.SourceIdentity {
	t.Helper()
	identity, err := domain.NewSourceIdentity(
		mustValue(t, domain.NewTenantID, "tenant-1"),
		mustValue(t, domain.NewCustomerAccountID, "customer-1"),
		mustValue(t, domain.NewSource, "source-a"),
		mustValue(t, domain.NewSourceRequestKey, "key-1"),
	)
	if err != nil {
		t.Fatalf("来源身份：%v", err)
	}
	return identity
}

// Covers: PS CONTEXT「外部标识关联只在被标识对象成为正式对象时形成」与 ADR-0184 决定三——接受时每件包裹的
// 客户包裹件号各形成一条：类型客户包裹件号、分配方与适用范围都是该委托的客户账户、依据是接受决定、形成
// 时刻是基线固定时刻；没给号的成员不形成。
func TestAnAcceptedRequestFormsOneRelationPerCustomerParcelNumber(t *testing.T) {
	request := submittedWithCustomerNumbers(t, customerNumber(t, "parcel-1", "CUST-0001"))
	accepted, err := request.Decide(decisionSpec(t, allGroupsPassing(t)))
	if err != nil {
		t.Fatalf("接受：%v", err)
	}
	relations := accepted.ExternalIdentifierRelationsFormedByAcceptance(identityOfFixture(t))
	if len(relations) != 1 {
		t.Fatalf("relations = %d, want 1（parcel-2 没给号）", len(relations))
	}
	relation := relations[0]
	if relation.Tenant().String() != "tenant-1" ||
		relation.Kind() != domain.CustomerParcelNumber ||
		relation.Allocator().String() != "customer-1" ||
		relation.Scope().String() != "customer-1" ||
		relation.Number().String() != "CUST-0001" ||
		relation.Parcel().String() != "parcel-1" ||
		relation.Basis().String() != "decision-1" {
		t.Fatalf("relation = %+v", relation)
	}
	baseline, _ := accepted.AcceptanceBaseline()
	if !relation.FormedAt().Equal(baseline.FixedAt()) {
		t.Fatalf("formed at %v, want baseline fixed at %v", relation.FormedAt(), baseline.FixedAt())
	}
}

// Covers: 接受前的声明包裹不是正式包裹——已提交不形成；被拒绝的委托也不形成。
func TestNoRelationIsFormedBeforeAcceptanceOrOnRejection(t *testing.T) {
	request := submittedWithCustomerNumbers(t, customerNumber(t, "parcel-1", "CUST-0001"))
	if relations := request.ExternalIdentifierRelationsFormedByAcceptance(identityOfFixture(t)); len(relations) != 0 {
		t.Fatalf("已提交即形成了 %d 条", len(relations))
	}
	checks := allGroupsPassing(t)
	checks[0] = versionCheck(t, domain.CustomerRelationshipCheck, domain.CheckFailed, "NO_RELATIONSHIP")
	rejected, err := request.Decide(decisionSpec(t, checks))
	if err != nil {
		t.Fatalf("拒绝：%v", err)
	}
	if rejected.State() != domain.ShipmentRequestRejected {
		t.Fatalf("state = %q, want REJECTED", rejected.State())
	}
	if relations := rejected.ExternalIdentifierRelationsFormedByAcceptance(identityOfFixture(t)); len(relations) != 0 {
		t.Fatalf("已拒绝形成了 %d 条", len(relations))
	}
}

// Covers: ADR-0184 决定四——配对指着成员集合外、或一件两个号即立不起来；两件同号不拒（越权风险点 4）。
func TestCustomerParcelNumbersMustNameMembersOnce(t *testing.T) {
	for name, declarations := range map[string][]domain.CustomerParcelNumberDeclaration{
		"成员集合外": {customerNumber(t, "parcel-9", "CUST-0009")},
		"一件两个号": {customerNumber(t, "parcel-1", "CUST-0001"), customerNumber(t, "parcel-1", "CUST-0002")},
	} {
		spec := submitSpec(t, "parcel-1", "parcel-2")
		spec.CustomerParcelNumbers = declarations
		if _, err := domain.SubmitShipmentRequest(spec); !errors.Is(err, domain.ErrInvalidCustomerParcelNumber) {
			t.Fatalf("%s：err = %v, want ErrInvalidCustomerParcelNumber", name, err)
		}
	}
	spec := submitSpec(t, "parcel-1", "parcel-2")
	spec.CustomerParcelNumbers = []domain.CustomerParcelNumberDeclaration{
		customerNumber(t, "parcel-1", "SAME"), customerNumber(t, "parcel-2", "SAME"),
	}
	request, err := domain.SubmitShipmentRequest(spec)
	if err != nil {
		t.Fatalf("两件同号被拒：%v", err)
	}
	if got := request.CustomerParcelNumbers(); len(got) != 2 {
		t.Fatalf("customer parcel numbers = %v", got)
	}
}

// Covers: 号码逐字保存，不做大小写或空白折算（ADR-0184 决定五）；空号码立不起来。
func TestAnExternalIdentifierNumberIsKeptVerbatim(t *testing.T) {
	number := mustValue(t, domain.NewExternalIdentifierNumber, " ab-01 ")
	if number.String() != " ab-01 " {
		t.Fatalf("number = %q", number.String())
	}
	if _, err := domain.NewExternalIdentifierNumber("  "); err == nil {
		t.Fatal("空号码也立得起来")
	}
	if _, err := domain.DeclareCustomerParcelNumber(domain.DeclaredParcelID{}, number); err == nil {
		t.Fatal("没有包裹的配对也立得起来")
	}
}

// Covers: 新提交版本不重报号码，委托上的配对原样带过去（ADR-0184 决定四）。
func TestANewSubmissionVersionKeepsTheCustomerParcelNumbers(t *testing.T) {
	request := submittedWithCustomerNumbers(t, customerNumber(t, "parcel-2", "CUST-0002"))
	superseded, err := request.FormNewSubmissionVersion(domain.NewSubmissionVersionSpec{
		VersionID:        mustValue(t, domain.NewSubmissionVersionID, "version-2"),
		TaskID:           mustValue(t, domain.NewAcceptanceDecisionTaskID, "task-2"),
		SourceSubmission: sourceFingerprint(t, "tenant-1", "customer-1", "source-a", "key-2", "digest-2"),
		DeclaredParcelIDs: []domain.DeclaredParcelID{
			mustValue(t, domain.NewDeclaredParcelID, "parcel-1"),
			mustValue(t, domain.NewDeclaredParcelID, "parcel-2"),
		},
		EstablishedAt: submittedAt.Add(1),
	})
	if err != nil {
		t.Fatalf("新提交版本：%v", err)
	}
	got := superseded.CustomerParcelNumbers()
	if len(got) != 1 || got[0].Parcel().String() != "parcel-2" || got[0].Number().String() != "CUST-0002" {
		t.Fatalf("customer parcel numbers = %+v", got)
	}
}

// Covers: 重建门把委托上的客户包裹件号原样带回；指着成员集合外的配对是坏数据，按重建拒收答，不折成「本期不支持」。
func TestRehydrationCarriesCustomerParcelNumbersAndRefusesAStrayOne(t *testing.T) {
	snapshot := submittedSnapshot(t)
	snapshot.CustomerParcelNumbers = []domain.CustomerParcelNumberDeclaration{customerNumber(t, "parcel-1", "CUST-0001")}
	request, err := domain.RehydrateShipmentRequest(snapshot)
	if err != nil {
		t.Fatalf("重建：%v", err)
	}
	if got := request.CustomerParcelNumbers(); len(got) != 1 || got[0].Number().String() != "CUST-0001" {
		t.Fatalf("customer parcel numbers = %+v", got)
	}
	snapshot.CustomerParcelNumbers = []domain.CustomerParcelNumberDeclaration{customerNumber(t, "parcel-9", "CUST-0009")}
	if _, err := domain.RehydrateShipmentRequest(snapshot); !errors.Is(err, domain.ErrInvalidRehydratedShipmentRequest) {
		t.Fatalf("err = %v, want ErrInvalidRehydratedShipmentRequest", err)
	}
}
