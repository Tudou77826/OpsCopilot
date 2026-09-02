import ts from 'typescript';
import { expect, it } from 'vitest';

const sources = import.meta.glob('./**/*.{ts,tsx}', { eager: true, query: '?raw', import: 'default' });

it('共享 UI 不直接调用宿主或导入实验工作台', () => {
  const violations: string[] = [];
  for (const [path, raw] of Object.entries(sources)) {
    if (path.includes('.test.')) continue;
    const source = ts.createSourceFile(path, raw as string, ts.ScriptTarget.Latest, true);
    function inspect(node: ts.Node) {
      if ((ts.isImportDeclaration(node) || ts.isExportDeclaration(node)) && node.moduleSpecifier) {
        if (/wailsjs|adapters\/|workbench|ProjectSession/.test(node.moduleSpecifier.getText(source))) violations.push(path);
      }
      if (ts.isPropertyAccessExpression(node) && /^window\.(go|runtime)$/.test(node.getText(source))) violations.push(path);
      if (ts.isNewExpression(node) && /^(SidecarClient|WebSocket)$/.test(node.expression.getText(source))) violations.push(path);
      ts.forEachChild(node, inspect);
    }
    inspect(source);
  }
  expect(violations).toEqual([]);
});
