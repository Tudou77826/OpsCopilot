---
service: Network Service
module: 基础网络
---

# 网络故障排查手册

## 1. 概述
本手册用于指导运维人员排查常见的网络连接问题，包括丢包、高延迟、连接超时等。

## 2. 常用排查工具
- `ping`: 检查连通性和延迟
- `traceroute` / `tracert`: 路由追踪
- `netstat`: 查看端口状态
- `tcpdump`: 抓包分析

## 3. 常见场景与处理

### 场景：服务器无法连接公网

- **现象**: ping 8.8.8.8 超时
- **关键词**: ping, 公网, 无法上网, 外网, 路由, DNS
- **涉及组件**: 网卡, 路由器, DNS

**排查步骤**:
1. 检查网卡状态: `ip link show`
2. 检查路由表: `ip route show`，确认是否有 default gateway。
3. 检查 DNS: `cat /etc/resolv.conf`。

### 场景：日志源开关未开启

- **现象**: 日志源开关下发失败，关闭
- **关键词**: 日志源, 开关, 下发失败, rsyslog
- **涉及组件**: rsyslog, 日志采集

**排查步骤**:
1. 检查日志源开关状态: `ip link show`
2. 检查日志源状态表: `ip route show`，确认是否有 default gateway。
3. 检查 DNS: `cat /etc/resolv.conf`。

### 场景：应用端口无法访问

- **现象**: 客户端报错 Connection Refused
- **关键词**: connection refused, 端口, 防火墙, 安全组
- **涉及组件**: iptables, ufw

**排查步骤**:
1. 在服务器上检查端口监听: `netstat -tulpn | grep <PORT>`
2. 检查防火墙: `iptables -L` 或 `ufw status`
3. 检查云厂商安全组设置。

## 4. 关键命令示例
```bash
# 检查 80 端口占用
netstat -tulpn | grep :80

# 抓取 eth0 网卡的 80 端口流量
tcpdump -i eth0 port 80 -n -vv
```
