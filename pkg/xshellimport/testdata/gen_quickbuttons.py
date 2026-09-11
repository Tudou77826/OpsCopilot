"""生成一套复杂的 Xshell 快捷按钮（.qbl）夹具。

覆盖点：
- 多套按钮（6 个 .qbl），其中一套 30 条，用来试长列表的滚动与全选/全不选
- Type=1（可导入）/ Type=2、3（不支持）/ 缺名称 / 缺内容
- 超长命令、含 = 与 ; 的值、中文与英文混排
- GBK 老编码（无 BOM）且 [Info] 缺 Count，走键名扫描兜底
- 一个没有按钮的 .qbl（文件级告警）
- 同目录混入非 .qbl 干扰文件
- 多条命令的内容与 OpsCopilot 现有命令相同，用来触发"落点参考现状"与「已在某分组」

产物写成 Xshell 的真实形态：UTF-16LE + BOM，`[Info]` + `[QuickButton]`。
"""

import os
import sys


def utf16le(text: str) -> bytes:
    return b"\xff\xfe" + text.encode("utf-16-le")


def gbk(text: str) -> bytes:
    return text.encode("gbk")


def build(info_lines, buttons):
    """buttons: list of dict(name, action, type, param, desc)"""
    lines = ["[Info]"]
    lines += info_lines
    lines.append("[QuickButton]")
    for i, b in enumerate(buttons):
        lines.append(f"Button_{i}_Name={b.get('name', '')}")
        lines.append(f"Button_{i}_Param={b.get('param', '')}")
        lines.append(f"Button_{i}_Icon={b.get('icon', 0)}")
        lines.append(f"Button_{i}_Desc={b.get('desc', '')}")
        if "type" in b:
            lines.append(f"Button_{i}_Type={b['type']}")
        lines.append(f"Button_{i}_Action={b.get('action', '')}")
    return "\n".join(lines) + "\n"


def btn(name, action, type_="1", **kw):
    return {"name": name, "action": action, "type": type_, **kw}


# —— 1. 常用运维：混合可导入与四种不可导入，含超长命令 ——
common = build(
    ["Version=8.2", "Count=12", "Expanded=1"],
    [
        btn("磁盘占用", "df -h"),                                    # 内容命中「默认」分组
        btn("内存", "free -h"),                                      # 内容命中「默认」
        btn("监听端口", "ss -tlnp"),                                  # 内容命中「默认」
        btn("跟踪 syslog", "tail -f /var/log/syslog"),                # 内容命中「默认」
        btn("容器列表", "docker ps -a"),                              # 内容命中「容器命令」
        btn("进容器", "docker exec -it container_name bash"),          # 内容命中「容器命令」
        btn("上传文件（Zmodem）", "rz -E"),
        btn(
            "排错日志",
            'journalctl -u app --since "2 hours ago" | grep -iE "error|fail|timeout" | tail -n 200',
        ),
        btn("分析列 = 分割", "awk -F= '{print $2}' /etc/os-release"),
        btn("菜单类按钮", "echo menu", type_="2"),                    # 不支持：菜单
        btn("脚本按钮", "echo script", type_="3"),                    # 不支持：脚本
        btn("", "echo noname"),                                     # 不支持：缺名称
    ],
)

# 另一条缺内容的按钮，单独构造（放在后续集合里）
common_extra = btn("空内容按钮", "", type_="1")

# —— 2. 容器：整体系「容器命令」分组的镜像，用来试重复与落点 ——
docker = build(
    ["Version=8.2", "Count=8", "Expanded=1"],
    [
        btn("容器列表", "docker ps -a"),
        btn("资源占用", "docker stats --no-stream"),
        btn("清理悬挂层", "docker system prune -f"),
        btn("Compose 起服务", "docker compose up -d"),
        btn("容器日志", "docker logs -f --tail 100 container_name"),
        btn("进容器", "docker exec -it container_name bash"),
        common_extra,
        btn("容器内看端口", "docker exec -it container_name ss -tlnp"),
    ],
)

# —— 3. K8s：含超长中文名 ——
k8s = build(
    ["Version=8.2", "Count=10", "Expanded=1"],
    [
        btn("Pods", "kubectl get pods -A"),
        btn("进 Pod", "kubectl exec -it pod_name -- sh"),
        btn("服务日志", "kubectl logs -f pod_name -c container_name"),
        btn("Describe", "kubectl describe pod pod_name"),
        btn("重启部署", "kubectl rollout restart deployment/app"),     # 内容命中「K8s 操作」
        btn("节点负载", "kubectl top nodes"),
        btn("查看所有服务与端口映射并过滤出没有就绪的端点", "kubectl get endpoints -A | grep -v '<none>'"),
        btn("驱逐节点并等待 Pod 优雅迁移完成后再排空", "kubectl drain node-01 --ignore-daemonsets --delete-emptydir-data"),
        btn("事件", "kubectl get events --sort-by=.lastTimestamp | tail -30"),
        btn("进入调试容器", "kubectl debug -it pod_name --image=busybox --target=app"),
    ],
)

# —— 4. 旧版中文（GBK、无 BOM、缺 Count）：走键名扫描兜底 ——
legacy = build(
    ["Version=6.0"],  # 刻意不写 Count
    [
        btn("重启 nginx", "systemctl restart nginx"),
        btn("重载 nginx 配置", "nginx -s reload"),                    # 内容命中「Nginx 维护」
        btn("查看错误日志", "tail -f /var/log/nginx/error.log"),       # 内容命中「Nginx 维护」
        btn("磁盘大目录", "du -sh /* 2>/dev/null | sort -h | tail -20"),
        btn("带分号的值", "echo a; echo b"),
    ],
)

# —— 5. 大批量（30 条）：试长列表滚动与全选/全不选 ——
big = build(
    ["Version=8.2", "Count=30", "Expanded=0"],
    [btn(f"批量任务 {i:02d}", f"sh /opt/ops/task-{i:02d}.sh") for i in range(1, 31)],
)

# —— 6. 没有按钮的文件：文件级告警 ——
empty = build(["Version=8.2", "Count=0", "Expanded=1"], [])

FILES = {
    "common.qbl": utf16le(common),
    "docker.qbl": utf16le(docker),
    "k8s.qbl": utf16le(k8s),
    "legacy-gbk.qbl": gbk(legacy),
    "batch-30.qbl": utf16le(big),
    "empty.qbl": utf16le(empty),
    # 干扰项：非 .qbl 必须被忽略
    "trigger.ini": b"Count=0\n",
    "readme.txt": "这不是快捷按钮集\n".encode("utf-8"),
}


def write_dir(target):
    os.makedirs(target, exist_ok=True)
    for name, data in FILES.items():
        with open(os.path.join(target, name), "wb") as f:
            f.write(data)
    return target


if __name__ == "__main__":
    for target in sys.argv[1:]:
        print("written:", write_dir(target))
