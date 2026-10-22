import { describe, expect, it } from 'vitest';
import { buildExportFilename } from './export';

describe('buildExportFilename（issue #120：图表名 + 本地时间戳）', () => {
  const at = new Date(2026, 9, 23, 8, 5, 9);

  it('拼出 名称-年月日-时分秒.扩展名', () => {
    expect(buildExportFilename('月度销售', 'csv', at)).toBe('月度销售-20261023-080509.csv');
  });

  it('剔除文件名非法字符、空白折成下划线', () => {
    expect(buildExportFilename('月/度 销售*?', 'xlsx', at)).toBe('月度_销售-20261023-080509.xlsx');
  });

  it('空名称回落 chart，不会得到只剩时间戳的文件名', () => {
    expect(buildExportFilename('   ', 'png', at)).toBe('chart-20261023-080509.png');
  });
});
