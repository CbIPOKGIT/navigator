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

	var token string
	var errToken error

	switch s.solverType {
	case CLOUDFLARE_SOLVER_2CAPTCHA:
		if task, err := s.create2CaptchaTask(data); err == nil {
			token, errToken = s.get2CaptchaTaskResult(task)
		} else {
			errToken = err
		}

	case CLOUDFLARE_SOLVER_CAP_MONSTER:
		if task, err := s.createCapMonsterTask(data); err == nil {
			token, errToken = s.getCapMonsterTaskResult(task)
		} else {
			errToken = err
		}
	default:
		errToken = errors.New("unknown solver type")
	}

	if errToken != nil {
		return errToken
	}

	return s.resolveToken(page, standalone, token, clfData)
}
