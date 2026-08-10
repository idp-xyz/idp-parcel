// Package httpapi 只拥有 Parcel 各二进制共享的传输层 HTTP 路由。业务端点不落在这里，
// 按 ADR-0018 归各上下文的 adapters/http。
package httpapi

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"go.idp.xyz/idp-parcel/internal/platform/buildinfo"
)

// New 返回进程级端点的传输层路由。
func New(info buildinfo.Info) http.Handler {
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

	return router
}

func writeJSON(response http.ResponseWriter, status int, value any) {
	response.Header().Set("Content-Type", "application/json")
	response.WriteHeader(status)
	_ = json.NewEncoder(response).Encode(value)
}
