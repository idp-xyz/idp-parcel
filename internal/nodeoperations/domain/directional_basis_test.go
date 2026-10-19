package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
)

var basisAt = time.Date(2026, 10, 16, 9, 0, 0, 0, time.UTC)

func observation(t *testing.T, source, parcel string, at time.Time) domain.IntakeIdentityObservation {
	t.Helper()
	observed := domain.IntakeIdentityObservation{
		SourceID:   source,
		Version:    mustValue(t, domain.NewIntakeResultVersion, "v-"+source),
		ReceivedAt: at,
	}
	if parcel != "" {
		observed.Association = mustValue(t, domain.NewParcelAssociationReference, parcel)
	}
	return observed
}

// Covers: 身份按收寄册各行折——同一件实物收过两次且关联同一正式包裹时成立，所依收寄取业务时间最晚的那一行（后一次
// 才是此刻的控制来由）；待识别行不推翻已建立的关联。
func TestIdentityBasisTakesTheLatestIntakeOfOneParcel(t *testing.T) {
	identity, err := domain.JudgeIdentityBasis([]domain.IntakeIdentityObservation{
		observation(t, "src-early", "DPCL-1", basisAt),
		observation(t, "src-pending", "", basisAt.Add(time.Hour)),
		observation(t, "src-late", "DPCL-1", basisAt.Add(2*time.Hour)),
	})
	if err != nil {
		t.Fatalf("judge: %v", err)
	}
	if identity.Cell().Answer() != domain.BasisEstablished || identity.IntakeSourceID() != "src-late" {
		t.Fatalf("identity = %s based on %q", identity.Cell().Answer(), identity.IntakeSourceID())
	}
}

// Covers: 记录的三格之间那一条：身份成立时另两格都问过，不成立时都没问——调用方拼不出一份「身份缺少却问了 PS」的
// 记录，也拼不出「身份成立却没问」的。
func TestADirectionalBasisRecordHoldsTheIdentityGate(t *testing.T) {
	missing, err := domain.JudgeIdentityBasis(nil)
	if err != nil {
		t.Fatalf("identity: %v", err)
	}
	established, err := domain.JudgeIdentityBasis([]domain.IntakeIdentityObservation{observation(t, "src-1", "DPCL-1", basisAt)})
	if err != nil {
		t.Fatalf("identity: %v", err)
	}
	service, err := domain.JudgeServiceResultBasis("", "", "")
	if err != nil {
		t.Fatalf("service: %v", err)
	}
	if _, err := domain.RecordDirectionalBasis(missing, service, domain.NotConfiguredRoutingBasis(), basisAt); !errors.Is(err, domain.ErrInvalidDirectionalBasis) {
		t.Fatalf("asked PS without an identity: err = %v", err)
	}
	if _, err := domain.RecordDirectionalBasisWithoutIdentity(established, basisAt); !errors.Is(err, domain.ErrInvalidDirectionalBasis) {
		t.Fatalf("left an established identity unasked: err = %v", err)
	}
	if _, err := domain.RecordDirectionalBasis(established, service, domain.RoutingBasis{}, basisAt); !errors.Is(err, domain.ErrInvalidDirectionalBasis) {
		t.Fatalf("a zero-value routing cell was accepted: err = %v", err)
	}
	if _, err := domain.RecordDirectionalBasis(established, service, domain.NotConfiguredRoutingBasis(), time.Time{}); !errors.Is(err, domain.ErrInvalidDirectionalBasis) {
		t.Fatalf("a record without a judgment time was accepted: err = %v", err)
	}
	if _, err := domain.JudgeServiceResultBasis("", "RETURNED", ""); !errors.Is(err, domain.ErrInvalidDirectionalBasis) {
		t.Fatalf("half a final was accepted: err = %v", err)
	}
}

// Covers: 重建门过的是与构造同一套规则：合法的一份原样回来，词表外的词、不属其格的原因、引用与原因不同在、身份
// 不成立却问过另两格，各自响亮。
func TestRehydratingADirectionalBasisAppliesTheSameRules(t *testing.T) {
	valid := domain.RehydrateDirectionalBasisSpec{
		Identity:       domain.RehydrateDirectionalBasisCellSpec{Answer: "ESTABLISHED"},
		Association:    "DPCL-1",
		IntakeSourceID: "src-1",
		IntakeVersion:  "v-src-1",
		ServiceResult:  domain.RehydrateDirectionalBasisCellSpec{Answer: "MISSING", Reasons: []string{"FINAL", "CANCELLED"}},
		Cancellation:   "CANCEL-1",
		FinalKind:      "RETURNED",
		FinalVersion:   "FINAL-2",
		Routing:        domain.RehydrateDirectionalBasisCellSpec{Answer: "NOT_CONFIGURED"},
		JudgedAt:       basisAt,
	}
	record, err := domain.RehydrateDirectionalBasis(valid)
	if err != nil {
		t.Fatalf("rehydrate: %v", err)
	}
	if reasons := record.ServiceResult().Cell().Reasons(); len(reasons) != 2 || reasons[0] != domain.ReasonCancelled {
		t.Fatalf("reasons are not kept in vocabulary order: %v", reasons)
	}

	broken := map[string]func(spec *domain.RehydrateDirectionalBasisSpec){
		"词表外的取值": func(spec *domain.RehydrateDirectionalBasisSpec) { spec.Routing.Answer = "COMPATIBLE" },
		"不属其格的原因": func(spec *domain.RehydrateDirectionalBasisSpec) {
			spec.ServiceResult.Reasons = []string{"PENDING_IDENTIFICATION"}
		},
		"已取消却没有取消引用": func(spec *domain.RehydrateDirectionalBasisSpec) { spec.Cancellation = "" },
		"成立却缺所依收寄":   func(spec *domain.RehydrateDirectionalBasisSpec) { spec.IntakeSourceID = "" },
		"重复的原因": func(spec *domain.RehydrateDirectionalBasisSpec) {
			spec.ServiceResult.Reasons = []string{"FINAL", "FINAL"}
			spec.Cancellation = ""
		},
		"身份缺少却问过另两格": func(spec *domain.RehydrateDirectionalBasisSpec) {
			spec.Identity = domain.RehydrateDirectionalBasisCellSpec{Answer: "MISSING", Reasons: []string{"NO_INTAKE_RECORD"}}
			spec.Association, spec.IntakeSourceID, spec.IntakeVersion = "", "", ""
		},
		"身份读不动却带了所依收寄": func(spec *domain.RehydrateDirectionalBasisSpec) {
			spec.Identity = domain.RehydrateDirectionalBasisCellSpec{Answer: "UNREADABLE"}
			spec.ServiceResult = domain.RehydrateDirectionalBasisCellSpec{Answer: "NOT_JUDGED"}
			spec.Cancellation, spec.FinalKind, spec.FinalVersion = "", "", ""
			spec.Routing = domain.RehydrateDirectionalBasisCellSpec{Answer: "NOT_JUDGED"}
		},
	}
	for name, mutate := range broken {
		spec := valid
		spec.ServiceResult.Reasons = append([]string(nil), valid.ServiceResult.Reasons...)
		mutate(&spec)
		if _, err := domain.RehydrateDirectionalBasis(spec); !errors.Is(err, domain.ErrInvalidDirectionalBasis) {
			t.Errorf("%s: err = %v", name, err)
		}
	}
}
