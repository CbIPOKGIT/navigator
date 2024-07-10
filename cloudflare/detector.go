package cloudflare

import (
	"time"

	"github.com/go-rod/rod"
)

const (
	DEFAULT_CAPTCHA_SELECTOR = "#turnstile-wrapper, #ie-container, #turnstile-box"
)

func (s *Solver) Is(page *rod.Page) bool {
	time.Sleep(time.Millisecond * 400)
	if s.captchaSelector != "" {
		return s.hasElementBySelector(page, s.captchaSelector)
	}

	return s.hasElementBySelector(page, DEFAULT_CAPTCHA_SELECTOR)
}

func (s *Solver) hasElementBySelector(page *rod.Page, selector string) bool {
	elements, err := page.Elements(selector)
	return err == nil && len(elements) > 0
}
