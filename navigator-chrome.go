package navigator

import (
	"errors"
	"fmt"
	"log"
	"strings"
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
	if err := navigator.closePage(); err != nil {
		return err
	}
	if err := navigator.closeBrowser(); err != nil {
		return err
	}
	return nil
}

func (navigator *ChromeNavigator) closePage() error {
	var err error = nil
	if navigator.Page != nil && navigator.Model.UseSystemChrome {
		err = navigator.Page.Close()
		navigator.Page = nil
	}
	return err
}

func (navigator *ChromeNavigator) closeBrowser() error {
	var err error = nil
	if navigator.Browser != nil && !navigator.Model.UseSystemChrome {
		err = navigator.Browser.Close()
		navigator.Browser = nil
	}
	return err
}

// Interface implementation
func (navigator *ChromeNavigator) Navigate(url string) error {
	if err := navigator.writeAndFormatURL(url); err != nil {
		return err
	}

	navigator.initEmptyCrawler()

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

func (navigator *ChromeNavigator) GetActualUrl() string {
	if navigator.Page == nil {
		return navigator.Url
	}
	info, err := navigator.Page.Info()
	if err != nil {
		return navigator.Url
	}
	return info.URL
}

func (navigator *ChromeNavigator) navigateUrl() error {
	if navigator.Model.ClosePageEverytime && navigator.Page != nil {
		navigator.Page.Close()
	}

	var reloading bool = false

	for i := 0; i < navigator.calculateTriesCount(); i++ {
		if i > 0 {
			navigator.Close()
		} else {
			if !navigator.JustCreated && navigator.Model.DelayBeforeNavigate > 0 {
				time.Sleep(time.Second * time.Duration(navigator.Model.DelayBeforeNavigate))
			}
		}
		reloading = false

		if err := navigator.createClientIfNeed(); err != nil {
			navigator.LastError = err
			break
		}

		if err := navigator.WaitTotalLoad(navigator.Url); err != nil {
			navigator.LastError = err
			continue
		}

		if navigator.JustCreated {
			if err := navigator.executePreScript(); err != nil {
				navigator.LastError = err
				continue
			}
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

		if err := navigator.waitCreateCrawler(); err != nil {
			navigator.LastError = err
			continue
		}

		if navigator.isValidResponse(navigator.NavigateStatus) {
			break
		}
	}

	return navigator.LastError
}

// Wait navigation response and sign page loaded
func (navigator *ChromeNavigator) WaitTotalLoad(url ...string) error {
	response, err := navigator.waitResponseAndLoad(url...)
	if err != nil {
		return err
	}

	navigator.LastError = nil
	navigator.NavigateStatus = response.Response.Status
	return nil
}

func (navigator *ChromeNavigator) waitResponseAndLoad(url ...string) (*proto.NetworkResponseReceived, error) {
	// Статус відповіді на запит
	response := proto.NetworkResponseReceived{}

	// Функція, що спрацює лише коли отримаємо відповідь на запит
	waitResponse := navigator.Page.WaitEvent(&response)

	// Таймаут очікування відповіді
	timeoutResponse := time.NewTimer(time.Duration(navigator.getPageLoadTimeout()) * time.Second)

	// Канал сигналізації, що відповідь від сервера отримана
	responseRecived := make(chan any, 1)

	// Таймаут завантаження сторінки.
	// Запускається лише після того як отримана відповідь від сервера
	timeoutload := time.NewTimer(time.Duration(navigator.getPageLoadTimeout()) * time.Second)
	timeoutload.Stop()

	// Канал сигналізації, що сторінка завантажена
	waitLoad := make(chan error)

	go func() {
		waitResponse()
		// log.Println("Response recived")
		responseRecived <- nil
		timeoutload.Reset(time.Duration(navigator.getPageLoadTimeout()) * time.Second)
	}()

	if navigator.Model.NavigationSelector == "" {
		waitEventLoad := navigator.Page.WaitNavigation(navigator.getPageLoadEvent())

		go func() {
			waitEventLoad()
			// log.Println("Loaded")

			// Якщо так сталось, що подія завантаження сторінки сталася раніше
			// ніж оброблений статус навігації
			// тоді чекаємо доки обробиться статус навігації
			<-responseRecived
			// log.Println("Page loaded")

			// Сигналізуємо, що сторінка завантажилась
			waitLoad <- nil
		}()
	} else {
		go func() {
			waitLoad <- navigator.Page.WaitElementsMoreThan(navigator.Model.NavigationSelector, 1)
		}()
	}

	if len(url) > 0 {
		time.Sleep(time.Millisecond * 10)
		if err := navigator.Page.Navigate(url[0]); err != nil {
			return nil, err
		}
	}

	select {
	case err := <-waitLoad:
		// log.Println("Navigated")
		return &response, err
	case <-timeoutResponse.C:
		log.Println("Timeout response")
		return nil, errors.New("Timeout response")
	case <-timeoutload.C:
		log.Println("Timeout navigation")
		return nil, errors.New("Timeout navigation")
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

// Get load event name
func (navigator *ChromeNavigator) getPageLoadEvent() proto.PageLifecycleEventName {
	switch navigator.Model.NavigationWaitfor {
	case 1:
		return proto.PageLifecycleEventNameNetworkAlmostIdle
	case 2:
		return proto.PageLifecycleEventNameNetworkIdle
	case 3:
		return proto.PageLifecycleEventNameLoad
	default:
		return proto.PageLifecycleEventNameDOMContentLoaded
	}
}

func (navigator *ChromeNavigator) waitCreateCrawler() error {
	for i := 0; i < 2; i++ {
		html, err := navigator.Page.HTML()
		if err != nil {
			return err
		}

		if err := navigator.createCrawlerFromHTML(html); err != nil {
			return errors.New(fmt.Sprintf("Error create crawler from HTML: %s", err.Error()))
		}

		if strings.TrimSpace(navigator.Crawler.Find("body").Text()) == "" {
			log.Println("Waiting DOM document loaded")
			time.Sleep(time.Millisecond * 500)
			continue
		}
	}

	return nil
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

			if err := navigator.WaitTotalLoad(); err != nil {
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

	has, _, err := navigator.Page.Has(navigator.Model.CaptchaSelector)
	if err != nil {
		return err
	}

	if has == navigator.Model.CaptchaSelectorInverted {
		return nil
	}

	navigator.CptchSolver.SetNavigator(navigator)
	solved, err := navigator.CptchSolver.Solve()
	if err != nil {
		return err
	}
	if !solved {
		return errors.New("Cannot solve captcha")
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
		err = navigator.WaitTotalLoad()
	}

	return err
}
