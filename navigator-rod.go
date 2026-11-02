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
	"sync"
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

type ExecutorStep uint8

const (
	EXECUTOR_NONE ExecutorStep = iota
	EXECUTOR_CLOSE_PAGE
	EXECUTOR_DELAY_BEFORE_NAVIGATE
	EXECUTOR_GOTO_URL
	EXECUTOR_DELAY_BEFORE_READ
	EXECUTOR_EMPTY_LOAD
	EXECUTOR_BEAT_CHALLENGE
	EXECUTOR_SOLVE_CAPTCHA
	EXECUTOR_VALIDATE_RESPONSE
)

const (
	EMULATE_GOOGLE_PIXEL uint8 = iota + 1
)

// ExecutorFunc — типізована функція кроку навігації.
type ExecutorFunc func(ctx context.Context) error

// StepExecutor прив'язує константу кроку до його типізованої функції.
type StepExecutor struct {
	Step ExecutorStep
	Exec ExecutorFunc
}

type ChromeNavigator struct {
	CommonNavigator

	mu sync.RWMutex

	Page *rod.Page

	ClfSolver CloudflareSolver

	context context.Context
	cancel  context.CancelFunc

	isLoading atomic.Bool

	cmd *exec.Cmd // Команда для запуску stealth браузера

	executors     []StepExecutor
	nextStep      ExecutorStep
	tryIndex      int
	needEmptyLoad atomic.Bool
}

func (n *ChromeNavigator) SetModel(model *Model) {
	n.mu.Lock()
	defer n.mu.Unlock()

	if model == nil {
		model = &Model{}
	}
	n.CommonNavigator.SetModel(model)
	n.buildExecutorsLocked()
}

func (n *ChromeNavigator) SetCaptchaSolver(solver CaptchaSolver) {
	n.mu.Lock()
	defer n.mu.Unlock()

	n.CommonNavigator.SetCaptchaSolver(solver)
	n.buildExecutorsLocked()
}

func (n *ChromeNavigator) SetCloudflareSolver(solver CloudflareSolver) {
	n.mu.Lock()
	defer n.mu.Unlock()

	n.ClfSolver = solver
	n.buildExecutorsLocked()
}

func (n *ChromeNavigator) getPage() *rod.Page {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.Page
}

func (n *ChromeNavigator) setPage(page *rod.Page) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.Page = page
}

func (n *ChromeNavigator) getContext() context.Context {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.context
}

// setExecutorStep встановлює крок, на який має перескочити вказівник після поточного виконання.
func (n *ChromeNavigator) setExecutorStep(step ...ExecutorStep) {
	if len(step) > 0 {
		n.nextStep = step[0]
	} else {
		n.nextStep = EXECUTOR_NONE
	}
}

// findExecutorIndex знаходить індекс виконавця у масиві за його константою.
func (n *ChromeNavigator) findExecutorIndex(executors []StepExecutor, target ExecutorStep) int {
	for idx, ex := range executors {
		if ex.Step == target {
			return idx
		}
	}
	return -1
}

// buildExecutorsLocked формує масив типізованих функцій-виконавців відповідно до поточної моделі.
func (n *ChromeNavigator) buildExecutorsLocked() {
	if n.Model == nil {
		n.Model = &Model{}
	}

	if n.Model.LaunchStealth && n.Model.DelayBeforeRead == 0 {
		n.Model.DelayBeforeRead = 1
	}

	executors := make([]StepExecutor, 0, 8)

	if n.Model.ClosePageEverytime {
		executors = append(executors, StepExecutor{
			Step: EXECUTOR_CLOSE_PAGE,
			Exec: n.stepClosePage,
		})
	}

	if n.Model.DelayBeforeNavigate > 0 {
		executors = append(executors, StepExecutor{
			Step: EXECUTOR_DELAY_BEFORE_NAVIGATE,
			Exec: n.stepDelayBeforeNavigate,
		})
	}

	executors = append(executors, StepExecutor{
		Step: EXECUTOR_GOTO_URL,
		Exec: n.stepGotoUrl,
	})

	if n.Model.DelayBeforeRead > 0 {
		executors = append(executors, StepExecutor{
			Step: EXECUTOR_DELAY_BEFORE_READ,
			Exec: n.stepDelayBeforeRead,
		})
	}

	if n.Model.EmptyLoad {
		executors = append(executors, StepExecutor{
			Step: EXECUTOR_EMPTY_LOAD,
			Exec: n.stepEmptyLoad,
		})
	}

	if n.Model.ChallangeSelector != "" || n.ClfSolver != nil {
		executors = append(executors, StepExecutor{
			Step: EXECUTOR_BEAT_CHALLENGE,
			Exec: n.stepBeatChallenge,
		})
	}

	if n.Model.CaptchaSelector != "" && n.CptchSolver != nil {
		executors = append(executors, StepExecutor{
			Step: EXECUTOR_SOLVE_CAPTCHA,
			Exec: n.stepSolveCaptcha,
		})
	}

	executors = append(executors, StepExecutor{
		Step: EXECUTOR_VALIDATE_RESPONSE,
		Exec: n.stepValidateResponse,
	})

	n.executors = executors
}

