package tfhttp

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// ReviewCatalogueReader 是本端点消费的读口：班次册、周期安排册、容量池册、权威交接判断册、有效交付结果册、
// 总单册、运输准备结果册、订舱册与承运答复挡回册等的列表读面（管理台 transport-fulfillment-review 页）。
type ReviewCatalogueReader interface {
	ListTransportSchedules(
		ctx context.Context,
		tenant domain.TenantID,
		limit int,
	) ([]ports.TransportScheduleCatalogueRow, error)
	ListScheduleTemplates(
		ctx context.Context,
		tenant domain.TenantID,
		limit int,
	) ([]ports.ScheduleTemplateCatalogueRow, error)
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
	ListCarrierMasterDocuments(
		ctx context.Context,
		tenant domain.TenantID,
		limit int,
	) ([]ports.CarrierMasterDocumentCatalogueRow, error)
	ListTransportPreparations(
		ctx context.Context,
		tenant domain.TenantID,
		limit int,
	) ([]ports.TransportPreparationCatalogueRow, error)
	ListDeliveryDispatchPendings(
		ctx context.Context,
		tenant domain.TenantID,
		limit int,
	) ([]ports.DeliveryDispatchPendingRecord, error)
	ListDeliveryDispatchReaskCadences(
		ctx context.Context,
		tenant domain.TenantID,
		limit int,
	) ([]ports.DeliveryDispatchReaskCadenceRecord, error)
	ListActualFulfillmentUnformed(
		ctx context.Context,
		tenant domain.TenantID,
		limit int,
	) ([]ports.ActualFulfillmentUnformedRecord, error)
	ListBookingRequests(
		ctx context.Context,
		tenant domain.TenantID,
		limit int,
	) ([]ports.BookingRequestCatalogueRow, error)
	ListBookingAnswerBounces(
		ctx context.Context,
		tenant domain.TenantID,
		filter ports.BookingAnswerBounceFilter,
		limit int,
	) ([]ports.BookingAnswerBounceCatalogueRow, error)
}

// 编译期锁缝：读口形状与端口保持一致——本端点不新造查询语义。
var _ ReviewCatalogueReader = ports.ReviewCatalogueRead(nil)

const (
	outcomeTransportSchedulesListed        = "TRANSPORT_SCHEDULES_LISTED"
	outcomeScheduleTemplatesListed         = "SCHEDULE_TEMPLATES_LISTED"
	outcomeCapacityPoolsListed             = "CAPACITY_POOLS_LISTED"
	outcomeTransportHandoversListed        = "TRANSPORT_HANDOVERS_LISTED"
	outcomeEffectiveDeliveriesListed       = "EFFECTIVE_DELIVERIES_LISTED"
	outcomeCarrierMasterDocumentsListed    = "CARRIER_MASTER_DOCUMENTS_LISTED"
	outcomeTransportPreparationsListed     = "TRANSPORT_PREPARATIONS_LISTED"
	outcomeDeliveryDispatchPendingsListed  = "DELIVERY_DISPATCH_PENDINGS_LISTED"
	outcomeReaskCadencesListed             = "REASK_CADENCES_LISTED"
	outcomeActualFulfillmentUnformedListed = "ACTUAL_FULFILLMENT_UNFORMED_LISTED"
	outcomeBookingRequestsListed           = "BOOKING_REQUESTS_LISTED"
	outcomeBookingAnswerBouncesListed      = "BOOKING_ANSWER_BOUNCES_LISTED"
)

const (
	registryTransportSchedule         = "transport-schedule"
	registryScheduleTemplate          = "transport-schedule-template"
	registryCapacityPool              = "capacity-pool"
	registryTransportHandover         = "transport-handover"
	registryEffectiveDelivery         = "effective-delivery"
	registryCarrierMasterDocument     = "carrier-master-document"
	registryTransportPreparation      = "transport-preparation"
	registryDeliveryDispatchPending   = "delivery-dispatch-pending"
	registryReaskCadence              = "delivery-dispatch-reask-cadence"
	registryActualFulfillmentUnformed = "actual-fulfillment-unformed"
	registryBookingRequest            = "booking-request"
	registryBookingAnswerBounce       = "booking-answer-bounce"
)

