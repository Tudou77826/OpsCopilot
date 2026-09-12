/* 门禁 fixture：测试文件里的字面色值不参与扫描（用例本身需要断言具体颜色）。 */
import { expect, it } from 'vitest';

it('asserts a literal color', () => {
  expect('#abcdef').toBe('#abcdef');
});
