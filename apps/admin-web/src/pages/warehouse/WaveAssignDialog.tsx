import { Button, Dialog, DialogBody, DialogContent, DialogFooter, DialogHeader, DialogTitle } from '@idpxyz/ui-primitives';
import { useEffect, useState } from 'react';
import { primaryActionClass } from '../../components/primary-action';
import { statusToneBanner } from '../../components/status-tone';
import { listOperators } from '../operator-access/api';
import { noOperatorNames, operatorLabel, type OperatorNames } from '../operator-access/operator-names';
import { postWarehouseAction, warehouseActionOutcomeLabels } from './actions';
import { useCloseOnHashChange } from './use-close-on-hash';
import { readFailureMessage } from './use-detail';
import { assigneeOptions, waveAssignmentPayload, type AssigneeOption } from './wave-assign';

// 指派拣货人：从操作者册里选人，不用手抄「发行方#sub」；登了姓名的人按姓名选。读不到名册（没有查阅操作者的授予）
// 或人不在第一页时可以手填。

const fieldClass =
  'h-9 w-full rounded border border-idpxyz-border bg-idpxyz-inputBg px-2 type-body text-idpxyz-text focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-idpxyz-accent';

const manualChoice = '__manual__';

type Roster = { phase: 'loading' } | { phase: 'unread'; reason: string } | { phase: 'ready'; options: AssigneeOption[]; more: boolean };

export interface WaveAssignDialogProps {
  open: boolean;
  onClose: () => void;
  onAssigned: () => void;
  waveId: string;
  warehouse: string;
  /** 这个波次此刻派给了谁；没派是 undefined。 */
  assignee?: string;
  /** 登了姓名的操作者，给「现在派给了谁」那一句用；不给就示 sub。 */
  names?: OperatorNames;
}

export function WaveAssignDialog({ open, onClose, onAssigned, waveId, warehouse, assignee, names = noOperatorNames }: WaveAssignDialogProps) {
  useCloseOnHashChange(open, onClose);
  const [roster, setRoster] = useState<Roster>({ phase: 'loading' });
  const [choice, setChoice] = useState('');
  const [manual, setManual] = useState('');
  const [submitting, setSubmitting] = useState(false);
  const [note, setNote] = useState<{ tone: 'success' | 'danger' | 'warning'; text: string } | null>(null);

  useEffect(() => {
    if (!open) return;
    let cancelled = false;
    setRoster({ phase: 'loading' });
    setNote(null);
    setManual('');
    void listOperators().then((answer) => {
      if (cancelled) return;
      if (answer.kind !== 'outcome') {
        setRoster({ phase: 'unread', reason: readFailureMessage(answer) ?? '没读出操作者名册' });
        setChoice(manualChoice);
        setManual(assignee ?? '');
        return;
      }
      const options = assigneeOptions(answer.body.operators, warehouse);
      setRoster({ phase: 'ready', options, more: answer.body.page.next !== null });
      // 此刻的拣货人在名册里就选中他；不在（名册没列到他）就落到手填，把他的标识填上。
      if (!assignee) setChoice('');
      else if (options.some((option) => option.value === assignee)) setChoice(assignee);
      else {
        setChoice(manualChoice);
        setManual(assignee);
      }
    });
    return () => {
      cancelled = true;
    };
  }, [open, warehouse, assignee]);

  const chosen = choice === manualChoice ? manual.trim() : choice;
  const unchanged = chosen === (assignee ?? '');

  const submit = async () => {
    if (submitting) return;
    setSubmitting(true);
    const answer = await postWarehouseAction('/warehouse-fulfillment-wave-assignments', waveAssignmentPayload(waveId, chosen));
    setSubmitting(false);
    if (answer.kind !== 'outcome') {
      setNote({ tone: 'warning', text: `${readFailureMessage(answer) ?? '没有送到'}。没有改动；再点一次。` });
      return;
    }
    const outcome = answer.body.outcome;
    const done = outcome === 'ASSIGNED' || outcome === 'UNASSIGNED' || outcome === 'EXISTING_RESULT';
    setNote({
      tone: done ? 'success' : 'danger',
      text:
        outcome === 'EXISTING_RESULT'
          ? '本来就是这样，没有改动。'
          : outcome === 'STATUS_MISMATCH'
            ? '这个波次已经拣完或已取消，不能再派。'
            : (warehouseActionOutcomeLabels[outcome] ?? `后端答了一个这里不认识的结果（${outcome}），没有当成派成了。`),
    });
    if (done) onAssigned();
  };

  return (
    <Dialog
      open={open}
      onOpenChange={(next) => {
        if (!next) onClose();
      }}
    >
      <DialogContent className="flex max-h-[85vh] max-w-xl flex-col">
        <DialogHeader>
          <DialogTitle>指派拣货人 · {waveId}</DialogTitle>
        </DialogHeader>
        <DialogBody className="space-y-4 overflow-y-auto">
          <p className="type-body text-idpxyz-textMuted">
            现在：{operatorLabel(assignee, names) ?? '还没派，谁都可以拣'}。派给一个人之后，别人拣这个波次的任务后端不收；选「不派」就是取消指派，谁都可以拣、可以在工位上领。
          </p>
          {roster.phase === 'loading' ? (
            <p role="status" className="type-body text-idpxyz-textMuted">
              正在读操作者名册…
            </p>
          ) : (
            <>
              {roster.phase === 'unread' ? (
                <p className={`rounded border px-3 py-2 type-body ${statusToneBanner.warning}`}>
                  没读出操作者名册（{roster.reason}），只能手填拣货人的标识。
                </p>
              ) : (
                <label className="flex flex-col gap-1 type-body text-idpxyz-text">
                  拣货人
                  <select className={fieldClass} value={choice} disabled={submitting} onChange={(event) => setChoice(event.target.value)}>
                    <option value="">不派（谁都可以拣）</option>
                    {roster.options.map((option) => (
                      <option key={option.value} value={option.value}>
                        {option.label}
                      </option>
                    ))}
                    <option value={manualChoice}>名册里没有，手填标识…</option>
                  </select>
                  {roster.more ? <span className="type-caption text-idpxyz-textMuted">名册只列了第一页；没列出的人用「手填标识」。</span> : null}
                </label>
              )}
              {choice === manualChoice ? (
                <label className="flex flex-col gap-1 type-body text-idpxyz-text">
                  拣货人的操作者标识
                  <input
                    className={`${fieldClass} font-mono`}
                    value={manual}
                    disabled={submitting}
                    placeholder="https://issuer.example#operator-sub"
                    onChange={(event) => setManual(event.target.value)}
                  />
                  <span className="type-caption text-idpxyz-textMuted">写法是「发行方#sub」，与「放波次人」一格相同。留空是不派。</span>
                </label>
              ) : null}
            </>
          )}
          {note ? (
            <p role={note.tone === 'success' ? 'status' : 'alert'} className={`rounded border px-3 py-2 type-body ${statusToneBanner[note.tone]}`}>
              {note.text}
            </p>
          ) : null}
        </DialogBody>
        <DialogFooter>
          <Button type="button" variant="outline" onClick={onClose}>
            关闭
          </Button>
          <Button type="button" className={primaryActionClass} disabled={roster.phase === 'loading' || submitting || unchanged} onClick={() => void submit()}>
            {submitting ? '正在提交…' : chosen === '' ? '取消指派' : assignee ? '改派给他' : '派给他'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
