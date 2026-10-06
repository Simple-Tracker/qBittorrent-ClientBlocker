package app

import "net"

func IsMatchCIDR(peerNet *net.IPNet) bool {
	if peerNet != nil {
		blockCIDRMapMutex.RLock()
		_, exist := blockCIDRMap[peerNet.String()]
		blockCIDRMapMutex.RUnlock()
		if exist {
			return true
		}
	}

	return false
}
