package warehousehttp

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/adapters/registrationjson"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/application"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
)

// 本文件是仓测尺寸重量的作业事实写口与运营/货主读口（UC-WF-002；ADR-0384）。
// 写口挂作业事实登记（能力词 WORK_FACT_REGISTRATION，事实类型 WAREHOUSE_MEASUREMENT，范围种类 WAREHOUSE）；
// 运营读口挂主数据与运营查阅读并带设备与操作者；货主读口挂 WAREHOUSE_READ 且不带设备与操作者。

// WarehouseMeasurementAuthenticator 铸仓测登记的作业事实信封。范围种类与引用先在 Intake 核过。
type WarehouseMeasurementAuthenticator interface {
	AuthenticateWarehouseMeasurement(
		ctx context.Context,
		bearerToken, deviceID, scopeKind, scopeReference string,
	) (domain.TenantID, domain.MeasurementOperatorID, domain.MeasurementDeviceID, error)
}

// WarehouseMeasurementIntake 是仓测登记口的 Intake。
type WarehouseMeasurementIntake interface {
	IntakeWarehouseMeasurement(ctx context.Context, request *http.Request) (application.RecordWarehouseMeasurementCommand, error)
}

// WarehouseMeasurementRecorder 是仓测登记转交的编排。事务边界在编排侧给出。
type WarehouseMeasurementRecorder interface {
	Record(ctx context.Context, command application.RecordWarehouseMeasurementCommand) (application.RecordWarehouseMeasurementResult, error)
}

var _ WarehouseMeasurementRecorder = (*application.RecordWarehouseMeasurementHandler)(nil)

// WarehouseMeasurementReader 查商品包装层级与在库一箱的仓测各版和当前那一版。
type WarehouseMeasurementReader interface {
	GoodsLevelMeasurements(
		ctx context.Context,
		tenant domain.TenantID,
		owner domain.OwnerAccountID,
		goods domain.GoodsCode,
		level domain.GoodsUnitName,
	) (application.GoodsLevelMeasurementReading, error)
	CartonMeasurements(
		ctx context.Context,
		tenant domain.TenantID,
		owner domain.OwnerAccountID,
		warehouse domain.WarehouseCode,
		carton domain.CartonNumber,
	) (application.CartonMeasurementReading, error)
}

var _ WarehouseMeasurementReader = (*application.QueryWarehouseMeasurementsHandler)(nil)

// WarehouseMeasurementWorkFactIntake 读作业事实请求头与载荷，认证后把租户、操作者与设备填进命令。
type WarehouseMeasurementWorkFactIntake struct {
	authenticator WarehouseMeasurementAuthenticator
}

func NewWarehouseMeasurementWorkFactIntake(authenticator WarehouseMeasurementAuthenticator) (*WarehouseMeasurementWorkFactIntake, error) {
	if authenticator == nil {
		return nil, errors.New("warehouse fulfillment http: warehouse measurement intake needs an authenticator")
	}
	return &WarehouseMeasurementWorkFactIntake{authenticator: authenticator}, nil
}

var _ WarehouseMeasurementIntake = (*WarehouseMeasurementWorkFactIntake)(nil)

func (intake *WarehouseMeasurementWorkFactIntake) IntakeWarehouseMeasurement(
	ctx context.Context,
	request *http.Request,
) (application.RecordWarehouseMeasurementCommand, error) {
	var none application.RecordWarehouseMeasurementCommand
	device, kind, reference, err := warehouseWorkScope(request)
	if err != nil {
		return none, err
	}
	body := request.Body
	if body == nil {
		body = http.NoBody
	}
	command, err := registrationjson.WarehouseMeasurementCommand(body)
	if err != nil {
		var malformed *registrationjson.MalformedDocumentError
		if errors.As(err, &malformed) {
			return none, &malformedRequest{reason: malformed.Reason}
		}
		return none, err
	}
	// 作业范围引用必须等于载荷里的仓，不读册：免得对不上的请求探出设备登没登。
	if reference != command.Warehouse.String() {
		return none, &malformedRequest{reason: httpapi.HeaderWorkScopeReference + " must equal warehouse"}
	}
	tenant, operator, recordedDevice, err := intake.authenticator.AuthenticateWarehouseMeasurement(
		ctx, httpapi.BearerToken(request), device, kind, reference,
	)
	if err != nil {
		return none, err
	}
	command.Tenant = tenant
	command.Operator = operator
	command.Device = recordedDevice
	return command, nil
}

