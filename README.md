# qBittorrent-ClientBlocker

[中文 (默认, Beta 版本)](README.md) [English (Default, Beta Version)](README.en.md)  
[中文 (Public 正式版)](https://github.com/Simple-Tracker/qBittorrent-ClientBlocker/blob/master/README.md) [English (Public version)](https://github.com/Simple-Tracker/qBittorrent-ClientBlocker/blob/master/README.en.md)

一款适用于 qBittorrent (4.1+)/Transmission (3.0+, Beta)/BitComet (2.0+, Beta, Partial) 的客户端屏蔽器, 默认屏蔽包括但不限于迅雷等客户端.

-   全平台支持
-   支持记录日志及热重载配置
-   支持忽略私有 IP 地址
-   支持自定义屏蔽列表 (不区分大小写, 支持正则表达式)
-   支持多种客户端及其认证, 同时可自动检测部分客户端 (目前支持 qBittorrent. 可以安全忽略检测客户端时发生的错误)
-   支持增强自动屏蔽 (默认禁用): 根据默认或设定的相关参数自动屏蔽 Peer
-   在 Windows 下支持通过系统托盘或窗口热键 (CTRL+ALT+B) 显示及隐藏窗口 (部分用户[反馈](https://github.com/Simple-Tracker/qBittorrent-ClientBlocker/issues/10)其可能会影响屏蔽, 由于原因不明, 若遇到相关问题可避免使用该功能)

另: 我们相信, 通过即时通讯, 能够: 改善问题跟踪及处理的速度和流程 及 更好的加快想法流转. 因此, 我们创建了一个 QQ 用户群 (临时): [857326151](http://qm.qq.com/cgi-bin/qm/qr?_wv=1027&k=edXDN0Dk0kFfgS6t2Uc1MeqUD4NLx76_&authKey=u2Cm6up4ctiHrLTSwCvIo%2FxETz5xYa6%2BBWK187BSGHlgEng6ZRIuv8OC870QGoGq&noverify=0&group_code=857326151)

![Preview](Preview.png)

## 使用 Usage

### 前提

-   必须启用客户端的 Web UI 功能.

### 常规版本使用

1. 从 [**GitHub Release**](https://github.com/Simple-Tracker/qBittorrent-ClientBlocker/releases) 下载压缩包并解压;

    <details>
    <summary>查看 常见平台下载版本 对照表</summary>

    | 操作系统 | 处理器架构 | 处理器位数 | 下载版本      | 说明 |
    | -------- | ---------- | ---------- | ------------- | ----------------- |
    | macOS    | ARM64      | 64 位      | darwin-arm64  | 常见于 Apple M 系列 |
    | macOS    | AMD64      | 64 位      | darwin-amd64  | 常见于 Intel 系列 |
    | Windows  | AMD64      | 64 位      | windows-amd64 | 常见于大部分现代 PC |
    | Windows  | i386       | 32 位      | windows-386   | 少见于部分老式 PC |
    | Windows  | ARM64      | 64 位      | windows-arm64 | 常见于新型平台, 应用于部分平板/笔记本/少数特殊硬件 |
    | Windows  | ARMv7      | 32 位      | windows-arm   | 少见于罕见平台, 应用于部分上古硬件, 如 Surface RT 等 |
    | Linux    | AMD64      | 64 位      | linux-amd64   | 常见于大部分 NAS 及服务器 |
    | Linux    | i386       | 32 位      | linux-386     | 少见于部分老式 NAS 及服务器 |
    | Linux    | ARM64      | 64 位      | linux-arm64   | 常见于部分服务器及开发板, 如 Oracle 或 Raspberry Pi 等 |
    | Linux    | ARMv*      | 32 位      | linux-armv*   | 少见于部分老式服务器及开发板, 查看 /proc/cpuinfo 或 从高到底试哪个能跑 |

    其它版本的 Linux/NetBSD/FreeBSD/OpenBSD/Solaris 可以此类推, 并在列表中选择适合自己的.
    </details>

2. 解压后, 可能需要修改随附的配置文件 ```config.json```;

    - 可根据高级需求, 按需设置配置, 具体见 [配置 Config](#配置-config).
    - 若客户端屏蔽器运行于本机, 但未启用客户端 "跳过本机客户端认证" (及密码不为空且并未手动设置密码), 则必须修改配置文件, 并填写 ```clientPassword```.
    - 若客户端屏蔽器不运行于本机 或 客户端未安装在默认路径 或 客户端不支持自动读取配置文件, 则必须修改配置文件, 并填写 ```clientURL```/```clientUsername```/```clientPassword```.

3. 启动客户端屏蔽器, 并观察信息输出是否正常即可;
   
    对于 Windows, 可选修改客户端的快捷方式, 放入自己的屏蔽器路径, 使客户端与屏蔽器同时运行;

    qBittorrent: ```C:\Windows\System32\cmd.exe /c "(tasklist | findstr qBittorrent-ClientBlocker || start C:\Users\Example\qBittorrent-ClientBlocker\qBittorrent-ClientBlocker.exe) && start qbittorrent.exe"```

    对于 macOS, 可选使用一基本 [LaunchAgent 用户代理](https://github.com/Simple-Tracker/qBittorrent-ClientBlocker/wiki#launchagent-macos) 用于开机自启及后台运行;

    对于 Linux, 可选使用一基本 [Systemd 服务配置文件](https://github.com/Simple-Tracker/qBittorrent-ClientBlocker/wiki#systemd-linux) 用于开机自启及后台运行;

### Docker 版本使用

-   从 [**Docker Hub**](https://hub.docker.com/r/simpletracker/qbittorrent-clientblocker) 拉取 Docker 镜像.

    ```
    docker pull simpletracker/qbittorrent-clientblocker:latest
    ```

-   配置方法一: 使用 配置文件映射

    1. 在合适位置新建 ```config.json``` 作为配置文件, 具体内容可参考 [config.json](config.json) 及 [配置 Config](#配置-config);

    2. 填入 ```clientURL```/```clientUsername```/```clientPassword```;

        - 可根据高级需求, 按需设置其它配置, 具体见 [配置 Config](#配置-config).
        - 若启用客户端的 "IP 子网白名单", 则可不填写 ```clientUsername``` 和 ```clientPassword```.

    3. 运行 Docker 并查看日志, 观察信息输出是否正常即可;

       以下命令模版仅作为参考, 请替换 ```/path/config.json``` 为你的配置文件路径.

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
            -e clientURL=http://example.com \
            -e clientUsername=exampleUser \
            -e clientPassword=examplePass \
            simpletracker/qbittorrent-clientblocker:latest
        ```

## 参数 Flag

| 设置项 | 默认值 | 配置说明 |
| ----- | ----- | ----- |
| -v/--version | false (禁用) | 显示程序版本后退出 |
| -c/--config | config.json | 配置文件路径 |
| -ca/--config_additional | config_additional.json | 附加配置文件路径 |
| --debug | false (禁用) | 调试模式. 加载配置文件前生效 |
| --startdelay | 0 (秒, 禁用) | 启动延迟. 部分用户的特殊用途 |
| --nochdir | false (禁用) | 不切换工作目录. 默认会切换至程序目录 |
| --hidewindow | false (禁用) | 默认隐藏窗口. 仅 Windows 可用 |
| --hidesystray | false (禁用) | 默认隐藏托盘图标. 仅 Windows 可用 |

## 配置 Config

Docker 版本通过相同名称的环境变量配置, 通过自动转换环境变量为配置文件实现.

| 设置项 | 类型 | 默认值 | 配置说明 |
| ----- | ----- | ----- | ----- |
| checkUpdate | bool | true (启用) | 检查更新. 默认会自动检查更新 |
| debug | bool | false (禁用) | 调试模式. 启用可看到更多信息, 但可能扰乱视野 |
| debug_CheckTorrent | string | false (禁用) | 调试模式 (CheckTorrent, 须先启用 debug). 启用后调试信息会包括每个 Torrent Hash, 但信息量较大 |
| debug_CheckPeer | string | false (禁用) | 调试模式 (CheckPeer, 须先启用 debug). 启用后调试信息会包括每个 Torrent Peer, 但信息量较大 |
| interval | uint32 | 6 (秒) | 屏蔽循环间隔 (不支持热重载). 每个循环间隔会从后端获取相关信息用于判断及屏蔽, 短间隔有助于降低封禁耗时但可能造成客户端卡顿, 长间隔有助于降低 CPU 资源占用 |
| cleanInterval | uint32 | 3600 (秒) | 屏蔽清理间隔. 短间隔会使过期 Peer 在达到屏蔽持续时间后更快被解除屏蔽, 长间隔有助于合并清理过期 Peer 日志 |
| updateInterval | uint32 | 86400 (秒) | 列表 URL 更新间隔 (blockListURL/ipBlockListURL). 合理的间隔有助于提高更新效率并降低网络占用 |
| restartInterval | uint32 | 6 (秒) | 重启 Torrent 间隔. 用于部分客户端 (Transmission) 屏蔽列表无法立即生效的措施, 通过重启 Torrent 来实现. 过短间隔可能造成屏蔽不生效 |
| torrentMapCleanInterval | uint32 | 60 (秒) | Torrent Map 清理间隔 (启用 ipUploadedCheck+ipUpCheckPerTorrentRatio/banByRelativeProgressUploaded 后生效, 也是其判断间隔). 短间隔可使判断更频繁但可能造成滞后误判 |
| banTime | uint32 | 86400 (秒) | 屏蔽持续时间. 短间隔会使 Peer 更快被解除屏蔽 |
| banAllPort | bool | true (启用) | 屏蔽 IP 所有端口. 默认启用且当前不支持设置 |
| banIPCIDR | string | /32 | 封禁 IPv4 CIDR. 可扩大单个 Peer 的封禁 IP 范围 |
| banIP6CIDR | string | /128 | 封禁 IPv6 CIDR. 可扩大单个 Peer 的封禁 IP 范围 |
| ignoreEmptyPeer | bool | true (启用) | 忽略无 PeerID 及 ClientName 的 Peer. 通常出现于连接未完全建立的客户端 |
| ignoreNoLeechersTorrent | bool | true (启用) | 忽略没有下载者的 Torrent. 启用后有助于提高性能 |
| ignorePTTorrent | bool | true (启用) | 忽略 PT Torrent. 若主要 Tracker 包含 ```?passkey=```/```?authkey=```/```?secure=```/```32 位大小写英文及数字组成的字符串``` |
| sleepTime | uint32 | 20 (毫秒) | 查询每个 Torrent Peers 的等待时间. 短间隔可使屏蔽 Peer 更快但可能造成客户端卡顿, 长间隔有助于平均 CPU 资源占用 |
| timeout | uint32 | 6 (秒) | 请求超时. 过短间隔可能会造成无法正确屏蔽 Peer, 过长间隔会使超时请求影响屏蔽其它 Peer 的性能 |
| proxy | string | Auto (自动) | 使用代理. 设置为空可以禁止此行为, 但仍会在首次加载配置文件时自动检测代理 |
| longConnection | bool | true (启用) | 长连接. 启用可降低资源消耗 |
| logToFile | bool | true (启用) | 记录普通信息到日志. 启用后可用于一般的分析及统计用途 |
| logPath | string | logs | 日志的目录，需要启用 logToFile |
| logDebug | bool | false (禁用) | 记录调试信息到日志 (须先启用 debug 及 logToFile). 启用后可用于进阶的分析及统计用途, 但信息量较大 |
| listen | string | 127.0.0.1:26262 | 监听端口. 用于向部分客户端 (Transmission) 提供 BlockPeerList. 非本机使用可改为 ```<Host>:<Port>``` |
| clientType | string | 空 | 客户端类型. 使用客户端屏蔽器的前提条件, 若未能自动检测客户端类型, 则须正确填入. 目前支持 ```qBittorrent```/```Transmission```/```BitComet``` |
| clientURL | string | 空 | 客户端地址. 可用 Web API 或 RPC 地址. 使用客户端屏蔽器的前提条件, 若未能自动读取客户端配置文件, 则须正确填入. 前缀必须指定 http 或 https 协议, 如 ```http://127.0.0.1:990/api``` 或 ```http://127.0.0.1:9091/transmission/rpc``` |
| clientUsername | string | 空 | 客户端账号. 留空会跳过认证. 若启用客户端内 "跳过本机客户端认证" 可默认留空, 因可自动读取客户端配置文件并设置 |
| clientPassword | string | 空 | 客户端密码. 若启用客户端内 "跳过本机客户端认证" 可默认留空 |
| useBasicAuth | bool | false (禁用) | 同时通过 HTTP Basic Auth 进行认证. 适合只支持 Basic Auth (如 Transmission/BitComet) 或通过反向代理等方式 增加/换用 认证方式的后端 |
| skipCertVerification | bool | false (禁用) | 跳过 Web API 证书校验. 适合自签及过期证书 |
| fetchFailedThreshold | int | 0 (禁用) | 最大获取失败次数. 当超过设定次数, 将执行设置的外部命令 |
| execCommand_FetchFailed | string | 空 | 执行外部命令 (FetchFailed). 首个参数被视作外部程序路径, 当获取失败次数超过设定次数后执行 |
| execCommand_Run | string | 空 | 执行外部命令 (Run). 首个参数被视作外部程序路径, 当程序启动后执行 |
| execCommand_Ban | string | 空 | 执行外部命令 (Ban). 首个参数被视作外部程序路径, 各参数均应使用 ```\|``` 分割, 命令可以使用 ```{peerIP}```/```{peerPort}```/```{torrentInfoHash}``` 来使用相关信息 (peerPort=-1 意味着全端口封禁) |
| execCommand_Unban | string | 空 | 执行外部命令 (Unban). 首个参数被视作外部程序路径, 各参数均应使用 ```\|``` 分割, 命令可以使用 ```{peerIP}```/```{peerPort}```/```{torrentInfoHash}``` 来使用相关信息 (peerPort=-1 意味着全端口封禁) |
| syncServerURL | string | 空 | 同步服务器 URL. 同步服务器会将 TorrentMap 提交至服务器, 并从服务器接收屏蔽 IPCIDR 列表 |
| syncServerToken | string | 空 | 同步服务器 Token. 部分同步服务器可能需要认证 |
| blockList | []string | 空 (于 config.json 附带) | 屏蔽客户端列表. 同时判断 PeerID 及 ClientName, 不区分大小写, 支持正则表达式 |
| blockListURL | string | 空 | 屏蔽客户端列表 URL. 支持格式同 blockList, 一行一条 |
| blockListFile | string | 空 | 屏蔽客户端列表文件. 支持格式同 blockList, 一行一条 |
| portBlockList | []uint32 | 空 | 屏蔽端口列表. 若 Peer 端口与列表内任意端口匹配, 则允许屏蔽 Peer |
| ipBlockList | []string | 空 | 屏蔽 IP 列表. 支持不包括端口的 IP (1.2.3.4) 及 IPCIDR (2.3.3.3/3) |
| ipBlockListURL | string | 空 | 屏蔽 IP 列表 URL. 支持格式同 ipBlockList, 一行一条 |
| ipBlockListFile | string | 空 | 屏蔽 IP 列表文件. 支持格式同 ipBlockList, 一行一条 |
| genIPDat | uint32 | 0 (禁用) | 1: 生成 IPBlockList.dat. 包括所有被封禁的 Peer IPCIDR, 格式同 ipBlockList; 2: 生成 IPFilter.dat. 包括所有被封禁的 Peer IP; 一行一条 |
| ipUploadedCheck | bool | false (禁用) | IP 上传增量检测. 在满足下列 IP 上传增量 条件后, 会自动屏蔽 Peer |
| ipUpCheckInterval | uint32 | 300 (秒) | IP 上传增量检测/检测间隔. 用于确定上一周期及当前周期, 以比对客户端对 IP 上传增量. 也顺便用于 maxIPPortCount |
| ipUpCheckIncrementMB | uint32 | 38000 (MB) | IP 上传增量检测/增量大小. 若 IP 全局上传增量大小大于设置增量大小, 则允许屏蔽 Peer |
| ipUpCheckPerTorrentRatio | float64 | 3 (X) | IP 上传增量检测/增量倍率. 若 IP 单个 Torrent 上传增量大小大于设置增量倍率及 Torrent 大小之乘积, 则允许屏蔽 Peer |
| maxIPPortCount | uint32 | 0 (禁用) | 每 IP 最大端口数. 若 IP 端口数大于设置值, 会自动屏蔽 Peer |
| banByProgressUploaded | bool | false (禁用) | 增强自动屏蔽 (根据进度及上传量屏蔽 Peer, 未经测试验证). 在满足下列 增强自动屏蔽 条件后, 会自动屏蔽 Peer |
| banByPUStartMB | uint32 | 20 (MB) | 增强自动屏蔽/起始大小. 若客户端上传量大于起始大小, 则允许屏蔽 Peer |
| banByPUStartPrecent | float64 | 2 (%) | 增强自动屏蔽/起始进度. 若客户端上传进度大于设置起始进度, 则允许屏蔽 Peer |
| banByPUAntiErrorRatio | float64 | 3 (X) | 增强自动屏蔽/滞后防误判倍率. 若 Peer 报告下载进度与设置倍率及 Torrent 大小之乘积得到之下载量 比 客户端上传量 还低, 则允许屏蔽 Peer |
| banByRelativeProgressUploaded | bool | false (禁用) | 增强自动屏蔽_相对 (根据相对进度及相对上传量屏蔽 Peer, 未经测试验证). 在满足下列 增强自动屏蔽_相对 条件后, 会自动屏蔽 Peer |
| banByRelativePUStartMB | uint32 | 20 (MB) | 增强自动屏蔽_相对/起始大小. 若客户端相对上传量大于设置起始大小, 则允许屏蔽 Peer |
| banByRelativePUStartPrecent | float64 | 2 (%) | 增强自动屏蔽_相对/起始进度. 若客户端相对上传进度大于设置起始进度, 则允许屏蔽 Peer |
| banByRelativePUAntiErrorRatio | float64 | 3 (X) | 增强自动屏蔽_相对/滞后防误判倍率. 若 Peer 报告相对下载进度与设置倍率之乘积得到之相对下载进度 比 客户端相对上传进度 还低, 则允许屏蔽 Peer |
| ignoreByDownloaded | uint32 | 100 (MB) | 增强自动屏蔽*/最高下载量. 若从 Peer 下载量大于此项, 则跳过增强自动屏蔽 |

## 反馈 Feedback
用户及开发者可以通过 [Issue](https://github.com/Simple-Tracker/qBittorrent-ClientBlocker/issues) 反馈 bug, 通过 [Discussion](https://github.com/Simple-Tracker/qBittorrent-ClientBlocker/discussions) 提问/讨论/分享 使用方法, 通过 [Pull Request](https://github.com/Simple-Tracker/qBittorrent-ClientBlocker/pulls) 向客户端屏蔽器贡献代码改进.  
注意: 应基于 dev 分支. 为 Feature 发起 Pull Request 时, 请不要同步创建 Issue. 由于人手有限, 开发进度可能较为缓慢.

## 致谢 Credit

1. 我们在客户端屏蔽器的早期开发过程中部分参考了 [jinliming2/qbittorrent-ban-xunlei](https://github.com/jinliming2/qbittorrent-ban-xunlei). 我们可能也会参考其它同类项目对项目进行优化, 但将不在此处单独列出;
2. 我们会在每期版本的 Release Note 中感谢当期通过 Pull Request 向客户端屏蔽器贡献代码改进的用户及开发者;