// NewQueryTransportFulfillmentRecordsEndpoint 交回运输履约查阅页各本册子的 HTTP
// 入口（GET /transport-fulfillment-records，票 admin-skeleton-closure-batch/05；总单一格
// 由 ADR-0113 决定六补入，周期安排与运输准备结果两格由 ADR-0201 补入，订舱一格由票
// demo-chain-gaps/48、承运答复挡回一格由票 demo-chain-gaps/167 补入）。
//
// 本页是治理查阅面，不是一线作业端（ADR-0021）：实时现场作业走设备渠道的命令端点，
// 本端点零登记零编辑动作，只上列已登记的履约判断与事实。册名封闭集只有有表的册：
// 总单在 ADR-0113 立册之后进了封闭集，空册从此是「登记册为空」（ADR-0077 决定四）；
// 运输舱单在存储上仍没有登记册，它的名字仍不在集合内——没有表就没有读法，答一份
// 恒空的册子会把「无处可登」演成「登记册为空」（票 05 Comments 记明）。
//
// **不下推交接改判或交付更正参数**：权威交接结果与有效交付的更正各有命令用例，
// 查阅面收下这些参数就等于让目录读口长出第二种「处置」语义。挡回册那一格收两个筛选参数
// （booking、source），只筛不改；别的册子不认它们。
//
// 承运答复挡回一格只挂这一口的操作者查阅读：挡回册说得出一个订舱在不在、归不归某个
// 承运商，开给集成客户端族就把 ADR-0239 决定一合并掉的那一位又交了回去。
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
			registry != registryScheduleTemplate &&
			registry != registryCapacityPool &&
			registry != registryTransportHandover &&
			registry != registryEffectiveDelivery &&
			registry != registryCarrierMasterDocument &&
			registry != registryTransportPreparation &&
			registry != registryDeliveryDispatchPending &&
			registry != registryReaskCadence &&
			registry != registryActualFulfillmentUnformed &&
			registry != registryBookingRequest &&
			registry != registryBookingAnswerBounce {
			writeProblem(response, http.StatusBadRequest, codeMalformedRequest)
			return
		}
		var bounceFilter ports.BookingAnswerBounceFilter
		if registry == registryBookingAnswerBounce {
			filter, ok := bookingAnswerBounceFilter(request)
			if !ok {
				writeProblem(response, http.StatusBadRequest, codeMalformedRequest)
				return
			}
			bounceFilter = filter
		}
		query, ok := intakeCatalogueQuery(response, request, intake)
		if !ok {
			return
		}
		tenant := query.Scope.Tenant()

		switch registry {
		case registryTransportSchedule:
			serveTransportSchedules(response, request, reader, tenant, query.Limit)
		case registryScheduleTemplate:
			serveScheduleTemplates(response, request, reader, tenant, query.Limit)
		case registryCapacityPool:
			serveCapacityPools(response, request, reader, tenant, query.Limit)
		case registryTransportHandover:
			serveTransportHandovers(response, request, reader, tenant, query.Limit)
		case registryEffectiveDelivery:
			serveEffectiveDeliveries(response, request, reader, tenant, query.Limit)
		case registryCarrierMasterDocument:
			serveCarrierMasterDocuments(response, request, reader, tenant, query.Limit)
		case registryTransportPreparation:
			serveTransportPreparations(response, request, reader, tenant, query.Limit)
		case registryDeliveryDispatchPending:
			serveDeliveryDispatchPendings(response, request, reader, tenant, query.Limit)
		case registryReaskCadence:
			serveReaskCadences(response, request, reader, tenant, query.Limit)
		case registryActualFulfillmentUnformed:
			serveActualFulfillmentUnformed(response, request, reader, tenant, query.Limit)
		case registryBookingRequest:
			serveBookingRequests(response, request, reader, tenant, query.Limit)
		case registryBookingAnswerBounce:
			serveBookingAnswerBounces(response, request, reader, tenant, bounceFilter, query.Limit)
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
			ScheduleID:      row.ScheduleID,
			Version:         row.Version,
			Connection:      row.Connection,
			FromNode:        row.FromNode,
			ToNode:          row.ToNode,
			DepartsAt:       rfc3339(row.DepartsAt),
			ArrivesAt:       rfc3339(row.ArrivesAt),
			ServiceScope:    row.ServiceScope,
			Origin:          row.Origin,
			Template:        row.Template,
			TemplateVersion: row.TemplateVersion,
			RunningDate:     row.RunningDate,
			RecordedAt:      rfc3339(row.RecordedAt),
		})
	}
	writeJSON(response, http.StatusOK, transportScheduleListResponse{
		Outcome:   outcomeTransportSchedulesListed,
		Schedules: bodies,
	})
}

