package navigator

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math/rand/v2"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/devices"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/launcher/flags"
	"github.com/go-rod/rod/lib/proto"
	"github.com/go-rod/rod/lib/utils"
	"github.com/go-rod/stealth"
)

const (
	NAVIGATION_STATE_INITIAL uint8 = iota
	NAVIGATION_STATE_NAVIGATED
	NAVIGATION_STATE_DELAYED
	NAVIGATION_STATE_CHALLANGE_SOLVED
	NAVIGATION_STATE_CAPTCHA_SOLVED
	NAVIGATION_STATE_CREATED_CRAWLER
)

const (
	EMULATE_GOOGLE_PIXEL uint8 = iota + 1
)

type ChromeNavigator struct {
	CommonNavigator

	Page *rod.Page

	ClfSolver CloudflareSolver

	context context.Context
	cancel  context.CancelFunc

	isLoading atomic.Bool

	cmd *exec.Cmd // Команда для запуску stealth браузера
}

type ChromeStateStatus struct {
	Step  uint8
	Error error
}

type StateChannel chan *ChromeStateStatus

func (n *ChromeNavigator) Close() error {
	if n.cancel == nil {
		return nil
	}
	n.cancel()

	n.closeClient()

	return nil
}

func (n *ChromeNavigator) GetActualUrl() string {
	if n.Page == nil {
		return ""
	}
	return n.Page.MustInfo().URL
}

func (n *ChromeNavigator) Navigate(url string) error {
	// Індекс спроб навігації
	var tryIndex int = 0

	n.Crawler = nil // Скидаємо crawler

	// Якщо в моделі вказано, що потрібно закривати сторінку при кожній навігації
	var needEmptyLoad bool = n.Page == nil && n.Model.EmptyLoad

	n.createContextIfNeed()

	// Фіксуємо поточний URL
	if err := n.writeAndFormatURL(url); err != nil {
		return err
	}

	// Якщо потрібно закривати щоразу при навігації, то закриваємо попередню сторінку
	if n.Model.ClosePageEverytime {
		n.closeClient()
	}

	navigationStatus := make(StateChannel, 1)

	go func() { navigationStatus <- &ChromeStateStatus{} }()

	for {
		select {

		// Виконаний певний етап
		case state := <-navigationStatus:

			// Початкова ініціалізація
			if state.Step == NAVIGATION_STATE_INITIAL {
				log.Println("Initial navigation")
				tryIndex++
				go n.gotoUrl(url, navigationStatus)
				continue
			}

			// Спробували перейти на URL, але це не вдалося
			if state.Step == NAVIGATION_STATE_NAVIGATED && state.Error != nil {
				// Якщо вже більше не можемо пробувати, тоді виходимо з помилкою
				if tryIndex >= n.calculateTriesCount() {
					n.PrxGetter = nil
					return state.Error
				} else {
					tryIndex++
					n.closeClient()
					go n.gotoUrl(url, navigationStatus)
					continue
				}
			}

			// Спробували перейти на URL і це вдалося
			if state.Step == NAVIGATION_STATE_NAVIGATED {
				// Затримка перед читанням
				go n.delayBeforeRead(navigationStatus)
				continue
			}

			// Перечекали затримку перед читанням
			if state.Step == NAVIGATION_STATE_DELAYED {
				if needEmptyLoad {
					needEmptyLoad = false
					// Якщо потрібно зробити порожнє завантаження, то робимо його
					go n.gotoUrl(url, navigationStatus)
					continue
				} else {
					go n.beatChallange(navigationStatus)
					continue
				}
			}

			// Вирішили челендж
			if state.Step == NAVIGATION_STATE_CHALLANGE_SOLVED {
				if state.Error != nil {
					return state.Error
				}

				go n.solveCaptcha(navigationStatus)
				continue
			}

			// Вирішили капчу
			if state.Step == NAVIGATION_STATE_CAPTCHA_SOLVED && state.Error != nil {
				return state.Error
			}

			if n.isValidResponse(n.NavigateStatus.Load()) {
				return nil
			}

			if tryIndex >= n.calculateTriesCount() {
				n.NoMoreTry = true
				return nil
			} else {
				n.closeClient()
				tryIndex++
				go n.gotoUrl(url, navigationStatus)
			}

		// Якщо контекст скасовано, то виходимо з помилкою
		case <-n.context.Done():
			return errors.New("navigation cancelled")
		}
	}
}

