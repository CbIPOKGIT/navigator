package cloudflare

import (
	"time"

	"github.com/go-rod/rod"
)

const (
	DEFAULT_CAPTCHA_SELECTOR = "#turnstile-wrapper, #ie-container, #turnstile-box, .main-content .core-msg"
)

func (s *Solver) Is(page *rod.Page) bool {
	if page == nil {
		return false
	}

	time.Sleep(time.Millisecond * 400)
	if s.captchaSelector != "" {
		return s.hasElementBySelector(page, s.captchaSelector)
	}

	return s.hasElementBySelector(page, DEFAULT_CAPTCHA_SELECTOR)
}

func (s *Solver) hasElementBySelector(page *rod.Page, selector string) (has bool) {
	if page == nil {
		return false
	}

	defer func() {
		if recover() != nil {
			has = false
		}
	}()

	elements, err := page.Elements(selector)
	return err == nil && len(elements) > 0
}
