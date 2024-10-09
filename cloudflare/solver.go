package cloudflare

type Solver struct {
	// Ключ API для сервісу 2captcha
	apiKey string

	// Селектор для ознаки наявності капчі
	captchaSelector string

	scriptSitekey string

	scriptSolve string
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

func (s *Solver) SetSitekeyScript(script string) {
	s.scriptSitekey = script
}

func (s *Solver) SetSolveScript(script string) {
	s.scriptSolve = script
}
