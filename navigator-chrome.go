package navigator

import (
	"errors"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/proto"
)

const (
	DEFAULT_BROWSER_NAVIGATION_TIMEOUT = 60
	CHALLANGE_TRIES_MAX_DURATION       = 30 // Challange beet max duration time
)

type ChromeNavigator struct {
	CommonNavigator

	Browser *rod.Browser
	Page    *rod.Page
}

// Interface implementation
func (navigator *ChromeNavigator) Close() error {
	var errPage, errBrowser error = navigator.closePage(), navigator.closeBrowser()
	if errPage != nil {
		return errPage
	}
	if errBrowser != nil {
		return errBrowser
	}
	return nil
}

func (navigator *ChromeNavigator) closePage() error {
	var err error
	if navigator.Page != nil {
		err = navigator.Page.Close()
		navigator.Page = nil
	}
	return err
}

func (navigator *ChromeNavigator) closeBrowser() error {
	var err error
	if navigator.Browser != nil && !navigator.Model.UseSystemChrome {
		err = navigator.Close()
		navigator.Browser = nil
	}
	return err
}

// Interface implementation
func (navigator *ChromeNavigator) Navigate(url string) error {
	if err := navigator.writeAndFormatURL(url); err != nil {
		return err
	}

	return navigator.navigateUrl()
}

// Evaluate script
func (navigator *ChromeNavigator) Evaluate(script string, args ...string) (string, error) {
	navigator.createClientIfNeed()

	result, err := navigator.Page.Eval(script, args)
	if err != nil {
		return "", err
	}
	return result.Value.Str(), nil
}

func (navigator *ChromeNavigator) navigateUrl() error {
	if navigator.Model.ClosePageEverytime && navigator.Page != nil {
		navigator.Page.Close()
	}

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

		if err := navigator.waitTotalLoad(navigator.Url); err != nil {
			navigator.LastError = err
			continue
		}

		if navigator.JustCreated {
			if err := navigator.executePreScript(); err != nil {
				navigator.LastError = err
				continue
			}
		}

		html, err := navigator.Page.HTML()
		if err != nil {
			navigator.LastError = errors.New(fmt.Sprintf("Error read HTML from page: %s", err.Error()))
			continue
		}

		if navigator.Model.DelayBeforeRead > 0 {
			time.Sleep(time.Second * time.Duration(navigator.Model.DelayBeforeRead))
		}

		if navigator.JustCreated && navigator.Model.EmptyLoad && !reloading {
			i = -1
			reloading = true
			continue
		}

		if err := navigator.beatChallange(); err != nil {
			navigator.LastError = err
			continue
		}

		if err := navigator.solveCaptcha(); err != nil {
			navigator.LastError = err
			continue
		}

		if err := navigator.createCrawlerFromHTML(html); err != nil {
			navigator.LastError = errors.New(fmt.Sprintf("Error create crawler from HTML: %s", err.Error()))
			continue
		}

		if navigator.isValidResponse(i) {
			break
		}
	}

	return navigator.LastError
}

// Wait for url response and sign page loaded
func (navigator *ChromeNavigator) waitTotalLoad(url ...string) error {
	response, err := navigator.waitPageResponse(url...)
	if err != nil {
		return err
	}

	if err := navigator.waitPageLoaded(); err != nil {
		return err
	}

	navigator.LastError = nil
	navigator.NavigateStatus = response.Response.Status
	return nil
}

// Wait for url response
func (navigator *ChromeNavigator) waitPageResponse(url ...string) (*proto.NetworkResponseReceived, error) {
	response := proto.NetworkResponseReceived{}
	wait := navigator.Page.WaitEvent(&response)

	channelSuccess := make(chan interface{})
	go func() {
		wait()
		channelSuccess <- nil
	}()

	timeout := time.NewTimer(time.Duration(navigator.getPageLoadTimeout()) * time.Second)

	if len(url) > 0 {
		if err := navigator.Page.Navigate(url[0]); err != nil {
			return nil, err
		}
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

// If page is nil - create new page
func (navigator *ChromeNavigator) createClientIfNeed() error {
	if navigator.Page != nil {
		navigator.JustCreated = false
		return nil
	}

	if navigator.Browser == nil {
		var err error
		navigator.Browser, err = navigator.createBrowser()
		if err != nil {
			return err
		}
	}

	navigator.Page = navigator.Browser.MustPage()
	navigator.JustCreated = true

	return nil
}

func (navigator *ChromeNavigator) createBrowser() (*rod.Browser, error) {
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
		return nil, err
	}

	return rod.New().ControlURL(u).MustConnect().NoDefaultDevice(), nil
}

// Beat the challange. Its something like Cloudflare protection.
//
// Max reloads count - 5
func (navigator *ChromeNavigator) beatChallange() error {
	if navigator.Model.ChallangeSelector == "" {
		return nil
	}

	var stopReloading atomic.Bool

	successChannel := make(chan error)

	go func() {
		for {
			if stopReloading.Load() {
				return
			}

			elements, err := navigator.Page.Elements(navigator.Model.ChallangeSelector)
			if err != nil {
				successChannel <- err
				return
			}

			if elements.Empty() {
				successChannel <- nil
				return
			}

			if err := navigator.waitTotalLoad(); err != nil {
				successChannel <- err
				return
			}
		}
	}()

	timer := time.NewTimer(time.Second * CHALLANGE_TRIES_MAX_DURATION)

	select {
	case err := <-successChannel:
		stopReloading.Store(true)
		return err
	case <-timer.C:
		stopReloading.Store(true)
		return errors.New("Unable pass challange form")
	}
}

// Solve captcha if presented
func (navigator *ChromeNavigator) solveCaptcha() error {
	if navigator.Model.CaptchaSelector == "" || navigator.CptchSolver == nil {
		return nil
	}

	elements, err := navigator.Page.Elements(navigator.Model.CaptchaSelector)
	if err != nil {
		return err
	}

	hasCaptcha := elements.Empty() == navigator.Model.CaptchaSelectorInverted
	if !hasCaptcha {
		return nil
	}

	navigator.CptchSolver.SetPage(navigator.Page)
	solved, err := navigator.CptchSolver.Solve()
	if err != nil {
		return err
	}
	if !solved {
		return errors.New("Unable to solve captcha")
	}
	return nil
}

// Execute pre script
func (navigator *ChromeNavigator) executePreScript() error {
	if navigator.Model.PreScript == "" {
		return nil
	}

	_, err := navigator.Evaluate(navigator.Model.PreScript)
	if err != nil {
		return err
	}

	if navigator.Model.PreScriptNeedReload {
		err = navigator.waitTotalLoad()
	}

	return err
}
