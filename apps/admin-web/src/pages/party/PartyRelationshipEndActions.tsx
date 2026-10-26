import { useState } from 'react';
import { Button, ConfirmDialog, Input } from '@idpxyz/ui-primitives';
import { confirmDialogFooter } from '../../components/confirm-dialog-footer';
import { primaryActionClass } from '../../components/primary-action';
import { RegistrationAnswerNote, type RegistrationPanelState } from '../../components/registration';
import { moduleInfoById } from '../../navigation';
import { currentDisplayTimeZone } from '../moment';
import {
  endPartyRelationship,
  partyRelationshipEndingEndpoints,
  partyRelationshipEndingOutcomeLabels,
  type PartyRelationshipEndingAction,
  type PartyRelationshipRecord,
} from './api';
import { problemNote } from './presentation';
import { Field } from './PublicationFormFields';
import { WallTimeField } from './party-registration-fields';
import {
  endingAvailability,
  endingConfirmLines,
  endingConfirmMessage,
  endingLocalProblems,
  endingPayloadOf,
  initialEndingDraft,
  partyRelationshipEndingActionLabels,
  type RelationshipEndingDraft,
} from './party-relationship-end-form';

/**
 * 关系详情抽屉头部的「撤销」「到期」两个动作（票 ps-external-mark-relations/17）。替代只开写口与受控批量口，界面另议。
 *
 * 次要按钮，撤销用危险样式；关系不在`已生效`（到期另要区间有终点）时禁用并写明原因。点开后在头部下方填依据与时刻（按本地
 * 墙钟填，送出换 RFC 3339），再进确认框：复述关系一句话、生效边界（本地 + UTC）、依据与落点修订，写明只影响边界之后的新
 * 决定、不可撤回。答 `ENDED` 时回调页面重取关系册——重取保留旧答案（register-list.ts），表格不回骨架，抽屉里这一行就地
 * 换成终止后的那一笔。
 *
 * **本组件不裁任何门**：能不能结束由服务端按册面答，答复原样摆在按钮下方（RegistrationAnswerNote）。纯逻辑在
 * party-relationship-end-form.ts。
 */
const info = moduleInfoById['business-parties'];
const actions: PartyRelationshipEndingAction[] = ['revocation', 'expiration'];

export function PartyRelationshipEndActions({
  row,
  onEnded,
}: {
  row: PartyRelationshipRecord;
  /** 答 `ENDED` 时调用，页面借它重取关系册。 */
  onEnded: () => void;
}) {
  const [open, setOpen] = useState<PartyRelationshipEndingAction | null>(null);
  const [draft, setDraft] = useState<RelationshipEndingDraft>({ basis: '', at: '' });
  const [confirming, setConfirming] = useState(false);
  const [state, setState] = useState<RegistrationPanelState>({ kind: 'idle' });
  const timeZone = currentDisplayTimeZone();
  const problems = open === null ? {} : endingLocalProblems(draft, timeZone);
  const locked = state.kind === 'submitting';
  const unavailable = actions.flatMap((action) => {
    const availability = endingAvailability(row, action);
    return availability.available ? [] : [{ action, reason: availability.reason }];
  });

  const start = (action: PartyRelationshipEndingAction) => {
    setOpen(action);
    setDraft(initialEndingDraft(row, action, new Date(), timeZone));
    setState({ kind: 'idle' });
  };

  const submit = async () => {
    if (open === null) return;
    setState({ kind: 'submitting' });
    const answer = await endPartyRelationship(open, endingPayloadOf(row, open, draft, timeZone));
    setConfirming(false);
    setState({ kind: 'answered', answer });
    if (answer.kind === 'outcome' && answer.body.outcome === 'ENDED') {
      setOpen(null);
      onEnded();
    }
  };

  return (
    <div className="mt-2 flex flex-col gap-2">
      <div className="flex flex-wrap items-center gap-2">
        {actions.map((action) => {
          const availability = endingAvailability(row, action);
          return (
            <Button
              key={action}
              variant="outline"
              size="sm"
              className={`text-[12px] ${action === 'revocation' ? 'text-idpxyz-danger' : 'text-idpxyz-breadcrumb'}`}
              disabled={!availability.available || locked}
              title={availability.available ? undefined : availability.reason}
              onClick={() => start(action)}
            >
              {partyRelationshipEndingActionLabels[action]}
            </Button>
          );
        })}
      </div>
      {unavailable.length > 0 ? (
        <ul className="text-[12px] text-idpxyz-textMuted">
          {unavailable.map(({ action, reason }) => (
            <li key={action}>
              {partyRelationshipEndingActionLabels[action]}不可用：{reason}
            </li>
          ))}
        </ul>
      ) : null}

      {open !== null ? (
        <div className="rounded border border-idpxyz-border p-3 flex flex-col gap-3">
          <p className="text-[12px] text-idpxyz-textMuted">
            {open === 'revocation'
              ? '撤销即时结束这段关系：终止修订一落册，它就不再支持新的商业决定。时刻不得晚于此刻。'
              : '登记这段关系已到期：到期时刻不早于有效区间终点，也不晚于此刻；终点未到的关系由服务端拒。'}
            提交打到 <span className="font-mono">POST {partyRelationshipEndingEndpoints[open]}</span>；租户不在表单上。
          </p>
          <Field label={`${partyRelationshipEndingActionLabels[open]}依据 *`} path="basis" problems={problems}>
            <Input
              value={draft.basis}
              readOnly={locked}
              className="font-mono text-[13px]"
              placeholder={`${partyRelationshipEndingActionLabels[open]}依据引用`}
              onChange={(event) => setDraft((current) => ({ ...current, basis: event.target.value }))}
            />
          </Field>
          <WallTimeField
            label={`${open === 'revocation' ? '撤销' : '到期'}时刻 *`}
            path="at"
            problems={problems}
            value={draft.at}
            locked={locked}
            timeZone={timeZone}
            onChange={(at) => setDraft((current) => ({ ...current, at }))}
          />
          <div className="flex items-center gap-2">
            <Button
              variant="default"
              size="sm"
              className={`${primaryActionClass} text-[12px]`}
              disabled={locked || Object.keys(problems).length > 0}
              onClick={() => setConfirming(true)}
            >
              核对并{partyRelationshipEndingActionLabels[open]}
            </Button>
            <Button
              variant="outline"
              size="sm"
              className="text-[12px] text-idpxyz-breadcrumb"
              disabled={locked}
              onClick={() => setOpen(null)}
            >
              不{partyRelationshipEndingActionLabels[open]}
            </Button>
          </div>
        </div>
      ) : null}

      <RegistrationAnswerNote
        state={state}
        owner={info.owner}
        outcomeLabels={partyRelationshipEndingOutcomeLabels}
        problemNote={problemNote}
      />

      {open !== null ? (
        <ConfirmDialog
          renderFooter={confirmDialogFooter}
          open={confirming}
          tone={open === 'revocation' ? 'danger' : 'warning'}
          title={`${partyRelationshipEndingActionLabels[open]}这段参与方关系？`}
          message={endingConfirmMessage(open)}
          extraContent={
            <ul className="text-[12px] text-idpxyz-text flex flex-col gap-1">
              {endingConfirmLines(row, open, draft, timeZone).map((line) => (
                <li key={line}>{line}</li>
              ))}
            </ul>
          }
          confirmLabel={`确认${partyRelationshipEndingActionLabels[open]}`}
          cancelLabel="再想想"
          onConfirm={submit}
          onCancel={() => setConfirming(false)}
        />
      ) : null}
    </div>
  );
}
