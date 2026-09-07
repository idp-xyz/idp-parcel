package postgres_test

import (
	"testing"
	"time"

	adapter "go.idp.xyz/idp-parcel/internal/customscompliance/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
)

// 本文件对真实 PostgreSQL 16 证按正式包裹反查申报链的读面（票 ps-port-remainder/05 的 CC
// 半边）。播种走显式 SQL（理由见 viewFixture 头注）；三格独立各证一次，再证它们不互相折叠
// ——折叠正是这只读口要挡的事：哪一格压过哪一格归 parcel-shipment 判。

func newParcelDeclarationFactsView(t *testing.T) (*adapter.ParcelDeclarationFactsView, *viewFixture) {
	t.Helper()
	fixture := newViewFixture(t)
	view, err := adapter.NewParcelDeclarationFactsView(fixture.db)
	if err != nil {
		t.Fatalf("构造包裹申报事实读口：%v", err)
	}
	return view, fixture
}

func loadParcelDeclarationFacts(
	t *testing.T,
	view *adapter.ParcelDeclarationFactsView,
	tenant, parcel string,
) ports.ParcelDeclarationFacts {
	t.Helper()
	facts, err := view.LoadParcelDeclarationFacts(t.Context(),
		viewValue(t, domain.NewTenantID, tenant),
		viewValue(t, domain.NewDeclaredParcelReference, parcel))
	if err != nil {
		t.Fatalf("读包裹申报事实：%v", err)
	}
	return facts
}

// seedCase 落一个案件行（单元表外键要它在）。parcels 是案件与包裹的直接关联。
func (fixture *viewFixture) seedCase(t *testing.T, tenant, caseID string, parcels ...string) {
	t.Helper()
	fixture.seed(t,
		`INSERT INTO customs_compliance.customs_case
			(tenant_id, jurisdiction_ref, direction, procedure_ref, obligation_ref,
			 case_id, parcels, roles, established_at)
		 VALUES ($1, 'CN', 'EXPORT', 'PROC-'||$2, 'OBL-'||$2, $2,
		         (SELECT jsonb_agg(jsonb_build_object('parcel', p, 'customer', 'CUST-1', 'sourceRef', 'SRC-'||p))
		            FROM unnest($3::text[]) AS p),
		         '[]'::jsonb, $4)`,
		tenant, caseID, parcels, viewBaseAt)
}

// seedUnit 落一个申报单元行，成员按字典序（与写口同口径）。
func (fixture *viewFixture) seedUnit(t *testing.T, tenant, unitID, caseID string, members ...string) {
	t.Helper()
	fixture.seed(t,
		`INSERT INTO customs_compliance.declaration_unit
			(tenant_id, unit_id, case_id, procedure_ref, members, formed_at)
		 VALUES ($1, $2, $3, 'PROC-'||$3,
		         (SELECT jsonb_agg(m ORDER BY m) FROM unnest($4::text[]) AS m), $5)`,
		tenant, unitID, caseID, members, viewBaseAt)
}

// seedVersion 为单元落一个已固定的提交版本，组成快照独立给出——版本固定时的组成才是
// 交出去的那一份，读面按它反查而不经单元。
func (fixture *viewFixture) seedVersion(t *testing.T, tenant, unitID, caseID, versionID string, members ...string) {
	t.Helper()
	fixture.seed(t,
		`INSERT INTO customs_compliance.declaration_submission
			(tenant_id, unit_id, procedure_ref, version_id, content_digest, members,
			 dossier_ref, roles_ref, readiness_basis, authority_ref, fixed_at, recorded_at, is_current)
		 VALUES ($1, $2, 'PROC-'||$3, $4, 'digest-'||$4,
		         (SELECT jsonb_agg(m ORDER BY m) FROM unnest($5::text[]) AS m),
		         'DOSSIER-'||$4, 'ROLES-'||$4, 'READY-'||$4, 'AUTH-'||$4, $6, $6, true)`,
		tenant, unitID, caseID, versionID, members, viewBaseAt)
}

