package ports_test

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strconv"
	"strings"
	"testing"

	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
)

// 只挂税费付款时的指纹必须与加毛重读数之前逐字节相同，旧门禁行才撞得上。
func TestGateVersionDigestKeepsPriorBytesWhenWeightIsNotApplied(t *testing.T) {
	findings := []domain.PreconditionFinding{{
		Precondition: mustDigestRef(t, domain.NewPreconditionReference, "RESTRICTION/released"),
		State:        domain.PreconditionMet,
	}}
	if ports.GateVersionDigest(findings, domain.DutyPaymentGateReading{}, false, domain.WeightDiscrepancyGateReading{}, false) !=
		ports.FindingsDigest(findings) {
		t.Fatal("两道都没挂时指纹不再等于 FindingsDigest")
	}
	reading := domain.DutyPaymentGateReading{
		State:    domain.PreconditionMet,
		Coverage: domain.CoverageFull,
		Delta:    domain.DeltaNone,
		Validity: domain.FundsFactValid,
		Verification: domain.DutyVerificationReference{
			Duty:    mustDigestRef(t, domain.NewAssessedDutyReference, "SYN-DUTY-01/v1"),
			Funds:   mustDigestRef(t, domain.NewExternalFundsFactReference, "SYN-FUNDS-01"),
			Version: "SYN-DIGEST-01",
		},
	}
	lines := []string{findings[0].Precondition.String() + "=" + strconv.Itoa(int(findings[0].State))}
	sort.Strings(lines)
	lines = append(lines, "duty-payment="+reading.Coverage.String()+"/"+reading.Delta.String()+"/"+reading.Validity.String()+
		"@"+reading.Verification.Duty.String()+"/"+reading.Verification.Funds.String()+"/"+reading.Verification.Version)
	sum := sha256.Sum256([]byte(strings.Join(lines, "\x00")))
	want := hex.EncodeToString(sum[:])
	if got := ports.GateVersionDigest(findings, reading, true, domain.WeightDiscrepancyGateReading{}, false); got != want {
		t.Fatalf("只挂税费付款的指纹变了：%s", got)
	}
	withWeight := ports.GateVersionDigest(findings, reading, true, domain.WeightDiscrepancyGateReading{
		State:             domain.PreconditionUnmet,
		SubmissionVersion: mustDigestRef(t, domain.NewSubmissionVersionID, "submission-1"),
		MaxDelta:          "0.5",
		Unit:              "KG",
		Members: []domain.WeightDiscrepancyMemberReading{{
			Parcel:            mustDigestRef(t, domain.NewDeclaredParcelReference, "parcel-a"),
			MeasuredWeight:    "3.00",
			MeasuredUnit:      "KG",
			MeasurementSource: "SYN-WEIGH-OVER",
		}},
	}, true)
	if withWeight == want {
		t.Fatal("挂上毛重读数后指纹没变")
	}
}

// 指纹认的是这一次比较。毛重数字与单位不变，只换实测来源或成版来源，就是另一版门禁判断。
func TestGateVersionDigestChangesWhenOnlyTheWeighingSourceChanges(t *testing.T) {
	findings := []domain.PreconditionFinding{{
		Precondition: mustDigestRef(t, domain.NewPreconditionReference, "RESTRICTION/released"),
		State:        domain.PreconditionMet,
	}}
	digest := func(declaredSource, measuredSource string) string {
		t.Helper()
		return ports.GateVersionDigest(findings, domain.DutyPaymentGateReading{}, false, domain.WeightDiscrepancyGateReading{
			State:             domain.PreconditionMet,
			SubmissionVersion: mustDigestRef(t, domain.NewSubmissionVersionID, "submission-1"),
			MaxDelta:          "0.5",
			Unit:              "KG",
			Members: []domain.WeightDiscrepancyMemberReading{{
				Parcel:            mustDigestRef(t, domain.NewDeclaredParcelReference, "parcel-a"),
				DeclaredWeight:    "2.20",
				DeclaredUnit:      "KG",
				DeclaredSource:    declaredSource,
				MeasuredWeight:    "2.20",
				MeasuredUnit:      "KG",
				MeasurementSource: measuredSource,
				WithinTolerance:   true,
			}},
		}, true)
	}
	base := digest("WEIGH-0A", "WEIGH-1")
	if digest("WEIGH-0A", "WEIGH-2") == base {
		t.Fatal("同重量只换实测来源，指纹没变")
	}
	if digest("WEIGH-0B", "WEIGH-1") == base {
		t.Fatal("同重量只换成版来源，指纹没变")
	}
}

func mustDigestRef[T any](t *testing.T, construct func(string) (T, error), raw string) T {
	t.Helper()
	value, err := construct(raw)
	if err != nil {
		t.Fatal(err)
	}
	return value
}
