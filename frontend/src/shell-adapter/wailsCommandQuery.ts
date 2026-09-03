import type { CommandQueryResult } from '../../../frontend-shell/src/ui/command/CommandQueryOverlay';

export async function generateWailsCommand(query: string): Promise<CommandQueryResult> {
  const app = (window as any).go?.main?.App;
  if (!app?.GenerateLinuxCommand) throw new Error('Wails 运行时未就绪');
  const response = await app.GenerateLinuxCommand(query);
  if (typeof response === 'string' && response.startsWith('Error:')) throw new Error(response);
  return JSON.parse(response);
}
