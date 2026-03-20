package domain

type OutputType string

const (
	OutputTypeWebhook OutputType = "WEBHOOK"
	OutputTypeSlack   OutputType = "SLACK"
	OutputTypeDiscord OutputType = "DISCORD"
)

func (c OutputType) IsValid() bool {
	switch c {
	case OutputTypeWebhook, OutputTypeSlack, OutputTypeDiscord:
		return true
	}
	return false
}
