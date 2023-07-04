package navigator

const (
	WAIT_DOMCONTENTLOADED int = iota
	WAIT_NETWORK_ALMOST_IDLE
	WAIT_NETWORK_IDLE
	WAIT_LOAD
)

// Navigator rules
type Model struct {

	// Use chrome insteal of gentelman ([gopkg.in/h2non/gentleman.v2])
	Chrome bool `json:"chrome"`

	// Chrome visible or headless
	Visible bool `json:"visible"`

	// Show images in chrome or not
	ShowImages bool `json:"show_images"`

	// Use browser from OS or own
	UseSystemChrome bool `json:"use_system_chrome"`

	// Event wich need to fire, that we would know the page is loaded enought
	NavigationWaitfor int `json:"navigation_waitfor"`

	// Element selector which loading means that page is loaded enought
	NavigationSelector string `json:"navigation_selector"`

	// Timeout navigation
	NavigationTimeout int `json:"navigation_timeout"`

	// For the first page load we need load once more
	EmptyLoad bool `json:"empty_load"`

	// Chrome user agent
	UserAgent string `json:"user_agent"`

	// Delay before read DOM tree
	DelayBeforeRead int `json:"delay_before_read"`

	// Delay before navigating next URL
	DelayBeforeNavigate int `json:"delay_before_navigate"`

	// Captcha selector. If has selector - we have a captcha on page.
	CaptchaSelector string `json:"captcha_selector"`

	// Captcha selector inverted, so if there is no element by selector - there is a captcha
	CaptchaSelectorInverted bool `json:"captcha_selector_inverted"`

	// List of initial cookies on page
	InitialCookies map[string]string `json:"initial_cookies"`

	// JS script wich eval after first navigation
	PreScript string `json:"pre_script"`

	// Wait reload after prescript execute
	PreScriptNeedReload bool `json:"pre_scrpit_need_reload"`

	// Selector for challange form
	ChallangeSelector string `json:"challange_selector"`

	// Bad statuses to check is domen banned us or not
	BanSettings []*BanFlags `json:"ban_settings"`

	// Close client after every navigation
	ClosePageEverytime bool `json:"close_page_everytime"`
}

type BanFlags struct {
	Selector string `json:"selector"`
	Code     int    `json:"code"`
	Inverted bool   `json:"inverted"`
}
