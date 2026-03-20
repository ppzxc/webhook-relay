package domain

type ChannelType string

const (
	ChannelTypeWebhook ChannelType = "WEBHOOK"
	ChannelTypeSlack   ChannelType = "SLACK"
	ChannelTypeDiscord ChannelType = "DISCORD"
)

func (c ChannelType) IsValid() bool {
	switch c {
	case ChannelTypeWebhook, ChannelTypeSlack, ChannelTypeDiscord:
		return true
	}
	return false
}
