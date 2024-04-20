# SyncServer
SyncServer 同步服务器是一个设想. 在设想中: 屏蔽器客户端发送所有的 TorrentMap (其中包括 Peer), 服务器即掌握所有连接至该服务器的屏蔽器客户端的 Torrent 及 Peer 的相关信息. 服务器通过所掌握的信息作出决策, 这些决策随本次或下次请求返回给客户端.

## 请求结构
一个示例应该如下:
```
POST /api/syncserver
Content-Type: application/json

{
	"version": 1,
	"timestamp": 1714306700,
	"token": "",
	"torrentMap": {
		"(InfoHash)": {
			"size": 1048576,
			"peers": {
				"(PeerIP)": {
					"net": "(Unknown)",
					"port": {
						(Port): true
					},
					"progress": 0.233,
					"downloaded": 1048576,
					"uploaded": 1024
				}
			}
		}
	}
}
```

## 响应结构
客户端会根据服务器的响应重新调整自身 ```Interval```, 若要避免尖锋, 可在每次返回时使用不同的 Offset 调整 ```Interval```. 若没有发生错误, 则 ```Status``` 应该置空.  
一个示例应该如下:
```
{
	"interval": 300,
	"status": "",
	"blockIPRule": {
		"(Reason)": [
			"123.132.213.231/32"
		]
	}
}
```