func (n *ChromeNavigator) Close() error {
	n.mu.Lock()
	cancel := n.cancel
	n.cancel = nil
	n.context = nil
	n.mu.Unlock()

	if cancel != nil {
		cancel()
	}

	return n.closeClient()
}

func (n *ChromeNavigator) GetActualUrl() string {
	page := n.getPage()
	if page == nil {
		return ""
	}

	var actualURL string
	func() {
		defer func() {
			recover()
		}()
		if info, err := page.Info(); err == nil && info != nil {
			actualURL = info.URL
		}
	}()

	return actualURL
}

func (n *ChromeNavigator) Navigate(url string) error {
	n.mu.Lock()
	n.Crawler = nil // Скидаємо crawler
	n.LastError = nil
	n.tryIndex = 1
	n.nextStep = EXECUTOR_NONE

	if n.Model == nil {
		n.Model = &Model{}
	}

	// Якщо в моделі вказано, що потрібно зробити порожнє завантаження при першому відкритті
	n.needEmptyLoad.Store(n.Page == nil && n.Model.EmptyLoad)

	n.createContextIfNeedLocked()
	ctx := n.context

	if len(n.executors) == 0 {
		n.buildExecutorsLocked()
	}
	executors := append([]StepExecutor(nil), n.executors...)
	n.mu.Unlock()

	// Фіксуємо поточний URL
	if err := n.writeAndFormatURL(url); err != nil {
		n.LastError = err
		return err
	}

	for i := 0; i < len(executors); {
		if err := ctx.Err(); err != nil {
			errCancel := errors.New("navigation cancelled")
			n.LastError = errCancel
			return errCancel
		}

		n.setExecutorStep(EXECUTOR_NONE)

		if err := executors[i].Exec(ctx); err != nil {
			n.LastError = err
			return err
		}

		if n.nextStep != EXECUTOR_NONE {
			if targetIdx := n.findExecutorIndex(executors, n.nextStep); targetIdx >= 0 {
				i = targetIdx
				continue
			}
		}

		i++
	}

	return nil
}

// stepClosePage закриває попередню сторінку/клієнт перед навігацією, якщо вказано ClosePageEverytime.
func (n *ChromeNavigator) stepClosePage(ctx context.Context) error {
	_ = n.closeClient()
	return nil
}

// stepDelayBeforeNavigate виконує затримку перед переходом на наступний URL.
func (n *ChromeNavigator) stepDelayBeforeNavigate(ctx context.Context) error {
	if n.Model == nil || n.Model.DelayBeforeNavigate <= 0 {
		return nil
	}

	if n.getPage() == nil || n.JustCreated {
		n.JustCreated = false
		return nil
	}

	delayDuration := calculateDelay(n.Model.DelayBeforeNavigate)
	timer := time.NewTimer(delayDuration)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return errors.New("navigation cancelled")
	case <-timer.C:
		return nil
	}
}

// stepGotoUrl виконує перехід на цільовий URL з підтримкою повторних спроб.
func (n *ChromeNavigator) stepGotoUrl(ctx context.Context) error {
	log.Println("Initial navigation")
	err := n.gotoUrl(ctx, n.GetUrl())
	if err != nil {
		// Якщо вже більше не можемо пробувати, тоді виходимо з помилкою
		if n.tryIndex >= n.calculateTriesCount() {
			n.PrxGetter = nil
			return err
		}

		n.tryIndex++
		_ = n.closeClient()
		n.setExecutorStep(EXECUTOR_GOTO_URL)
		return nil
	}

	return nil
}

// stepDelayBeforeRead затримує читання сторінки на вказаний час.
func (n *ChromeNavigator) stepDelayBeforeRead(ctx context.Context) error {
	return n.delayBeforeRead(ctx)
}

