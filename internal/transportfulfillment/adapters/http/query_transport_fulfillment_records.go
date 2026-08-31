package tfhttp

import (
	"context"
	"net/http"
	"strconv"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// ReviewCatalogueReader 是本端点消费的读口：班次册、容量池册、权威交接判断册与
// 有效交付结果册四本册子的列表读面（管理台 transport-fulfillment-review 页）。
type ReviewCatalogueReader interface {
	ListTransportSchedules(
		ctx context.Context,
		tenant domain.TenantID,
		limit int,
	) ([]ports.TransportScheduleCatalogueRow, error)
	ListCapacityPools(
		ctx context.Context,
		tenant domain.TenantID,
		limit int,
	) ([]ports.CapacityPoolCatalogueRow, error)
	ListTransportHandovers(
		ctx context.Context,
		tenant domain.TenantID,
		limit int,
	) ([]ports.TransportHandoverCatalogueRow, error)
	ListEffectiveDeliveries(
		ctx context.Context,
		tenant domain.TenantID,
		limit int,
	) ([]ports.EffectiveDeliveryCatalogueRow, error)
}

// 编译期锁缝：读口形状与端口保持一致——本端点不新造查询语义。
var _ ReviewCatalogueReader = ports.ReviewCatalogueRead(nil)

const (
	outcomeTransportSchedulesListed  = "TRANSPORT_SCHEDULES_LISTED"
	outcomeCapacityPoolsListed       = "CAPACITY_POOLS_LISTED"
	outcomeTransportHandoversListed  = "TRANSPORT_HANDOVERS_LISTED"
	outcomeEffectiveDeliveriesListed = "EFFECTIVE_DELIVERIES_LISTED"
)

const (
	registryTransportSchedule = "transport-schedule"
	registryCapacityPool      = "capacity-pool"
	registryTransportHandover = "transport-handover"
	registryEffectiveDelivery = "effective-delivery"
)

// NewQueryTransportFulfillmentRecordsEndpoint 交回运输履约查阅页四本册子的 HTTP
// 入口（GET /transport-fulfillment-records，票 admin-skeleton-closure-batch/05）。
//
// 本页是治理查阅面，不是一线作业端（ADR-0021）：实时现场作业走设备渠道的命令端点，
// 本端点零登记零编辑动作，只上列已登记的履约判断与事实。页面五区里承运总单与运输
// 舱单一区在存储上还没有登记册，本端点的册名封闭集刻意没有那一格——没有表就没有
// 读法，答一份恒空的册子会把「无处可登」演成「登记册为空」（票 05 Comments 记明）。
//
// **不下推交接改判或交付更正参数**：权威交接结果与有效交付的更正各有命令用例，
// 查阅面收下这些参数就等于让目录读口长出第二种「处置」语义。
func NewQueryTransportFulfillmentRecordsEndpoint(
	intake CatalogueQueryIntake,
	reader ReviewCatalogueReader,
) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if !guardGet(response, request) {
			return
		}
		registry := request.URL.Query().Get("registry")
		if registry != registryTransportSchedule &&
			registry != registryCapacityPool &&
			registry != registryTransportHandover &&
			registry != registryEffectiveDelivery {
			writeProblem(response, http.StatusBadRequest, codeMalformedRequest)
			return
		}
		query, ok := intakeCatalogueQuery(response, request, intake)
		if !ok {
			return
		}
		tenant := query.Scope.Tenant()

		switch registry {
		case registryTransportSchedule:
			serveTransportSchedules(response, request, reader, tenant, query.Limit)
		case registryCapacityPool:
			serveCapacityPools(response, request, reader, tenant, query.Limit)
		case registryTransportHandover:
			serveTransportHandovers(response, request, reader, tenant, query.Limit)
		case registryEffectiveDelivery:
			serveEffectiveDeliveries(response, request, reader, tenant, query.Limit)
		}
	})
}

