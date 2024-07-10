package cloudflare

type Solver struct {
	// Ключ API для сервісу 2captcha
	apiKey string

	// Селектор для ознаки наявності капчі
	captchaSelector string
}

func New(apiKey string) *Solver {
	return &Solver{
		apiKey: apiKey,
	}
}

func (s *Solver) SetCaptchaSelector(selector string) *Solver {
	s.captchaSelector = selector
	return s
}
