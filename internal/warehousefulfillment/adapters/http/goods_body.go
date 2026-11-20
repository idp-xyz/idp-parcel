package warehousehttp

import (
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

// goodsBody 是一笔商品修订的答复形状，代录回显与运营查阅共用。运营查阅带出提交途径；代录另带依据引用。
// 登记责任方在这一面带出：货主自己的读口不带那一格，那一口还没接。
type goodsBody struct {
	OwnerAccount         string                 `json:"ownerAccount"`
	GoodsCode            string                 `json:"goodsCode"`
	Revision             int                    `json:"revision"`
	Name                 string                 `json:"name"`
	BaseUnit             string                 `json:"baseUnit"`
	PackagingLevels      []goodsLevelBody       `json:"packagingLevels"`
	TrackBatch           bool                   `json:"trackBatch"`
	TrackExpiry          bool                   `json:"trackExpiry"`
	Aliases              []goodsAliasBody       `json:"aliases"`
	Composite            bool                   `json:"composite"`
	Composition          *goodsCompositionBody  `json:"composition,omitempty"`
	Declaration          *goodsDeclarationBody  `json:"declaration,omitempty"`
	DeclaredMeasurements []goodsMeasurementBody `json:"declaredMeasurements"`
	SubmissionPath       string                 `json:"submissionPath"`
	EvidenceReference    string                 `json:"evidenceReference,omitempty"`
	Registrant           string                 `json:"registrant"`
	RegisteredAt         string                 `json:"registeredAt"`
}

type goodsLevelBody struct {
	Name   string `json:"name"`
	Factor int64  `json:"factor"`
}

type goodsAliasBody struct {
	Code     string             `json:"code"`
	Type     string             `json:"type"`
	Platform string             `json:"platform,omitempty"`
	Validity *goodsValidityBody `json:"validity,omitempty"`
}

type goodsValidityBody struct {
	From string `json:"from,omitempty"`
	To   string `json:"to,omitempty"`
}

type goodsCompositionBody struct {
	OutboundMode string               `json:"outboundMode"`
	Components   []goodsComponentBody `json:"components"`
}

type goodsComponentBody struct {
	Code     string `json:"code"`
	Quantity int64  `json:"quantity"`
}

type goodsDeclarationBody struct {
	Name          string `json:"name,omitempty"`
	HSCode        string `json:"hsCode,omitempty"`
	OriginCountry string `json:"originCountry,omitempty"`
	UnitPrice     string `json:"unitPrice,omitempty"`
	Currency      string `json:"currency,omitempty"`
}

type goodsMeasurementBody struct {
	Level      string               `json:"level"`
	Dimensions *goodsDimensionsBody `json:"dimensions,omitempty"`
	Weight     *goodsWeightBody     `json:"weight,omitempty"`
}

type goodsDimensionsBody struct {
	Length string `json:"length"`
	Width  string `json:"width"`
	Height string `json:"height"`
	Unit   string `json:"unit"`
}

type goodsWeightBody struct {
	Value string `json:"value"`
	Unit  string `json:"unit"`
}

func goodsBodiesOfRows(rows []ports.GoodsRow) []goodsBody {
	bodies := make([]goodsBody, 0, len(rows))
	for _, row := range rows {
		bodies = append(bodies, goodsBodyOfRow(row))
	}
	return bodies
}

func goodsBodyOfRow(row ports.GoodsRow) goodsBody {
	body := goodsBody{
		OwnerAccount: row.OwnerAccount, GoodsCode: row.GoodsCode, Revision: row.Revision,
		Name: row.Name, BaseUnit: row.BaseUnit, TrackBatch: row.TrackBatch, TrackExpiry: row.TrackExpiry,
		Composite: row.Composite, SubmissionPath: row.SubmissionPath, EvidenceReference: row.EvidenceReference,
		Registrant: row.Registrant, RegisteredAt: rfc3339(row.RegisteredAt),
		PackagingLevels: goodsLevelBodies(row.PackagingLevels), Aliases: goodsAliasBodies(row.Aliases),
		DeclaredMeasurements: goodsMeasurementBodies(row.DeclaredMeasurements),
	}
	if row.Composite {
		components := make([]goodsComponentBody, 0, len(row.Components))
		for _, component := range row.Components {
			components = append(components, goodsComponentBody{Code: component.Code, Quantity: component.Quantity})
		}
		body.Composition = &goodsCompositionBody{OutboundMode: row.OutboundMode, Components: components}
	}
	if declaration := goodsDeclarationBodyOf(row.Declaration); declaration != nil {
		body.Declaration = declaration
	}
	return body
}

func goodsLevelBodies(rows []ports.GoodsLevelRow) []goodsLevelBody {
	bodies := make([]goodsLevelBody, 0, len(rows))
	for _, row := range rows {
		bodies = append(bodies, goodsLevelBody{Name: row.Name, Factor: row.Factor})
	}
	return bodies
}

func goodsAliasBodies(rows []ports.GoodsAliasRow) []goodsAliasBody {
	bodies := make([]goodsAliasBody, 0, len(rows))
	for _, row := range rows {
		body := goodsAliasBody{Code: row.Code, Type: row.Type, Platform: row.Platform}
		if row.ValidFrom != nil || row.ValidTo != nil {
			validity := goodsValidityBody{}
			if row.ValidFrom != nil {
				validity.From = rfc3339(*row.ValidFrom)
			}
			if row.ValidTo != nil {
				validity.To = rfc3339(*row.ValidTo)
			}
			body.Validity = &validity
		}
		bodies = append(bodies, body)
	}
	return bodies
}

func goodsDeclarationBodyOf(row ports.GoodsDeclarationRow) *goodsDeclarationBody {
	if row.Name == "" && row.HSCode == "" && row.OriginCountry == "" && row.UnitPrice == "" && row.Currency == "" {
		return nil
	}
	return &goodsDeclarationBody{
		Name: row.Name, HSCode: row.HSCode, OriginCountry: row.OriginCountry, UnitPrice: row.UnitPrice, Currency: row.Currency,
	}
}

func goodsMeasurementBodies(rows []ports.GoodsMeasurementRow) []goodsMeasurementBody {
	bodies := make([]goodsMeasurementBody, 0, len(rows))
	for _, row := range rows {
		body := goodsMeasurementBody{Level: row.Level}
		if row.Dimensions != nil {
			body.Dimensions = &goodsDimensionsBody{
				Length: row.Dimensions.Length, Width: row.Dimensions.Width, Height: row.Dimensions.Height, Unit: row.Dimensions.Unit,
			}
		}
		if row.Weight != nil {
			body.Weight = &goodsWeightBody{Value: row.Weight.Value, Unit: row.Weight.Unit}
		}
		bodies = append(bodies, body)
	}
	return bodies
}

func goodsBodyOfRevision(revision domain.GoodsRevision) goodsBody {
	content := revision.Content()
	levels := make([]ports.GoodsLevelRow, 0, len(content.PackagingLevels()))
	for _, level := range content.PackagingLevels() {
		levels = append(levels, ports.GoodsLevelRow{Name: level.Name().String(), Factor: level.Factor()})
	}
	aliases := make([]ports.GoodsAliasRow, 0, len(content.Aliases()))
	for _, alias := range content.Aliases() {
		row := ports.GoodsAliasRow{Code: alias.Code().String(), Type: alias.Kind().String()}
		if platform, given := alias.Platform(); given {
			row.Platform = platform.String()
		}
		from, hasFrom := alias.Validity().From()
		to, hasTo := alias.Validity().To()
		if hasFrom {
			row.ValidFrom = &from
		}
		if hasTo {
			row.ValidTo = &to
		}
		aliases = append(aliases, row)
	}
	var components []ports.GoodsComponentRow
	outbound := ""
	if content.Composite() {
		if mode, given := content.OutboundMode(); given {
			outbound = mode.String()
		}
		components = make([]ports.GoodsComponentRow, 0, len(content.Components()))
		for _, component := range content.Components() {
			components = append(components, ports.GoodsComponentRow{Code: component.Code().String(), Quantity: component.Quantity()})
		}
	}
	declaration := content.Declaration()
	declared := ports.GoodsDeclarationRow{}
	if name, given := declaration.Name(); given {
		declared.Name = name.String()
	}
	if code, given := declaration.HSCode(); given {
		declared.HSCode = code.String()
	}
	if country, given := declaration.OriginCountry(); given {
		declared.OriginCountry = country.String()
	}
	if price, currency, given := declaration.UnitPrice(); given {
		declared.UnitPrice = price.String()
		declared.Currency = currency.String()
	}
	measurements := make([]ports.GoodsMeasurementRow, 0, len(content.DeclaredMeasurements()))
	for _, measurement := range content.DeclaredMeasurements() {
		row := ports.GoodsMeasurementRow{Level: measurement.Level().String()}
		if dimensions, given := measurement.Dimensions(); given {
			row.Dimensions = &ports.GoodsDimensionsRow{
				Length: dimensions.Length().String(), Width: dimensions.Width().String(),
				Height: dimensions.Height().String(), Unit: dimensions.Unit().String(),
			}
		}
		if weight, given := measurement.Weight(); given {
			row.Weight = &ports.GoodsWeightRow{Value: weight.Value().String(), Unit: weight.Unit().String()}
		}
		measurements = append(measurements, row)
	}
	evidence := ""
	if reference, given := revision.Submission().EvidenceReference(); given {
		evidence = reference.String()
	}
	return goodsBodyOfRow(ports.GoodsRow{
		OwnerAccount: revision.Owner().String(), GoodsCode: revision.Code().String(), Revision: revision.Revision(),
		Name: content.Name().String(), BaseUnit: content.BaseUnit().String(), PackagingLevels: levels,
		TrackBatch: content.TrackBatch(), TrackExpiry: content.TrackExpiry(), Aliases: aliases,
		Composite: content.Composite(), OutboundMode: outbound, Components: components, Declaration: declared,
		DeclaredMeasurements: measurements, SubmissionPath: revision.Submission().Path().String(),
		EvidenceReference: evidence, Registrant: revision.Registrant().String(), RegisteredAt: revision.RegisteredAt(),
	})
}