func serveScheduleTemplates(
	response http.ResponseWriter,
	request *http.Request,
	reader ReviewCatalogueReader,
	tenant domain.TenantID,
	limit int,
) {
	rows, err := reader.ListScheduleTemplates(request.Context(), tenant, limit)
	if err != nil {
		writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
		return
	}
	bodies := make([]scheduleTemplateBody, 0, len(rows))
	for _, row := range rows {
		bodies = append(bodies, scheduleTemplateBody{
			Template:           row.Template,
			Version:            row.Version,
			Connection:         row.Connection,
			FromNode:           row.FromNode,
			ToNode:             row.ToNode,
			Weekdays:           append([]string{}, row.Weekdays...),
			DepartureLocalTime: domain.LocalTimeOfDay(row.DepartureLocalMinute),
			DurationMinutes:    row.DurationMinutes,
			TimeZone:           row.TimeZone,
			ServiceScope:       row.ServiceScope,
			EffectiveFrom:      row.EffectiveFrom,
			EffectiveUntil:     row.EffectiveUntil,
			Capacities:         capacityDeclarationRows(row.Capacities),
			RecordedAt:         rfc3339(row.RecordedAt),
		})
	}
	writeJSON(response, http.StatusOK, scheduleTemplateListResponse{
		Outcome:   outcomeScheduleTemplatesListed,
		Templates: bodies,
	})
}

func capacityDeclarationRows(rows []ports.ScheduleTemplateCapacityRow) []capacityDeclarationBody {
	bodies := make([]capacityDeclarationBody, 0, len(rows))
	for _, row := range rows {
		bodies = append(bodies, capacityDeclarationBody{
			Dimension: row.Dimension,
			Unit:      row.Unit,
			Capacity:  quantity(row.Capacity),
		})
	}
	return bodies
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
			Dimension:  row.Dimension,
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
			ReleasingNode:   row.ReleasingNode,
			ReceivingNode:   row.ReceivingNode,
			OnwardNode:      row.OnwardNode,
			GateReading:     row.GateReading,
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
			GateReading:     row.GateReading,
		})
	}
	writeJSON(response, http.StatusOK, effectiveDeliveryListResponse{
		Outcome:    outcomeEffectiveDeliveriesListed,
		Deliveries: bodies,
	})
}

func serveCarrierMasterDocuments(
	response http.ResponseWriter,
	request *http.Request,
	reader ReviewCatalogueReader,
	tenant domain.TenantID,
	limit int,
) {
	rows, err := reader.ListCarrierMasterDocuments(request.Context(), tenant, limit)
	if err != nil {
		writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
		return
	}
	bodies := make([]carrierMasterDocumentBody, 0, len(rows))
	for _, row := range rows {
		bodies = append(bodies, carrierMasterDocumentBody{
			Document:         row.Document,
			Version:          row.Version,
			Issuer:           row.Issuer,
			Scope:            row.Scope,
			Commission:       row.Commission,
			Booking:          row.Booking,
			Standing:         row.Standing,
			Supersedes:       row.Supersedes,
			ReplacedBy:       row.ReplacedBy,
			AssociationCount: quantity(row.AssociationCount),
			ChangedAt:        optionalInstant(row.ChangedAt),
			RecordedAt:       rfc3339(row.RecordedAt),
		})
	}
	writeJSON(response, http.StatusOK, carrierMasterDocumentListResponse{
		Outcome:         outcomeCarrierMasterDocumentsListed,
		MasterDocuments: bodies,
	})
}