// seedClosure 落一份关闭记录；reopenings 由调用方给（'[]' 即当前已关闭）。
func (fixture *viewFixture) seedClosure(t *testing.T, tenant, caseID, reopenings string) {
	t.Helper()
	fixture.seed(t,
		`INSERT INTO customs_compliance.case_closure
			(tenant_id, case_ref, cutoff_at, verified_at, decided_by, closed_at, items, reopenings)
		 VALUES ($1, $2, $3, $3, 'OFFICER-1', $4,
		         '[{"obligation":"DECLARE","scope":"ALL","state":"CONCLUDED","basis":"B-1"}]'::jsonb,
		         $5::jsonb)`,
		tenant, caseID, viewBaseAt, viewBaseAt.Add(time.Hour), reopenings)
}

func assertFacts(t *testing.T, got, want ports.ParcelDeclarationFacts) {
	t.Helper()
	if got != want {
		t.Fatalf("facts = %+v, want %+v", got, want)
	}
}

// 空册三格皆否——那是如实答案「关务对这个包裹没有任何单元、版本或案件」，不是「答不出」。
// `不知道`归消费方的未接适配器，本读面不出它。
func TestParcelDeclarationFactsAreAllAbsentOnAnEmptyRegister(t *testing.T) {
	view, _ := newParcelDeclarationFactsView(t)
	assertFacts(t, loadParcelDeclarationFacts(t, view, "tenant-a", "PARCEL-1"), ports.ParcelDeclarationFacts{})
}

// 单元成员而尚无提交版本：形成中那一格在，另两格不在。
func TestAParcelInAUnitWithoutAVersionIsAMemberOfAnUnsubmittedUnit(t *testing.T) {
	view, fixture := newParcelDeclarationFactsView(t)
	fixture.seedCase(t, "tenant-a", "CASE-1", "PARCEL-1")
	fixture.seedUnit(t, "tenant-a", "UNIT-1", "CASE-1", "PARCEL-1", "PARCEL-2")

	assertFacts(t, loadParcelDeclarationFacts(t, view, "tenant-a", "PARCEL-1"),
		ports.ParcelDeclarationFacts{MemberOfUnsubmittedUnit: true})
	// 同单元的另一成员同样在形成中；不在任何单元里的包裹三格皆否。
	assertFacts(t, loadParcelDeclarationFacts(t, view, "tenant-a", "PARCEL-2"),
		ports.ParcelDeclarationFacts{MemberOfUnsubmittedUnit: true})
	assertFacts(t, loadParcelDeclarationFacts(t, view, "tenant-a", "PARCEL-9"), ports.ParcelDeclarationFacts{})
}

// 版本一旦固定，同一个单元就不再是「尚无提交版本」；已提交那一格按版本的组成快照答。
func TestAFixedVersionMovesTheParcelFromFormingToSubmitted(t *testing.T) {
	view, fixture := newParcelDeclarationFactsView(t)
	fixture.seedCase(t, "tenant-a", "CASE-1", "PARCEL-1")
	fixture.seedUnit(t, "tenant-a", "UNIT-1", "CASE-1", "PARCEL-1")
	fixture.seedVersion(t, "tenant-a", "UNIT-1", "CASE-1", "VERSION-1", "PARCEL-1")

	assertFacts(t, loadParcelDeclarationFacts(t, view, "tenant-a", "PARCEL-1"),
		ports.ParcelDeclarationFacts{InFixedSubmissionVersion: true})
}

