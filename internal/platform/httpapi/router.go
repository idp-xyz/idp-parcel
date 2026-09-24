// Package httpapi 只拥有 Parcel 各二进制共享的传输层 HTTP 路由。业务端点不落在这里，
// 按 ADR-0018 归各上下文的 adapters/http；本包只提供把它们挂上路由的形状。
package httpapi

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"go.idp.xyz/idp-parcel/internal/platform/buildinfo"
)

// BusinessEndpoint 是一个待挂载的业务端点。处理器由各上下文的 adapters/http 构造
// （含各自的 Intake 与应用编排），这里只收成品——路由层不参与任何业务翻译。
//
// 刻意没有 Method 字段：方法约束由各处理器自守——七个处理器都带自己的 405 分支
// （JSON problem 体加 Allow 头）并有单测钉住。路由层再按方法设卡，chi 会用自己的
// 纯文本 405 先答，处理器那一支永远不触发，单测钉住的形状与装配后的运行时形状
// 就分了家。
type BusinessEndpoint struct {
	Pattern string
	Handler http.Handler
}

// New 返回进程级端点的传输层路由。
func New(info buildinfo.Info) http.Handler {
	return NewWithEndpoints(info, nil)
}

// NewWithEndpoints 在进程级端点之外挂载业务端点。空清单合法——那正是接线闸门关着
// 时的形状：装配缝已开，逐端点等各自那一族的真 Intake 就位（客户渠道 PAR-INT-01、操作者渠道 ADR-0100、集成客户端族 ADR-0149）。
func NewWithEndpoints(info buildinfo.Info, endpoints []BusinessEndpoint) http.Handler {
	router := chi.NewRouter()
	router.Use(middleware.RequestID)
	router.Use(middleware.Recoverer)

	router.Get("/healthz", func(response http.ResponseWriter, _ *http.Request) {
		writeJSON(response, http.StatusOK, map[string]string{"status": "ok"})
	})
	router.Get("/version", func(response http.ResponseWriter, _ *http.Request) {
		response.Header().Set("Cache-Control", "no-store")
		writeJSON(response, http.StatusOK, info)
	})

	for _, endpoint := range endpoints {
		router.Handle(endpoint.Pattern, endpoint.Handler)
	}

	return router
}

func writeJSON(response http.ResponseWriter, status int, value any) {
	response.Header().Set("Content-Type", "application/json")
	response.WriteHeader(status)
	_ = json.NewEncoder(response).Encode(value)
}
