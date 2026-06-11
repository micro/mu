package discord

import (
	"sync"

	"mu/internal/data"
)

type GuildConfig struct {
	BriefingChannel string `json:"briefing_channel,omitempty"`
}

var (
	guildMu      sync.RWMutex
	guildConfigs = map[string]*GuildConfig{} // guild ID → config
)

func loadGuildConfigs() {
	data.LoadJSON("discord_guilds.json", &guildConfigs)
}

func getGuildConfig(guildID string) *GuildConfig {
	guildMu.RLock()
	defer guildMu.RUnlock()
	if c, ok := guildConfigs[guildID]; ok {
		return c
	}
	return &GuildConfig{}
}

func setGuildBriefingChannel(guildID, channelID string) {
	guildMu.Lock()
	defer guildMu.Unlock()
	c, ok := guildConfigs[guildID]
	if !ok {
		c = &GuildConfig{}
		guildConfigs[guildID] = c
	}
	c.BriefingChannel = channelID
	data.SaveJSON("discord_guilds.json", guildConfigs)
}

// getBriefingChannels returns all configured briefing channels across servers.
func getBriefingChannels() []string {
	guildMu.RLock()
	defer guildMu.RUnlock()
	var channels []string
	for _, c := range guildConfigs {
		if c.BriefingChannel != "" {
			channels = append(channels, c.BriefingChannel)
		}
	}
	return channels
}
