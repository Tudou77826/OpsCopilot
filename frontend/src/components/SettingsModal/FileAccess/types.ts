// 文件访问控制相关类型定义

// FileAccessPolicy 文件访问策略
export interface FileAccessPolicy {
  id: string;
  name: string;
  ip_ranges: string[];
  read_paths: string[];
  write_paths: string[];
  denied_paths: string[];
  allowed_local_dirs: string[];
  max_read_bytes: number;
  max_write_bytes: number;
}

// FileAccessConfig 文件访问控制配置
export interface FileAccessConfig {
  version: string;
  policies: FileAccessPolicy[];
}

// 默认配置
export const DEFAULT_FILE_ACCESS_CONFIG: FileAccessConfig = {
  version: '1.0',
  policies: [
    {
      id: 'default',
      name: 'Default File Access',
      ip_ranges: ['*'],
      read_paths: ['/var/log/', '/etc/', '/tmp/', '/home/', '/opt/', '/srv/'],
      write_paths: [],
      denied_paths: ['/etc/shadow', '/etc/ssh/', '/root/.ssh/', '/home/*/.ssh/id_*'],
      allowed_local_dirs: ['/tmp/opscopilot-mcp/'],
      max_read_bytes: 10 * 1024 * 1024,
      max_write_bytes: 5 * 1024 * 1024,
    },
  ],
};
