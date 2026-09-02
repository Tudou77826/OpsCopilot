import '@testing-library/jest-dom/vitest';
import { cleanup } from '@testing-library/react';
import { afterEach } from 'vitest';

// 显式 cleanup：防止组件测试间 DOM 泄漏（getByTestId 命中上一用例残留节点）。
afterEach(cleanup);