// stepEmptyLoad виконує повторне завантаження сторінки при першому старті, якщо увімкнено EmptyLoad.
func (n *ChromeNavigator) stepEmptyLoad(ctx context.Context) error {
	if n.needEmptyLoad.CompareAndSwap(true, false) {
		n.setExecutorStep(EXECUTOR_GOTO_URL)
	}
	return nil
}

// stepBeatChallenge проходить захист (Cloudflare або простий челендж).
func (n *ChromeNavigator) stepBeatChallenge(ctx context.Context) error {
	return n.beatChallange(ctx)
}

// stepSolveCaptcha розв'язує капчу, якщо вона присутня на сторінці.
func (n *ChromeNavigator) stepSolveCaptcha(ctx context.Context) error {
	return n.solveCaptcha(ctx)
}

// stepValidateResponse перевіряє статус-код відповіді та ініціює повторну спробу за потреби.
func (n *ChromeNavigator) stepValidateResponse(ctx context.Context) error {
	if n.isValidResponse(n.NavigateStatus.Load()) {
		return nil
	}

	if n.tryIndex >= n.calculateTriesCount() {
		n.NoMoreTry = true
		return nil
	}

	_ = n.closeClient()
	n.tryIndex++
	n.setExecutorStep(EXECUTOR_GOTO_URL)
	return nil
}

func (n *ChromeNavigator) SetCookies(cookies []*http.Cookie) {
	page := n.getPage()
	if page == nil || len(cookies) == 0 {
		return
	}

	cooks := make([]*proto.NetworkCookieParam, 0, len(cookies))

	for _, cookie := range cookies {
		if cookie == nil {
			continue
		}
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

		cooks = append(cooks, c)
	}

	if len(cooks) == 0 {
		return
	}

	func() {
		defer func() {
			recover()
		}()
		_ = page.SetCookies(cooks)
	}()
}

func (n *ChromeNavigator) GetCrawler() *goquery.Document {
	n.mu.RLock()
	if n.Crawler != nil {
		crawler := n.Crawler
		n.mu.RUnlock()
		return crawler
	}
	ctx := n.context
	n.mu.RUnlock()

	if n.getPage() == nil {
		n.mu.Lock()
		n.initEmptyCrawler()
		crawler := n.Crawler
		n.mu.Unlock()
		return crawler
	}

	var doneCh <-chan struct{}
	if ctx != nil {
		doneCh = ctx.Done()
	}

	ticker := time.NewTicker(time.Millisecond * 20)
	defer ticker.Stop()

	var tryIndex int = 0

	for {
		select {
		case <-doneCh:
			log.Println("Context cancelled, returning empty crawler")
			n.mu.Lock()
			n.initEmptyCrawler()
			crawler := n.Crawler
			n.mu.Unlock()
			return crawler

		case <-ticker.C:
			tryIndex++
			page := n.getPage()
			if tryIndex > 100 || page == nil {
				n.mu.Lock()
				n.initEmptyCrawler()
				crawler := n.Crawler
				n.mu.Unlock()
				return crawler
			}

			var html string
			var err error
			func() {
				defer func() {
					if r := recover(); r != nil {
						err = errors.New("panic getting html")
					}
				}()
				html, err = page.HTML()
			}()

			if err != nil {
				continue
			}
			n.mu.Lock()
			err = n.СreateCrawlerFromHTML(html)
			crawler := n.Crawler
			n.mu.Unlock()
			if err != nil {
				continue
			}

			return crawler
		}
	}
}

// Створюємо контекст для навігатора
func (n *ChromeNavigator) createContextIfNeed() {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.createContextIfNeedLocked()
}

func (n *ChromeNavigator) createContextIfNeedLocked() {
	if n.context != nil && n.context.Err() == nil {
		return
	}

	n.context, n.cancel = context.WithCancel(context.Background())
	log.Println("Context created")
}

