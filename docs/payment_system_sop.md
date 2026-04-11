---
service: Payment Service
module: 核心支付模块
---

# 支付系统架构与排查手册

## 1. 系统架构
支付系统 (Payment Service) 主要由以下模块组成：
- **API Gateway (Nginx)**: 接收外部请求，反向代理到后端服务。
- **Core Service (Go)**: 处理核心支付逻辑，状态流转。
- **Database (MySQL)**: 存储订单数据。
- **Cache (Redis)**: 缓存热点数据和分布式锁。

## 2. 常见问题排查 SOP

### 场景：API 接口超时 (504 Gateway Timeout)

- **所属模块**: 核心支付模块
- **页面/接口**: POST /api/payment/create
- **现象**: 前端提示请求超时，Nginx 日志出现大量 504
- **关键词**: 504, timeout, 超时, 网关超时, 接口超时, nginx
- **涉及组件**: Nginx, Core Service, MySQL, Redis

**可能原因**：
1. Core Service 负载过高。
2. 数据库慢查询导致线程阻塞。
3. 网络抖动。

**排查步骤**：
1. **查看 Nginx 日志**，确认超时接口：
   ```bash
   tail -n 100 /var/log/nginx/access.log | grep "504"
   ```
2. **检查 Core Service 负载**：
   ```bash
   top -p $(pgrep core-service)
   ```
3. **检查数据库连接数**：
   ```bash
   netstat -an | grep 3306 | wc -l
   ```

### 场景：订单状态未流转

- **所属模块**: 核心支付模块
- **页面/接口**: 回调接口 POST /api/payment/callback
- **现象**: 用户支付成功，但订单状态仍为 "PENDING"
- **关键词**: PENDING, 状态未更新, 回调失败, 订单, 未流转
- **涉及组件**: callback-worker, Redis, Core Service

**排查步骤**：
1. **查询特定订单日志**：
   ```bash
   grep "Order-12345" /var/log/app/payment.log
   ```
2. **检查回调服务状态**：
   ```bash
   systemctl status callback-worker
   ```