// UnconfiguredWarehouseMeasurementIntake 在作业事实渠道未配时答接入渠道未配置，不读载荷。
type UnconfiguredWarehouseMeasurementIntake struct{}

func (UnconfiguredWarehouseMeasurementIntake) IntakeWarehouseMeasurement(context.Context, *http.Request) (application.RecordWarehouseMeasurementCommand, error) {
	return application.RecordWarehouseMeasurementCommand{}, ErrAccessChannelNotConfigured
}

var _ WarehouseMeasurementIntake = UnconfiguredWarehouseMeasurementIntake{}

type recordedMeasurementEchoBody struct {
	FactID         string              `json:"factId"`
	WarehouseCode  string              `json:"warehouseCode"`
	TargetKind     string              `json:"targetKind"`
	OwnerAccount   string              `json:"ownerAccount"`
	GoodsCode      string              `json:"goodsCode,omitempty"`
	PackagingLevel string              `json:"packagingLevel,omitempty"`
	CartonNumber   string              `json:"cartonNumber,omitempty"`
	Device         string              `json:"device"`
	Operator       string              `json:"operator"`
	OccurredAt     string              `json:"occurredAt"`
	DeviceSequence int64               `json:"deviceSequence"`
	ReceivedAt     string              `json:"receivedAt"`
	Dimensions     goodsDimensionsBody `json:"dimensions"`
	Weight         goodsWeightBody     `json:"weight"`
}

type recordWarehouseMeasurementAnswer struct {
	Outcome     string                       `json:"outcome"`
	Reason      string                       `json:"reason,omitempty"`
	Fields      []string                     `json:"fields,omitempty"`
	Measurement *recordedMeasurementEchoBody `json:"measurement,omitempty"`
}

// NewRecordWarehouseMeasurementEndpoint 交回仓测登记的 HTTP 入口（POST /warehouse-fulfillment-warehouse-measurements）。
// 新建答 201，其余业务结果答 200，问题体用作业事实族的问题详情。
func NewRecordWarehouseMeasurementEndpoint(intake WarehouseMeasurementIntake, recorder WarehouseMeasurementRecorder) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			response.Header().Set("Allow", http.MethodPost)
			httpapi.WriteProblem(response, request, http.StatusMethodNotAllowed, codeMethodNotAllowed)
			return
		}
		command, err := intake.IntakeWarehouseMeasurement(request.Context(), request)
		if err != nil {
			writeReceiptProblem(response, request, err)
			return
		}
		result, err := recorder.Record(request.Context(), command)
		if err != nil {
			slog.ErrorContext(request.Context(), "warehouse measurement did not form an answer",
				"warehouse", command.Warehouse.String(), "factId", command.FactID.String())
			httpapi.WriteProblem(response, request, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		writeWarehouseMeasurementAnswer(response, request, result)
	})
}

