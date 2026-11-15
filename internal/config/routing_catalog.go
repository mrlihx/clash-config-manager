package config

var serviceEmoji = map[string]string{
	"Claude": "🧠", "Gemini": "✨", "OpenAI": "🤖", "Grok": "🦙", "Perplexity": "🔎",
	"GooglePlay": "🛍️", "YouTube": "▶️", "Google": "🌐", "Telegram": "📨", "Discord": "🎮", "Twitter": "🐦", "Facebook": "📘",
	"Netflix": "🎬", "Disney": "🏰", "Spotify": "🎵", "TikTok": "🎶", "GitHub": "🐙", "Steam": "🎮",
}

var serviceSlugs = map[string]string{
	"Ads": "category-ads-all", "SteamCDN": "steam@cn", "Claude": "anthropic", "Gemini": "google-gemini", "OpenAI": "openai", "Grok": "xai", "Perplexity": "perplexity",
	"GooglePlay": "google-play", "YouTube": "youtube", "Google": "google", "Telegram": "telegram", "Discord": "discord", "Twitter": "twitter", "Facebook": "meta",
	"Netflix": "netflix", "Disney": "disney", "Spotify": "spotify", "TikTok": "tiktok", "GitHub": "github", "Steam": "steam",
}

func init() {
	for i := range Categories {
		if emoji := serviceEmoji[Categories[i].ID]; emoji != "" {
			Categories[i].Name = emoji + " " + Categories[i].Name
		}
	}
}

func BuiltinProviderSources() map[string]string {
	sources := map[string]string{}
	for id, slug := range serviceSlugs {
		name := "System_" + id
		if id == "Ads" {
			name = "Ads"
		}
		sources[name] = "https://raw.githubusercontent.com/MetaCubeX/meta-rules-dat/meta/geo/geosite/" + slug + ".yaml"
	}
	return sources
}
