import React, { useState, useEffect, useRef, useCallback } from 'react';
import { colors, radius, font, btnPrimary } from '../settingsStyles';
import Switch from '../Switch';
import { WhitelistConfig, Policy, Command } from './types';

interface CommandWhitelistPanelProps {
  onSave?: () => void;
}

const CommandWhitelistPanel: React.FC<CommandWhitelistPanelProps> = ({ onSave }) => {
  const [config, setConfig] = useState<WhitelistConfig | null>(null);
  const [loading, setLoading] = useState(false);
  const [saveStatus, setSaveStatus] = useState<'idle' | 'saving' | 'saved' | 'error'>('idle');
  const [expandedPolicy, setExpandedPolicy] = useState<string | null>(null);
  const [editingPolicy, setEditingPolicy] = useState<Policy | null>(null);
  // IP 查询相关状态
  const [ipQuery, setIpQuery] = useState('');
  const [ipQueryResult, setIpQueryResult] = useState<Policy[] | null>(null);
  const [ipQueryError, setIpQueryError] = useState('');
  // 用于防抖保存
  const saveTimeoutRef = useRef<NodeJS.Timeout | null>(null);
  // 追踪初始加载是否完成
  const initialLoadCompleteRef = useRef(false);
  // 保存上一次的配置，用于比较是否有变化
  const prevConfigRef = useRef<string>('');
  // IP 查询防抖
  const ipQueryTimeoutRef = useRef<NodeJS.Timeout | null>(null);

  useEffect(() => {
    loadConfig();
  }, []);

  // 自动保存（防抖）
  useEffect(() => {
    // 跳过初始加载完成前的保存
    if (!initialLoadCompleteRef.current) {
      return;
    }
    if (!config) return;

    // 检查配置是否真的有变化
    const configStr = JSON.stringify(config);
    if (configStr === prevConfigRef.current) {
      return;
    }
    prevConfigRef.current = configStr;

    // 清除之前的定时器
    if (saveTimeoutRef.current) {
      clearTimeout(saveTimeoutRef.current);
    }

    // 设置新的防抖保存
    setSaveStatus('saving');
    saveTimeoutRef.current = setTimeout(async () => {
      try {
        // @ts-ignore
        await window.go.main.App.SaveCommandWhitelist(config);
        setSaveStatus('saved');
        onSave?.();
        // 3秒后清除状态
        setTimeout(() => setSaveStatus('idle'), 3000);
      } catch (err) {
        console.error('自动保存失败:', err);
        setSaveStatus('error');
      }
    }, 500); // 500ms 防抖

    return () => {
      if (saveTimeoutRef.current) {
        clearTimeout(saveTimeoutRef.current);
      }
    };
  }, [config]);

  const loadConfig = async () => {
    setLoading(true);
    try {
      // @ts-ignore
      const result = await window.go.main.App.GetCommandWhitelist();
      setConfig(result);
      // 记录初始配置，用于后续比较
      prevConfigRef.current = JSON.stringify(result);
      // 标记初始加载完成
      initialLoadCompleteRef.current = true;
    } catch (err) {
      console.error('加载配置失败:', err);
    } finally {
      setLoading(false);
    }
  };

  // IP 查询:输入合法 IP 时 debounce 300ms 后调用后端
  useEffect(() => {
    if (ipQueryTimeoutRef.current) {
      clearTimeout(ipQueryTimeoutRef.current);
    }
    const trimmed = ipQuery.trim();
    if (!trimmed) {
      setIpQueryResult(null);
      setIpQueryError('');
      return;
    }
    // 简单 IPv4 校验
    const ipv4Regex = /^(\d{1,3}\.){3}\d{1,3}$/;
    if (!ipv4Regex.test(trimmed) || trimmed.split('.').some(p => Number(p) > 255)) {
      setIpQueryResult(null);
      setIpQueryError('IP 格式不正确,请输入合法 IPv4 地址');
      return;
    }
    ipQueryTimeoutRef.current = setTimeout(async () => {
      try {
        // @ts-ignore
        const policies = await window.go.main.App.GetPoliciesForIP(trimmed);
        setIpQueryResult(policies || []);
        setIpQueryError('');
      } catch (err) {
        console.error('IP 查询失败:', err);
        setIpQueryResult(null);
        setIpQueryError('查询失败: ' + err);
      }
    }, 300);

    return () => {
      if (ipQueryTimeoutRef.current) {
        clearTimeout(ipQueryTimeoutRef.current);
      }
    };
  }, [ipQuery]);

  const handleAddPolicy = () => {
    if (!config) return;
    const newPolicy: Policy = {
      id: `policy-${Date.now()}`,
      name: '新策略',
      description: '',
      ip_ranges: [],
      commands: [],
    };
    setConfig({
      ...config,
      policies: [...config.policies, newPolicy],
    });
    setEditingPolicy(newPolicy);
  };

  const handleDeletePolicy = (policyId: string) => {
    if (!config) return;
    setConfig({
      ...config,
      policies: config.policies.filter(p => p.id !== policyId),
    });
  };

  const handleUpdatePolicy = (updatedPolicy: Policy) => {
    if (!config) return;
    setConfig({
      ...config,
      policies: config.policies.map(p => p.id === updatedPolicy.id ? updatedPolicy : p),
    });
    setEditingPolicy(null);
  };

  const toggleCommand = (policyId: string, cmdIndex: number) => {
    if (!config) return;
    setConfig({
      ...config,
      policies: config.policies.map(p => {
        if (p.id === policyId) {
          const newCommands = [...p.commands];
          newCommands[cmdIndex] = { ...newCommands[cmdIndex], enabled: !newCommands[cmdIndex].enabled };
          return { ...p, commands: newCommands };
        }
        return p;
      }),
    });
  };

  const getCategoryColor = (category: string) => {
    return category === 'read_only' ? 'var(--success)' : 'var(--warning)';
  };

  if (loading) {
    return <div style={styles.loading}>加载中...</div>;
  }

  if (!config) {
    return <div style={styles.loading}>无法加载配置</div>;
  }

  return (
    <div style={styles.container}>
      {/* IP 查询 */}
      <div style={styles.section}>
        <div style={styles.toolbar}>
          <span style={styles.sectionTitle}>按 IP 查询匹配策略</span>
        </div>
        <div style={styles.ipQueryDesc}>输入服务器 IP,实时展示该 IP 命中的所有策略</div>
        <div style={styles.ipQueryRow}>
          <input
            style={styles.input}
            placeholder="例如:38.1.2.3"
            value={ipQuery}
            onChange={e => setIpQuery(e.target.value)}
          />
          {ipQuery && (
            <button style={styles.ipQueryClear} onClick={() => setIpQuery('')}>×</button>
          )}
        </div>
        {ipQueryError && (
          <div style={styles.ipQueryError}>{ipQueryError}</div>
        )}
        {ipQueryResult && (
          <div style={styles.ipQueryResultBox}>
            {ipQueryResult.length === 0 ? (
              <div style={styles.ipQueryEmpty}>
                该 IP 未匹配任何策略,执行命令将被拒绝
              </div>
            ) : (
              <>
                <div style={styles.ipQueryCount}>
                  共匹配 {ipQueryResult.length} 个策略
                </div>
                {ipQueryResult.map(p => {
                  const enabledCount = p.commands.filter(c => c.enabled).length;
                  return (
                    <div key={p.id} style={styles.ipQueryPolicyItem}>
                      <span style={styles.ipQueryPolicyName}>{p.name}</span>
                      <span style={styles.ipQueryPolicyMeta}>
                        {enabledCount} / {p.commands.length} 条命令
                      </span>
                    </div>
                  );
                })}
              </>
            )}
          </div>
        )}
      </div>

      {/* 策略列表 */}
      <div style={styles.section}>
        <div style={styles.toolbar}>
          <span style={styles.sectionTitle}>策略列表</span>
          <button onClick={handleAddPolicy} style={styles.primaryBtn}>
            + 添加策略
          </button>
        </div>

        <div style={styles.policyList}>
          {config.policies.map((policy) => (
            <div key={policy.id} style={styles.policyItem}>
              {/* 策略头部 */}
              <div
                onClick={() => setExpandedPolicy(expandedPolicy === policy.id ? null : policy.id)}
                style={styles.policyHeader}
              >
                <div style={styles.policyInfo}>
                  <div style={styles.policyName}>{policy.name}</div>
                  <div style={styles.policyMeta}>
                    <span>IP 段: {policy.ip_ranges.length > 0 ? policy.ip_ranges.join(', ') : '未配置'}</span>
                    <span style={styles.metaSeparator}>|</span>
                    <span>命令: {policy.commands.filter(c => c.enabled).length}/{policy.commands.length}</span>
                  </div>
                </div>
                <div style={styles.policyActions}>
                  <button
                    onClick={(e) => {
                      e.stopPropagation();
                      setEditingPolicy(policy);
                    }}
                    style={styles.editBtn}
                  >
                    编辑
                  </button>
                  {policy.id !== 'default' && (
                    <button
                      onClick={(e) => {
                        e.stopPropagation();
                        handleDeletePolicy(policy.id);
                      }}
                      style={styles.deleteBtn}
                    >
                      删除
                    </button>
                  )}
                  <span style={styles.expandIcon}>
                    {expandedPolicy === policy.id ? '▾' : '▸'}
                  </span>
                </div>
              </div>

              {/* 策略详情（展开时显示） */}
              {expandedPolicy === policy.id && (
                <div style={styles.policyContent}>
                  {policy.description && (
                    <div style={styles.policyDesc}>{policy.description}</div>
                  )}
                  <div style={styles.commandSection}>
                    <div style={styles.commandSectionTitle}>命令规则</div>
                    {policy.commands.length === 0 ? (
                      <div style={styles.emptyText}>暂无命令规则</div>
                    ) : (
                      <div style={styles.commandList}>
                        {policy.commands.map((cmd, idx) => (
                          <div key={idx} style={styles.commandItem}>
                            <div style={styles.commandInfo}>
                              <code style={styles.commandPattern}>{cmd.pattern}</code>
                              <span style={{
                                ...styles.categoryBadge,
                                backgroundColor: getCategoryColor(cmd.category),
                              }}>
                                {cmd.category === 'read_only' ? '只读' : '写入'}
                              </span>
                              <span style={styles.commandDesc}>{cmd.description}</span>
                            </div>
                            <Switch
                              size="small"
                              checked={cmd.enabled}
                              onChange={() => toggleCommand(policy.id, idx)}
                            />
                          </div>
                        ))}
                      </div>
                    )}
                  </div>
                </div>
              )}
            </div>
          ))}
        </div>
      </div>

      {/* 保存状态指示器 */}
      {saveStatus !== 'idle' && (
        <div style={{
          ...styles.saveStatus,
          ...(saveStatus === 'saving' ? styles.saveStatusSaving : {}),
          ...(saveStatus === 'saved' ? styles.saveStatusSaved : {}),
          ...(saveStatus === 'error' ? styles.saveStatusError : {}),
        }}>
          {saveStatus === 'saving' && '保存中...'}
          {saveStatus === 'saved' && '已保存'}
          {saveStatus === 'error' && '保存失败'}
        </div>
      )}

      {/* 策略编辑模态框 */}
      {editingPolicy && (
        <PolicyEditor
          policy={editingPolicy}
          onSave={handleUpdatePolicy}
          onCancel={() => setEditingPolicy(null)}
        />
      )}
    </div>
  );
};

