package tfhttp_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	tfhttp "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/http"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// 本文件证装载分配口在容量保障各格上的传输层纪律（票 demo-chain-gaps/21，ADR-0203 决定一、二）：带预占引用的分配 201 并回显
// 逐池的分配量；`须引用预占`、`预占不在本班次`、`分配超出`各 200，各带判出那一格所对的东西；分配行带越过余量一类键是坏报文。
// 载荷经生产的操作者决定 Intake 封闭解码。夹具全为合成值（S 级）：班次 SYN-TPL-1/2026-10-21 的重量池 5000 KG 上一份预占
// SYN-RES-1 1200 KG，别的分配已在上面分走 1000 KG。

const (
	allocationSchedule = "SYN-TPL-1/2026-10-21"
	allocationPoolID   = "SYN-TPL-1/2026-10-21/WEIGHT"
)

// schedulePoolsDouble 是按班次读池的只读替身。
type schedulePoolsDouble struct{ records []ports.CapacityPoolRecord }

func (double schedulePoolsDouble) ListPoolsOnSchedules(
	_ context.Context,
	tenant domain.TenantID,
	schedules []domain.ScheduleReference,
) ([]ports.CapacityPoolRecord, error) {
	var found []ports.CapacityPoolRecord
	for _, record := range double.records {
		for _, schedule := range schedules {
			if record.Key.TenantID == tenant && record.Pool.Schedule() == schedule {
				found = append(found, record)
			}
		}
	}
	return found, nil
}

func noPools() schedulePoolsDouble { return schedulePoolsDouble{} }

