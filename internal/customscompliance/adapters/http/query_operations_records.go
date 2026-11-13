package customshttp

import (
	"context"
	"net/http"
	"net/url"
	"time"

	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
)

// 关务运营查阅的四本记录读面（演示票 demo-2026-10-30/01）：案件、提交版本、外部监管结果、动作准入判断册。
// 都是操作者查阅面——租户只取自认证信封，请求自报的租户一概不采信；只读，不触发判断、决定或披露
// （ADR-0077 Decision 一）。读不回是答案未形成（5xx），不伪装成空册；空册如实答空数组走 2xx。
//
// 四口各立入口、不合成一个 `?registry=` 分派：它们是 UC-CC-001/005/006/013 四个不同层次的对象，
// 分层保存的外部结果不能被一个端点的参数集串成「一个状态」。技术回执、监管接收、业务受理、放行
// 逐层原样上列，响应形里没有任何跨层折出的「清关成功」之类的字段。
//
// 查询参数按各册声明（ports.CustomsCasePages 等）解码，在 Intake 之后（ADR-0144 决定六；未配置的渠道对一切请求同答 403，
// ADR-0055 决定三）；答复带 page。
const (
	outcomeCustomsCasesListed        = "CUSTOMS_CASES_LISTED"
	outcomeSubmissionVersionsListed  = "SUBMISSION_VERSIONS_LISTED"
	outcomeExternalResultsListed     = "EXTERNAL_RESULTS_LISTED"
	outcomeAdmissionJudgementsListed = "ACTION_ADMISSION_JUDGEMENTS_LISTED"
)

// CustomsCaseRecordReader、SubmissionRecordReader、ExternalResultViewReader、
// ActionAdmissionJudgementReader 是各端点消费的读口，编译期锁缝与端口保持一致。
type CustomsCaseRecordReader interface {
	ListCustomsCases(
		ctx context.Context, tenant domain.TenantID, limit int, query cataloguepage.Query,
	) (cataloguepage.Result[ports.CustomsCaseRecord], error)
}

type SubmissionRecordReader interface {
	ListSubmissionVersions(
		ctx context.Context, tenant domain.TenantID, limit int, query cataloguepage.Query,
	) (cataloguepage.Result[ports.SubmissionVersionRecord], error)
}

type ExternalResultViewReader interface {
	ListExternalResults(
		ctx context.Context, tenant domain.TenantID, version string, limit int, query cataloguepage.Query,
	) (cataloguepage.Result[ports.ExternalResultViewRecord], error)
}

type ActionAdmissionJudgementReader interface {
	ListActionAdmissionJudgements(
		ctx context.Context, tenant domain.TenantID, limit int, query cataloguepage.Query,
	) (cataloguepage.Result[ports.ActionAdmissionJudgementRecord], error)
}

var (
	_ CustomsCaseRecordReader        = ports.CustomsCaseRecordRead(nil)
	_ SubmissionRecordReader         = ports.SubmissionRecordRead(nil)
	_ ExternalResultViewReader       = ports.ExternalResultViewRead(nil)
	_ ActionAdmissionJudgementReader = ports.ActionAdmissionJudgementRead(nil)
)

// serveRecordList 收拢四口共有的外壳：方法、Intake、按本册声明解码查询参数、读、成形。read 拿到请求与解好的那一份
// 查询参数，选择器（外部结果的 version）从这份取值里读，不另解一遍。
func serveRecordList[Record any, Body any](
	intake CatalogueQueryIntake,
	catalogue *cataloguepage.Catalogue,
	outcome string,
	key string,
	read func(
		request *http.Request, values url.Values, tenant domain.TenantID, limit int, query cataloguepage.Query,
	) (cataloguepage.Result[Record], error),
	transcribe func(Record) Body,
) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet {
			response.Header().Set("Allow", http.MethodGet)
			writeProblem(response, http.StatusMethodNotAllowed, codeMethodNotAllowed)
			return
		}
		query, err := intake.IntakeCatalogueQuery(request.Context(), request)
		if err != nil {
			writeCatalogueIntakeProblem(response, err)
			return
		}
		values, err := cataloguepage.ParseQuery(request.URL.RawQuery)
		if err != nil {
			writePageQueryProblem(response, err)
			return
		}
		pageQuery, err := decodeRecordQuery(catalogue, values)
		if err != nil {
			writePageQueryProblem(response, err)
			return
		}
		page, err := read(request, values, query.Scope.Tenant(), query.Limit, pageQuery)
		if err != nil {
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		bodies := make([]Body, 0, len(page.Rows))
		for _, record := range page.Rows {
			bodies = append(bodies, transcribe(record))
		}
		writeJSON(response, http.StatusOK, map[string]any{
			"outcome": outcome, key: bodies, "page": page.Page(query.Limit),
		})
	})
}

