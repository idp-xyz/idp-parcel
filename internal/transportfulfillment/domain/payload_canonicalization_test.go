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
			// 票 operator-channel/41：两侧都从本体现算，更正时刻先折到登记册精度。
			name:     "delivery proof correction",
			document: `{"canonicalization":"TFC-1","face":"CORRECT_DELIVERY_PROOF","predecessor_version":"v1","new_proof":"POD2","corrected_at":"2026-03-05T02:11:12.345678Z"}`,
			digest:   "TFC-1:352a6c1d6f2c2e8fc63050f5106e47fba91c68b2bed1facd40893482c4807b2d",
			call: func() ([]byte, string, error) {
				return domain.CanonicalizeDeliveryProofCorrectionPayload("v1", "POD2", at)
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
			// 星期按周一到周日写词（登记时给的是乱序带重复）；生效区间不设终点写 null。
			name:     "schedule template",
			document: `{"canonicalization":"TFC-1","face":"REGISTER_SCHEDULE_TEMPLATE","connection":"CONN","from_node":"A","to_node":"B","weekdays":["MON","WED","SUN"],"departure_local_minute":1320,"duration_minutes":600,"time_zone":"Asia/Shanghai","service_scope":"SCOPE","effective_from":"2026-01-01","effective_until":null}`,
			digest:   "TFC-1:0e3cf3ae653725cb0fe21e2970927fa89f6e5bcc987ca37ac931909dd7735d7e",
			call: func() ([]byte, string, error) {
				return domain.CanonicalizeScheduleTemplatePayload(templateContentFor(t, []time.Weekday{time.Sunday, time.Wednesday, time.Monday, time.Sunday}, ""))
			},
		},
		{
			// 物化来的班次：出处的周期安排版本不进摘要，代码与运行日期进。
			name:     "materialized schedule",
			document: `{"canonicalization":"TFC-1","face":"REGISTER_TRANSPORT_SCHEDULE","connection":"CONN","from_node":"A","to_node":"B","departs_at":"2026-03-04T14:00:00Z","arrives_at":"2026-03-05T00:00:00Z","service_scope":"SCOPE","origin":"TEMPLATE","template":"TPL","running_date":"2026-03-04"}`,
			digest:   "TFC-1:8335f29706cd108d1af36ce2185464c3f641b9cf7879b864b9138805f77c97ea",
			call: func() ([]byte, string, error) {
				return domain.CanonicalizeTransportSchedulePayload(materializedFor(t, "2026-03-04"))
			},
		},
		{
			// 临时班次：出处两格写 null；出发与到达按微秒取，纳秒尾巴在册上存不下，不构成下一版。
			name:     "ad hoc schedule",
			document: `{"canonicalization":"TFC-1","face":"REGISTER_TRANSPORT_SCHEDULE","connection":"CONN","from_node":"A","to_node":"B","departs_at":"2026-03-05T02:11:12.345678Z","arrives_at":"2026-03-05T04:11:12.345678Z","service_scope":"SCOPE","origin":"AD_HOC","template":null,"running_date":null}`,
			digest:   "TFC-1:6c1a7a71da1ea34ddcf37fb1be8d1b72c92129c1d4ffad62c672e5af3acf20e3",
			call: func() ([]byte, string, error) {
				return domain.CanonicalizeTransportSchedulePayload(adHocScheduleFor(t, "SCH-ADHOC", at, at.Add(2*time.Hour)))
			},
		},
		{
			// 逐维容量声明按维度次序成组（登记时给的是件数在前）。
			name:     "schedule template with capacities",
			document: `{"canonicalization":"TFC-1","face":"REGISTER_SCHEDULE_TEMPLATE","connection":"CONN","from_node":"A","to_node":"B","weekdays":["MON","TUE","WED","THU","FRI","SAT","SUN"],"departure_local_minute":1320,"duration_minutes":600,"time_zone":"Asia/Shanghai","service_scope":"SCOPE","effective_from":"2026-01-01","effective_until":null,"capacities":[{"dimension":"WEIGHT","unit":"KG","capacity":5000},{"dimension":"PIECES","unit":"PCS","capacity":800}]}`,
			digest:   "TFC-1:1e9018fa08e972343e3b9f408bf8aefb58af4f054987ce189257efcb4851222c",
			call: func() ([]byte, string, error) {
				content := templateContentFor(t, everyDay(), "")
				content.Capacities = []domain.CapacityDeclaration{
					capacityDeclaration(t, domain.CapacityPieces, "PCS", 800),
					capacityDeclaration(t, domain.CapacityWeight, "KG", 5000),
				}
				return domain.CanonicalizeScheduleTemplatePayload(content)
			},
		},
		{
			// 池身份由班次与维度派生，不另进；预占不进。
			name:     "pool",
			document: `{"canonicalization":"TFC-1","face":"ESTABLISH_POOL","schedule":"SCH","dimension":"WEIGHT","unit":"KG","capacity":3}`,
			digest:   "TFC-1:c8b9e5b3b737fc80059113a98ce678ff1ec1628b12cceca08a6fed9dbecd2348",
			call: func() ([]byte, string, error) {
				pool, err := domain.EstablishCapacityPool(domain.CapacityPoolSpec{
					TenantID:  mustValue(t, domain.NewTenantID, "SYN-TENANT-01"),
					Schedule:  mustValue(t, domain.NewScheduleReference, "SCH"),
					Dimension: domain.CapacityWeight,
					Unit:      mustValue(t, domain.NewQuantityUnitReference, "KG"),
					Capacity:  3,
				})
				if err != nil {
					return nil, "", err
				}
				return domain.CanonicalizeEstablishPoolPayload(pool)
			},
		},
		{
			// 业务时间按微秒取：在册那一份只到微秒，纳秒尾巴不构成内容。
			name:     "carrier pickup judgment expressing control",
			document: `{"canonicalization":"TFC-1","face":"JUDGE_CARRIER_FIRST_EFFECTIVE_PICKUP","expresses_control":true,"subject_kind":"EXTERNAL_PARTY","subject_reference":"party/carrier-x","occurred_at":"2026-03-05T02:11:12.345678Z"}`,
			digest:   "TFC-1:f64576e6564cc143749941be44df070728636a5cd9ade39a40dbde90106fb6c5",
			call: func() ([]byte, string, error) {
				subject, err := domain.NewCarrierSubject(domain.ExternalCarrierParty, "party/carrier-x")
				if err != nil {
					return nil, "", err
				}
				return domain.CanonicalizeCarrierFirstEffectivePickupPayload(true, subject, at)
			},
		},
		{
			// 不表达时主体与时间不构成内容：给了也不进文档（失效版本上没有这两格可比）。
			name:     "carrier pickup judgment not expressing control",
			document: `{"canonicalization":"TFC-1","face":"JUDGE_CARRIER_FIRST_EFFECTIVE_PICKUP","expresses_control":false}`,
			digest:   "TFC-1:81475c1acecee9af095e266aee2bd6156b8efe61979354f5857e389823186131",
			call: func() ([]byte, string, error) {
				subject, err := domain.NewCarrierSubject(domain.ExternalCarrierParty, "party/carrier-x")
				if err != nil {
					return nil, "", err
				}
				return domain.CanonicalizeCarrierFirstEffectivePickupPayload(false, subject, at)
			},
		},
		{
			// 票 tf-face-algebra/08：业务时间照 ADR-0023 不进摘要；来源与 GateRequired 不是参数，文档上也就没有这几格。
			name:     "movement fact with gate clearance",
			document: `{"canonicalization":"TFC-1","face":"RECORD_MOVEMENT_FACT","schedule":"SCH","kind":"DEPARTURE","location":"LOC","gate_clearance":"GATE"}`,
			digest:   "TFC-1:ebe62f9cff78fc476f8f342b665cf3c5e4cb8bc57599a6901bf02a06c4831e60",
			call: func() ([]byte, string, error) {
				schedule, location, clearance, err := movementFactContent("SCH", "LOC", "GATE")
				if err != nil {
					return nil, "", err
				}
				return domain.CanonicalizeMovementFactPayload(schedule, domain.DepartureFact, location, clearance)
			},
		},
		{
			// 放行缺席写 null 而不写空串：「没有门禁依据」是一格内容，不是一个空的引用。
			name:     "movement fact without gate clearance",
			document: `{"canonicalization":"TFC-1","face":"RECORD_MOVEMENT_FACT","schedule":"SCH","kind":"ARRIVAL","location":"LOC","gate_clearance":null}`,
			digest:   "TFC-1:d8a427cf3f95a44bbefcdfaff188584b6e06d44adec6e9572b3d81489a7ce390",
			call: func() ([]byte, string, error) {
				schedule, location, _, err := movementFactContent("SCH", "LOC", "GATE")
				if err != nil {
					return nil, "", err
				}
				return domain.CanonicalizeMovementFactPayload(schedule, domain.ArrivalFact, location, domain.GateClearanceReference{})
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

// Covers: 票 operator-channel/20——承运商首次有效收寄判断两侧都从本体现算，而在册的业务时间只到微秒（对真库实测：
// 存 …00.123456789Z 读回 …00.123456Z）。按纳秒比，同一条带纳秒时刻的命令原样重放会被判成内容冲突；按微秒比则同，
// 跨过一微秒才异。
func TestCarrierPickupJudgmentBusinessTimeIsComparedToTheMicrosecond(t *testing.T) {
	subject, err := domain.NewCarrierSubject(domain.ExternalCarrierParty, "party/carrier-x")
	if err != nil {
		t.Fatalf("承运主体：%v", err)
	}
	given := time.Date(2026, 9, 10, 9, 0, 0, 123456789, time.UTC)
	digest := func(at time.Time) string {
		t.Helper()
		_, digest, err := domain.CanonicalizeCarrierFirstEffectivePickupPayload(true, subject, at)
		if err != nil {
			t.Fatalf("canonicalize: %v", err)
		}
		return digest
	}
	if digest(given) != digest(given.Truncate(time.Microsecond)) {
		t.Fatal("纳秒尾巴不应构成另一份内容")
	}
	if digest(given) == digest(given.Add(time.Microsecond)) {
		t.Fatal("相差一微秒应是另一份内容")
	}
}

// Covers: 票 operator-channel/41——POD 更正两侧都从本体现算，在册更正时刻只到微秒；纳秒尾巴不构成另一份更正，
// 跨过一微秒才异。
func TestDeliveryProofCorrectionTimeIsComparedToTheMicrosecond(t *testing.T) {
	given := time.Date(2026, 9, 10, 9, 0, 0, 123456789, time.UTC)
	digest := func(at time.Time) string {
		t.Helper()
		_, digest, err := domain.CanonicalizeDeliveryProofCorrectionPayload("v1", "POD2", at)
		if err != nil {
			t.Fatalf("canonicalize: %v", err)
		}
		return digest
	}
	if digest(given) != digest(given.Truncate(time.Microsecond)) {
		t.Fatal("纳秒尾巴不应构成另一份更正")
	}
	if digest(given) == digest(given.Add(time.Microsecond)) {
		t.Fatal("相差一微秒应是另一份更正")
	}
}

// Covers: ADR-0201 决定四——这本册子上同身份异内容是下一版，不是冲突；临时班次带纳秒的出发与到达，与照读回值
// （微秒）原样重登算出同一摘要，不多出一版库面相同的班次；跨过一微秒才异。
func TestAdHocScheduleTimesAreComparedToTheMicrosecond(t *testing.T) {
	given := time.Date(2026, 9, 10, 9, 0, 0, 123456789, time.UTC)
	digest := func(departsAt time.Time) string {
		t.Helper()
		_, digest, err := domain.CanonicalizeTransportSchedulePayload(adHocScheduleFor(t, "SCH-ADHOC", departsAt, departsAt.Add(2*time.Hour)))
		if err != nil {
			t.Fatalf("canonicalize: %v", err)
		}
		return digest
	}
	if digest(given) != digest(given.Truncate(time.Microsecond)) {
		t.Fatal("纳秒尾巴不应构成下一版")
	}
	if digest(given) == digest(given.Add(time.Microsecond)) {
		t.Fatal("相差一微秒应是另一份内容")
	}
}

// Covers: ADR-0014「摘要只在同一规范化版本内可比」——带 TFC-1 前缀的与本次 TFC-1 摘要比，不带前缀的
// 与无版本那一版比，认不出的前缀既不是重放也不是冲突。
func TestStoredDigestIsComparedWithinItsOwnShape(t *testing.T) {
	const current, unversioned = "TFC-1:aa", "bb"
	cases := []struct {
		name   string
		stored string
		want   domain.StoredDigestComparison
	}{
		{"TFC-1 same content", "TFC-1:aa", domain.SamePayload},
		{"TFC-1 different content", "TFC-1:cc", domain.DifferentPayload},
		{"unversioned same content", "bb", domain.SamePayload},
		{"unversioned different content", "cc", domain.DifferentPayload},
		{"an unversioned digest is not read as TFC-1", "aa", domain.DifferentPayload},
		{"a later shape", "TFC-2:aa", domain.UnknownPayloadShape},
		{"another context's shape", "NOC-1:bb", domain.UnknownPayloadShape},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := domain.CompareStoredDigest(testCase.stored, current, unversioned); got != testCase.want {
				t.Fatalf("CompareStoredDigest(%q) = %d, want %d", testCase.stored, got, testCase.want)
			}
		})
	}
}

func movementFactContent(schedule, location, clearance string) (domain.ScheduleReference, domain.MovementLocationReference, domain.GateClearanceReference, error) {
	scheduleRef, err := domain.NewScheduleReference(schedule)
	if err != nil {
		return domain.ScheduleReference{}, domain.MovementLocationReference{}, domain.GateClearanceReference{}, err
	}
	locationRef, err := domain.NewMovementLocationReference(location)
	if err != nil {
		return domain.ScheduleReference{}, domain.MovementLocationReference{}, domain.GateClearanceReference{}, err
	}
	clearanceRef, err := domain.NewGateClearanceReference(clearance)
	return scheduleRef, locationRef, clearanceRef, err
}