func allocationPoolRecord(t *testing.T) ports.CapacityPoolRecord {
	t.Helper()
	tenant := httpValue(t, domain.NewTenantID, operatorTenant)
	source, err := domain.NewCapacityDemandSource(domain.CapacityDemandOperatorDeclaration,
		httpValue(t, domain.NewCapacityDemandSourceReference, "SYN-DEMAND-DECL-01/v1"))
	if err != nil {
		t.Fatal(err)
	}
	pool, err := domain.RehydrateCapacityPool(domain.CapacityPoolSpec{
		TenantID:  tenant,
		Schedule:  httpValue(t, domain.NewScheduleReference, allocationSchedule),
		Dimension: domain.CapacityWeight,
		Unit:      httpValue(t, domain.NewQuantityUnitReference, "KG"),
		Capacity:  5000,
	}, []domain.CapacityReservationSnapshot{{
		Reference:  httpValue(t, domain.NewCapacityReservationReference, "SYN-RES-1"),
		Quantity:   1200,
		ValidUntil: time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC),
		Source:     source,
		ReservedAt: time.Date(2026, 10, 19, 8, 0, 0, 0, time.UTC),
		Allocations: []domain.CapacityAllocationSnapshot{{
			Assignment: httpValue(t, domain.NewLoadAssignmentReference, "SYN-LA-OTHER"),
			Version:    httpValue(t, domain.NewLoadAssignmentVersion, "SYN-LAV-1"),
			Quantity:   1000,
		}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	return ports.CapacityPoolRecord{
		Key:        ports.CapacityPoolKey{TenantID: tenant, Pool: pool.Pool()},
		Pool:       pool,
		RecordedAt: time.Date(2026, 10, 19, 8, 0, 0, 0, time.UTC),
	}
}

func allocationEndpoint(t *testing.T, registry *loadAssignmentRegistryDouble) http.Handler {
	t.Helper()
	handler := application.NewFormLoadAssignmentHandler(application.FormLoadAssignmentDeps{
		Assignments: registry,
		Schedules:   registeredSchedules(operatorTenant, allocationSchedule),
		Pools:       schedulePoolsDouble{records: []ports.CapacityPoolRecord{allocationPoolRecord(t)}},
		Clock:       scheduleClock{},
	})
	return tfhttp.NewFormLoadAssignmentEndpoint(operatorIntake(t, authenticatorFake{}), handler)
}

func allocationBody(assignment, allocations string) string {
	return `{"assignment": "` + assignment + `", "schedule": "` + allocationSchedule + `", "members": ["SYN-PARCEL-1", "SYN-PARCEL-2"],` +
		` "version": "SYN-LAV-1", "assignedAt": "2026-10-20T06:00:00Z", "allocations": [` + allocations + `]}`
}

func weightAllocationLine(pool string, quantity string) string {
	return `{"pool": "` + pool + `", "reservation": "SYN-RES-1", "quantity": ` + quantity + `, "unit": "KG",` +
		` "source": {"kind": "OPERATOR_DECLARATION", "reference": "SYN-DEMAND-DECL-01/v1"}}`
}

// Covers: 票 demo-chain-gaps/21（AT-TF-171）——带预占引用的分配 201 LOAD_ASSIGNMENT_FORMED，逐池回显分配量、得法与来源；同一版本
// 重投 200 已在册，回显的仍是那一组分配量。
func TestFormLoadAssignmentWithAReservationOnEveryPoolAnswersCreated(t *testing.T) {
	endpoint := allocationEndpoint(t, newLoadAssignmentRegistry())
	body := allocationBody("SYN-LA-0001", weightAllocationLine(allocationPoolID, "200"))

	response, fields := postDecision(t, endpoint, "t", body)
	if response.Code != http.StatusCreated || string(fields["outcome"]) != `"LOAD_ASSIGNMENT_FORMED"` {
		t.Fatalf("status = %d body = %s", response.Code, response.Body.String())
	}
	if !strings.Contains(string(fields["allocations"]), `{"pool":"`+allocationPoolID+`","reservation":"SYN-RES-1","quantity":"200",`+
		`"quantityBasis":"DECLARED","unit":"KG","source":{"kind":"OPERATOR_DECLARATION","reference":"SYN-DEMAND-DECL-01/v1"}}`) {
		t.Fatalf("allocations = %s", fields["allocations"])
	}
	replay, replayed := postDecision(t, endpoint, "t", body)
	if replay.Code != http.StatusOK || string(replayed["outcome"]) != `"LOAD_ASSIGNMENT_VERSION_EXISTS"` ||
		string(replayed["allocations"]) != string(fields["allocations"]) {
		t.Fatalf("重投 = %d %s", replay.Code, replay.Body.String())
	}
}

// Covers: 票 demo-chain-gaps/21（AT-TF-175）——超出那份预占剩下的量 200 ALLOCATION_EXCEEDED：带余量（1200 减别的分配 1000，余 200）、
// 挡下的那一行、池与那一份预占（已分配照实透出），不回显任何版本。
func TestFormLoadAssignmentBeyondTheReservationAnswersAllocationExceeded(t *testing.T) {
	registry := newLoadAssignmentRegistry()
	response, fields := postDecision(t, allocationEndpoint(t, registry), "t",
		allocationBody("SYN-LA-0001", weightAllocationLine(allocationPoolID, "201")))
	if response.Code != http.StatusOK || string(fields["outcome"]) != `"ALLOCATION_EXCEEDED"` ||
		string(fields["availableQuantity"]) != `"200"` {
		t.Fatalf("status = %d body = %s", response.Code, response.Body.String())
	}
	if string(nested(t, fields["refusedAllocation"])["quantity"]) != `"201"` ||
		string(nested(t, fields["pool"])["poolId"]) != `"`+allocationPoolID+`"` {
		t.Fatalf("挡下的那一行或池 = %s", response.Body.String())
	}
	reservation := nested(t, fields["reservation"])
	if string(reservation["quantity"]) != `"1200"` || string(reservation["allocated"]) != `"1000"` {
		t.Fatalf("reservation = %s", fields["reservation"])
	}
	if fields["version"] != nil || fields["allocations"] != nil || len(registry.records) != 0 {
		t.Fatalf("分配超出不该回显或登记版本：%s", response.Body.String())
	}
}

// Covers: 票 demo-chain-gaps/21（AT-TF-173、AT-TF-174）——班次挂了池而一行都没有 200 CAPACITY_RESERVATION_REQUIRED，带缺行的池；
// 一行指的是别的班次的池 200 RESERVATION_NOT_ON_SCHEDULE，带指错的那一行。两格都不登记。
func TestFormLoadAssignmentAnswersTheCapacityAssuranceGrades(t *testing.T) {
	registry := newLoadAssignmentRegistry()
	endpoint := allocationEndpoint(t, registry)

	response, fields := postDecision(t, endpoint, "t", allocationBody("SYN-LA-0001", ""))
	if response.Code != http.StatusOK || string(fields["outcome"]) != `"CAPACITY_RESERVATION_REQUIRED"` ||
		!strings.Contains(string(fields["poolsWithoutReservation"]), `"poolId":"`+allocationPoolID+`"`) {
		t.Fatalf("须引用预占 = %d %s", response.Code, response.Body.String())
	}

	response, fields = postDecision(t, endpoint, "t",
		allocationBody("SYN-LA-0001", weightAllocationLine("SYN-TPL-1/2026-10-22/WEIGHT", "10")))
	if response.Code != http.StatusOK || string(fields["outcome"]) != `"RESERVATION_NOT_ON_SCHEDULE"` ||
		!strings.Contains(string(fields["allocationsNotOnSchedule"]), `"pool":"SYN-TPL-1/2026-10-22/WEIGHT"`) {
		t.Fatalf("预占不在本班次 = %d %s", response.Code, response.Body.String())
	}
	if len(registry.records) != 0 {
		t.Fatalf("登记了 %d 个版本", len(registry.records))
	}
}

// Covers: 票 demo-chain-gaps/21——分配行带越过余量或审批一类的键是坏报文（400），走不到判断；`quantityFromMembers` 逐字段译成
// 命令。没挂池的班次不带 `allocations` 照旧 201。
func TestLoadAssignmentAllocationLinesAreDecodedClosed(t *testing.T) {
	override := allocationBody("SYN-LA-0001", strings.Replace(weightAllocationLine(allocationPoolID, "10"), `"unit"`, `"override": true, "unit"`, 1))
	response := httptest.NewRecorder()
	allocationEndpoint(t, newLoadAssignmentRegistry()).ServeHTTP(response, bearerRequest("t", override))
	if response.Code != http.StatusBadRequest {
		t.Fatalf("带 override 键 = %d %s, want 400", response.Code, response.Body.String())
	}

	members := allocationBody("SYN-LA-0001", `{"pool": "SYN-TPL-1/2026-10-21/PIECES", "reservation": "SYN-RES-2", "quantityFromMembers": true,`+
		` "unit": "PCS", "source": {"kind": "PREPARATION_DEMAND_SNAPSHOT", "reference": "SYN-PREP-1"}}`)
	command, err := operatorIntake(t, authenticatorFake{}).IntakeLoadAssignment(context.Background(), bearerRequest("t", members))
	if err != nil {
		t.Fatal(err)
	}
	if len(command.Allocations) != 1 || !command.Allocations[0].FromMembers || command.Allocations[0].Quantity != 0 ||
		command.Allocations[0].Pool != "SYN-TPL-1/2026-10-21/PIECES" || command.Allocations[0].Reservation != "SYN-RES-2" ||
		command.Allocations[0].Unit != "PCS" || command.Allocations[0].SourceKind != "PREPARATION_DEMAND_SNAPSHOT" ||
		command.Allocations[0].SourceReference != "SYN-PREP-1" {
		t.Fatalf("allocation line = %+v", command.Allocations)
	}

	handler := application.NewFormLoadAssignmentHandler(application.FormLoadAssignmentDeps{
		Assignments: newLoadAssignmentRegistry(),
		Schedules:   registeredSchedules(operatorTenant, allocationSchedule),
		Pools:       noPools(),
		Clock:       scheduleClock{},
	})
	plain := `{"assignment": "SYN-LA-0002", "schedule": "` + allocationSchedule + `", "members": ["SYN-PARCEL-1"],` +
		` "version": "SYN-LAV-1", "assignedAt": "2026-10-20T06:00:00Z"}`
	formed, fields := postDecision(t, tfhttp.NewFormLoadAssignmentEndpoint(operatorIntake(t, authenticatorFake{}), handler), "t", plain)
	if formed.Code != http.StatusCreated || fields["allocations"] != nil {
		t.Fatalf("没挂池的班次 = %d %s", formed.Code, formed.Body.String())
	}
}
