package navigator

import (
	"errors"
	"log"
	"os"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/proto"
)

const (
	TIMEOUT_ERROR = 30
)

type ChromeNavigator struct {
	CommonNavigator

	// Підключений браузер
	browser *rod.Browser

	// Активна сторінка
	page *rod.Page
}

func (chn *ChromeNavigator) Navigate(url string) error {
	if err := chn.parseUrl(url); err != nil {
		return err
	}

	return chn.gotoUrl(url)
}

func (chn *ChromeNavigator) Close() error {
	if chn.hasClient() {
		err := chn.browser.Close()
		chn.page = nil
		chn.browser = nil
		return err
	} else {
		return nil
	}
}

func (chn *ChromeNavigator) GetPage() *rod.Page {
	return chn.page
}

func (chn *ChromeNavigator) RefreshCrawler() error {
	if chn.page != nil {
		if html, err := chn.page.HTML(); err == nil {
			return chn.createCrawler(html)
		} else {
			return err
		}
	}
	return nil
}

func (chn *ChromeNavigator) SetCookie(key, value string, expired int) {
	// Need do
}

// ------------------------------------------------------------

func (chn *ChromeNavigator) checkIfClientExist() error {
	if !chn.hasClient() {
		return chn.createClient()
	}
	return nil
}

// Перевіряємо чи запущений браузер
func (chn *ChromeNavigator) hasClient() bool {
	return chn.browser != nil || chn.page != nil
}

// Запускаємо браузер та створюємо сторінку
func (chn *ChromeNavigator) createClient() error {
	l := launcher.New().Headless(!chn.model.visible)

	// Вимкнення LEAKNESS режиму.
	// https://pkg.go.dev/github.com/go-rod/rod@v0.79.0/lib/launcher#Launcher.Leakless
	if os.Getenv("DISABLE_LEAKLESS") == "1" {
		l = l.Leakless(false)
	}

	cs, err := l.Launch()
	if err != nil {
		return err
	}

	chn.browser = rod.New().ControlURL(cs).MustConnect()
	chn.page = chn.browser.MustPage()

	return nil
}

// Переходимо по URL
func (chn *ChromeNavigator) gotoUrl(url string) error {
	var errLoadPage error

	for i := 0; i < chn.getTriesCount(); i++ {

		if err := chn.checkIfClientExist(); err != nil {
			return err
		}

		if errLoadPage = chn.page.Navigate(url); errLoadPage != nil {
			chn.Close()
			continue
		}

		if errLoadPage = chn.WaitPageLoad(); errLoadPage != nil {
			chn.Close()
			continue
		}

		if chn.model.BeatRecaptcha() {
			if errLoadPage = chn.HandleRecaptcha(); errLoadPage != nil {
				chn.Close()
				continue
			}
		}

		if html, err := chn.page.HTML(); err == nil {
			errLoadPage = chn.createCrawler(html)
		} else {
			errLoadPage = err
		}

		if errLoadPage == nil {
			return nil
		} else {
			chn.Close()
		}
	}

	return errLoadPage
}

func (chn *ChromeNavigator) WaitPageLoad() error {

	e := proto.NetworkResponseReceived{}
	wait := chn.page.WaitEvent(&e)
	waitIdle := chn.page.WaitNavigation(proto.PageLifecycleEventNameLoad)

	log.Println("Wait for response")

	if err := chn.waitPage(wait); err != nil {
		return err
	}

	chn.CommonNavigator.NavigateStatus = e.Response.Status
	log.Println("Status", e.Response.Status)

	log.Println("Wait for page load")
	if err := chn.waitPage(waitIdle); err != nil {
		return err
	}
	log.Println("Loaded")

	return nil
}

// Чекаэмо завантаження сторінки
func (chn *ChromeNavigator) waitPage(wait func()) error {
	timeout := time.NewTimer(time.Second * time.Duration(TIMEOUT_ERROR))
	ch := make(chan interface{}, 1)

	go chn.waitPageEventLoad(wait, ch)

	select {
	case <-timeout.C:
		return errors.New("timeout_navigation")
	case <-ch:
		return nil
	}
}

func (chn *ChromeNavigator) waitPageEventLoad(wait func(), ch chan interface{}) {
	wait()
	ch <- nil
}