func writeWarehouseMeasurementAnswer(
	response http.ResponseWriter,
	request *http.Request,
	result application.RecordWarehouseMeasurementResult,
) {
	name := result.Outcome().String()
	if name == "" {
		httpapi.WriteProblem(response, request, http.StatusInternalServerError, codeUnnamedOutcome)
		return
	}
	answer := recordWarehouseMeasurementAnswer{Outcome: name}
	status := http.StatusOK
	switch result.Outcome() {
	case application.RecordMeasurementRecorded, application.RecordMeasurementExistingResult:
		recorded, has := result.Measurement()
		if !has {
			httpapi.WriteProblem(response, request, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		echo := recordedMeasurementEchoOf(recorded)
		answer.Measurement = &echo
		if result.Outcome() == application.RecordMeasurementRecorded {
			status = http.StatusCreated
		}
	case application.RecordMeasurementWarehouseNotAccepting:
		reason, has := result.NotAcceptingReason()
		if !has || reason.String() == "" {
			httpapi.WriteProblem(response, request, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		answer.Reason = reason.String()
	case application.RecordMeasurementNotAccepted:
		invalid := result.InvalidFields()
		answer.Fields = make([]string, 0, len(invalid))
		for _, field := range invalid {
			answer.Fields = append(answer.Fields, field.String())
		}
	}
	writeJSON(response, status, answer)
}

func recordedMeasurementEchoOf(item domain.WarehouseMeasurement) recordedMeasurementEchoBody {
	target := item.Target()
	dims := item.Dimensions()
	weight := item.Weight()
	body := recordedMeasurementEchoBody{
		FactID:         item.FactID().String(),
		WarehouseCode:  item.Warehouse().String(),
		TargetKind:     target.Kind().String(),
		OwnerAccount:   target.Owner().String(),
		Device:         item.Device().String(),
		Operator:       item.Operator().String(),
		OccurredAt:     rfc3339(item.OccurredAt()),
		DeviceSequence: item.DeviceSequence(),
		ReceivedAt:     rfc3339(item.ReceivedAt()),
		Dimensions: goodsDimensionsBody{
			Length: dims.Length().String(), Width: dims.Width().String(), Height: dims.Height().String(), Unit: dims.Unit().String(),
		},
		Weight: goodsWeightBody{Value: weight.Value().String(), Unit: weight.Unit().String()},
	}
	switch target.Kind() {
	case domain.MeasurementTargetGoodsLevel:
		body.GoodsCode = target.Goods().String()
		body.PackagingLevel = target.Level().String()
	case domain.MeasurementTargetCarton:
		body.CartonNumber = target.Carton().String()
	}
	return body
}

const (
	outcomeGoodsMeasurementsRead  = "GOODS_MEASUREMENTS_READ"
	outcomeCartonMeasurementsRead = "CARTON_MEASUREMENTS_READ"
	packagingLevelQueryKey        = "packagingLevel"
	cartonNumberQueryKey          = "cartonNumber"
)

type goodsMeasurementsResponse struct {
	Outcome        string                     `json:"outcome"`
	OwnerAccount   string                     `json:"ownerAccount"`
	GoodsCode      string                     `json:"goodsCode"`
	PackagingLevel string                     `json:"packagingLevel"`
	Found          bool                       `json:"found"`
	Measured       bool                       `json:"measured"`
	Current        *warehouseMeasurementBody  `json:"current,omitempty"`
	Declared       *declaredReferenceBody     `json:"declared,omitempty"`
	Versions       []warehouseMeasurementBody `json:"versions"`
}

type customerGoodsMeasurementsResponse struct {
	Outcome        string                             `json:"outcome"`
	OwnerAccount   string                             `json:"ownerAccount"`
	GoodsCode      string                             `json:"goodsCode"`
	PackagingLevel string                             `json:"packagingLevel"`
	Found          bool                               `json:"found"`
	Measured       bool                               `json:"measured"`
	Current        *customerWarehouseMeasurementBody  `json:"current,omitempty"`
	Declared       *declaredReferenceBody             `json:"declared,omitempty"`
	Versions       []customerWarehouseMeasurementBody `json:"versions"`
}

type cartonMeasurementsResponse struct {
	Outcome       string                     `json:"outcome"`
	OwnerAccount  string                     `json:"ownerAccount"`
	WarehouseCode string                     `json:"warehouseCode"`
	CartonNumber  string                     `json:"cartonNumber"`
	Measured      bool                       `json:"measured"`
	Current       *warehouseMeasurementBody  `json:"current,omitempty"`
	Versions      []warehouseMeasurementBody `json:"versions"`
}

type customerCartonMeasurementsResponse struct {
	Outcome       string                             `json:"outcome"`
	OwnerAccount  string                             `json:"ownerAccount"`
	WarehouseCode string                             `json:"warehouseCode"`
	CartonNumber  string                             `json:"cartonNumber"`
	Measured      bool                               `json:"measured"`
	Current       *customerWarehouseMeasurementBody  `json:"current,omitempty"`
	Versions      []customerWarehouseMeasurementBody `json:"versions"`
}

// NewQueryGoodsMeasurementsEndpoint 交回运营查阅商品某一包装层级仓测各版的入口
// （GET /warehouse-fulfillment-goods-measurements?ownerAccount=…&goodsCode=…&packagingLevel=…）。
func NewQueryGoodsMeasurementsEndpoint(intake CatalogueIntake, reader WarehouseMeasurementReader) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if refuseMethod(response, request, http.MethodGet) {
			return
		}
		query, err := intake.IntakeCatalogueQuery(request.Context(), request)
		if err != nil {
			writeIntakeProblem(response, err)
			return
		}
		owner, goods, level, reason := operatorGoodsLevelQuery(request)
		if reason != "" {
			writeMalformed(response, reason)
			return
		}
		reading, err := reader.GoodsLevelMeasurements(request.Context(), query.Tenant, owner, goods, level)
		if err != nil {
			slog.ErrorContext(request.Context(), "goods measurements read did not form an answer",
				"ownerAccount", owner.String(), "goodsCode", goods.String(), "packagingLevel", level.String())
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		answer := goodsMeasurementsResponse{
			Outcome:        outcomeGoodsMeasurementsRead,
			OwnerAccount:   owner.String(),
			GoodsCode:      goods.String(),
			PackagingLevel: level.String(),
			Found:          reading.Found,
			Measured:       reading.Measured,
			Declared:       declaredReferenceBodyOf(reading.Declared),
			Versions:       warehouseMeasurementBodiesOfRows(reading.Versions),
		}
		if reading.Current != nil {
			cur := warehouseMeasurementBodyOfRow(*reading.Current)
			answer.Current = &cur
		}
		writeJSON(response, http.StatusOK, answer)
	})
}

// NewQueryCartonMeasurementsEndpoint 交回运营查阅一箱仓测各版的入口
// （GET /warehouse-fulfillment-carton-measurements?ownerAccount=…&warehouseCode=…&cartonNumber=…）。
func NewQueryCartonMeasurementsEndpoint(intake CatalogueIntake, reader WarehouseMeasurementReader) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if refuseMethod(response, request, http.MethodGet) {
			return
		}
		query, err := intake.IntakeCatalogueQuery(request.Context(), request)
		if err != nil {
			writeIntakeProblem(response, err)
			return
		}
		owner, warehouse, carton, reason := operatorCartonMeasurementQuery(request)
		if reason != "" {
			writeMalformed(response, reason)
			return
		}
		reading, err := reader.CartonMeasurements(request.Context(), query.Tenant, owner, warehouse, carton)
		if err != nil {
			slog.ErrorContext(request.Context(), "carton measurements read did not form an answer",
				"ownerAccount", owner.String(), "warehouseCode", warehouse.String(), "cartonNumber", carton.String())
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		answer := cartonMeasurementsResponse{
			Outcome:       outcomeCartonMeasurementsRead,
			OwnerAccount:  owner.String(),
			WarehouseCode: warehouse.String(),
			CartonNumber:  carton.String(),
			Measured:      reading.Measured,
			Versions:      warehouseMeasurementBodiesOfRows(reading.Versions),
		}
		if reading.Current != nil {
			cur := warehouseMeasurementBodyOfRow(*reading.Current)
			answer.Current = &cur
		}
		writeJSON(response, http.StatusOK, answer)
	})
}

