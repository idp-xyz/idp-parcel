package warehousehttp

import (
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/application"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
)

// preAdviceBody 是读出的一份预报。单据号没有就省略，不答空串。依据引用没有就省略。
// 累计实收和逐次收货只在查阅口带上：写口回显不填这两格，omitempty 把空指针省掉。
// 没收过时查阅口仍给出按商品折成基本单位的 0，那是把已入账行加出来的。
type preAdviceBody struct {
	PreAdviceID        string                    `json:"preAdviceId"`
	OwnerAccount       string                    `json:"ownerAccount"`
	WarehouseCode      string                    `json:"warehouseCode"`
	ArrivalMode        string                    `json:"arrivalMode"`
	InboundMode        string                    `json:"inboundMode"`
	Status             string                    `json:"status"`
	DocumentNumber     *string                   `json:"documentNumber,omitempty"`
	NumberingAbsence   string                    `json:"numberingAbsence,omitempty"`
	SubmissionPath     string                    `json:"submissionPath"`
	EvidenceReference  string                    `json:"evidenceReference,omitempty"`
	Registrant         string                    `json:"registrant"`
	AcceptedAt         string                    `json:"acceptedAt"`
	GoodsLines         []preAdviceGoodsLineBody  `json:"goodsLines,omitempty"`
	Cartons            []preAdviceCartonBody     `json:"cartons,omitempty"`
	CurrentNotice      *preAdviceNoticeBody      `json:"currentNotice,omitempty"`
	Notices            []preAdviceNoticeBody     `json:"notices,omitempty"`
	CumulativeReceived *[]cumulativeReceivedBody `json:"cumulativeReceived,omitempty"`
	Receipts           *[]receiptBody            `json:"receipts,omitempty"`
	Closure            *preAdviceClosureBody     `json:"closure,omitempty"`
	UnclaimedArrival   bool                      `json:"unclaimedArrival,omitempty"`
}

type preAdviceGoodsLineBody struct {
	GoodsCode   string `json:"goodsCode"`
	Unit        string `json:"unit"`
	Quantity    int64  `json:"quantity"`
	BatchNumber string `json:"batchNumber,omitempty"`
	ExpiryDate  string `json:"expiryDate,omitempty"`
	CartonCount *int64 `json:"cartonCount,omitempty"`
}

type preAdviceCartonBody struct {
	CartonNumber string                    `json:"cartonNumber"`
	Lines        []preAdviceCartonLineBody `json:"lines"`
}

type preAdviceCartonLineBody struct {
	GoodsCode string `json:"goodsCode"`
	Quantity  int64  `json:"quantity"`
}

type preAdviceNoticeBody struct {
	Version           int    `json:"version"`
	ExpectedAt        string `json:"expectedAt"`
	ArrivalClue       string `json:"arrivalClue,omitempty"`
	OwnerReference    string `json:"ownerReference,omitempty"`
	SubmissionPath    string `json:"submissionPath"`
	EvidenceReference string `json:"evidenceReference,omitempty"`
	Registrant        string `json:"registrant"`
	RecordedAt        string `json:"recordedAt"`
}

type preAdviceIssueBody struct {
	Kind   string `json:"kind"`
	Line   int    `json:"line"`
	Carton int    `json:"carton"`
}

// preAdviceBodyOf 把一份预报译成读出的形状。withNotices 为假时只带现行告知，列表用这一格；单份查阅把全部版本按已有顺序带出。
func preAdviceBodyOf(advice domain.PreAdvice, withNotices bool) preAdviceBody {
	body := preAdviceBody{
		PreAdviceID:    advice.ID().String(),
		OwnerAccount:   advice.Owner().String(),
		WarehouseCode:  advice.Warehouse().String(),
		ArrivalMode:    advice.Arrival().String(),
		InboundMode:    advice.Inbound().String(),
		Status:         advice.Status().String(),
		SubmissionPath: advice.Submission().Path().String(),
		Registrant:     advice.Registrant().String(),
		AcceptedAt:     rfc3339(advice.AcceptedAt()),
	}
	if number, assigned := advice.DocumentNumber(); assigned {
		text := number.String()
		body.DocumentNumber = &text
	}
	if reason := advice.NumberingAbsence().String(); reason != "" {
		body.NumberingAbsence = reason
	}
	if evidence, given := advice.Submission().EvidenceReference(); given {
		body.EvidenceReference = evidence.String()
	}
	if advice.Inbound() == domain.InboundByCarton {
		body.Cartons = preAdviceCartonBodies(advice.Cartons())
	} else {
		body.GoodsLines = preAdviceGoodsLineBodies(advice.GoodsLines())
	}
	notices := advice.Notices()
	if len(notices) > 0 {
		current := preAdviceNoticeBodyOf(notices[len(notices)-1])
		body.CurrentNotice = &current
	}
	if withNotices {
		body.Notices = make([]preAdviceNoticeBody, 0, len(notices))
		for _, notice := range notices {
			body.Notices = append(body.Notices, preAdviceNoticeBodyOf(notice))
		}
	}
	return body
}

