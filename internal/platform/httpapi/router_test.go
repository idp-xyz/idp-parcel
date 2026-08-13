package httpapi_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.idp.xyz/idp-parcel/internal/platform/buildinfo"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
)

// Covers: 评审发现「405 双闸矛盾」的修法——业务端点全方法直通，方法约束由处理器
// 自守：装配后错误方法收到的必须是处理器自己的 405（JSON problem 体加 Allow 头），
// 不是 chi 的纯文本 405。路由层按方法设卡会让处理器那一支永远不触发，单测钉住的
// 形状与装配后的运行时形状分家。
func TestMethodDisciplineStaysWithTheHandlerAfterAssembly(t *testing.T) {
	handler := http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			response.Header().Set("Allow", http.MethodPost)
			response.Header().Set("Content-Type", "application/json")
			response.WriteHeader(http.StatusMethodNotAllowed)
			_ = json.NewEncoder(response).Encode(map[string]any{
				"error": map[string]string{"code": "METHOD_NOT_ALLOWED"},
			})
			return
		}
		response.WriteHeader(http.StatusCreated)
	})

	router := httpapi.NewWithEndpoints(buildinfo.Info{}, []httpapi.BusinessEndpoint{{
		Pattern: "/business/things",
		Handler: handler,
	}})

	get := httptest.NewRequest(http.MethodGet, "/business/things", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, get)

	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", response.Code)
	}
	if allow := response.Header().Get("Allow"); allow != http.MethodPost {
		t.Fatalf("Allow = %q; 405 不是处理器答的", allow)
	}
	if contentType := response.Header().Get("Content-Type"); contentType != "application/json" {
		t.Fatalf("Content-Type = %q; chi 的纯文本 405 劫走了答案", contentType)
	}

	post := httptest.NewRequest(http.MethodPost, "/business/things", nil)
	created := httptest.NewRecorder()
	router.ServeHTTP(created, post)
	if created.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201", created.Code)
	}
}

// TestProcessEndpointsStayAvailable 证进程级端点不被业务挂载影响。
func TestProcessEndpointsStayAvailable(t *testing.T) {
	router := httpapi.NewWithEndpoints(buildinfo.Info{}, []httpapi.BusinessEndpoint{{
		Pattern: "/business/things",
		Handler: http.NotFoundHandler(),
	}})

	health := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, health)
	if response.Code != http.StatusOK {
		t.Fatalf("healthz = %d", response.Code)
	}
}
