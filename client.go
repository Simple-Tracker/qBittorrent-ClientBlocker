package main

func IsBanPort() bool {
	if qB_useNewBanPeersMethod {
		return true
	}

	return false
}
func Login() bool {
	return qB_Login()
}
func FetchMaindata() interface{} {
	return qB_FetchMaindata()
}
func FetchTorrentPeers(infoHash string) interface{} {
	return qB_FetchTorrentPeers(infoHash)
}
func SubmitBlockPeer(blockPeerMap map[string]BlockPeerInfoStruct) {
	qB_SubmitBlockPeer(blockPeerMap)
}
