import type { DiagnoseRuntime, DiagnoseEvent, DiagnoseStartOptions, DiagnoseArchiveInput } from '../../ui';
import type { SidecarClient } from '../../core/sidecarClient';

/**
 * Sidecar 对共享诊断面板的宿主适配。
 * shell.diagnose.event 通知即中性契约本体（runId/kind/stage/message/…），
 * 适配层只做透传，不引入宿主内部 Agent 形状。
 */
export function makeSidecarDiagnoseRuntime(clientGetter: () => SidecarClient | null): DiagnoseRuntime {
  return {
    async start(problem: string, opts?: DiagnoseStartOptions): Promise<{ runId: string; caseId?: string }> {
      const client = clientGetter();
      if (!client) throw new Error('sidecar 未连接');
      return client.diagnoseStart(problem, opts) as Promise<{ runId: string; caseId?: string }>;
    },
    async cancel(runId: string): Promise<void> {
      const client = clientGetter();
      if (!client) throw new Error('sidecar 未连接');
      await client.diagnoseCancel(runId);
    },
    onEvent(handler: (event: DiagnoseEvent) => void): () => void {
      const client = clientGetter();
      if (!client) return () => {};
      return client.on('shell.diagnose.event', (params) => handler(params as unknown as DiagnoseEvent));
    },
    cases: {
      async stop(rootCause: string, conclusion: string): Promise<void> {
        const client = clientGetter();
        if (!client) throw new Error('sidecar 未连接');
        await client.diagnoseStop(rootCause, conclusion);
      },
      async conclusion(rootCause: string): Promise<void> {
        const client = clientGetter();
        if (!client) throw new Error('sidecar 未连接');
        await client.diagnoseConclusion(rootCause);
      },
      async archive(input: DiagnoseArchiveInput): Promise<{ filePath: string }> {
        const client = clientGetter();
        if (!client) throw new Error('sidecar 未连接');
        return client.diagnoseArchive(input as unknown as Record<string, unknown>);
      },
    },
  };
}