// NewGetCustomerGoodsMeasurementsEndpoint 交回货主查阅商品某一包装层级仓测各版的入口
// （GET /v1/warehouse/reads/goods-measurements?goodsCode=…&packagingLevel=…）。不带设备与操作者。
func NewGetCustomerGoodsMeasurementsEndpoint(intake CustomerGoodsPorts, reader WarehouseMeasurementReader) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if refuseCustomerMethod(response, request, http.MethodGet) {
			return
		}
		identity, err := intake.IntakeWarehouseRead(request.Context(), request)
		if err != nil {
			answerCustomerIntake(response, request, err)
			return
		}
		goods, level, err := customerGoodsLevelQuery(request)
		if err != nil {
			writeCustomerQueryProblem(response, request, err)
			return
		}
		reading, err := reader.GoodsLevelMeasurements(request.Context(), identity.Tenant, identity.Owner, goods, level)
		if err != nil {
			slog.ErrorContext(request.Context(), "customer goods measurements read did not form an answer",
				"ownerAccount", identity.Owner.String(), "goodsCode", goods.String(), "packagingLevel", level.String())
			writeCustomerProblem(response, request, http.StatusInternalServerError, codeNoAnswerFormed, "")
			return
		}
		answer := customerGoodsMeasurementsResponse{
			Outcome:        outcomeGoodsMeasurementsRead,
			OwnerAccount:   identity.Owner.String(),
			GoodsCode:      goods.String(),
			PackagingLevel: level.String(),
			Found:          reading.Found,
			Measured:       reading.Measured,
			Declared:       declaredReferenceBodyOf(reading.Declared),
			Versions:       customerWarehouseMeasurementBodiesOfRows(reading.Versions),
		}
		if reading.Current != nil {
			cur := customerWarehouseMeasurementBodyOfRow(*reading.Current)
			answer.Current = &cur
		}
		writeJSON(response, http.StatusOK, answer)
	})
}

