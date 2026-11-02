package docnumber

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"
)

// ScrambleSecretsFileEnv 是加扰密钥文件的实例配置项：值是一个文件路径，文件是一个 JSON 对象，键是租户标识、值是
// 该租户的加扰密钥，例如 `{"TENANT-A":"<至少 32 个字符>"}`。
//
// 取文件而不取环境变量本身：密钥按租户成表，挂载为部署平台的机密文件最自然，也不会随进程环境被一并打进诊断
// 输出。不设即没有任何租户配了密钥——用加扰的模板登记即拒（ADR-0355 决定五），不是「用一把默认密钥」。
// 设了而读不动或不合规，进程在开池之前带原因退出：半配的密钥表比没配更难查。
const ScrambleSecretsFileEnv = "IDP_PARCEL_DOCUMENT_NUMBER_SCRAMBLE_SECRETS_FILE"

// minScrambleSecretLength 是产品策略：密钥驱动平台的 Feistel 轮函数（keyed SHA-256），短到能被穷举时加扰就只剩
// 形式——拿到几个号的人可以离线试出密钥、反推业务量，而加扰要防的正是这个（ADR-0355 越权风险点 2）。
const minScrambleSecretLength = 32

// ErrScrambleSecretNotConfigured 表示模板用了加扰流水，而该租户没有配加扰密钥。
var ErrScrambleSecretNotConfigured = errors.New("document numbering: scramble secret is not configured for this tenant")

// ErrInvalidScrambleSecrets 表示实例配置里的加扰密钥表不合规。
var ErrInvalidScrambleSecrets = errors.New("document numbering: invalid scramble secrets")

// ScrambleSecrets 交回某租户的加扰密钥，没配答 false。
type ScrambleSecrets interface {
	ScrambleSecret(tenant string) (string, bool)
}

// SecretTable 是从实例配置读出的租户密钥表。零值即一个租户都没配。
type SecretTable map[string]string

var _ ScrambleSecrets = SecretTable(nil)

func (table SecretTable) ScrambleSecret(tenant string) (string, bool) {
	secret, found := table[tenant]
	return secret, found && secret != ""
}

// Tenants 是配了密钥的租户数，只给启动日志用：日志只说配没配、配了几个，不说是谁，更不说密钥。
func (table SecretTable) Tenants() int { return len(table) }

// LoadScrambleSecrets 按 ScrambleSecretsFileEnv 读密钥表。读取方式由调用方注入，本包不碰进程环境与文件系统。
// configured 为假表示没设这一项。
func LoadScrambleSecrets(getenv func(string) string, readFile func(string) ([]byte, error)) (table SecretTable, configured bool, err error) {
	path := strings.TrimSpace(getenv(ScrambleSecretsFileEnv))
	if path == "" {
		return SecretTable{}, false, nil
	}
	raw, err := readFile(path)
	if err != nil {
		return nil, true, fmt.Errorf("%w: read %s: %v", ErrInvalidScrambleSecrets, ScrambleSecretsFileEnv, err)
	}
	table, err = ParseScrambleSecrets(raw)
	if err != nil {
		return nil, true, err
	}
	return table, true, nil
}

// ParseScrambleSecrets 解一份密钥表。空对象合规（等于没配）；租户标识为空、密钥短于下限、重复键都拒——重复键在
// JSON 解码里是后者静默盖前者，配错了的那一把就无声生效。
func ParseScrambleSecrets(raw []byte) (SecretTable, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return nil, fmt.Errorf("%w: the file must hold one JSON object of tenant to secret", ErrInvalidScrambleSecrets)
	}
	table := SecretTable{}
	for decoder.More() {
		keyToken, err := decoder.Token()
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrInvalidScrambleSecrets, err)
		}
		tenant, _ := keyToken.(string)
		var secret string
		if err := decoder.Decode(&secret); err != nil {
			return nil, fmt.Errorf("%w: the secret of a tenant must be a string", ErrInvalidScrambleSecrets)
		}
		switch {
		case strings.TrimSpace(tenant) == "" || tenant != strings.TrimSpace(tenant):
			return nil, fmt.Errorf("%w: a tenant key is blank or padded", ErrInvalidScrambleSecrets)
		case utf8.RuneCountInString(secret) < minScrambleSecretLength:
			return nil, fmt.Errorf("%w: the secret of tenant %s is shorter than %d characters", ErrInvalidScrambleSecrets, tenant, minScrambleSecretLength)
		}
		if _, repeated := table[tenant]; repeated {
			return nil, fmt.Errorf("%w: tenant %s appears twice", ErrInvalidScrambleSecrets, tenant)
		}
		table[tenant] = secret
	}
	if _, err := decoder.Token(); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidScrambleSecrets, err)
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("%w: trailing content after the object", ErrInvalidScrambleSecrets)
	}
	return table, nil
}