func serveTransportPreparations(
	response http.ResponseWriter,
	request *http.Request,
	reader ReviewCatalogueReader,
	tenant domain.TenantID,
	limit int,
) {
	rows, err := reader.ListTransportPreparations(request.Context(), tenant, limit)
	if err != nil {
		writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
		return
	}
	bodies := make([]transportPreparationBody, 0, len(rows))
	for _, row := range rows {
		demand := make([]preparationDemandBody, 0, len(row.Demand))
		for _, each := range row.Demand {
			demand = append(demand, preparationDemandBody(each))
		}
		candidates := make([]preparationCandidateBody, 0, len(row.Candidates))
		for _, candidate := range row.Candidates {
			pools := make([]preparationPoolBody, 0, len(candidate.Pools))
			for _, pool := range candidate.Pools {
				pools = append(pools, preparationPoolBody{
					Pool:      pool.Pool,
					Dimension: pool.Dimension,
					Unit:      pool.Unit,
					Available: quantity(pool.Available),
					Requested: pool.Requested,
					Verdict:   pool.Verdict,
				})
			}
			candidates = append(candidates, preparationCandidateBody{
				Schedule:  candidate.Schedule,
				Version:   candidate.Version,
				DepartsAt: rfc3339(candidate.DepartsAt),
				ArrivesAt: rfc3339(candidate.ArrivesAt),
				Verdict:   candidate.Verdict,
				Pools:     pools,
			})
		}
		bodies = append(bodies, transportPreparationBody{
			PlannedLeg:   row.PlannedLeg,
			Object:       row.Object,
			Rule:         row.Rule,
			Outcome:      row.Outcome,
			Connection:   row.Connection,
			FromNode:     row.FromNode,
			ToNode:       row.ToNode,
			WindowStart:  rfc3339(row.WindowStart),
			WindowEnd:    rfc3339(row.WindowEnd),
			DemandSource: row.DemandSource,
			DemandAnchor: row.DemandAnchor,
			Demand:       demand,
			Selected:     row.Selected,
			Candidates:   candidates,
			JudgedAt:     rfc3339(row.JudgedAt),
			RecordedAt:   rfc3339(row.RecordedAt),
		})
	}
	writeJSON(response, http.StatusOK, transportPreparationListResponse{
		Outcome:      outcomeTransportPreparationsListed,
		Preparations: bodies,
	})
}

// bookingAnswerBounceFilter 读挡回册的两个筛选参数。缺席即不筛；给了却立不起来（空白、带 NUL、订舱标识超过字节界）是坏请求，
// 不当作没给——把一个写坏的筛选静默放宽成全册，读的人会以为那个订舱没有挡回。订舱标识照领域构造门判，答复方是集成客户端绑定
// 的来源身份、领域里没有它的类型，只判非空白与 NUL。
func bookingAnswerBounceFilter(request *http.Request) (ports.BookingAnswerBounceFilter, bool) {
	values := request.URL.Query()
	var filter ports.BookingAnswerBounceFilter
	if values.Has("booking") {
		booking, err := domain.NewBookingReference(values.Get("booking"))
		if err != nil {
			return ports.BookingAnswerBounceFilter{}, false
		}
		filter.Booking = booking.String()
	}
	if values.Has("source") {
		source := values.Get("source")
		if strings.TrimSpace(source) == "" || strings.ContainsRune(source, 0) {
			return ports.BookingAnswerBounceFilter{}, false
		}
		filter.Source = source
	}
	return filter, true
}

// serveBookingRequests 上列订舱册（票 demo-chain-gaps/48）：一次订舱一行，带读出的状态词、当前那一份答复与名下全部答复。
func serveBookingRequests(
	response http.ResponseWriter,
	request *http.Request,
	reader ReviewCatalogueReader,
	tenant domain.TenantID,
	limit int,
) {
	rows, err := reader.ListBookingRequests(request.Context(), tenant, limit)
	if err != nil {
		writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
		return
	}
	bodies := make([]bookingRequestRowBody, 0, len(rows))
	for _, row := range rows {
		answers := make([]bookingAnswerBody, 0, len(row.Answers))
		for _, answer := range row.Answers {
			answers = append(answers, bookingAnswerRowBody(row.Booking, answer))
		}
		body := bookingRequestRowBody{
			bookingRequestBody: bookingRequestBody{
				Booking:     row.Booking,
				Commission:  row.Commission,
				Schedule:    row.Schedule,
				Quantity:    row.Quantity,
				Unit:        row.Unit,
				RequestedAt: rfc3339(row.RequestedAt),
				CancelledAt: optionalInstant(row.CancelledAt),
				RecordedAt:  rfc3339(row.RecordedAt),
			},
			Standing: row.Standing,
			Answers:  answers,
		}
		if row.Current != nil {
			current := bookingAnswerRowBody(row.Booking, *row.Current)
			body.CurrentAnswer = &current
		}
		bodies = append(bodies, body)
	}
	writeJSON(response, http.StatusOK, bookingRequestListResponse{
		Outcome:  outcomeBookingRequestsListed,
		Bookings: bodies,
	})
}

func bookingAnswerRowBody(booking string, row ports.BookingAnswerCatalogueRow) bookingAnswerBody {
	return bookingAnswerBody{
		Booking:    booking,
		Answer:     row.Answer,
		Outcome:    row.Outcome,
		Quantity:   row.Quantity,
		Basis:      row.Basis,
		DecidedAt:  rfc3339(row.DecidedAt),
		RecordedAt: rfc3339(row.RecordedAt),
	}
}

