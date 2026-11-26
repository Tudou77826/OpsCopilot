import React, { useMemo, useState } from 'react';
import { DatabaseWorkspace } from '../ui/database/DatabaseWorkspace';
import type {
  DatabaseProfile,
  DatabaseResultSet,
  DatabaseRuntime,
  DatabaseStructure,
} from '../ui/database/types';

function createMockDatabaseRuntime(
  onUpdateLogged: (sql: string) => void,
): DatabaseRuntime {
  const profiles: DatabaseProfile[] = [
    {
      id: 'mock-staging',
      revision: 1,
      name: '订单库（测试可写）',
      environment: '测试',
      group: '本地 Mock',
      engine: 'jdbc',
      driverId: 'mysql-jdbc',
      host: '127.0.0.1',
      port: 3306,
      user: 'ops_dev',
      database: 'orders',
      tls: 'off',
      readOnly: false,
      gatewayId: '',
      passwordSet: true,
    },
    {
      id: 'mock-prod',
      revision: 1,
      name: '核心账务库（生产可写）',
      environment: '生产',
      group: '本地 Mock',
      engine: 'jdbc',
      driverId: 'mysql-jdbc',
      host: '10.0.12.8',
      port: 3306,
      user: 'ops_admin',
      database: 'orders',
      tls: 'preferred',
      readOnly: false,
      gatewayId: '',
      passwordSet: true,
    },
  ];

  const orderEventsSet: DatabaseResultSet = {
    statement: 'SELECT * FROM `orders`.`order_events` LIMIT 100',
    columns: [
      { name: 'id', type: 'BIGINT' },
      { name: 'order_no', type: 'VARCHAR(32)' },
      { name: 'amount', type: 'DECIMAL(10,2)' },
      { name: 'status', type: 'VARCHAR(16)' },
      { name: 'payload', type: 'JSON' },
      { name: 'remark', type: 'TEXT' },
    ],
    rows: [
      [
        '1002',
        'ORD-2026-0002',
        '89.50',
        'PAID',
        '{"channel":"alipay","retry":0,"items":[{"sku":"A-10","qty":2},{"sku":"B-04","qty":1}]}',
        '正常支付回调完成',
      ],
      [
        '10010',
        'ORD-2026-0010',
        '1280.00',
        'PENDING',
        '{"channel":"wechat","retry":3,"error":{"code":"TIMEOUT","msg":"gateway timeout"}}',
        '第一行备注\n第二行：等待人工核对流水号 9007199254740993\n第三行：可双击弹窗查看完整多行内容',
      ],
      [
        '9007199254740993',
        'ORD-2026-BIGINT',
        '9.99',
        'REFUNDED',
        null,
        '',
      ],
      [
        '9007199254740992',
        'ORD-2026-BIGINT-2',
        '420.00',
        'PAID',
        '[{"step":"create","ok":true},{"step":"settle","ok":true}]',
        null,
      ],
      [
        '15',
        'ORD-2026-0015',
        '315.25',
        'FAILED',
        '{"reason":"insufficient_balance","account":"acct_9921"}',
        '余额不足，已通知用户重试',
      ],
    ],
    affectedRows: 0,
    truncated: false,
  };

  const auditLogsSet: DatabaseResultSet = {
    statement: 'SELECT * FROM `orders`.`audit_logs` LIMIT 100',
    columns: [
      { name: 'operator', type: 'VARCHAR(32)' },
      { name: 'action', type: 'VARCHAR(32)' },
      { name: 'detail', type: 'TEXT' },
    ],
    rows: [
      ['alice', 'EXPORT_CSV', '导出订单明细 240 行'],
      ['bob', 'UPDATE_STATUS', '手动将异常单改回 PENDING'],
      ['charlie', 'LOGIN', null],
    ],
    affectedRows: 0,
    truncated: false,
  };

  const structures: Record<string, DatabaseStructure> = {
    order_events: {
      columns: {
        statement: '',
        columns: [
          { name: '字段', type: 'TEXT' },
          { name: '类型', type: 'TEXT' },
          { name: '可空', type: 'TEXT' },
          { name: '默认值', type: 'TEXT' },
          { name: '键', type: 'TEXT' },
          { name: '其他', type: 'TEXT' },
          { name: '注释', type: 'TEXT' },
        ],
        rows: [
          ['id', 'bigint', 'NO', null, 'PRI', 'auto_increment', '主键 ID'],
          ['order_no', 'varchar(32)', 'NO', null, 'UNI', '', '订单号'],
          ['amount', 'decimal(10,2)', 'NO', '0.00', '', '', '金额'],
          ['status', 'varchar(16)', 'NO', 'PENDING', '', '', '状态'],
          ['payload', 'json', 'YES', null, '', '', '扩展 JSON'],
          ['remark', 'text', 'YES', null, '', '', '备注'],
        ],
        affectedRows: 0,
        truncated: false,
      },
      indexes: {
        statement: '',
        columns: [
          { name: '索引', type: 'TEXT' },
          { name: '字段', type: 'TEXT' },
          { name: '顺序', type: 'TEXT' },
          { name: '非唯一', type: 'TEXT' },
          { name: '类型', type: 'TEXT' },
        ],
        rows: [['PRIMARY', 'id', '1', '0', 'BTREE']],
        affectedRows: 0,
        truncated: false,
      },
      ddl: 'CREATE TABLE `order_events` (\n  `id` bigint NOT NULL AUTO_INCREMENT,\n  `order_no` varchar(32) NOT NULL,\n  `amount` decimal(10,2) NOT NULL,\n  `status` varchar(16) NOT NULL,\n  `payload` json DEFAULT NULL,\n  `remark` text,\n  PRIMARY KEY (`id`)\n);',
    },
    audit_logs: {
      columns: {
        statement: '',
        columns: [
          { name: '字段', type: 'TEXT' },
          { name: '类型', type: 'TEXT' },
          { name: '可空', type: 'TEXT' },
          { name: '默认值', type: 'TEXT' },
          { name: '键', type: 'TEXT' },
        ],
        rows: [
          ['operator', 'varchar(32)', 'NO', null, ''],
          ['action', 'varchar(32)', 'NO', null, ''],
          ['detail', 'text', 'YES', null, ''],
        ],
        affectedRows: 0,
        truncated: false,
      },
      indexes: {
        statement: '',
        columns: [],
        rows: [],
        affectedRows: 0,
        truncated: false,
      },
      ddl: 'CREATE TABLE `audit_logs` (\n  `operator` varchar(32) NOT NULL,\n  `action` varchar(32) NOT NULL,\n  `detail` text\n);',
    },
  };

  const mockPasswords: Record<string, string> = {
    'mock-staging': 'dev_pass_123',
    'mock-prod': 'prod_secret_888',
  };

  return {
    jdbcStatus: async () => ({
      available: true,
      message: '',
      drivers: [
        {
          id: 'mysql-jdbc',
          name: 'MySQL Connector/J',
          className: 'com.mysql.cj.jdbc.Driver',
          urlTemplate: 'jdbc:mysql://{host}:{port}/{database}',
          dialect: 'mysql',
          version: '9.1',
          jars: [],
        },
      ],
    }),
    importJDBC: async () => {
      throw new Error('Mock 模式不支持导入驱动');
    },
    deleteJDBC: async () => {},
    chooseJDBCJars: async () => [],
    profiles: async () => profiles,
    password: async (id) => mockPasswords[id] ?? '',
    groups: async () => ['本地 Mock'],
    createGroup: async () => {},
    renameGroup: async () => {},
    deleteGroup: async () => {},
    save: async (input) => {
      const id = input.id || `mock-${Date.now()}`;
      if (input.password !== undefined) mockPasswords[id] = input.password;
      const saved: DatabaseProfile = {
        ...input,
        id,
        passwordSet: Boolean(mockPasswords[id]),
      };
      const idx = profiles.findIndex((p) => p.id === id);
      if (idx >= 0) profiles[idx] = saved;
      else profiles.push(saved);
      return saved;
    },
    test: async () => 'Mock 连接测试成功',
    remove: async () => {},
    gateways: async () => [],
    connect: async (id) => {
      const p = profiles.find((item) => item.id === id) ?? profiles[0];
      return {
        id: `session-${p.id}`,
        profileId: p.id,
        version: '8.4.0-mock',
        database: p.database,
        readOnly: p.readOnly,
      };
    },
    disconnect: async () => {},
    schemas: async () => ['orders'],
    tables: async () => [
      { name: 'order_events', type: 'BASE TABLE', comment: '订单事件（有主键，可编辑）' },
      { name: 'audit_logs', type: 'BASE TABLE', comment: '审计日志（无主键，测试警示）' },
    ],
    structure: async (req) =>
      structures[req.table] ?? structures.order_events,
    generateSQL: async (req) =>
      `SELECT * FROM \`${req.database}\`.\`${req.table}\` LIMIT 100;`,
    browse: async (req) => ({
      sets: [
        req.table === 'audit_logs'
          ? { ...auditLogsSet, rows: auditLogsSet.rows.map((r) => [...r]) }
          : { ...orderEventsSet, rows: orderEventsSet.rows.map((r) => [...r]) },
      ],
      durationMs: 12,
      error: '',
      cancelled: false,
      disconnected: false,
    }),
    query: async (req) => {
      const trimmed = req.sql.trim();
      if (/^update\b/i.test(trimmed)) {
        onUpdateLogged(trimmed);
        return {
          sets: [
            {
              statement: trimmed,
              columns: [],
              rows: [],
              affectedRows: 1,
              truncated: false,
            },
          ],
          durationMs: 8,
          error: '',
          cancelled: false,
          disconnected: false,
        };
      }
      return {
        sets: [
          { ...orderEventsSet, rows: orderEventsSet.rows.map((r) => [...r]) },
          { ...auditLogsSet, rows: auditLogsSet.rows.map((r) => [...r]) },
        ],
        durationMs: 15,
        error: '',
        cancelled: false,
        disconnected: false,
      };
    },
    cancel: async () => true,
  };
}

