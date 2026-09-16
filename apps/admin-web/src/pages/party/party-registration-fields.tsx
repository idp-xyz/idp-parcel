import { useState } from 'react';
import { Button, Input } from '@idpxyz/ui-primitives';
import {
  RegistrationPanel,
  type RegistrationPanelProps,
  type RegistrationPanelState,
} from '../../components/registration';
import { currentDisplayTimeZone } from '../moment';
import {
  commercialRegistrationEndpoints,
  partyIdentityOutcomeLabels,
  registerCommercial,
  type BusinessPartyListResponseBody,
  type CommercialRegistrationKind,
  type CustomerAccountListResponseBody,
  type GroupLegalEntityListResponseBody,
} from './api';
import {
  identityStatusLabels,
  labelOf,
  partyNameUnknownNote,
  problemNote,
  registrationSnapshotHints,
  registrationTitles,
} from './presentation';
import { Field, type PickerOption } from './PublicationFormFields';
import { effectiveDraftOf, registrationLanded, type RevisionedDraft } from './registration-form';

/**
 * 参与方模块各登记表单（票 admin-web-group-legal-entities/02 的法人表单与票 10 的三份）共用的壳与格。各表单各留一份
 * 同形副本就是票 09 / 10 评审点过的 Duplicated Code，所以这里各只有一份：表单壳（草稿、建议值顶格、锁定、提交、
 * 落地回调）、修订号格（建议值 + 「用建议值」复位）、墙钟时刻格（datetime-local + 时区说明）、各册的候选转写、以及
 * 折起来的 JSON 快照镜像区。
 *
 * **本文件不算摘要、不裁任何门**：修订号格只显建议、不拦手填；时刻格只收墙钟原值，换 RFC 3339 在各表单的纯逻辑里；
 * 壳只拦各表单自己报的编码层问题，领域规则一律送上去让服务端答。
 */

/** 一份登记表单要交给壳的：本册的形状与规则全在各自的 *-form.ts，壳不知道任何一格叫什么。 */
export interface RegistrationFormSpec<Draft extends RevisionedDraft> {
  /** 这一口的种类，决定端点。 */
  kind: CommercialRegistrationKind;
  empty: () => Draft;
  /** 修订号建议值：按草稿里的标识到已取回的册上数；闭包持有那份册（页面传进来的或表单自己读的）。 */
  suggestion: (draft: Draft) => number;
  /** 本地能判的编码层问题，按 JSON 路径归组；非空即不送。 */
  localProblems: (draft: Draft, timeZone: string) => Record<string, string[]>;
  payloadOf: (draft: Draft, timeZone: string) => unknown;
  /** 这一口「已落册」的线上名（REGISTERED / DEACTIVATED）；答别的都不算落地。 */
  landedOutcome: string;
  /**
   * 落地回调，页面借它重取读签。逐字段那条路把送出的草稿一并交回；JSON 镜像那条路交 null——快照是未译的 JSON，
   * 壳不解它，分不出送的是什么。
   */
  onLanded: (sent: Draft | null) => void;
}

/**
 * 登记表单的壳：草稿与修订号建议、编码层问题、提交锁定、两条路（逐字段 / JSON 镜像）共用的一次提交。
 * 建议值何时顶进草稿、答什么才算落地，规则在 registration-form.ts（node:test 钉着），这里只接线。
 *
 * `submit` 给 JSON 镜像区，与 `send` 打同一个端点、走同一条落地判据——两条路答落册时都要触发读签重取。
 */
export function useRegistrationForm<Draft extends RevisionedDraft>(spec: RegistrationFormSpec<Draft>) {
  const [draft, setDraft] = useState<Draft>(spec.empty);
  // 修订号建议随标识变；操作者一改过就不再跟着建议走，直到点「用建议值」复位。
  const [revisionEdited, setRevisionEdited] = useState(false);
  const [state, setState] = useState<RegistrationPanelState>({ kind: 'idle' });

  const timeZone = currentDisplayTimeZone();
  const suggestion = spec.suggestion(draft);
  const effectiveDraft = effectiveDraftOf(draft, revisionEdited, suggestion);
  const problems = spec.localProblems(effectiveDraft, timeZone);
  const locked = state.kind === 'submitting';
  const canSend = Object.keys(problems).length === 0;
  const patch = (change: Partial<Draft>) => setDraft((current) => ({ ...current, ...change }));

  const submit = (snapshot: unknown, sent: Draft | null = null) =>
    registerCommercial(spec.kind, snapshot).then((answer) => {
      if (registrationLanded(answer, spec.landedOutcome)) spec.onLanded(sent);
      return answer;
    });

  const send = () => {
    if (!canSend) return;
    setState({ kind: 'submitting' });
    void submit(spec.payloadOf(effectiveDraft, timeZone), effectiveDraft).then((answer) =>
      setState({ kind: 'answered', answer }),
    );
  };

  /** RevisionField 要的那几件，表单只需再给路径、问题表与格下那句。 */
  const revisionField = {
    value: effectiveDraft.revision,
    suggestion,
    edited: revisionEdited,
    locked,
    onChange: (revision: string) => {
      setRevisionEdited(true);
      setDraft((current) => ({ ...current, revision }));
    },
    onUseSuggestion: () => setRevisionEdited(false),
  };

  return { draft, effectiveDraft, patch, problems, canSend, locked, state, timeZone, submit, send, revisionField };
}