// gotoUrl переходить на вказану URL-адресу
func (n *ChromeNavigator) gotoUrl(ctx context.Context, targetUrl string) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = errors.New("panic")
		}
	}()

	if err := n.createClientIfNeed(); err != nil {
		return err
	}

	page := n.getPage()
	if page == nil {
		return errors.New("page is closed")
	}

	if ctx == nil {
		ctx = n.getContext()
		if ctx == nil {
			ctx = context.Background()
		}
	}

	errNavigate := make(chan error, 2)
	navigationContext, cancelWaitLoad := context.WithCancel(ctx)
	defer cancelWaitLoad()

	go func() {
		errNavigate <- n.WaitResponseAndLoad(navigationContext)
	}()

	go func(p *rod.Page) {
		defer func() {
			if recover() != nil {
				errNavigate <- errors.New("navigation panic")
			}
		}()

		log.Println("Navigating to URL:", targetUrl)

		if err := p.Context(navigationContext).Navigate(targetUrl); err != nil {
			log.Println("Error navigation: ", err)
			errNavigate <- err
			cancelWaitLoad()
		}
	}(page)

	select {
	case <-ctx.Done():
		return errors.New("navigation cancelled")

	case err := <-errNavigate:
		n.logRequest(n.NavigateStatus.Load(), err)
		return err
	}
}

