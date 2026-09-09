package service

import (
	"context"
	"go-micro.dev/v6/client"
	"time"
)

// OnCallTiming is wired once by the host before serving requests.
var OnCallTiming func(string, string, time.Duration, error)

type timedClient struct{ client.Client }

func timingClientWrapper(c client.Client) client.Client { return &timedClient{c} }
func (c *timedClient) Call(ctx context.Context, req client.Request, rsp interface{}, opts ...client.CallOption) (err error) {
	start := time.Now()
	defer func() {
		if OnCallTiming != nil {
			OnCallTiming(req.Service(), req.Endpoint(), time.Since(start), err)
		}
	}()
	return c.Client.Call(ctx, req, rsp, opts...)
}
