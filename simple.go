package navigator

import (
	"errors"
	"io"
	"log"
	"net/http"
	"net/url"
	"time"
)

type SimpleNavigator struct {
	CustomNavigator

	Client *http.Client
}

func (s *SimpleNavigator) NavigateUrl(url string) error {
	if err := s.gotoUrl(url); err != nil {
		return err
	}

	return nil
}

func (s *SimpleNavigator) gotoUrl(url string) error {
	var err error
	for try := 0; try < NAVIGATION_MAX_TRIES_COUNT; try++ {
		log.Printf("Try number %d\n", try)
		if try > 0 && s.DontTryAnyMore {
			err = errors.New("cannot_load_page")
			break
		}

		if try > 0 {
			s.DestroyClient(true)
		}

		s.CreateClientIfNeed()

		resp, err := s.Client.Get(url)
		if err != nil {
			s.StatusCode = -1
			s.DestroyClient(true)
			return err
		}
		html, err := io.ReadAll(resp.Body)
		if err != nil {
			s.StatusCode = -1
			s.DestroyClient(true)
			return err
		}
		s.CreateCrawler(string(html))

		if s.ban() {
			s.StatusCode = 0
			continue
		} else {
			return nil
		}
	}
	s.DontTryAnyMore = true
	return err
}
func (s *SimpleNavigator) CreateClientIfNeed() {
	if s.Client != nil {
		return
	}
	tr := &http.Transport{
		MaxIdleConns:    10,
		IdleConnTimeout: 30 * time.Second,
	}
	if s.useProxy() {
		tr.Proxy = s.getProxy
	}
	s.Client = &http.Client{Transport: tr}
}

func (s *SimpleNavigator) getProxy(*http.Request) (*url.URL, error) {
	return url.Parse(s.GetProxy())
}

func (s *SimpleNavigator) DestroyClient(must ...bool) {
	s.Client = nil
	if len(must) > 0 && must[0] {
		s.CurrentProxy = ""
	}
}
