import { TbAlertCircle, TbRefresh, TbSettings } from 'react-icons/tb';
import type { DatabaseProfile } from './types';
import type { ConnectionFailure } from './useDatabaseWorkspace';
import { DatabaseError } from './DatabaseError';

export function ConnectionFailurePanel({
  profile,
  failure,
  busy,
  onRetry,
  onEdit,
}: {
  profile: DatabaseProfile;
  failure: ConnectionFailure;
  busy: boolean;
  onRetry(): void;
  onEdit(): void;
}) {
  return (
    <section
      className="oc-db-connection-failure"
      aria-label="数据库连接失败详情"
      aria-busy={busy}
    >
      <header>
        <div className="oc-db-failure-title">
          <TbAlertCircle size={21} aria-hidden="true" />
          <h3>{busy ? '正在重新连接…' : '连接失败'}</h3>
        </div>
        <div className="oc-db-failure-actions">
          <button className="oc-db-primary" disabled={busy} onClick={onRetry}>
            <TbRefresh size={15} />
            {busy ? '正在连接…' : '重试连接'}
          </button>
          <button disabled={busy} onClick={onEdit}>
            <TbSettings size={15} />
            修改连接
          </button>
        </div>
      </header>
      <dl className="oc-db-failure-context">
        <div>
          <dt>目标地址</dt>
          <dd>
            {profile.host}:{profile.port}
          </dd>
        </div>
        <div>
          <dt>连接方式</dt>
          <dd>
            {profile.gatewayId ? '通过 SSH 连接' : '直接连接'}
          </dd>
        </div>
        <div>
          <dt>失败阶段</dt>
          <dd>{failure.stage}</dd>
        </div>
        <div>
          <dt>失败时间</dt>
          <dd>
            <time dateTime={failure.occurredAt}>
              {new Date(failure.occurredAt).toLocaleString()}
            </time>
          </dd>
        </div>
      </dl>
      <div className="oc-db-failure-details">
        <p>请检查连接信息，或将详细错误提供给维护人员。</p>
        <details>
          <summary>查看详细错误</summary>
          <DatabaseError message={failure.message} />
        </details>
      </div>
    </section>
  );
}
