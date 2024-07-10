package navigator

import (
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/devices"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/proto"
	"github.com/go-rod/stealth"
)

const (
	DEFAULT_BROWSER_NAVIGATION_TIMEOUT = 60
	CHALLANGE_SOLVE_DURATION           = time.Minute * 2 // Challange solve max time duration
)

const (
	EMULATE_GOOGLE_PIXEL uint8 = iota + 1
)

type ChromeNavigator struct {
	CommonNavigator

	Browser *rod.Browser
	Page    *rod.Page

	ClfSolver CloudflareSolver
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

func (navigator *ChromeNavigator) SetCloudflareSolver(solver CloudflareSolver) {
	navigator.ClfSolver = solver
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
	err := navigator.waitResponseAndLoad(url...)
	if err != nil {
		navigator.LastError = err
		return err
	}

	navigator.LastError = nil
	return nil
}

// Total rewrite of waitResponseAndLoad
func (navigator *ChromeNavigator) waitResponseAndLoad(url ...string) error {
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
			return err
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
			navigator.NavigateStatus = responsecode
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
				return nil
			}

		case <-timeout.C:
			if isResponsed {
				log.Println("Timeout response")
				return errors.New("timeout response")
			}

			return nil
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
			return fmt.Errorf("error create crawler from HTML: %s", err.Error())
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

	if navigator.Model.EmulateDevice != 0 {
		switch navigator.Model.EmulateDevice {
		case EMULATE_GOOGLE_PIXEL:
			navigator.Page = navigator.Page.MustEmulate(devices.Pixel2XL)
		}

	}

	if navigator.Model.InitialCookies != nil {
		navigator.SetCookies()
	}

	if navigator.Model.UserAgent != "" {
		navigator.Page.SetUserAgent(&proto.NetworkSetUserAgentOverride{
			UserAgent: navigator.Model.UserAgent,
		})
	}

	navigator.Page.MustEvalOnNewDocument(`window.alert = (message) => console.log(message)`)

	if navigator.ClfSolver != nil {
		navigator.Page.MustEvalOnNewDocument(`
			const i = setInterval(()=>{
			if (window.turnstile) {
				console.log('Turnstile found')
				clearInterval(i)
				window.turnstile.render = (a,b) => {
					let p = {
						type: "TurnstileTaskProxyless",
						websiteKey: b.sitekey,
						websiteURL: window.location.href,
						data: b.cData,
						pagedata: b.chlPageData,
						action: b.action,
						userAgent: navigator.userAgent
					}
					window.cloudflareData = JSON.stringify(p)
					console.log(cloudflareData || 'No data')
					window.tsCallback = b.callback
					return 'foo'
				}
			}
			},10)  
		`)
	}
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
		return errors.New("cannot solve captcha")
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
