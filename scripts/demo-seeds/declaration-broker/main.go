// demo-seeds/declaration-broker 是演示用的合成报关服务商，**仅限本机隔离环境**（票 product-strategy-boundary/10，ADR-0342
// 决定三）。它照产品随附 HTTP 连接器的线格式答：收 `POST /declarations`，按 Idempotency-Key 派生消息号，同一个键再到给同一个号，
// 答 `{"outcome":"ACCEPTED","messageId":…}`。消息号是 `SYN-MSG-` 加键的 sha256 前 16 位十六进制，演示脚本据此自己算出来去送
// 异步技术回执，不必读本程序的输出。
//
// 只听回环地址：通道登记只收回环上的明文 http，这里也不给绑到别处的口子。它不代表任何真实服务商的格式，证据只记 S。
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
)

func messageID(key string) string {
	sum := sha256.Sum256([]byte(key))
	return "SYN-MSG-" + strings.ToUpper(hex.EncodeToString(sum[:])[:16])
}

func handle(response http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost {
		response.Header().Set("Allow", http.MethodPost)
		response.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	key := strings.TrimSpace(request.Header.Get("Idempotency-Key"))
	var body struct {
		Version string `json:"version"`
	}
	raw, _ := io.ReadAll(io.LimitReader(request.Body, 1<<20))
	if key == "" || json.Unmarshal(raw, &body) != nil || body.Version == "" {
		response.Header().Set("Content-Type", "application/json")
		response.WriteHeader(http.StatusBadRequest)
		_, _ = response.Write([]byte(`{"outcome":"REJECTED"}`))
		return
	}
	id := messageID(key)
	log.Printf("收到申报 %s（键 %s），答 ACCEPTED %s", body.Version, key, id)
	response.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(response).Encode(map[string]string{"outcome": "ACCEPTED", "messageId": id})
}

func main() {
	addr := flag.String("addr", "127.0.0.1:19180", "监听地址，只收回环")
	flag.Parse()
	host, _, err := net.SplitHostPort(*addr)
	if ip := net.ParseIP(host); err != nil || ip == nil || !ip.IsLoopback() {
		fmt.Fprintf(os.Stderr, "declaration-broker: 只听回环地址，%q 不收\n", *addr)
		os.Exit(2)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/declarations", handle)
	log.Printf("合成报关服务商听 http://%s/declarations", *addr)
	log.Fatal(http.ListenAndServe(*addr, mux))
}
