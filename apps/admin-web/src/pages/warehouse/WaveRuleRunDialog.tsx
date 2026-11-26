import { Button, Dialog, DialogBody, DialogContent, DialogFooter, DialogHeader, DialogTitle } from '@idpxyz/ui-primitives';
import { useEffect, useRef, useState } from 'react';
import { primaryActionClass } from '../../components/primary-action';
import { statusToneBanner } from '../../components/status-tone';
import type { ApiResult } from '../catalogue-api';
import { getSettingsOverview, listOutboundRequests, runWaveRule } from './api';
import { DocumentLink } from './document-page';
import { useCloseOnHashChange } from './use-close-on-hash';
import { readFailureMessage } from './use-detail';
import {
  mintWaveRuleRunId,
  usableWaveRules,
  warehousesWithWaveRules,
  waveRulePreviewConditions,
  waveRulePreviewText,
  waveRuleRunNote,
  waveRuleSummary,
  type WaveRuleRecord,
  type WaveRuleRunBody,
} from './wave-rule';

// 按规则成波：选仓、选一条规则，看一眼此刻圈得到几张，点一下放出。圈单与分配都在后端；这里只摆规则说了什么、
// 点之前的估计与点之后的答复。规则在「仓配规则设置」登记，这里不改规则。

const selectClass =
  'h-9 w-full rounded border border-idpxyz-border bg-idpxyz-inputBg px-2 type-body text-idpxyz-text focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-idpxyz-accent';

type Rules = { phase: 'loading' } | { phase: 'failed'; message: string } | { phase: 'ready'; rules: readonly WaveRuleRecord[] };

export interface WaveRuleRunDialogProps {
  open: boolean;
  onClose: () => void;
  /** 放出了波次之后叫一次：列表该重读了。 */
  onReleased: () => void;
  /** 仓的名字，下拉里摆在仓编码后面；没有就只摆编码。 */
  warehouseNames?: ReadonlyMap<string, string>;
  /** 进来时先选中的仓（列表上正筛着的那个）；它没有可用的规则时不选。 */
  defaultWarehouse?: string;
}

