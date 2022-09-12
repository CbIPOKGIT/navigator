package navigator

import (
	"errors"
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

		e := proto.NetworkResponseReceived{}
		wait := chn.page.WaitEvent(&e)

		if errLoadPage = chn.page.Navigate(url); errLoadPage != nil {
			continue
		}

		if errLoadPage = chn.waitPageLoad(wait); errLoadPage != nil {
			continue
		}

		chn.CommonNavigator.NavigateStatus = e.Response.Status

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

// Чекаэмо завантаження сторінки
func (chn *ChromeNavigator) waitPageLoad(wait func()) error {
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
