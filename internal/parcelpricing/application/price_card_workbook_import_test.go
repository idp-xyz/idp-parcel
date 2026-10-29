package application_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"testing"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/application"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
)

// 本文件证价卡导入的第二种载体在应用层的编排（ADR-0349 决定四）：页面表单提交的结构化工作簿先规范化、来源标识取规范化字节的散列
// （form:<前 12 位>）、再读规范化字节；预览与录入走同一段，读法折成答复的方式与读文件同一个。

// workbookReaderDouble 是工作簿读口的替身：规范化 = 去掉所有空格（足以让「空白不同的同一份内容」得同一份规范化字节），
// 读到的永远是规范化字节；canonicalizeErr 非空时规范化失败。
type workbookReaderDouble struct {
	reading         ports.PriceCardTemplateReading
	canonicalizeErr error
	read            []byte
	canonicalized   int
}

func (double *workbookReaderDouble) CanonicalizePriceCardWorkbook(raw []byte) ([]byte, error) {
	double.canonicalized++
	if double.canonicalizeErr != nil {
		return nil, double.canonicalizeErr
	}
	return bytes.ReplaceAll(raw, []byte(" "), nil), nil
}

func (double *workbookReaderDouble) ReadPriceCardWorkbook(canonical []byte) ports.PriceCardTemplateReading {
	double.read = canonical
	return double.reading
}

func previewWithWorkbook(t *testing.T, files ports.PriceCardTemplateReader, workbook ports.PriceCardWorkbookReader) *application.PreviewPriceCardImportHandler {
	t.Helper()
	handler, err := application.NewPreviewPriceCardImportHandler(files, application.WithWorkbookReader(workbook))
	if err != nil {
		t.Fatal(err)
	}
	return handler
}

func formSourceOf(canonical string) (string, string) {
	sum := sha256.Sum256([]byte(canonical))
	digest := hex.EncodeToString(sum[:])
	return "form:" + digest[:12], digest
}

