package navigator

import (
	"crypto/tls"
	"errors"
	"fmt"
	"log"
	"net/url"
	"time"

	"github.com/PuerkitoBio/goquery"
	"resty.dev/v3"
)

type GentelmanNavigator struct {
	CommonNavigator

	Client *resty.Client
}

func (navigator *GentelmanNavigator) Navigate(url string) error {
	if err := navigator.writeAndFormatURL(url); err != nil {
		return err
	}

	navigator.initEmptyCrawler()

	return navigator.navigateUrl()
}

func (navigator *CommonNavigator) GetCrawler() *goquery.Document {
	if navigator.Crawler == nil {
		navigator.initEmptyCrawler()
	}
	return navigator.Crawler
}

func (navigator *GentelmanNavigator) Close() error {
	var err error = nil

	if navigator.Client != nil {
		err = navigator.Client.Close()
		navigator.Client = nil
	}
	return err
}

func (navigator *GentelmanNavigator) GetActualUrl() string {
	return navigator.GetUrl()
}

func (navigator *GentelmanNavigator) navigateUrl() error {
	var i int

	for i = 0; i < navigator.calculateTriesCount(); i++ {
		navigator.LastError = nil

		if i > 0 {
			navigator.Close()
		}
		navigator.createClientIfNotExist()

		if !navigator.JustCreated && navigator.Model.DelayBeforeNavigate > 0 {
			time.Sleep(time.Second * time.Duration(navigator.Model.DelayBeforeNavigate))
		}

		navigator.JustCreated = false

		response, err := navigator.Client.R().Get(navigator.Uri.String())
		if err != nil {
			log.Println(err)
			navigator.LastError = errors.New("error navigate")
			continue
		}

		navigator.NavigateStatus.Store(int32(response.StatusCode()))

		if err := navigator.СreateCrawlerFromHTML(response.String()); err != nil {
			navigator.LastError = fmt.Errorf("error create crawler from HTML: %s", err.Error())
			continue
		}

		if navigator.isValidResponse(navigator.NavigateStatus.Load()) {
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

	client := resty.New().
		SetTLSClientConfig(&tls.Config{InsecureSkipVerify: true}).
		SetTimeout(navigator.calculateNavigationTimeout())

	navigator.CurrentProxy = nil
	if navigator.PrxGetter != nil {
		if proxyvalue, err := navigator.PrxGetter.GetProxy(); err == nil && proxyvalue != "" {
			if u, err := url.Parse(proxyvalue); err == nil {
				client.SetProxy(u.String())

				if u.User != nil {
					username := u.User.Username()
					password, _ := u.User.Password()

					if username != "" && password != "" {
						client.SetBasicAuth(username, password)
					}

				}

				navigator.CurrentProxy = u
			}
		}
	}

	if navigator.Model.BlockRedirects {
		client.SetRedirectPolicy(resty.NoRedirectPolicy())
	}

	if len(navigator.Model.InitialCookies) > 0 {
		client.SetCookies(navigator.Model.InitialCookies)
	}

	if navigator.Model.UserAgent != "" {
		client.SetHeader("User-Agent", navigator.Model.UserAgent)
	}

	navigator.Client = client
	navigator.JustCreated = true
}
