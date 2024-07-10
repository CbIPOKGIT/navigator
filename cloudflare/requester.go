package cloudflare

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

type TwoCaptchaTaskBody struct {
	Key  string            `json:"clientKey"`
	Task map[string]string `json:"task"`
}

type TwoCaptchaTaskResponse struct {
	Error int    `json:"errorId"`
	Task  uint64 `json:"taskId"`
}

type TwoCaptchaResultResponse struct {
	Error    int    `json:"errorId"`
	Status   string `json:"status"`
	Solution struct {
		Token     string `json:"token"`
		Useragent string `json:"userAgent"`
	} `json:"solution"`
}

func (s *Solver) createTask(taskData string) (uint64, error) {
	if s.apiKey == "" {
		return 0, errors.New("apiKey is not set")
	}

	task := make(map[string]string)
	if err := json.Unmarshal([]byte(taskData), &task); err != nil {
		return 0, err
	}

	body := &TwoCaptchaTaskBody{
		Key:  s.apiKey,
		Task: task,
	}

	data, err := json.Marshal(body)
	if err != nil {
		return 0, err
	}

	request, err := http.Post("https://api.2captcha.com/createTask", "application/json", bytes.NewBuffer(data))
	if err != nil {
		return 0, err
	}
	defer request.Body.Close()

	content, err := io.ReadAll(request.Body)
	if err != nil {
		return 0, err
	}

	response := &TwoCaptchaTaskResponse{}
	if err := json.Unmarshal(content, response); err != nil {
		return 0, err
	}

	if response.Error != 0 {
		return 0, fmt.Errorf("2Captcha error: %d", response.Error)
	}

	return response.Task, nil
}

func (s *Solver) getTaskResult(task uint64) (string, error) {
	for i := 0; i < 12; i++ {
		<-time.After(time.Second * 10)

		body := map[string]any{
			"clientKey": s.apiKey,
			"taskId":    task,
		}

		data, err := json.Marshal(body)
		if err != nil {
			return "", err
		}

		request, err := http.Post("https://api.2captcha.com/getTaskResult", "application/json", bytes.NewBuffer(data))
		if err != nil {
			return "", err
		}

		defer request.Body.Close()

		content, err := io.ReadAll(request.Body)
		if err != nil {
			return "", err
		}

		response := &TwoCaptchaResultResponse{}
		if err := json.Unmarshal(content, response); err != nil {
			return "", err
		}

		if response.Error != 0 {
			return "", fmt.Errorf("2Captcha solve error: %d", response.Error)
		}

		if response.Status == "ready" {
			return response.Solution.Token, nil
		}
	}

	return "", errors.New("2Captcha task timeout")

}
