package navigator

import (
	"errors"
	"log"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/proto"
)

type ChromeNavigator struct {
	CustomNavigator

	Browser *rod.Browser
	Page    *rod.Page
}

func (c *ChromeNavigator) NavigateUrl(url string) error {
	if err := c.gotoUrl(url); err != nil {
		return err
	}

	c.solveCaptcha()
	return nil
}

func (c *ChromeNavigator) gotoUrl(url string) error {
	var err error
	for try := 0; try < NAVIGATION_MAX_TRIES_COUNT; try++ {
		if try > 0 && c.DontTryAnyMore {
			err = errors.New("cannot_load_page")
			break
		}

		if try > 0 {
			c.DestroyClient(true)
		}

		c.CreateClientIfNeed()
		c.Sleep(c.Model.DelayBeforeNavigation)

		e := proto.NetworkResponseReceived{}
		waitEvent := c.Page.Timeout(10 * time.Second).WaitEvent(&e)
		// waitPage := c.getWaitPageStatus()

		if err = c.Page.Navigate(url); err != nil {
			err = errors.New("cannot_load_page")
			c.StatusCode = -1
			c.DestroyClient(true)
			continue
		}
		waitEvent()
		if e.Response == nil {
			err = errors.New("cannot_load_page")
			c.StatusCode = -1
			c.DestroyClient(true)
			continue
		} else {
			c.StatusCode = e.Response.Status
		}
		// waitPage()
		if err2 := c.getWaitPageStatus(); err2 != nil {
			err = errors.New("cannot_load_page")
			c.StatusCode = -1
			c.DestroyClient(true)
			continue
		}

		c.Sleep(c.Model.DelayAfterNavigation)

		var html = ""
		if html, err = c.getPageHtml(); err == nil {
			c.CreateCrawler(html)
		} else {
			c.StatusCode = -1
			c.DestroyClient(true)
			continue
		}

		if c.ban() {
			c.StatusCode = 0
			continue
		} else {
			return nil
		}
	}
	c.DontTryAnyMore = true
	return err
}

func (c *ChromeNavigator) solveCaptcha() {
	step := 0

	processError := func() {
		c.DestroyClient(true)
		c.StatusCode = 0
	}

	for c.CaptchaSolver != nil && c.CaptchaSolver.IsCaptcha(c.Page) {
		if step > 5 {
			processError()
			return
		}

		if solved, err := c.CaptchaSolver.SolveCaptcha(c.Page); err != nil {
			processError()
			break
		} else {
			if solved {
				c.Sleep(c.Model.DelayAfterNavigation)
				html, _ := c.getPageHtml()
				c.CreateCrawler(html)
			}
		}
	}

	if c.ban() {
		c.StatusCode = 0
	}
}

func (c *ChromeNavigator) CreateClientIfNeed() {
	if c.Browser != nil && c.Page != nil {
		if c.Model.CloseEveryTime {
			c.CloseAndCreateTab()
		}
		return
	}
	c.Browser, c.Page = nil, nil

	rodLauncher := launcher.New().Headless(!c.Model.Visible).Proxy(c.GetProxy())
	if !c.Model.ShowImg {
		rodLauncher.Set("--blink-settings=imagesEnabled", "false")
	}
	c.Browser = rod.New().ControlURL(rodLauncher.MustLaunch()).MustConnect()
	c.Page = c.Browser.MustPage()

	if c.Model.Mobile {
		c.Page = c.Page.MustSetViewport(400, 800, 1, true)
	}
}

func (c *ChromeNavigator) DestroyClient(must ...bool) {
	if c.Browser != nil {
		c.Browser.Close()
	}
	c.Browser, c.Page = nil, nil

	if len(must) > 0 && must[0] {
		c.CurrentProxy = ""
	}
}

//Создаем новую вкладку, закрываем старую
func (c *ChromeNavigator) CloseAndCreateTab() {
	newPage := c.Browser.MustPage()
	c.Page.Close()
	c.Page = newPage
}

//Получаем HTML в виде строки со страницы
func (c *ChromeNavigator) getPageHtml() (string, error) {
	if html, err := c.Page.Element("html"); err == nil {
		return html.HTML()
	} else {
		log.Println(err)
	}
	if body, err := c.Page.Element("body"); err == nil {
		return body.HTML()
	} else {
		log.Println(err)
	}
	return "", errors.New("no_page_html")
}

//Вычисляем признак загрузки страницы
func (c *ChromeNavigator) getWaitPageStatus() error {
	timeout := c.Page.Timeout(30 * time.Second)
	if c.Model.ChromeWaitFor == "" {
		return timeout.WaitLoad()
	} else {
		return timeout.WaitElementsMoreThan(c.Model.ChromeWaitFor, 0)
	}
}
