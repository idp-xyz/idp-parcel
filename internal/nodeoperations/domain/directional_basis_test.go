package domain_test

import (
	"errors"
	"slices"
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

// succeeding 让 observed 接续 predecessor 那一行（ADR-0199 决定二的前版）。
func succeeding(t *testing.T, observed, predecessor domain.IntakeIdentityObservation) domain.IntakeIdentityObservation {
	t.Helper()
	observed.Predecessor = predecessor.Version
	return observed
}

func conflicted(observed domain.IntakeIdentityObservation) domain.IntakeIdentityObservation {
	observed.IdentityConflict = true
	return observed
}

// Covers: `AT-NO-061` 的领域面与 ADR-0210 决定五——身份一格只读每条判断链的当前有效版本，被组里另一行指名为前版的行
// 不再参与。一码两物先到那件的冲突版指名了原判断时，链头只剩冲突版，落成缺少·{待识别, 身份冲突}；第一期之前没有前版的
// 冲突版自成一条链，照旧判冲突、不回填；零候选的待识别被重核版接续即成立；冲突版之后接的处置版（这里用构造的观察，
// 不依处置编排）带关联即成立、不带即待识别，所依收寄是处置版而不是被接续的原判断；另一条链头记冲突照样挡住。前版指向
// 组外的行照常是链头。来源标识特意排成被接续的那一行在同刻定序里占先，挑错链头就会依错收寄。
func TestIdentityBasisReadsEachJudgmentChainHead(t *testing.T) {
	original := observation(t, "src-1-original", "DPCL-1", basisAt)
	conflict := succeeding(t, conflicted(observation(t, "src-2-conflict", "", basisAt)), original)
	pending := observation(t, "src-1-pending", "", basisAt)
	cases := []struct {
		name         string
		observations []domain.IntakeIdentityObservation
		answer       domain.DirectionalBasisAnswer
		reasons      []domain.DirectionalBasisReason
		basedOn      string
	}{
		{
			name:         "一码两物先到那件：冲突版指名原判断为前版",
			observations: []domain.IntakeIdentityObservation{original, conflict},
			answer:       domain.BasisMissing,
			reasons:      []domain.DirectionalBasisReason{domain.ReasonPendingIdentification, domain.ReasonIdentityConflict},
		},
		{
			name: "第一期之前的形状：冲突版没有前版",
			observations: []domain.IntakeIdentityObservation{
				original, conflicted(observation(t, "src-2-legacy-conflict", "", basisAt)),
			},
			answer:  domain.BasisMissing,
			reasons: []domain.DirectionalBasisReason{domain.ReasonIdentityConflict},
		},
		{
			name: "零候选的待识别被带关联的重核版接续",
			observations: []domain.IntakeIdentityObservation{
				pending, succeeding(t, observation(t, "src-2-recheck", "DPCL-1", basisAt), pending),
			},
			answer:  domain.BasisEstablished,
			basedOn: "src-2-recheck",
		},
		{
			name: "两条链：一条链头成立、一条链头记冲突",
			observations: []domain.IntakeIdentityObservation{
				observation(t, "src-0-other-chain", "DPCL-1", basisAt.Add(time.Hour)), original, conflict,
			},
			answer:  domain.BasisMissing,
			reasons: []domain.DirectionalBasisReason{domain.ReasonIdentityConflict},
		},
		{
			name: "冲突版之后接一版带关联、不记冲突的处置版",
			observations: []domain.IntakeIdentityObservation{
				original, conflict, succeeding(t, observation(t, "src-3-disposed", "DPCL-1", basisAt), conflict),
			},
			answer:  domain.BasisEstablished,
			basedOn: "src-3-disposed",
		},
		{
			name: "冲突版之后接一版不带关联、不记冲突的处置版",
			observations: []domain.IntakeIdentityObservation{
				original, conflict, succeeding(t, observation(t, "src-3-disposed", "", basisAt), conflict),
			},
			answer:  domain.BasisMissing,
			reasons: []domain.DirectionalBasisReason{domain.ReasonPendingIdentification},
		},
		{
			name: "前版指向组外的行",
			observations: []domain.IntakeIdentityObservation{
				succeeding(t, observation(t, "src-continued", "DPCL-1", basisAt), observation(t, "src-awaiting", "", basisAt)),
			},
			answer:  domain.BasisEstablished,
			basedOn: "src-continued",
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			identity, err := domain.JudgeIdentityBasis(testCase.observations)
			if err != nil {
				t.Fatalf("judge: %v", err)
			}
			cell := identity.Cell()
			if cell.Answer() != testCase.answer || !slices.Equal(cell.Reasons(), testCase.reasons) ||
				identity.IntakeSourceID() != testCase.basedOn {
				t.Fatalf("identity = %s %v based on %q, want %s %v based on %q",
					cell.Answer(), cell.Reasons(), identity.IntakeSourceID(), testCase.answer, testCase.reasons, testCase.basedOn)
			}
		})
	}
}

// Covers: ADR-0210 决定五的坏形状——同一前版被组里两行指名、一行指名自己、几行互相接续成环，哪一版是链头都说不准，
// 交回 ErrInvalidDirectionalBasis（编排把它记成`读不动`），不替它挑一个链头。前两种库面已拦，读回见到即说明有绕过
// 编排的写入；环库面拦不住。
func TestIdentityBasisRefusesMalformedJudgmentChains(t *testing.T) {
	original := observation(t, "src-original", "DPCL-1", basisAt)
	looping := observation(t, "src-loop-a", "", basisAt)
	closing := succeeding(t, observation(t, "src-loop-b", "", basisAt), looping)
	cases := map[string][]domain.IntakeIdentityObservation{
		"同一前版被两行指名": {
			original,
			succeeding(t, conflicted(observation(t, "src-conflict-1", "", basisAt)), original),
			succeeding(t, conflicted(observation(t, "src-conflict-2", "", basisAt)), original),
		},
		"一行指名自己": {original, succeeding(t, observation(t, "src-self", "DPCL-1", basisAt), observation(t, "src-self", "", basisAt))},
		"成环":     {original, succeeding(t, looping, closing), closing},
	}
	for name, observations := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := domain.JudgeIdentityBasis(observations); !errors.Is(err, domain.ErrInvalidDirectionalBasis) {
				t.Fatalf("err = %v, want ErrInvalidDirectionalBasis", err)
			}
		})
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
