package navigator

import (
	"crypto/tls"
	"net/http"
	"time"

	"gopkg.in/h2non/gentleman.v2"
	gtls "gopkg.in/h2non/gentleman.v2/plugins/tls"
)

type HttpClient struct {
	CommonNavigator

	client *gentleman.Client
}

func (htp *HttpClient) Navigate(url string) error {
	if err := htp.parseUrl(url); err != nil {
		return err
	}
	return htp.gotoUrl(url)
}

func (htp *HttpClient) Close() error {
	htp.client = nil
	return nil
}

func (htp *HttpClient) GetClient() *gentleman.Client {
	return htp.client
}

func (htp *HttpClient) SetCookie(name, value string, maxage int) {
	htp.client.AddCookie(&http.Cookie{
		Name:   name,
		Value:  value,
		MaxAge: maxage,
	})
}

// ---------------------------------------------------------------------
func (htp *HttpClient) createClientIfNeed() {
	if htp.client != nil {
		return
	}

	htp.createClient()
}

func (htp *HttpClient) createClient() {
	htp.client = gentleman.New()
	htp.client.Context.Client.Timeout = time.Second * time.Duration(10)
	htp.client.Use(gtls.Config(&tls.Config{
		InsecureSkipVerify: true,
	}))

	return
}

func (htp *HttpClient) gotoUrl(url string) error {
	var errLoadPage error

	for i := 0; i < htp.getTriesCount(); i++ {

		htp.createClientIfNeed()

		req := htp.client.Request()
		req.URL(url)

		res, err := req.Send()
		if err != nil {
			errLoadPage = err
			continue
		}

		htp.CommonNavigator.NavigateStatus = res.StatusCode

		errLoadPage = htp.createCrawler(res.String())

		if errLoadPage == nil {
			return nil
		} else {
			htp.Close()
		}
	}

	return errLoadPage
}
