package cloudflare

import "os"

const (
	CLOUDFLARE_SOLVER_2CAPTCHA uint8 = iota
	CLOUDFLARE_SOLVER_CAP_MONSTER
)

type Solver struct {
	// Ключ API для сервісу 2captcha
	apiKey string

	// Селектор для ознаки наявності капчі
	captchaSelector string

	scriptSitekey string

	scriptSolve string

	// Сервіс для розв'язання капчі
	solverType uint8
}

func New(apiKey string) *Solver {
	// solverType - по замовчуванню 2captcha.
	// Тип солвера можна змінити за допомогою SetSolverType або через змінні оточення.
	//
	// CLOUDFLARE_SOLVER_2CAPTCHA=1 - використовувати 2captcha.
	// CLOUDFLARE_SOLVER_CAP_MONSTER=1 - використовувати CapMonster.
	var solverType uint8 = CLOUDFLARE_SOLVER_2CAPTCHA

	if os.Getenv("CLOUDFLARE_SOLVER_2CAPTCHA") == "1" {
		solverType = CLOUDFLARE_SOLVER_2CAPTCHA
	} else if os.Getenv("CLOUDFLARE_SOLVER_CAP_MONSTER") == "1" {
		solverType = CLOUDFLARE_SOLVER_CAP_MONSTER
	}

	return &Solver{
		apiKey:     apiKey,
		solverType: solverType,
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

func (s *Solver) SetSolverType(solverType uint8) *Solver {
	s.solverType = solverType
	return s
}
