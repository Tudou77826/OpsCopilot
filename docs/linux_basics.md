# 基础运维命令速查

## 磁盘空间清理
当磁盘报警时，使用以下命令快速定位大文件：
```bash
# 查找当前目录下大于 100M 的文件
find . -type f -size +100M
```

## 网络连接分析
查看特定端口的连接情况：
```bash
# 查看 8080 端口的并发连接数
netstat -nat | grep -i "8080" | wc -l
```

## 日志分析技巧
统计日志中出现频率最高的 IP：
```bash
awk '{print $1}' access.log | sort | uniq -c | sort -nr | head -n 10
```