// serveBookingAnswerBounces 上列承运答复挡回册（ADR-0239 决定二，票 demo-chain-gaps/167）：哪个答复方、对哪个订舱的哪一份答复、
// 凭什么挡回、首次与最近一次挡回时刻、次数。空册答 []。
func serveBookingAnswerBounces(
	response http.ResponseWriter,
	request *http.Request,
	reader ReviewCatalogueReader,
	tenant domain.TenantID,
	filter ports.BookingAnswerBounceFilter,
	limit int,
) {
	rows, err := reader.ListBookingAnswerBounces(request.Context(), tenant, filter, limit)
	if err != nil {
		writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
		return
	}
	bodies := make([]bookingAnswerBounceBody, 0, len(rows))
	for _, row := range rows {
		bodies = append(bodies, bookingAnswerBounceBody{
			Source:         row.Source,
			Booking:        row.Booking,
			Answer:         row.Answer,
			Basis:          row.Basis,
			FirstBouncedAt: rfc3339(row.FirstBouncedAt),
			LastBouncedAt:  rfc3339(row.LastBouncedAt),
			Count:          quantity(row.Count),
		})
	}
	writeJSON(response, http.StatusOK, bookingAnswerBounceListResponse{
		Outcome: outcomeBookingAnswerBouncesListed,
		Bounces: bodies,
	})
}

type bookingRequestListResponse struct {
	Outcome  string                  `json:"outcome"`
	Bookings []bookingRequestRowBody `json:"bookings"`
}

// bookingRequestRowBody 是订舱册上一行（票 demo-chain-gaps/48）。订舱各格与订舱提交口回显的那一份同一形状（bookingRequestBody，
// 申请量照那一口写成数），答复与承运答复口回显的同一形状（bookingAnswerBody）：同一件东西在命令答复与查阅页上不换写法。
//
// standing 是读出的状态词，词与读法见 ports.BookingStandingPendingConfirmation 那组常量；只在读口派生一处，页面照抄、不另算。
// currentAnswer 是当前那一份答复，没有答复时缺席；answers 是名下全部答复，按答复时刻先后，候补与它的后继都在，总在场、空数组即
// 还没有答复。cancelledAt 缺席即没有取消——取消不进 standing。
type bookingRequestRowBody struct {
	bookingRequestBody
	Standing      string              `json:"standing"`
	CurrentAnswer *bookingAnswerBody  `json:"currentAnswer,omitempty"`
	Answers       []bookingAnswerBody `json:"answers"`
}

type bookingAnswerBounceListResponse struct {
	Outcome string                    `json:"outcome"`
	Bounces []bookingAnswerBounceBody `json:"bounces"`
}

