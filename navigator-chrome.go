package navigator

import (
	"errors"
	"fmt"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/proto"
)

const (
	DEFAULT_BROWSER_NAVIGATION_TIMEOUT = 60
)

type ChromeNavigator struct {
	CommonNavigator

	Browser *rod.Browser
	Page    *rod.Page
}

func (navigator *ChromeNavigator) Close() error {
	if navigator.Page != nil {
		if err := navigator.Page.Close(); err != nil {
			return err
		}
		navigator.Page = nil
	}

	if navigator.Browser != nil && !navigator.Model.UseSystemChrome {
		if err := navigator.Close(); err != nil {
			return err
		}

		navigator.Browser = nil
	}

	return nil
}

func (navigator *ChromeNavigator) Navigate(url string) error {
	if err := navigator.writeAndFormatURL(url); err != nil {
		return err
	}

	return navigator.navigateUrl()
}

func (navigator *ChromeNavigator) navigateUrl() error {
	var reloading bool = false

	for i := 0; i < navigator.calculateTriesCount(); i++ {
		if i > 0 {
			reloading = false
			navigator.Close()
		} else {
			if !navigator.JustCreated && navigator.Model.DelayBeforeNavigate > 0 {
				time.Sleep(time.Second * time.Duration(navigator.Model.DelayBeforeNavigate))
			}
		}

		if err := navigator.createClientIfNeed(); err != nil {
			navigator.LastError = err
			break
		}

		response, err := navigator.waitPageResponse(navigator.Url)
		if err != nil {
			navigator.LastError = err
			continue
		}

		if err := navigator.waitPageLoaded(); err != nil {
			navigator.LastError = err
			continue
		}

		navigator.NavigateStatus = response.Response.Status

		html, err := navigator.Page.HTML()
		if err != nil {
			navigator.LastError = errors.New(fmt.Sprintf("Error read HTML from page: %s", err.Error()))
			continue
		}

		if navigator.Model.DelayBeforeRead > 0 {
			time.Sleep(time.Second * time.Duration(navigator.Model.DelayBeforeRead))
		}

		if err := navigator.createCrawlerFromHTML(html); err != nil {
			navigator.LastError = errors.New(fmt.Sprintf("Error create crawler from HTML: %s", err.Error()))
			continue
		}

		if navigator.JustCreated && navigator.Model.EmptyLoad && !reloading {
			i = -1
			reloading = true
			continue
		}

		if navigator.checkNavigateStatus(i) {
			break
		}
	}

	return navigator.LastError
}

// Wait for url response
func (navigator *ChromeNavigator) waitPageResponse(url string) (*proto.NetworkResponseReceived, error) {
	response := proto.NetworkResponseReceived{}
	wait := navigator.Page.WaitEvent(&response)

	channelSuccess := make(chan interface{})
	go func() {
		wait()
		channelSuccess <- nil
	}()

	timeout := time.NewTimer(time.Duration(navigator.getPageLoadTimeout()) * time.Second)

	if err := navigator.Page.Navigate(url); err != nil {
		return nil, err
	}

	select {
	case <-channelSuccess:
		return &response, nil
	case <-timeout.C:
		return nil, errors.New("Timeout navigation")
	}
}

// Wait for page loaded
func (navigator *ChromeNavigator) waitPageLoaded() error {
	timeout := time.NewTimer(time.Duration(navigator.getPageLoadTimeout()) * time.Second)

	waitChan := make(chan error)

	if navigator.Model.NavigationSelector != "" {
		go func() {
			waitChan <- navigator.Page.WaitElementsMoreThan(navigator.Model.NavigationSelector, 1)
		}()
	} else {
		var eventName = proto.PageLifecycleEventNameDOMContentLoaded

		switch navigator.Model.NavigationWaitfor {
		case 1:
			eventName = proto.PageLifecycleEventNameNetworkAlmostIdle
		case 2:
			eventName = proto.PageLifecycleEventNameNetworkIdle
		case 3:
			eventName = proto.PageLifecycleEventNameLoad
		}

		wait := navigator.Page.WaitNavigation(eventName)

		go func() {
			wait()
			waitChan <- nil
		}()
	}

	select {
	case err := <-waitChan:
		return err
	case <-timeout.C:
		return errors.New("Timeout navigation")
	}
}

// Get page loading timeout
func (navigator *ChromeNavigator) getPageLoadTimeout() int {
	if navigator.Model.NavigationTimeout > 0 {
		return navigator.Model.NavigationTimeout
	} else {
		return DEFAULT_BROWSER_NAVIGATION_TIMEOUT
	}
}

func (navigator *ChromeNavigator) createClientIfNeed() error {
	if navigator.Page != nil {
		navigator.JustCreated = false
		return nil
	}

	var u string
	var err error

	var useSystemChrome bool = navigator.Model.UseSystemChrome

	// Пробуємо системний хром якщо потрібен.
	// В випадку помилки будемо запускатись стандартно
	if useSystemChrome {
		u, err = launcher.NewUserMode().Launch()
		if err != nil {
			useSystemChrome = false
		}
	}

	if !useSystemChrome {
		l := launcher.New().
			Headless(!navigator.Model.Visible && !navigator.Model.UseSystemChrome).
			Set("blink-settings", fmt.Sprintf("imagesEnabled=%t", navigator.Model.ShowImages))

		if len(navigator.Model.BanSettings) > 0 && navigator.PrxGetter != nil {
			proxyvalue, err := navigator.PrxGetter.GetProxy()
			if err == nil {
				l.Proxy(proxyvalue)
			}
		}

		u, err = l.Launch()
	}

	if err != nil {
		return err
	}

	navigator.Browser = rod.New().ControlURL(u).MustConnect()
	navigator.Page = navigator.Browser.MustPage()
	navigator.JustCreated = true

	return nil
}