// 三格独立：一个包裹同时在一个已固定版本的单元与一个尚无版本的单元里，两格同时为真。
// 读面不折成一格——哪一格压过哪一格是 parcel-shipment 的判断（domain.JudgeAmendmentStage）。
func TestFactsAreNotCollapsedWhenAParcelSitsInTwoUnits(t *testing.T) {
	view, fixture := newParcelDeclarationFactsView(t)
	fixture.seedCase(t, "tenant-a", "CASE-1", "PARCEL-1")
	fixture.seedCase(t, "tenant-a", "CASE-2", "PARCEL-1")
	fixture.seedUnit(t, "tenant-a", "UNIT-1", "CASE-1", "PARCEL-1")
	fixture.seedVersion(t, "tenant-a", "UNIT-1", "CASE-1", "VERSION-1", "PARCEL-1")
	fixture.seedUnit(t, "tenant-a", "UNIT-2", "CASE-2", "PARCEL-1")

	assertFacts(t, loadParcelDeclarationFacts(t, view, "tenant-a", "PARCEL-1"),
		ports.ParcelDeclarationFacts{MemberOfUnsubmittedUnit: true, InFixedSubmissionVersion: true})
}

// 「已关闭」两路都通：经当前单元的案件维，或案件建立时的直接包裹关联——后者在没有任何
// 申报单元时也成立（案件先于申报存在）。
func TestAClosedCaseIsSeenThroughEitherTheUnitOrTheDirectAssociation(t *testing.T) {
	view, fixture := newParcelDeclarationFactsView(t)
	// 经单元：案件直接关联里没有 PARCEL-1，只有单元把它带进案件。
	fixture.seedCase(t, "tenant-a", "CASE-1", "PARCEL-OTHER")
	fixture.seedUnit(t, "tenant-a", "UNIT-1", "CASE-1", "PARCEL-1")
	fixture.seedVersion(t, "tenant-a", "UNIT-1", "CASE-1", "VERSION-1", "PARCEL-1")
	fixture.seedClosure(t, "tenant-a", "CASE-1", `[]`)
	// 直接关联：案件列着 PARCEL-2，没有任何单元。
	fixture.seedCase(t, "tenant-a", "CASE-2", "PARCEL-2")
	fixture.seedClosure(t, "tenant-a", "CASE-2", `[]`)

	assertFacts(t, loadParcelDeclarationFacts(t, view, "tenant-a", "PARCEL-1"),
		ports.ParcelDeclarationFacts{InFixedSubmissionVersion: true, InClosedCase: true})
	assertFacts(t, loadParcelDeclarationFacts(t, view, "tenant-a", "PARCEL-2"),
		ports.ParcelDeclarationFacts{InClosedCase: true})
}

// 受控重开后案件处于「重新打开」，不再读作已关闭；原关闭记录还在，但它撑不起「已关闭」
// 这一格。
func TestAReopenedCaseIsNotReadAsClosed(t *testing.T) {
	view, fixture := newParcelDeclarationFactsView(t)
	fixture.seedCase(t, "tenant-a", "CASE-1", "PARCEL-1")
	fixture.seedClosure(t, "tenant-a", "CASE-1",
		`[{"lateFact":"LATE-1","affectedItems":["DECLARE"],"authority":"AUTH-1","reopenedAt":"2026-08-15T09:00:00Z"}]`)

	assertFacts(t, loadParcelDeclarationFacts(t, view, "tenant-a", "PARCEL-1"), ports.ParcelDeclarationFacts{})
}