// bookingAnswerBounceBody 是挡回册上一行。basis 是 ports.BookingAnswerBounceBasis 的词；两个时刻是本上下文记下挡回的时刻，不是
// 答复方给的答复时刻；count 是十进制计数串。
type bookingAnswerBounceBody struct {
	Source         string `json:"source"`
	Booking        string `json:"booking"`
	Answer         string `json:"answer"`
	Basis          string `json:"basis"`
	FirstBouncedAt string `json:"firstBouncedAt"`
	LastBouncedAt  string `json:"lastBouncedAt"`
	Count          string `json:"count"`
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

// transportScheduleBody 逐字段透出一个具体班次的一版（查阅列面与登记答复同一形状）。
//
// 方向是连接与起讫三格，各自透出，不拼成一个「A → B」串——组合是呈现的事。物化来的班次带周期安排、物化时那一版
// 与运行日期；临时班次这三键缺席。没有执行准备键、也没有实际执行键——那两组在存储上还没有登记格，不代填「未
// 出发」一类派生状态词；缺席说的是「无处可登」，代填会把它演成「已登而未发生」。
type transportScheduleBody struct {
	ScheduleID      string `json:"scheduleId"`
	Version         int    `json:"version"`
	Connection      string `json:"connection"`
	FromNode        string `json:"fromNode"`
	ToNode          string `json:"toNode"`
	DepartsAt       string `json:"departsAt"`
	ArrivesAt       string `json:"arrivesAt"`
	ServiceScope    string `json:"serviceScope"`
	Origin          string `json:"origin"`
	Template        string `json:"template,omitempty"`
	TemplateVersion int    `json:"templateVersion,omitempty"`
	RunningDate     string `json:"runningDate,omitempty"`
	RecordedAt      string `json:"recordedAt"`
}

type scheduleTemplateListResponse struct {
	Outcome   string                 `json:"outcome"`
	Templates []scheduleTemplateBody `json:"templates"`
}

// scheduleTemplateBody 逐字段透出一份周期安排的一版（查阅列面与登记答复同一形状）。出发时刻是当地 HH:MM，时区
// 随行给出——周期安排没有绝对出发时刻，那要按日期物化才有；effectiveUntil 缺席即不设终点。capacities 总在场，
// 空数组即这份周期安排不声明容量、物化出的班次不带池。
type scheduleTemplateBody struct {
	Template           string                    `json:"template"`
	Version            int                       `json:"version"`
	Connection         string                    `json:"connection"`
	FromNode           string                    `json:"fromNode"`
	ToNode             string                    `json:"toNode"`
	Weekdays           []string                  `json:"weekdays"`
	DepartureLocalTime string                    `json:"departureLocalTime"`
	DurationMinutes    int64                     `json:"durationMinutes"`
	TimeZone           string                    `json:"timeZone"`
	ServiceScope       string                    `json:"serviceScope"`
	EffectiveFrom      string                    `json:"effectiveFrom"`
	EffectiveUntil     string                    `json:"effectiveUntil,omitempty"`
	Capacities         []capacityDeclarationBody `json:"capacities"`
	RecordedAt         string                    `json:"recordedAt"`
}

// capacityDeclarationBody 是周期安排上一条逐维容量声明。硬上限与池容量一样按十进制串透出。
type capacityDeclarationBody struct {
	Dimension string `json:"dimension"`
	Unit      string `json:"unit"`
	Capacity  string `json:"capacity"`
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
	Dimension  string `json:"dimension"`
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
// receivedBy 两个参与方引用表达，组合成「A → B」是呈现的事。releasingNode / receivingNode /
// onwardNode 是登记方声明的三个节点（ADR-0213），没声明的那格缺席——缺席是「没有声明」，
// 不是读不出。gateReading 是`已交接`那一版随记的交出门禁读数整格（ADR-0213 决定四），拒收与待确认上缺席。
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
	ReleasingNode   string `json:"releasingNode,omitempty"`
	ReceivingNode   string `json:"receivingNode,omitempty"`
	OnwardNode      string `json:"onwardNode,omitempty"`
	GateReading     string `json:"gateReading,omitempty"`
}

type effectiveDeliveryListResponse struct {
	Outcome    string                  `json:"outcome"`
	Deliveries []effectiveDeliveryBody `json:"deliveries"`
}

// effectiveDeliveryBody 逐字段透出一行当前版有效交付结果。
//
// proof 是交付证明引用不是证据内容——证据构成属交付证明本体，列面只指名，不把
// 「已签收」演成一个可直接修改的状态。corrects 两键在场即这行是更正版本：更正翻旧
// 插新，本册只列当前版，历史版本按键与版本走详情读口。gateReading 是这一版随记的交付门禁读数整格（ADR-0213
// 决定五），每一版都有。
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
	GateReading     string `json:"gateReading"`
}

type carrierMasterDocumentListResponse struct {
	Outcome         string                      `json:"outcome"`
	MasterDocuments []carrierMasterDocumentBody `json:"masterDocuments"`
}

// carrierMasterDocumentBody 逐字段透出一行总单版本（ADR-0113）。
//
// standing 是三值封闭词（IN_FORCE / REVOKED / SUPERSEDED），原样透出不折并；supersedes 与
// changedAt 成对缺席表示首版——一行一版本，撤销、替代、关联重述都是新行回指前版，原
// 版本在册面上继续可见；replacedBy 只在已替代上在场。commission 与 booking 是可缺引用，
// 缺席即不带键。associationCount 是关联子表的行数（十进制计数串，理由同容量四量）——
// 关联本体不在列面展开，哪些集运单元、包裹、段列入了这一版按键与版本走登记册读口。
type carrierMasterDocumentBody struct {
	Document         string `json:"document"`
	Version          string `json:"version"`
	Issuer           string `json:"issuer"`
	Scope            string `json:"scope"`
	Commission       string `json:"commission,omitempty"`
	Booking          string `json:"booking,omitempty"`
	Standing         string `json:"standing"`
	Supersedes       string `json:"supersedes,omitempty"`
	ReplacedBy       string `json:"replacedBy,omitempty"`
	AssociationCount string `json:"associationCount"`
	ChangedAt        string `json:"changedAt,omitempty"`
	RecordedAt       string `json:"recordedAt"`
}

