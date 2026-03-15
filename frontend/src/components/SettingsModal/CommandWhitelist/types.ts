// 命令白名单相关类型定义

// CommandCategory 命令分类
export type CommandCategory = 'read_only' | 'write';

// RiskLevel 风险等级
export type RiskLevel = 'low' | 'medium' | 'high';

// Command 命令规则
export interface Command {
  pattern: string;
  category: CommandCategory;
  description: string;
  enabled: boolean;
}

// Policy 策略（按 IP 段区分）
export interface Policy {
  id: string;
  name: string;
  description: string;
  ip_ranges: string[];
  commands: Command[];
}

// WhitelistConfig 白名单配置
export interface WhitelistConfig {
  version: string;
  llm_check_enabled: boolean;
  policies: Policy[];
}

// RiskAssessment LLM 风险评估结果
export interface RiskAssessment {
  is_risky: boolean;
  risk_level: RiskLevel;
  reason: string;
  suggestions: string;
}

// 默认配置
export const DEFAULT_WHITELIST_CONFIG: WhitelistConfig = {
  version: '1.0',
  llm_check_enabled: true,
  policies: [
    {
      id: 'default',
      name: '默认策略',
      description: '适用于所有服务器的只读命令策略',
      ip_ranges: ['*'],
      commands: getDefaultCommands(),
    },
  ],
};

function getDefaultCommands(): Command[] {
  return [
    // === 文件查看 ===
    { pattern: '^ls(\\s|$)', category: 'read_only', description: '列出目录', enabled: true },
    { pattern: '^cat\\s', category: 'read_only', description: '查看文件', enabled: true },
    { pattern: '^head\\s', category: 'read_only', description: '查看文件头部', enabled: true },
    { pattern: '^tail\\s', category: 'read_only', description: '查看文件尾部', enabled: true },
    { pattern: '^find\\s', category: 'read_only', description: '查找文件', enabled: true },
    { pattern: '^tree\\s', category: 'read_only', description: '目录树', enabled: true },
    { pattern: '^stat\\s', category: 'read_only', description: '文件状态', enabled: true },
    { pattern: '^wc\\s', category: 'read_only', description: '统计行数', enabled: true },

    // === 文本处理 ===
    { pattern: '^grep\\s', category: 'read_only', description: '搜索文本', enabled: true },
    { pattern: '^awk\\s', category: 'read_only', description: '文本处理', enabled: true },
    { pattern: '^sed\\s+-n', category: 'read_only', description: '流编辑（只读模式）', enabled: true },
    { pattern: '^sort\\s', category: 'read_only', description: '排序', enabled: true },
    { pattern: '^uniq\\s', category: 'read_only', description: '去重', enabled: true },
    { pattern: '^cut\\s', category: 'read_only', description: '剪切文本', enabled: true },
    { pattern: '^jq\\s', category: 'read_only', description: 'JSON 处理', enabled: true },

    // === 进程 ===
    { pattern: '^ps\\s', category: 'read_only', description: '进程列表', enabled: true },
    { pattern: '^top(\\s|$)', category: 'read_only', description: '进程监控', enabled: true },
    { pattern: '^pgrep\\s', category: 'read_only', description: '进程搜索', enabled: true },
    { pattern: '^pstree\\s', category: 'read_only', description: '进程树', enabled: true },
    { pattern: '^lsof\\s', category: 'read_only', description: '打开文件列表', enabled: true },

    // === 系统资源 ===
    { pattern: '^free\\s', category: 'read_only', description: '内存信息', enabled: true },
    { pattern: '^df\\s', category: 'read_only', description: '磁盘使用', enabled: true },
    { pattern: '^du\\s', category: 'read_only', description: '目录大小', enabled: true },
    { pattern: '^uptime(\\s|$)', category: 'read_only', description: '系统运行时间', enabled: true },
    { pattern: '^iostat\\s', category: 'read_only', description: 'IO 统计', enabled: true },
    { pattern: '^vmstat\\s', category: 'read_only', description: '虚拟内存统计', enabled: true },
    { pattern: '^uname(\\s|$)', category: 'read_only', description: '系统信息', enabled: true },

    // === 网络 ===
    { pattern: '^netstat\\s', category: 'read_only', description: '网络连接', enabled: true },
    { pattern: '^ss\\s', category: 'read_only', description: 'Socket 统计', enabled: true },
    { pattern: '^ip\\s', category: 'read_only', description: '网络配置', enabled: true },
    { pattern: '^ping\\s', category: 'read_only', description: '网络连通性', enabled: true },
    { pattern: '^curl\\s+.*-I', category: 'read_only', description: 'HTTP 头检查', enabled: true },
    { pattern: '^nslookup\\s', category: 'read_only', description: 'DNS 查询', enabled: true },

    // === 服务/日志 ===
    { pattern: '^systemctl\\s+status', category: 'read_only', description: '服务状态', enabled: true },
    { pattern: '^journalctl\\s', category: 'read_only', description: '系统日志', enabled: true },
    { pattern: '^dmesg(\\s|$)', category: 'read_only', description: '内核日志', enabled: true },

    // === 容器 ===
    { pattern: '^docker\\s+(ps|images|logs|inspect|stats)', category: 'read_only', description: 'Docker 查询', enabled: true },
    { pattern: '^kubectl\\s+(get|describe|logs|top)', category: 'read_only', description: 'Kubernetes 查询', enabled: true },
    { pattern: '^crictl\\s+(ps|logs|pods)', category: 'read_only', description: 'CRI 查询', enabled: true },

    // === Java ===
    { pattern: '^jps(\\s|$)', category: 'read_only', description: 'Java 进程', enabled: true },
    { pattern: '^jstat\\s', category: 'read_only', description: 'JVM 统计', enabled: true },
    { pattern: '^jinfo\\s', category: 'read_only', description: 'JVM 配置', enabled: true },
    { pattern: '^jstack\\s', category: 'read_only', description: 'Java 线程栈', enabled: true },
    { pattern: '^jmap\\s+-histo', category: 'read_only', description: 'Java 堆统计', enabled: true },

    // === 通用 ===
    { pattern: '^echo\\s', category: 'read_only', description: '输出文本', enabled: true },
    { pattern: '^which\\s', category: 'read_only', description: '查找命令', enabled: true },
    { pattern: '^hostname(\\s|$)', category: 'read_only', description: '主机名', enabled: true },
    { pattern: '^whoami(\\s|$)', category: 'read_only', description: '当前用户', enabled: true },
    { pattern: '^id(\\s|$)', category: 'read_only', description: '用户 ID', enabled: true },
    { pattern: '^date(\\s|$)', category: 'read_only', description: '日期时间', enabled: true },
    { pattern: '^env(\\s|$)', category: 'read_only', description: '环境变量', enabled: true },
    { pattern: '^pwd(\\s|$)', category: 'read_only', description: '当前目录', enabled: true },
  ];
}
