package navigator

import (
	"crypto/tls"
	"errors"
	"fmt"
	"log"
	"net/url"
	"time"

	"gopkg.in/h2non/gentleman.v2"
	"gopkg.in/h2non/gentleman.v2/plugins/proxy"
	gtls "gopkg.in/h2non/gentleman.v2/plugins/tls"
)

type GentelmanNavigator struct {
	CommonNavigator

	Client *gentleman.Client
}

func (navigator *GentelmanNavigator) Navigate(url string) error {
	if err := navigator.writeAndFormatURL(url); err != nil {
		return err
	}

	navigator.initEmptyCrawler()

	return navigator.navigateUrl()
}

func (navigator *GentelmanNavigator) Close() error {
	navigator.destoyClient()
	return nil
}

func (navigator *GentelmanNavigator) navigateUrl() error {
	var i int

	for i = 0; i < navigator.calculateTriesCount(); i++ {
		navigator.LastError = nil

		if i > 0 {
			navigator.destoyClient()
		}
		navigator.createClientIfNotExist()

		request := navigator.Client.Request().URL(navigator.Url)
		if navigator.Model.UserAgent != "" {
			request.AddHeader("user-agent", navigator.Model.UserAgent)
		}

		if !navigator.JustCreated && navigator.Model.DelayBeforeNavigate > 0 {
			time.Sleep(time.Second * time.Duration(navigator.Model.DelayBeforeNavigate))
		}

		navigator.JustCreated = false

		response, err := request.Send()
		if err != nil {
			log.Println(err)
			navigator.LastError = errors.New("Error navigate")
			continue
		}

		navigator.NavigateStatus = response.StatusCode

		if err := navigator.createCrawlerFromHTML(response.String()); err != nil {
			navigator.LastError = errors.New(fmt.Sprintf("Error create crawler from HTML: %s", err.Error()))
			continue
		}

		if navigator.isValidResponse(navigator.NavigateStatus) {
			navigator.NoMoreTry = false
			break
		}
	}

	if i == navigator.calculateTriesCount() && navigator.PrxGetter != nil {
		navigator.PrxGetter = nil
	}

	return navigator.LastError
}

// Create new client if not exist
func (navigator *GentelmanNavigator) createClientIfNotExist() {
	if navigator.Client != nil {
		return
	}

	client := gentleman.New()
	client.Use(gtls.Config(&tls.Config{InsecureSkipVerify: true}))
	client.Context.Client.Timeout = time.Second * 30

	if navigator.PrxGetter != nil {
		if proxyvalue, err := navigator.PrxGetter.GetProxy(); err == nil {
			if u, err := url.Parse("http://" + proxyvalue); err == nil {
				client.Use(proxy.Set(map[string]string{
					"http":  u.String(),
					"https": u.String(),
				}))
			}
		}
	} else {
		client.Use(proxy.Set(map[string]string{}))
	}

	if navigator.Model.InitialCookies != nil && len(navigator.Model.InitialCookies) > 0 {
		client.AddCookies(navigator.Model.InitialCookies)
	}

	navigator.Client = client
	navigator.JustCreated = true
}

func (navigator *GentelmanNavigator) destoyClient() {
	if navigator.Client != nil {
		navigator.Client = nil
	}
}