// NewGetCustomerCartonMeasurementsEndpoint 交回货主查阅一箱仓测各版的入口
// （GET /v1/warehouse/reads/carton-measurements?warehouseCode=…&cartonNumber=…）。不带设备与操作者。
func NewGetCustomerCartonMeasurementsEndpoint(intake CustomerGoodsPorts, reader WarehouseMeasurementReader) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if refuseCustomerMethod(response, request, http.MethodGet) {
			return
		}
		identity, err := intake.IntakeWarehouseRead(request.Context(), request)
		if err != nil {
			answerCustomerIntake(response, request, err)
			return
		}
		warehouse, carton, err := customerCartonMeasurementQuery(request)
		if err != nil {
			writeCustomerQueryProblem(response, request, err)
			return
		}
		reading, err := reader.CartonMeasurements(request.Context(), identity.Tenant, identity.Owner, warehouse, carton)
		if err != nil {
			slog.ErrorContext(request.Context(), "customer carton measurements read did not form an answer",
				"ownerAccount", identity.Owner.String(), "warehouseCode", warehouse.String(), "cartonNumber", carton.String())
			writeCustomerProblem(response, request, http.StatusInternalServerError, codeNoAnswerFormed, "")
			return
		}
		answer := customerCartonMeasurementsResponse{
			Outcome:       outcomeCartonMeasurementsRead,
			OwnerAccount:  identity.Owner.String(),
			WarehouseCode: warehouse.String(),
			CartonNumber:  carton.String(),
			Measured:      reading.Measured,
			Versions:      customerWarehouseMeasurementBodiesOfRows(reading.Versions),
		}
		if reading.Current != nil {
			cur := customerWarehouseMeasurementBodyOfRow(*reading.Current)
			answer.Current = &cur
		}
		writeJSON(response, http.StatusOK, answer)
	})
}

