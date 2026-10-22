package domain_test

import (
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
)

// supersededAt 把最后加进来的那份放行读数标成已被生效更正取代，取代时点是那份更正的业务发生时间。
func (c *admissionCase) supersededAt(at time.Time) *admissionCase {
	c.input.Releases[len(c.input.Releases)-1].SupersededAt = at
	return c
}

// Covers: AT-CC-156 的准入半边，按时点读（ADR-0226 决定三「取业务发生时间不晚于所问时点的最后一份」）——
// 被生效更正取代的放行只在更正发生之前算数，更正也只从它自己的业务发生时间起算数，不继承原结果的时间：
// 收窄更正之前的动作时点由原全部放行答放行，更正之后只剩部分放行答未决；放宽更正之前只有原部分放行，答未决。
func TestASupersededReleaseCountsOnlyBeforeItsCorrectionOccurred(t *testing.T) {
	before, after := admissionAt.Add(-2*time.Hour), admissionAt.Add(time.Hour)
	gated := func() *admissionCase {
		return newAdmissionCase(t).inUnit().gate(domain.LoadingDeparture, domain.GateReadingFormed, domain.GateMet)
	}

	narrowedLater := gated().
		release("SYN-SRC-FULL", domain.FullRelease, before).supersededAt(after).
		release("SYN-SRC-PART", domain.PartialRelease, after).judge()
	assertFinding(t, findingOf(t, narrowedLater, domain.LoadingDeparture), domain.AdmissionAdmitted, domain.CauseReleasedInFull)

	narrowedEarlier := gated().
		release("SYN-SRC-FULL", domain.FullRelease, before).supersededAt(admissionAt.Add(-time.Hour)).
		release("SYN-SRC-PART", domain.PartialRelease, admissionAt.Add(-time.Hour)).judge()
	assertFinding(t, findingOf(t, narrowedEarlier, domain.LoadingDeparture), domain.AdmissionUndecided, domain.CauseReleasePartialOnly)

	supersededAtTheInstant := gated().
		release("SYN-SRC-FULL", domain.FullRelease, before).supersededAt(admissionAt).
		release("SYN-SRC-PART", domain.PartialRelease, admissionAt).judge()
	assertFinding(t, findingOf(t, supersededAtTheInstant, domain.LoadingDeparture), domain.AdmissionUndecided, domain.CauseReleasePartialOnly)

	widenedLater := gated().
		release("SYN-SRC-PART", domain.PartialRelease, before).supersededAt(after).
		release("SYN-SRC-FULL", domain.FullRelease, after).judge()
	assertFinding(t, findingOf(t, widenedLater, domain.LoadingDeparture), domain.AdmissionUndecided, domain.CauseReleasePartialOnly)
}
