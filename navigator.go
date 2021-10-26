package navigator

import (
	"github.com/PuerkitoBio/goquery"
	"github.com/go-rod/rod"
)

const (
	NAVIGATION_MAX_TRIES_COUNT = 10
)

type Navigator interface {
	NavigateUrl(url string) error
	SetProxyGetter(ProxyGetter)
	SetModel(*NavigatorModel)
	GetCrawler() *goquery.Document
	GetNavigateStatus() int
	DestroyClient(must ...bool)
	SetCaptchaSolver(CaptchaSolver)
}

type ProxyGetter interface {
	GetProxy() string
	FilterCountries(*[]string)
	FilterExcludeCountries(*[]string)
	FilterHttp(bool)
	FilterSock(bool)
	FilterHighSpeed(bool)
}

type CaptchaSolver interface {
	IsCaptcha(*rod.Page) bool
	SolveCaptcha(*rod.Page) (bool, error)
}

func NewNavigator(model *NavigatorModel) Navigator {
	var navigator Navigator

	if model.Chrome {
		navigator = new(ChromeNavigator)
	} else {
		// navigator = new(HttpNavigator)
	}

	navigator.SetModel(model)

	return navigator
}

//-------------------------------------------Геттеры свойств------------------------------------------
func (c *CustomNavigator) useProxy() bool {
	return !c.Model.DontUseProxy && c.Model.BanFlags != "" && c.ProxyGetter != nil
}

func (c *CustomNavigator) ban() bool {
	for _, code := range c.BanFlags.Codes {
		if code == c.StatusCode {
			return true
		}
	}
	for _, sel := range c.BanFlags.Selectors {
		el := c.Crawler.Find(sel.Selector)

		if (el.Size() == 0) == sel.Inverted {
			return true
		}
	}
	return false
}