func operatorPreAdviceBody(reading application.PreAdviceReading, withNotices bool) preAdviceBody {
	body := preAdviceBodyOf(reading.Advice, withNotices)
	cumulative := cumulativeBodies(reading.Cumulative)
	receipts := receiptBodies(reading.Receipts, reading.Conclusions, true, true)
	body.CumulativeReceived = &cumulative
	body.Receipts = &receipts
	body.Closure = closureBodyOf(reading.Closure, reading.Conclusions, true, true)
	body.UnclaimedArrival = reading.UnclaimedArrival
	return body
}

func operatorPreAdviceBodies(readings []application.PreAdviceReading) []preAdviceBody {
	bodies := make([]preAdviceBody, 0, len(readings))
	for _, reading := range readings {
		bodies = append(bodies, operatorPreAdviceBody(reading, false))
	}
	return bodies
}

type cumulativeReceivedBody struct {
	GoodsCode    string `json:"goodsCode,omitempty"`
	CartonNumber string `json:"cartonNumber,omitempty"`
	Quantity     int64  `json:"quantity"`
}

// preAdviceClosureBody 是一份完结。没有少收时不带 shortages：不要为到齐的商品编一行 0。
// 货主口不填 Operator，omitempty 把它省掉。有少收时带差异标识和这份少收的结论。
type preAdviceClosureBody struct {
	ClosedAt                  string                      `json:"closedAt"`
	Operator                  string                      `json:"operator,omitempty"`
	Note                      string                      `json:"note,omitempty"`
	DiscrepancyID             string                      `json:"discrepancyId,omitempty"`
	AwaitingOwnerConfirmation bool                        `json:"awaitingOwnerConfirmation,omitempty"`
	Conclusion                *discrepancyConclusionBody  `json:"conclusion,omitempty"`
	SupersededConclusions     []discrepancyConclusionBody `json:"supersededConclusions,omitempty"`
	Shortages                 []shortageBody              `json:"shortages,omitempty"`
}

type shortageBody struct {
	GoodsCode    string `json:"goodsCode,omitempty"`
	CartonNumber string `json:"cartonNumber,omitempty"`
	Kind         string `json:"kind"`
	Quantity     int64  `json:"quantity"`
}

func closureBodyOf(
	closure *domain.PreAdviceClosure,
	conclusions []application.DiscrepancyConclusionView,
	withConclusions bool,
	withOperator bool,
) *preAdviceClosureBody {
	if closure == nil {
		return nil
	}
	body := &preAdviceClosureBody{ClosedAt: rfc3339(closure.ClosedAt())}
	if closure.Note().Given() {
		body.Note = closure.Note().String()
	}
	if withOperator {
		body.Operator = closure.Operator().String()
	}
	if id, has := closure.Discrepancy(); has {
		body.DiscrepancyID = id.String()
		if withConclusions {
			body.AwaitingOwnerConfirmation, body.Conclusion, body.SupersededConclusions = lookupConclusion(id.String(), conclusions, withOperator)
		}
	}
	lines := closure.Shortages()
	if len(lines) == 0 {
		return body
	}
	body.Shortages = make([]shortageBody, 0, len(lines))
	for _, line := range lines {
		item := shortageBody{Kind: line.Kind(), Quantity: line.Quantity()}
		if line.Goods().String() != "" {
			item.GoodsCode = line.Goods().String()
		}
		if line.Carton().String() != "" {
			item.CartonNumber = line.Carton().String()
		}
		body.Shortages = append(body.Shortages, item)
	}
	return body
}

type receiptBody struct {
	FactID                    string                      `json:"factId"`
	OccurredAt                string                      `json:"occurredAt"`
	Device                    string                      `json:"device,omitempty"`
	Operator                  string                      `json:"operator,omitempty"`
	Lines                     []receiptLineBody           `json:"lines"`
	DiscrepancyID             string                      `json:"discrepancyId,omitempty"`
	Discrepancies             []receiptDiscrepancyBody    `json:"discrepancies,omitempty"`
	AwaitingOwnerConfirmation bool                        `json:"awaitingOwnerConfirmation,omitempty"`
	Conclusion                *discrepancyConclusionBody  `json:"conclusion,omitempty"`
	SupersededConclusions     []discrepancyConclusionBody `json:"supersededConclusions,omitempty"`
}

