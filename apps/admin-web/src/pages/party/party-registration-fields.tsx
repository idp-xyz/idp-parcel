import { Button, Input } from '@idpxyz/ui-primitives';
import { RegistrationPanel, type RegistrationPanelProps } from '../../components/registration';
import {
  commercialRegistrationEndpoints,
  partyIdentityOutcomeLabels,
  type BusinessPartyListResponseBody,
  type CommercialRegistrationKind,
  type CustomerAccountListResponseBody,
  type GroupLegalEntityListResponseBody,
} from './api';
import { identityStatusLabels, labelOf, problemNote, registrationSnapshotHints, registrationTitles } from './presentation';
import { Field, type PickerOption } from './PublicationFormFields';

/**
 * 业务参与方页三份登记表单（票 admin-web-group-legal-entities/10）共用的几格。三份表单同一票里落地，各留一份同形
 * 副本就是票 09 评审点过的那条 Duplicated Code，所以这里各只有一份：修订号格（建议值 + 「用建议值」复位）、墙钟
 * 时刻格（datetime-local + 时区说明）、三本册的候选转写、以及折起来的 JSON 快照镜像区。
 *
 * **本文件不算摘要、不裁任何门**：修订号格只显建议、不拦手填；时刻格只收墙钟原值，换 RFC 3339 在各表单的纯逻辑里。
 * 票 02 的法人表单早于本层写成，仍各留一份同形的格与候选转写；把它切过来归收口票，本票不碰那个文件。
 */

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

// 三本册的候选转写：显名称 · 标识 · 状态，不按状态过滤——表单不裁，届时是否已生效由服务端判；状态摆出来只是让人看。
// 法人与客户账户的「名称」在参与方册上转写而来，转不到（悬空引用）时如实写出、不补占位。

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
      `${entity.partyNameKnown ? entity.partyName : '参与方册查无此身份'} · ${entity.legalEntityId} · ` +
      labelOf(identityStatusLabels, entity.status),
  }));
}

export function customerAccountPickerOptions(body: CustomerAccountListResponseBody): PickerOption[] {
  return body.accounts.map((account) => ({
    value: account.accountId,
    label:
      `${account.customerPartyNameKnown ? account.customerPartyName : '参与方册查无此身份'} · ${account.accountId} · ` +
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
