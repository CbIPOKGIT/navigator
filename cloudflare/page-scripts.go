package cloudflare

import (
	"errors"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
)

func (s *Solver) getCloudflareData(page *rod.Page) (string, error) {
	time.Sleep(time.Second)
	data, err := page.Eval(`() => cloudflareData`)
	if err != nil {
		return "", err
	}

	return data.Value.Str(), nil
}

func (s *Solver) resolveToken(page *rod.Page, token string) error {
	if _, err := page.Eval(`token => tsCallback(token)`, token); err != nil {
		return err
	}

	loaded := make(chan any, 1)

	go page.EachEvent(func(e *proto.PageLoadEventFired) (stop bool) {
		loaded <- nil
		return false
	})()

	select {
	case <-loaded:
		return nil
	case <-time.After(time.Second * 30):
		return errors.New("timeout waiting for page to load")
	}
}
