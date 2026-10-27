import { useId } from 'react';
import { Input } from '@idpxyz/ui-primitives';
import type { ApiResult } from '../catalogue-api';
import { listCustomerAccounts } from './api';
import { Field, PathProblems, SearchableSelect, fieldLabel, useLoaded, type PickerOption } from './PublicationFormFields';
import { versionChoiceKey, type VersionChoice } from './picker-options';

// 发布与登记表单里「引用他物」的两种格（票 party-commercial-uiux/13），与共享的 ReferencePicker（选一个对象）并列：
//   - ObjectVersionPicker：对象与版本一起选，两格同时落，不让人手拼版本号（照结算政策 ContractPicker 的做法）。
//   - SuggestInput：读口分页的册（货主客户账户）不能做成选单——选单只列得出第一页，第一页之外的人选不到。这里是能直接
//     输入的格，带上已取回那一页的候选作输入提示。
// 读不到读口时一律退回手填并说明原因；选出来的只是引用，在不在册、能不能引仍由服务端判。

const note = 'block text-[12px] text-idpxyz-breadcrumb mt-1';

/** 货主客户账户册的第一页，给输入提示用；useLoaded 要一个不随渲染变的函数，所以单独立一个。 */
export function loadCustomerAccountSuggestions() {
  return listCustomerAccounts();
}

export function ObjectVersionPicker<Body>({
  label,
  objectPath,
  versionPath,
  alsoPaths = [],
  problems,
  objectId,
  version,
  locked,
  onChange,
  load,
  choicesOf,
  emptyNote,
  readFace,
}: {
  label: string;
  objectPath: string;
  versionPath: string;
  /** 服务端也可能点名到的别的路径（整节、壳上的指名引用），问题一并显在这一格下。 */
  alsoPaths?: readonly string[];
  problems: Record<string, string[]>;
  objectId: string;
  version: string;
  locked: boolean;
  onChange: (choice: { objectId: string; version: string }) => void;
  load: () => Promise<ApiResult<Body>>;
  choicesOf: (body: Body) => VersionChoice[];
  emptyNote: string;
  readFace: string;
}) {
  const answer = useLoaded(load);
  const paths = [objectPath, versionPath, ...alsoPaths];
  const currentKey = objectId === '' && version === '' ? '' : versionChoiceKey({ objectId, version });

  if (answer?.kind === 'outcome') {
    const choices = choicesOf(answer.body);
    return (
      <label className="block">
        <span className={fieldLabel}>{label}</span>
        <SearchableSelect
          options={choices.map((choice) => ({ value: versionChoiceKey(choice), label: choice.label }))}
          value={currentKey}
          disabled={locked}
          unknownLabel={`${objectId} · ${version}（手填，不在${readFace}里）`}
          onChange={(key) => {
            const picked = choices.find((choice) => versionChoiceKey(choice) === key);
            onChange(picked ? { objectId: picked.objectId, version: picked.version } : { objectId: '', version: '' });
          }}
        />
        {choices.length === 0 ? <span className={note}>{emptyNote}</span> : null}
        <PathProblems problems={problems} paths={paths} />
      </label>
    );
  }

  return (
    <div className="flex flex-col gap-1">
      <span className={fieldLabel}>{label}</span>
      <div className="grid grid-cols-2 gap-2">
        <Input
          value={objectId}
          readOnly={locked}
          className="font-mono text-[13px]"
          placeholder="对象标识"
          aria-label={`${label}：对象标识`}
          onChange={(event) => onChange({ objectId: event.target.value, version })}
        />
        <Input
          value={version}
          readOnly={locked}
          className="font-mono text-[13px]"
          placeholder="版本号"
          aria-label={`${label}：版本号`}
          onChange={(event) => onChange({ objectId, version: event.target.value })}
        />
      </div>
      <span className={note}>{answer === null ? `正在读取${readFace}…` : `暂时读不到${readFace}，先手填对象标识与版本号。`}</span>
      <PathProblems problems={problems} paths={paths} />
    </div>
  );
}

export function SuggestInput<Body>({
  label,
  path,
  alsoPaths,
  problems,
  value,
  locked,
  onChange,
  load,
  optionsOf,
  readFace,
  placeholder,
}: {
  label: string;
  path: string;
  alsoPaths?: string[];
  problems: Record<string, string[]>;
  value: string;
  locked: boolean;
  onChange: (value: string) => void;
  load: () => Promise<ApiResult<Body>>;
  optionsOf: (body: Body) => PickerOption[];
  readFace: string;
  placeholder?: string;
}) {
  const answer = useLoaded(load);
  const listId = useId();
  const options = answer?.kind === 'outcome' ? optionsOf(answer.body) : [];
  const hint =
    answer === null
      ? `正在读取${readFace}…`
      : answer.kind !== 'outcome'
        ? `暂时读不到${readFace}，直接输入标识。`
        : options.length === 0
          ? `${readFace}里还没有内容，直接输入标识。`
          : '可以从输入提示里选，也可以直接输入标识。';
  return (
    <Field label={label} path={path} alsoPaths={alsoPaths} problems={problems}>
      <Input
        value={value}
        readOnly={locked}
        list={options.length > 0 ? listId : undefined}
        className="font-mono text-[13px]"
        placeholder={placeholder}
        onChange={(event) => onChange(event.target.value)}
      />
      {options.length > 0 ? (
        <datalist id={listId}>
          {options.map((option) => (
            <option key={option.value} value={option.value} label={option.label} />
          ))}
        </datalist>
      ) : null}
      <span className={note}>{hint}</span>
    </Field>
  );
}
