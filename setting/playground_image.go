package setting

import "sync/atomic"

var playgroundImageMaxConcurrency atomic.Int64

func GetPlaygroundImageMaxConcurrency() int {
	return int(playgroundImageMaxConcurrency.Load())
}

func SetPlaygroundImageMaxConcurrency(value int) {
	if value < 0 {
		value = 0
	}
	playgroundImageMaxConcurrency.Store(int64(value))
}
