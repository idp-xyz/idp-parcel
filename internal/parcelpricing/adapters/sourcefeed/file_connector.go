// Package sourcefeed 放计价参考序列的来源连接器实现（ADR-0099 决定六；票
// pricing-reference-series-operations/06）。契约在 ports（SourceConnector），本包只放实现：
// 首个是以本地受控目录文件为源的 FileConnector，供测试与受控导入——运营把来源公布页另存的
// 文件放进受控目录，连接器读文件、算 SHA-256、记读取时刻与文件定位符。
//
// **本包不写任何出网代码。** 出网连接器（CFETS 人民币中间价一段）留 draft：它的全部机制
// （契约、转录、凭证、失败行为）由 FileConnector 先验证；开工前置是部署侧登记出网能力与代理
// 策略（票 06 裁决一），届时加的是一个 SourceConnector 实现与一行装配，不是契约。
package sourcefeed

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
)

// FileConnectorKind 是绑定里声明「从受控目录文件抓」时写的连接器种类。
const FileConnectorKind = "FILE"

var (
	// ErrSourceUnavailable 表示来源不可达：文件不在受控目录里，或读不出来。
	ErrSourceUnavailable = errors.New("parcel pricing source feed: source unavailable")
	// ErrPublicationUnreadable 表示原文解不开：不是本连接器认的文档形状，或缺公布日期、生效起点、取值。
	ErrPublicationUnreadable = errors.New("parcel pricing source feed: publication unreadable")
	// ErrLocatorOutsideRoot 表示定位符指到受控目录之外——上溯或绝对路径。连接器只读受控目录。
	ErrLocatorOutsideRoot = errors.New("parcel pricing source feed: locator points outside the controlled directory")
	// ErrPublicationSeriesMismatch 表示文件自报的序列标识与绑定不符——放错目录的文件不得登进别的序列。
	ErrPublicationSeriesMismatch = errors.New("parcel pricing source feed: publication declares another series")
)

// publishedDateLayout 是文件里公布日期的写法；生效起点按 RFC 3339 写，两者分开是因为来源公布的
// 是一个日期，而期次的起点是一个时刻。
const publishedDateLayout = "2006-01-02"

// publicationDocument 是受控目录文件的形状：来源公布页另存后由运营转成这四格。取值原样是字符串
// ——十进制不过浮点。
type publicationDocument struct {
	SeriesID      string `json:"seriesId"`
	PublishedOn   string `json:"publishedOn"`
	EffectiveFrom string `json:"effectiveFrom"`
	Value         string `json:"value"`
}

// FileConnector 实现 ports.SourceConnector：从受控目录读一份公布文件。
type FileConnector struct {
	root  string
	clock ports.Clock
}

// NewFileConnector 收受控目录根与时钟。根不给就拒——连接器不猜读取根；时钟不给也拒——抓取时刻是
// 凭证的一格，不能凭空。
func NewFileConnector(root string, clock ports.Clock) (*FileConnector, error) {
	if strings.TrimSpace(root) == "" {
		return nil, fmt.Errorf("parcel pricing source feed: controlled directory root is required")
	}
	if clock == nil {
		return nil, fmt.Errorf("parcel pricing source feed: clock is required")
	}
	return &FileConnector{root: filepath.Clean(root), clock: clock}, nil
}

func (connector *FileConnector) Kind() string { return FileConnectorKind }

// Fetch 读受控目录内 spec.Locator 指的文件。字节读回那一刻由 domain.NewPublishedRecord 算摘要；
// 时刻取时钟当下；定位符记 `file:` 加受控目录内的相对路径——根属部署，不进凭证。
func (connector *FileConnector) Fetch(_ context.Context, spec ports.FetchSpec) (domain.PublishedRecord, error) {
	relative, err := connector.relativeWithinRoot(spec.Locator)
	if err != nil {
		return domain.PublishedRecord{}, err
	}
	body, err := os.ReadFile(filepath.Join(connector.root, filepath.FromSlash(relative)))
	if err != nil {
		return domain.PublishedRecord{}, fmt.Errorf("%w: %s: %v", ErrSourceUnavailable, relative, err)
	}
	document, err := parsePublication(body)
	if err != nil {
		return domain.PublishedRecord{}, err
	}
	publishedOn, err := time.Parse(publishedDateLayout, document.PublishedOn)
	if err != nil {
		return domain.PublishedRecord{}, fmt.Errorf("%w: publishedOn %q is not %s", ErrPublicationUnreadable, document.PublishedOn, publishedDateLayout)
	}
	record, err := domain.NewPublishedRecord(body, connector.clock.Now(), "file:"+relative, publishedOn)
	if err != nil {
		return domain.PublishedRecord{}, fmt.Errorf("%w: %v", ErrPublicationUnreadable, err)
	}
	return record, nil
}