export function WaveRuleRunDialog({ open, onClose, onReleased, warehouseNames, defaultWarehouse }: WaveRuleRunDialogProps) {
  useCloseOnHashChange(open, onClose);
  const [rules, setRules] = useState<Rules>({ phase: 'loading' });
  const [warehouse, setWarehouse] = useState('');
  const [ruleCode, setRuleCode] = useState('');
  const [total, setTotal] = useState<number | null | 'reading'>('reading');
  const [running, setRunning] = useState(false);
  const [answer, setAnswer] = useState<ApiResult<WaveRuleRunBody> | null>(null);
  // 没送到时「再送一次」用的还是这个号：同一个号只成立一次，不会放两遍。答复回来之后才换新号。
  const runId = useRef<string | null>(null);

  // 每次打开都重读规则：规则可能刚在设置页改过。
  useEffect(() => {
    if (!open) return;
    let cancelled = false;
    setRules({ phase: 'loading' });
    setAnswer(null);
    runId.current = null;
    void getSettingsOverview().then((loaded) => {
      if (cancelled) return;
      if (loaded.kind !== 'outcome') {
        setRules({ phase: 'failed', message: readFailureMessage(loaded) ?? '没读出成波规则' });
        return;
      }
      const listed = loaded.body.waveRules ?? [];
      setRules({ phase: 'ready', rules: listed });
      const warehouses = warehousesWithWaveRules(listed);
      const first = defaultWarehouse && warehouses.includes(defaultWarehouse) ? defaultWarehouse : (warehouses[0] ?? '');
      setWarehouse(first);
      setRuleCode(usableWaveRules(listed, first)[0]?.ruleCode ?? '');
    });
    return () => {
      cancelled = true;
    };
  }, [open, defaultWarehouse]);

  const listed = rules.phase === 'ready' ? rules.rules : [];
  const warehouses = warehousesWithWaveRules(listed);
  const usable = usableWaveRules(listed, warehouse);
  const rule = usable.find((candidate) => candidate.ruleCode === ruleCode);

  // 选定一条规则就问一次此刻圈得到几张；成过波之后再问一次，数会变。
  const settled = answer?.kind === 'outcome';
  useEffect(() => {
    if (!open || rule === undefined) return;
    let cancelled = false;
    setTotal('reading');
    void listOutboundRequests(waveRulePreviewConditions(rule)).then((loaded) => {
      if (!cancelled) setTotal(loaded.kind === 'outcome' ? loaded.body.page.total : null);
    });
    return () => {
      cancelled = true;
    };
    // 规则对象每次渲染都是新的引用，按它的身份与修订订阅
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, rule?.warehouseCode, rule?.ruleCode, rule?.revision, settled]);

  const run = async () => {
    if (rule === undefined || running) return;
    if (runId.current === null) runId.current = mintWaveRuleRunId(rule.ruleCode);
    setRunning(true);
    const answered = await runWaveRule(rule.warehouseCode, rule.ruleCode, runId.current);
    setRunning(false);
    setAnswer(answered);
    if (answered.kind !== 'outcome') return;
    runId.current = null;
    if (waveRuleRunNote(answered.body).released) onReleased();
  };

  const choose = (nextWarehouse: string, nextRule: string) => {
    setWarehouse(nextWarehouse);
    setRuleCode(nextRule);
    setAnswer(null);
    runId.current = null;
  };

  const note = answer?.kind === 'outcome' ? waveRuleRunNote(answer.body) : null;
  const unsent = answer !== null && answer.kind !== 'outcome' ? (readFailureMessage(answer) ?? '没有送到') : null;
  const waves = answer?.kind === 'outcome' ? (answer.body.waves ?? []) : [];

  return (
    <Dialog
      open={open}
      onOpenChange={(next) => {
        if (!next) onClose();
      }}
    >
      <DialogContent className="flex max-h-[85vh] max-w-2xl flex-col">
        <DialogHeader>
          <DialogTitle>按规则成波</DialogTitle>
        </DialogHeader>
        <DialogBody className="space-y-4 overflow-y-auto">
          <p className="type-body text-idpxyz-textMuted">
            照一条成波规则，把这个仓里合条件的已预占的出库单按受理先后放成几个波次。分配不出的单会跳过、留在已预占，不挡别的单。
          </p>
          {rules.phase === 'loading' ? (
            <p role="status" className="type-body text-idpxyz-textMuted">
              正在读成波规则…
            </p>
          ) : rules.phase === 'failed' ? (
            <p role="alert" className={`rounded border px-3 py-2 type-body ${statusToneBanner.danger}`}>
              {rules.message}
            </p>
          ) : warehouses.length === 0 ? (
            <p className={`rounded border px-3 py-2 type-body ${statusToneBanner.warning}`}>
              未配置：还没有可用的成波规则。规则由你们按仓登记——圈哪些单、一个波次最多几张、怎么拣——到
              <a className="mx-1 whitespace-nowrap text-idpxyz-accent underline" href="#/warehouse-settings">
                仓配规则设置
              </a>
              的「成波规则」登一条。没有规则时照旧可以在出库单列表勾单放波次。
            </p>
          ) : (
            <>
              <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
                <label className="flex flex-col gap-1 type-body text-idpxyz-text">
                  仓
                  <select
                    className={selectClass}
                    value={warehouse}
                    disabled={running}
                    onChange={(event) => choose(event.target.value, usableWaveRules(listed, event.target.value)[0]?.ruleCode ?? '')}
                  >
                    {warehouses.map((code) => (
                      <option key={code} value={code}>
                        {warehouseNames?.get(code) ? `${code} · ${warehouseNames.get(code)}` : code}
                      </option>
                    ))}
                  </select>
                </label>
                <label className="flex flex-col gap-1 type-body text-idpxyz-text">
                  成波规则
                  <select className={selectClass} value={ruleCode} disabled={running} onChange={(event) => choose(warehouse, event.target.value)}>
                    {usable.map((candidate) => (
                      <option key={candidate.ruleCode} value={candidate.ruleCode}>
                        {candidate.name}（{candidate.ruleCode}）
                      </option>
                    ))}
                  </select>
                </label>
              </div>
              {rule ? (
                <div className="space-y-1 rounded border border-idpxyz-border px-3 py-2">
                  <p className="type-body text-idpxyz-text">{waveRuleSummary(rule)}。</p>
                  <p className="type-caption text-idpxyz-textMuted">第 {rule.revision} 版</p>
                  <p role="status" className="type-body text-idpxyz-textMuted">
                    {total === 'reading' ? '正在看此刻圈得到几张…' : waveRulePreviewText(rule, total)}
                  </p>
                </div>
              ) : null}
            </>
          )}
          {note ? (
            <div role={note.tone === 'success' ? 'status' : 'alert'} className={`space-y-2 rounded border px-3 py-2 type-body ${statusToneBanner[note.tone]}`}>
              <p>{note.text}</p>
              {waves.length > 0 ? (
                <ul className="flex flex-wrap gap-x-4 gap-y-1">
                  {waves.map((wave) => (
                    <li key={wave.waveId} className="whitespace-nowrap">
                      <DocumentLink moduleId="warehouse-waves" parts={[wave.waveId]} />
                      <span className="ml-1 type-caption text-idpxyz-textMuted">{wave.requests} 张</span>
                    </li>
                  ))}
                </ul>
              ) : null}
            </div>
          ) : null}
          {unsent ? (
            <p role="alert" className={`rounded border px-3 py-2 type-body ${statusToneBanner.warning}`}>
              {unsent}。这一次有没有成立还不知道；点「再送一次」用的是同一个执行号，不会放两遍。
            </p>
          ) : null}
        </DialogBody>
        <DialogFooter>
          <Button type="button" variant="outline" onClick={onClose}>
            关闭
          </Button>
          <Button type="button" className={primaryActionClass} disabled={rule === undefined || running} onClick={() => void run()}>
            {running ? '正在成波…' : unsent ? '再送一次' : note ? '再成一次' : '按这条规则成波'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