// blankNarrowing 是迁移前就在用的窄口参数：那时空值即不收窄、落到整册，这一点迁移后照旧——解码前去掉它的空值，
// 而不是答「筛选维的取值为空」。只对这几个键做；别的筛选维给空值仍是坏请求（ADR-0144 决定四）。
var blankNarrowing = map[*cataloguepage.Catalogue]string{
	ports.CustomsCasePages:        "case",
	ports.SubmissionVersionPages:  "version",
	ports.AdmissionJudgementPages: "object",
}

// decodeRecordQuery 去掉本册窄口参数上的空值，再按本册声明解码。
func decodeRecordQuery(catalogue *cataloguepage.Catalogue, values url.Values) (cataloguepage.Query, error) {
	if key, narrowing := blankNarrowing[catalogue]; narrowing && values.Has(key) {
		kept := make([]string, 0, len(values[key]))
		for _, value := range values[key] {
			if value != "" {
				kept = append(kept, value)
			}
		}
		if len(kept) == 0 {
			values.Del(key)
		} else {
			values[key] = kept
		}
	}
	return catalogue.Decode(values)
}

func instant(at time.Time) string { return at.UTC().Format(time.RFC3339Nano) }

// instantIfSet 把零值时间转成缺席：未越过发送边界的尝试没有发送时间，不能显成 0001 年。
func instantIfSet(at time.Time) string {
	if at.IsZero() {
		return ""
	}
	return instant(at)
}

// —— 案件 ——

// customsCaseBody 的 documentNumber 与 documentNumberAbsence 恰有其一：没有单据号的案件不带 documentNumber 这一格，
// 不答空串、不拿 case 顶，另由 documentNumberAbsence 说是哪一种情形（ADR-0355 决定十一）。
type customsCaseBody struct {
	Case                  string                `json:"case"`
	DocumentNumber        string                `json:"documentNumber,omitempty"`
	DocumentNumberAbsence string                `json:"documentNumberAbsence,omitempty"`
	Jurisdiction          string                `json:"jurisdiction"`
	Direction             string                `json:"direction"`
	Procedure             string                `json:"procedure"`
	Obligation            string                `json:"obligation"`
	EstablishedAt         string                `json:"establishedAt"`
	Parcels               []caseParcelBody      `json:"parcels"`
	Roles                 []caseRoleBody        `json:"roles"`
	Units                 []declarationUnitBody `json:"units"`
	Closure               *caseClosureBody      `json:"closure,omitempty"`
}

type caseParcelBody struct {
	Parcel    string `json:"parcel"`
	Customer  string `json:"customer"`
	SourceRef string `json:"sourceRef"`
}

type caseRoleBody struct {
	Role      string `json:"role"`
	Party     string `json:"party"`
	Authority string `json:"authority"`
}

type declarationUnitBody struct {
	Unit         string   `json:"unit"`
	Procedure    string   `json:"procedure"`
	Members      []string `json:"members"`
	FormedAt     string   `json:"formedAt"`
	ReplacesUnit string   `json:"replacesUnit,omitempty"`
}

// caseClosureBody 只在案件已有关闭记录时在场；缺席即未关闭，不是「关闭状态未知」。
type caseClosureBody struct {
	DecidedBy   string `json:"decidedBy"`
	CutoffAt    string `json:"cutoffAt"`
	VerifiedAt  string `json:"verifiedAt"`
	ClosedAt    string `json:"closedAt"`
	ItemCount   int    `json:"itemCount"`
	ReopenCount int    `json:"reopenCount"`
}

