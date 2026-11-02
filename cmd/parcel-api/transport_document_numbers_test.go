package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/platform/docnumber"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件对真实 PostgreSQL 证运输委托与订舱取号的生产装配（ADR-0355 决定三、七、八、十一；票 document-numbering/09）：委托
// 提交口与订舱、承运答复两口经生产的事务壳与编号组件取号。编号组件取服务端时钟，所以这里的日期按此刻的上海日期算。

// registerShanghaiNumberingTimeZone 给演示租户登一版单据编号时区（演示取值，与种子 050 同一版）。
func registerShanghaiNumberingTimeZone(t *testing.T, db *bentopg.DB) {
	t.Helper()
	if _, err := documentNumberingComponents(t, db, docnumber.SecretTable{}).Registrations.RegisterTimeZone(t.Context(), docnumber.RegisterTimeZoneCommand{
		Tenant: "SYN-TENANT-01", TimeZone: "Asia/Shanghai", EffectiveFrom: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), RegisteredBy: "SYN-TEST",
	}); err != nil {
		t.Fatalf("登单据编号时区：%v", err)
	}
}

// shanghaiMonth 是此刻上海时间的年月，默认模板 `{yyyy}{mm}` 那一段。
func shanghaiMonth(t *testing.T) string {
	t.Helper()
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatal(err)
	}
	return time.Now().In(location).Format("200601")
}

// sequenceValue 是演示租户这一类单据在各周期序列上取过的值数之和：平台序列表的 next_val 存的是最后取出的那个值。
func sequenceValue(t *testing.T, pool *pgxpool.Pool, docType string) int64 {
	t.Helper()
	var total int64
	if err := pool.QueryRow(t.Context(),
		`SELECT COALESCE(sum(next_val), 0) FROM `+migrate.SchemaPlatform+`.numbering_sequence
		  WHERE scope = 'SYN-TENANT-01' AND doc_type = $1`, docType,
	).Scan(&total); err != nil {
		t.Fatalf("读序列：%v", err)
	}
	return total
}

func nestedString(answer map[string]any, object, key string) string {
	body, _ := answer[object].(map[string]any)
	value, _ := body[key].(string)
	return value
}

// Covers: AT-TF-371、AT-TF-372（生产装配真库面）——租户没登单据编号时区时委托照常提交、答复带无号原因、库里记原因；登了之后
// 委托与订舱各按默认模板取号，答复带号，重放答原号；承运答复照旧按订舱标识认单，拿单据号当订舱标识答订舱不在册。
func TestTheWiredCommissionAndBookingFacesIssueDocumentNumbers(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	seeded := publishSeededSupplierAgreements(t, db)
	trunk := seeded["SYN-SUPPLIER-TRUNK-CN-SG-01"]
	registerSeededTrunkSchedule(t, db)
	endpoints := bookingFacesForTest(t, db)
	const token = "presented.operator.token"
	supplier := trunk.Declarations.SupplierAgreementBody.Supplier

	status, answer := postWithToken(t, endpoints.commission, commissionSubmissionPath, token,
		commissionBody("SYN-COMMISSION-UNNUMBERED", supplier, trunk.ObjectID, trunk.Version, "2026-10-19T08:00:00Z"))
	if status != http.StatusCreated || nestedString(answer, "commission", "documentNumberAbsence") != "NUMBERING_TIME_ZONE_NOT_CONFIGURED" ||
		nestedString(answer, "commission", "documentNumber") != "" {
		t.Fatalf("没登时区的委托 = %d %v", status, answer)
	}
	var absence string
	if err := pool.QueryRow(t.Context(), `SELECT document_number_absence FROM transport_fulfillment.transport_commission
		WHERE tenant_id = 'SYN-TENANT-01' AND commission_id = 'SYN-COMMISSION-UNNUMBERED' AND document_number IS NULL`).Scan(&absence); err != nil ||
		absence != "NUMBERING_TIME_ZONE_NOT_CONFIGURED" {
		t.Fatalf("库里的无号原因 = %q %v", absence, err)
	}

	registerShanghaiNumberingTimeZone(t, db)
	month := shanghaiMonth(t)
	body := commissionBody("SYN-COMMISSION-TRUNK-DCG24", supplier, trunk.ObjectID, trunk.Version, "2026-10-19T08:00:00Z")
	status, answer = postWithToken(t, endpoints.commission, commissionSubmissionPath, token, body)
	if want := "TC-" + month + "-00001"; status != http.StatusCreated || nestedString(answer, "commission", "documentNumber") != want {
		t.Fatalf("委托 = %d %v，want %s", status, answer, want)
	}
	if _, replay := postWithToken(t, endpoints.commission, commissionSubmissionPath, token, body); replay["outcome"] != "EXISTING_COMMISSION" ||
		nestedString(replay, "commission", "documentNumber") != "TC-"+month+"-00001" {
		t.Fatalf("委托重放 = %v", replay)
	}

	bookingBody := trunkBookingBody("SYN-BOOKING-TRUNK-DCG24", seededTrunkSchedule)
	status, answer = postWithToken(t, endpoints.booking, bookingSubmissionPath, token, bookingBody)
	bookingNumber := nestedString(answer, "booking", "documentNumber")
	if status != http.StatusCreated || bookingNumber != "BK-"+month+"-00001" || nestedString(answer, "booking", "booking") != "SYN-BOOKING-TRUNK-DCG24" {
		t.Fatalf("订舱 = %d %v", status, answer)
	}
	if _, replay := postWithToken(t, endpoints.booking, bookingSubmissionPath, token, bookingBody); nestedString(replay, "booking", "documentNumber") != bookingNumber {
		t.Fatalf("订舱重放 = %v", replay)
	}

	byNumber := `{"booking": "` + bookingNumber + `", "answer": "SYN-CARRIER-ANSWER-BY-NUMBER", "outcome": "ACCEPTED", "quantity": 120, "decidedAt": "2026-10-19T09:00:00Z"}`
	if status, answer := postWithToken(t, endpoints.trunk, bookingAnswerPath, "presented.client.token", byNumber); status != http.StatusOK || answer["outcome"] != "BOOKING_NOT_FOUND" {
		t.Fatalf("拿单据号答复 = %d %v，want 订舱不在册（认单键是订舱标识）", status, answer)
	}
	if status, answer := postWithToken(t, endpoints.trunk, bookingAnswerPath, "presented.client.token", trunkAcceptedAnswer); status != http.StatusCreated || answer["outcome"] != "BOOKING_ANSWERED" {
		t.Fatalf("按订舱标识答复 = %d %v", status, answer)
	}
	if got := sequenceValue(t, pool, "TRANSPORT_COMMISSION"); got != 1 {
		t.Fatalf("委托序列取了 %d 个值，want 1——重放或没登时区那一份占了号", got)
	}
}

