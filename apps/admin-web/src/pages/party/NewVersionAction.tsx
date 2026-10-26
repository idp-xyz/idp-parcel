import { Button } from '@idpxyz/ui-primitives';
import type { ListColumn } from '../../templates';

// 目录行上的「发新版本」按钮列（票 party-commercial-uiux/14）。点了把这一版带去发布表单预填；按钮不吞行点击——表格行
// 若自己能点开详情，点按钮不该同时开详情。

export function withNewVersionAction<Row>(
  columns: ListColumn<Row>[],
  options: {
    /** 这一行能不能发新版本；不能就不画按钮（如随所属版本发布的两本册）。 */
    canStart?: (row: Row) => boolean;
    /** 读屏用的名字，例「信用政策 SYN-CREDIT-01 v1」。 */
    labelOf: (row: Row) => string;
    onStart: (row: Row) => void;
  },
): ListColumn<Row>[] {
  return [
    ...columns,
    {
      id: 'new-version',
      header: '操作',
      render: (row: Row) =>
        options.canStart && !options.canStart(row) ? null : (
          <Button
            variant="outline"
            size="sm"
            className="whitespace-nowrap"
            title="用这一版的内容预填发布表单，发布一个新版本；会替换该表单里已填的草稿"
            aria-label={`基于 ${options.labelOf(row)} 发新版本`}
            onClick={(event) => {
              event.stopPropagation();
              options.onStart(row);
            }}
          >
            发新版本
          </Button>
        ),
    },
  ];
}
