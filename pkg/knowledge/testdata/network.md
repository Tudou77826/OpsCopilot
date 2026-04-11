---
service: Network Service
module: 基础网络
---

# 网络排查手册

## 场景：无法连接公网

- **现象**: ping 8.8.8.8 超时
- **关键词**: ping, 公网, 无法上网, 外网, 路由
- **涉及组件**: 网卡, 路由器, DNS

**排查步骤**:
1. 检查网络连通性
   ```bash
   ping -c 4 8.8.8.8
   ```

## 场景：应用端口无法访问

- **现象**: 客户端报错 Connection Refused
- **关键词**: connection refused, 端口, 防火墙, 安全组
- **涉及组件**: iptables, ufw

**排查步骤**:
1. 检查端口监听
   ```bash
   ss -tlnp | grep :8080
   ```
