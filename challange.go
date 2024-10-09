package navigator

import (
	"errors"
	"time"

	"github.com/go-rod/rod"
)

const (
	CHALLENGE_SOLVE_DURATION = time.Minute * 3
)

// CloudflareSolver - interface for Cloudflare protection solver
type CloudflareSolver interface {
	Is(*rod.Page) bool // check if page has Cloudflare protection

	Solve(*rod.Page) error // solve Cloudflare protection

	SetSitekeyScript(string) // set script to get sitekey

	SetSolveScript(string) // set script to solve challenge
}

// beatChallange - beat the challange. Its something like Cloudflare protection.
//
// Max reloads count - 5
func (n *ChromeNavigator) beatChallange() error {
	if n.ClfSolver != nil && n.ClfSolver.Is(n.Page) {
		if err := n.ClfSolver.Solve(n.Page); err == nil {
			n.NavigateStatus = 200
			return nil
		} else {
			return err
		}
	}

	if !n.hasChallenge() {
		return nil
	}

	errChannel := make(chan error, 1)
	go n.waitReloads(errChannel)

	select {
	case err := <-errChannel:
		return err
	case <-time.After(CHALLENGE_SOLVE_DURATION):
		return errors.New("timeout challenge solve")
	}
}

func (n *ChromeNavigator) waitReloads(response chan error) {
	var step int = 0

	for step < 5 {
		if n.hasChallenge() {
			response <- nil
			return
		}

		if err := n.waitResponseAndLoad(); err != nil {
			response <- err
			return
		}
	}
}

func (n *ChromeNavigator) hasChallenge() bool {
	return n.Model.ChallangeSelector != "" && n.hasChallengeElement(n.Model.ChallangeSelector)
}

func (n *ChromeNavigator) hasChallengeElement(selector string) bool {
	elements, err := n.Page.Elements(selector)
	return err == nil && len(elements) > 0
}
