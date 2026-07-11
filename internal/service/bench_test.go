package service

import (
	"context"
	"testing"
)

type BenchReq struct {
	N int `json:"n"`
}
type BenchRsp struct {
	N int `json:"n"`
}

// BenchSrv is a trivial in-process service: no work, so the measurement is pure
// framework round-trip cost (codec + registry + loopback transport).
type BenchSrv struct{}

func (BenchSrv) Echo(_ context.Context, req *BenchReq, rsp *BenchRsp) error {
	rsp.N = req.N + 1
	return nil
}

// echoDirect is the same work reached as a plain Go call, for comparison.
func echoDirect(n int) int { return n + 1 }

// BenchmarkServiceCall measures one in-process go-micro Call round-trip.
func BenchmarkServiceCall(b *testing.B) {
	if err := Register("benchsvc", BenchSrv{}); err != nil {
		b.Fatalf("register: %v", err)
	}
	ctx := context.Background()
	var rsp BenchRsp
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := Call(ctx, "benchsvc", "BenchSrv.Echo", &BenchReq{N: i}, &rsp); err != nil {
			b.Fatalf("call: %v", err)
		}
	}
}

// BenchmarkDirectCall is the baseline: a plain function call, no framework.
func BenchmarkDirectCall(b *testing.B) {
	b.ReportAllocs()
	sink := 0
	for i := 0; i < b.N; i++ {
		sink = echoDirect(i)
	}
	_ = sink
}
