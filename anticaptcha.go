package navigator

import (
	"errors"
	"os"
	"time"

	"github.com/nuveo/anticaptcha"
)

const (
	RECAPTCHA_SELECTOR      = "#g-recaptcha-response"
	RECAPTCHA_KEY_SELECTOR  = ".g-recaptcha"
	RECAPTCHA_KEY_ATTRIBUTE = "data-sitekey"
)

func (chn *ChromeNavigator) HandleRecaptcha() error {
	if !chn.hasRecaptcha() {
		return nil
	}
	return chn.resolveRecaptcha()
}

func (chn *ChromeNavigator) hasRecaptcha() bool {
	res := chn.page.MustEval(
		`selector => document.querySelectorAll(selector).length > 0 ? '1' : '0'`,
		RECAPTCHA_SELECTOR,
	)

	return res.Str() == "1"
}

func (chn *ChromeNavigator) resolveRecaptcha() error {
	key := os.Getenv("ANTICAPTCHA_KEY")
	if key == "" {
		return errors.New("No anticaptcha key")
	}

	recaptchaKey := chn.getRecaptchaKey()
	if recaptchaKey == "" {
		return errors.New("Recapcha exist, but cannot find its key")
	}

	ac := &anticaptcha.Client{APIKey: key}

	key, err := ac.SendRecaptcha(chn.URL, recaptchaKey, time.Second*300)
	if err != nil {
		return err
	}

	return chn.enterRecaptchaAnswer(key)
}

func (chn *ChromeNavigator) getRecaptchaKey() string {
	element, err := chn.page.Element(RECAPTCHA_KEY_SELECTOR)
	if err != nil || element == nil {
		return ""
	}

	key, err := element.Attribute(RECAPTCHA_KEY_ATTRIBUTE)
	if err != nil || key == nil {
		return ""
	}
	return *key
}

func (chn *ChromeNavigator) enterRecaptchaAnswer(answer string) error {
	_, err := chn.page.Eval(`(answer, selector) => {
		document.querySelector(selector).value = answer
		document.querySelector('form').submit()
	}`, answer, RECAPTCHA_SELECTOR)

	if err != nil {
		return err
	}

	if err := chn.WaitPageLoad(); err != nil {
		return err
	}

	return err
}
