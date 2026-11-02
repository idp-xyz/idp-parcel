import { useMemo, useState, type ReactNode } from 'react';
import { X } from 'lucide-react';
import {
  Button,
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@idpxyz/ui-primitives';
import { primaryActionClass } from '../../components/primary-action';
import { StatusPill } from '../../components/StatusPill';
import { statusToneBanner, statusToneText } from '../../components/status-tone';
import { TechnicalDetails } from '../../components/TechnicalDetails';
import { currentDisplayTimeZone, formatInstant, wallTimeToRfc3339, zoneLabel, zoneOffsetMinutes } from '../moment';
import { registerNumberingTimeZone, type DocumentNumberingReadBody } from './api';
import {
  effectiveFromProblem,
  numberingZoneStateOf,
  registrantWords,
  timeZoneHistoryOf,
  timeZoneOptionsOf,
  timeZoneRegistrationAnswerOf,
  wallTimeIn,
  zoneWords,
  type RegistrationAnswerView,
} from './rows';

// 单据编号时区（ADR-0355 决定七）：此刻是哪个、历次登记，以及登记一版的表单。时区是租户取值、产品不给默认：候选里
// 不预选任何一项，「用这台电脑的时区」只是替人把那一项挑出来，仍要人自己点登记。登记表单按登记册配置写授予收起（ADR-0315）。

const fieldClass =
  'w-full rounded border border-idpxyz-border bg-idpxyz-inputBg px-2 py-1.5 text-[12px] text-idpxyz-text focus:border-idpxyz-accent focus:outline-none';

/** 运行环境认得的 IANA 时区名；老环境没有这项能力时只剩裸 UTC 与这台电脑所在的区，仍选得出来。 */
function supportedZones(browserZone: string): string[] {
  const intl = Intl as unknown as { supportedValuesOf?: (key: string) => string[] };
  try {
    const zones = intl.supportedValuesOf?.('timeZone');
    if (zones && zones.length > 0) return zones.includes(browserZone) ? zones : [...zones, browserZone];
  } catch {
    // 落到下面的最小集合。
  }
  return [browserZone];
}

function Section({ title, children }: { title: string; children: ReactNode }) {
  return (
    <section>
      <h3 className="text-[13px] font-semibold text-idpxyz-textBright">{title}</h3>
      <div className="mt-1.5">{children}</div>
    </section>
  );
}

function RegistrationForm({ onRegistered }: { onRegistered: () => void }) {
  const displayZone = currentDisplayTimeZone();
  const browserZone = useMemo(() => Intl.DateTimeFormat().resolvedOptions().timeZone, []);
  const options = useMemo(() => {
    const now = new Date();
    return timeZoneOptionsOf(
      supportedZones(browserZone),
      (zone) => zoneLabel(zone, now),
      (zone) => zoneOffsetMinutes(now, zone),
    );
  }, [browserZone]);
  const [zone, setZone] = useState('');
  const [wallTime, setWallTime] = useState('');
  const [touched, setTouched] = useState(false);
  const [pending, setPending] = useState(false);
  const [answer, setAnswer] = useState<RegistrationAnswerView | null>(null);

  const effectiveFrom = wallTime.trim() === '' ? null : wallTimeToRfc3339(wallTime, displayZone);
  const problems = {
    zone: zone === '' ? '选一个时区' : null,
    effectiveFrom: effectiveFromProblem(wallTime, effectiveFrom),
  };

  const submit = async () => {
    setTouched(true);
    if (problems.zone !== null || problems.effectiveFrom !== null || effectiveFrom === null) return;
    setPending(true);
    const result = await registerNumberingTimeZone({ timeZone: zone, effectiveFrom });
    setPending(false);
    const next = timeZoneRegistrationAnswerOf(result);
    setAnswer(next);
    if (next.landed) {
      setTouched(false);
      onRegistered();
    }
  };

  return (
    <form
      className="space-y-3"
      aria-label="登记单据编号时区"
      onSubmit={(event) => {
        event.preventDefault();
        void submit();
      }}
    >
      <label className="block">
        <span className="mb-0.5 block text-[11px] text-idpxyz-textMuted">时区</span>
        <select
          className={fieldClass}
          value={zone}
          onChange={(event) => {
            setZone(event.target.value);
            setAnswer(null);
          }}
          aria-invalid={touched && problems.zone !== null}
        >
          <option value="">选一个时区</option>
          {options.map((option) => (
            <option key={option.zone} value={option.zone}>
              {option.label}
            </option>
          ))}
        </select>
        <span className="mt-1 flex flex-wrap items-center gap-1 text-[12px] text-idpxyz-breadcrumb">
          <Button type="button" size="sm" variant="ghost" className="h-6 px-1.5 text-[12px] text-idpxyz-link" onClick={() => setZone(browserZone)}>
            用这台电脑的时区（{zoneWords(browserZone)}）
          </Button>
        </span>
        {touched && problems.zone ? <span className={`mt-0.5 block text-[11px] ${statusToneText.warning}`}>{problems.zone}</span> : null}
      </label>
      <div>
        <span className="mb-0.5 block text-[11px] text-idpxyz-textMuted">从什么时候起用（按 {zoneLabel(displayZone, new Date())} 填写）</span>
        <div className="flex flex-wrap items-center gap-1.5">
          <input
            className={`${fieldClass} w-52`}
            type="datetime-local"
            value={wallTime}
            onChange={(event) => {
              setWallTime(event.target.value);
              setAnswer(null);
            }}
            aria-label="从什么时候起用"
            aria-invalid={touched && problems.effectiveFrom !== null}
          />
          <Button type="button" size="sm" variant="ghost" className="h-7 px-2 text-[12px] text-idpxyz-link" onClick={() => setWallTime(wallTimeIn(new Date(), displayZone))}>
            现在
          </Button>
        </div>
        {touched && problems.effectiveFrom ? (
          <span className={`mt-0.5 block text-[11px] ${statusToneText.warning}`}>{problems.effectiveFrom}</span>
        ) : null}
      </div>
      <p className="text-[12px] leading-5 text-idpxyz-breadcrumb">
        登记之后开立的单据才按它出日期；此前开立、没带单据号的单据不补号。时区只增不改，要换就再登一版。
      </p>
      <Button type="submit" size="sm" className={primaryActionClass} disabled={pending}>
        {pending ? '登记中…' : '登记单据编号时区'}
      </Button>
      {answer ? (
        <div role="status" className={`rounded border px-3 py-2 text-[12px] leading-5 ${statusToneBanner[answer.tone]}`}>
          <p>{answer.words}</p>
          <TechnicalDetails className="mt-1" rows={answer.technical} />
        </div>
      ) : null}
    </form>
  );
}

export function TimeZoneDialog({
  open,
  onOpenChange,
  body,
  mayWrite,
  onRegistered,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  body: DocumentNumberingReadBody;
  mayWrite: boolean;
  onRegistered: () => void;
}) {
  const state = numberingZoneStateOf(body);
  const history = timeZoneHistoryOf(body.numberingTimeZoneVersions, body.at);

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent size="lg" className="max-h-[85vh] overflow-y-auto">
        <DialogHeader className="items-start">
          <div>
            <DialogTitle>单据编号时区</DialogTitle>
            <DialogDescription className="mt-1">
              运单、异常案件这类开立方没有业务时区的单据，单据号里的日期按它算；网点、结算账户开立的单据按自己的业务时区，没登时也退到它。
            </DialogDescription>
          </div>
          <Button type="button" variant="ghost" size="icon" aria-label="关闭" onClick={() => onOpenChange(false)}>
            <X className="h-4 w-4" aria-hidden="true" />
          </Button>
        </DialogHeader>
        <DialogBody className="space-y-4 text-[12px] leading-5 text-idpxyz-text">
          <Section title="此刻">
            {state.configured ? (
              <p>
                <span className="font-semibold text-idpxyz-textBright" title={state.currentZone}>
                  {state.current}
                </span>
                {body.numberingTimeZone.effectiveFrom ? `，${formatInstant(body.numberingTimeZone.effectiveFrom)} 起` : ''}
              </p>
            ) : (
              <p className={`rounded border px-3 py-2 ${statusToneBanner.warning}`}>
                单据编号时区未配置——开立方没有业务时区的单据（如运单、异常案件）暂不带单据号。
                {state.upcoming ? `已登记 ${formatInstant(state.upcoming.effectiveFrom)} 起用${zoneWords(state.upcoming.zone)}，到时自动生效。` : ''}
              </p>
            )}
          </Section>
          <Section title="历次登记">
            {history.length === 0 ? (
              <p className="text-idpxyz-breadcrumb">还没有登记过。产品不替公司选时区，要由本公司登记。</p>
            ) : (
              <ol className="space-y-1.5">
                {history.map((entry) => (
                  <li key={entry.key} className="flex flex-wrap items-center gap-2 rounded border border-idpxyz-border px-3 py-1.5">
                    <StatusPill code={entry.standing} />
                    <span className="font-medium text-idpxyz-textBright" title={entry.timeZone}>
                      {zoneWords(entry.timeZone)}
                    </span>
                    <span>{formatInstant(entry.effectiveFrom)} 起</span>
                    <span className="text-idpxyz-breadcrumb">
                      <span title={entry.registeredBy}>{registrantWords(entry.registeredBy)}</span> 登记于 {formatInstant(entry.registeredAt)}
                    </span>
                  </li>
                ))}
              </ol>
            )}
            {history.length > 0 ? (
              <TechnicalDetails
                className="mt-1.5"
                rows={history.map((entry) => ({ label: `${formatInstant(entry.effectiveFrom)} 起`, value: entry.timeZone }))}
              />
            ) : null}
          </Section>
          {mayWrite ? (
            <Section title="登记一版">
              <RegistrationForm onRegistered={onRegistered} />
            </Section>
          ) : (
            <p className="text-idpxyz-breadcrumb">登记单据编号时区要登记册配置写授予，需要时请联系管理员。</p>
          )}
        </DialogBody>
        <DialogFooter>
          <Button type="button" variant="ghost" onClick={() => onOpenChange(false)}>
            关闭
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
