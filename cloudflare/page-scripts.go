package cloudflare

import (
	"errors"
	"fmt"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
)

type CloudflareData struct {
	Data       string
	Standalone bool
}

func (s *Solver) getCloudflareData(page *rod.Page, response chan *CloudflareData) (dataStr string, standalone bool, err error) {
	defer close(response)
	defer func() {
		if r := recover(); r != nil {
			err = errors.New("panic getting cloudflare data")
		}
	}()

	if page == nil {
		return "", false, errors.New("page is nil")
	}

	for range 10 {
		_, _ = page.Activate()

		time.Sleep(time.Second)

		data, err := page.Eval(`() => cloudflareData`)
		if err == nil && data != nil {
			dataValue := data.Value.String()
			if len(dataValue) > 100 {
				response <- &CloudflareData{Data: dataValue, Standalone: false}
				return dataValue, false, nil
			}
		} else if s.scriptSitekey != "" {
			script := fmt.Sprintf(`() => {
				%s
			}`, s.scriptSitekey)
			dataValue, errValue := page.Eval(script)

			if errValue == nil && dataValue != nil {
				value := dataValue.Value.Str()
				response <- &CloudflareData{Data: value, Standalone: false}
				return value, true, nil
			}
		}

		_ = page.Reload()
		if err := s.waitReload(page); err != nil {
			return "", false, err
		}

	}

	return "", false, errors.New("cloudflare data not found")
}

func (s *Solver) resolveToken(page *rod.Page, standalone bool, token string, clfData map[string]any) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = errors.New("panic resolving token")
		}
	}()

	if page == nil {
		return errors.New("page is nil")
	}

	if standalone && s.scriptSolve != "" {
		script := fmt.Sprintf(`async (response, clfData = {}) => {
			%s
		}`, s.scriptSolve)
		_, err := page.Eval(script, token, clfData)
		if err != nil {
			return err
		}
	} else {
		if _, err := page.Eval(`token => tsCallback(token)`, token); err != nil {
			return err
		}
	}

	if s.reloadFunction != nil {
		return s.reloadFunction()
	}

	return s.waitReload(page)
}

func (s *Solver) waitReload(page *rod.Page) error {
	if page == nil {
		return errors.New("page is nil")
	}

	loaded := make(chan any, 1)

	go func(p *rod.Page) {
		defer func() {
			recover()
		}()
		p.EachEvent(func(e *proto.PageLoadEventFired) (stop bool) {
			select {
			case loaded <- nil:
			default:
			}
			return true
		})()
	}(page)

	timer := time.NewTimer(time.Second * 60)
	defer timer.Stop()

	select {
	case <-loaded:
		return nil
	case <-timer.C:
		return errors.New("timeout waiting for page to load")
	}
}
