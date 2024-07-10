package cloudflare

import (
	"github.com/go-rod/rod"
)

func (s *Solver) Solve(page *rod.Page) error {
	page.Activate()

	data, err := s.getCloudflareData(page)
	if err != nil {
		return err
	}

	task, err := s.createTask(data)
	if err != nil {
		return err
	}

	token, err := s.getTaskResult(task)
	if err != nil {
		return err
	}

	return s.resolveToken(page, token)
}
