---
service: Database Service
module: 连接管理
---

# 数据库维护与应急手册 (MySQL/PostgreSQL)

## 场景：数据库连接池满

- **现象**: 应用日志报错 Too many connections
- **关键词**: too many connections, 连接池, 耗尽, MaxOpenConns
- **涉及组件**: MySQL, PostgreSQL, 应用连接池

**排查**:
- MySQL: `SHOW PROCESSLIST;` 查看当前连接数。
- 检查应用层连接池配置 (MaxOpenConns)。

## 场景：慢查询阻塞

- **现象**: 接口响应变慢，CPU 飙升
- **关键词**: 慢查询, slow query, CPU高, 响应慢, 无索引
- **涉及组件**: MySQL, PostgreSQL

**排查**:
1. 开启慢查询日志 (Slow Query Log)。
2. 使用 `EXPLAIN` 分析 SQL 执行计划。
3. 检查是否缺少索引。

## 备份与恢复
### MySQL
- **备份**: `mysqldump -u root -p mydb > mydb_backup.sql`
- **恢复**: `mysql -u root -p mydb < mydb_backup.sql`

### PostgreSQL
- **备份**: `pg_dump mydb > mydb_backup.sql`
- **恢复**: `psql mydb < mydb_backup.sql`

## 紧急操作
如果数据库死锁或负载过高，可能需要 Kill 掉相关 Session。
```sql
-- MySQL Kill Session
KILL <CONNECTION_ID>;
```
