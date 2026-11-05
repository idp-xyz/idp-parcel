import { useEffect, useRef, useState, type ReactNode } from 'react';
import { Button } from '@idpxyz/ui-primitives';
import { AlertTriangle } from 'lucide-react';
import { primaryActionClass } from '../../components/primary-action';
import { statusToneBanner, statusToneText } from '../../components/status-tone';
import { TechnicalDetails } from '../../components/TechnicalDetails';
import { resetRuleSentence, templateTokenGuide } from '../../vocabulary/document-numbering';
import { currentDisplayTimeZone, formatInstant, wallTimeToRfc3339, zoneLabel } from '../moment';
import { previewDocumentNumber, registerRuleVersion } from './api';
import {
  effectiveFromProblem,
  insertToken,
  nextMonthStartWallTime,
  overlapSourceWords,
  previewViewOf,
  ruleRegistrationAnswerOf,
  templateProblem,
  tokenWithDigits,
  wallTimeIn,
  type PreviewView,
  type RegistrationAnswerView,
  type RuleRow,
} from './rows';
import { TemplateParts } from './TemplateParts';

// 任务式表单「改用新的编号格式」（ADR-0355 决定五、八）：填格式与生效时刻，边填边调预览口看样例号与重叠提示，再登记。
// 只在持登记册配置写授予（或还不知道有没有）时摆出（ADR-0315），服务端照旧每次判授予。
//
// 本地只拦「没填」「与此刻相同」「时刻写不成」；格式合不合法由预览口答（拒因与登记口同一套码）。预览说登不上时不放提交：
// 那一刻提交必拒，不如让人先看到为什么。预览答不出时照样放提交——登记口自己会判，页面不替它拦。

const fieldClass =
  'w-full rounded border border-idpxyz-border bg-idpxyz-inputBg px-2 py-1.5 type-body text-idpxyz-text focus:border-idpxyz-accent focus:outline-none';

// 等人停手再预览：每敲一个字就问一次，会把半截格式的拒因闪个不停。
const PREVIEW_DELAY_MS = 350;

/** 一段答复或预览说明：淡底横幅，正文色字，语气只落在描边与图标上。 */
function Note({ tone, children }: { tone: 'success' | 'info' | 'warning'; children: ReactNode }) {
  return (
    <div role="status" className={`rounded border px-3 py-2 type-body leading-5 ${statusToneBanner[tone]}`}>
      {children}
    </div>
  );
}

