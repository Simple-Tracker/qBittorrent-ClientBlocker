package main

type PeerTrafficCounter struct {
	Downloaded int64
	Uploaded   int64
	LastSeen   int64
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
