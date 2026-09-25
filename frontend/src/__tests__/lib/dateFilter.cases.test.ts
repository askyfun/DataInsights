import dayjs from 'dayjs';
import { describe, expect, it } from 'vitest';
import fixture from '../../lib/__fixtures__/dateFilterCases.json';
import {
  type DateFilterValue,
  type DateGranularity,
  resolveDateFilter,
  type WeekStart,
} from '../../lib/dateFilter';

/**
 * 日期筛选解析的**双端一致性**用例表。
 *
 * 同一个用例表也被后端 `backend/internal/query/datefilter_test.go` 读取：
 * 两侧必须逐例一致，任一侧漂移都会让对应用例失败。
 * ⚠️ 本文件只校验「解析语义」；展示形态（月/周粒度的预览文案）是纯前端的事，在 dateFilter.test.ts。
 */

interface SharedCase {
  name: string;
  /** 覆盖夹具默认 now（例如文档里 2020 年的原例）。 */
  now?: string;
  granularity: DateGranularity;
  weekStart: number;
  withTime?: boolean;
  value: DateFilterValue;
  want: {
    operator: string | null;
    start?: string;
    end?: string;
    includeEmpty?: boolean;
  };
}

const table = fixture.cases as unknown as SharedCase[];

describe('日期筛选解析：双端共用用例表', () => {
  it('用例表非空且每例都有名字', () => {
    expect(table.length).toBeGreaterThan(30);
    for (const testCase of table) {
      expect(testCase.name, '用例名不能为空').toBeTruthy();
    }
  });

  it.each(table)('$name', (testCase) => {
    const got = resolveDateFilter(
      testCase.value,
      {
        granularity: testCase.granularity,
        weekStart: testCase.weekStart as WeekStart,
        withTime: testCase.withTime,
      },
      dayjs(testCase.now ?? fixture.now)
    );

    expect(got).toEqual({
      operator: testCase.want.operator,
      ...(testCase.want.start === undefined ? {} : { start: testCase.want.start }),
      ...(testCase.want.end === undefined ? {} : { end: testCase.want.end }),
      includeEmpty: testCase.want.includeEmpty ?? false,
    });
  });
});