// NewQueryCustomsCasesEndpoint 交回关务案件及其申报单元查阅的 HTTP 入口
// （GET /customs-case-records；?case= 单份读回是 case 筛选维，见 ports.CustomsCasePages）。
func NewQueryCustomsCasesEndpoint(intake CatalogueQueryIntake, reader CustomsCaseRecordReader) http.Handler {
	return serveRecordList(intake, ports.CustomsCasePages, outcomeCustomsCasesListed, "cases",
		func(
			request *http.Request, _ url.Values, tenant domain.TenantID, limit int, query cataloguepage.Query,
		) (cataloguepage.Result[ports.CustomsCaseRecord], error) {
			return reader.ListCustomsCases(request.Context(), tenant, limit, query)
		},
		func(record ports.CustomsCaseRecord) customsCaseBody {
			body := customsCaseBody{
				Case: record.Case, DocumentNumber: record.DocumentNumber, DocumentNumberAbsence: record.DocumentNumberAbsence,
				Jurisdiction: record.Jurisdiction, Direction: record.Direction,
				Procedure: record.Procedure, Obligation: record.Obligation, EstablishedAt: instant(record.EstablishedAt),
				Parcels: make([]caseParcelBody, 0, len(record.Parcels)),
				Roles:   make([]caseRoleBody, 0, len(record.Roles)),
				Units:   make([]declarationUnitBody, 0, len(record.Units)),
			}
			for _, parcel := range record.Parcels {
				body.Parcels = append(body.Parcels, caseParcelBody(parcel))
			}
			for _, role := range record.Roles {
				body.Roles = append(body.Roles, caseRoleBody(role))
			}
			for _, unit := range record.Units {
				members := unit.Members
				if members == nil {
					members = []string{}
				}
				body.Units = append(body.Units, declarationUnitBody{
					Unit: unit.Unit, Procedure: unit.Procedure, Members: members,
					FormedAt: instant(unit.FormedAt), ReplacesUnit: unit.ReplacesUnit,
				})
			}
			if closure := record.Closure; closure != nil {
				body.Closure = &caseClosureBody{
					DecidedBy: closure.DecidedBy, CutoffAt: instant(closure.CutoffAt), VerifiedAt: instant(closure.VerifiedAt),
					ClosedAt: instant(closure.ClosedAt), ItemCount: closure.ItemCount, ReopenCount: closure.ReopenCount,
				}
			}
			return body
		})
}

// —— 提交版本 ——

type submissionVersionBody struct {
	Version        string              `json:"version"`
	Unit           string              `json:"unit"`
	Procedure      string              `json:"procedure"`
	ContentDigest  string              `json:"contentDigest"`
	Members        []string            `json:"members"`
	Dossier        string              `json:"dossier"`
	Roles          string              `json:"roles"`
	ReadinessBasis string              `json:"readinessBasis"`
	Authority      string              `json:"authority"`
	FixedAt        string              `json:"fixedAt"`
	IsCurrent      bool                `json:"isCurrent"`
	CorrectedFrom  string              `json:"correctedFrom,omitempty"`
	Attempts       []submissionAttempt `json:"attempts"`
}

// submissionAttempt 的 sendFact 与 receipts 各是各的一层：尝试登记、发送事实、技术回执三件事
// 分开上列，技术判断（由这几份折出）不在查阅面算——折叠是领域的判断语义。
type submissionAttempt struct {
	Sequence   int           `json:"sequence"`
	Target     string        `json:"target"`
	Result     string        `json:"result"`
	SafeResend string        `json:"safeResend,omitempty"`
	SentAt     string        `json:"sentAt,omitempty"`
	SendFact   *sendFactBody `json:"sendFact,omitempty"`
	Receipts   []receiptBody `json:"receipts"`
}

// sendFactBody 的 manualLodgement 只在这份发送事实是人工递交登记形成的时在场；缺席即发送口发出去的。
type sendFactBody struct {
	SentAt          string               `json:"sentAt"`
	Outcome         string               `json:"outcome"`
	ChannelMessage  string               `json:"channelMessage,omitempty"`
	ResponseRef     string               `json:"responseRef,omitempty"`
	ManualLodgement *manualLodgementBody `json:"manualLodgement,omitempty"`
}

