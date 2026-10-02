package ai

import (
	gmai "go-micro.dev/v6/model"
	"go-micro.dev/v6/model/anthropic"
	"go-micro.dev/v6/model/atlascloud"
	"go-micro.dev/v6/model/openai"
	"mu/internal/privacy"
)

func init() {
	gmai.Register("openai", func(opts ...gmai.Option) gmai.Model { return privacy.New(openai.NewProvider, opts...) })
	gmai.Register("anthropic", func(opts ...gmai.Option) gmai.Model { return privacy.New(anthropic.NewProvider, opts...) })
	gmai.Register("atlascloud", func(opts ...gmai.Option) gmai.Model { return privacy.New(atlascloud.NewProvider, opts...) })
}
