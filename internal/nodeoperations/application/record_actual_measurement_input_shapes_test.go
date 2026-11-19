package application_test

import (
	"context"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/application"
)

// 本文件逐一钉住测量命令里数量与单位的各种写法：哪些收、哪些不收。命令是线格式的镜像，六个格都是字符串，所以这里
// 只经命令说话，不碰领域值的取法；夹具全为合成（S 级）。

func handleMeasurement(t *testing.T, command application.RecordActualMeasurementCommand) (application.RecordActualMeasurementResult, *memoryMeasurements) {
	t.Helper()
	store := newMemoryMeasurements()
	handler := newMeasurementHandler(t, store, time.Date(2026, 11, 17, 3, 0, 0, 0, time.UTC))
	result, err := handler.Handle(context.Background(), command)
	if err != nil {
		t.Fatalf("登记：%v", err)
	}
	return result, store
}

// Covers: 每一格数量都要是正的十进制数。不是数、零、负数、带正号、小数点缺一头、指数写法、千分位、夹着空白、全角数字、
// 多个小数点，重量与长宽高各自都不收；不收的一条不碰册。
func TestAMalformedQuantityIsNotAcceptedInAnyOfTheFourFields(t *testing.T) {
	fields := map[string]func(*application.RecordActualMeasurementCommand, string){
		"重量": func(command *application.RecordActualMeasurementCommand, raw string) { command.WeightValue = raw },
		"长":  func(command *application.RecordActualMeasurementCommand, raw string) { command.Length = raw },
		"宽":  func(command *application.RecordActualMeasurementCommand, raw string) { command.Width = raw },
		"高":  func(command *application.RecordActualMeasurementCommand, raw string) { command.Height = raw },
	}
	malformed := map[string]string{
		"不是数":      "abc",
		"零":        "0",
		"零带小数":     "0.00",
		"负数":       "-2",
		"带正号":      "+2",
		"小数点后没有数":  "2.",
		"小数点前没有数":  ".5",
		"指数写法":     "1e2",
		"千分位":      "1,000",
		"数字之间夹着空白": "2 5",
		"全角数字":     "２５",
		"两个小数点":    "1.2.3",
		"只有空白":     " \t ",
	}
	for field, set := range fields {
		for name, raw := range malformed {
			t.Run(field+"/"+name, func(t *testing.T) {
				command := weightCommand()
				set(&command, raw)
				result, store := handleMeasurement(t, command)
				if result.Outcome() != application.ActualMeasurementNotAccepted {
					t.Fatalf("%q：结果 %s，want 不收", raw, result.Outcome())
				}
				if len(store.calls) != 0 {
					t.Fatalf("不收的一条碰了册：%v", store.calls)
				}
			})
		}
	}
}

// Covers: 单位去空白后要在场，并且在产品内置目录上、类别对得上：重量格要重量单位，尺寸格要长度单位。空白、大小写不对、
// 拿长度单位报重量、拿重量单位或体积单位报尺寸，都不收；不收的一条不碰册。
func TestAUnitThatIsBlankOrOffTheCatalogueOrOfTheWrongCategoryIsNotAccepted(t *testing.T) {
	for name, change := range map[string]func(*application.RecordActualMeasurementCommand){
		"重量单位只有空白":    func(command *application.RecordActualMeasurementCommand) { command.WeightUnit = "  " },
		"尺寸单位只有空白":    func(command *application.RecordActualMeasurementCommand) { command.DimensionUnit = "\t" },
		"重量单位大小写不对":   func(command *application.RecordActualMeasurementCommand) { command.WeightUnit = "kg" },
		"尺寸单位大小写不对":   func(command *application.RecordActualMeasurementCommand) { command.DimensionUnit = "cm" },
		"重量格报了长度单位":   func(command *application.RecordActualMeasurementCommand) { command.WeightUnit = "CM" },
		"尺寸格报了重量单位":   func(command *application.RecordActualMeasurementCommand) { command.DimensionUnit = "KG" },
		"尺寸格报了体积单位":   func(command *application.RecordActualMeasurementCommand) { command.DimensionUnit = "M3" },
		"重量格报了件数单位":   func(command *application.RecordActualMeasurementCommand) { command.WeightUnit = "PCS" },
		"重量单位带了不可见字符": func(command *application.RecordActualMeasurementCommand) { command.WeightUnit = "K\u00a0G" },
	} {
		t.Run(name, func(t *testing.T) {
			command := weightCommand()
			change(&command)
			result, store := handleMeasurement(t, command)
			if result.Outcome() != application.ActualMeasurementNotAccepted {
				t.Fatalf("结果 %s，want 不收", result.Outcome())
			}
			if len(store.calls) != 0 {
				t.Fatalf("不收的一条碰了册：%v", store.calls)
			}
		})
	}
}

// Covers: 数量与单位两头的空白不是内容：带着空白报来的一条，与不带空白的同一条落下同一份内容（摘要相同）；
// 目录上别的合法单位仍是另一份内容。摘要是落册的，读回时重算比对，所以这里比的是摘要而不是取值写法。
func TestPaddingAroundAQuantityOrAUnitIsNotContent(t *testing.T) {
	clean, _ := handleMeasurement(t, weightCommand())
	cleanMeasurement, present := clean.Measurement()
	if clean.Outcome() != application.ActualMeasurementRecorded || !present {
		t.Fatalf("不带空白的一条：结果 %s", clean.Outcome())
	}

	padded := weightCommand()
	padded.WeightValue, padded.WeightUnit = " 2.10 ", " KG "
	padded.Length, padded.Width, padded.Height, padded.DimensionUnit = "\t20", "15\n", " 10", " CM "
	result, _ := handleMeasurement(t, padded)
	paddedMeasurement, present := result.Measurement()
	if result.Outcome() != application.ActualMeasurementRecorded || !present {
		t.Fatalf("带空白的一条：结果 %s，want 记下", result.Outcome())
	}
	if paddedMeasurement.ContentDigest() != cleanMeasurement.ContentDigest() {
		t.Fatalf("带空白的摘要 %s ≠ 不带空白的 %s", paddedMeasurement.ContentDigest(), cleanMeasurement.ContentDigest())
	}

	otherUnits := weightCommand()
	otherUnits.WeightUnit, otherUnits.DimensionUnit = "LB", "IN"
	other, _ := handleMeasurement(t, otherUnits)
	otherMeasurement, present := other.Measurement()
	if other.Outcome() != application.ActualMeasurementRecorded || !present {
		t.Fatalf("目录上别的单位：结果 %s，want 记下", other.Outcome())
	}
	if otherMeasurement.ContentDigest() == cleanMeasurement.ContentDigest() {
		t.Fatal("换了单位，摘要却没变")
	}
}
