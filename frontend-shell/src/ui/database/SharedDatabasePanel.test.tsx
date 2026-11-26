import { describe, expect, it, vi, beforeEach } from 'vitest';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { SharedDatabasePanel } from './SharedDatabasePanel';
import type {
  DatabaseProfile,
  SharedDatabaseEntry,
  SharedDatabaseRuntime,
} from './types';

const sampleEntries: SharedDatabaseEntry[] = [
  {
    entryKey: 'alice|mysql||10.0.1.5|3306|readonly|orders|',
    owner: 'alice',
    name: '订单库-只读',
    engine: 'mysql',
    host: '10.0.1.5',
    port: 3306,
    user: 'readonly',
    database: 'orders',
    readOnly: true,
    lastLoginAt: '2026-11-26 10:00:00',
    own: false,
    hasSecrets: true,
    decryptable: true,
  },
  {
    entryKey: 'me|jdbc|dm.jdbc.driver.DmDriver|10.0.1.9|5236|SYSDBA||',
    owner: 'me',
    name: '达梦测试库',
    engine: 'jdbc',
    driverName: '达梦 JDBC',
    driverClassName: 'dm.jdbc.driver.DmDriver',
    driverDialect: 'dm',
    host: '10.0.1.9',
    port: 5236,
    user: 'SYSDBA',
    lastLoginAt: '2026-11-26 11:00:00',
    own: true,
    hasSecrets: true,
    decryptable: true,
  },
];

const sampleProfile: DatabaseProfile = {
  id: 'p-imported',
  revision: 1,
  name: '订单库-只读',
  environment: 'prod',
  group: '团队共享',
  engine: 'mysql',
  host: '10.0.1.5',
  port: 3306,
  user: 'readonly',
  database: 'orders',
  tls: 'off',
  readOnly: true,
  gatewayId: '',
  passwordSet: true,
};

describe('SharedDatabasePanel', () => {
  beforeEach(() => {
    localStorage.clear();
  });

  it('renders nothing when disabled', async () => {
    const runtime: SharedDatabaseRuntime = {
      list: vi.fn(async () => ({ enabled: false, databases: [] })),
      importToLocal: vi.fn(),
      remove: vi.fn(),
    };
    const { container } = render(
      <SharedDatabasePanel
        runtime={runtime}
        onImported={vi.fn()}
        onMissingDriver={vi.fn()}
      />,
    );
    await waitFor(() => expect(runtime.list).toHaveBeenCalled());
    expect(container.firstChild).toBeNull();
  });

  it('lists shared databases, filters by searchTerm, and imports+connects on double click', async () => {
    const runtime: SharedDatabaseRuntime = {
      list: vi.fn(async () => ({ enabled: true, databases: sampleEntries })),
      importToLocal: vi.fn(async () => ({
        success: true,
        profile: sampleProfile,
        reused: false,
      })),
      remove: vi.fn(),
    };
    const onImported = vi.fn();

    const { rerender } = render(
      <SharedDatabasePanel
        runtime={runtime}
        searchTerm=""
        onImported={onImported}
        onMissingDriver={vi.fn()}
      />,
    );

    expect(await screen.findByText('订单库-只读')).toBeTruthy();
    expect(screen.getByText('达梦测试库')).toBeTruthy();
    expect(screen.getByText('只读')).toBeTruthy();

    rerender(
      <SharedDatabasePanel
        runtime={runtime}
        searchTerm="alice"
        onImported={onImported}
        onMissingDriver={vi.fn()}
      />,
    );
    expect(screen.getByText('订单库-只读')).toBeTruthy();
    expect(screen.queryByText('达梦测试库')).toBeNull();

    fireEvent.doubleClick(screen.getByText('订单库-只读'));
    await waitFor(() => {
      expect(runtime.importToLocal).toHaveBeenCalledWith(
        sampleEntries[0].entryKey,
      );
      expect(onImported).toHaveBeenCalledWith(sampleProfile, true);
    });
  });

  it('triggers onMissingDriver when backend reports missing JDBC driver', async () => {
    const missingResult = {
      success: false,
      missingDriver: true,
      driverName: '达梦 JDBC',
      driverClassName: 'dm.jdbc.driver.DmDriver',
      message: '本机未导入驱动「达梦 JDBC」',
    };
    const runtime: SharedDatabaseRuntime = {
      list: vi.fn(async () => ({ enabled: true, databases: sampleEntries })),
      importToLocal: vi.fn(async () => missingResult),
      remove: vi.fn(),
    };
    const onMissingDriver = vi.fn();

    render(
      <SharedDatabasePanel
        runtime={runtime}
        onImported={vi.fn()}
        onMissingDriver={onMissingDriver}
      />,
    );

    fireEvent.doubleClick(await screen.findByText('达梦测试库'));
    await waitFor(() => {
      expect(onMissingDriver).toHaveBeenCalledWith(missingResult);
    });
  });
});