func operatorGoodsLevelQuery(request *http.Request) (domain.OwnerAccountID, domain.GoodsCode, domain.GoodsUnitName, string) {
	cells, reason := queryCells(request, ownerAccountQueryKey, goodsCodeQueryKey, packagingLevelQueryKey)
	if reason != "" {
		return domain.OwnerAccountID{}, domain.GoodsCode{}, domain.GoodsUnitName{}, reason
	}
	owner, err := domain.NewOwnerAccountID(cells[0])
	if err != nil {
		return domain.OwnerAccountID{}, domain.GoodsCode{}, domain.GoodsUnitName{}, blankCellReason(ownerAccountQueryKey)
	}
	goods, err := domain.NewGoodsCode(cells[1])
	if err != nil {
		return domain.OwnerAccountID{}, domain.GoodsCode{}, domain.GoodsUnitName{}, blankCellReason(goodsCodeQueryKey)
	}
	level, err := domain.NewGoodsUnitName(cells[2])
	if err != nil {
		return domain.OwnerAccountID{}, domain.GoodsCode{}, domain.GoodsUnitName{}, blankCellReason(packagingLevelQueryKey)
	}
	return owner, goods, level, ""
}

func operatorCartonMeasurementQuery(request *http.Request) (domain.OwnerAccountID, domain.WarehouseCode, domain.CartonNumber, string) {
	cells, reason := queryCells(request, ownerAccountQueryKey, warehouseCodeQueryKey, cartonNumberQueryKey)
	if reason != "" {
		return domain.OwnerAccountID{}, domain.WarehouseCode{}, domain.CartonNumber{}, reason
	}
	owner, err := domain.NewOwnerAccountID(cells[0])
	if err != nil {
		return domain.OwnerAccountID{}, domain.WarehouseCode{}, domain.CartonNumber{}, blankCellReason(ownerAccountQueryKey)
	}
	warehouse, err := domain.NewWarehouseCode(cells[1])
	if err != nil {
		return domain.OwnerAccountID{}, domain.WarehouseCode{}, domain.CartonNumber{}, blankCellReason(warehouseCodeQueryKey)
	}
	carton, err := domain.NewCartonNumber(cells[2])
	if err != nil {
		return domain.OwnerAccountID{}, domain.WarehouseCode{}, domain.CartonNumber{}, blankCellReason(cartonNumberQueryKey)
	}
	return owner, warehouse, carton, ""
}

func customerGoodsLevelQuery(request *http.Request) (domain.GoodsCode, domain.GoodsUnitName, error) {
	if reason := refuseCarriedQueryIdentity(request); reason != "" {
		return domain.GoodsCode{}, domain.GoodsUnitName{}, &malformedRequest{reason: reason}
	}
	cells, reason := queryCells(request, goodsCodeQueryKey, packagingLevelQueryKey)
	if reason != "" {
		return domain.GoodsCode{}, domain.GoodsUnitName{}, &malformedRequest{reason: reason}
	}
	goods, err := domain.NewGoodsCode(cells[0])
	if err != nil {
		return domain.GoodsCode{}, domain.GoodsUnitName{}, &malformedRequest{reason: blankCellReason(goodsCodeQueryKey)}
	}
	level, err := domain.NewGoodsUnitName(cells[1])
	if err != nil {
		return domain.GoodsCode{}, domain.GoodsUnitName{}, &malformedRequest{reason: blankCellReason(packagingLevelQueryKey)}
	}
	return goods, level, nil
}

func customerCartonMeasurementQuery(request *http.Request) (domain.WarehouseCode, domain.CartonNumber, error) {
	if reason := refuseCarriedQueryIdentity(request); reason != "" {
		return domain.WarehouseCode{}, domain.CartonNumber{}, &malformedRequest{reason: reason}
	}
	cells, reason := queryCells(request, warehouseCodeQueryKey, cartonNumberQueryKey)
	if reason != "" {
		return domain.WarehouseCode{}, domain.CartonNumber{}, &malformedRequest{reason: reason}
	}
	warehouse, err := domain.NewWarehouseCode(cells[0])
	if err != nil {
		return domain.WarehouseCode{}, domain.CartonNumber{}, &malformedRequest{reason: blankCellReason(warehouseCodeQueryKey)}
	}
	carton, err := domain.NewCartonNumber(cells[1])
	if err != nil {
		return domain.WarehouseCode{}, domain.CartonNumber{}, &malformedRequest{reason: blankCellReason(cartonNumberQueryKey)}
	}
	return warehouse, carton, nil
}