func (n *ChromeNavigator) SetCookies(cookies []*http.Cookie) {
	cooks := make([]*proto.NetworkCookieParam, len(cookies))

	for index, cookie := range cookies {
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

		cooks[index] = c
	}

	n.Page.SetCookies(cooks)
}

func (n *ChromeNavigator) SetCloudflareSolver(solver CloudflareSolver) {
	n.ClfSolver = solver
}

func (n *ChromeNavigator) GetCrawler() *goquery.Document {
	if n.Crawler != nil {
		return n.Crawler
	}

	ticker := time.NewTicker(time.Millisecond * 20)
	var tryIndex int = 0

	for {
		select {
		case <-n.context.Done():
			log.Println("Context cancelled, returning empty crawler")
			n.initEmptyCrawler()
			return n.Crawler

		case <-ticker.C:
			tryIndex++
			if tryIndex > 100 {
				n.initEmptyCrawler()
				return n.Crawler
			}
			html, err := n.Page.HTML()
			if err != nil {
				continue
			}
			if err := n.СreateCrawlerFromHTML(html); err != nil {
				continue
			}

			return n.Crawler
		}

	}
}

// Створюємо контекст для навігатора
func (n *ChromeNavigator) createContextIfNeed() {
	if n.context != nil {
		return
	}

	n.context, n.cancel = context.WithCancel(context.Background())
	log.Println("Context created")
}

// gotoUrl переходить на вказану URL-адресу
func (n *ChromeNavigator) gotoUrl(url string, response ...StateChannel) error {

	state := &ChromeStateStatus{
		Step: NAVIGATION_STATE_NAVIGATED,
	}

	defer writeToChromeStatusChannel(state, response...)

	// defer func() {
	// 	if r := recover(); r != nil {
	// 		state.Error = fmt.Errorf("navigation panic: %v", r)
	// 		log.Println("Recovered from panic during navigation:", state.Error)
	// 	}
	// }()

	if err := n.createClientIfNeed(); err != nil {
		state.Error = err
		return err
	}

	errNavigate := make(chan error, 1)
	navigationContext, cancelWaitLoad := context.WithCancel(n.context)

	go func() { errNavigate <- n.WaitResponseAndLoad(navigationContext) }()

	go func() {
		defer func() {
			if recover() != nil {
				errNavigate <- errors.New("navigation panic")
			}
		}()

		if !n.JustCreated {
			if n.Model.DelayBeforeNavigate > 0 {
				time.Sleep(time.Duration(n.Model.DelayBeforeNavigate) * time.Second)
			} else {
				n.JustCreated = true
			}
		} else {
			n.JustCreated = false
		}

		log.Println("Navigating to URL:", url)

		if err := n.Page.Navigate(url); err != nil {
			log.Println("Error navigation: ", err)
			errNavigate <- err
			cancelWaitLoad()
		}
	}()

	select {
	case <-navigationContext.Done():
		return errors.New("navigation cancelled")

	case err := <-errNavigate:
		state.Error = err
		return err
	}
}

func (n *ChromeNavigator) WaitResponseAndLoad(ctx ...context.Context) error {
	defer func() {
		n.isLoading.Store(false)
	}()
	n.NavigateStatus.Store(0) // Скидаємо статус навігації

	closeCtx := context.Background()
	if len(ctx) > 0 {
		closeCtx = ctx[0]
	}

	// Канал сигналізує, що сторінка завантажена
	loaded := make(chan error)

	navigationTimeout := n.calculateNavigationTimeout()
	navigationTimeoutExpired := time.NewTimer(navigationTimeout)

	if n.Model.NavigationSelector != "" {
		go func() {
			loaded <- n.Page.Timeout(
				navigationTimeout-time.Second,
			).WaitElementsMoreThan(n.Model.NavigationSelector, 0)
		}()
	} else {
		go n.Page.EachEvent(
			func(e *proto.PageDomContentEventFired) bool {
				loaded <- nil
				return true
			},
		)()
	}

	pingNavigateCode := time.NewTicker(time.Second)
	pingNavigateCode.Stop()

	for {
		select {
		case err := <-loaded:
			if err != nil {
				log.Println("Error while waiting for page load:", err)
				return err
			} else {
				log.Println("Page loaded successfully")
			}

			if n.NavigateStatus.Load() != 0 {
				return nil
			} else {
				pingNavigateCode.Reset(time.Second)
			}

		case <-pingNavigateCode.C:
			if n.NavigateStatus.Load() != 0 {
				log.Println("Navigation status received:", n.NavigateStatus.Load())
				return nil
			}

		case <-navigationTimeoutExpired.C:
			if n.NavigateStatus.Load() != 0 {
				log.Println("Navigation timeout expired, but response received")
				return nil
			} else {
				log.Println("Navigation timeout expired without response")
				return errors.New("navigation timeout")
			}

		case <-closeCtx.Done():
			return errors.New("navigation cancelled")
		}
	}
}