export const DatabasePreview: React.FC = () => {
  const [theme, setTheme] = useState<'dark' | 'light'>(() =>
    document.documentElement.getAttribute('data-theme') === 'light'
      ? 'light'
      : 'dark',
  );
  const [lastUpdateSQL, setLastUpdateSQL] = useState('');

  const runtime = useMemo(
    () => createMockDatabaseRuntime((sql) => setLastUpdateSQL(sql)),
    [],
  );

  function toggleTheme() {
    const next = theme === 'dark' ? 'light' : 'dark';
    setTheme(next);
    document.documentElement.setAttribute('data-theme', next);
  }

  return (
    <div
      style={{
        display: 'flex',
        flexDirection: 'column',
        width: '100vw',
        height: '100vh',
        background: 'var(--bg-primary)',
        color: 'var(--text-primary)',
      }}
    >
      <div
        style={{
          display: 'flex',
          alignItems: 'center',
          gap: 12,
          height: 36,
          padding: '0 12px',
          borderBottom: '1px solid var(--border)',
          background: 'var(--bg-elevated)',
          fontSize: 12,
          flexShrink: 0,
        }}
      >
        <strong>SQL 结果交互预览 (Mock)</strong>
        <span style={{ color: 'var(--text-muted)', fontSize: 11 }}>
          双击左侧「订单库」→ 展开 orders → 双击 order_events（有主键）或 audit_logs（无主键）测试表格交互与 F2 行内编辑
        </span>
        <span style={{ flex: 1 }} />
        {lastUpdateSQL && (
          <span
            title={lastUpdateSQL}
            style={{
              color: 'var(--accent)',
              fontSize: 11,
              maxWidth: 320,
              overflow: 'hidden',
              textOverflow: 'ellipsis',
              whiteSpace: 'nowrap',
            }}
          >
            最近模拟执行：{lastUpdateSQL.replace(/\s+/g, ' ')}
          </span>
        )}
        <button
          type="button"
          onClick={toggleTheme}
          style={{
            padding: '3px 10px',
            borderRadius: 4,
            border: '1px solid var(--border)',
            background: 'var(--bg-secondary)',
            color: 'var(--text-primary)',
            cursor: 'pointer',
            fontSize: 11,
          }}
        >
          切换为{theme === 'dark' ? '明亮' : '暗夜'}主题
        </button>
      </div>
      <div style={{ flex: 1, minHeight: 0, display: 'flex' }}>
        <DatabaseWorkspace runtime={runtime} />
      </div>
    </div>
  );
};
