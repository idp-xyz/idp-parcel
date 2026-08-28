package customshttp

import (
	"context"
	"net/http"
	"time"

	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
)

// PortsPathsCatalogueReader 是本端点消费的读口：口岸目录与申报路径目录两本册子的
// 列表读面（票 admin-remainder-mechanism-batch/03）。查阅不触发判断、决定或披露——
// 接存储读面，不接应用编排，与 /customs-case-registers 同一条分界（ADR-0077
// Decision 一）。
type PortsPathsCatalogueReader interface {
	ListCandidatePorts(
		ctx context.Context,
		tenant domain.TenantID,
		limit int,
	) ([]ports.CandidatePortEntry, error)
	ListDeclarationPaths(
		ctx context.Context,
		tenant domain.TenantID,
		limit int,
	) ([]ports.DeclarationPathEntry, error)
}

// 编译期锁缝：读口形状与端口保持一致——本端点不新造查询语义。
var _ PortsPathsCatalogueReader = ports.PortsPathsCatalogueRead(nil)

// 业务结果的封闭集合：两本册子各占一格。空册如实答空列表走 2xx 成格，不折成未配置
// （ADR-0077 Decision 四）。
const (
	outcomeCandidatePortsListed   = "CANDIDATE_PORTS_LISTED"
	outcomeDeclarationPathsListed = "DECLARATION_PATHS_LISTED"
)

// registry 查询参数的封闭集（未知值坏请求），判据同 /customs-case-registers；取值
// 与受控 CLI 的两命令同词（candidate-port / declaration-path）。
const (
	registryCandidatePort   = "candidate-port"
	registryDeclarationPath = "declaration-path"
)

// NewQueryPortsPathsEndpoint 交回口岸目录与申报路径目录查阅的 HTTP 入口
// （GET /customs-ports-paths，票 admin-remainder-mechanism-batch/03）。
//
// 两本册子是「口岸与申报路径」一张页面的两签查阅面，共用一个端点按 `registry` 分派
// ——「一页里的页签」共入口、「各自独立的页」各立入口，分界照 /customs-case-registers
// 文件头那道裁决。合规候选区域不在本分派：区域维未建模（等自己的票），封闭集不为
// 它预留一个 501 格。
//
// 全部版本连同生效区间原样上列，**不下推评估时点参数**（裁决同关闭义务上列不下推
// 截点那条）：按时点解析版本是候选读取的判断输入，那是点读 PortsPathsView 伺候的
// 另一个调用面——查阅面收下时点参数，就等于让目录读口长出第二种「解析」语义。
func NewQueryPortsPathsEndpoint(
	intake CatalogueQueryIntake,
	reader PortsPathsCatalogueReader,
) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet {
			response.Header().Set("Allow", http.MethodGet)
			writeProblem(response, http.StatusMethodNotAllowed, codeMethodNotAllowed)
			return
		}

		registry := request.URL.Query().Get("registry")
		if registry != registryCandidatePort && registry != registryDeclarationPath {
			writeProblem(response, http.StatusBadRequest, codeMalformedRequest)
			return
		}

		query, err := intake.IntakeCatalogueQuery(request.Context(), request)
		if err != nil {
			writeCatalogueIntakeProblem(response, err)
			return
		}
		tenant := query.Scope.Tenant()

		switch registry {
		case registryCandidatePort:
			serveCandidatePorts(response, request, reader, tenant, query.Limit)
		case registryDeclarationPath:
			serveDeclarationPaths(response, request, reader, tenant, query.Limit)
		}
	})
}

func serveCandidatePorts(
	response http.ResponseWriter,
	request *http.Request,
	reader PortsPathsCatalogueReader,
	tenant domain.TenantID,
	limit int,
) {
	entries, err := reader.ListCandidatePorts(request.Context(), tenant, limit)
	if err != nil {
		// 读不回是答案未形成，不是「空册」——伪装成后者会让一次该重试的故障变成终局。
		writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
		return
	}
	// 空列表交回空数组而不是 null：调用方判「没有行」不该先判「有没有字段」。
	bodies := make([]candidatePortBody, 0, len(entries))
	for _, entry := range entries {
		body := candidatePortBody{
			Port:        entry.Port.String(),
			AppliesFrom: entry.AppliesFrom.UTC().Format(time.RFC3339Nano),
		}
		// 终点零值即尚无终点（开放版）：缺席是真话不是缺陷，不为区间完整补一个
		// 编造的「无限远」时刻。
		if !entry.AppliesUntil.IsZero() {
			body.AppliesUntil = entry.AppliesUntil.UTC().Format(time.RFC3339Nano)
		}
		bodies = append(bodies, body)
	}
	writeJSON(response, http.StatusOK, candidatePortListResponse{
		Outcome: outcomeCandidatePortsListed,
		Ports:   bodies,
	})
}

func serveDeclarationPaths(
	response http.ResponseWriter,
	request *http.Request,
	reader PortsPathsCatalogueReader,
	tenant domain.TenantID,
	limit int,
) {
	entries, err := reader.ListDeclarationPaths(request.Context(), tenant, limit)
	if err != nil {
		writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
		return
	}
	bodies := make([]declarationPathBody, 0, len(entries))
	for _, entry := range entries {
		body := declarationPathBody{
			Path:            entry.Path.String(),
			Port:            entry.Route.Port().String(),
			Direction:       entry.Route.Direction().String(),
			DeclarationMode: entry.Route.Mode().String(),
			AppliesFrom:     entry.AppliesFrom.UTC().Format(time.RFC3339Nano),
		}
		if !entry.AppliesUntil.IsZero() {
			body.AppliesUntil = entry.AppliesUntil.UTC().Format(time.RFC3339Nano)
		}
		bodies = append(bodies, body)
	}
	writeJSON(response, http.StatusOK, declarationPathListResponse{
		Outcome: outcomeDeclarationPathsListed,
		Paths:   bodies,
	})
}

type candidatePortListResponse struct {
	Outcome string              `json:"outcome"`
	Ports   []candidatePortBody `json:"ports"`
}

type declarationPathListResponse struct {
	Outcome string                `json:"outcome"`
	Paths   []declarationPathBody `json:"paths"`
}

// candidatePortBody 逐字段透出一版口岸合规候选：口岸标识与生效区间。目录事实只有
// 这两件——所属区域与适用性判断都不在册（0013 自注），传输层不为它们造空列。
type candidatePortBody struct {
	Port         string `json:"port"`
	AppliesFrom  string `json:"appliesFrom"`
	AppliesUntil string `json:"appliesUntil,omitempty"`
}

// declarationPathBody 逐字段透出一版申报路径：路径标识、三维路径事实（经哪个口岸、
// 按哪个方向、以哪种申报模式）与生效区间。port 是标识引用——「引用的口岸此刻是否
// 在册」是读者拿两册对照的判断，本行不代答（裁量记在票 03）。
type declarationPathBody struct {
	Path            string `json:"path"`
	Port            string `json:"port"`
	Direction       string `json:"direction"`
	DeclarationMode string `json:"declarationMode"`
	AppliesFrom     string `json:"appliesFrom"`
	AppliesUntil    string `json:"appliesUntil,omitempty"`
}