func (n *ChromeNavigator) closeClient() error {
	if n.Page == nil {
		return nil
	}

	var err error

	if !n.Model.UseSystemChrome {
		log.Println("Closing browser without system Chrome")
		err = n.Page.Browser().Close()
	} else {
		log.Println("Closing browser with system Chrome")
		err = n.Page.Close()
	}

	n.Page = nil

	if n.cmd != nil {
		log.Printf("Killing stealth browser process %d", n.cmd.Process.Pid)
		n.cmd.Process.Kill()
		n.cmd = nil
	}
	return err
}

// createClientIfNeed створює клієнт, якщо він ще не створений
func (n *ChromeNavigator) createClientIfNeed() error {
	if n.Page != nil {
		n.JustCreated = false
		return nil
	}

	browser, err := n.createBrowser()
	if err != nil {
		log.Println("Error creating browser:", err)
		return err
	}

	n.Page = n.createPage(browser)
	n.SetCookies(n.Model.InitialCookies)
	log.Println("Browser created and page initialized")

	return nil
}

// createBrowser створюємо екземпляр браузера, або підключаємось до існуючого
func (n *ChromeNavigator) createBrowser() (*rod.Browser, error) {
	var u string
	var err error
	var proxy *url.URL

	if n.Model.LaunchStealth {

		u, proxy, err = n.createStealthChromium()

	} else {

		// Пробуємо системний хром якщо потрібен.
		// В випадку помилки будемо запускатись стандартно
		if n.Model.UseSystemChrome {
			if u, err = n.createSystemChrome(); err != nil {
				n.Model.UseSystemChrome = false
			}
		}

		if !n.Model.UseSystemChrome {
			u, proxy, err = n.createChromium()
		}
	}

	if err != nil {
		return nil, err
	}

	browser := rod.New().
		ControlURL(u).
		MustConnect().
		NoDefaultDevice()

	if proxy != nil && proxy.User != nil {
		if username := proxy.User.Username(); username != "" {
			password, _ := proxy.User.Password()
			go browser.HandleAuth(username, password)()
		}
	}

	browser.IgnoreCertErrors(true)
	return browser, nil
}

// createStealthChromium створює екземпляр браузера в режимі максимальної непомітності для антибот систем
func (n *ChromeNavigator) createStealthChromium() (string, *url.URL, error) {
	if n.Model.DelayBeforeRead == 0 {
		n.Model.DelayBeforeRead = 1
	}

	bin, err := launcher.NewBrowser().Get()
	if err != nil {
		return "", nil, err
	}

	remoteDebugPort := strconv.Itoa(rand.IntN(7000) + 3000)

	userDataDir := filepath.Join(launcher.DefaultUserDataDirPrefix, utils.RandString(8))

	args := []string{
		bin,
		"--remote-debugging-port=" + remoteDebugPort,
		fmt.Sprintf("--%s=%s", flags.UserDataDir, userDataDir),
	}

	var proxy *url.URL = nil
	if proxy = n.getNavigatorProxy(); proxy != nil {
		args = append(args,
			fmt.Sprintf(
				"--proxy-server=%s://%s:%s",
				proxy.Scheme,
				proxy.Hostname(),
				proxy.Port(),
			))
	}

	cmd := exec.Command(args[0], args[1:]...)
	if err := cmd.Start(); err != nil {
		log.Printf("Error starting stealth browser command: %v", err)
		cmd.Process.Kill()
		return "", nil, err
	} else {
		log.Printf("Stealth browser command started with PID %d on port %s", cmd.Process.Pid, remoteDebugPort)
	}

	// Чекаємо, поки браузер запуститься
	var u string
	for range 10 {
		u, err = launcher.ResolveURL(remoteDebugPort)
		if err != nil {
			time.Sleep(time.Millisecond * 200)
			continue
		}

		n.cmd = cmd
		return u, proxy, nil
	}

	cmd.Process.Kill()
	return "", nil, err
}

// createSystemChrome створює екземпляр Chrome операційної системи
func (n *ChromeNavigator) createSystemChrome() (string, error) {
	l := launcher.NewUserMode()

	if dataDir := os.Getenv("CHROME_USER_DATA_DIR"); dataDir != "" {
		l.Set(flags.UserDataDir, dataDir)
	}

	return l.Launch()
}