// serveDeliveryDispatchPendings 上列派送待形成的那几件（ADR-0230 决定三）：哪个段里的哪个对象、凭哪条入场依据、缺哪几件、首次
// 与最近一次答复时刻、最近一次问的时刻。只列待形成的；了结与作罢的不上列——这一页是要人去催所有者的清单，不是派送的历史。
func serveDeliveryDispatchPendings(
	response http.ResponseWriter,
	request *http.Request,
	reader ReviewCatalogueReader,
	tenant domain.TenantID,
	limit int,
) {
	rows, err := reader.ListDeliveryDispatchPendings(request.Context(), tenant, limit)
	if err != nil {
		writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
		return
	}
	bodies := make([]deliveryDispatchPendingBody, 0, len(rows))
	for _, row := range rows {
		bodies = append(bodies, deliveryDispatchPendingBody{
			Segment:         row.Key.Segment.String(),
			Object:          row.Key.Object.String(),
			EntryBasis:      row.EntryBasis.String(),
			Missing:         append([]string{}, row.Missing...),
			FirstAnsweredAt: rfc3339(row.FirstAnsweredAt),
			LastAnsweredAt:  rfc3339(row.LastAnsweredAt),
			LastAskedAt:     rfc3339(row.LastAskedAt),
		})
	}
	writeJSON(response, http.StatusOK, deliveryDispatchPendingListResponse{
		Outcome:  outcomeDeliveryDispatchPendingsListed,
		Pendings: bodies,
	})
}

// serveReaskCadences 上列一个租户登过的各版重问间隔（ADR-0230 决定五），最近登记的在前、第一行生效。空册答 []：那个租户的到期重问
// 答未配置。
func serveReaskCadences(
	response http.ResponseWriter,
	request *http.Request,
	reader ReviewCatalogueReader,
	tenant domain.TenantID,
	limit int,
) {
	rows, err := reader.ListDeliveryDispatchReaskCadences(request.Context(), tenant, limit)
	if err != nil {
		writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
		return
	}
	bodies := make([]reaskCadenceBody, 0, len(rows))
	for _, row := range rows {
		bodies = append(bodies, reaskCadenceBody{
			Version:         row.Cadence.Version().String(),
			IntervalMinutes: int64(row.Cadence.Interval().Minutes()),
			Basis:           row.Cadence.Basis(),
			RecordedAt:      rfc3339(row.RecordedAt),
		})
	}
	writeJSON(response, http.StatusOK, reaskCadenceListResponse{Outcome: outcomeReaskCadencesListed, Cadences: bodies})
}

// serveActualFulfillmentUnformed 上列实际履约未形成册里还未形成的那几件（ADR-0231 决定六）：哪一段的哪个对象、在哪份委托下按第几版答的、
// 为什么没形成、问的是哪一版协议、参与哪一刻结束、首次与最近一次答复时刻。空册答 []。这是要人去补的清单：续办各格不同——未配置与收尾
// 两格是租户发一版接得住的协议再给委托换版，缺采购上下文是给委托换版、记下版本，缺陷是先修好它再让补形成再问一次。
func serveActualFulfillmentUnformed(
	response http.ResponseWriter,
	request *http.Request,
	reader ReviewCatalogueReader,
	tenant domain.TenantID,
	limit int,
) {
	rows, err := reader.ListActualFulfillmentUnformed(request.Context(), tenant, limit)
	if err != nil {
		writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
		return
	}
	bodies := make([]actualFulfillmentUnformedBody, 0, len(rows))
	for _, row := range rows {
		entry := row.Entry
		bodies = append(bodies, actualFulfillmentUnformedBody{
			Segment:            entry.Key.Segment.String(),
			Object:             entry.Key.Object.String(),
			Commission:         entry.Commission.String(),
			CommissionRevision: entry.Revision,
			Reason:             entry.Reason.String(),
			AgreementObject:    entry.Agreement.Object(),
			AgreementVersion:   entry.Agreement.Version(),
			Defect:             entry.Defect,
			EndedAt:            rfc3339(entry.EndedAt),
			FirstAnsweredAt:    rfc3339(row.FirstAnsweredAt),
			LastAnsweredAt:     rfc3339(row.LastAnsweredAt),
		})
	}
	writeJSON(response, http.StatusOK, actualFulfillmentUnformedListResponse{
		Outcome:  outcomeActualFulfillmentUnformedListed,
		Unformed: bodies,
	})
}

type actualFulfillmentUnformedListResponse struct {
	Outcome  string                          `json:"outcome"`
	Unformed []actualFulfillmentUnformedBody `json:"unformed"`
}

