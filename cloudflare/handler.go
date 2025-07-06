package cloudflare

import (
	"context"
	"errors"
	"time"

	"github.com/go-rod/rod"
)

func (s *Solver) Solve(page *rod.Page, ctxs ...context.Context) error {
	ctx := context.Background()
	if len(ctxs) > 0 {
		for _, c := range ctxs {
			ctx = context.WithoutCancel(c)
		}
	}

	clfResponse := make(chan *CloudflareData, 1)
	var clfData *CloudflareData
	go s.getCloudflareData(page, clfResponse)

	select {
	case <-ctx.Done():
		return errors.New("context canceled")

	case d, e := <-clfResponse:
		if !e {
			return errors.New("failed to get cloudflare data")
		} else {
			clfData = d
		}
	}

	if clfData == nil {
		return errors.New("cloudflare data is nil")
	}

	task, err := s.createOrder(clfData.Data)
	if err != nil {
		return err
	}

	token, err := s.getAnswer(task, ctx)
	if err != nil {
		return err
	}

	return s.resolveToken(page, clfData.Standalone, token, nil)
}

func (s *Solver) createOrder(taskData string) (uint64, error) {
	switch s.solverType {
	case CLOUDFLARE_SOLVER_2CAPTCHA:
		if t, err := s.create2CaptchaTask(taskData); err == nil {
			return t, nil
		} else {
			return 0, err
		}

	case CLOUDFLARE_SOLVER_CAP_MONSTER:
		if t, err := s.createCapMonsterTask(taskData); err == nil {
			return t, nil
		} else {
			return 0, err
		}
	default:
		return 0, errors.New("unknown solver type")
	}
}

func (s *Solver) getAnswer(taskID uint64, ctx context.Context) (string, error) {
	var step int = 0
	interval := time.NewTimer(time.Second * 10)

	var requester func(uint64) (string, error)

	switch s.solverType {
	case CLOUDFLARE_SOLVER_2CAPTCHA:
		requester = s.get2CaptchaTaskResult
	case CLOUDFLARE_SOLVER_CAP_MONSTER:
		requester = s.getCapMonsterTaskResult
	}

	for {
		select {
		case <-ctx.Done():
			return "", errors.New("context canceled")

		case <-interval.C:
			step++
			if step > 12 {
				return "", errors.New("token solve timeout")
			}

			answer, err := requester(taskID)
			if err != nil {
				return "", err
			}
			if answer != "" {
				return answer, nil
			}

			interval.Reset(time.Second * 10)
		}
	}
}
