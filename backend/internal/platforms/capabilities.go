// Package platforms describes what each external platform allows through
// official APIs. The catalog drives the guided Platform Setup flow; it
// never claims an account exists or is connected. See
// docs/platform-capabilities.md for sources and verification status.
package platforms

// Automation describes how a setup step is performed.
type Automation string

// Automation levels.
const (
	// Automated: performed through an official API after user consent.
	Automated Automation = "automated"
	// UserAction: the user must complete the step on the platform itself.
	UserAction Automation = "user_action"
	// Verified: the platform API confirms the result of the step.
	Verified Automation = "verified"
)

// Step is one step of a platform's setup workflow.
type Step struct {
	ID          string     `json:"id"`
	Title       string     `json:"title"`
	Description string     `json:"description"`
	Automation  Automation `json:"automation"`
	URL         string     `json:"url,omitempty"`
}

// Capability summarizes one platform.
type Capability struct {
	Platform         string   `json:"platform"`
	DisplayName      string   `json:"displayName"`
	AccountCreation  string   `json:"accountCreation"`
	OAuthScopes      []string `json:"oauthScopes"`
	Limits           []string `json:"limits"`
	Steps            []Step   `json:"steps"`
	IntegrationReady bool     `json:"integrationReady"`
	LastReviewed     string   `json:"lastReviewed"`
}

// Catalog returns the platform capability catalog. Figures that public
// sources disagree on are marked "verify" and must be confirmed against
// the official documentation before relying on them. youtubeReady reports
// whether the YouTube OAuth integration is configured.
func Catalog(youtubeReady bool) []Capability {
	return []Capability{
		{
			Platform:        "youtube",
			DisplayName:     "YouTube",
			AccountCreation: "No API creates channels. The user creates the channel in YouTube, then the app detects it through the Data API.",
			OAuthScopes: []string{
				"https://www.googleapis.com/auth/youtube.upload",
				"https://www.googleapis.com/auth/youtube.readonly",
				"https://www.googleapis.com/auth/yt-analytics.readonly",
			},
			Limits: []string{
				"Data API default quota: 10,000 units per day per Google Cloud project.",
				"videos.insert quota cost changed during 2025-2026; sources disagree (verify in the Google quota calculator).",
				"Uploads from unaudited API projects have been restricted to private; recent reports say this was relaxed (verify).",
			},
			Steps: []Step{
				{ID: "identity", Title: "Prepare channel identity", Description: "Finish the name, bio, avatar, and banner in the Branding step.", Automation: Automated},
				{ID: "oauth", Title: "Sign in with Google", Description: "Authorize this app with OAuth 2.0. Passwords are never entered into or stored by this app.", Automation: UserAction},
				{ID: "create", Title: "Create the channel on YouTube", Description: "If the Google account has no channel for this brand, create one in YouTube. Use the generated name and handle suggestions.", Automation: UserAction, URL: "https://www.youtube.com/create_channel"},
				{ID: "detect", Title: "Detect and verify the channel", Description: "The app lists the channels the authorized account can manage and saves the selected channel ID.", Automation: Verified},
				{ID: "profile", Title: "Apply profile information", Description: "Paste the generated description and keywords and upload the avatar and banner in YouTube Studio. Editing them through the API needs a broader permission that this app does not request yet.", Automation: UserAction, URL: "https://studio.youtube.com/"},
				{ID: "permissions", Title: "Verify upload and analytics permissions", Description: "The app checks the granted scopes and confirms it can read the channel.", Automation: Verified},
			},
			IntegrationReady: youtubeReady,
			LastReviewed:     "2026-11-10",
		},
		{
			Platform:        "tiktok",
			DisplayName:     "TikTok",
			AccountCreation: "No API creates accounts. The user creates the account in the TikTok app.",
			OAuthScopes:     []string{"user.info.basic", "video.upload", "video.publish", "video.list"},
			Limits: []string{
				"Unaudited API clients can post only as private (SELF_ONLY), to private accounts, for up to 5 users per 24 hours.",
				"Public posting requires passing the Content Posting API audit.",
				"Per-creator Direct Post cap is roughly 15 posts per day, shared across API clients.",
			},
			Steps: []Step{
				{ID: "create", Title: "Create the TikTok account", Description: "Create the account in the TikTok app using the generated name and bio.", Automation: UserAction, URL: "https://www.tiktok.com/signup"},
				{ID: "oauth", Title: "Authorize with Login Kit", Description: "Authorize this app with TikTok OAuth.", Automation: UserAction},
				{ID: "detect", Title: "Verify the account", Description: "The app reads the account profile through the API and saves its ID.", Automation: Verified},
				{ID: "profile", Title: "Set profile picture and bio", Description: "TikTok does not expose profile editing through the public API. Apply the generated assets in the app.", Automation: UserAction},
				{ID: "permissions", Title: "Verify posting permissions", Description: "The app checks granted scopes and the client's audit status.", Automation: Verified},
			},
			LastReviewed: "2026-11-10",
		},
		{
			Platform:        "instagram",
			DisplayName:     "Instagram",
			AccountCreation: "No API creates accounts. The user creates the account and switches it to a professional (Business or Creator) account.",
			OAuthScopes:     []string{"instagram_business_basic", "instagram_business_content_publish"},
			Limits: []string{
				"Publishing requires a professional account and Meta app review for publishing permissions.",
				"API publishing is capped per account per rolling 24 hours; Meta pages list 50 or 100 (verify with the content_publishing_limit endpoint).",
			},
			Steps: []Step{
				{ID: "create", Title: "Create the Instagram account", Description: "Create the account in Instagram and switch it to a professional account.", Automation: UserAction, URL: "https://www.instagram.com/accounts/emailsignup/"},
				{ID: "oauth", Title: "Authorize with Instagram Login", Description: "Authorize this app with Instagram OAuth.", Automation: UserAction},
				{ID: "detect", Title: "Verify the account", Description: "The app reads the account through the API and saves its ID.", Automation: Verified},
				{ID: "profile", Title: "Set profile picture and bio", Description: "Profile editing is not available through the publishing API. Apply the generated assets in Instagram.", Automation: UserAction},
				{ID: "permissions", Title: "Verify publishing permissions", Description: "The app checks granted permissions and the current publishing quota.", Automation: Verified},
			},
			LastReviewed: "2026-11-10",
		},
	}
}
