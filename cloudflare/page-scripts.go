package cloudflare

import (
	"errors"
	"fmt"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
)

func (s *Solver) getCloudflareData(page *rod.Page) (string, bool, error) {

	for i := 0; i < 10; i++ {
		page.Activate()

		time.Sleep(time.Second)

		data, err := page.Eval(`() => cloudflareData`)
		if err == nil {
			dataValue := data.Value.String()
			if len(dataValue) > 100 {
				return dataValue, false, nil
			}
		} else if s.scriptSitekey != "" {
			script := fmt.Sprintf(`() => {
				%s
			}`, s.scriptSitekey)
			dataValue, errValue := page.Eval(script)

			if errValue == nil {
				return dataValue.Value.Str(), true, nil
			}
		}

		page.Reload()
		if err := s.waitReload(page); err != nil {
			return "", false, err
		}

	}

	return "", false, errors.New("cloudflare data not found")
}

func (s *Solver) resolveToken(page *rod.Page, standalone bool, token string) error {
	if standalone && s.scriptSolve != "" {
		script := fmt.Sprintf(`async (response) => {
			%s
		}`, s.scriptSolve)
		_, err := page.Eval(script, token)
		if err != nil {
			return err
		}
	} else {
		if _, err := page.Eval(`token => tsCallback(token)`, token); err != nil {
			return err
		}
	}

	return s.waitReload(page)
}

func (s *Solver) waitReload(page *rod.Page) error {
	loaded := make(chan any, 1)

	go page.EachEvent(func(e *proto.PageLoadEventFired) (stop bool) {
		loaded <- nil
		return false
	})()

	select {
	case <-loaded:
		return nil
	case <-time.After(time.Second * 60):
		return errors.New("timeout waiting for page to load")
	}
}