// Covers: `AT-PP-046`——预览收页面表单的工作簿：不碰文件读口；读口读到的是规范化字节；来源标识名为 form:<规范化字节散列前 12 位>、散列为
// 规范化字节的完整 SHA-256；空白不同的同一份内容得同一个来源标识；读法折成答复与读文件同一个（已校验带内容、带问题带坐标）。
func TestPreviewReadsAFormWorkbookThroughTheWorkbookReader(t *testing.T) {
	files := &templateReaderDouble{}
	workbook := &workbookReaderDouble{reading: validatedReading(t, "30")}
	handler := previewWithWorkbook(t, files, workbook)

	answer, err := handler.Handle(context.Background(), application.PreviewPriceCardImportCommand{
		Tenant: previewTenant(t), FromForm: true, Raw: []byte(`{"a": 1, "b": 2}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	name, digest := formSourceOf(`{"a":1,"b":2}`)
	if answer.Outcome != application.PriceCardImportValidated || answer.Content == nil || answer.SourceFile.Name() != name || answer.SourceFile.SHA256() != digest ||
		!answer.SourceFile.FromForm() || string(workbook.read) != `{"a":1,"b":2}` || files.read != nil {
		t.Fatalf("answer = %+v read=%s fileReaderTouched=%v", answer, workbook.read, files.read != nil)
	}

	spaced, err := handler.Handle(context.Background(), application.PreviewPriceCardImportCommand{
		Tenant: previewTenant(t), FromForm: true, FileName: "ignored.xlsx", Raw: []byte(`{ "a" : 1 , "b" : 2 }`),
	})
	if err != nil || spaced.SourceFile != answer.SourceFile {
		t.Fatalf("空白不同的同一份内容该得同一个来源标识：%+v vs %+v err=%v", spaced.SourceFile, answer.SourceFile, err)
	}

	problem := ports.PriceCardTemplateProblem{Sheet: "tables", Row: 2, Column: "currency", Code: ports.TemplateCellInvalid, Message: "x"}
	workbook.reading = ports.PriceCardTemplateReading{TemplateVersion: "PPT-2", Accepted: true, Plan: pricingPlanForPreview(t).Reference(), Problems: []ports.PriceCardTemplateProblem{problem}}
	withProblems, err := handler.Handle(context.Background(), application.PreviewPriceCardImportCommand{Tenant: previewTenant(t), FromForm: true, Raw: []byte(`{}`)})
	if err != nil || withProblems.Outcome != application.PriceCardImportHasProblems || withProblems.Content != nil || len(withProblems.Problems) != 1 ||
		withProblems.TemplateVersion != "PPT-2" || withProblems.Plan.ID() != "SYN-PLAN-PREVIEW" {
		t.Fatalf("带问题：%+v err=%v", withProblems, err)
	}

	workbook.reading = ports.PriceCardTemplateReading{TemplateVersion: "PPT-2", Problems: []ports.PriceCardTemplateProblem{{Code: ports.TemplateVersionUnsupported, Message: "x"}}}
	rejected, err := handler.Handle(context.Background(), application.PreviewPriceCardImportCommand{Tenant: previewTenant(t), FromForm: true, Raw: []byte(`{}`)})
	if err != nil || rejected.Outcome != application.PriceCardImportNotAccepted || rejected.SourceFile.Name() == "" {
		t.Fatalf("整份不收的读法仍带来源标识：%+v err=%v", rejected, err)
	}
}

// 形状没过、缺租户、没装工作簿读口：各答各的——形状没过是整份不收（读不通是一种读法，没有来源标识可记），缺租户未受理，没装读口是装配错误。
func TestPreviewOfAFormWorkbookRefusesWhatCannotBeRead(t *testing.T) {
	cases := map[string]struct {
		err  error
		code ports.PriceCardTemplateProblemCode
		text string
	}{
		"形状不对": {ports.PriceCardWorkbookRejection{Detail: "顶层缺 sheets"}, ports.TemplateFileNotWorkbook, "读不出这份工作簿：顶层缺 sheets"},
		"超限":   {ports.PriceCardWorkbookRejection{TooLarge: true, Detail: "超过技术上限"}, ports.TemplateFileTooLarge, "读不出这份工作簿：超过技术上限"},
		"别的错误": {errors.New("boom"), ports.TemplateFileNotWorkbook, "boom"},
	}
	for name, testCase := range cases {
		workbook := &workbookReaderDouble{canonicalizeErr: testCase.err}
		answer, err := previewWithWorkbook(t, &templateReaderDouble{}, workbook).Handle(context.Background(),
			application.PreviewPriceCardImportCommand{Tenant: previewTenant(t), FromForm: true, Raw: []byte(`x`)})
		if err != nil || answer.Outcome != application.PriceCardImportNotAccepted || answer.SourceFile.Name() != "" || workbook.read != nil ||
			len(answer.Problems) != 1 || answer.Problems[0].Code != testCase.code || !strings.Contains(answer.Problems[0].Message, testCase.text) {
			t.Errorf("%s：%+v err=%v", name, answer, err)
		}
	}

	workbook := &workbookReaderDouble{reading: validatedReading(t, "30")}
	answer, err := previewWithWorkbook(t, &templateReaderDouble{}, workbook).Handle(context.Background(),
		application.PreviewPriceCardImportCommand{FromForm: true, Raw: []byte(`{}`)})
	if err != nil || answer.Outcome != application.PriceCardImportNotAccepted || workbook.canonicalized != 0 {
		t.Fatalf("缺租户：%+v err=%v 规范化 %d 次", answer, err, workbook.canonicalized)
	}

	if _, err := previewHandler(t, &templateReaderDouble{}).Handle(context.Background(),
		application.PreviewPriceCardImportCommand{Tenant: previewTenant(t), FromForm: true, Raw: []byte(`{}`)}); !errors.Is(err, application.ErrWorkbookCarrierNotWired) {
		t.Fatalf("没装工作簿读口：err = %v", err)
	}
}

// 没装工作簿读口时，原有的文件路径一字不变（选项是可缺的）。
func TestFilePreviewDoesNotNeedTheWorkbookReader(t *testing.T) {
	reader := &templateReaderDouble{reading: validatedReading(t, "30")}
	answer, err := previewHandler(t, reader).Handle(context.Background(),
		application.PreviewPriceCardImportCommand{Tenant: previewTenant(t), FileName: "card.xlsx", Raw: []byte("bytes")})
	if err != nil || answer.Outcome != application.PriceCardImportValidated || answer.SourceFile.Name() != "card.xlsx" || answer.SourceFile.FromForm() {
		t.Fatalf("answer = %+v err=%v", answer, err)
	}
}

// Covers: `AT-PP-045`——录入收页面表单的工作簿：草稿落册，源文件身份是 form:<散列前 12 位> 加规范化字节的完整散列，且认得出来自页面表单；
// 录入者仍取自命令（信封）；同一份内容（空白不同）再录答重放；缺录入者与文件路径同样未受理，不读不落。
func TestSubmitSavesAFormWorkbookAsADraftWhoseSourceIsTheForm(t *testing.T) {
	register := newDraftRegisterFake()
	workbook := &workbookReaderDouble{reading: validatedReading(t, "30")}
	handler, err := application.NewSubmitPriceCardDraftHandler(&templateReaderDouble{}, register, fixedClock{at: draftClockAt}, application.WithWorkbookReader(workbook))
	if err != nil {
		t.Fatal(err)
	}
	command := application.SubmitPriceCardDraftCommand{Tenant: previewTenant(t), Submitter: draftSubmitter, FromForm: true, Raw: []byte(`{"a": 1}`)}

	first, err := handler.Handle(context.Background(), command)
	if err != nil {
		t.Fatal(err)
	}
	name, digest := formSourceOf(`{"a":1}`)
	if first.Outcome != application.PriceCardDraftSubmitted || !first.HasDraft || first.Draft.SourceFile().Name() != name ||
		first.Draft.SourceFile().SHA256() != digest || !first.Draft.SourceFile().FromForm() || first.Draft.Submitter() != draftSubmitter ||
		first.Draft.Status() != domain.PriceCardDraftStatusValidated {
		t.Fatalf("首录 = %+v", first)
	}
	command.Raw = []byte(`{ "a" :   1 }`)
	if again, err := handler.Handle(context.Background(), command); err != nil || again.Outcome != application.PriceCardDraftReplayed {
		t.Fatalf("空白不同的同一份内容该答重放：%+v err=%v", again, err)
	}

	noSubmitter := command
	noSubmitter.Submitter = ""
	calls := workbook.canonicalized
	if result, err := handler.Handle(context.Background(), noSubmitter); err != nil || result.Outcome != application.PriceCardDraftNotAccepted || workbook.canonicalized != calls {
		t.Fatalf("缺录入者：%+v err=%v", result, err)
	}

	broken := &workbookReaderDouble{canonicalizeErr: ports.PriceCardWorkbookRejection{Detail: "不是对象"}}
	rows := newDraftRegisterFake()
	brokenHandler, err := application.NewSubmitPriceCardDraftHandler(&templateReaderDouble{}, rows, fixedClock{at: draftClockAt}, application.WithWorkbookReader(broken))
	if err != nil {
		t.Fatal(err)
	}
	if result, err := brokenHandler.Handle(context.Background(), command); err != nil || result.Outcome != application.PriceCardDraftNotAccepted || result.HasDraft || rows.calls != 0 {
		t.Fatalf("形状不对的工作簿不落行：%+v err=%v calls=%d", result, err, rows.calls)
	}

	if _, err := submitHandler(t, &templateReaderDouble{}, newDraftRegisterFake()).Handle(context.Background(), command); !errors.Is(err, application.ErrWorkbookCarrierNotWired) {
		t.Fatalf("没装工作簿读口：err = %v", err)
	}
}
