import { primaryReference } from './document-number';

// 单据号那一格（异常案件页主列、处置请求签与追偿签的「案件号」、客户索赔签主列共用）：有号显号；没号照旧显内部标识，
// 次要样式，悬停说为什么没有号。

export function DocumentNumberCell({
  documentNumber,
  internal,
  absence,
}: {
  documentNumber: string | undefined;
  internal: string;
  absence?: string;
}) {
  const reference = primaryReference(documentNumber, internal, absence);
  return reference.numbered ? (
    <span className="truncate whitespace-nowrap font-mono text-xs font-medium text-idpxyz-textBright" title={reference.text}>
      {reference.text}
    </span>
  ) : (
    <span className="truncate whitespace-nowrap font-mono text-[11px] text-idpxyz-breadcrumb" title={`${reference.text}（${reference.hint}）`}>
      {reference.text}
    </span>
  );
}