type manualLodgementBody struct {
	LodgedBy   string `json:"lodgedBy"`
	Original   string `json:"original"`
	Reference  string `json:"reference,omitempty"`
	RecordedAt string `json:"recordedAt"`
}

// manualEntryBody 是人工录入的留痕：谁录的、凭哪份原件。整键缺席即接口送来的。
type manualEntryBody struct {
	EnteredBy string `json:"enteredBy"`
	Original  string `json:"original"`
}

type receiptBody struct {
	Source     string `json:"source"`
	Outcome    string `json:"outcome"`
	OccurredAt string `json:"occurredAt"`
	ReceivedAt string `json:"receivedAt"`
}

// NewQuerySubmissionVersionsEndpoint 交回提交版本、提交尝试、发送事实与技术回执查阅的 HTTP 入口
// （GET /customs-submission-records；?version= 单份读回（ADR-0342）是 version 筛选维，见 ports.SubmissionVersionPages）。
func NewQuerySubmissionVersionsEndpoint(intake CatalogueQueryIntake, reader SubmissionRecordReader) http.Handler {
	return serveRecordList(intake, ports.SubmissionVersionPages, outcomeSubmissionVersionsListed, "submissions",
		func(
			request *http.Request, _ url.Values, tenant domain.TenantID, limit int, query cataloguepage.Query,
		) (cataloguepage.Result[ports.SubmissionVersionRecord], error) {
			return reader.ListSubmissionVersions(request.Context(), tenant, limit, query)
		},
		func(record ports.SubmissionVersionRecord) submissionVersionBody {
			members := record.Members
			if members == nil {
				members = []string{}
			}
			body := submissionVersionBody{
				Version: record.Version, Unit: record.Unit, Procedure: record.Procedure,
				ContentDigest: record.ContentDigest, Members: members, Dossier: record.Dossier, Roles: record.Roles,
				ReadinessBasis: record.ReadinessBasis, Authority: record.Authority, FixedAt: instant(record.FixedAt),
				IsCurrent: record.IsCurrent, CorrectedFrom: record.CorrectedFrom,
				Attempts: make([]submissionAttempt, 0, len(record.Attempts)),
			}
			for _, attempt := range record.Attempts {
				entry := submissionAttempt{
					Sequence: attempt.Sequence, Target: attempt.Target, Result: attempt.Result,
					SafeResend: attempt.SafeResend, SentAt: instantIfSet(attempt.SentAt),
					Receipts: make([]receiptBody, 0, len(attempt.Receipts)),
				}
				if fact := attempt.SendFact; fact != nil {
					entry.SendFact = &sendFactBody{
						SentAt: instant(fact.SentAt), Outcome: fact.Outcome,
						ChannelMessage: fact.ChannelMessage, ResponseRef: fact.ResponseRef,
					}
					if lodgement := fact.ManualLodgement; lodgement != nil {
						entry.SendFact.ManualLodgement = &manualLodgementBody{
							LodgedBy: lodgement.LodgedBy, Original: lodgement.Original,
							Reference: lodgement.Reference, RecordedAt: instant(lodgement.RecordedAt),
						}
					}
				}
				for _, receipt := range attempt.Receipts {
					entry.Receipts = append(entry.Receipts, receiptBody{
						Source: receipt.Source, Outcome: receipt.Outcome,
						OccurredAt: instant(receipt.OccurredAt), ReceivedAt: instant(receipt.ReceivedAt),
					})
				}
				body.Attempts = append(body.Attempts, entry)
			}
			return body
		})
}

// —— 外部监管结果与放行 ——

type externalResultBody struct {
	Source            string       `json:"source"`
	Layer             string       `json:"layer,omitempty"`
	Role              string       `json:"role,omitempty"`
	RawSemantics      string       `json:"rawSemantics,omitempty"`
	Rule              string       `json:"rule,omitempty"`
	SubmissionVersion string       `json:"submissionVersion,omitempty"`
	ClaimedVersion    string       `json:"claimedVersion,omitempty"`
	Scope             string       `json:"scope,omitempty"`
	Unattributable    bool         `json:"unattributable"`
	LayerConflict     bool         `json:"layerConflict"`
	CorrectsSource    string       `json:"correctsSource,omitempty"`
	CorrectionInForce bool         `json:"correctionInForce"`
	OccurredAt        string       `json:"occurredAt,omitempty"`
	ReceivedAt        string       `json:"receivedAt,omitempty"`
	RecordedAt        string       `json:"recordedAt"`
	Release           *releaseBody `json:"release,omitempty"`
	// ManualEntry 只在这条结果是人工录入的时在场。
	ManualEntry *manualEntryBody `json:"manualEntry,omitempty"`
}

