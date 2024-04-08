//go:build !windows
package main

func main() {
	/*
	torrentMap2 := make(map[string]TorrentInfoStruct)
	peers := make(map[string]PeerInfoStruct)
	peerPortMap := make(map[int]bool)
	peerPortMap[233] = true
	peers["testpeer"] = PeerInfoStruct { Port: peerPortMap, Progress: 0.1, Uploaded: 123 }
	torrentMap2["testhash"] = TorrentInfoStruct { Size: 233, Peers: peers }
	lastTorrentMap2 := make(map[string]TorrentInfoStruct)
	DeepCopyTorrentMap(torrentMap2, lastTorrentMap2)

	for torrentInfoHash, torrentInfo := range lastTorrentMap2 {
		Log("Test", "%s %d", false, torrentInfoHash, torrentInfo.Size)
		for peerIP, peerInfo := range torrentInfo.Peers {
			Log("Test", "%s %v", false, peerIP, peerInfo.Progress)
			for port, _ := range peerInfo.Port {
				Log("Test", "%d", false, port)
			}
		}
	}
	return
	*/
	
	if PrepareEnv() {
		RunConsole()
	}
}
func Platform_Stop() {
}