func (n *ChromeNavigator) WaitResponseAndLoad(ctx ...context.Context) (err error) {
	defer func() {
		n.isLoading.Store(false)
		if r := recover(); r != nil {
			err = errors.New("panic")
		}
	}()
	n.NavigateStatus.Store(0) // Скидаємо статус навігації

	page := n.getPage()
	if page == nil {
		return errors.New("page is closed")
	}

	closeCtx := n.getContext()
	if closeCtx == nil {
		closeCtx = context.Background()
	}
	if len(ctx) > 0 && ctx[0] != nil {
		closeCtx = ctx[0]
	}

	waitCtx, cancelWait := context.WithCancel(closeCtx)
	defer cancelWait()

	// Канал сигналізує, що сторінка завантажена
	loaded := make(chan error, 1)

	navigationTimeout := n.calculateNavigationTimeout()
	navigationTimeoutExpired := time.NewTimer(navigationTimeout)
	defer navigationTimeoutExpired.Stop()

	if n.Model != nil && n.Model.NavigationSelector != "" {
		selectorTimeout := navigationTimeout - time.Second
		if selectorTimeout <= 0 {
			selectorTimeout = time.Second
		}
		go func(p *rod.Page, selector string) {
			defer func() {
				if recover() != nil {
					select {
					case loaded <- errors.New("panic waiting selector"):
					default:
					}
				}
			}()
			waitErr := p.Context(waitCtx).Timeout(selectorTimeout).WaitElementsMoreThan(selector, 0)
			select {
			case loaded <- waitErr:
			default:
			}
		}(page, n.Model.NavigationSelector)
	} else {
		go func(p *rod.Page) {
			defer func() {
				if recover() != nil {
					select {
					case loaded <- errors.New("panic waiting dom content"):
					default:
					}
				}
			}()
			p.Context(waitCtx).EachEvent(
				func(e *proto.PageDomContentEventFired) bool {
					select {
					case loaded <- nil:
					default:
					}
					return true
				},
			)()
		}(page)
	}

	pingNavigateCode := time.NewTicker(time.Second)
	pingNavigateCode.Stop()
	defer pingNavigateCode.Stop()

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

func (n *ChromeNavigator) closeClient() (err error) {
	n.mu.Lock()
	page := n.Page
	n.Page = nil
	cmd := n.cmd
	n.cmd = nil
	useSystemChrome := n.Model != nil && n.Model.UseSystemChrome
	n.mu.Unlock()

	defer func() {
		if r := recover(); r != nil {
			err = errors.New("panic closing client")
		}
	}()

	if page != nil {
		if !useSystemChrome {
			log.Println("Closing browser without system Chrome")
			if browser := page.Browser(); browser != nil {
				err = browser.Close()
			}
		} else {
			log.Println("Closing browser with system Chrome")
			err = page.Close()
		}
	}

	if cmd != nil && cmd.Process != nil {
		log.Printf("Killing stealth browser process %d", cmd.Process.Pid)
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}

	return err
}

// createClientIfNeed створює клієнт, якщо він ще не створений
func (n *ChromeNavigator) createClientIfNeed() error {
	if n.getPage() != nil {
		n.JustCreated = false
		return nil
	}

	browser, err := n.createBrowser()
	if err != nil {
		log.Println("Error creating browser:", err)
		return err
	}

	page, err := n.createPage(browser)
	if err != nil {
		log.Println("Error creating page:", err)
		func() {
			defer func() { recover() }()
			if n.Model != nil && !n.Model.UseSystemChrome {
				_ = browser.Close()
			}
		}()
		return err
	}

	n.setPage(page)
	n.JustCreated = true

	if n.Model != nil && len(n.Model.InitialCookies) > 0 {
		n.SetCookies(n.Model.InitialCookies)
	}
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
		n.mu.RLock()
		useSystemChrome := n.Model.UseSystemChrome
		n.mu.RUnlock()

		if useSystemChrome {
			if u, err = n.createSystemChrome(); err != nil {
				n.mu.Lock()
				n.Model.UseSystemChrome = false
				n.Model.Visible = true
				n.mu.Unlock()
				useSystemChrome = false
			}
		}

		if !useSystemChrome {
			u, proxy, err = n.createChromium()
		}
	}

	if err != nil {
		return nil, err
	}

	browser := rod.New().
		ControlURL(u).
		NoDefaultDevice()

	if err := browser.Connect(); err != nil {
		return nil, err
	}

	n.CurrentProxy = proxy
	if proxy != nil && proxy.User != nil {
		if username := proxy.User.Username(); username != "" {
			password, _ := proxy.User.Password()
			go func(b *rod.Browser, user, pass string) {
				defer func() { recover() }()
				_ = b.HandleAuth(user, pass)()
			}(browser, username, password)
		}
	}

	_ = browser.IgnoreCertErrors(true)
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

		n.mu.Lock()
		n.cmd = cmd
		n.mu.Unlock()
		return u, proxy, nil
	}

	if cmd.Process != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}
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

	l = l.Headless(!n.Model.Visible).
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
func (n *ChromeNavigator) createPage(browser *rod.Browser) (page *rod.Page, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("create page panic: %v", r)
		}
	}()

	if browser == nil {
		return nil, errors.New("browser is nil")
	}

	page, err = stealth.Page(browser)
	if err != nil {
		return nil, err
	}

	if n.Model.EmulateDevice != 0 {
		switch n.Model.EmulateDevice {
		case EMULATE_GOOGLE_PIXEL:
			if err := page.Emulate(devices.Pixel2XL); err != nil {
				return nil, err
			}
		}
	}

	if n.Model.UserAgent != "" {
		_ = page.SetUserAgent(&proto.NetworkSetUserAgentOverride{
			UserAgent: n.Model.UserAgent,
		})
	}

	if _, err := page.EvalOnNewDocument(`window.alert = (message) => console.log(message)`); err != nil {
		return nil, err
	}
	if _, err := page.EvalOnNewDocument(`Object.defineProperty(navigator, 'webdriver', {
		get: () => false,
		configurable: true,
	})`); err != nil {
		return nil, err
	}

	if n.ClfSolver != nil {
		if _, err := page.EvalOnNewDocument(`
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
		`); err != nil {
			return nil, err
		}
	}

	go func(p *rod.Page) {
		defer func() {
			recover()
		}()
		p.EachEvent(
			func(e *proto.NetworkResponseReceived) {
				if e.Type == proto.NetworkResourceTypeDocument && e.Response != nil {
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
	}(page)

	return page, nil
}

// delayBeforeRead затримує читання сторінки на вказаний час
func (n *ChromeNavigator) delayBeforeRead(ctx context.Context) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = errors.New("panic")
		}
	}()

	if n.Model == nil || n.Model.DelayBeforeRead == 0 {
		log.Println("No delay before read, skipping")
		return nil
	}

	var delayDuration time.Duration = calculateDelay(n.Model.DelayBeforeRead)
	timer := time.NewTimer(delayDuration)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return errors.New("navigation cancelled")

	case <-timer.C:
		log.Printf("Delay before read completed after %d seconds", n.Model.DelayBeforeRead)
		return nil
	}
}

// Solve captcha if presented
func (n *ChromeNavigator) solveCaptcha(ctx context.Context) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = errors.New("panic")
		}
	}()

	if n.Model == nil || n.Model.CaptchaSelector == "" || n.CptchSolver == nil {
		log.Println("No captcha selector or solver provided, skipping captcha solving")
		return nil
	}

	page := n.getPage()
	if page == nil {
		return errors.New("page is closed")
	}

	has, _, err := page.Context(ctx).Timeout(time.Second * 5).Has(n.Model.CaptchaSelector)
	if err != nil {
		return err
	}

	if has == n.Model.CaptchaSelectorInverted {
		return nil
	}

	n.CptchSolver.SetNavigator(n)
	solved, err := n.CptchSolver.Solve()
	if err != nil {
		return err
	}
	if !solved {
		return errors.New("cannot solve captcha")
	}

	return nil
}

func calculateDelay(duration int) time.Duration {
	if duration < 20 {
		return time.Duration(duration) * time.Second
	} else {
		return time.Duration(duration) * time.Millisecond
	}
}