// 策略编辑器组件
const PolicyEditor: React.FC<{
  policy: Policy;
  onSave: (policy: Policy) => void;
  onCancel: () => void;
}> = ({ policy, onSave, onCancel }) => {
  const [editing, setEditing] = useState<Policy>({ ...policy });
  const [newIPRange, setNewIPRange] = useState('');
  const [showCidrHelp, setShowCidrHelp] = useState(false);
  const [newCommand, setNewCommand] = useState<Partial<Command>>({
    pattern: '',
    category: 'read_only',
    description: '',
    enabled: true,
  });

  const handleAddIPRange = () => {
    if (!newIPRange.trim()) return;
    setEditing({
      ...editing,
      ip_ranges: [...editing.ip_ranges, newIPRange.trim()],
    });
    setNewIPRange('');
  };

  const handleRemoveIPRange = (index: number) => {
    setEditing({
      ...editing,
      ip_ranges: editing.ip_ranges.filter((_, i) => i !== index),
    });
  };

  const handleAddCommand = () => {
    if (!newCommand.pattern?.trim()) return;
    setEditing({
      ...editing,
      commands: [...editing.commands, newCommand as Command],
    });
    setNewCommand({
      pattern: '',
      category: 'read_only',
      description: '',
      enabled: true,
    });
  };

  const handleRemoveCommand = (index: number) => {
    setEditing({
      ...editing,
      commands: editing.commands.filter((_, i) => i !== index),
    });
  };

  return (
    <div style={editorStyles.overlay}>
      <div style={editorStyles.modal}>
        <div style={editorStyles.header}>
          <h3 style={editorStyles.title}>编辑策略</h3>
          <button onClick={onCancel} style={editorStyles.closeBtn}>×</button>
        </div>

        <div style={editorStyles.body}>
          {/* 基本信息 */}
          <div style={editorStyles.field}>
            <label style={editorStyles.label}>策略名称</label>
            <input
              type="text"
              value={editing.name}
              onChange={(e) => setEditing({ ...editing, name: e.target.value })}
              style={editorStyles.input}
            />
          </div>

          <div style={editorStyles.field}>
            <label style={editorStyles.label}>描述</label>
            <input
              type="text"
              value={editing.description}
              onChange={(e) => setEditing({ ...editing, description: e.target.value })}
              style={editorStyles.input}
              placeholder="可选的策略描述"
            />
          </div>

          {/* IP 段配置 */}
          <div style={editorStyles.field}>
            <label style={editorStyles.label}>IP 段</label>
            <div style={editorStyles.hintRow}>
              <span>支持 CIDR 格式（如 192.168.1.0/24）或 * 表示所有</span>
              <span
                style={editorStyles.helpIcon}
                onClick={() => setShowCidrHelp(!showCidrHelp)}
                title="点击查看 CIDR 说明"
              >
                ?
              </span>
            </div>
            {showCidrHelp && (
              <div style={editorStyles.helpBox}>
                <div style={editorStyles.helpTitle}>CIDR 格式说明</div>
                <div style={editorStyles.helpContent}>
                  <p><code>192.168.1.0/24</code> 表示一个 IP 地址范围：</p>
                  <ul>
                    <li><strong>192.168.1.0</strong> - 网络地址</li>
                    <li><strong>/24</strong> - 前 24 位是网络部分（相当于子网掩码 255.255.255.0）</li>
                    <li><strong>IP 范围</strong> - 192.168.1.1 ~ 192.168.1.254（共 254 个地址）</li>
                  </ul>
                  <div style={editorStyles.helpExample}>
                    <strong>常见示例：</strong><br/>
                    <code>*.*.*.0/24</code> - 匹配同网段 254 个地址<br/>
                    <code>10.0.0.0/8</code> - 匹配 10.x.x.x（约 1600 万地址）<br/>
                    <code>* </code> - 匹配所有 IP
                  </div>
                </div>
              </div>
            )}
            <div style={editorStyles.inputRow}>
              <input
                type="text"
                value={newIPRange}
                onChange={(e) => setNewIPRange(e.target.value)}
                onKeyDown={(e) => e.key === 'Enter' && handleAddIPRange()}
                placeholder="例如: 192.168.1.0/24"
                style={editorStyles.input}
              />
              <button onClick={handleAddIPRange} style={editorStyles.addBtn}>添加</button>
            </div>
            <div style={editorStyles.tagList}>
              {editing.ip_ranges.map((ip, idx) => (
                <span key={idx} style={editorStyles.tag}>
                  {ip}
                  <button
                    onClick={() => handleRemoveIPRange(idx)}
                    style={editorStyles.tagRemove}
                  >
                    ×
                  </button>
                </span>
              ))}
            </div>
          </div>

          {/* 命令规则 */}
          <div style={editorStyles.field}>
            <label style={editorStyles.label}>命令规则</label>
            <div style={editorStyles.commandList}>
              {editing.commands.map((cmd, idx) => (
                <div key={idx} style={editorStyles.commandItem}>
                  <div style={editorStyles.commandInfo}>
                    <code style={editorStyles.commandPattern}>{cmd.pattern}</code>
                    <span style={{
                      ...editorStyles.categoryBadge,
                      backgroundColor: cmd.category === 'read_only' ? 'var(--success-bg-subtle)' : 'var(--warning-bg-subtle)',
                      color: cmd.category === 'read_only' ? 'var(--severity-success)' : 'var(--severity-warning)',
                    }}>
                      {cmd.category === 'read_only' ? '只读' : '写入'}
                    </span>
                  </div>
                  <button
                    onClick={() => handleRemoveCommand(idx)}
                    style={editorStyles.removeBtn}
                  >
                    删除
                  </button>
                </div>
              ))}
              {editing.commands.length === 0 && (
                <div style={editorStyles.emptyText}>暂无命令规则</div>
              )}
            </div>

            {/* 添加新命令 */}
            <div style={editorStyles.addCommandSection}>
              <div style={editorStyles.inputRow}>
                <input
                  type="text"
                  value={newCommand.pattern || ''}
                  onChange={(e) => setNewCommand({ ...newCommand, pattern: e.target.value })}
                  placeholder="正则表达式，如: ^ls(\\s|$)"
                  style={{ ...editorStyles.input, fontFamily: 'var(--font-mono)' }}
                />
                <select
                  value={newCommand.category || 'read_only'}
                  onChange={(e) => setNewCommand({ ...newCommand, category: e.target.value as 'read_only' | 'write' })}
                  style={editorStyles.select}
                >
                  <option value="read_only">只读</option>
                  <option value="write">写入</option>
                </select>
              </div>
              <div style={editorStyles.inputRow}>
                <input
                  type="text"
                  value={newCommand.description || ''}
                  onChange={(e) => setNewCommand({ ...newCommand, description: e.target.value })}
                  placeholder="命令描述（可选）"
                  style={editorStyles.input}
                />
                <button
                  onClick={handleAddCommand}
                  disabled={!newCommand.pattern?.trim()}
                  style={{
                    ...editorStyles.addBtn,
                    ...(!newCommand.pattern?.trim() ? editorStyles.addBtnDisabled : {}),
                  }}
                >
                  添加命令
                </button>
              </div>
            </div>
          </div>
        </div>

        {/* 操作按钮 */}
        <div style={editorStyles.footer}>
          <button onClick={onCancel} style={editorStyles.cancelBtn}>
            取消
          </button>
          <button onClick={() => onSave(editing)} style={editorStyles.saveBtn}>
            保存
          </button>
        </div>
      </div>
    </div>
  );
};

