package cloudflare

import (
	"encoding/json"
	"errors"

	"github.com/go-rod/rod"
)

func (s *Solver) Solve(page *rod.Page) error {
	page.Activate()

	data, standalone, err := s.getCloudflareData(page)
	if err != nil {
		return err
	}

	clfData := make(map[string]any)

	if standalone {
		el, err := page.Search(".main-wrapper")
		if err != nil {
			return err
		}

		if el == nil {
			return errors.New("cloudflare: main-wrapper element not found")
		}
		data, err := el.First.Eval(`() => JSON.stringify(window._cf_chl_opt)`)
		if err != nil {
			return err
		}

		if err := json.Unmarshal([]byte(data.Value.Str()), &clfData); err != nil {
			return err
		}
	}

	// task, err := s.createTask(data)
	task, err := s.createCapMonsterTask(data)
	if err != nil {
		return err
	}

	token, err := s.getCapMonsterTaskResult(task)
	if err != nil {
		return err
	}

	return s.resolveToken(page, standalone, token, clfData)
}
