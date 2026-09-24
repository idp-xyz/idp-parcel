package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/pilotgovernance/application"
	"go.idp.xyz/idp-parcel/internal/pilotgovernance/domain"
)

const selfAuthority = "IDP-PARCEL"

var coverageAt = time.Date(2026, 9, 25, 3, 0, 0, 0, time.UTC)

type intervalStoreFake struct {
	intervals []domain.AuthorityInterval
	err       error
}

func (fake intervalStoreFake) ListCurrent(context.Context) ([]domain.AuthorityInterval, error) {
	return fake.intervals, fake.err
}

func (intervalStoreFake) Append(context.Context, domain.AuthorityInterval) error { return nil }

var coordinates = application.AuthorityCoordinates{ObjectScope: "SYN-SCOPE-LANE-A", Capability: "OPERATION_DECISION", FactKind: "MANUAL_REVIEW_COMPLETION"}

func interval(authority string, from, to time.Time) domain.AuthorityInterval {
	return domain.AuthorityInterval{
		ObjectScope: coordinates.ObjectScope, Capability: coordinates.Capability, FactKind: coordinates.FactKind,
		Authority: authority, From: from, To: to,
	}
}

func TestAuthorityCoverageAnswersWhetherThisProductHoldsTheInterval(t *testing.T) {
	open := interval(selfAuthority, coverageAt.Add(-time.Hour), time.Time{})
	otherScope := open
	otherScope.ObjectScope = "SYN-SCOPE-LANE-B"
	cases := map[string]struct {
		intervals []domain.AuthorityInterval
		self      string
		want      bool
	}{
		"open interval held by this product":             {intervals: []domain.AuthorityInterval{open}, self: selfAuthority, want: true},
		"closed interval ending after the moment":        {intervals: []domain.AuthorityInterval{interval(selfAuthority, coverageAt.Add(-time.Hour), coverageAt.Add(time.Minute))}, self: selfAuthority, want: true},
		"register holds no interval at all":              {self: selfAuthority, want: false},
		"interval held by another authority":             {intervals: []domain.AuthorityInterval{interval("LEGACY-TMS", coverageAt.Add(-time.Hour), time.Time{})}, self: selfAuthority, want: false},
		"interval already closed at the moment":          {intervals: []domain.AuthorityInterval{interval(selfAuthority, coverageAt.Add(-time.Hour), coverageAt)}, self: selfAuthority, want: false},
		"interval not yet begun":                         {intervals: []domain.AuthorityInterval{interval(selfAuthority, coverageAt.Add(time.Minute), time.Time{})}, self: selfAuthority, want: false},
		"interval on another object scope":               {intervals: []domain.AuthorityInterval{otherScope}, self: selfAuthority, want: false},
		"this product's authority string not configured": {intervals: []domain.AuthorityInterval{open}, self: "", want: false},
	}
	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			coverage, err := application.NewAuthorityCoverage(intervalStoreFake{intervals: testCase.intervals}, testCase.self)
			if err != nil {
				t.Fatal(err)
			}
			covered, err := coverage.Covers(context.Background(), coordinates, coverageAt)
			if err != nil || covered != testCase.want {
				t.Fatalf("Covers = (%v, %v), want (%v, nil)", covered, err, testCase.want)
			}
		})
	}
}

func TestAuthorityCoverageReadFailureIsADependencyFailureNotNotCovered(t *testing.T) {
	coverage, err := application.NewAuthorityCoverage(intervalStoreFake{err: errors.New("connection refused")}, selfAuthority)
	if err != nil {
		t.Fatal(err)
	}
	covered, err := coverage.Covers(context.Background(), coordinates, coverageAt)
	if !errors.Is(err, application.ErrAuthorityCoverageUnavailable) || covered {
		t.Fatalf("Covers = (%v, %v), want (false, ErrAuthorityCoverageUnavailable)", covered, err)
	}
}

func TestAuthorityCoverageNeedsAStore(t *testing.T) {
	if _, err := application.NewAuthorityCoverage(nil, selfAuthority); err == nil {
		t.Fatal("coverage built without an interval store")
	}
}
