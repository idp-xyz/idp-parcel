package domain_test

import (
	"encoding/json"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

func tfcInstant() time.Time {
	return time.Date(2026, 3, 5, 10, 11, 12, 345678901, time.FixedZone("CST", 8*3600))
}

func TestCommandPayloadsRoundTripAsTFC1(t *testing.T) {
	at := tfcInstant()
	cases := []struct {
		name     string
		document string
		digest   string
		call     func() ([]byte, string, error)
	}{
		{
			name:     "handover",
			document: `{"canonicalization":"TFC-1","face":"REGISTER_TRANSPORT_HANDOVER","verdict":"HANDED_OVER","released_by":"REL","received_by":"RCV","releasing_evidence":"RE","receiving_evidence":"VE","rule":"RULE","basis":"BASIS","judged_at":"2026-03-05T02:11:12.345678901Z"}`,
			digest:   "TFC-1:133a3202db0aa795dfe9d4ee4d1a46e1c68b4e34230ea4c05daa3f9681e24990",
			call: func() ([]byte, string, error) {
				return domain.CanonicalizeHandoverPayload(domain.ObjectHandedOver, "REL", "RCV", "RE", "VE", "RULE", "BASIS", at)
			},
		},
		{
			name:     "correction",
			document: `{"canonicalization":"TFC-1","face":"CORRECT_TRANSPORT_HANDOVER","verdict":"REFUSED","predecessor_version":"v1","releasing_evidence":"RE","receiving_evidence":"VE","rule":"RULE","basis":"BASIS","corrected_at":"2026-03-05T02:11:12.345678901Z"}`,
			digest:   "TFC-1:e81e1912dedcdeee8265d5b0c310b70f2b3d29d3aa0b06a6e42aa167bf66035f",
			call: func() ([]byte, string, error) {
				return domain.CanonicalizeHandoverCorrectionPayload(domain.HandoverRefused, "v1", "RE", "VE", "RULE", "BASIS", at)
			},
		},
		{
			name:     "delivery",
			document: `{"canonicalization":"TFC-1","face":"REGISTER_EFFECTIVE_DELIVERY","method":"SIGNED","recipient":"R","proof":"POD"}`,
			digest:   "TFC-1:936622139fadc18c7db7b9e6f940c4750f36e5298ba3ef058b1f0e955d6b715e",
			call: func() ([]byte, string, error) {
				return domain.CanonicalizeEffectiveDeliveryPayload("SIGNED", "R", "POD")
			},
		},
		{
			name:     "pickup registration",
			document: `{"canonicalization":"TFC-1","face":"REGISTER_OFFSITE_PICKUP","task":"TASK","place":"PLACE","control":"CTRL","executed_by":"WHO","occurred_at":"2026-03-05T02:11:12.345678901Z"}`,
			digest:   "TFC-1:8f02990c25632d5c58613bb8f7753b687414ba590b3fc668a4914e52d2ca7664",
			call: func() ([]byte, string, error) {
				return domain.CanonicalizeOffsitePickupRegistrationPayload("TASK", "PLACE", "CTRL", "WHO", at)
			},
		},
		{
			name:     "pickup",
			document: `{"canonicalization":"TFC-1","face":"PERFORM_OFFSITE_PICKUP","attempt":"ATT","task":"TASK","arrived_at":"2026-03-05T02:11:12.345678901Z","objects":[{"object":"OBJ-A","outcome":"CUSTOMER_ABSENT","basis":"B1","control":"C1","occurred_at":"2026-03-05T02:11:12.345678901Z"},{"object":"OBJ-B","outcome":"PICKED_UP","basis":"B2","control":"C2","occurred_at":"2026-03-05T02:11:12.345678901Z"}]}`,
			digest:   "TFC-1:bb1109a7d9850a2c7febaac04ea0b16e82646cd8621155c572b50f63ed60a3df",
			call: func() ([]byte, string, error) {
				return domain.CanonicalizeOffsitePickupPayload("ATT", "TASK", at, []domain.OffsitePickupObjectContent{
					{Object: "OBJ-B", Outcome: domain.ObjectPickedUp, Basis: "B2", Control: "C2", OccurredAt: at},
					{Object: "OBJ-A", Outcome: domain.CustomerAbsent, Basis: "B1", Control: "C1", OccurredAt: at},
				})
			},
		},
		{
			name:     "commission",
			document: `{"canonicalization":"TFC-1","face":"SUBMIT_COMMISSION","provider":"PROV","agreement":"AGR","conditions":"COND","role":"ROLE","responsibility":"RESP","submitted_at":"2026-03-05T02:11:12.345678901Z","members":["M-A","M-B"]}`,
			digest:   "TFC-1:cdf31298450119c31d7bda2e2b68fab01bb60ec6330dc76f31a8105d0c03b2f4",
			call: func() ([]byte, string, error) {
				return domain.CanonicalizeCommissionPayload("PROV", "AGR", "COND", "ROLE", "RESP", at, []string{"M-B", "M-A"})
			},
		},
		{
			name:     "booking",
			document: `{"canonicalization":"TFC-1","face":"SUBMIT_BOOKING","commission":"COM-1","quantity":2,"unit":"PIECE","requested_at":"2026-03-05T02:11:12.345678901Z"}`,
			digest:   "TFC-1:7b51aa6dd349ee0d6487d1ea591d69ee92c89124465f5c5727caee55ead1ea82",
			call: func() ([]byte, string, error) {
				return domain.CanonicalizeBookingPayload("COM-1", 2, "PIECE", at)
			},
		},
		{
			name:     "answer",
			document: `{"canonicalization":"TFC-1","face":"ANSWER_BOOKING","outcome":"ACCEPTED","acceptance":"ACC","quantity":2,"basis":"BASIS","decided_at":"2026-03-05T02:11:12.345678901Z"}`,
			digest:   "TFC-1:91eab90d09ee1c0a509bf10373dcb8572e3c6b0deccce745aa99e02bedec54dd",
			call: func() ([]byte, string, error) {
				return domain.CanonicalizeBookingAnswerPayload(domain.BookingAccepted, "ACC", 2, "BASIS", at)
			},
		},
		{
			name:     "journey",
			document: `{"canonicalization":"TFC-1","face":"START_ALTERNATE_JOURNEY","journey":"J-1","basis_kind":"REGULATORY_DISPOSITION","started_at":"2026-03-05T02:11:12.345678901Z","members":["M-A","M-B"]}`,
			digest:   "TFC-1:7b31d6e5af78fac1d19fd96aa2bc1ccd8c224d57a5086e24091b14c75bea95ef",
			call: func() ([]byte, string, error) {
				return domain.CanonicalizeAlternateJourneyPayload("J-1", domain.RegulatoryDispositionDecision, at, []string{"M-B", "M-A"})
			},
		},
		{
			name:     "disposition",
			document: `{"canonicalization":"TFC-1","face":"ACCEPT_REGULATORY_DISPOSITION","basis":"BASIS","movement_action":"MOVE","decision":"ACCEPTED","accepted_objects":["OBJ-A","OBJ-B"],"decline_basis":"","movement_authority":"AUTH","decided_at":"2026-03-05T02:11:12.345678901Z"}`,
			digest:   "TFC-1:e851765f73be1f2183afc85bd346bd3f85d4bdea0a421142c231a9a9196b94f1",
			call: func() ([]byte, string, error) {
				return domain.CanonicalizeRegulatoryDispositionPayload("BASIS", " MOVE ", domain.DispositionAccepted, []string{"OBJ-B", "OBJ-A"}, "", " AUTH ", at)
			},
		},
		{
			name:     "schedule",
			document: `{"canonicalization":"TFC-1","face":"ESTABLISH_SCHEDULE","direction":"EAST","departs_at":"2026-03-05T02:11:12.345678901Z"}`,
			digest:   "TFC-1:7cc90e5b932e8669012567d2d7300b6d2b7da486bcbec8b01dc6824c66d03512",
			call: func() ([]byte, string, error) {
				return domain.CanonicalizeEstablishSchedulePayload("EAST", at)
			},
		},
		{
			name:     "pool",
			document: `{"canonicalization":"TFC-1","face":"ESTABLISH_POOL","schedule":"SCH","unit":"KG","capacity":3}`,
			digest:   "TFC-1:8700eba6390c7e3ffd97681ec03d2c1d71d57cc3c73666d09904d184a7c68798",
			call: func() ([]byte, string, error) {
				return domain.CanonicalizeEstablishPoolPayload("SCH", "KG", 3)
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			document, digest, err := tc.call()
			if err != nil {
				t.Fatalf("canonicalize: %v", err)
			}
			if string(document) != tc.document {
				t.Fatalf("document\n got %s\nwant %s", document, tc.document)
			}
			if digest != tc.digest {
				t.Fatalf("digest %s, want %s", digest, tc.digest)
			}
			var roundTrip map[string]any
			if err := json.Unmarshal(document, &roundTrip); err != nil {
				t.Fatalf("round trip: %v", err)
			}
			if roundTrip["canonicalization"] != "TFC-1" {
				t.Fatalf("canonicalization %v", roundTrip["canonicalization"])
			}
		})
	}
}
