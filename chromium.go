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
		log.Printf("Try number %d\n", try)
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
		if err := c.Page.Timeout(30*time.Second).WaitElementsMoreThan("body", 0); err != nil {
			err = errors.New("cannot_load_page")
			c.StatusCode = -1
			c.DestroyClient(true)
			continue
		}

		c.Sleep(c.Model.DelayAfterNavigation)
		c.CreateCrawler(c.getPageHtml())

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
				c.CreateCrawler(c.getPageHtml())
			}
		}
	}
}

func (c *ChromeNavigator) CreateClientIfNeed() {
	if c.Browser != nil && c.Page != nil {
		return
	}
	c.Browser, c.Page = nil, nil

	rodLauncher := launcher.New().Headless(!c.Model.Visible).Proxy(c.GetProxy()).MustLaunch()
	c.Browser = rod.New().ControlURL(rodLauncher).MustConnect()
	c.Page = c.Browser.MustPage()

	if c.Model.Mobile {
		c.Page = c.Page.MustSetViewport(400, 800, 1, true)
	}

	return
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

//Получаем HTML в виде строки со страницы
func (c *ChromeNavigator) getPageHtml() string {
	if html, err := c.Page.Element("html"); err == nil {
		return html.MustHTML()
	} else {
		log.Println(err)
	}
	if body, err := c.Page.Element("body"); err == nil {
		return body.MustHTML()
	} else {
		log.Println(err)
	}
	return ""
}

//Вычисляем признак загрузки страницы
func (c *ChromeNavigator) getWaitPageStatus() func() {
	var waitEvent proto.PageLifecycleEventName
	switch c.Model.ChromeWaitFor {
	case "networkidle2":
		waitEvent = proto.PageLifecycleEventNameNetworkAlmostIdle
	case "networkidle0":
		waitEvent = proto.PageLifecycleEventNameNetworkIdle
	default:
		// waitEvent = proto.PageLifecycleEventNameDOMContentLoaded
		waitEvent = proto.PageLifecycleEventNameFirstContentfulPaint
	}
	return c.Page.Timeout(20 * time.Second).WaitNavigation(waitEvent)
}
