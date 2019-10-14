package main

import (
	"bytes"
	"testing"
)

func Benchmark_drawClock(b *testing.B) {

	var buf bytes.Buffer

	schedule1 := randomSchedule(3)
	schedule2 := randomSchedule(4)
	schedule3 := randomSchedule(5)
	for n := 0; n < b.N; n++ {

		_ = drawClock(schedule1, &buf)
		_ = drawClock(schedule2, &buf)
		_ = drawClock(schedule3, &buf)

		buf.Reset()
	}
}