// 被替代的单元不再是这个包裹的「形成中」：原申报单元不能继续使用。它已固定的版本仍算
// ——交出去的那一份不因替代而消失。
func TestAReplacedUnitNoLongerCountsAsForming(t *testing.T) {
	view, fixture := newParcelDeclarationFactsView(t)
	fixture.seedCase(t, "tenant-a", "CASE-1", "PARCEL-1")
	fixture.seedCase(t, "tenant-a", "CASE-2", "PARCEL-1")
	fixture.seedUnit(t, "tenant-a", "UNIT-OLD", "CASE-1", "PARCEL-1")
	// 替代单元落在另一个案件下，且这一次不含 PARCEL-1：替代关系今天没有写入方，直接落列。
	fixture.seed(t,
		`INSERT INTO customs_compliance.declaration_unit
			(tenant_id, unit_id, case_id, procedure_ref, members, formed_at, replaces_unit_id)
		 VALUES ('tenant-a', 'UNIT-NEW', 'CASE-2', 'PROC-CASE-2', '["PARCEL-3"]'::jsonb, $1, 'UNIT-OLD')`,
		viewBaseAt.Add(time.Hour))

	assertFacts(t, loadParcelDeclarationFacts(t, view, "tenant-a", "PARCEL-1"), ports.ParcelDeclarationFacts{})

	fixture.seedVersion(t, "tenant-a", "UNIT-OLD", "CASE-1", "VERSION-OLD", "PARCEL-1")
	assertFacts(t, loadParcelDeclarationFacts(t, view, "tenant-a", "PARCEL-1"),
		ports.ParcelDeclarationFacts{InFixedSubmissionVersion: true})
}

// 被替代的死单元连它的案件关闭也不再算作这个包裹「所在案件」——除非案件直接关联着它。
func TestAReplacedUnitDoesNotCarryItsCaseClosureToTheParcel(t *testing.T) {
	view, fixture := newParcelDeclarationFactsView(t)
	fixture.seedCase(t, "tenant-a", "CASE-1", "PARCEL-OTHER")
	fixture.seedCase(t, "tenant-a", "CASE-2", "PARCEL-OTHER")
	fixture.seedUnit(t, "tenant-a", "UNIT-OLD", "CASE-1", "PARCEL-1")
	fixture.seed(t,
		`INSERT INTO customs_compliance.declaration_unit
			(tenant_id, unit_id, case_id, procedure_ref, members, formed_at, replaces_unit_id)
		 VALUES ('tenant-a', 'UNIT-NEW', 'CASE-2', 'PROC-CASE-2', '["PARCEL-1"]'::jsonb, $1, 'UNIT-OLD')`,
		viewBaseAt.Add(time.Hour))
	fixture.seedClosure(t, "tenant-a", "CASE-1", `[]`)

	assertFacts(t, loadParcelDeclarationFacts(t, view, "tenant-a", "PARCEL-1"),
		ports.ParcelDeclarationFacts{MemberOfUnsubmittedUnit: true})
}

func TestParcelDeclarationFactsOfAnotherTenantAreInvisible(t *testing.T) {
	view, fixture := newParcelDeclarationFactsView(t)
	fixture.seedCase(t, "tenant-a", "CASE-1", "PARCEL-1")
	fixture.seedUnit(t, "tenant-a", "UNIT-1", "CASE-1", "PARCEL-1")
	fixture.seedVersion(t, "tenant-a", "UNIT-1", "CASE-1", "VERSION-1", "PARCEL-1")
	fixture.seedClosure(t, "tenant-a", "CASE-1", `[]`)

	assertFacts(t, loadParcelDeclarationFacts(t, view, "tenant-b", "PARCEL-1"), ports.ParcelDeclarationFacts{})
}

// 包裹引用是整值匹配：前缀相同的另一个引用不命中——jsonb 包含按元素相等，不按子串。
func TestParcelDeclarationFactsMatchTheWholeReference(t *testing.T) {
	view, fixture := newParcelDeclarationFactsView(t)
	fixture.seedCase(t, "tenant-a", "CASE-1", "PARCEL-10")
	fixture.seedUnit(t, "tenant-a", "UNIT-1", "CASE-1", "PARCEL-10")

	assertFacts(t, loadParcelDeclarationFacts(t, view, "tenant-a", "PARCEL-1"), ports.ParcelDeclarationFacts{})
	assertFacts(t, loadParcelDeclarationFacts(t, view, "tenant-a", "PARCEL-10"),
		ports.ParcelDeclarationFacts{MemberOfUnsubmittedUnit: true})
}
