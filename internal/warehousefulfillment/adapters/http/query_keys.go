package warehousehttp

import (
	"errors"
	"net/http"
	"slices"
	"strings"

	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
)

// queryCells 从修订史口的查询串取各点名键的值：每个键恰给一次，除它们之外没有别的键，各册的修订史口共用。按 ParseQuery 解，
// 解不开的一段整条答 400，不静默丢掉后照常作答——一个写坏的编码被丢下就成了没给。取值原样交回、不去空白：编码逐字交给领域。
// 不成立时交回一句理由（只谈查询串的形状，不回显取值），成立时理由为空。
//
// 「取值是不是非空白、存不存得下」由各册用自己的构造器判，判不过的理由用 blankCellReason。
func queryCells(request *http.Request, keys ...string) (cells []string, reason string) {
	values, err := cataloguepage.ParseQuery(request.URL.RawQuery)
	if err != nil {
		var malformed *cataloguepage.MalformedQuery
		if errors.As(err, &malformed) {
			return nil, malformed.Reason
		}
		return nil, "the query string could not be read"
	}
	for key := range values {
		if !slices.Contains(keys, key) {
			return nil, onlyKeysReason(keys)
		}
	}
	cells = make([]string, len(keys))
	for index, key := range keys {
		given := values[key]
		if len(given) != 1 {
			return nil, key + " must be given exactly once"
		}
		cells[index] = given[0]
	}
	return cells, ""
}

func onlyKeysReason(keys []string) string {
	if len(keys) == 1 {
		return "only " + keys[0] + " is accepted as a query key"
	}
	return "only " + strings.Join(keys, ", ") + " are accepted as query keys"
}

func blankCellReason(key string) string {
	return key + " must be a non-blank value that can be stored verbatim"
}