// Covers: AT-TF-373（生产装配真库面）——租户的编号规则换成与已用号整段重叠的格式：跳到上限仍撞，委托提交答 200
// NUMBERING_RULE_COLLISION 并指名委托的编号规则，整笔回滚——委托不落、序列取过的流水一个不留；换一版格式之后同一份委托
// 照常提交，接着数的流水没有被那一次撞号烧掉。
func TestAWiredCommissionThatCollidesIsRolledBackWithItsSequence(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	seeded := publishSeededSupplierAgreements(t, db)
	trunk := seeded["SYN-SUPPLIER-TRUNK-CN-SG-01"]
	endpoint := commissionSubmissionEndpointForTest(t, db)
	registerShanghaiNumberingTimeZone(t, db)
	numbers := documentNumberingComponents(t, db, docnumber.SecretTable{})
	registerRule := func(template string, from time.Time) {
		t.Helper()
		if _, err := numbers.Registrations.RegisterRule(t.Context(), docnumber.RegisterRuleCommand{
			Tenant: "SYN-TENANT-01", Type: "TRANSPORT_COMMISSION", Template: template, EffectiveFrom: from, RegisteredBy: "SYN-TEST",
		}); err != nil {
			t.Fatalf("登规则 %s：%v", template, err)
		}
	}
	registerRule("TCX-{seq:5}", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	supplier := trunk.Declarations.SupplierAgreementBody.Supplier
	submit := func(commission string) (int, map[string]any) {
		t.Helper()
		return postWithToken(t, endpoint, commissionSubmissionPath, "presented.operator.token",
			commissionBody(commission, supplier, trunk.ObjectID, trunk.Version, "2026-10-19T08:00:00Z"))
	}

	if status, answer := submit("SYN-COMMISSION-FIRST"); status != http.StatusCreated || nestedString(answer, "commission", "documentNumber") != "TCX-00001" {
		t.Fatalf("首份 = %d %v", status, answer)
	}
	// 把接下来的二十个号都做成已用：复制首份委托二十行，各带一个号。
	if _, err := pool.Exec(t.Context(),
		`INSERT INTO transport_fulfillment.transport_commission
		 SELECT (jsonb_populate_record(c, jsonb_build_object(
		            'commission_id', c.commission_id || '-USED-' || n,
		            'document_number', 'TCX-' || lpad(n::text, 5, '0')))).*
		   FROM transport_fulfillment.transport_commission AS c, generate_series(2, 21) AS n
		  WHERE c.tenant_id = 'SYN-TENANT-01' AND c.commission_id = 'SYN-COMMISSION-FIRST'`); err != nil {
		t.Fatalf("做出已用号段：%v", err)
	}
	before := sequenceValue(t, pool, "TRANSPORT_COMMISSION")

	status, answer := submit("SYN-COMMISSION-COLLIDED")
	if status != http.StatusOK || answer["outcome"] != "NUMBERING_RULE_COLLISION" || answer["numberingRule"] != "TRANSPORT_COMMISSION" || answer["commission"] != nil {
		t.Fatalf("撞号 = %d %v", status, answer)
	}
	var rows int
	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM transport_fulfillment.transport_commission
		WHERE commission_id = 'SYN-COMMISSION-COLLIDED'`).Scan(&rows); err != nil || rows != 0 {
		t.Fatalf("撞号的委托落了 %d 行 %v", rows, err)
	}
	if after := sequenceValue(t, pool, "TRANSPORT_COMMISSION"); after != before {
		t.Fatalf("撞号之后序列 %d → %d：取过的流水没随回滚释放", before, after)
	}

	registerRule("TCY-{seq:5}", time.Now().Add(-time.Minute))
	if status, answer := submit("SYN-COMMISSION-COLLIDED"); status != http.StatusCreated || nestedString(answer, "commission", "documentNumber") != "TCY-00002" {
		t.Fatalf("换格式之后 = %d %v，want TCY-00002（同粒度接着数，撞号那一次没占号）", status, answer)
	}
}

// Covers: AT-TF-373（生产装配真库面，并发）——不同委托并发提交各取一个不同的号、序列不留空号；同一份委托并发首交只取一个号，
// 输家答已有委托、回显赢家那个号。
func TestConcurrentWiredCommissionSubmissionsNeverShareOrSkipANumber(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	seeded := publishSeededSupplierAgreements(t, db)
	trunk := seeded["SYN-SUPPLIER-TRUNK-CN-SG-01"]
	endpoint := commissionSubmissionEndpointForTest(t, db)
	registerShanghaiNumberingTimeZone(t, db)
	supplier := trunk.Declarations.SupplierAgreementBody.Supplier

	const distinct, same = 6, 4
	type answered struct {
		commission string
		status     int
		outcome    string
		number     string
	}
	results := make(chan answered, distinct+same)
	var wait sync.WaitGroup
	for index := 0; index < distinct+same; index++ {
		commission := fmt.Sprintf("SYN-COMMISSION-CONCURRENT-%d", index)
		if index >= distinct {
			commission = "SYN-COMMISSION-CONCURRENT-SAME"
		}
		wait.Add(1)
		go func() {
			defer wait.Done()
			status, raw := postWithTokenRaw(endpoint, commissionSubmissionPath, "presented.operator.token",
				commissionBody(commission, supplier, trunk.ObjectID, trunk.Version, "2026-10-19T08:00:00Z"))
			var answer struct {
				Outcome    string `json:"outcome"`
				Commission *struct {
					DocumentNumber string `json:"documentNumber"`
				} `json:"commission"`
			}
			if err := json.Unmarshal([]byte(raw), &answer); err != nil || answer.Commission == nil {
				results <- answered{commission: commission, status: status, outcome: raw}
				return
			}
			results <- answered{commission, status, answer.Outcome, answer.Commission.DocumentNumber}
		}()
	}
	wait.Wait()
	close(results)

	seen := map[string]string{}
	var sameNumbers []string
	created := 0
	for result := range results {
		if result.status == http.StatusCreated {
			created++
		}
		if result.commission == "SYN-COMMISSION-CONCURRENT-SAME" {
			sameNumbers = append(sameNumbers, result.number)
			if result.outcome != "COMMISSION_SUBMITTED" && result.outcome != "EXISTING_COMMISSION" {
				t.Fatalf("同一份委托并发首交 = %d %s", result.status, result.outcome)
			}
			continue
		}
		if result.status != http.StatusCreated || result.number == "" {
			t.Fatalf("%s = %d %s", result.commission, result.status, result.outcome)
		}
		if other, taken := seen[result.number]; taken {
			t.Fatalf("%s 与 %s 拿到同一个号 %s", result.commission, other, result.number)
		}
		seen[result.number] = result.commission
	}
	if created != distinct+1 {
		t.Fatalf("新提交 %d 份，want %d", created, distinct+1)
	}
	for _, number := range sameNumbers {
		if number == "" || number != sameNumbers[0] {
			t.Fatalf("同一份委托并发首交回显的号不一：%v", sameNumbers)
		}
		if other, taken := seen[number]; taken {
			t.Fatalf("同一份委托的号 %s 与 %s 撞了", number, other)
		}
	}
	if got := sequenceValue(t, pool, "TRANSPORT_COMMISSION"); got != distinct+1 {
		t.Fatalf("序列取了 %d 个值，want %d——并发首交的输家也取了号", got, distinct+1)
	}
}