/**
 * 修订号一格：显示的是「建议值」或操作者改过的值；改过就不再跟着建议走，直到点「用建议值」复位。建议只是省一次
 * 翻册，连续性（登记）或错位（停用）仍由服务端按册面判——这一句就显在格下，不让人把建议读成校验。
 */
export function RevisionField({
  path,
  problems,
  value,
  suggestion,
  edited,
  locked,
  onChange,
  onUseSuggestion,
  note,
}: {
  path: string;
  problems: Record<string, string[]>;
  value: string;
  suggestion: number;
  edited: boolean;
  locked: boolean;
  onChange: (value: string) => void;
  onUseSuggestion: () => void;
  note: string;
}) {
  return (
    <Field label="修订号 *" path={path} problems={problems}>
      <div className="flex items-center gap-2">
        <Input
          value={value}
          readOnly={locked}
          inputMode="numeric"
          className="font-mono text-[13px]"
          onChange={(event) => onChange(event.target.value)}
        />
        {edited ? (
          <Button variant="outline" size="sm" disabled={locked} onClick={onUseSuggestion}>
            用建议值 {suggestion}
          </Button>
        ) : null}
      </div>
      <span className="block text-[11px] text-idpxyz-textMuted mt-1">{note}</span>
    </Field>
  );
}

/** 墙钟时刻一格：按操作者本地时刻填，送出时由纯逻辑换成 RFC 3339 UTC；这一句显在格下，免得有人按 UTC 填。 */
export function WallTimeField({
  label,
  path,
  problems,
  value,
  locked,
  timeZone,
  onChange,
  note,
}: {
  label: string;
  path: string;
  problems: Record<string, string[]>;
  value: string;
  locked: boolean;
  timeZone: string;
  onChange: (value: string) => void;
  /** 时区那句之前再加的一句；不给就只显时区那句。 */
  note?: string;
}) {
  return (
    <Field label={label} path={path} problems={problems}>
      <Input
        type="datetime-local"
        step={1}
        value={value}
        readOnly={locked}
        className="font-mono text-[13px]"
        onChange={(event) => onChange(event.target.value)}
      />
      <span className="block text-[11px] text-idpxyz-textMuted mt-1">
        {note ? `${note} ` : ''}按操作者本地时刻（{timeZone}）填，送出时换成 RFC 3339 UTC。
      </span>
    </Field>
  );
}

// 各册的候选转写：显名称 · 标识 · 状态，不按状态过滤——表单不裁，届时是否已生效由服务端判；状态摆出来只是让人看。
// 法人与客户账户的「名称」在参与方册上转写而来，转不到（悬空引用）时如实写出、不补占位（partyNameUnknownNote）。

export function businessPartyPickerOptions(body: BusinessPartyListResponseBody): PickerOption[] {
  return body.parties.map((party) => ({
    value: party.partyId,
    label: `${party.partyName} · ${party.partyId} · ${labelOf(identityStatusLabels, party.status)}`,
  }));
}

export function legalEntityPickerOptions(body: GroupLegalEntityListResponseBody): PickerOption[] {
  return body.entities.map((entity) => ({
    value: entity.legalEntityId,
    label:
      `${entity.partyNameKnown ? entity.partyName : partyNameUnknownNote} · ${entity.legalEntityId} · ` +
      labelOf(identityStatusLabels, entity.status),
  }));
}

export function customerAccountPickerOptions(body: CustomerAccountListResponseBody): PickerOption[] {
  return body.accounts.map((account) => ({
    value: account.accountId,
    label:
      `${account.customerPartyNameKnown ? account.customerPartyName : partyNameUnknownNote} · ${account.accountId} · ` +
      labelOf(identityStatusLabels, account.status),
  }));
}

/**
 * JSON 快照签降成的折叠区（票 10 第 4 条，做法同票 02）：受控批量口的在线镜像（ADR-0101 决定一），不是运营配置员
 * 的主路径，但仍要在——CLI 那份形状能原样粘进来核对，是两口锁到同一登记用例的可见证据。提示句取 registrationSnapshotHints
 * 里票 08 改过的那份（已写明不带 tenantId）。`submit` 与逐字段表单共用同一个：两条路答 REGISTERED / DEACTIVATED 时
 * 都要触发读签重取。
 */
export function SnapshotJsonDetails({
  kind,
  submit,
}: {
  kind: CommercialRegistrationKind;
  submit: RegistrationPanelProps['submit'];
}) {
  return (
    <details className="rounded border border-idpxyz-border">
      <summary className="cursor-pointer select-none px-4 py-2 text-[13px] text-idpxyz-textMuted">
        高级：粘贴登记快照 JSON（受控批量口 parcel-commercial 的在线镜像）
      </summary>
      <RegistrationPanel
        moduleId="business-parties"
        title={registrationTitles[kind]}
        endpoint={`POST ${commercialRegistrationEndpoints[kind]}`}
        snapshotHint={registrationSnapshotHints[kind]}
        submit={submit}
        outcomeLabels={partyIdentityOutcomeLabels}
        problemNote={problemNote}
      />
    </details>
  );
}