function PreviewPanel({
  view,
  loading,
  row,
  overlapConfirmed,
  onConfirmOverlap,
}: {
  view: PreviewView | null;
  loading: boolean;
  row: RuleRow;
  overlapConfirmed: boolean;
  onConfirmOverlap: (value: boolean) => void;
}) {
  if (view === null) {
    return <p className="type-body text-idpxyz-breadcrumb">{loading ? '正在预览…' : '填了格式就能看到样例号。'}</p>;
  }
  if (view.kind === 'rejected') {
    return (
      <Note tone="warning">
        <p className="font-medium text-idpxyz-textBright">这个格式登记不上</p>
        <p className="mt-0.5">{view.words}</p>
        <TechnicalDetails className="mt-1.5" rows={[{ label: '拒因码', value: view.reason }]} />
      </Note>
    );
  }
  if (view.kind === 'noZone') {
    return (
      <Note tone="info">
        这个格式可以登记，但单据编号时区未配置，算不出带日期的样例号。{view.reset.sentence ? `${view.reset.sentence}。` : ''}
      </Note>
    );
  }
  if (view.kind === 'failed') {
    return (
      <Note tone="warning">
        <p>{view.words}</p>
        <p className="mt-0.5 text-idpxyz-breadcrumb">预览不影响登记；格式合不合规，提交时登记口照样会判。</p>
        <TechnicalDetails className="mt-1.5" rows={view.technical} />
      </Note>
    );
  }
  const issuerType = row.record.dateTimeZone.source === 'ISSUER_BUSINESS_TIME_ZONE';
  return (
    <div className={`space-y-2 ${loading ? 'opacity-60' : ''}`}>
      <div className="rounded border border-idpxyz-border bg-idpxyz-sidebar px-3 py-2">
        <p className="type-caption text-idpxyz-breadcrumb">样例号</p>
        <p className="mt-0.5 break-all font-mono type-title font-semibold text-idpxyz-textBright">{view.sample}</p>
        <p className="mt-1 type-body leading-5 text-idpxyz-text">
          {formatInstant(view.at)} 开立的第一张{row.type.name}会拿到这个号；日期按{view.zone}算
          {issuerType ? '（实际按开立方的业务时区，这里只示形状）' : ''}。{view.reset.sentence ? `${view.reset.sentence}。` : ''}
        </p>
      </div>
      {view.overlaps.length > 0 ? (
        <Note tone="warning">
          <p className="flex items-center gap-1.5 font-medium text-idpxyz-textBright">
            <AlertTriangle className={`h-3.5 w-3.5 ${statusToneText.warning}`} aria-hidden="true" />
            可能与已用号段重叠
          </p>
          <ul className="mt-1 space-y-1">
            {view.overlaps.map((overlap) => (
              <li key={`${overlap.source}|${overlap.template}|${overlap.effectiveFrom ?? ''}`} className="flex flex-wrap items-center gap-1.5">
                <span>{overlapSourceWords(overlap)}</span>
                <TemplateParts template={overlap.template} />
              </li>
            ))}
          </ul>
          <p className="mt-1.5">
            新格式出的号可能和上面这些格式出过的号相同。开立时真撞上已用的号，系统会跳过、取下一个流水，跳过的号成为空号；成片撞上时这一次开立会失败，并提示回到这里改格式。需要号码连续的单据，换格式前请先确认。
          </p>
          <label className="mt-1.5 flex items-center gap-2">
            <input type="checkbox" checked={overlapConfirmed} onChange={(event) => onConfirmOverlap(event.target.checked)} />
            我看过重叠提示，仍要改用这个格式
          </label>
        </Note>
      ) : (
        <p className="type-body text-idpxyz-breadcrumb">没看出与这一类用过的格式重叠。</p>
      )}
    </div>
  );
}

