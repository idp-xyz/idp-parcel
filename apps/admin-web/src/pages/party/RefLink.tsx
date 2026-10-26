import { useEffect, useState, type ReactNode } from 'react';
import type { ListColumn } from '../../templates';
import { listBusinessParties, listGroupLegalEntities } from './api';
import { emptyNameDirectory, nameDirectoryOf, refHash, refName, type NameDirectory, type RefKind, type RowRefs } from './ref-links';

// 引用画成链接（票 party-commercial-uiux/07）。去处与名称的规则在 ref-links.ts，这里只摆。

/**
 * 页面里各引用格共用的名称目录：业务参与方册与责任法人册各读一次，不逐行请求。读不到（未配置、无权、出错）就是空目录，
 * 引用格照样只显标识——名称是帮人认的，读不到不该让表格报错。
 */
export function useNameDirectory(): NameDirectory {
  const [directory, setDirectory] = useState<NameDirectory>(emptyNameDirectory);
  useEffect(() => {
    let cancelled = false;
    void Promise.all([listBusinessParties(), listGroupLegalEntities()]).then(([parties, entities]) => {
      if (cancelled) return;
      setDirectory(
        nameDirectoryOf(
          parties.kind === 'outcome' ? parties.body : null,
          entities.kind === 'outcome' ? entities.body : null,
        ),
      );
    });
    return () => {
      cancelled = true;
    };
  }, []);
  return directory;
}

/**
 * 一格引用：有名称时名称在上、标识小字在下；没有名称只显标识（`text` 给了就显 `text`，如带版本号的「合同@v1」）。
 * 点了去对象所在的页并带上检索词。阻止冒泡：引用格所在的行可能自己就能点开详情，点链接不该同时开详情。
 */
export function RefLink({
  kind,
  id,
  text,
  directory,
}: {
  kind: RefKind;
  id: string;
  text?: string;
  /** 合同、产品、规则包没有名称，可不传。 */
  directory?: NameDirectory;
}) {
  const name = directory ? refName(directory, kind, id) : null;
  return (
    <a
      href={refHash(kind, id)}
      onClick={(event) => event.stopPropagation()}
      className="text-idpxyz-accent hover:underline focus-visible:underline"
    >
      {name ? (
        <>
          <span className="block font-sans text-[13px]">{name}</span>
          <span className="block font-mono text-[11px] text-idpxyz-breadcrumb">{id}</span>
        </>
      ) : (
        <span className="font-mono">{text ?? id}</span>
      )}
    </a>
  );
}

/** 行模型列表里有引用的格换成链接，其余照原样；格里显示的字仍取行模型的那一份（可能带版本号）。 */
export function withRefLinks<Row extends { values: Readonly<Record<string, string>>; refs?: RowRefs }>(
  columns: ListColumn<Row>[],
  directory: NameDirectory,
): ListColumn<Row>[] {
  return columns.map((column) => ({
    ...column,
    render: (row: Row): ReactNode => {
      const ref = row.refs?.[column.id];
      return ref ? (
        <RefLink kind={ref.kind} id={ref.id} text={row.values[column.id]} directory={directory} />
      ) : (
        column.render(row)
      );
    },
  }));
}
