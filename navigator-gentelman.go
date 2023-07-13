package navigator

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"gopkg.in/h2non/gentleman.v2"
	"gopkg.in/h2non/gentleman.v2/plugins/proxy"
)

type GentelmanNavigator struct {
	CommonNavigator

	Client *gentleman.Client
}

func (navigator *GentelmanNavigator) Navigate(url string) error {
	if err := navigator.writeAndFormatURL(url); err != nil {
		return err
	}

	return navigator.navigateUrl()
}

func (navigator *GentelmanNavigator) Close() error {
	navigator.destoyClient()
	return nil
}

func (navigator *GentelmanNavigator) navigateUrl() error {
	for i := 0; i < navigator.calculateTriesCount(); i++ {
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
			navigator.LastError = err
			continue
		}

		navigator.NavigateStatus = response.StatusCode

		if err := navigator.createCrawlerFromHTML(response.String()); err != nil {
			navigator.LastError = errors.New(fmt.Sprintf("Error create crawler from HTML: %s", err.Error()))
			continue
		}

		if navigator.isValidResponse(i) {
			break
		}
	}

	return navigator.LastError
}

// Create new client if not exist
func (navigator *GentelmanNavigator) createClientIfNotExist() {
	if navigator.Client != nil {
		return
	}

	client := gentleman.New()

	if len(navigator.Model.BanSettings) > 0 && navigator.PrxGetter != nil {
		proxyvalue, err := navigator.PrxGetter.GetProxy()
		if err == nil {
			client.Use(proxy.Set(map[string]string{"http": proxyvalue}))
		}
	}

	if navigator.Model.InitialCookies != nil && len(navigator.Model.InitialCookies) > 0 {
		cookies := make([]*http.Cookie, 0, len(navigator.Model.InitialCookies))
		for key, val := range navigator.Model.InitialCookies {
			cookies = append(cookies, &http.Cookie{
				Name:  key,
				Value: val,
			})
		}
	}

	navigator.Client = client
	navigator.JustCreated = true
}

func (navigator *GentelmanNavigator) destoyClient() {
	if navigator.Client != nil {
		navigator.Client = nil
	}
}
