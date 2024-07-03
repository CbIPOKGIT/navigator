package navigator

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/proto"
	"github.com/go-rod/stealth"
)

const (
	DEFAULT_BROWSER_NAVIGATION_TIMEOUT = 60
	CHALLANGE_SOLVE_DURATION           = time.Minute * 2 // Challange solve max time duration
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
	if navigator.Page != nil {
		err = navigator.Page.Close()
	}
	navigator.Page = nil
	return err
}

func (navigator *ChromeNavigator) closeBrowser() error {
	var err error = nil
	if navigator.Browser != nil && !navigator.Model.UseSystemChrome {
		err = navigator.Browser.Close()
	}
	navigator.Browser = nil
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
		navigator.Close()
	}

	var i int

	for i = 0; i < navigator.calculateTriesCount(); i++ {
		navigator.LastError = nil

		if i > 0 {
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

		if navigator.JustCreated && navigator.Model.EmptyLoad {
			i = -1
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

		if err := navigator.WaitCreateCrawler(); err != nil {
			navigator.LastError = err
			continue
		}

		if navigator.isValidResponse(navigator.NavigateStatus) {
			break
		}
	}

	if i == navigator.calculateTriesCount() && navigator.PrxGetter != nil {
		navigator.PrxGetter = nil
	}

	return navigator.LastError
}

// Wait navigation response and sign page loaded
func (navigator *ChromeNavigator) WaitTotalLoad(url ...string) error {
	responseCode, err := navigator.waitResponseAndLoad(url...)
	if err != nil {
		navigator.LastError = err
		return err
	}

	navigator.LastError = nil
	navigator.NavigateStatus = responseCode
	return nil
}

// Total rewrite of waitResponseAndLoad
func (navigator *ChromeNavigator) waitResponseAndLoad(url ...string) (int, error) {
	defer handleErrorWithErrorChan(nil)

	responserecived := make(chan int, 1)
	pageloaded := make(chan error, 1)

	go navigator.Page.EachEvent(func(e *proto.NetworkResponseReceived) (stop bool) {
		defer func() {
			if r := recover(); r != nil {
				log.Println("Recovered in waitResponseAndLoad", r)
				responserecived <- 0
			}
		}()

		if e.Type == proto.NetworkResourceTypeDocument {
			responserecived <- e.Response.Status
			return true
		} else {
			return false
		}
	})()

	if navigator.Model.NavigationSelector == "" {
		// Навігація
		go navigator.Page.EachEvent(func(e *proto.PageLoadEventFired) (stop bool) {
			defer handleErrorWithErrorChan(pageloaded)
			pageloaded <- nil
			return false
		})()
	} else {
		go func() {
			defer handleErrorWithErrorChan(pageloaded)
			pageloaded <- navigator.Page.Timeout(time.Minute).WaitElementsMoreThan(navigator.Model.NavigationSelector, 0)
		}()
	}

	if len(url) > 0 {
		time.Sleep(time.Millisecond * 10)
		if err := navigator.Page.Navigate(url[0]); err != nil {
			return 0, err
		}
	}

	var responsecode int
	var isResponsed, isLoaded bool

	checksuccess := make(chan any, 1)

	// Ліміт часу на виконання операції. Або на отримання відповіді від сайту, або на завантаження сторінки
	timeout := time.NewTimer(time.Minute)

	for {
		select {

		// Response recived
		case responsecode = <-responserecived:
			// log.Printf("Response code %d", responsecode)
			isResponsed = true
			go func() { checksuccess <- nil }()
			timeout.Reset(time.Second * 30)

		// Page loaded
		case <-pageloaded:
			// log.Println("Page loaded")
			isLoaded = true
			go func() { checksuccess <- nil }()

		// Checking status
		case <-checksuccess:
			if isLoaded && isResponsed {
				return responsecode, nil
			}

		case <-timeout.C:
			if isResponsed {
				log.Println("Timeout response")
				return 0, errors.New("Timeout response")
			}

			return responsecode, nil
		}
	}
}

func (navigator *ChromeNavigator) WaitCreateCrawler() error {
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

	navigator.createPage()

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

		if navigator.PrxGetter != nil {

			if proxyvalue, err := navigator.PrxGetter.GetProxy(); err == nil {
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

func (navigator *ChromeNavigator) createPage() {
	navigator.Page = stealth.MustPage(navigator.Browser)

	if navigator.Model.InitialCookies != nil {
		navigator.SetCookies()
	}

	navigator.Page.MustEvalOnNewDocument(`window.alert = (message) => console.log(message)`)
}

// Set cookies
func (navigator *ChromeNavigator) SetCookies() {
	cookies := make([]*proto.NetworkCookieParam, 0, len(navigator.Model.InitialCookies))

	for _, cookie := range navigator.Model.InitialCookies {
		c := &proto.NetworkCookieParam{
			Name:     cookie.Name,
			Value:    cookie.Value,
			Domain:   cookie.Domain,
			Path:     cookie.Path,
			Secure:   cookie.Secure,
			HTTPOnly: cookie.HttpOnly,
		}

		if cookie.MaxAge == 0 {
			c.Expires = proto.TimeSinceEpoch(time.Now().Add(time.Minute * 30).Unix())
		} else {
			c.Expires = proto.TimeSinceEpoch(time.Now().Add(time.Second * time.Duration(cookie.MaxAge)).Unix())
		}
		cookies = append(cookies, c)
	}

	navigator.Page.SetCookies(cookies)
}

// Beat the challange. Its something like Cloudflare protection.
//
// Max reloads count - 5
func (navigator *ChromeNavigator) beatChallange() error {
	if navigator.Model.ChallangeSelector == "" {
		return nil
	}

	if has, _ := navigator.hasChallange(); !has {
		return nil
	}

	reloaded := make(chan error, 1)

	var isReloading bool

	waitreload := func() {
		defer handleErrorWithErrorChan(reloaded)
		navigator.WaitTotalLoad()
		isReloading = false
		log.Println("Reloaded")
		reloaded <- nil
	}

	clickticker := time.NewTicker(time.Second * 20)

	for {
		select {
		case <-reloaded:
			if !isReloading {
				isReloading = true
			} else {
				continue
			}

			if has, _ := navigator.hasChallange(); has {
				go waitreload()
				continue
			} else {
				return nil
			}
		case <-clickticker.C:
			if has, err := navigator.hasChallange(); err == nil && !has {
				return nil
			}

			if err := navigator.confirmNotARobot(); err == nil {
				log.Println("Success clicked I'm not robot")
			} else {
				log.Println("Error click I'm not robot: " + err.Error())
			}

			go func() { reloaded <- nil }()
			continue
		case <-time.After(CHALLANGE_SOLVE_DURATION):
			return errors.New("unable pass challange form")
		}
	}

}

// Try click I'm not robot
func (navigator *ChromeNavigator) confirmNotARobot() error {
	var errorUnablePassChallange error = errors.New("Unable pass challange form")

	iframe, err := navigator.Page.Element("iframe")
	if err != nil || iframe == nil {
		return errorUnablePassChallange
	}

	if _, err = iframe.Frame(); err != nil {
		return errorUnablePassChallange
	}

	resp := navigator.Page.MustEval("() => JSON.stringify(document.querySelector('iframe').getBoundingClientRect())")
	coords := make(map[string]float64, 4)

	if err := json.Unmarshal([]byte(resp.Str()), &coords); err != nil {
		return errorUnablePassChallange
	}

	navigator.Page.Activate()
	time.Sleep(time.Millisecond * 500)

	navigator.Page.Mouse.MoveLinear(proto.Point{X: coords["x"] + 20, Y: coords["y"] + 20}, 10)
	time.Sleep(time.Millisecond * 500)

	navigator.Page.Mouse.MustClick(proto.InputMouseButtonLeft)
	return nil
}

func (navigator *ChromeNavigator) hasChallange() (bool, error) {
	has, _, err := navigator.Page.Has(navigator.Model.ChallangeSelector)
	return has, err
}

// Solve captcha if presented
func (navigator *ChromeNavigator) solveCaptcha() error {
	if navigator.Model.CaptchaSelector == "" || navigator.CptchSolver == nil {
		return nil
	}

	has, _, err := navigator.Page.Timeout(time.Second * 5).Has(navigator.Model.CaptchaSelector)
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
