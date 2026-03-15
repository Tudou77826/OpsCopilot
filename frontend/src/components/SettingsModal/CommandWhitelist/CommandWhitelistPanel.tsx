import React, { useState, useEffect, useRef, useCallback } from 'react';
import { WhitelistConfig, Policy, Command, RiskAssessment } from './types';

interface CommandWhitelistPanelProps {
  onSave?: () => void;
}

const CommandWhitelistPanel: React.FC<CommandWhitelistPanelProps> = ({ onSave }) => {
  const [config, setConfig] = useState<WhitelistConfig | null>(null);
  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState(false);
  const [saveStatus, setSaveStatus] = useState<'idle' | 'saving' | 'saved' | 'error'>('idle');
  const [expandedPolicy, setExpandedPolicy] = useState<string | null>(null);
  const [editingPolicy, setEditingPolicy] = useState<Policy | null>(null);
  const [testingCommand, setTestingCommand] = useState('');
  const [testResult, setTestResult] = useState<RiskAssessment | null>(null);
  const [testing, setTesting] = useState(false);

  // 用于防抖保存
  const saveTimeoutRef = useRef<NodeJS.Timeout | null>(null);
  const initialLoadRef = useRef(true);

  useEffect(() => {
    loadConfig();
  }, []);

  // 自动保存（防抖）
  useEffect(() => {
    // 跳过初始加载
    if (initialLoadRef.current) {
      initialLoadRef.current = false;
      return;
    }
    if (!config) return;

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
    } catch (err) {
      console.error('加载配置失败:', err);
    } finally {
      setLoading(false);
    }
  };

  const handleToggleLLMCheck = () => {
    if (!config) return;
    setConfig({ ...config, llm_check_enabled: !config.llm_check_enabled });
  };

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

  const handleTestCommand = async () => {
    if (!testingCommand.trim()) return;
    setTesting(true);
    setTestResult(null);
    try {
      // @ts-ignore
      const result = await window.go.main.App.AssessCommandRisk(testingCommand);
      setTestResult(result);
    } catch (err) {
      setTestResult({
        is_risky: true,
        risk_level: 'high',
        reason: `评估失败: ${err}`,
        suggestions: '',
      });
    } finally {
      setTesting(false);
    }
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

  const getRiskColor = (level: string) => {
    switch (level) {
      case 'low': return '#4caf50';
      case 'medium': return '#ff9800';
      case 'high': return '#f44336';
      default: return '#9e9e9e';
    }
  };

  const getRiskBgColor = (level: string) => {
    switch (level) {
      case 'low': return '#1a2a24';
      case 'medium': return '#2a2518';
      case 'high': return '#2a1818';
      default: return '#252526';
    }
  };

  const getCategoryColor = (category: string) => {
    return category === 'read_only' ? '#4caf50' : '#ff9800';
  };

  if (loading) {
    return <div style={styles.loading}>加载中...</div>;
  }

  if (!config) {
    return <div style={styles.loading}>无法加载配置</div>;
  }

  return (
    <div style={styles.container}>
      {/* 全局设置 */}
      <div style={styles.section}>
        <div style={styles.sectionHeader}>
          <span style={styles.sectionTitle}>LLM 风险检查</span>
          <label style={styles.switch}>
            <input
              type="checkbox"
              checked={config.llm_check_enabled}
              onChange={handleToggleLLMCheck}
            />
            <span style={styles.slider}></span>
          </label>
        </div>
        <div style={styles.sectionDesc}>
          启用后，对于不在白名单中的命令，将使用 LLM 进行风险评估
        </div>
      </div>

      {/* 命令测试区域 */}
      <div style={styles.section}>
        <div style={styles.sectionTitle}>命令风险测试</div>
        <div style={styles.sectionDesc}>
          输入命令测试 LLM 风险评估功能
        </div>
        <div style={styles.testInputRow}>
          <input
            type="text"
            value={testingCommand}
            onChange={(e) => setTestingCommand(e.target.value)}
            onKeyDown={(e) => e.key === 'Enter' && handleTestCommand()}
            placeholder="输入命令进行风险评估..."
            style={styles.input}
          />
          <button
            onClick={handleTestCommand}
            disabled={testing || !testingCommand.trim()}
            style={{
              ...styles.primaryBtn,
              ...(testing || !testingCommand.trim() ? styles.primaryBtnDisabled : {}),
            }}
          >
            {testing ? '评估中...' : '评估'}
          </button>
        </div>
        {testResult && (
          <div style={{
            ...styles.testResult,
            borderColor: getRiskColor(testResult.risk_level),
            backgroundColor: getRiskBgColor(testResult.risk_level),
          }}>
            <div style={styles.testResultHeader}>
              <span style={{
                ...styles.riskBadge,
                backgroundColor: getRiskColor(testResult.risk_level),
              }}>
                {testResult.risk_level.toUpperCase()}
              </span>
              <span style={styles.testResultReason}>{testResult.reason}</span>
            </div>
            {testResult.suggestions && (
              <div style={styles.testResultSuggestion}>
                {testResult.suggestions}
              </div>
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
                            <label style={styles.switchSmall}>
                              <input
                                type="checkbox"
                                checked={cmd.enabled}
                                onChange={() => toggleCommand(policy.id, idx)}
                              />
                              <span style={styles.sliderSmall}></span>
                            </label>
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
                      backgroundColor: cmd.category === 'read_only' ? '#2e5a3a' : '#5a4a2e',
                      color: cmd.category === 'read_only' ? '#8fdf9a' : '#f0c060',
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
                  style={{ ...editorStyles.input, fontFamily: 'monospace' }}
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
    color: '#888',
    fontSize: '14px',
  },
  section: {
    padding: '16px',
    backgroundColor: '#1e1e1e',
    borderRadius: '6px',
    border: '1px solid #3c3c3c',
  },
  sectionHeader: {
    display: 'flex',
    justifyContent: 'space-between',
    alignItems: 'center',
  },
  sectionTitle: {
    color: '#fff',
    fontSize: '14px',
    fontWeight: 600,
  },
  sectionDesc: {
    color: '#888',
    fontSize: '12px',
    marginTop: '8px',
  },
  toolbar: {
    display: 'flex',
    justifyContent: 'space-between',
    alignItems: 'center',
  },
  // Switch 样式
  switch: {
    position: 'relative',
    display: 'inline-block',
    width: '40px',
    height: '20px',
    flexShrink: 0,
  },
  slider: {
    position: 'absolute',
    cursor: 'pointer',
    top: 0,
    left: 0,
    right: 0,
    bottom: 0,
    backgroundColor: '#555',
    transition: '0.3s',
    borderRadius: '20px',
  },
  switchSmall: {
    position: 'relative',
    display: 'inline-block',
    width: '32px',
    height: '16px',
    flexShrink: 0,
  },
  sliderSmall: {
    position: 'absolute',
    cursor: 'pointer',
    top: 0,
    left: 0,
    right: 0,
    bottom: 0,
    backgroundColor: '#555',
    transition: '0.3s',
    borderRadius: '16px',
  },
  // 输入框样式
  input: {
    flex: 1,
    padding: '8px 12px',
    borderRadius: '4px',
    border: '1px solid #3c3c3c',
    backgroundColor: '#252526',
    color: '#fff',
    outline: 'none',
    fontSize: '13px',
  },
  testInputRow: {
    display: 'flex',
    gap: '8px',
    marginTop: '12px',
  },
  testResult: {
    marginTop: '12px',
    padding: '12px',
    borderRadius: '4px',
    border: '1px solid',
  },
  testResultHeader: {
    display: 'flex',
    alignItems: 'center',
    gap: '10px',
  },
  riskBadge: {
    padding: '2px 8px',
    borderRadius: '4px',
    color: '#fff',
    fontSize: '11px',
    fontWeight: 600,
  },
  testResultReason: {
    color: '#ccc',
    fontSize: '13px',
  },
  testResultSuggestion: {
    marginTop: '8px',
    color: '#888',
    fontSize: '12px',
  },
  // 按钮样式
  primaryBtn: {
    padding: '8px 16px',
    borderRadius: '4px',
    border: 'none',
    backgroundColor: '#007acc',
    color: '#fff',
    cursor: 'pointer',
    fontSize: '13px',
    fontWeight: 500,
  },
  primaryBtnDisabled: {
    backgroundColor: '#444',
    cursor: 'not-allowed',
  },
  editBtn: {
    padding: '4px 10px',
    borderRadius: '4px',
    border: '1px solid #5a5a5a',
    backgroundColor: '#3c3c3c',
    color: '#fff',
    cursor: 'pointer',
    fontSize: '12px',
  },
  deleteBtn: {
    padding: '4px 10px',
    borderRadius: '4px',
    border: '1px solid #5a3a3a',
    backgroundColor: 'transparent',
    color: '#f44336',
    cursor: 'pointer',
    fontSize: '12px',
  },
  // 策略列表样式
  policyList: {
    display: 'flex',
    flexDirection: 'column',
    gap: '8px',
    marginTop: '12px',
  },
  policyItem: {
    border: '1px solid #3c3c3c',
    borderRadius: '6px',
    overflow: 'hidden',
    backgroundColor: '#252526',
  },
  policyHeader: {
    padding: '12px 16px',
    backgroundColor: '#2d2d2d',
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
    color: '#fff',
    fontSize: '13px',
    fontWeight: 500,
  },
  policyMeta: {
    color: '#888',
    fontSize: '11px',
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
    color: '#888',
    fontSize: '12px',
    marginLeft: '4px',
  },
  policyContent: {
    padding: '16px',
    borderTop: '1px solid #3c3c3c',
    backgroundColor: '#1e1e1e',
  },
  policyDesc: {
    color: '#888',
    fontSize: '12px',
    marginBottom: '12px',
  },
  commandSection: {
    display: 'flex',
    flexDirection: 'column',
    gap: '8px',
  },
  commandSectionTitle: {
    color: '#ccc',
    fontSize: '12px',
    fontWeight: 500,
  },
  emptyText: {
    color: '#666',
    fontSize: '12px',
    padding: '12px',
    textAlign: 'center',
  },
  commandList: {
    border: '1px solid #3c3c3c',
    borderRadius: '4px',
    overflow: 'hidden',
    maxHeight: '200px',
    overflowY: 'auto',
  },
  commandItem: {
    padding: '10px 12px',
    borderBottom: '1px solid #2d2d2d',
    display: 'flex',
    justifyContent: 'space-between',
    alignItems: 'center',
    backgroundColor: '#1e1e1e',
  },
  commandInfo: {
    display: 'flex',
    alignItems: 'center',
    gap: '8px',
    flex: 1,
    minWidth: 0,
  },
  commandPattern: {
    color: '#9cdcfe',
    fontSize: '12px',
    fontFamily: 'monospace',
  },
  categoryBadge: {
    padding: '2px 6px',
    borderRadius: '3px',
    color: '#fff',
    fontSize: '10px',
    fontWeight: 500,
    flexShrink: 0,
  },
  commandDesc: {
    color: '#888',
    fontSize: '11px',
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
    borderRadius: '20px',
    fontSize: '12px',
    fontWeight: 500,
    zIndex: 1000,
    transition: 'opacity 0.3s',
  },
  saveStatusSaving: {
    backgroundColor: '#2d3a4a',
    color: '#9cdcfe',
  },
  saveStatusSaved: {
    backgroundColor: '#1a2a24',
    color: '#4caf50',
  },
  saveStatusError: {
    backgroundColor: '#2a1818',
    color: '#f44336',
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
    backgroundColor: 'rgba(0, 0, 0, 0.7)',
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
    zIndex: 2100,
  },
  modal: {
    backgroundColor: '#252526',
    borderRadius: '8px',
    width: '560px',
    maxHeight: '80vh',
    display: 'flex',
    flexDirection: 'column',
    boxShadow: '0 4px 12px rgba(0, 0, 0, 0.5)',
  },
  header: {
    padding: '16px 20px',
    borderBottom: '1px solid #3c3c3c',
    display: 'flex',
    justifyContent: 'space-between',
    alignItems: 'center',
    backgroundColor: '#1e1e1e',
  },
  title: {
    margin: 0,
    fontSize: '15px',
    color: '#fff',
    fontWeight: 600,
  },
  closeBtn: {
    background: 'none',
    border: 'none',
    color: '#ccc',
    fontSize: '20px',
    cursor: 'pointer',
    padding: '0',
    width: '28px',
    height: '28px',
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
    borderRadius: '4px',
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
    color: '#ccc',
    fontSize: '13px',
    fontWeight: 500,
  },
  hint: {
    color: '#888',
    fontSize: '11px',
  },
  hintRow: {
    display: 'flex',
    alignItems: 'center',
    gap: '6px',
    color: '#888',
    fontSize: '11px',
  },
  helpIcon: {
    display: 'inline-flex',
    alignItems: 'center',
    justifyContent: 'center',
    width: '14px',
    height: '14px',
    borderRadius: '50%',
    backgroundColor: '#3c3c3c',
    color: '#aaa',
    fontSize: '10px',
    fontWeight: 'bold',
    cursor: 'pointer',
    userSelect: 'none',
  },
  helpBox: {
    marginTop: '8px',
    padding: '12px',
    backgroundColor: '#1e1e1e',
    border: '1px solid #3c3c3c',
    borderRadius: '6px',
    fontSize: '12px',
  },
  helpTitle: {
    color: '#fff',
    fontWeight: 600,
    marginBottom: '8px',
  },
  helpContent: {
    color: '#ccc',
    lineHeight: 1.6,
  },
  helpExample: {
    marginTop: '8px',
    padding: '8px',
    backgroundColor: '#252526',
    borderRadius: '4px',
    fontFamily: 'monospace',
    fontSize: '11px',
  },
  input: {
    padding: '8px 12px',
    borderRadius: '4px',
    border: '1px solid #3c3c3c',
    backgroundColor: '#1e1e1e',
    color: '#fff',
    outline: 'none',
    fontSize: '13px',
  },
  select: {
    padding: '8px 12px',
    borderRadius: '4px',
    border: '1px solid #3c3c3c',
    backgroundColor: '#1e1e1e',
    color: '#fff',
    outline: 'none',
    fontSize: '13px',
    cursor: 'pointer',
    width: '100px',
  },
  inputRow: {
    display: 'flex',
    gap: '8px',
  },
  addBtn: {
    padding: '8px 16px',
    borderRadius: '4px',
    border: 'none',
    backgroundColor: '#007acc',
    color: '#fff',
    cursor: 'pointer',
    fontSize: '13px',
    whiteSpace: 'nowrap',
  },
  addBtnDisabled: {
    backgroundColor: '#444',
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
    backgroundColor: '#2d3a4a',
    borderRadius: '4px',
    color: '#9cdcfe',
    fontSize: '12px',
    display: 'flex',
    alignItems: 'center',
    gap: '4px',
  },
  tagRemove: {
    background: 'none',
    border: 'none',
    color: '#888',
    cursor: 'pointer',
    fontSize: '14px',
    padding: '0',
    lineHeight: 1,
  },
  commandList: {
    border: '1px solid #3c3c3c',
    borderRadius: '4px',
    marginBottom: '12px',
    maxHeight: '180px',
    overflowY: 'auto',
  },
  commandItem: {
    padding: '8px 12px',
    borderBottom: '1px solid #2d2d2d',
    display: 'flex',
    justifyContent: 'space-between',
    alignItems: 'center',
    backgroundColor: '#1e1e1e',
  },
  commandInfo: {
    display: 'flex',
    alignItems: 'center',
    gap: '8px',
    flex: 1,
    minWidth: 0,
  },
  commandPattern: {
    color: '#9cdcfe',
    fontSize: '11px',
    fontFamily: 'monospace',
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
    borderRadius: '4px',
    border: '1px solid #5a3a3a',
    backgroundColor: 'transparent',
    color: '#f44336',
    cursor: 'pointer',
    fontSize: '11px',
  },
  emptyText: {
    padding: '16px',
    color: '#666',
    fontSize: '12px',
    textAlign: 'center',
  },
  addCommandSection: {
    backgroundColor: '#2d2d2d',
    padding: '12px',
    borderRadius: '4px',
    display: 'flex',
    flexDirection: 'column',
    gap: '8px',
  },
  footer: {
    padding: '16px 20px',
    borderTop: '1px solid #3c3c3c',
    display: 'flex',
    justifyContent: 'flex-end',
    gap: '12px',
    backgroundColor: '#1e1e1e',
  },
  cancelBtn: {
    padding: '8px 20px',
    borderRadius: '4px',
    border: '1px solid #5a5a5a',
    backgroundColor: 'transparent',
    color: '#ccc',
    cursor: 'pointer',
    fontSize: '13px',
  },
  saveBtn: {
    padding: '8px 20px',
    borderRadius: '4px',
    border: 'none',
    backgroundColor: '#007acc',
    color: '#fff',
    cursor: 'pointer',
    fontSize: '13px',
    fontWeight: 500,
  },
};

export default CommandWhitelistPanel;
