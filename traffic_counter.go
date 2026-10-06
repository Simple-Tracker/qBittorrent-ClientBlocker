package main

type PeerTrafficCounter struct {
	Downloaded int64
	Uploaded   int64
	LastSeen   int64
	PeerID     string
}

// AlignPeerTrafficCounter preserves the last known identity through empty IDs.
// A confirmed replacement starts both raw counters at zero, including while unknown.
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

// CounterDelta handles a connection counter restarting from zero.
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