// createChromium створює екземпляр Chromium
func (n *ChromeNavigator) createChromium() (string, *url.URL, error) {
	l := launcher.New()

	l.Set(
		"blink-settings",
		fmt.Sprintf("imagesEnabled=%t", n.Model.ShowImages))

	l = l.Headless(false).
		NoSandbox(true).
		Set("disable-web-security", "1")

	proxy := n.getNavigatorProxy()
	if proxy != nil {
		l.Proxy(fmt.Sprintf(
			"%s://%s:%s",
			proxy.Scheme,
			proxy.Hostname(),
			proxy.Port(),
		))
	}

	u, err := l.Launch()
	return u, proxy, err
}

func (n *ChromeNavigator) getNavigatorProxy() *url.URL {
	if n.PrxGetter != nil {
		if proxyStr, err := n.PrxGetter.GetProxy(); err == nil {
			if parsedProxy, err := url.Parse(proxyStr); err == nil {
				return parsedProxy
			}
		}
	}
	return nil
}

// createPage створює нову сторінку в браузері
func (n *ChromeNavigator) createPage(browser *rod.Browser) *rod.Page {
	page := stealth.MustPage(browser)

	if n.Model.EmulateDevice != 0 {
		switch n.Model.EmulateDevice {
		case EMULATE_GOOGLE_PIXEL:
			page = page.MustEmulate(devices.Pixel2XL)
		}

	}

	if n.Model.UserAgent != "" {
		page.SetUserAgent(&proto.NetworkSetUserAgentOverride{
			UserAgent: n.Model.UserAgent,
		})
	}

	page.MustEvalOnNewDocument(`window.alert = (message) => console.log(message)`)
	page.MustEvalOnNewDocument(`Object.defineProperty(navigator, 'webdriver', {
		get: () => false,
		configurable: true,
	})`)

	if n.ClfSolver != nil {
		page.MustEvalOnNewDocument(`
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

	go page.EachEvent(
		func(e *proto.NetworkResponseReceived) {
			if e.Type == proto.NetworkResourceTypeDocument {
				log.Println("Network response received: ", e.Response.Status)
				n.NavigateStatus.Store(int32(e.Response.Status))
			}
		},
		func(e *proto.PageFrameStartedLoading) {
			n.isLoading.Store(true)
		},
		func(e *proto.PageDomContentEventFired) {
			n.isLoading.Store(false)
		},
	)()

	return page
}

// delayBeforeRead затримує читання сторінки на вказаний час
func (n *ChromeNavigator) delayBeforeRead(response ...StateChannel) {
	state := &ChromeStateStatus{
		Step: NAVIGATION_STATE_DELAYED,
	}

	defer writeToChromeStatusChannel(state, response...)

	if n.Model.DelayBeforeRead == 0 {
		log.Println("No delay before read, skipping")
		return
	}

	select {
	case <-n.context.Done():
		state.Error = errors.New("navigation cancelled")
		return

	case <-time.After(time.Duration(n.Model.DelayBeforeRead) * time.Second):
		log.Printf("Delay before read completed after %d seconds", n.Model.DelayBeforeRead)
		return
	}
}

// Solve captcha if presented
func (n *ChromeNavigator) solveCaptcha(response ...StateChannel) error {
	state := &ChromeStateStatus{
		Step: NAVIGATION_STATE_CAPTCHA_SOLVED,
	}

	defer writeToChromeStatusChannel(state, response...)

	if n.Model.CaptchaSelector == "" || n.CptchSolver == nil {
		log.Println("No captcha selector or solver provided, skipping captcha solving")
		return nil
	}

	has, _, err := n.Page.Timeout(time.Second * 5).Has(n.Model.CaptchaSelector)
	if err != nil {
		state.Error = err
		return err
	}

	if has == n.Model.CaptchaSelectorInverted {
		return nil
	}

	n.CptchSolver.SetNavigator(n)
	solved, err := n.CptchSolver.Solve()
	if err != nil {
		state.Error = err
		return err
	}
	if !solved {
		state.Error = errors.New("cannot solve captcha")
		return state.Error
	}

	return nil
}

// Прокиваємо статуси кожного кроку в канали відповіді
func writeToChromeStatusChannel(state *ChromeStateStatus, response ...StateChannel) {
	if len(response) > 0 {
		for _, ch := range response {
			ch <- state
		}
	}
}
