# qBittorrent-ClientBlocker

[中文](README.md) [English](README.en.md)

A client blocker compatible with qBittorrent, which is prohibited to include but not limited to clients such as Xunlei, and support qBittorrent 4.1 and above version.

-   Support many platforms
-   Support log and hot-reload config
-   Support ignore private ip
-   Support custom blockList (Case-Inensitive, Support regular expression)
-   Support client authentication
-   Support enhanced automatic ban (Default disable): Automatically ban peer based on the default or set related parameter
-   Under Windows, support show and hide window through the Ctrl+Alt+B window hotkey (Some users [feedback](https://github.com/Simple-Tracker/qBittorrent-ClientBlocker/issues/10) it may affect ban. Due to unknown reason, the function can be avoided if related problem are encountered)

![Preview](Preview.png)

## 使用 Usage

### Prerequisite

-   qBittorrent Web UI must be enabled.

### Use conventional version

1. Download compressed from [**GitHub Release**](https://github.com/Simple-Tracker/qBittorrent-ClientBlocker/releases) and decompress it;

    <details>
    <summary>View common platform download version of comparison table</summary>

    | OS       | Processor Arch | Processor Integers | Download Version | Note                                                                                    |
    | -------- | ----------     | ----------         | -------------    | ------------------------------------------------------                                  |
    | macOS    | ARM64          | 64-bit             | darwin-arm64     | Common in Apple M series                                                                |
    | macOS    | AMD64          | 64-bit             | darwin-amd64     | Common in Intel series                                                                  |
    | Windows  | AMD64          | 64-bit             | windows-amd64    | Common in most modern PC                                                                |
    | Windows  | i386           | 32-bit             | windows-386      | Occasionally on some old PC                                                             |
    | Windows  | ARM64          | 64-bit             | windows-arm64    | Common on new platform, it's applied to some tablet/notebooks/minority special hardware |
    | Windows  | ARMv6          | 32-bit             | windows-arm      | Rare platform, applied to some ancient hardware, such as Surface RT, etc                |
    | Linux    | AMD64          | 64-bit             | linux-amd64      | Common NAS and server                                                                   |
    | Linux    | i386           | 32-bit             | linux-386        | Rarely in some old NAS and server                                                       |
    | Linux    | ARM64          | 64-bit             | linux-arm64      | Common server and development board, such as Oracle or Raspberry Pi, etc                |
    | Linux    | ARMv6          | 32-bit             | linux-arm        | Rarely in some old server and development board                                         |

    Other versions of Linux/Netbsd/FreeBSD/OpenBSD/Solaris can use this form as an example, and select one that suits you in the list.
    </details>

2. After decompression, you may need to modify the attached config file ```config.json```;

    - You can set config according to high-level needs. See [配置 Config](#配置-config).
    - If blocker runs on this machine, but qBittorrent "Skip client certification" is disabled (and password is not empty and does not manually set password in blocker), you must modify config file and fill in ```qBPassword```.
    - If blocker is not running on this machine or qBittorrent is not installed on the default path or using blocker with 2.4 and below version, config file must be modified and fills in ```qBURL```/```qBUsername```/```qBPassword```.

3. Start blocker and observe whether the information output is normal;
   
   For Windows, you can choose shortcut of qBittorrent, put your own blocker path, and run qBittorrent and blocker at the same time;

   ```
   C:\Windows\System32\cmd.exe /c "(tasklist | findstr qBittorrent-ClientBlocker || start C:\Users\Example\qBittorrent-ClientBlocker\qBittorrent-ClientBlocker.exe) && start     qbittorrent.exe"
   ```

   For macOS, You can choose a basic [LaunchAgent](https://github.com/Simple-Tracker/qBittorrent-ClientBlocker/wiki#launchagent-macos) for starting from OS start and background run;

   For Linux, You can choose a basic [Systemd service](https://github.com/Simple-Tracker/qBittorrent-ClientBlocker/wiki#systemd-linux) for starting from OS start and background run;

### Use Docker version

-   Pull image from [**Docker Hub**](https://hub.docker.com/r/simpletracker/qbittorrent-clientblocker).

    ```
    docker pull simpletracker/qbittorrent-clientblocker:latest
    ```

-   Configuration method 1: Use config file mapping

    1. Create a new ```config.json``` in right location, as a configuration file, the specific config can refer [config.json](config.json) and [配置 Config](#配置-config);

    2. Fills in ```qBURL```/```qBUsername```/```qBPassword```;

        - You can set config according to high-level needs. See [配置 Config](#配置-config).
        - If qBittorrent "IP subnet whitelist" is enabled, you don't need fill in  ```qBUsername``` and ```qBPassword```.

    3. Replace ```/path/config.json``` to your config path;

    4. Run docker image and view log to observe whether the information output is normal;

       The following command templates are used as a reference only.

        ```
        docker run -d \
            --name=qbittorrent-clientblocker --network=bridge --restart unless-stopped \
            -v /path/config.json:/app/config.json \
            simpletracker/qbittorrent-clientblocker:latest
        ```

-   Configuration method 1: Use environment variable

    -   Prerequisite: Set the ```useENV``` environment variable is ```true```.
    -   Use environment variables to configure settings on demand. For details, see [配置 Config](#配置-config).
    -   If config is complicated,  blockList may not take effect. Therefore, if you need to configure this setting, it's not recommended to use environment variable.
    -   The following command templates are used as a reference only.

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

| Parameter | Default | Note |
| ----- | ----- | ----- |
| -v/--version | false | Show program version and exit  |
| -c/--config | config.json | Config path |
| --debug | false | Debug mode. Effective before loading config file |
| --nochdir | false | Don't change working directory. Change to the program directory by default |

## 配置 Config

Docker version is configured through the same name variable configuration, which actually is implemented by automatically conversion environment variable as config file.

Translation not completed yet...

| Parameter | Default | Note |
| ----- | ----- | ----- |
| debug | false | Debug mode. Enable you can see more information, but it may disrupt the field of vision |
| debug_CheckTorrent | false | Debug mode (CheckTorrent, must enable debug). If it's enabled, debug info will include each Torrent Hash, but the amount of information will be large |
| debug_CheckPeer | false | Debug mode (CheckPeer, must enable debug). If it's enabled, debug info will include each Torrent Peer, but the amount of information will be large |
| interval | 6 (秒) | Ban Check Interval (Hot-reload is not supported). Each cycle interval will obtain relevant information from qBittorrent API for judgment and blocking. Short interval can help reduce ban time but may cause qBittorrent to freeze, but Long interval can help reduce CPU usage |
| cleanInterval | 3600 (Sec) | Clean blocked peer interval. Short interval will cause expired Peer to be unblocked faster after blocking duration is reached, but Long interval will help merge and clean up expired Peer log |
| torrentMapCleanInterval | 60 (Sec) | Torrent Map Clean Interval (Only useful after enable ipUploadedCheck+ipUpCheckPerTorrentRatio/banByRelativeProgressUploaded, It's also the judgment interval). Short interval can make judgments more frequent but may cause delayed misjudgments |
| banTime | 86400 (Sec) | Ban duration. Short interval will cause peer to be unblocked faster |
| banAllPort | false | Block IP all port. Setting is currently not supported |
| ignoreEmptyPeer | true | Ignore peers without PeerID and UserAgent. Usually occurs on clients where connection is not fully established |
| ignorePTTorrent | true | Ignore PT Torrent. If the main Tracker contains ```?passkey=```/```?authkey=```/```?secure=```/```A string of 32 digits consisting of uppercase and lowercase char or/and number``` |
| startDelay | 0 (Sec, Disable) | Start delay. Special uses for some user |
| sleepTime | 20 (MicroSec) | Query waiting time of each Torrent Peers. Short interval can make blocking Peer faster but may cause qBittorrent lag, Long interval can help average CPU usage |
| timeout | 6 (MillSec) | Request timeout. If interval is too short, peer may not be properly blocked. If interval is too long, timeout request will affect blocking other peer |
| longConnection | true | Long connection. Enable to reduce resource consumption |
| logToFile | true | Log general information to file. If enabled, it can be used for general analysis and statistical purposes |
| logDebug | false | Log debug information to file (Must enable debug and logToFile). If enabled, it can be used for advanced analysis and statistical purposes, but the amount of information is large |
| qBURL | Empty | qBittorrent Web UI Address. Prerequisite for using blocker, if qBittorrent config file cannot be automatically read, must be filled in correctly. Prefix must specify http or https protocol, such as ```http://127.0.0.1:990``` |
| qBUsername | Empty | qBittorrent Web UI Username. Leaving it blank will skip authentication. If you enable qBittorrent "Skip local client authentication", you can leave it blank by default, because the qBittorrent config file can be automatically read and set |
| qBPassword | Empty | qBittorrent Web UI Password. If qBittorrent "Skip local client authentication" is enabled, it can be left blank by default |
| useBasicAuth | false | At the same time, authentication is performed through HTTP Basic Auth. It can be used to add/replace authentication method of qBittorrent Web UI through reverse proxy, etc |
| skipCertVerification | false | Skip qBittorrent Web UI certificate verification. Suitable for self-signed and expired certificates |
| blockList | Empty (Included in config.json) | Block client list. Judge PeerID or UserAgent at the same time, case-insensitive, support regular expression |
| ipBlockList | Empty | Block IP list. Support excluding ports IP (1.2.3.4) or IPCIDR (2.3.3.3/3) |
| ipFilterURL | Empty | Block IP list URL. Updated every 24 hours, support format is same as ipBlockList, one rule per line |
| ipUploadedCheck | false | IP 上传增量检测. 在满足下列 IP 上传增量 条件后, 会自动屏蔽 Peer |
| ipUpCheckInterval | 300 (Sec) | IP 上传增量检测/检测间隔. 用于确定上一周期及当前周期, 以比对客户端对 IP 上传增量. 也顺便用于 maxIPPortCount |
| ipUpCheckIncrementMB | 38000 (MB) | IP 上传增量检测/增量大小. 若 IP 全局上传增量大小大于设置增量大小, 则允许屏蔽 Peer |
| ipUpCheckPerTorrentRatio | 3 (X) | IP 上传增量检测/增量倍率. 若 IP 单个 Torrent 上传增量大小大于设置增量倍率及 Torrent 大小之乘积, 则允许屏蔽 Peer |
| maxIPPortCount | 0 (Disable) | 每 IP 最大端口数. 若 IP 端口数大于设置值, 会自动屏蔽 Peer |
| banByProgressUploaded | false | 增强自动屏蔽 (根据进度及上传量屏蔽 Peer, 未经测试验证). 在满足下列 增强自动屏蔽 条件后, 会自动屏蔽 Peer |
| banByPUStartMB | 20 (MB) | 增强自动屏蔽/起始大小. 若客户端上传量大于起始大小, 则允许屏蔽 Peer |
| banByPUStartPrecent | 2 (%) | 增强自动屏蔽/起始进度. 若客户端上传进度大于设置起始进度, 则允许屏蔽 Peer |
| banByPUAntiErrorRatio | 3 (X) | 增强自动屏蔽/滞后防误判倍率. 若 Peer 报告下载进度与设置倍率及 Torrent 大小之乘积得到之下载量 比 客户端上传量 还低, 则允许屏蔽 Peer |
| banByRelativeProgressUploaded | false | 增强自动屏蔽_相对 (根据相对进度及相对上传量屏蔽 Peer, 未经测试验证). 在满足下列 增强自动屏蔽_相对 条件后, 会自动屏蔽 Peer. 此功能当前故障 |
| banByRelativePUStartMB | 20 (MB) | 增强自动屏蔽_相对/起始大小. 若客户端相对上传量大于设置起始大小, 则允许屏蔽 Peer |
| banByRelativePUStartPrecent | 2 (%) | 增强自动屏蔽_相对/起始进度. 若客户端相对上传进度大于设置起始进度, 则允许屏蔽 Peer |
| banByRelativePUAntiErrorRatio | 3 (X) | 增强自动屏蔽_相对/滞后防误判倍率. 若 Peer 报告相对下载进度与设置倍率之乘积得到之相对下载进度 比 客户端相对上传进度 还低, 则允许屏蔽 Peer |

## 反馈 Feedback
User and developer can report bug through [Issue](https://github.com/Simple-Tracker/qBittorrent-ClientBlocker/issues), through [Discussion](https://github.com/Simple-Tracker/qBittorrent-ClientBlocker/discussions) ask/discuss/share usage, through [Pull Request](https://github.com/Simple-Tracker/qBittorrent-ClientBlocker/pulls) contribute code improvement to blocker.  
Note: When opening a Pull Request for a Feature, please do not create an Issue simultaneously.

## 致谢 Credit

1. We partially referenced [jinliming2/qbittorrent-ban-xunlei](https://github.com/jinliming2/qbittorrent-ban-xunlei) during early development of blocker;
2. We will thank the user and developer who contributed code improvement to blocker through Pull Request in Release Note;
