# qBittorrent-ClientBlocker

一款适用于 qBittorrent 的客户端屏蔽器, 默认屏蔽包括但不限于迅雷等客户端, 支持 qBittorrent 4.1 及以上版本.

-   全平台支持
-   支持记录日志及热重载配置
-   支持忽略私有 IP 地址
-   支持自定义屏蔽列表 (不区分大小写, 支持正则表达式)
-   支持客户端认证
-   支持增强自动屏蔽 (默认禁用): 根据默认或设定的相关参数自动屏蔽 Peer
-   在 Windows 下支持通过 CTRL+ALT+B 窗口热键显示及隐藏窗口 (部分用户[反馈](https://github.com/Simple-Tracker/qBittorrent-ClientBlocker/issues/10)其可能会影响屏蔽, 由于原因不明, 若遇到相关问题可避免使用该功能)

![Preview](Preview.png)

## 使用 Usage

### 前提

-   必须启用 qBittorrent Web UI 功能.

### 常规版本安装

1. 从 [**GitHub Release**](https://github.com/Simple-Tracker/qBittorrent-ClientBlocker/releases) 下载压缩包并解压;

    <details>
    <summary>查看 常见平台下载版本 对照表</summary>

    | 操作系统 | 处理器架构 | 处理器位数 | 下载版本      | 说明                                                   |
    | -------- | ---------- | ---------- | ------------- | ------------------------------------------------------ |
    | macOS    | ARM64      | 64 位      | darwin-arm64  | 常见于 Apple M 系列                                    |
    | macOS    | AMD64      | 64 位      | darwin-amd64  | 常见于 Intel 系列                                      |
    | Windows  | AMD64      | 64 位      | windows-amd64 | 常见于大部分现代 PC                                    |
    | Windows  | i386       | 32 位      | windows-386   | 少见于部分老式 PC                                      |
    | Windows  | ARM64      | 64 位      | windows-arm64 | 常见于新型平台, 应用于部分平板/笔记本/少数特殊硬件     |
    | Windows  | ARMv6      | 32 位      | windows-arm   | 少见于罕见平台, 应用于部分上古硬件, 如 Surface RT 等   |
    | Linux    | AMD64      | 64 位      | linux-arm64   | 常见于大部分 NAS 及服务器                              |
    | Linux    | i386       | 32 位      | linux-386     | 少见于部分老式 NAS 及服务器                            |
    | Linux    | ARM64      | 64 位      | linux-arm64   | 常见于部分服务器及开发板, 如 Oracle 或 Raspberry Pi 等 |
    | Linux    | ARMv6      | 32 位      | linux-arm     | 少见于部分老式服务器及开发板                           |

    其它版本的 Linux/NetBSD/FreeBSD/OpenBSD/Solaris 可以此类推, 并在列表中选择适合自己的.
    </details>

2. 解压后, 可能需要修改随附的配置文件 ```config.json```;

    - 可根据高级需求, 按需设置配置, 具体见 [配置 Config](#配置-config)。
    - 若客户端屏蔽器运行于本机, 但未启用 qBittorrent "跳过本机客户端认证" (及密码不为空且并未手动设置密码), 则必须修改配置文件, 并填写 `qBPassword`.
    - 若客户端屏蔽器不运行于本机 或 qBittorrent 未安装在默认路径 或 使用 2.4 及以下版本的客户端屏蔽器, 则必须修改配置文件, 并填写 `qBURL`/`qBUsername`/`qBPassword`.

3. 启动客户端屏蔽器, 并观察信息输出是否正常即可;
   
   对于 Windows, 可选修改 qBittorrent 的快捷方式, 放入自己的屏蔽器路径, 使 qBittorrent 与屏蔽器同时运行;

   ```
   C:\Windows\System32\cmd.exe /c "(tasklist | findstr qBittorrent-ClientBlocker || start C:\Users\Example\qBittorrent-ClientBlocker\qBittorrent-ClientBlocker.exe) && start     qbittorrent.exe"
   ```

   对于 macOS, 可选使用一基本 [LaunchAgent 用户代理](https://github.com/Simple-Tracker/qBittorrent-ClientBlocker/wiki#launchagent-macos) 用于开机自启及后台运行;

   对于 Linux, 可选使用一基本 [Systemd 服务配置文件](https://github.com/Simple-Tracker/qBittorrent-ClientBlocker/wiki#systemd) 用于开机自启及后台运行;

### Docker 版本安装

-   从 [**Docker Hub**](https://hub.docker.com/r/simpletracker/qbittorrent-clientblocker) 拉取 Docker 镜像.

    ```
    docker pull simpletracker/qbittorrent-clientblocker:latest
    ```

-   配置方法一: 使用 配置文件映射

    1. 在合适位置新建 `config.json` 作为配置文件, 具体内容可参考 [config.json](config.json) 及 [配置 Config](#配置-config);

    2. 填入 `qBURL`/`qBUsername`/`qBPassword`;

        - 可根据高级需求, 按需设置其它配置, 具体见 [配置 Config](#配置-config).
        - 若启用 qBittorrent 的 "IP 子网白名单", 则可不填写 `qBUsername` 和 `qBPassword`.

    3. 替换 `/path/config.json` 为你的配置文件路径;

    4. 运行 Docker 并查看日志, 观察信息输出是否正常即可;

       以下命令模版仅作为参考.

        ```
        docker run -d \
            --name=qbittorrent-clientblocker --network=bridge --restart unless-stopped \
            -v /path/config.json:/app/config.json \
            simpletracker/qbittorrent-clientblocker:latest
        ```

-   配置方法二: 使用 环境变量

    -   使用前提: 设置 ```useENV``` 环境变量为 ```true```.
    -   使用环境变量按需配置设置, 具体见 [配置 Config](#配置-config).
    -   若设置较复杂, 则可能出现 blockList 不生效的情况. 因此, 若需要配置此设置, 则使用环境变量是不推荐的.
    -   以下命令模版仅作为参考.

        ```
        docker run -d \
            --name=qbittorrent-clientblocker --network=bridge --restart unless-stopped \
            -e debug=false \
            -e logPath=logs \
            -e blockList='["ExampleBlockList1", "ExampleBlockList2"]' \
            -e qBURL=http://example.com \
            -e qBUsername=exampleUser \
            -e qBPassword=examplePass \
            simpletracker/qbittorrent-clientblocker:latest
        ```

## 参数 Flag

| 设置项 | 默认值 | 配置说明 |
| ----- | ----- | ----- |
| -v/--version | false (禁用) | 显示程序版本后退出 |
| -c/--config | config.json | 配置文件路径 |
| --debug | false (禁用) | 调试模式. 加载配置文件前生效 |
| --nochdir | false (禁用) | 不切换工作目录. 默认会切换至程序目录 |

## 配置 Config

Docker 版本通过相同名称的环境变量配置, 通过自动转换环境变量为配置文件实现.

| 设置项 | 默认值 | 配置说明 |
| ----- | ----- | ----- |
| debug | false (禁用) | 调试模式. 启用可看到更多信息, 但可能扰乱视野 |
| debug_CheckTorrent | false (禁用) | 调试模式 (CheckTorrent, 须先启用 debug). 启用后调试信息会包括每个 Torrent Hash, 但信息量较大 |
| debug_CheckPeer | false (禁用) | 调试模式 (CheckPeer, 须先启用 debug). 启用后调试信息会包括每个 Torrent Peer, 但信息量较大 |
| interval | 6 (秒) | 屏蔽循环间隔. 每个循环间隔会从 qBittorrent API 获取相关信息用于判断及屏蔽, 短间隔有助于降低封禁耗时但可能造成 qBittorrent 卡顿, 长间隔有助于降低 CPU 资源占用 |
| cleanInterval | 3600 (秒) | 屏蔽清理间隔. 短间隔会使过期 Peer 在达到屏蔽持续时间后更快被解除屏蔽, 长间隔有助于合并清理过期 Peer 日志 |
| peerMapCleanInterval | 60 (秒) | Peer Map 清理间隔 (启用 maxIPPortCount/banByRelativeProgressUploaded 后生效, 也是其判断间隔). 短间隔可使判断更频繁但可能造成滞后误判 |
| banTime | 86400 (秒) | 屏蔽持续时间. 短间隔会使 Peer 更快被解除屏蔽 |
| banAllPort | false (禁用) | 屏蔽 IP 所有端口. 当前不支持设置 |
| IgnoreEmptyPeer | false (禁用) | 忽略无客户端名称的 Peer. 通常出现于连接未完全建立的客户端 |
| startDelay | 0 (秒, 禁用) | 启动延迟. 部分用户的特殊用途 |
| sleepTime | 20 (毫秒) | 查询每个 Torrent Peers 的等待时间. 短间隔可使屏蔽 Peer 更快但可能造成 qBittorrent 卡顿, 长间隔有助于平均 CPU 资源占用 |
| timeout | 6 (秒) | 请求超时. 过短间隔可能会造成无法正确屏蔽 Peer, 过长间隔会使超时请求影响屏蔽其它 Peer 的性能 |
| ipUploadedCheck | false (禁用) | IP 上传增量检测. 在满足下列 IP 上传增量 条件后, 会自动屏蔽 Peer |
| ipUpCheckInterval | 300 (秒) | IP 上传增量检测/检测间隔. 用于确定上一周期及当前周期, 以比对客户端对 IP 上传增量 |
| ipUpCheckIncrementMB | 38000 (MB) | IP 上传增量检测/增量大小. 若 IP 全局上传增量大小大于设置增量大小, 则允许屏蔽 Peer |
| ipUpCheckPerTorrentRatio | 3 (X) | IP 上传增量检测/增量倍率. 若 IP 单个 Torrent 上传增量大小大于设置增量倍率及 Torrent 大小之乘积, 则允许屏蔽 Peer |
| maxIPPortCount | 0 (禁用) | 每 IP 最大端口数. 若 IP 端口数大于设置值, 会自动屏蔽 Peer |
| banByProgressUploaded | false (禁用) | 增强自动屏蔽 (根据进度及上传量屏蔽 Peer, 未经测试验证). 在满足下列 增强自动屏蔽 条件后, 会自动屏蔽 Peer |
| banByPUStartMB | 10 (MB) | 增强自动屏蔽/起始大小. 若客户端上传量大于起始大小, 则允许屏蔽 Peer |
| banByPUStartPrecent | 2 (%) | 增强自动屏蔽/起始进度. 若客户端上传进度大于设置起始进度, 则允许屏蔽 Peer |
| banByPUAntiErrorRatio | 5 (X) | 增强自动屏蔽/滞后防误判倍率. 若 Peer 报告下载进度与设置倍率及 Torrent 大小之乘积得到之下载量 比 客户端上传量 还低, 则允许屏蔽 Peer |
| banByRelativeProgressUploaded | false (禁用) | 增强自动屏蔽_相对 (根据相对进度及相对上传量屏蔽 Peer, 未经测试验证). 在满足下列 增强自动屏蔽_相对 条件后, 会自动屏蔽 Peer |
| banByRelativePUStartMB | 10 (MB) | 增强自动屏蔽_相对/起始大小. 若客户端相对上传量大于设置起始大小, 则允许屏蔽 Peer |
| banByRelativePUStartPrecent | 2 (%) | 增强自动屏蔽_相对/起始进度. 若客户端相对上传进度大于设置起始进度, 则允许屏蔽 Peer |
| banByRelativePUAntiErrorRatio | 5 (X) | 增强自动屏蔽_相对/滞后防误判倍率. 若 Peer 报告相对下载进度与设置倍率之乘积得到之相对下载进度 比 客户端相对上传进度 还低, 则允许屏蔽 Peer |
| longConnection | true (启用) | 长连接. 启用可降低资源消耗 |
| logToFile | true (启用) | 记录普通信息到日志. 启用后可用于一般的分析及统计用途 |
| logDebug | false (禁用) | 记录调试信息到日志 (须先启用 debug 及 logToFile). 启用后可用于进阶的分析及统计用途, 但信息量较大 |
| qBURL | 空 | qBittorrent Web UI 地址. 使用客户端屏蔽器的前提条件, 若未能自动读取 qBittorrent 配置文件, 则须正确填入. 前缀必须指定 http 或 https 协议, 如 ```http://127.0.0.1:990```. |
| qBUsername | 空 | qBittorrent Web UI 账号. 若启用 qBittorrent 内 "跳过本机客户端认证" 可默认留空, 可自动读取 qBittorrent 配置文件并设置 |
| qBPassword | 空 | qBittorrent Web UI 密码. 若启用 qBittorrent 内 "跳过本机客户端认证" 可默认留空 |
| useBasicAuth | false (禁用) | 同时通过 HTTP Basic Auth 进行认证. 适合通过反向代理等方式 增加/换用 认证方式的 qBittorrent Web UI |
| skipCertVerification | false (禁用) | 跳过 qBittorrent Web UI 证书校验. 适合自签及过期证书 |
| blockList | 空 (于 config.json 附带) | 屏蔽客户端列表. 同时判断 PeerID 及 UserAgent, 不区分大小写, 支持正则表达式 |
| ipBlockList | 空 | 屏蔽 IP 列表. 支持不包括端口的 IP (1.2.3.4) 及 IPCIDR (2.3.3.3/3) |

## 致谢 Credit

1. 我们在客户端屏蔽器的早期开发过程中部分参考了 [jinliming2/qbittorrent-ban-xunlei](https://github.com/jinliming2/qbittorrent-ban-xunlei);
2. 我们会在每期版本的 Release Note 中感谢当期通过 Pull Request 向客户端屏蔽器贡献代码改进的用户及开发者;