// actualFulfillmentUnformedBody 是册上一件。endedAt 是参与终点的业务时间，两个答复时刻是登记时刻；agreementObject 与 agreementVersion 是
// 那一次问 PC 的那一版，缺采购上下文那一格没问、两键不出现。defect 只随 FORMATION_DEFECT，是类别码连同完整错误原文；那一格停在登记
// 答复之前，commissionRevision 与协议两键随之不出现。
type actualFulfillmentUnformedBody struct {
	Segment            string `json:"segment"`
	Object             string `json:"object"`
	Commission         string `json:"commission"`
	CommissionRevision int    `json:"commissionRevision,omitempty"`
	Reason             string `json:"reason"`
	AgreementObject    string `json:"agreementObject,omitempty"`
	AgreementVersion   string `json:"agreementVersion,omitempty"`
	Defect             string `json:"defect,omitempty"`
	EndedAt            string `json:"endedAt"`
	FirstAnsweredAt    string `json:"firstAnsweredAt"`
	LastAnsweredAt     string `json:"lastAnsweredAt"`
}

type reaskCadenceListResponse struct {
	Outcome  string             `json:"outcome"`
	Cadences []reaskCadenceBody `json:"cadences"`
}

type reaskCadenceBody struct {
	Version         string `json:"version"`
	IntervalMinutes int64  `json:"intervalMinutes"`
	Basis           string `json:"basis"`
	RecordedAt      string `json:"recordedAt"`
}

type deliveryDispatchPendingListResponse struct {
	Outcome  string                        `json:"outcome"`
	Pendings []deliveryDispatchPendingBody `json:"pendings"`
}

type deliveryDispatchPendingBody struct {
	Segment         string   `json:"segment"`
	Object          string   `json:"object"`
	EntryBasis      string   `json:"entryBasis"`
	Missing         []string `json:"missing"`
	FirstAnsweredAt string   `json:"firstAnsweredAt"`
	LastAnsweredAt  string   `json:"lastAnsweredAt"`
	LastAskedAt     string   `json:"lastAskedAt"`
}

type transportPreparationListResponse struct {
	Outcome      string                     `json:"outcome"`
	Preparations []transportPreparationBody `json:"preparations"`
}

// transportPreparationBody 逐字段透出一份运输准备结果（ADR-0201 决定七）。
//
// outcome 是领域那一组封闭词（NOT_CONFIGURED、OPPORTUNITY_PREPARED 等），原样透出；selectedSchedule 只在已准备上在场。候选与需求量照判断当时写下的转写：可用量是
// 那一刻的，不是此刻的；候选的 version 为 0 即那一班只在评估时物化、没有登记，后续订舱或装载要先登它。需求量缺了哪一维，
// 那一维就是待补充——不代填零。
type transportPreparationBody struct {
	PlannedLeg   string                     `json:"plannedLeg"`
	Object       string                     `json:"object"`
	Rule         string                     `json:"rule"`
	Outcome      string                     `json:"outcome"`
	Connection   string                     `json:"connection"`
	FromNode     string                     `json:"fromNode"`
	ToNode       string                     `json:"toNode"`
	WindowStart  string                     `json:"windowStart"`
	WindowEnd    string                     `json:"windowEnd"`
	DemandSource string                     `json:"demandSource"`
	DemandAnchor string                     `json:"demandAnchor,omitempty"`
	Demand       []preparationDemandBody    `json:"demand"`
	Selected     string                     `json:"selectedSchedule,omitempty"`
	Candidates   []preparationCandidateBody `json:"candidates"`
	JudgedAt     string                     `json:"judgedAt"`
	RecordedAt   string                     `json:"recordedAt"`
}

type preparationDemandBody struct {
	Dimension string `json:"dimension"`
	Amount    string `json:"amount"`
	Unit      string `json:"unit"`
}

type preparationCandidateBody struct {
	Schedule  string                `json:"schedule"`
	Version   int                   `json:"version"`
	DepartsAt string                `json:"departsAt"`
	ArrivesAt string                `json:"arrivesAt"`
	Verdict   string                `json:"verdict"`
	Pools     []preparationPoolBody `json:"pools"`
}

// preparationPoolBody 是一个池在一个候选上的依据。available 按十进制串透出，理由同容量四量；requested 是换成池单位的
// 需求量，换不成时缺席。
type preparationPoolBody struct {
	Pool      string `json:"pool"`
	Dimension string `json:"dimension"`
	Unit      string `json:"unit"`
	Available string `json:"available"`
	Requested string `json:"requested,omitempty"`
	Verdict   string `json:"verdict"`
}