func serveTransportSchedules(
	response http.ResponseWriter,
	request *http.Request,
	reader ReviewCatalogueReader,
	tenant domain.TenantID,
	limit int,
) {
	rows, err := reader.ListTransportSchedules(request.Context(), tenant, limit)
	if err != nil {
		writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
		return
	}
	bodies := make([]transportScheduleBody, 0, len(rows))
	for _, row := range rows {
		bodies = append(bodies, transportScheduleBody{
			ScheduleID: row.ScheduleID,
			Direction:  row.Direction,
			DepartsAt:  rfc3339(row.DepartsAt),
			RecordedAt: rfc3339(row.RecordedAt),
		})
	}
	writeJSON(response, http.StatusOK, transportScheduleListResponse{
		Outcome:   outcomeTransportSchedulesListed,
		Schedules: bodies,
	})
}

func serveCapacityPools(
	response http.ResponseWriter,
	request *http.Request,
	reader ReviewCatalogueReader,
	tenant domain.TenantID,
	limit int,
) {
	rows, err := reader.ListCapacityPools(request.Context(), tenant, limit)
	if err != nil {
		writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
		return
	}
	bodies := make([]capacityPoolBody, 0, len(rows))
	for _, row := range rows {
		bodies = append(bodies, capacityPoolBody{
			PoolID:     row.PoolID,
			Schedule:   row.Schedule,
			Unit:       row.Unit,
			Capacity:   quantity(row.Capacity),
			Reserved:   quantity(row.Reserved),
			Released:   quantity(row.Released),
			Consumed:   quantity(row.Consumed),
			RecordedAt: rfc3339(row.RecordedAt),
		})
	}
	writeJSON(response, http.StatusOK, capacityPoolListResponse{
		Outcome: outcomeCapacityPoolsListed,
		Pools:   bodies,
	})
}

func serveTransportHandovers(
	response http.ResponseWriter,
	request *http.Request,
	reader ReviewCatalogueReader,
	tenant domain.TenantID,
	limit int,
) {
	rows, err := reader.ListTransportHandovers(request.Context(), tenant, limit)
	if err != nil {
		writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
		return
	}
	bodies := make([]transportHandoverBody, 0, len(rows))
	for _, row := range rows {
		bodies = append(bodies, transportHandoverBody{
			Object:          row.Object,
			Scope:           row.Scope,
			Version:         row.Version,
			ReleasedBy:      row.ReleasedBy,
			ReceivedBy:      row.ReceivedBy,
			Verdict:         row.Verdict,
			Basis:           row.Basis,
			CorrectsVersion: row.CorrectsVersion,
			CorrectedAt:     optionalInstant(row.CorrectedAt),
			JudgedAt:        rfc3339(row.JudgedAt),
			RecordedAt:      rfc3339(row.RecordedAt),
		})
	}
	writeJSON(response, http.StatusOK, transportHandoverListResponse{
		Outcome:   outcomeTransportHandoversListed,
		Handovers: bodies,
	})
}

func serveEffectiveDeliveries(
	response http.ResponseWriter,
	request *http.Request,
	reader ReviewCatalogueReader,
	tenant domain.TenantID,
	limit int,
) {
	rows, err := reader.ListEffectiveDeliveries(request.Context(), tenant, limit)
	if err != nil {
		writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
		return
	}
	bodies := make([]effectiveDeliveryBody, 0, len(rows))
	for _, row := range rows {
		bodies = append(bodies, effectiveDeliveryBody{
			Object:          row.Object,
			Attempt:         row.Attempt,
			Version:         row.Version,
			Place:           row.Place,
			Method:          row.Method,
			Recipient:       row.Recipient,
			Proof:           row.Proof,
			CorrectsVersion: row.CorrectsVersion,
			CorrectedAt:     optionalInstant(row.CorrectedAt),
			OccurredAt:      rfc3339(row.OccurredAt),
			RecordedAt:      rfc3339(row.RecordedAt),
		})
	}
	writeJSON(response, http.StatusOK, effectiveDeliveryListResponse{
		Outcome:    outcomeEffectiveDeliveriesListed,
		Deliveries: bodies,
	})
}

// quantity 把容量数量转写成十进制计数串：int64 直投 JSON number 在 2^53 以上的取值
// 会被 JS 读者悄悄取整——串是照实转写，数才是替读者做的算术承诺（判据同
// settlementhttp minorAmount）。
func quantity(value int64) string {
	return strconv.FormatInt(value, 10)
}