type receiptLineBody struct {
	Observed              string `json:"observed,omitempty"`
	Unit                  string `json:"unit,omitempty"`
	StockState            string `json:"stockState,omitempty"`
	Quantity              int64  `json:"quantity,omitempty"`
	BatchNumber           string `json:"batchNumber,omitempty"`
	ExpiryDate            string `json:"expiryDate,omitempty"`
	LabelAlias            string `json:"labelAlias,omitempty"`
	LocationCode          string `json:"locationCode,omitempty"`
	GoodsCode             string `json:"goodsCode,omitempty"`
	CartonNumber          string `json:"cartonNumber,omitempty"`
	WarehouseCartonNumber string `json:"warehouseCartonNumber,omitempty"`
	Damaged               *bool  `json:"damaged,omitempty"`
	Result                string `json:"result,omitempty"`
	UnpostedReason        string `json:"unpostedReason,omitempty"`
}

// receiptDiscrepancyBody 只带类型与数量。结论挂在这次收货的差异标识上，不挂在行上。
type receiptDiscrepancyBody struct {
	Kind         string `json:"kind"`
	Quantity     int64  `json:"quantity"`
	GoodsCode    string `json:"goodsCode,omitempty"`
	Observed     string `json:"observed,omitempty"`
	CartonNumber string `json:"cartonNumber,omitempty"`
}

func cumulativeBodies(rows []application.CumulativeReceived) []cumulativeReceivedBody {
	bodies := make([]cumulativeReceivedBody, 0, len(rows))
	for _, row := range rows {
		body := cumulativeReceivedBody{Quantity: row.Quantity}
		if row.Goods.String() != "" {
			body.GoodsCode = row.Goods.String()
		}
		if row.Carton.String() != "" {
			body.CartonNumber = row.Carton.String()
		}
		bodies = append(bodies, body)
	}
	return bodies
}

func receiptBodies(
	receipts []domain.RecordedReceipt,
	conclusions []application.DiscrepancyConclusionView,
	withConclusions bool,
	withActors bool,
) []receiptBody {
	bodies := make([]receiptBody, 0, len(receipts))
	for _, receipt := range receipts {
		lines := receiptLineBodies(receipt.Lines())
		if receipt.Shape() == domain.InboundByCarton {
			lines = cartonReceiptLineBodies(receipt.CartonLines())
		}
		body := receiptBody{
			FactID: receipt.FactID().String(), OccurredAt: rfc3339(receipt.OccurredAt()),
			Lines: lines,
		}
		if withActors {
			body.Device = receipt.Device().String()
			body.Operator = receipt.Operator().String()
		}
		if id, has := receipt.Discrepancy(); has {
			body.DiscrepancyID = id.String()
			body.Discrepancies = receiptDiscrepancyBodies(receipt.Differences())
			if withConclusions {
				body.AwaitingOwnerConfirmation, body.Conclusion, body.SupersededConclusions = lookupConclusion(id.String(), conclusions, withActors)
			}
		}
		bodies = append(bodies, body)
	}
	return bodies
}

// discrepancyConclusionBody 是一条结论。货主口带途径；代录的另带依据。操作者只在运营查阅出现。
type discrepancyConclusionBody struct {
	Conclusion        string `json:"conclusion"`
	Note              string `json:"note,omitempty"`
	SubmissionPath    string `json:"submissionPath"`
	EvidenceReference string `json:"evidenceReference,omitempty"`
	Operator          string `json:"operator,omitempty"`
	RecordedAt        string `json:"recordedAt"`
}

func lookupConclusion(
	id string,
	views []application.DiscrepancyConclusionView,
	withOperator bool,
) (bool, *discrepancyConclusionBody, []discrepancyConclusionBody) {
	for _, view := range views {
		if view.ID().String() != id {
			continue
		}
		recorded, ok := view.Current()
		if !ok {
			return true, nil, nil
		}
		current := discrepancyConclusionBodyOf(recorded, withOperator)
		var superseded []discrepancyConclusionBody
		for _, old := range view.Superseded() {
			superseded = append(superseded, discrepancyConclusionBodyOf(old, withOperator))
		}
		return false, &current, superseded
	}
	return true, nil, nil
}

func discrepancyConclusionBodyOf(conclusion domain.DiscrepancyConclusion, withOperator bool) discrepancyConclusionBody {
	body := discrepancyConclusionBody{
		Conclusion: conclusion.Verdict().String(), SubmissionPath: conclusion.Path().String(),
		RecordedAt: rfc3339(conclusion.RecordedAt()),
	}
	if conclusion.Note().Given() {
		body.Note = conclusion.Note().String()
	}
	if conclusion.Path() == domain.SubmissionOnBehalf {
		body.EvidenceReference = conclusion.Evidence().String()
		if withOperator {
			body.Operator = conclusion.Operator().String()
		}
	}
	return body
}