// releaseBody 只在放行层结果上在场（UC-CC-006：放行层落成范围与条件明确的放行事实）。
type releaseBody struct {
	Kind      string `json:"kind"`
	Authority string `json:"authority"`
	Scope     string `json:"scope"`
	Condition string `json:"condition,omitempty"`
}

// NewQueryExternalResultsEndpoint 交回外部监管结果（含放行结果）查阅的 HTTP 入口
// （GET /customs-external-result-records，可选 ?version= 只列归属于该提交版本的；version 是选择器，见 ports.ExternalResultPages）。
//
// 归属不上任何提交版本的响应（unattributable）照样上列、只带原始语义——留存不猜，
// 不补造版本或层次；同层冲突以 layerConflict 标出，双方事实都在，不选边。
func NewQueryExternalResultsEndpoint(intake CatalogueQueryIntake, reader ExternalResultViewReader) http.Handler {
	return serveRecordList(intake, ports.ExternalResultPages, outcomeExternalResultsListed, "results",
		func(
			request *http.Request, values url.Values, tenant domain.TenantID, limit int, query cataloguepage.Query,
		) (cataloguepage.Result[ports.ExternalResultViewRecord], error) {
			return reader.ListExternalResults(request.Context(), tenant, values.Get("version"), limit, query)
		},
		func(record ports.ExternalResultViewRecord) externalResultBody {
			body := externalResultBody{
				Source: record.Source, Layer: record.Layer, Role: record.Role, RawSemantics: record.RawSemantics,
				Rule: record.Rule, SubmissionVersion: record.SubmissionVersion, ClaimedVersion: record.ClaimedVersion,
				Scope: record.Scope, Unattributable: record.Unattributable, LayerConflict: record.LayerConflict,
				CorrectsSource: record.CorrectsSource, CorrectionInForce: record.CorrectionInForce,
				OccurredAt: instantIfSet(record.OccurredAt), ReceivedAt: instantIfSet(record.ReceivedAt),
				RecordedAt: instant(record.RecordedAt),
			}
			if release := record.Release; release != nil {
				body.Release = &releaseBody{
					Kind: release.Kind, Authority: release.Authority, Scope: release.Scope, Condition: release.Condition,
				}
			}
			if record.EnteredBy != "" {
				body.ManualEntry = &manualEntryBody{EnteredBy: record.EnteredBy, Original: record.Original}
			}
			return body
		})
}

// —— 动作准入判断册 ——

type admissionJudgementBody struct {
	Judgement  string                 `json:"judgement"`
	Object     string                 `json:"object"`
	Intent     string                 `json:"intent"`
	Node       string                 `json:"node"`
	OnwardNode string                 `json:"onwardNode,omitempty"`
	ActionAt   string                 `json:"actionAt"`
	CrossZone  string                 `json:"crossZone"`
	Answer     string                 `json:"answer"`
	Zones      []admissionZoneBody    `json:"zones"`
	Findings   []admissionFindingBody `json:"findings"`
	JudgedAt   string                 `json:"judgedAt"`
}

type admissionZoneBody struct {
	Node       string `json:"node"`
	Zone       string `json:"zone,omitempty"`
	Registered bool   `json:"registered"`
}

type admissionFindingBody struct {
	Action string               `json:"action"`
	Answer string               `json:"answer"`
	Bases  []admissionBasisBody `json:"bases"`
}

type admissionBasisBody struct {
	Cause     string   `json:"cause"`
	Scope     string   `json:"scope,omitempty"`
	Procedure string   `json:"procedure,omitempty"`
	Sources   []string `json:"sources"`
}