// Transcribe 从原文读出观测（生效起点、取值），核对文件自报的序列标识，其余交给
// domain.TranscribeSourceFeed——整版重述、幂等重放、版本号与凭证写法都不在这里。
func (connector *FileConnector) Transcribe(input ports.TranscriptionInput) (domain.ReferenceSeriesRegistrationSpec, error) {
	document, err := parsePublication(input.Record.Body())
	if err != nil {
		return domain.ReferenceSeriesRegistrationSpec{}, err
	}
	if document.SeriesID != input.Binding.SeriesID() {
		return domain.ReferenceSeriesRegistrationSpec{}, fmt.Errorf("%w: file declares %q, binding is %q",
			ErrPublicationSeriesMismatch, document.SeriesID, input.Binding.SeriesID())
	}
	effectiveFrom, err := time.Parse(time.RFC3339, document.EffectiveFrom)
	if err != nil {
		return domain.ReferenceSeriesRegistrationSpec{}, fmt.Errorf("%w: effectiveFrom %q is not RFC 3339", ErrPublicationUnreadable, document.EffectiveFrom)
	}
	value, err := domain.ParseDecimal(document.Value)
	if err != nil {
		return domain.ReferenceSeriesRegistrationSpec{}, fmt.Errorf("%w: value %q: %v", ErrPublicationUnreadable, document.Value, err)
	}
	observed, err := domain.NewSeriesObservation(effectiveFrom, value)
	if err != nil {
		return domain.ReferenceSeriesRegistrationSpec{}, fmt.Errorf("%w: %v", ErrPublicationUnreadable, err)
	}
	storageLocator, _ := input.Placement.StoredLocator()
	return domain.TranscribeSourceFeed(domain.SourceFeedTranscription{
		Binding:        input.Binding,
		Record:         input.Record,
		StorageLocator: storageLocator,
		Prior:          input.Prior,
		Observation:    observed,
	})
}

// relativeWithinRoot 把绑定声明的定位符收成受控目录内的相对路径（斜杠形）。空、绝对、带卷名、
// 以分隔符起头、清理后上溯出根的，一律拒——受控目录是本连接器唯一能读的地方。
func (connector *FileConnector) relativeWithinRoot(locator string) (string, error) {
	if locator == "" || filepath.IsAbs(locator) || filepath.VolumeName(locator) != "" ||
		strings.HasPrefix(locator, "/") || strings.HasPrefix(locator, `\`) {
		return "", fmt.Errorf("%w: %q", ErrLocatorOutsideRoot, locator)
	}
	cleaned := filepath.Clean(filepath.FromSlash(locator))
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("%w: %q", ErrLocatorOutsideRoot, locator)
	}
	relative, err := filepath.Rel(connector.root, filepath.Join(connector.root, cleaned))
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("%w: %q", ErrLocatorOutsideRoot, locator)
	}
	return filepath.ToSlash(relative), nil
}

// parsePublication 只做形状翻译；缺公布日期的文件当场拒——没有它记录放不到时间线上。
func parsePublication(body []byte) (publicationDocument, error) {
	var document publicationDocument
	if err := json.Unmarshal(body, &document); err != nil {
		return publicationDocument{}, fmt.Errorf("%w: %v", ErrPublicationUnreadable, err)
	}
	if strings.TrimSpace(document.PublishedOn) == "" {
		return publicationDocument{}, fmt.Errorf("%w: publishedOn is missing", ErrPublicationUnreadable)
	}
	return document, nil
}

var _ ports.SourceConnector = (*FileConnector)(nil)
