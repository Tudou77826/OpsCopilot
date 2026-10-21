# 测试零售环境部署

| 页面功能 | Service | 日志目录 |
|---|---|---|
| 订单详情查询 | orders-reader | /var/log/retail/orders-reader |
| 商品搜索 | catalog-search | /var/log/retail/catalog-search |

日志位于业务节点，管理面节点只提供资源管理。应核对用户选定的节点；无法确定节点归属时提问，不自行连接未知环境。

订单详情日志按日期命名为 `orders-YYYY-MM-DD.log`，包含时间、请求关联号、错误码与上游调用信息。这份说明不包含任何账号或凭据。