// NewQueryActionAdmissionJudgementsEndpoint 交回动作准入判断册查阅的 HTTP 入口
// （GET /customs-action-admission-judgements，可选 ?object= 按包裹号精确查——判断册的对象是声明包裹，传申报单元号只会得到空册；
// object 是筛选维，见 ports.AdmissionJudgementPages）。
//
// 判断册是判断当时的记录：每行带当时的区域读数与逐动作答复及依据，旧行不改。查阅面不重判、
// 不拿「此刻」的登记册覆盖当时的答案——换了登记册之后同一请求的新判断是另一行。
func NewQueryActionAdmissionJudgementsEndpoint(intake CatalogueQueryIntake, reader ActionAdmissionJudgementReader) http.Handler {
	return serveRecordList(intake, ports.AdmissionJudgementPages, outcomeAdmissionJudgementsListed, "judgements",
		func(
			request *http.Request, _ url.Values, tenant domain.TenantID, limit int, query cataloguepage.Query,
		) (cataloguepage.Result[ports.ActionAdmissionJudgementRecord], error) {
			return reader.ListActionAdmissionJudgements(request.Context(), tenant, limit, query)
		},
		admissionJudgementBodyOf)
}

func admissionJudgementBodyOf(record ports.ActionAdmissionJudgementRecord) admissionJudgementBody {
	body := admissionJudgementBody{
		Judgement: record.Judgement, Object: record.Object, Intent: record.Intent, Node: record.Node,
		OnwardNode: record.OnwardNode, ActionAt: instant(record.ActionAt), CrossZone: record.CrossZone,
		Answer: record.Answer, JudgedAt: instant(record.JudgedAt),
		Zones:    make([]admissionZoneBody, 0, len(record.Zones)),
		Findings: make([]admissionFindingBody, 0, len(record.Findings)),
	}
	for _, zone := range record.Zones {
		body.Zones = append(body.Zones, admissionZoneBody(zone))
	}
	for _, finding := range record.Findings {
		entry := admissionFindingBody{
			Action: finding.Action, Answer: finding.Answer,
			Bases: make([]admissionBasisBody, 0, len(finding.Bases)),
		}
		for _, basis := range finding.Bases {
			sources := basis.Sources
			if sources == nil {
				sources = []string{}
			}
			entry.Bases = append(entry.Bases, admissionBasisBody{
				Cause: basis.Cause, Scope: basis.Scope, Procedure: basis.Procedure, Sources: sources,
			})
		}
		body.Findings = append(body.Findings, entry)
	}
	return body
}

// —— 卡关对象 ——

// UnadmittedObjectReader 是卡关对象查阅消费的读口。
type UnadmittedObjectReader interface {
	ListUnadmittedObjects(
		ctx context.Context, tenant domain.TenantID, limit int, query cataloguepage.Query,
	) (ports.UnadmittedObjects, error)
}

var _ UnadmittedObjectReader = ports.UnadmittedObjectRead(nil)

// 空清单仍是 LISTED：「此刻没有卡关的对象」是如实答案，件数 0 只在读口真答了 0 时出现。
const outcomeUnadmittedObjectsListed = "UNADMITTED_OBJECTS_LISTED"

// unadmittedObjectsResponse 的 unadmittedCount 是整个队列的积压，不随筛选与 q 变；page.total 才是这一组条件下的件数。
// truncated 即还有下一页（page.next 不为 null），与件数无关：带了筛选时件数本就大于这一组条件能列出的。
type unadmittedObjectsResponse struct {
	Outcome         string                   `json:"outcome"`
	UnadmittedCount int                      `json:"unadmittedCount"`
	Truncated       bool                     `json:"truncated"`
	Judgements      []admissionJudgementBody `json:"judgements"`
	Page            cataloguepage.Page       `json:"page"`
}

