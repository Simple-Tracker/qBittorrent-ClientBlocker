package stats

type PeerTrafficCounter struct {
	Downloaded int64
	Uploaded   int64
	LastSeen   int64
	PeerID     string
}

// AlignPeerTrafficCounter 在 ID 为空时保留最近一次已知的身份.
// 确认身份更换后, 两个原始计数均从零开始, 即使当前计数未知也如此.
func AlignPeerTrafficCounter(counter PeerTrafficCounter, peerIDs ...string) (PeerTrafficCounter, bool) {
	if len(peerIDs) == 0 || peerIDs[0] == "" {
		return counter, false
	}
	changed := counter.PeerID != "" && counter.PeerID != peerIDs[0]
	if changed {
		counter.Downloaded, counter.Uploaded = 0, 0
	}
	counter.PeerID = peerIDs[0]
	return counter, changed
}

// CounterDelta 处理连接计数从零重新开始的情况.
func CounterDelta(current, previous int64) int64 {
	if current < 0 {
		return 0
	}
	if previous < 0 || current < previous {
		return current
	}
	return current - previous
}

func AccumulateCounter(total, current, previous int64) int64 {
	if current < 0 && total <= 0 {
		return -1
	}
	if total < 0 {
		total = 0
	}
	return total + CounterDelta(current, previous)
}
