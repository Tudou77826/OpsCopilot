# 网络故障排查手册

## 1. 概述
本手册用于指导运维人员排查常见的网络连接问题，包括丢包、高延迟、连接超时等。

## 2. 常用排查工具
- `ping`: 检查连通性和延迟
- `traceroute` / `tracert`: 路由追踪
- `netstat`: 查看端口状态
- `tcpdump`: 抓包分析

## 3. 常见场景与处理

### 场景一：服务器无法连接公网
**现象**: `ping 8.8.8.8` 超时。
**排查步骤**:
1. 检查网卡状态: `ip link show`
2. 检查路由表: `ip route show`，确认是否有 default gateway。
3. 检查 DNS: `cat /etc/resolv.conf`。

### 场景二：应用端口无法访问
**现象**: 客户端报错 Connection Refused。
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
