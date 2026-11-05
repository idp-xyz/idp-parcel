import { useMemo } from 'react';
import type { ApiResult } from '../catalogue-api';
import type { LegalEntityProfileContent as ProfileContent, RegistrationNumberTypeListResponseBody } from './api';
import { registrationNumberTypeNamesOf, type RegistrationNumberTypeNameOf } from './legal-entity-identity';
import type { ProfileChangeKind, ProfileComparison, ProfileFieldChange } from './legal-entity-profile-history';
import { profileRevisionLabel } from './presentation';

// 公司资料两种表单共用的零件（票 company-subject-section-edit/02）：整张表单住在抽屉里（LegalEntityProfileSection.tsx），分区
// 表单就地住在读出卡里（LegalEntityProfileSectionEditor.tsx），保存前的差异预览、把缺项滚进视野、目录译名三件两边一样，各留一份
// 就是同一条规则定两处。本文件只摆，不裁任何门。

/** 目录答案折成查法；没答（还在读、读不到）就是查不到任何码的空表，调用方按码显。 */
export function useTypeNameOf(typeCatalogue: ApiResult<RegistrationNumberTypeListResponseBody> | null): RegistrationNumberTypeNameOf {
  return useMemo(
    () => registrationNumberTypeNamesOf(typeCatalogue?.kind === 'outcome' ? typeCatalogue.body.registrationNumberTypes : []),
    [typeCatalogue],
  );
}

/** 把第一个缺项滚进视野并聚焦它的控件：锚是 FieldAnchor 按路径挂的 data-field-path。 */
export function scrollToField(root: HTMLElement | null, path: string): void {
  // 联系人那一组在编辑态下经 portal 落在联系人分区里、不在表单容器内（票 company-subject-section-edit/04）：容器里找不到就整页找。
  const selector = `[data-field-path="${path}"]`;
  const anchor = root?.querySelector<HTMLElement>(selector) ?? document.querySelector<HTMLElement>(selector);
  if (!anchor) return;
  anchor.scrollIntoView({ block: 'center' });
  anchor.querySelector<HTMLElement>('input, textarea, select')?.focus({ preventScroll: true });
}

const changeKindLabels: Record<ProfileChangeKind, string> = { added: '新增', modified: '修改', removed: '移除', unchanged: '未变化' };
const changeKindClasses: Record<ProfileChangeKind, string> = {
  added: 'text-emerald-700 dark:text-emerald-300',
  modified: 'text-idpxyz-accent',
  removed: 'text-idpxyz-danger',
  unchanged: 'text-idpxyz-breadcrumb',
};

/**
 * 保存前的「这次改了什么」（票面第 2 项）：与当前生效版比，只列有变化的项；空白项移除旧值就是一行「移除」，不靠说明文字。
 * 比较的是将要送上去的内容（profileComparisonContentOf），来源单据与生效时间不在比较里、另起一句说。
 */
export function ChangePreview({
  comparison,
  current,
  nextRevision,
  effectiveFrom,
  timeZone,
  basis,
}: {
  comparison: ProfileComparison;
  current: ProfileContent;
  nextRevision: string;
  /** datetime-local 原值，按操作者时区读；送出时才换 RFC 3339。 */
  effectiveFrom: string;
  timeZone: string;
  /** 分区保存时把将要送的来源单据也复述一遍——它多半是自动写的，人在确认前该看到。 */
  basis?: string;
}) {
  const changed = comparison.fields.filter((field) => field.kind !== 'unchanged');
  return (
    <div className="flex flex-col gap-3" aria-label="保存前差异预览">
      <div>
        <h3 className="type-section font-semibold text-idpxyz-textBright">这次改了什么</h3>
        <p className="mt-1 type-body leading-5 text-idpxyz-breadcrumb">
          与当前生效的{profileRevisionLabel(current.revision)}相比。确认后登记为第 {nextRevision.trim() || '?'} 版，{effectiveFrom.replace('T', ' ')}（{timeZone}）起生效；
          版本连不连续、国家对不对得上由服务端判。
          {basis !== undefined ? <>来源单据：<span className="font-mono text-idpxyz-text">{basis}</span>。</> : null}
        </p>
        {comparison.contactMatchingLimited ? (
          <p role="note" className="mt-1 type-body leading-5 text-idpxyz-breadcrumb">
            资料里没有联系人的稳定标识：无法唯一对应的联系人按移除和新增列出，不推测为同一人。
          </p>
        ) : null}
      </div>
      {changed.length === 0 ? (
        <p role="status" className="rounded-md border border-idpxyz-border bg-idpxyz-sidebar px-3 py-2 type-body text-idpxyz-text">
          资料内容与{profileRevisionLabel(current.revision)}完全相同。确认后仍会登记为新的一版，只是来源单据与生效时间换成这一次的。
        </p>
      ) : (
        <div className="overflow-x-auto rounded-md border border-idpxyz-border">
          <table className="w-full min-w-[32rem] text-left type-body">
            <thead className="bg-idpxyz-sidebar text-idpxyz-breadcrumb">
              <tr>
                <th className="px-3 py-2 font-medium">项目</th>
                <th className="px-3 py-2 font-medium">{profileRevisionLabel(current.revision)}</th>
                <th className="px-3 py-2 font-medium">这一次</th>
              </tr>
            </thead>
            <tbody>
              {changed.map((field) => (
                <ChangeRow key={field.id} field={field} />
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}

function ChangeRow({ field }: { field: ProfileFieldChange }) {
  const label = field.contactKey ? `${field.subject}${field.detail ? `（${field.detail}）` : ''} · ${field.field}` : field.field;
  return (
    <tr className="border-t border-idpxyz-border align-top">
      <td className="min-w-28 px-3 py-2">
        <span className={`mr-1.5 font-medium ${changeKindClasses[field.kind]}`}>{changeKindLabels[field.kind]}</span>
        <span className="text-idpxyz-text">{label}</span>
      </td>
      <td className={`max-w-64 whitespace-pre-wrap px-3 py-2 [overflow-wrap:anywhere] ${field.before ? 'line-through decoration-idpxyz-border text-idpxyz-breadcrumb' : 'text-idpxyz-breadcrumb'}`}>
        {field.before || '—'}
      </td>
      <td className={`max-w-64 whitespace-pre-wrap px-3 py-2 [overflow-wrap:anywhere] ${field.after ? 'font-medium text-idpxyz-text' : 'text-idpxyz-breadcrumb'}`}>
        {field.after || '—'}
      </td>
    </tr>
  );
}
