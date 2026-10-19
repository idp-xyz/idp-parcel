package application_test

import (
	"reflect"
	"slices"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// previewOf 把一条出发命令里决定读数的三格原样搬进问询：同参即这三格逐字相同。
func previewOf(command application.RecordMovementFactCommand) application.PreviewDepartureGateCommand {
	return application.PreviewDepartureGateCommand{
		TenantID:  command.TenantID,
		Schedule:  command.Schedule,
		Segment:   command.Segment,
		DepartsAt: command.OccurredAt,
	}
}

func previewHandler(t *testing.T, fixture *departureGateFixture) *application.PreviewDepartureGateHandler {
	t.Helper()
	handler, err := application.NewPreviewDepartureGateHandler(fixture.reader(t))
	if err != nil {
		t.Fatalf("装问询编排：%v", err)
	}
	return handler
}

// Covers: AT-TF-227 编排半边——问询交回的读数与随后同参登记的那条出发同格：整格、起讫、停点、逐对象那一格连同关务
// 判断标识与依据都相同，问关务的问法也逐问相同。读数的五格各至少一例、本侧新登记出发会停的各停点各一例：同一条路径，
// 任何一格都不该分岔。每例另钉住它读出的整格，用例表漏了哪一格由末尾那道核对指名，覆盖面不靠注释自称。
func TestAPreviewAnswersTheSameReadingAsTheDepartureWithTheSameParameters(t *testing.T) {
	type arrangement func(*departureGateFixture, *application.RecordMovementFactCommand)
	cases := map[string]struct {
		arrange arrangement
		want    domain.DepartureGateAnswer
	}{
		"两件都不受门禁": {
			arrange: func(*departureGateFixture, *application.RecordMovementFactCommand) {},
			want:    domain.DepartureGateNotGuarded,
		},
		"两件都放行": {
			arrange: func(fixture *departureGateFixture, _ *application.RecordMovementFactCommand) {
				fixture.customs.answers["parcel-1"] = domain.DepartureGateAdmitted
				fixture.customs.answers["parcel-2"] = domain.DepartureGateAdmitted
			},
			want: domain.DepartureGateAdmitted,
		},
		"一件关务答未决": {
			arrange: func(fixture *departureGateFixture, _ *application.RecordMovementFactCommand) {
				fixture.customs.answers["parcel-2"] = domain.DepartureGateUndecided
			},
			want: domain.DepartureGateUndecided,
		},
		"段里两件、一件阻断": {
			arrange: func(fixture *departureGateFixture, _ *application.RecordMovementFactCommand) {
				fixture.customs.answers["parcel-2"] = domain.DepartureGateBlocked
			},
			want: domain.DepartureGateBlocked,
		},
		"一件调用失败、一件译不回来": {
			arrange: func(fixture *departureGateFixture, _ *application.RecordMovementFactCommand) {
				fixture.customs.errors["parcel-1"] = true
				fixture.customs.untranslatable["parcel-2"] = true
			},
			want: domain.DepartureGateUnreadable,
		},
		"没声明段": {
			arrange: func(_ *departureGateFixture, command *application.RecordMovementFactCommand) { command.Segment = "" },
			want:    domain.DepartureGateUndecided,
		},
		"段不在册": {
			arrange: func(_ *departureGateFixture, command *application.RecordMovementFactCommand) {
				command.Segment = "segment-9"
			},
			want: domain.DepartureGateUndecided,
		},
		"段册读不动": {
			arrange: func(fixture *departureGateFixture, _ *application.RecordMovementFactCommand) {
				fixture.segments.findErr = departureGateDown
			},
			want: domain.DepartureGateUnreadable,
		},
		"拟出发那一刻段里没有在控对象": {
			arrange: func(_ *departureGateFixture, command *application.RecordMovementFactCommand) {
				command.OccurredAt = movedAtFixture.Add(-2 * time.Hour)
			},
			want: domain.DepartureGateUndecided,
		},
		"班次不在册": {
			arrange: func(fixture *departureGateFixture, _ *application.RecordMovementFactCommand) {
				fixture.schedules.registered = false
			},
			want: domain.DepartureGateUndecided,
		},
		"班次册读不动": {
			arrange: func(fixture *departureGateFixture, _ *application.RecordMovementFactCommand) {
				fixture.schedules.findErr = departureGateDown
			},
			want: domain.DepartureGateUnreadable,
		},
	}
	covered := map[domain.DepartureGateAnswer]bool{}
	for name, tc := range cases {
		covered[tc.want] = true
		t.Run(name, func(t *testing.T) {
			fixture := newDepartureGateFixture(t, "parcel-1", "parcel-2")
			command := departureCommand(t)
			tc.arrange(fixture, &command)

			previewed, err := previewHandler(t, fixture).Preview(t.Context(), previewOf(command))
			if err != nil {
				t.Fatalf("问询：%v", err)
			}
			if previewed.Outcome() != application.DepartureGatePreviewed {
				t.Fatalf("outcome = %s, want DEPARTURE_GATE_PREVIEWED", previewed.Outcome())
			}
			previewReading, read := previewed.Reading()
			if !read {
				t.Fatal("问到了却没交回读数")
			}
			if previewReading.Answer() != tc.want {
				t.Fatalf("问询读出 %s，这一例要的是 %s：用例没走到它声称的那一格", previewReading.Answer(), tc.want)
			}
			previewQueries := slices.Clone(fixture.customs.queries)

			fixture.customs.queries = nil
			departed, err := fixture.reader(t).Read(t.Context(), command)
			if err != nil {
				t.Fatalf("出发读数：%v", err)
			}
			if !reflect.DeepEqual(previewReading, departed) {
				t.Fatalf("问询读数与同参出发不同：\n问询 %s %v %+v\n出发 %s %v %+v",
					previewReading.Answer(), stopWords(previewReading), previewReading.Findings(),
					departed.Answer(), stopWords(departed), departed.Findings())
			}
			if !reflect.DeepEqual(previewQueries, fixture.customs.queries) {
				t.Fatalf("问关务的问法不同：\n问询 %+v\n出发 %+v", previewQueries, fixture.customs.queries)
			}
		})
	}
	for _, answer := range []domain.DepartureGateAnswer{
		domain.DepartureGateNotGuarded, domain.DepartureGateAdmitted, domain.DepartureGateUndecided,
		domain.DepartureGateUnreadable, domain.DepartureGateBlocked,
	} {
		if !covered[answer] {
			t.Errorf("用例表里没有一例读出 %s", answer)
		}
	}
}

// Covers: AT-TF-228 编排半边——问询受理不了答未受理、不问任何一口，也不交读数；同样三格的出发在出发那一口同样问不出读数，
// 两口对同一个输入的受理一致。拟出发时点缺席不拿服务端时钟补（ADR-0023 同一条：时刻由设备给）。
func TestAPreviewThatCannotBeAcceptedAsksNothing(t *testing.T) {
	cases := map[string]func(*application.RecordMovementFactCommand){
		"缺租户":     func(command *application.RecordMovementFactCommand) { command.TenantID = domain.TenantID{} },
		"缺班次":     func(command *application.RecordMovementFactCommand) { command.Schedule = "  " },
		"段号带 NUL": func(command *application.RecordMovementFactCommand) { command.Segment = "segment\x001" },
		"缺拟出发时点":  func(command *application.RecordMovementFactCommand) { command.OccurredAt = time.Time{} },
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			fixture := newDepartureGateFixture(t, "parcel-1")
			command := departureCommand(t)
			change(&command)

			previewed, err := previewHandler(t, fixture).Preview(t.Context(), previewOf(command))
			if err != nil {
				t.Fatalf("问询：%v", err)
			}
			if previewed.Outcome() != application.DepartureGatePreviewNotAccepted {
				t.Fatalf("outcome = %s, want INPUT_NOT_ACCEPTED", previewed.Outcome())
			}
			if _, read := previewed.Reading(); read {
				t.Fatal("未受理却交回了读数")
			}
			if fixture.segments.reads != 0 || len(fixture.customs.queries) != 0 {
				t.Fatalf("未受理却问了：段册 %d 次、关务 %d 次", fixture.segments.reads, len(fixture.customs.queries))
			}
			if departed, _ := fixture.reader(t).Read(t.Context(), command); departed.Answer() != domain.DepartureGateAnswerInvalid {
				t.Fatalf("同样三格的出发却读出了 %s：两口受理不一致", departed.Answer())
			}
		})
	}
}

// 端口答非所问是实现坏了：问询与出发一样上抛，不记成读不动。缺读数编排的问询在启动时拒。
func TestAPreviewSurfacesAMisansweringViewAndRefusesAMissingReader(t *testing.T) {
	fixture := newDepartureGateFixture(t, "parcel-1")
	fixture.customs.wrong = true
	if _, err := previewHandler(t, fixture).Preview(t.Context(), previewOf(departureCommand(t))); err == nil {
		t.Fatal("答非所问被记成了一格")
	}
	if _, err := application.NewPreviewDepartureGateHandler(nil); err == nil {
		t.Fatal("缺读数编排也装得起来")
	}
}