// NewQueryUnadmittedObjectsEndpoint 交回卡关对象查阅的 HTTP 入口（GET /customs-unadmitted-objects）：每个对象最晚
// 那一刻上有一份动作准入判断不让动作过门的，连同全部件数与是否截断。哪一格算「不过门」与「最近」怎么取，见
// ports.UnadmittedObjects。行形与判断册同一份，列表页按同一套列读它。
//
// 查询参数按 ports.UnadmittedObjectPages 解码，在 Intake 之后；按对象精确查归判断册那一口。
func NewQueryUnadmittedObjectsEndpoint(intake CatalogueQueryIntake, reader UnadmittedObjectReader) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet {
			response.Header().Set("Allow", http.MethodGet)
			writeProblem(response, http.StatusMethodNotAllowed, codeMethodNotAllowed)
			return
		}
		query, err := intake.IntakeCatalogueQuery(request.Context(), request)
		if err != nil {
			writeCatalogueIntakeProblem(response, err)
			return
		}
		pageQuery, err := ports.UnadmittedObjectPages.DecodeRaw(request.URL.RawQuery)
		if err != nil {
			writePageQueryProblem(response, err)
			return
		}
		listed, err := reader.ListUnadmittedObjects(request.Context(), query.Scope.Tenant(), query.Limit, pageQuery)
		if err != nil {
			// 读不回不伪装成 0 件：首页把 0 读成「没有卡关的」，卡住的包裹就更没人看见了。
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		bodies := make([]admissionJudgementBody, 0, len(listed.Rows))
		for _, record := range listed.Rows {
			bodies = append(bodies, admissionJudgementBodyOf(record))
		}
		writeJSON(response, http.StatusOK, unadmittedObjectsResponse{
			Outcome:         outcomeUnadmittedObjectsListed,
			UnadmittedCount: listed.Count,
			Truncated:       listed.Next != "",
			Judgements:      bodies,
			Page:            listed.Page(query.Limit),
		})
	})
}

// —— 外部监管舱单引用 ——

const outcomeExternalManifestReferencesListed = "EXTERNAL_MANIFEST_REFERENCES_LISTED"

// ExternalManifestReferenceReader 是舱单引用查阅端点消费的读口。
type ExternalManifestReferenceReader interface {
	ListExternalManifestReferences(
		ctx context.Context, tenant domain.TenantID, limit int, query cataloguepage.Query,
	) (cataloguepage.Result[ports.ExternalManifestReferenceRecord], error)
}

var _ ExternalManifestReferenceReader = ports.ExternalManifestReferenceRead(nil)

// externalManifestReferenceBody 逐字段透出一份舱单在册的那一版。priorVersion 首版缺席；associationUnit 尚未关联到申报单元即缺席；
// manualEntry 只在在册的这一版是人工录入的时在场，整键缺席即承运方系统声明的。
type externalManifestReferenceBody struct {
	Carrier         string           `json:"carrier"`
	Manifest        string           `json:"manifest"`
	Version         string           `json:"version"`
	Procedure       string           `json:"procedure"`
	Direction       string           `json:"direction"`
	Scope           string           `json:"scope"`
	SourceFact      string           `json:"sourceFact"`
	AcceptedAt      string           `json:"acceptedAt"`
	PriorVersion    string           `json:"priorVersion,omitempty"`
	AssociationUnit string           `json:"associationUnit,omitempty"`
	ManualEntry     *manualEntryBody `json:"manualEntry,omitempty"`
}

// NewQueryExternalManifestReferencesEndpoint 交回外部监管舱单引用查阅的 HTTP 入口
// （GET /customs-external-manifest-reference-records；收哪些键见 ports.ExternalManifestReferencePages）。
func NewQueryExternalManifestReferencesEndpoint(intake CatalogueQueryIntake, reader ExternalManifestReferenceReader) http.Handler {
	return serveRecordList(intake, ports.ExternalManifestReferencePages, outcomeExternalManifestReferencesListed, "manifests",
		func(
			request *http.Request, _ url.Values, tenant domain.TenantID, limit int, query cataloguepage.Query,
		) (cataloguepage.Result[ports.ExternalManifestReferenceRecord], error) {
			return reader.ListExternalManifestReferences(request.Context(), tenant, limit, query)
		},
		func(record ports.ExternalManifestReferenceRecord) externalManifestReferenceBody {
			body := externalManifestReferenceBody{
				Carrier: record.Carrier, Manifest: record.Manifest, Version: record.Version, Procedure: record.Procedure,
				Direction: record.Direction, Scope: record.Scope, SourceFact: record.SourceFact,
				AcceptedAt: instant(record.AcceptedAt), PriorVersion: record.PriorVersion, AssociationUnit: record.AssociationUnit,
			}
			if record.EnteredBy != "" {
				body.ManualEntry = &manualEntryBody{EnteredBy: record.EnteredBy, Original: record.Original}
			}
			return body
		})
}