export function NewFormatForm({
  row,
  readAt,
  onRegistered,
  onClose,
}: {
  row: RuleRow;
  /** 规则册这一份答复的时刻。册一重读（例如刚登了单据编号时区）预览就要重问，哪怕格式与生效时刻都没动。 */
  readAt: string;
  onRegistered: () => void;
  onClose: () => void;
}) {
  const displayZone = currentDisplayTimeZone();
  const [template, setTemplate] = useState(row.template);
  const [wallTime, setWallTime] = useState('');
  const [digits, setDigits] = useState(5);
  const [overlapConfirmed, setOverlapConfirmed] = useState(false);
  const [touched, setTouched] = useState(false);
  const [pending, setPending] = useState(false);
  const [answer, setAnswer] = useState<RegistrationAnswerView | null>(null);
  const [preview, setPreview] = useState<{ key: string; view: PreviewView } | null>(null);
  const [previewLoading, setPreviewLoading] = useState(false);
  const inputRef = useRef<HTMLInputElement>(null);
  const requestSeq = useRef(0);

  const effectiveFrom = wallTime.trim() === '' ? null : wallTimeToRfc3339(wallTime, displayZone);
  const previewKey = `${template}|${effectiveFrom ?? ''}`;

  useEffect(() => {
    if (template.trim() === '') {
      setPreview(null);
      setPreviewLoading(false);
      return;
    }
    setPreviewLoading(true);
    const seq = ++requestSeq.current;
    const timer = window.setTimeout(() => {
      void previewDocumentNumber({ documentType: row.code, template, ...(effectiveFrom ? { at: effectiveFrom } : {}) }).then((result) => {
        if (seq !== requestSeq.current) return;
        setPreview({ key: previewKey, view: previewViewOf(result) });
        setPreviewLoading(false);
      });
    }, PREVIEW_DELAY_MS);
    return () => window.clearTimeout(timer);
    // previewKey 由 template 与 effectiveFrom 拼成，二者变它才变。
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [previewKey, row.code, readAt]);

  // 换了格式就要重新确认重叠：确认的是看过的那一份提示。
  useEffect(() => setOverlapConfirmed(false), [previewKey]);

  const view = preview?.view ?? null;
  const fresh = preview?.key === previewKey;
  const problems = {
    template: templateProblem(template, row.template),
    effectiveFrom: effectiveFromProblem(wallTime, effectiveFrom),
  };
  const rejectedNow = fresh && view?.kind === 'rejected';
  const overlapUnconfirmed = fresh && view?.kind === 'sample' && view.overlaps.length > 0 && !overlapConfirmed;

  const insert = (token: string) => {
    const input = inputRef.current;
    const start = input?.selectionStart ?? template.length;
    const end = input?.selectionEnd ?? template.length;
    const next = insertToken(template, start, end, token);
    setTemplate(next.text);
    window.requestAnimationFrame(() => {
      input?.focus();
      input?.setSelectionRange(next.caret, next.caret);
    });
  };

  const submit = async () => {
    setTouched(true);
    if (problems.template !== null || problems.effectiveFrom !== null || effectiveFrom === null) return;
    if (rejectedNow || overlapUnconfirmed) return;
    setPending(true);
    const result = await registerRuleVersion({ documentType: row.code, template, effectiveFrom });
    setPending(false);
    const next = ruleRegistrationAnswerOf(result, row.type);
    setAnswer(next);
    if (next.landed) {
      // 登上了：表单留着答复与刚登的格式，不再拿「与此刻相同」之类的本地检查去红它。
      setTouched(false);
      onRegistered();
    }
  };

  return (
    <form
      className="space-y-3 rounded-md border border-idpxyz-border bg-idpxyz-bg p-3"
      aria-label="改用新的编号格式"
      onSubmit={(event) => {
        event.preventDefault();
        void submit();
      }}
    >
      <div>
        <h3 className="type-value font-semibold text-idpxyz-textBright">改用新的编号格式</h3>
        <p className="mt-0.5 type-body leading-5 text-idpxyz-breadcrumb">
          新格式只管生效之后开立的{row.type.name}；已开立的{row.type.numberName}不变。规则只增不改，想换回来就再登一版。
        </p>
      </div>

      <label className="block">
        <span className="mb-0.5 block type-caption text-idpxyz-textMuted">格式</span>
        <input
          ref={inputRef}
          className={`${fieldClass} font-mono`}
          value={template}
          spellCheck={false}
          autoComplete="off"
          onChange={(event) => {
            setTemplate(event.target.value);
            setAnswer(null);
          }}
          aria-invalid={touched && problems.template !== null}
        />
        {touched && problems.template ? <span className={`mt-0.5 block type-caption ${statusToneText.warning}`}>{problems.template}</span> : null}
      </label>
      <div className="flex flex-wrap items-center gap-1.5 type-body">
        <span className="text-idpxyz-breadcrumb">插入：</span>
        {templateTokenGuide.map((entry) => (
          <Button
            key={entry.token}
            type="button"
            size="sm"
            variant="outline"
            className="h-6 px-2 type-body"
            onClick={() => insert(tokenWithDigits(entry.token, digits))}
            title={`${entry.label}：${tokenWithDigits(entry.token, digits)}`}
          >
            {entry.token.includes(':N') ? `${digits} 位${entry.label}` : entry.label}
          </Button>
        ))}
        <label className="ml-1 inline-flex items-center gap-1 whitespace-nowrap text-idpxyz-breadcrumb">
          流水位数
          <input
            className={`${fieldClass} w-14 py-0.5`}
            type="number"
            min={1}
            max={12}
            value={digits}
            onChange={(event) => setDigits(Math.min(12, Math.max(1, Number(event.target.value) || 1)))}
          />
        </label>
      </div>
      <details className="type-body">
        <summary className="cursor-pointer select-none text-idpxyz-link">格式怎么写</summary>
        <div className="mt-1.5 space-y-1.5 leading-5 text-idpxyz-text">
          <p>花括号以外的字原样出现在号里，例如前缀「P」、连字符「-」。花括号里写下面这些部件：</p>
          <table className="w-full text-left">
            <thead className="text-idpxyz-breadcrumb">
              <tr>
                <th className="py-0.5 pr-2 font-normal">写法</th>
                <th className="py-0.5 pr-2 font-normal">是什么</th>
                <th className="py-0.5 pr-2 font-normal">例如</th>
              </tr>
            </thead>
            <tbody>
              {templateTokenGuide.map((entry) => (
                <tr key={entry.token} className="align-top">
                  <td className="py-0.5 pr-2 font-mono">{entry.token}</td>
                  <td className="py-0.5 pr-2">
                    {entry.label}
                    {entry.note ? <span className="block text-idpxyz-breadcrumb">{entry.note}</span> : null}
                  </td>
                  <td className="py-0.5 pr-2 font-mono">{entry.example}</td>
                </tr>
              ))}
            </tbody>
          </table>
          <p>{resetRuleSentence}</p>
          <p>
            按月或按日重排的格式要带上年（按日的还要带月），否则下个周期会出与上个周期相同的号，登记时会被拒。例如按月：
            <span className="font-mono">AB-{'{yyyy}{mm}'}-{'{seq:5}'}</span>。
          </p>
        </div>
      </details>

      <div className="block">
        <span className="mb-0.5 block type-caption text-idpxyz-textMuted">
          从什么时候起用（按 {zoneLabel(displayZone, new Date())} 填写）
        </span>
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
          <Button type="button" size="sm" variant="ghost" className="h-7 px-2 type-body text-idpxyz-link" onClick={() => setWallTime(wallTimeIn(new Date(), displayZone))}>
            现在
          </Button>
          <Button
            type="button"
            size="sm"
            variant="ghost"
            className="h-7 px-2 type-body text-idpxyz-link"
            onClick={() => setWallTime(nextMonthStartWallTime(new Date(), displayZone))}
          >
            下月 1 日零点
          </Button>
        </div>
        {touched && problems.effectiveFrom ? (
          <span className={`mt-0.5 block type-caption ${statusToneText.warning}`}>{problems.effectiveFrom}</span>
        ) : null}
      </div>

      <div>
        <p className="mb-1 type-caption text-idpxyz-textMuted">预览（不占号）</p>
        <PreviewPanel
          view={view}
          loading={previewLoading || !fresh}
          row={row}
          overlapConfirmed={overlapConfirmed}
          onConfirmOverlap={setOverlapConfirmed}
        />
      </div>

      {touched && overlapUnconfirmed ? (
        <p className={`type-body ${statusToneText.warning}`}>先看过重叠提示并勾选确认，再登记。</p>
      ) : null}
      <div className="flex items-center gap-2">
        <Button type="submit" size="sm" className={primaryActionClass} disabled={pending || rejectedNow}>
          {pending ? '登记中…' : '登记新格式'}
        </Button>
        <Button type="button" size="sm" variant="ghost" className="text-idpxyz-breadcrumb" onClick={onClose}>
          收起
        </Button>
      </div>
      {answer ? (
        <Note tone={answer.tone}>
          <p>{answer.words}</p>
          <TechnicalDetails className="mt-1" rows={answer.technical} />
        </Note>
      ) : null}
    </form>
  );
}