type transportScheduleListResponse struct {
	Outcome   string                  `json:"outcome"`
	Schedules []transportScheduleBody `json:"schedules"`
}

// transportScheduleBody 逐字段透出一个具体班次。
//
// 没有执行准备键、也没有实际执行键——那两组在存储上还没有登记格（班次行只登身份、
// 方向与出发时刻），不代填「未出发」一类派生状态词；缺席说的是「无处可登」，代填
// 会把它演成「已登而未发生」。
type transportScheduleBody struct {
	ScheduleID string `json:"scheduleId"`
	Direction  string `json:"direction"`
	DepartsAt  string `json:"departsAt"`
	RecordedAt string `json:"recordedAt"`
}

type capacityPoolListResponse struct {
	Outcome string             `json:"outcome"`
	Pools   []capacityPoolBody `json:"pools"`
}

// capacityPoolBody 逐字段透出一个容量池。
//
// 四量各自照实转写（CONTEXT：有效容量、已预占、已释放、实际使用分别维护），不互相
// 抵扣——可用量是领域按时点算的判断，读面不代算；单位随维度引用给出，不同池之间
// 不换算（容量维度不折算）。
type capacityPoolBody struct {
	PoolID     string `json:"poolId"`
	Schedule   string `json:"schedule"`
	Unit       string `json:"unit"`
	Capacity   string `json:"capacity"`
	Reserved   string `json:"reserved"`
	Released   string `json:"released"`
	Consumed   string `json:"consumed"`
	RecordedAt string `json:"recordedAt"`
}

type transportHandoverListResponse struct {
	Outcome   string                  `json:"outcome"`
	Handovers []transportHandoverBody `json:"handovers"`
}

// transportHandoverBody 逐字段透出一个权威交接判断版本。
//
// verdict 是三值封闭词（HANDED_OVER / REFUSED / PENDING_CONFIRMATION），原样透出
// 不折并——整批、整车结论只能由对象级结果派生，本册行粒度就是载运对象。basis 只在
// 拒收与待确认上在场（已交接不带依据）；corrects 两键成对缺席表示首登版本——一行
// 一版本，更正是新行指回前版，原判断在册面上继续可见。交接边界由 releasedBy 与
// receivedBy 两个参与方引用表达，组合成「A → B」是呈现的事。
type transportHandoverBody struct {
	Object          string `json:"object"`
	Scope           string `json:"scope"`
	Version         string `json:"version"`
	ReleasedBy      string `json:"releasedBy"`
	ReceivedBy      string `json:"receivedBy"`
	Verdict         string `json:"verdict"`
	Basis           string `json:"basis,omitempty"`
	CorrectsVersion string `json:"correctsVersion,omitempty"`
	CorrectedAt     string `json:"correctedAt,omitempty"`
	JudgedAt        string `json:"judgedAt"`
	RecordedAt      string `json:"recordedAt"`
}

type effectiveDeliveryListResponse struct {
	Outcome    string                  `json:"outcome"`
	Deliveries []effectiveDeliveryBody `json:"deliveries"`
}

// effectiveDeliveryBody 逐字段透出一行当前版有效交付结果。
//
// proof 是交付证明引用不是证据内容——证据构成属交付证明本体，列面只指名，不把
// 「已签收」演成一个可直接修改的状态。corrects 两键在场即这行是更正版本：更正翻旧
// 插新，本册只列当前版，历史版本按键与版本走详情读口。
type effectiveDeliveryBody struct {
	Object          string `json:"object"`
	Attempt         string `json:"attempt"`
	Version         string `json:"version"`
	Place           string `json:"place"`
	Method          string `json:"method"`
	Recipient       string `json:"recipient"`
	Proof           string `json:"proof"`
	CorrectsVersion string `json:"correctsVersion,omitempty"`
	CorrectedAt     string `json:"correctedAt,omitempty"`
	OccurredAt      string `json:"occurredAt"`
	RecordedAt      string `json:"recordedAt"`
}