// 主面板样式
const styles: Record<string, React.CSSProperties> = {
  container: {
    display: 'flex',
    flexDirection: 'column',
    gap: '16px',
    padding: '8px 0',
  },
  loading: {
    padding: '40px',
    textAlign: 'center',
    color: colors.textTertiary,
    fontSize: font.lg,
  },
  section: {
    padding: '16px',
    backgroundColor: colors.bgPrimary,
    borderRadius: radius.md,
    border: `1px solid ${colors.borderPrimary}`,
  },
  sectionHeader: {
    display: 'flex',
    justifyContent: 'space-between',
    alignItems: 'center',
  },
  sectionTitle: {
    color: colors.textPrimary,
    fontSize: font.lg,
    fontWeight: 600,
  },
  sectionDesc: {
    color: colors.textTertiary,
    fontSize: font.sm,
    marginTop: '8px',
  },
  toolbar: {
    display: 'flex',
    justifyContent: 'space-between',
    alignItems: 'center',
  },
  primaryBtn: {
    ...btnPrimary,
  },
  // 输入框样式
  input: {
    flex: 1,
    padding: '8px 12px',
    borderRadius: radius.sm,
    border: `1px solid ${colors.borderPrimary}`,
    backgroundColor: colors.bgSecondary,
    color: colors.textPrimary,
    outline: 'none',
    fontSize: font.base,
  },
  // IP 查询相关样式
  ipQueryDesc: {
    color: colors.textTertiary,
    fontSize: font.sm,
    margin: '8px 0 12px',
  },
  ipQueryRow: {
    display: 'flex',
    alignItems: 'center',
    gap: '8px',
  },
  ipQueryClear: {
    background: 'transparent',
    border: 'none',
    color: colors.textTertiary,
    cursor: 'pointer',
    fontSize: font.lg,
    padding: '0 8px',
  },
  ipQueryError: {
    color: colors.danger,
    fontSize: font.sm,
    marginTop: '8px',
  },
  ipQueryResultBox: {
    marginTop: '12px',
    padding: '12px',
    backgroundColor: colors.bgSecondary,
    borderRadius: radius.sm,
    border: `1px solid ${colors.borderPrimary}`,
  },
  ipQueryCount: {
    color: colors.textTertiary,
    fontSize: font.sm,
    marginBottom: '8px',
  },
  ipQueryEmpty: {
    color: colors.danger,
    fontSize: font.sm,
  },
  ipQueryPolicyItem: {
    display: 'flex',
    justifyContent: 'space-between',
    alignItems: 'center',
    padding: '6px 0',
    borderBottom: `1px solid ${colors.borderPrimary}`,
    fontSize: font.base,
  },
  ipQueryPolicyName: {
    color: colors.textPrimary,
    fontWeight: 500,
  },
  ipQueryPolicyMeta: {
    color: colors.textTertiary,
    fontSize: font.sm,
  },
  editBtn: {
    padding: '4px 10px',
    borderRadius: radius.sm,
    border: `1px solid ${colors.borderPrimary}`,
    backgroundColor: colors.borderPrimary,
    color: colors.textPrimary,
    cursor: 'pointer',
    fontSize: font.sm,
  },
  deleteBtn: {
    padding: '4px 10px',
    borderRadius: radius.sm,
    border: '1px solid #5a3a3a',
    backgroundColor: 'transparent',
    color: colors.danger,
    cursor: 'pointer',
    fontSize: font.sm,
  },
  // 策略列表样式
  policyList: {
    display: 'flex',
    flexDirection: 'column',
    gap: '8px',
    marginTop: '12px',
  },
  policyItem: {
    border: `1px solid ${colors.borderPrimary}`,
    borderRadius: radius.md,
    overflow: 'hidden',
    backgroundColor: colors.bgSecondary,
  },
  policyHeader: {
    padding: '12px 16px',
    backgroundColor: colors.bgTertiary,
    cursor: 'pointer',
    display: 'flex',
    justifyContent: 'space-between',
    alignItems: 'center',
  },
  policyInfo: {
    display: 'flex',
    flexDirection: 'column',
    gap: '4px',
  },
  policyName: {
    color: colors.textPrimary,
    fontSize: font.base,
    fontWeight: 500,
  },
  policyMeta: {
    color: colors.textTertiary,
    fontSize: font.xs,
  },
  metaSeparator: {
    margin: '0 8px',
  },
  policyActions: {
    display: 'flex',
    gap: '8px',
    alignItems: 'center',
  },
  expandIcon: {
    color: colors.textTertiary,
    fontSize: font.sm,
    marginLeft: '4px',
  },
  policyContent: {
    padding: '16px',
    borderTop: `1px solid ${colors.borderPrimary}`,
    backgroundColor: colors.bgPrimary,
  },
  policyDesc: {
    color: colors.textTertiary,
    fontSize: font.sm,
    marginBottom: '12px',
  },
  commandSection: {
    display: 'flex',
    flexDirection: 'column',
    gap: '8px',
  },
  commandSectionTitle: {
    color: colors.textSecondary,
    fontSize: font.sm,
    fontWeight: 500,
  },
  emptyText: {
    color: colors.textMuted,
    fontSize: font.sm,
    padding: '12px',
    textAlign: 'center',
  },
  commandList: {
    border: `1px solid ${colors.borderPrimary}`,
    borderRadius: radius.sm,
    overflow: 'hidden',
    maxHeight: '200px',
    overflowY: 'auto',
  },
  commandItem: {
    padding: '10px 12px',
    borderBottom: `1px solid ${colors.bgTertiary}`,
    display: 'flex',
    justifyContent: 'space-between',
    alignItems: 'center',
    backgroundColor: colors.bgPrimary,
  },
  commandInfo: {
    display: 'flex',
    alignItems: 'center',
    gap: '8px',
    flex: 1,
    minWidth: 0,
  },
  commandPattern: {
    color: 'var(--accent)',
    fontSize: font.sm,
    fontFamily: 'var(--font-mono)',
  },
  categoryBadge: {
    padding: '2px 6px',
    borderRadius: '3px',
    color: colors.textPrimary,
    fontSize: '10px',
    fontWeight: 500,
    flexShrink: 0,
  },
  commandDesc: {
    color: colors.textTertiary,
    fontSize: font.xs,
    overflow: 'hidden',
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
  },
  // 保存状态样式
  saveStatus: {
    position: 'fixed',
    bottom: '20px',
    right: '20px',
    padding: '8px 16px',
    borderRadius: radius.full,
    fontSize: font.sm,
    fontWeight: 500,
    zIndex: 1000,
    transition: 'opacity 0.3s',
  },
  saveStatusSaving: {
    backgroundColor: 'var(--info-bg-subtle)',
    color: 'var(--accent)',
  },
  saveStatusSaved: {
    backgroundColor: 'var(--success-bg-subtle)',
    color: colors.success,
  },
  saveStatusError: {
    backgroundColor: 'var(--danger-bg-subtle)',
    color: colors.danger,
  },
};

