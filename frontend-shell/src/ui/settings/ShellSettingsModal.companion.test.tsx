import React from 'react';
import { render, screen, fireEvent, waitFor, cleanup, within } from '@testing-library/react';
import { afterEach, it, expect, vi } from 'vitest';
import ShellSettingsModal from './ShellSettingsModal';
import { normalizeTerminalConfig } from '../Terminal/terminalAppearance';
import type { CompanionHost, CompanionSnapshot } from '../../companion/types';
import type { SocialHost } from '../../companion/socialHost';

afterEach(cleanup);

const emptyCompanion: CompanionSnapshot = {
  schemaVersion: 1, ruleVersion: 1, revision: 0, level: 0, levelXP: 0, levelXPRequired: 0,
  pet: null, todayXP: 0, dailyXPMax: 12, hatchCost: 0, canHatch: true,
};

function setup(companionEnabled: boolean) {
  const snapshot = vi.fn(async () => emptyCompanion);
  const status = vi.fn(async () => ({
    summary: { ruleVersion: 1 as const, progress: [] },
    pending: 0, repairs: 0, selected: [], profilePending: false,
  }));
  const companionHost: CompanionHost = { snapshot, hatch: vi.fn(), rename: vi.fn() };
  const socialHost: SocialHost = {
    status, catalog: vi.fn(async () => []), connect: vi.fn(), sync: vi.fn(), participation: vi.fn(),
    showcase: vi.fn(), rankings: vi.fn(), profile: vi.fn(),
  };
  const runtime = {
    load: vi.fn(async () => ({
      theme: 'light' as const, terminal: normalizeTerminalConfig(), completionDelay: 150,
      highlightRules: [], companionEnabled,
    })),
    save: vi.fn(async () => {}),
  };
  render(<ShellSettingsModal isOpen initialTab="achievements" runtime={runtime}
    companionHost={companionHost} socialHost={socialHost} onApply={() => {}} onClose={() => {}}/>);
  return { snapshot, status, runtime };
}

it('shows companion and achievements without the persona preference', async () => {
  const { runtime } = setup(true);
  expect(await screen.findByRole('button', { name: '开蛋' })).toBeVisible();
  expect(screen.getByRole('heading', { name: '伙伴与奖章' })).toBeVisible();
  expect(screen.queryByText('伙伴人设', { exact: false })).toBeNull();
  expect(screen.queryByRole('button', { name: '保存更改' })).toBeNull();
  expect(runtime.save).not.toHaveBeenCalled();
});

it('hides disabled growth until successfully re-enabled and hides it again after saving off', async () => {
  const { snapshot, status, runtime } = setup(false);
  const toggle = await screen.findByLabelText('启用伙伴养成与功能奖章');
  const nav = within(screen.getByRole('navigation'));
  expect(nav.queryByText('伙伴与奖章')).toBeNull();
  expect(snapshot).not.toHaveBeenCalled();
  expect(status).not.toHaveBeenCalled();
  expect(toggle).not.toBeChecked();
  fireEvent.click(toggle);
  expect(nav.queryByText('伙伴与奖章')).toBeNull();
  expect(runtime.save).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole('button', { name: '保存更改' }));
  await waitFor(() => expect(nav.getByText('伙伴与奖章')).toBeVisible());
  expect(runtime.save).toHaveBeenLastCalledWith(expect.objectContaining({ companionEnabled: true }));
  fireEvent.click(toggle);
  runtime.save.mockRejectedValueOnce(new Error('写入失败'));
  fireEvent.click(screen.getByRole('button', { name: '保存更改' }));
  await screen.findByText(/写入失败/);
  expect(nav.getByText('伙伴与奖章')).toBeVisible();
  fireEvent.click(screen.getByRole('button', { name: '保存更改' }));
  await waitFor(() => expect(nav.queryByText('伙伴与奖章')).toBeNull());
  expect(runtime.save).toHaveBeenLastCalledWith(expect.objectContaining({ companionEnabled: false }));
  expect(snapshot).not.toHaveBeenCalled();
  expect(status).not.toHaveBeenCalled();
});
