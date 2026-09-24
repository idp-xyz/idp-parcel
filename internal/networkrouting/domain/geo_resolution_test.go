package domain_test

import (
	"errors"
	"testing"

	"go.idp.xyz/idp-parcel/internal/networkrouting/domain"
)

func side(country, postal string) domain.GeoResolutionSide {
	return domain.NewGeoResolutionSide(country, country != "", postal, postal != "")
}

func coverage(t *testing.T, country string, prefixes ...string) domain.ServiceAreaCoverage {
	t.Helper()
	value, err := domain.NewServiceAreaCoverage(country, prefixes)
	if err != nil {
		t.Fatalf("new service area coverage: %v", err)
	}
	return value
}

// Covers: ADR-0148 决定二——覆盖文法首版两种形态，国家码缺席或不成形答资料不足，不补默认国家、不改大小写。
func TestServiceAreaCoverageMatchesTheCarriedAddressVerbatim(t *testing.T) {
	wholeCountry := coverage(t, "XA")
	byPrefix := coverage(t, "XA", "10", "20")

	cases := []struct {
		name     string
		coverage domain.ServiceAreaCoverage
		side     domain.GeoResolutionSide
		want     domain.CoverageMatch
	}{
		{"整国家覆盖同国地址", wholeCountry, side("XA", ""), domain.CoverageCovered},
		{"整国家不覆盖他国地址", wholeCountry, side("XB", "100001"), domain.CoverageNotCovered},
		{"邮编前缀命中", byPrefix, side("XA", "200300"), domain.CoverageCovered},
		{"邮编前缀不命中", byPrefix, side("XA", "300300"), domain.CoverageNotCovered},
		{"前缀形态缺邮编即资料不足", byPrefix, side("XA", ""), domain.CoverageInsufficient},
		{"他国地址不看邮编", byPrefix, side("XB", ""), domain.CoverageNotCovered},
		{"缺国家码即资料不足", wholeCountry, side("", "100001"), domain.CoverageInsufficient},
		{"小写国家码不成形即资料不足", wholeCountry, side("xa", ""), domain.CoverageInsufficient},
		{"邮编逐字比、不去空白", byPrefix, side("XA", " 200300"), domain.CoverageNotCovered},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := testCase.coverage.Match(testCase.side); got != testCase.want {
				t.Fatalf("match = %s, want %s", got, testCase.want)
			}
		})
	}
}

// Covers: 覆盖在登记时就要成形——国家码两位大写字母，前缀不能是空串、不能重复。
func TestServiceAreaCoverageRefusesMalformedShapes(t *testing.T) {
	for name, build := range map[string]func() error{
		"缺国家码":  func() error { _, err := domain.NewServiceAreaCoverage("", nil); return err },
		"国家码小写": func() error { _, err := domain.NewServiceAreaCoverage("xa", nil); return err },
		"国家码三位": func() error { _, err := domain.NewServiceAreaCoverage("XAB", nil); return err },
		"空前缀":   func() error { _, err := domain.NewServiceAreaCoverage("XA", []string{""}); return err },
		"前缀重复":  func() error { _, err := domain.NewServiceAreaCoverage("XA", []string{"10", "10"}); return err },
	} {
		if err := build(); !errors.Is(err, domain.ErrInvalidServiceAreaCoverage) {
			t.Fatalf("%s: error = %v, want ErrInvalidServiceAreaCoverage", name, err)
		}
	}
	if got := coverage(t, "XA", "20", "10").PostalPrefixes(); len(got) != 2 || got[0] != "10" || got[1] != "20" {
		t.Fatalf("prefixes = %v, want sorted [10 20]", got)
	}
}

// Covers: UC-NR-002 层次 1 的起止服务区域——起点侧不在覆盖内同样淘汰候选，理由点名起点侧。
func TestServiceAreaResolutionExcludesOnTheOriginSide(t *testing.T) {
	resolution, err := domain.NewServiceAreaResolution(domain.ServiceAreaResolutionSpec{
		Candidate:   mustValue(t, domain.NewCandidateID, "CAND-ORIGIN"),
		Outcome:     domain.AreaExcludesOrigin,
		AreaVersion: mustValue(t, domain.NewServiceAreaVersionReference, "AREA-ORIGIN@1"),
	})
	if err != nil {
		t.Fatalf("new origin-excluding resolution: %v", err)
	}
	candidates, gaps, err := domain.EvaluateServiceAreas([]domain.ServiceAreaResolution{resolution})
	if err != nil || len(gaps) != 0 || len(candidates) != 1 {
		t.Fatalf("evaluate = (%v, %v, %v)", candidates, gaps, err)
	}
	if candidates[0].Outcome() != domain.CandidateEliminated ||
		candidates[0].Reason().String() != "SERVICE_AREA_EXCLUDES_ORIGIN/AREA-ORIGIN@1" {
		t.Fatalf("candidate = %s / %s", candidates[0].Outcome(), candidates[0].Reason())
	}
	if _, err := domain.NewServiceAreaResolution(domain.ServiceAreaResolutionSpec{
		Candidate: mustValue(t, domain.NewCandidateID, "CAND-ORIGIN"),
		Outcome:   domain.AreaExcludesOrigin,
	}); !errors.Is(err, domain.ErrInvalidServiceAreaResolution) {
		t.Fatalf("origin exclusion without area version: error = %v", err)
	}
}