// 编辑器模态框样式
const editorStyles: Record<string, React.CSSProperties> = {
  overlay: {
    position: 'fixed',
    top: 0,
    left: 0,
    right: 0,
    bottom: 0,
    backgroundColor: colors.overlay,
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
    zIndex: 2100,
  },
  modal: {
    backgroundColor: colors.bgSecondary,
    borderRadius: radius.lg,
    width: '560px',
    maxHeight: '80vh',
    display: 'flex',
    flexDirection: 'column',
    boxShadow: '0 4px 12px rgba(0, 0, 0, 0.5)',
  },
  header: {
    padding: '16px 20px',
    borderBottom: `1px solid ${colors.borderPrimary}`,
    display: 'flex',
    justifyContent: 'space-between',
    alignItems: 'center',
    backgroundColor: colors.bgPrimary,
  },
  title: {
    margin: 0,
    fontSize: '15px',
    color: colors.textPrimary,
    fontWeight: 600,
  },
  closeBtn: {
    background: 'none',
    border: 'none',
    color: colors.textSecondary,
    fontSize: '20px',
    cursor: 'pointer',
    padding: '0',
    width: '28px',
    height: '28px',
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
    borderRadius: radius.sm,
  },
  body: {
    flex: 1,
    overflowY: 'auto',
    padding: '20px',
    display: 'flex',
    flexDirection: 'column',
    gap: '16px',
  },
  field: {
    display: 'flex',
    flexDirection: 'column',
    gap: '6px',
  },
  label: {
    color: colors.textSecondary,
    fontSize: font.base,
    fontWeight: 500,
  },
  hint: {
    color: colors.textTertiary,
    fontSize: font.xs,
  },
  hintRow: {
    display: 'flex',
    alignItems: 'center',
    gap: '6px',
    color: colors.textTertiary,
    fontSize: font.xs,
  },
  helpIcon: {
    display: 'inline-flex',
    alignItems: 'center',
    justifyContent: 'center',
    width: '14px',
    height: '14px',
    borderRadius: '50%',
    backgroundColor: colors.borderPrimary,
    color: colors.textTertiary,
    fontSize: '10px',
    fontWeight: 'bold',
    cursor: 'pointer',
    userSelect: 'none',
  },
  helpBox: {
    marginTop: '8px',
    padding: '12px',
    backgroundColor: colors.bgPrimary,
    border: `1px solid ${colors.borderPrimary}`,
    borderRadius: radius.md,
    fontSize: font.sm,
  },
  helpTitle: {
    color: colors.textPrimary,
    fontWeight: 600,
    marginBottom: '8px',
  },
  helpContent: {
    color: colors.textSecondary,
    lineHeight: 1.6,
  },
  helpExample: {
    marginTop: '8px',
    padding: '8px',
    backgroundColor: colors.bgSecondary,
    borderRadius: radius.sm,
    fontFamily: 'var(--font-mono)',
    fontSize: font.xs,
  },
  input: {
    padding: '8px 12px',
    borderRadius: radius.sm,
    border: `1px solid ${colors.borderPrimary}`,
    backgroundColor: colors.bgPrimary,
    color: colors.textPrimary,
    outline: 'none',
    fontSize: font.base,
  },
  select: {
    padding: '8px 12px',
    borderRadius: radius.sm,
    border: `1px solid ${colors.borderPrimary}`,
    backgroundColor: colors.bgPrimary,
    color: colors.textPrimary,
    outline: 'none',
    fontSize: font.base,
    cursor: 'pointer',
    width: '100px',
  },
  inputRow: {
    display: 'flex',
    gap: '8px',
  },
  addBtn: {
    padding: '8px 16px',
    borderRadius: radius.sm,
    border: 'none',
    backgroundColor: colors.accent,
    color: colors.textPrimary,
    cursor: 'pointer',
    fontSize: font.base,
    whiteSpace: 'nowrap',
  },
  addBtnDisabled: {
    backgroundColor: 'var(--border-strong)',
    cursor: 'not-allowed',
  },
  tagList: {
    display: 'flex',
    flexWrap: 'wrap',
    gap: '8px',
    marginTop: '8px',
  },
  tag: {
    padding: '4px 8px',
    backgroundColor: 'var(--info-bg-subtle)',
    borderRadius: radius.sm,
    color: 'var(--accent)',
    fontSize: font.sm,
    display: 'flex',
    alignItems: 'center',
    gap: '4px',
  },
  tagRemove: {
    background: 'none',
    border: 'none',
    color: colors.textTertiary,
    cursor: 'pointer',
    fontSize: font.lg,
    padding: '0',
    lineHeight: 1,
  },
  commandList: {
    border: `1px solid ${colors.borderPrimary}`,
    borderRadius: radius.sm,
    marginBottom: '12px',
    maxHeight: '180px',
    overflowY: 'auto',
  },
  commandItem: {
    padding: '8px 12px',
    borderBottom: `1px solid ${colors.bgTertiary}`,
    display: 'flex',
    justifyContent: 'space-between',
    alignItems: 'center',
    backgroundColor: colors.bgPrimary,
  },
  commandInfo: {
    display: 'flex',
    alignItems: 'center',
    gap: '8px',
    flex: 1,
    minWidth: 0,
  },
  commandPattern: {
    color: 'var(--accent)',
    fontSize: font.xs,
    fontFamily: 'var(--font-mono)',
  },
  categoryBadge: {
    padding: '2px 6px',
    borderRadius: '3px',
    fontSize: '10px',
    fontWeight: 500,
    flexShrink: 0,
  },
  removeBtn: {
    padding: '2px 8px',
    borderRadius: radius.sm,
    border: '1px solid #5a3a3a',
    backgroundColor: 'transparent',
    color: colors.danger,
    cursor: 'pointer',
    fontSize: font.xs,
  },
  emptyText: {
    padding: '16px',
    color: colors.textMuted,
    fontSize: font.sm,
    textAlign: 'center',
  },
  addCommandSection: {
    backgroundColor: colors.bgTertiary,
    padding: '12px',
    borderRadius: radius.sm,
    display: 'flex',
    flexDirection: 'column',
    gap: '8px',
  },
  footer: {
    padding: '16px 20px',
    borderTop: `1px solid ${colors.borderPrimary}`,
    display: 'flex',
    justifyContent: 'flex-end',
    gap: '12px',
    backgroundColor: colors.bgPrimary,
  },
  cancelBtn: {
    padding: '8px 20px',
    borderRadius: radius.sm,
    border: `1px solid ${colors.borderPrimary}`,
    backgroundColor: 'transparent',
    color: colors.textSecondary,
    cursor: 'pointer',
    fontSize: font.base,
  },
  saveBtn: {
    padding: '8px 20px',
    borderRadius: radius.sm,
    border: 'none',
    backgroundColor: colors.accent,
    color: colors.textPrimary,
    cursor: 'pointer',
    fontSize: font.base,
    fontWeight: 500,
  },
};

export default CommandWhitelistPanel;