func receiptLineBodies(lines []domain.ReceiptLine) []receiptLineBody {
	bodies := make([]receiptLineBody, 0, len(lines))
	for _, line := range lines {
		body := receiptLineBody{
			Observed: line.Observed(), Unit: line.Unit().String(), StockState: line.State().String(),
			Quantity: line.Quantity(), LocationCode: line.Location().String(),
			Result: line.Result().String(), UnpostedReason: line.Unposted().String(),
		}
		if line.Batch().Given() {
			body.BatchNumber = line.Batch().String()
		}
		if line.Expiry().Given() {
			body.ExpiryDate = line.Expiry().String()
		}
		if line.Alias().String() != "" {
			body.LabelAlias = line.Alias().String()
		}
		if line.Goods().String() != "" {
			body.GoodsCode = line.Goods().String()
		}
		bodies = append(bodies, body)
	}
	return bodies
}

func receiptDiscrepancyBodies(lines []domain.ReceiptDiscrepancyLine) []receiptDiscrepancyBody {
	bodies := make([]receiptDiscrepancyBody, 0, len(lines))
	for _, line := range lines {
		body := receiptDiscrepancyBody{Kind: line.Kind().String(), Quantity: line.Quantity()}
		if line.Goods().String() != "" {
			body.GoodsCode = line.Goods().String()
		}
		if line.Observed() != "" {
			body.Observed = line.Observed()
		}
		if line.Carton().String() != "" {
			body.CartonNumber = line.Carton().String()
		}
		bodies = append(bodies, body)
	}
	return bodies
}

func cartonReceiptLineBodies(lines []domain.CartonReceiptLine) []receiptLineBody {
	bodies := make([]receiptLineBody, 0, len(lines))
	for _, line := range lines {
		damaged := line.Damaged()
		body := receiptLineBody{
			CartonNumber: line.Number().String(), LocationCode: line.Location().String(),
			Damaged: &damaged, Result: line.Result().String(), UnpostedReason: line.Unposted().String(),
		}
		if line.WarehouseNumber().String() != "" {
			body.WarehouseCartonNumber = line.WarehouseNumber().String()
		}
		bodies = append(bodies, body)
	}
	return bodies
}

func preAdviceGoodsLineBodies(lines []domain.GoodsInboundLine) []preAdviceGoodsLineBody {
	bodies := make([]preAdviceGoodsLineBody, 0, len(lines))
	for _, line := range lines {
		body := preAdviceGoodsLineBody{
			GoodsCode: line.Goods().String(), Unit: line.Unit().String(), Quantity: line.Quantity(),
		}
		if line.Batch().Given() {
			body.BatchNumber = line.Batch().String()
		}
		if line.Expiry().Given() {
			body.ExpiryDate = line.Expiry().String()
		}
		if line.CartonCount().Given() {
			count := line.CartonCount().Value()
			body.CartonCount = &count
		}
		bodies = append(bodies, body)
	}
	return bodies
}

func preAdviceCartonBodies(cartons []domain.Carton) []preAdviceCartonBody {
	bodies := make([]preAdviceCartonBody, 0, len(cartons))
	for _, carton := range cartons {
		lines := carton.Lines()
		body := preAdviceCartonBody{CartonNumber: carton.Number().String(), Lines: make([]preAdviceCartonLineBody, 0, len(lines))}
		for _, line := range lines {
			body.Lines = append(body.Lines, preAdviceCartonLineBody{GoodsCode: line.Goods().String(), Quantity: line.Quantity()})
		}
		bodies = append(bodies, body)
	}
	return bodies
}

func preAdviceNoticeBodyOf(notice domain.PreAdviceNotice) preAdviceNoticeBody {
	body := preAdviceNoticeBody{
		Version:        notice.Version(),
		ExpectedAt:     rfc3339(notice.ExpectedAt()),
		SubmissionPath: notice.Submission().Path().String(),
		Registrant:     notice.Registrant().String(),
		RecordedAt:     rfc3339(notice.RecordedAt()),
	}
	if notice.Clue().Given() {
		body.ArrivalClue = notice.Clue().String()
	}
	if notice.Reference().Given() {
		body.OwnerReference = notice.Reference().String()
	}
	if evidence, given := notice.Submission().EvidenceReference(); given {
		body.EvidenceReference = evidence.String()
	}
	return body
}
